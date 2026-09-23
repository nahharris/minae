package worldgen

import (
	"math"
	"math/rand"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// withDetailDisabled flips enable3DDetail off for the duration of a test,
// restoring it afterwards. This is criterion 1's single switch, exercised
// exactly the way its own doc comment says a test should use it.
func withDetailDisabled(t *testing.T) {
	t.Helper()
	old := enable3DDetail
	enable3DDetail = false
	t.Cleanup(func() { enable3DDetail = old })
}

// referenceFillColumn and referenceGenerate are the pre-M20 generator,
// copied verbatim from what chunk.go's fillColumn/Generate looked like before
// this milestone (git history has the original; this is not a paraphrase).
// TestGenerateReproducesHeightmapExactly compares the real, current Generate
// (with the 3D term switched off) against THIS independent implementation,
// not against itself -- so a bug that crept into both density.go's
// fast-path bookkeeping and some hypothetical "obviously right" self-check
// would still be caught, which comparing Generate's output to its own logic
// restated differently could not do.
func referenceFillColumn(c *world.Chunk, x, z, height int, biome *Biome) {
	for y := 0; y < height; y++ {
		switch {
		case y < height-dirtDepth-1:
			c.SetBlock(x, y, z, blocks.Stone)
		case y < height-1:
			c.SetBlock(x, y, z, biome.Filler)
		default:
			c.SetBlock(x, y, z, biome.Surface)
		}
	}
}

func referenceGenerate(g *Generator, coord world.ChunkCoord) *world.Chunk {
	c := world.NewChunk(coord.X, coord.Z)
	for localX := range config.ChunkWidth {
		globalX := coord.X*config.ChunkWidth + localX
		for localZ := range config.ChunkWidth {
			globalZ := coord.Z*config.ChunkWidth + localZ
			height, climate := g.heightAndClimate(globalX, globalZ)
			biome := g.biomes.Select(climate)
			referenceFillColumn(c, localX, localZ, height, biome)
		}
	}
	g.paintFeatures(c, coord)
	return c
}

// Criterion 1, exact: with the 3D term off, Generate must reproduce the
// pre-M20 generator block for block, over a large sample of chunks and
// several seeds. This is also the test mutation 1 (interpolating the whole
// density, base term included) and mutation 6 below are checked against.
func TestGenerateReproducesHeightmapExactly(t *testing.T) {
	blocks.ResetToVanilla()
	withDetailDisabled(t)

	seeds := []int64{1, 42, -7, -999999}
	checked := 0
	for _, seed := range seeds {
		g := NewGenerator(seed)
		for cx := -3; cx <= 3; cx++ {
			for cz := -3; cz <= 3; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				got := g.Generate(coord)
				want := referenceGenerate(g, coord)
				checked++
				if got.Blocks != want.Blocks {
					t.Fatalf("seed %d chunk (%d,%d): blocks differ from the reference heightmap-only generator with the 3D term disabled",
						seed, cx, cz)
				}
			}
		}
	}
	if checked != len(seeds)*49 {
		t.Fatalf("checked %d chunks, want %d", checked, len(seeds)*49)
	}
}

