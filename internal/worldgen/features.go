package worldgen

import (
	"sort"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// This file places trees and bushes: the first content generated *on*
// terrain rather than as terrain (docs/milestones/M18-vegetation-features.md).
//
// Two properties are non-negotiable, per the milestone's own framing, and
// every design choice below exists to hold them:
//
//   - Purity: a feature's existence and shape depend only on (worldSeed,
//     its own root chunk coordinate), never on a running RNG and never on
//     which neighbouring chunks happen to be loaded. candidateRoots is the
//     only place randomness enters, and it is a pure function of exactly
//     those two inputs.
//   - Boundedness: every feature declares a maximum horizontal radius
//     (treeRadius, bushRadius), and generating one chunk considers every
//     candidate rooted within featureChunkRadius chunks of it — no further,
//     and no feature's shape function may ever produce an offset beyond its
//     declared radius. TestFeatureRadiusIsEnforced and
//     TestGenerate_FeaturesNeverWriteOutsideDeclaredRadius both assert this
//     directly rather than relying on the shape functions being reviewed
//     carefully.

// featureKind is what an accepted root becomes.
type featureKind uint8

const (
	featureTree featureKind = iota
	featureBush
)

const (
	// treeRadius is the largest horizontal (Chebyshev) distance from a
	// tree's root column that treeShape ever writes to. It is a property of
	// the feature, not a guess: TestFeatureRadiusIsEnforced checks every
	// offset treeShape can produce, for every trunk height in
	// [minTrunkHeight, minTrunkHeight+trunkHeightRange), against this
	// constant.
	treeRadius = 2

	// bushRadius is bushShape's equivalent bound.
	bushRadius = 1

	// maxFeatureRadius is the largest radius any feature declares. It is
	// what featureChunkRadius is derived from.
	maxFeatureRadius = treeRadius

	// minTrunkHeight and trunkHeightRange bound a tree's randomly chosen
	// trunk height to [minTrunkHeight, minTrunkHeight+trunkHeightRange): 4,
	// 5 or 6 blocks. This does not affect treeRadius — every layer in
	// treeShape's canopy has a fixed horizontal radius regardless of trunk
	// height, only the number of trunk blocks below it changes.
	minTrunkHeight   = 4
	trunkHeightRange = 3

	// treeChanceNumerator / treeChanceDenominator is the fraction of accepted
	// roots that become trees rather than bushes (2/5 = 40%). Expressed as
	// two integers rather than a float so the per-candidate coin flip
	// (candidateRoots) is an exact `state % treeChanceDenominator <
	// treeChanceNumerator` on the raw splitmix64 draw, with no float
	// conversion and no rounding in the hot path.
	treeChanceNumerator   = 2
	treeChanceDenominator = 5

	// attemptsPerChunk is how many candidate roots each chunk proposes on
	// its own, before Poisson-disk rejection. Kept small: at minSpacing a
	// 16x16 chunk cannot fit many accepted roots regardless, and every
	// neighbouring chunk's generation re-derives this same chunk's
	// candidates from scratch (see rootsAffecting), so proposing more than
	// could ever survive rejection just multiplies wasted work across the
	// whole neighbourhood.
	attemptsPerChunk = 3

	// minSpacing is the minimum centre-to-centre distance, in blocks,
	// between two accepted roots — the Poisson-disk radius. It is chosen
	// comfortably larger than 2*maxFeatureRadius so two accepted features'
	// declared radii can never overlap (validation criterion 5), with margin
	// to spare.
	minSpacing = 6.0

	// featureChunkRadius is how many chunks in every direction, beyond the
	// one being generated, must be considered for roots whose geometry could
	// reach in: ceil(maxFeatureRadius / config.ChunkWidth). maxFeatureRadius
	// (2) is far inside one chunk width (16), so this is 1 — matching the
	// milestone's own "(2r+1)^2 at r=1" estimate for the extra generation
	// cost.
	featureChunkRadius = 1

	// spacingChunkRadius is the same ceiling division applied to minSpacing:
	// how many chunks away a candidate that could reject (or be rejected by)
	// another one can be.
	spacingChunkRadius = 1

	// poolRadius is how far the candidate pool gathered when generating one
	// chunk must reach. Deciding the fate of a root up to featureChunkRadius
	// chunks away requires knowing about every earlier-ordered candidate
	// within minSpacing of it, which can itself be up to spacingChunkRadius
	// chunks further out — so the pool must cover both radii from the chunk
	// being generated. See rootsAffecting.
	poolRadius = featureChunkRadius + spacingChunkRadius
)

func init() {
	// Defensive: if these constants are ever retuned independently, a
	// mismatch here would silently reintroduce exactly the correctness bug
	// the milestone's "feature radius is declared, bounded, and enforced"
	// design decision exists to prevent — a feature or a spacing check
	// reaching further than the chunk-radius constants assume. Panicking at
	// package init turns that into a build-time-obvious failure instead of a
	// silent one, matching how NewGenerator already panics on a malformed
	// spline rather than producing subtly wrong terrain.
	if maxFeatureRadius > featureChunkRadius*config.ChunkWidth {
		panic("worldgen: maxFeatureRadius exceeds featureChunkRadius*ChunkWidth")
	}
	if minSpacing > float64(spacingChunkRadius*config.ChunkWidth) {
		panic("worldgen: minSpacing exceeds spacingChunkRadius*ChunkWidth")
	}
	if minSpacing <= 2*maxFeatureRadius {
		panic("worldgen: minSpacing does not leave features room to avoid overlapping")
	}
}

// featureBlock is one block a feature places, relative to its root column
// and the ground height at that column: (dx, dz) is the horizontal offset
// from the root, dy is the vertical offset from the root's own
// Generator.SurfaceHeight.
//
// Expressing a feature's shape this way — offsets from its own root, with no
// reference to any chunk — is what makes the shape testable on its own
// (TestFeatureRadiusIsEnforced calls treeShape/bushShape directly) and reused
// unchanged for however many chunks a root's geometry happens to cross.
type featureBlock struct {
	dx, dy, dz int
	block      *blocks.Block
}

// treeShape returns every block a tree with the given trunk height places,
// relative to its root: a solid trunk of Wood from the ground up, then a
// tapered canopy of Leaves around its top. It never returns an offset whose
// horizontal (Chebyshev) distance from the root exceeds treeRadius —
// TestFeatureRadiusIsEnforced checks this for every trunk height the
// generator can produce.
func treeShape(trunkHeight int) []featureBlock {
	out := make([]featureBlock, 0, trunkHeight+4*13)

	for dy := 0; dy < trunkHeight; dy++ {
		out = append(out, featureBlock{0, dy, 0, blocks.Wood})
	}

	top := trunkHeight - 1
	// Layers taper: radius 1 at the bottom and top of the canopy, radius 2
	// (with corners trimmed for a rounder silhouette) through the middle.
	// The widest layers set treeRadius.
	layers := []struct{ dy, radius int }{
		{top - 2, 1},
		{top - 1, 2},
		{top, 2},
		{top + 1, 1},
	}
	for _, layer := range layers {
		for dx := -layer.radius; dx <= layer.radius; dx++ {
			for dz := -layer.radius; dz <= layer.radius; dz++ {
				if dx == 0 && dz == 0 && layer.dy <= top {
					continue // never overwrite the trunk column
				}
				if layer.radius == 2 && absOffset(dx) == 2 && absOffset(dz) == 2 {
					continue // trim the outer corners
				}
				out = append(out, featureBlock{dx, layer.dy, dz, blocks.Leaves})
			}
		}
	}
	return out
}

// bushShape returns every block a bush places, relative to its root: a
// low, two-layer blob of Leaves with no trunk. It never returns an offset
// whose horizontal distance from the root exceeds bushRadius.
func bushShape() []featureBlock {
	out := make([]featureBlock, 0, 2*9)
	layers := []int{0, 1}
	for _, dy := range layers {
		for dx := -bushRadius; dx <= bushRadius; dx++ {
			for dz := -bushRadius; dz <= bushRadius; dz++ {
				if absOffset(dx) == 1 && absOffset(dz) == 1 {
					continue // trim corners for a rounder bush
				}
				out = append(out, featureBlock{dx, dy, dz, blocks.Leaves})
			}
		}
	}
	return out
}

// rootCandidate is one proposed feature root, before Poisson-disk rejection.
type rootCandidate struct {
	coord world.ChunkCoord
	// order distinguishes candidates rooted in the same chunk and, together
	// with coord, gives every candidate a well-defined position in the
	// canonical total order rootsAffecting sorts by — see before.
	order       int
	lx, lz      int
	kind        featureKind
	trunkHeight int // meaningful only when kind == featureTree
}

func (r rootCandidate) globalX() int { return r.coord.X*config.ChunkWidth + r.lx }
func (r rootCandidate) globalZ() int { return r.coord.Z*config.ChunkWidth + r.lz }

// before defines the canonical total order candidates are considered in for
// Poisson-disk rejection: by chunk (X then Z), then by index within the
// chunk.
//
// This is what makes acceptance independent of which chunk asked. Deciding
// whether candidate A survives only ever consults candidates that are
// "before" A in this order — never the reverse — so the same pool, gathered
// from any chunk whose neighbourhood happens to contain both A and its
// rejectors, produces the same verdict for A. See rootsAffecting's doc
// comment for the radius argument that guarantees the pool always does
// contain them.
func (r rootCandidate) before(o rootCandidate) bool {
	if r.coord.X != o.coord.X {
		return r.coord.X < o.coord.X
	}
	if r.coord.Z != o.coord.Z {
		return r.coord.Z < o.coord.Z
	}
	return r.order < o.order
}

// tooClose reports whether two candidates are within minSpacing of each
// other, centre to centre.
func tooClose(a, b rootCandidate) bool {
	dx := float64(a.globalX() - b.globalX())
	dz := float64(a.globalZ() - b.globalZ())
	return dx*dx+dz*dz < minSpacing*minSpacing
}

// splitmix64Step advances a 64-bit state by one splitmix64 step. Duplicated
// from the algorithm internal/noise.NewSeed uses (that package's own copy is
// unexported, and unlike NewSeed's use — mixing a single seed into a
// permutation table — featureSeed64 below needs to fold three independent
// integers together first), for the same reason noise.NewSeed uses it:
// adjacent inputs must produce unrelated outputs, or adjacent chunk
// coordinates would produce visibly correlated feature layouts.
func splitmix64Step(z uint64) uint64 {
	z += 0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// featureSeed64 derives a 64-bit stream seed from the world seed and a chunk
// coordinate, well-mixed so that adjacent chunks — which differ by 1 in a
// single coordinate — do not produce visibly related candidate sets. cx and
// cz are folded in with different multipliers so (cx, cz) and (cz, cx) do not
// collide.
func featureSeed64(seed int64, cx, cz int) uint64 {
	s := uint64(seed)
	s = splitmix64Step(s ^ uint64(uint32(cx))*0x9E3779B97F4A7C15)
	s = splitmix64Step(s ^ uint64(uint32(cz))*0xC2B2AE3D27D4EB4F)
	return s
}

// newChunkRNGState returns the seed for a chunk's own splitmix64 stream: the
// sole source of randomness for feature placement, and the sole reason
// placement is deterministic. It must never be shared across chunks and
// never advanced by anything other than that chunk's own candidateRoots
// call, or two chunks generated in different orders could observe different
// draws from a shared stream -- precisely the load-order dependency the
// milestone singles out as the standard way feature placement goes wrong.
//
// This returns a plain uint64 rather than a math/rand/v2 *rand.Rand
// deliberately: rootsAffecting's pool builds one of these per chunk in its
// neighbourhood (up to (2*poolRadius+1)^2 per Generate call), and profiling
// chunk generation after the M18 feature-placement change showed the two
// heap allocations *rand.Rand + its *rand.PCG source cost on every one of
// those constructions as a real, measurable share of the milestone's
// generation-time regression. Stepping splitmix64Step directly needs no
// allocation at all and is exactly as uniform for the tiny number of draws
// (four) each candidate needs.
func newChunkRNGState(seed int64, coord world.ChunkCoord) uint64 {
	return featureSeed64(seed, coord.X, coord.Z)
}

// candidateRoots proposes attemptsPerChunk candidate roots for coord, using
// only (g.worldSeed, coord) — no world access, no shared RNG, no dependence
// on any other chunk. Same inputs, same candidates, forever: this is the
// property every other guarantee about feature placement in this file rests
// on.
func (g *Generator) candidateRoots(coord world.ChunkCoord) []rootCandidate {
	state := newChunkRNGState(g.worldSeed, coord)

	out := make([]rootCandidate, attemptsPerChunk)
	for i := range out {
		state = splitmix64Step(state)
		kind := featureBush
		if state%treeChanceDenominator < treeChanceNumerator {
			kind = featureTree
		}

		state = splitmix64Step(state)
		lx := int(state % config.ChunkWidth)

		state = splitmix64Step(state)
		lz := int(state % config.ChunkWidth)

		state = splitmix64Step(state)
		trunkHeight := minTrunkHeight + int(state%trunkHeightRange)

		out[i] = rootCandidate{
			coord:       coord,
			order:       i,
			lx:          lx,
			lz:          lz,
			kind:        kind,
			trunkHeight: trunkHeight,
		}
	}
	return out
}

// rootsAffecting returns every accepted root — spacing rejection already
// applied — whose chunk lies within featureChunkRadius of coord, i.e. every
// root whose feature might place a block inside coord.
//
// It works by gathering candidates from every chunk within poolRadius of
// coord, running Poisson-disk rejection over that whole pool in the
// canonical order (rootCandidate.before), and keeping the accepted roots
// that landed within featureChunkRadius.
//
// The radius argument, spelled out because it is the one thing here that
// must hold for load-order independence to be real rather than assumed:
// deciding a candidate's fate needs to see every earlier-ordered candidate
// within minSpacing of it. A root within featureChunkRadius of coord can
// itself have such a rejector up to spacingChunkRadius chunks beyond that,
// so the pool must reach poolRadius = featureChunkRadius + spacingChunkRadius
// chunks out — no less, and no chunk further out can affect anything
// featureChunkRadius-close to coord, so no more. Because that pool is
// rebuilt identically regardless of which real chunks happen to be loaded —
// it only ever calls candidateRoots, never reads World — the accepted set
// for any given root is the same no matter which nearby chunk's generation
// triggered the computation. That equality is exactly what
// TestGenerateIsIndependentOfLoadOrder and
// TestFeatures_CrossBoundaryLoadOrderIndependence check.
func (g *Generator) rootsAffecting(coord world.ChunkCoord) []rootCandidate {
	pool := make([]rootCandidate, 0, (2*poolRadius+1)*(2*poolRadius+1)*attemptsPerChunk)
	for dx := -poolRadius; dx <= poolRadius; dx++ {
		for dz := -poolRadius; dz <= poolRadius; dz++ {
			c := world.ChunkCoord{X: coord.X + dx, Z: coord.Z + dz}
			pool = append(pool, g.candidateRoots(c)...)
		}
	}

	sort.Slice(pool, func(i, j int) bool { return pool[i].before(pool[j]) })

	accepted := make([]rootCandidate, 0, len(pool))
	for _, cand := range pool {
		accept := true
		for _, a := range accepted {
			if tooClose(cand, a) {
				accept = false
				break
			}
		}
		if accept {
			accepted = append(accepted, cand)
		}
	}

	out := make([]rootCandidate, 0, len(accepted))
	for _, a := range accepted {
		ddx := a.coord.X - coord.X
		ddz := a.coord.Z - coord.Z
		if ddx >= -featureChunkRadius && ddx <= featureChunkRadius &&
			ddz >= -featureChunkRadius && ddz <= featureChunkRadius {
			out = append(out, a)
		}
	}
	return out
}

// absOffset is the integer absolute value, used throughout this file for
// Chebyshev-style radius checks.
func absOffset(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// paintFeatures writes every root affecting coord (see rootsAffecting) into
// c, clipping each one to the parts that actually land inside c. It is
// called once per Generate, after terrain has already filled c: features are
// content placed *on* terrain, never a replacement for it.
func (g *Generator) paintFeatures(c *world.Chunk, coord world.ChunkCoord) {
	for _, root := range g.rootsAffecting(coord) {
		g.paintRoot(c, coord, root)
	}
}

// paintRoot writes one root's feature geometry into c, translating each of
// the feature's shape offsets from (root-relative) to (global), then to
// (this chunk's local), and skipping anything that lands outside c or that
// would overwrite a block already there (defensive: a correctly-spaced,
// correctly-shaped feature should never need to overwrite anything, but
// silently refusing to is safer than corrupting terrain if some future
// change to spacing or shape ever lets two features' bounds touch).
func (g *Generator) paintRoot(c *world.Chunk, coord world.ChunkCoord, root rootCandidate) {
	groundY := g.SurfaceHeight(root.globalX(), root.globalZ())

	var shape []featureBlock
	switch root.kind {
	case featureTree:
		shape = treeShape(root.trunkHeight)
	case featureBush:
		shape = bushShape()
	}

	for _, fb := range shape {
		gx := root.globalX() + fb.dx
		gz := root.globalZ() + fb.dz
		y := groundY + fb.dy
		placeIfInChunk(c, coord, gx, y, gz, fb.block)
	}
}

// placeIfInChunk writes b at global column (gx, gz), height y, into c only
// if that column belongs to coord, y is in bounds, and the cell is currently
// air. It is the single point where a feature's global-space geometry is
// clipped down to "the parts landing inside this chunk" — the mechanism the
// milestone's "bounded feature radius" design decision calls for.
func placeIfInChunk(c *world.Chunk, coord world.ChunkCoord, gx, y, gz int, b *blocks.Block) {
	cx, lx := world.ChunkAndLocal(gx)
	cz, lz := world.ChunkAndLocal(gz)
	if cx != coord.X || cz != coord.Z {
		return
	}
	if y < 0 || y >= config.ChunkHeight {
		return
	}
	if c.GetBlock(lx, y, lz) != nil {
		return
	}
	c.SetBlock(lx, y, lz, b)
}