// Criterion 2 (chunk-seam half), and mutation 3's target directly: two
// adjacent chunks' corner caches must agree exactly on the corners they
// share. Sampling detailCornerValue at LOCAL rather than global coordinates
// -- the bug this guards against -- would make every chunk's cache look
// identical instead, so this also incidentally would catch chunks repeating.
func TestCornerCacheSharedBoundaryMatches(t *testing.T) {
	g := NewGenerator(9)

	pairs := []struct {
		a, b world.ChunkCoord
		// axis along which a and b are adjacent: "x" means b is a's east
		// neighbour, "z" means b is a's south neighbour.
		axis string
	}{
		{world.ChunkCoord{X: 0, Z: 0}, world.ChunkCoord{X: 1, Z: 0}, "x"},
		{world.ChunkCoord{X: -1, Z: 5}, world.ChunkCoord{X: 0, Z: 5}, "x"},
		{world.ChunkCoord{X: 2, Z: -3}, world.ChunkCoord{X: 2, Z: -2}, "z"},
	}

	for _, p := range pairs {
		ca := newCornerCache(g, p.a)
		cb := newCornerCache(g, p.b)

		for other := 0; other < cornersPerChunkXZ; other++ {
			for b := 0; b < cornersPerChunkY; b++ {
				var av, bv float64
				switch p.axis {
				case "x":
					av = ca.corner(cornersPerChunkXZ-1, b, other)
					bv = cb.corner(0, b, other)
				case "z":
					av = ca.corner(other, b, cornersPerChunkXZ-1)
					bv = cb.corner(other, b, 0)
				}
				if av != bv {
					t.Fatalf("chunks %v/%v axis=%s: shared corner (other=%d, b=%d) disagrees: %.17g vs %.17g",
						p.a, p.b, p.axis, other, b, av, bv)
				}
			}
		}
	}
}

// Criterion 7 ("one field, one path"), and mutation 2's target: Generate's
// cached path and SurfaceHeight's uncached path must produce identical
// solidity for the same global cell. This is checked directly against the
// two functions density.go documents as having to agree
// (Generator.solidAt / cornerCache.solidAt), across many points including
// ones that do not land on a cell corner -- exactly where a "recompute at
// full resolution instead of reading the interpolated field" bug would show
// up and a corner-only sample would not.
func TestSolidAtAgreesCachedAndUncached(t *testing.T) {
	g := NewGenerator(21)
	rng := rand.New(rand.NewSource(2020))

	checkedNonTrivial := 0
	for i := 0; i < 20000; i++ {
		cx := rng.Intn(21) - 10
		cz := rng.Intn(21) - 10
		coord := world.ChunkCoord{X: cx, Z: cz}
		cache := newCornerCache(g, coord)

		lx := rng.Intn(config.ChunkWidth)
		lz := rng.Intn(config.ChunkWidth)
		gx := coord.X*config.ChunkWidth + lx
		gz := coord.Z*config.ChunkWidth + lz
		h := g.clampHeight(g.rawSurfaceHeight(gx, gz))

		y := h + rng.Intn(2*surfaceHeightBand+1) - surfaceHeightBand
		if y < 0 || y >= config.ChunkHeight {
			continue
		}

		uncached := g.solidAt(gx, y, gz, h)
		cached := cache.solidAt(lx, y, lz, h)
		if uncached != cached {
			t.Fatalf("(%d,%d,%d) h=%d: uncached solidAt=%v, cached solidAt=%v -- Generate and SurfaceHeight would disagree here",
				gx, y, gz, h, uncached, cached)
		}
		if math.Abs(g.interpolatedDetailAt(gx, y, gz)) > 1e-9 {
			checkedNonTrivial++
		}
	}

	// Precondition rule: if every sampled point happened to have essentially
	// zero detail, this test would pass whether or not the two paths ever
	// disagree about a non-trivial cell -- the exact vacuous-pass shape M18
	// hit. detail is a smooth, (almost) everywhere non-zero field (see
	// opensimplex3.go), so this floor should be easy to clear; failing it
	// means the sampled cells were not actually testing anything.
	if checkedNonTrivial == 0 {
		t.Fatal("every sampled cell had ~zero interpolated detail; this test never actually exercised the 3D term")
	}
}

// countSolidRuns returns the number of separate contiguous solid runs
// (excluding Wood/Leaves features) in column (x, z) of c, scanning top to
// bottom. More than one run means an overhang: a column whose topmost solid
// cell has open air somewhere beneath it before the ground resumes.
func countSolidRuns(c *world.Chunk, x, z int) int {
	runs := 0
	inSolid := false
	for y := config.ChunkHeight - 1; y >= 0; y-- {
		b := c.GetBlock(x, y, z)
		solid := b != nil && b != blocks.Wood && b != blocks.Leaves
		if solid && !inSolid {
			runs++
		}
		inSolid = solid
	}
	return runs
}

// findOverhangColumn scans a seed/chunk-coordinate space for a real,
// generator-produced overhang column, per the precondition rule: rather than
// hardcoding a coordinate believed to contain one, every test that needs an
// overhang computes where one actually is. It returns the generator, chunk,
// local column and global column for the first overhang found, or ok=false
// if the search space is exhausted.
func findOverhangColumn(t *testing.T, maxChunks int) (g *Generator, coord world.ChunkCoord, c *world.Chunk, lx, lz, gx, gz int, ok bool) {
	t.Helper()
	blocks.ResetToVanilla()

	checked := 0
	for seed := int64(0); seed < 20 && checked < maxChunks; seed++ {
		g = NewGenerator(seed)
		for cx := 0; cx < 16 && checked < maxChunks; cx++ {
			for cz := 0; cz < 16 && checked < maxChunks; cz++ {
				checked++
				coord = world.ChunkCoord{X: cx, Z: cz}
				// Locate the column through the density field first and generate
				// only the chunk that holds it. Overhangs are rare enough (about
				// one chunk in two hundred) that generating every chunk on the way
				// was the single most expensive thing in this package's tests.
				// The chunk is still checked with countSolidRuns below, so what is
				// returned is an overhang in real generated blocks.
				c = nil
				for x := 0; x < config.ChunkWidth; x++ {
					for z := 0; z < config.ChunkWidth; z++ {
						if !columnHasOverhang(g, cx*config.ChunkWidth+x, cz*config.ChunkWidth+z) {
							continue
						}
						if c == nil {
							c = g.Generate(coord)
						}
						if countSolidRuns(c, x, z) > 1 {
							lx, lz = x, z
							gx = coord.X*config.ChunkWidth + x
							gz = coord.Z*config.ChunkWidth + z
							return g, coord, c, lx, lz, gx, gz, true
						}
					}
				}
			}
		}
	}
	return nil, world.ChunkCoord{}, nil, 0, 0, 0, 0, false
}

// Criterion 6: overhangs actually occur, at a measured, bounded rate -- not
// never (an expensive no-op) and not so often that every column has one.
// Both halves are asserted from the real generator's output, computed rather
// than assumed, per the precondition rule.
func TestOverhangsOccurAtABoundedRate(t *testing.T) {
	blocks.ResetToVanilla()

	const seeds = 5 // seeds 0-4 hold eight overhang columns between them; see the milestone result
	const chunksPerAxis = 16
	var totalColumns, overhangColumns int
	for seed := int64(0); seed < seeds; seed++ {
		g := NewGenerator(seed)
		for cx := 0; cx < chunksPerAxis; cx++ {
			for cz := 0; cz < chunksPerAxis; cz++ {
				// The density field directly, not Generate: the same columns, without
				// features, biomes or a 256-cell fill per column. Terrain blocks are
				// solid exactly where density is positive, and the layering tests hold
				// Generate to that, so the count is the same. Generating 2,560 whole
				// chunks here was most of why this package brushed Go's ten-minute
				// test timeout under race and coverage instrumentation.
				for x := 0; x < config.ChunkWidth; x++ {
					for z := 0; z < config.ChunkWidth; z++ {
						totalColumns++
						if columnHasOverhang(g, cx*config.ChunkWidth+x, cz*config.ChunkWidth+z) {
							overhangColumns++
						}
					}
				}
			}
		}
	}

	if overhangColumns == 0 {
		t.Fatalf("found zero overhang columns across %d columns (%d seeds x %dx%d chunks); "+
			"the 3D detail term is an expensive no-op", totalColumns, seeds, chunksPerAxis, chunksPerAxis)
	}

	// Bounded: overhangs are the exception, not the rule. If this ever fires,
	// `detail` has been tuned so aggressively that "overhang" has stopped
	// meaning anything -- see TestNoFloatingDebris and TestPlainsAreFlat for
	// the failures that tuning would also cause.
	const maxRate = 0.05 // 5% of columns
	rate := float64(overhangColumns) / float64(totalColumns)
	if rate > maxRate {
		t.Fatalf("%d/%d columns (%.4f%%) are overhangs, want at most %.0f%%", overhangColumns, totalColumns, 100*rate, 100*maxRate)
	}
	t.Logf("overhang rate: %d/%d columns (%.6f%%)", overhangColumns, totalColumns, 100*rate)
}

// Criterion 7, targeted at overhang columns specifically -- the milestone's
// own instruction ("the sample deliberately weighted toward columns where the
// 3D term is non-zero... a test over flat columns would pass against this
// exact bug"). findOverhangColumn's search satisfies the precondition rule:
// this fails loudly, rather than passing vacuously, if no overhang is found.
func TestSurfaceHeightAgreesOnOverhangColumns(t *testing.T) {
	g, _, c, lx, lz, gx, gz, ok := findOverhangColumn(t, 20*16*16)
	if !ok {
		t.Fatal("found no overhang column to test against; the precondition this test relies on does not hold")
	}

	want := topSolidY(c, lx, lz)
	got := g.SurfaceHeight(gx, gz)
	if got != want {
		t.Fatalf("overhang column (%d,%d) [global (%d,%d)]: SurfaceHeight=%d, but the generated blocks' top solid cell implies %d",
			lx, lz, gx, gz, got, want)
	}
}

// Criterion 4: flood-fill every solid cell reachable from y=0 (always solid,
// see minSurfaceHeight/surfaceHeightBand's comment) and assert no other solid
// component -- one an overhang disconnected from every downward path to the
// ground -- is smaller than debrisThreshold. This is the milestone's own
// name for the failure mode, asserted directly rather than eyeballed.
const debrisThreshold = 4

func TestNoFloatingDebris(t *testing.T) {
	blocks.ResetToVanilla()

	const chunksPerAxis = 6
	for seed := int64(0); seed < 2; seed++ { // two seeds: see TestOverhangsOccurAtABoundedRate on the timeout
		g := NewGenerator(seed)
		chunks := make(map[world.ChunkCoord]*world.Chunk)
		for cx := 0; cx < chunksPerAxis; cx++ {
			for cz := 0; cz < chunksPerAxis; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				chunks[coord] = g.Generate(coord)
			}
		}

		components := floodFillSolidComponents(chunks, chunksPerAxis)
		for _, comp := range components {
			if !comp.grounded && comp.size < debrisThreshold {
				t.Fatalf("seed %d: found a disconnected solid component of size %d (< threshold %d) at %v -- floating debris",
					seed, comp.size, debrisThreshold, comp.sample)
			}
		}
	}
}

type solidComponent struct {
	grounded bool
	size     int
	sample   [3]int
}

// floodFillSolidComponents partitions every solid cell across
// chunksPerAxis^2 chunks (generated with X,Z in [0, chunksPerAxis)) into
// connected components (6-connectivity), and marks each one "grounded" if it
// touches y=0 -- which minSurfaceHeight (32) being comfortably above
// surfaceHeightBand guarantees is solid in every column, so it is always a
// valid anchor to test connectivity against.
//
// The scan is bounded to y in [0, maxSurfaceHeight+surfaceHeightBand]: above
// that, density.go's own bound guarantees air everywhere, so there is
// nothing to visit.
func floodFillSolidComponents(chunks map[world.ChunkCoord]*world.Chunk, chunksPerAxis int) []solidComponent {
	maxY := maxSurfaceHeight + surfaceHeightBand + 1
	if maxY > config.ChunkHeight {
		maxY = config.ChunkHeight
	}
	width := chunksPerAxis * config.ChunkWidth

	solid := func(x, y, z int) bool {
		if y < 0 || y >= config.ChunkHeight || x < 0 || x >= width || z < 0 || z >= width {
			return false
		}
		cx, lx := x/config.ChunkWidth, x%config.ChunkWidth
		cz, lz := z/config.ChunkWidth, z%config.ChunkWidth
		c, ok := chunks[world.ChunkCoord{X: cx, Z: cz}]
		if !ok {
			return false
		}
		b := c.GetBlock(lx, y, lz)
		return b != nil && b != blocks.Wood && b != blocks.Leaves
	}

	type cell struct{ x, y, z int }
	visited := make(map[cell]bool)
	var components []solidComponent

	bfs := func(start cell) (int, bool) {
		queue := []cell{start}
		visited[start] = true
		size := 0
		grounded := false
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			size++
			if cur.y == 0 {
				grounded = true
			}
			neighbours := [6]cell{
				{cur.x + 1, cur.y, cur.z}, {cur.x - 1, cur.y, cur.z},
				{cur.x, cur.y + 1, cur.z}, {cur.x, cur.y - 1, cur.z},
				{cur.x, cur.y, cur.z + 1}, {cur.x, cur.y, cur.z - 1},
			}
			for _, n := range neighbours {
				if visited[n] || !solid(n.x, n.y, n.z) {
					continue
				}
				visited[n] = true
				queue = append(queue, n)
			}
		}
		return size, grounded
	}

	for x := 0; x < width; x++ {
		for z := 0; z < width; z++ {
			for y := 0; y < maxY; y++ {
				c := cell{x, y, z}
				if visited[c] || !solid(x, y, z) {
					continue
				}
				size, grounded := bfs(c)
				components = append(components, solidComponent{grounded: grounded, size: size, sample: [3]int{x, y, z}})
			}
		}
	}
	return components
}

// TestDetailBandBoundsRealDensity cross-checks maxErosionScale (and therefore
// detailBand/surfaceHeightBand) against real sampled columns, rather than
// trusting the erosion spline's control points blindly: if the spline is ever
// retuned to a higher peak than 0.6, this catches the bound silently going
// stale before density.go's fast paths start giving wrong answers outside
// the (now too narrow) band.
func TestDetailBandBoundsRealDensity(t *testing.T) {
	rng := rand.New(rand.NewSource(4242))
	for seed := int64(0); seed < 6; seed++ {
		g := NewGenerator(seed)
		for i := 0; i < 3000; i++ {
			x := rng.Intn(40000) - 20000
			z := rng.Intn(40000) - 20000
			e := g.erosionFactorAt(x, z)
			if e > maxErosionScale {
				t.Fatalf("seed %d: erosionFactorAt(%d,%d) = %.6f exceeds maxErosionScale = %.2f; detailBand's bound is unsound",
					seed, x, z, e, maxErosionScale)
			}
		}
	}
}

// columnHasOverhang reports whether global column (x, z) has terrain above air
// above terrain -- countSolidRuns(c, x, z) > 1 on a generated chunk, evaluated
// from the density field instead.
//
// Only the band around the heightmap height needs scanning: below it density
// is always positive and above it always negative (see surfaceHeightBand), so
// an air gap with terrain over it can only exist inside the band.
func columnHasOverhang(g *Generator, x, z int) bool {
	h := g.clampHeight(g.rawSurfaceHeight(x, z))
	lo := max(0, h-surfaceHeightBand)
	hi := min(config.ChunkHeight-1, h+surfaceHeightBand)

	col := g.newColumnDetail(x, z, lo, hi)
	seenAir := false
	for y := lo; y <= hi; y++ {
		switch solid := col.solidAt(y, h); {
		case !solid:
			seenAir = true
		case seenAir:
			return true
		}
	}
	return false
}
