package worldgen

import (
	"math"
	"math/rand"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// absurdBiomes builds a deliberately nonsensical BiomeSet for
// TestSurfaceHeightIgnoresBiomes: surfaces swapped from what a sane person
// would pick, points and weights nowhere near the vanilla set's, and a
// different number of biomes entirely. Nothing about it is meant to look
// like a real world -- the whole point is that SurfaceHeight cannot tell the
// difference between this and DefaultBiomes(), because it never looks.
func absurdBiomes() *BiomeSet {
	mk := func(id string, surface, filler *blocks.Block, point ClimatePoint, w Weights, features ...FeatureDef) *Biome {
		return &Biome{ID: id, Surface: surface, Filler: filler, Point: point, Weights: w, Features: features}
	}
	biomes := []*Biome{
		mk("zzz/upside-down", blocks.Sand, blocks.Stone,
			ClimatePoint{AxisTemperature: -1, AxisHumidity: 1, AxisMysticness: 1},
			Weights{AxisTemperature: 50, AxisHumidity: 50, AxisMysticness: 50},
			FeatureDef{Kind: featureTree, Numerator: 1, Denominator: 1}),
		mk("aaa/sideways", blocks.Grass, blocks.Sandstone,
			ClimatePoint{AxisContinentalness: 1, AxisErosion: -1, AxisPeaksValleys: 1},
			Weights{AxisContinentalness: 1000, AxisErosion: 1000, AxisPeaksValleys: 1000},
			FeatureDef{Kind: featureBush, Numerator: 1, Denominator: 1}),
		mk("mmm/inside-out", blocks.Dirt, blocks.Grass,
			ClimatePoint{AxisTemperature: 0.999, AxisHumidity: -0.999},
			Weights{AxisTemperature: 0.001, AxisHumidity: 0.001, AxisMysticness: 900}),
	}
	sortByID(biomes)
	return &BiomeSet{biomes: biomes}
}

// TestSurfaceHeightIgnoresBiomes is criterion 1, checked exactly the way the
// milestone prescribes: generate with the real biome set and with a
// deliberately absurd one, and assert SurfaceHeight agrees everywhere. This
// is the architectural decision ("terrain shape must not depend on biome")
// written as an assertion, per the milestone's own framing.
func TestSurfaceHeightIgnoresBiomes(t *testing.T) {
	for seed := int64(0); seed < 5; seed++ {
		real := NewGenerator(seed)
		absurd := newGeneratorWithBiomes(seed, absurdBiomes())

		rng := rand.New(rand.NewSource(seed + 500))
		for i := 0; i < 3000; i++ {
			x := rng.Intn(200000) - 100000
			z := rng.Intn(200000) - 100000

			wantH := real.SurfaceHeight(x, z)
			gotH := absurd.SurfaceHeight(x, z)
			if gotH != wantH {
				t.Fatalf("seed %d: SurfaceHeight(%d,%d) = %d with the absurd biome set, want %d (the real set's answer) -- "+
					"terrain height must never depend on which biomes are loaded", seed, x, z, gotH, wantH)
			}
		}
	}
}

// bruteForceSelect is an independent reimplementation of BiomeSet.Select,
// deliberately written without reusing weightedDistanceSquared or addProduct,
// so a bug shared between the production code and this reference would have
// to be coincidental rather than structural. It is the milestone's "checked
// against a brute-force reference" for criterion 2.
func bruteForceSelect(p ClimatePoint, biomes []*Biome) *Biome {
	var best *Biome
	bestDist := math.Inf(1)
	for _, b := range biomes {
		var d float64
		for axis := 0; axis < int(numAxes); axis++ {
			diff := p[axis] - b.Point[axis]
			d += b.Weights[axis] * diff * diff
		}
		if best == nil || d < bestDist || (d == bestDist && b.ID < best.ID) {
			best, bestDist = b, d
		}
	}
	return best
}

// randomBiomeSet builds n biomes with random (but valid: positive weight
// sum) points and weights, for exercising selection over data shapes wider
// than the three vanilla biomes.
func randomBiomeSet(rng *rand.Rand, n int) []*Biome {
	biomes := make([]*Biome, n)
	for i := range biomes {
		var p ClimatePoint
		var w Weights
		for a := 0; a < int(numAxes); a++ {
			p[a] = rng.Float64()*2 - 1
			w[a] = rng.Float64() * 5
		}
		biomes[i] = &Biome{ID: string(rune('a' + i)), Point: p, Weights: w}
	}
	return biomes
}

// TestSelectMatchesBruteForce is criterion 2's main check: over a large
// sample of random climate points and several independently-random biome
// sets, BiomeSet.Select must agree with bruteForceSelect exactly.
func TestSelectMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	for trial := 0; trial < 20; trial++ {
		n := 2 + rng.Intn(8)
		biomes := randomBiomeSet(rng, n)
		sortByID(biomes)
		bs := &BiomeSet{biomes: biomes}

		for i := 0; i < 500; i++ {
			var p ClimatePoint
			for a := 0; a < int(numAxes); a++ {
				p[a] = rng.Float64()*2 - 1
			}

			want := bruteForceSelect(p, biomes)
			got := bs.Select(p)
			if got.ID != want.ID {
				t.Fatalf("trial %d, point %v: Select = %q, brute force = %q", trial, p, got.ID, want.ID)
			}
		}
	}
}

// TestSelectMatchesBruteForce_DefaultBiomes runs the same comparison over
// the real vanilla set, so the production data -- not just synthetic random
// sets -- is checked against the reference too.
func TestSelectMatchesBruteForce_DefaultBiomes(t *testing.T) {
	blocks.ResetToVanilla()
	bs := DefaultBiomes()
	rng := rand.New(rand.NewSource(2))

	for i := 0; i < 5000; i++ {
		var p ClimatePoint
		for a := 0; a < int(numAxes); a++ {
			p[a] = rng.Float64()*2 - 1
		}
		want := bruteForceSelect(p, bs.Biomes())
		got := bs.Select(p)
		if got.ID != want.ID {
			t.Fatalf("point %v: Select = %q, brute force = %q", p, got.ID, want.ID)
		}
	}
}

// TestSelectBreaksTiesByID is criterion 2's other half: constructed EXACT
// ties, resolved by biome ID regardless of which order the tied biomes were
// declared in. Two biomes are placed symmetrically around the query point on
// one axis, with identical weights and identical values on every other axis,
// which makes their weighted squared distance from the query point exactly
// equal in floating point -- not approximately, since the two differences are
// exact negatives of each other and squaring removes the sign.
func TestSelectBreaksTiesByID(t *testing.T) {
	base := Weights{1, 1, 1, 1, 1, 1}
	query := ClimatePoint{0, 0, 0, 0, 0, 0}

	lo := &Biome{ID: "aaa/low", Point: ClimatePoint{0, 0, 0, -0.5, 0, 0}, Weights: base}
	hi := &Biome{ID: "zzz/high", Point: ClimatePoint{0, 0, 0, 0.5, 0, 0}, Weights: base}

	// Sanity: the two really are an exact tie, not merely close.
	dLo := weightedDistanceSquared(query, lo.Point, lo.Weights)
	dHi := weightedDistanceSquared(query, hi.Point, hi.Weights)
	if dLo != dHi {
		t.Fatalf("fixture is not an exact tie: dLo=%v dHi=%v", dLo, dHi)
	}

	declOrders := [][]*Biome{
		{lo, hi},
		{hi, lo},
	}
	for _, order := range declOrders {
		bs := &BiomeSet{biomes: append([]*Biome(nil), order...)}
		got := bs.Select(query)
		if got.ID != "aaa/low" {
			t.Fatalf("declaration order %v: Select returned %q on an exact tie, want the lexicographically lowest id %q",
				[]string{order[0].ID, order[1].ID}, got.ID, "aaa/low")
		}
	}
}

// TestEveryBiomeOccursInPlausibleProportion is criterion 4, applying the
// constraints doc's "range criterion has two halves" lesson to biomes
// directly: every declared biome must actually occur over a large sample,
// and none may dominate absurdly.
//
// Sampling is a grid rather than points chosen near each biome's declared
// centre -- the whole point is to find out what a generator actually
// produces, not to confirm it can produce what it was tuned to.
func TestEveryBiomeOccursInPlausibleProportion(t *testing.T) {
	blocks.ResetToVanilla()

	const (
		span = 24000
		step = 160
	)

	for seed := int64(0); seed < 4; seed++ {
		g := NewGenerator(seed)
		counts := map[string]int{}
		total := 0
		for x := -span; x <= span; x += step {
			for z := -span; z <= span; z += step {
				b := g.selectBiome(x, z)
				counts[b.ID]++
				total++
			}
		}

		for _, id := range []string{"minae/plains", "minae/forest", "minae/dunes"} {
			c := counts[id]
			if c == 0 {
				t.Fatalf("seed %d: biome %q never occurred across %d sampled columns", seed, id, total)
			}
			share := float64(c) / float64(total)
			// Generous bounds: three biomes, symmetric-ish in the climate
			// space, should each land well within an order of magnitude of
			// an even 1/3 split. This is not tuned to the exact measured
			// split (which varies by seed) -- it exists to catch a biome
			// reduced to a sliver or ballooned to nearly everything.
			if share < 0.03 {
				t.Fatalf("seed %d: biome %q covers only %.2f%% of sampled columns -- close to never occurring", seed, id, share*100)
			}
			if share > 0.85 {
				t.Fatalf("seed %d: biome %q covers %.2f%% of sampled columns -- dominates absurdly", seed, id, share*100)
			}
		}
	}
}

// runLengths walks a scanline of biome IDs and returns the length of every
// maximal run of the same biome. It is the measurement TestBiomeRegionsAreCoherent
// bases both of its assertions on.
func runLengths(ids []string) []int {
	if len(ids) == 0 {
		return nil
	}
	var out []int
	cur := 1
	for i := 1; i < len(ids); i++ {
		if ids[i] == ids[i-1] {
			cur++
			continue
		}
		out = append(out, cur)
		cur = 1
	}
	out = append(out, cur)
	return out
}

// TestBiomeRegionsAreCoherent is criterion 5: a bound on how often
// horizontally adjacent columns disagree, and a minimum typical region
// size, both measured directly from the terrain rather than assumed.
//
// Per the constraints doc's "test terrain must have the shape the bug
// needs": the bug this guards against is climateFrequency set too high (or
// biome axes weighted so noise alone flips the nearest neighbour every few
// columns), which reads as static rather than as places. A long scanline
// through real generated climate is exactly the shape that bug would show
// up in -- a speckled scanline has a disagreement rate near 0.5 and a mean
// run length near 1, regardless of how it is sampled.
func TestBiomeRegionsAreCoherent(t *testing.T) {
	blocks.ResetToVanilla()

	const length = 40000

	for seed := int64(0); seed < 3; seed++ {
		g := NewGenerator(seed)

		ids := make([]string, length)
		for x := 0; x < length; x++ {
			ids[x] = g.selectBiome(x, 0).ID
		}

		disagreements := 0
		for i := 1; i < len(ids); i++ {
			if ids[i] != ids[i-1] {
				disagreements++
			}
		}
		disagreeRate := float64(disagreements) / float64(len(ids)-1)

		// A speckled selector disagrees on a large fraction of adjacent
		// pairs (a coin-flip selector would disagree roughly half the
		// time). Regions read as places when this is a small fraction
		// instead.
		const maxDisagreeRate = 0.02
		if disagreeRate > maxDisagreeRate {
			t.Fatalf("seed %d: adjacent columns disagree on biome %.4f of the time (%d/%d), want at most %.4f",
				seed, disagreeRate, disagreements, len(ids)-1, maxDisagreeRate)
		}

		runs := runLengths(ids)
		var sum int
		for _, r := range runs {
			sum += r
		}
		mean := float64(sum) / float64(len(runs))

		// climateFrequency's wavelength (1/climateFrequency = 1000 blocks)
		// sets the scale regions should be built on. A minimum two orders
		// of magnitude below that wavelength is a conservative floor: a
		// selector that produced regions anywhere near that small would
		// already have failed the disagreement-rate check above, so this
		// exists to catch the two properties independently rather than
		// relying on one to imply the other.
		const minMeanRunLength = 10.0
		if mean < minMeanRunLength {
			t.Fatalf("seed %d: mean biome run length is %.1f columns (over %d runs), want at least %.1f",
				seed, mean, len(runs), minMeanRunLength)
		}
	}
}

// TestSurfaceAndFillerFollowBiome is criterion 6's surface/filler half, run
// across a wide grid of chunks spanning both signs and hundreds of blocks in
// every direction -- deliberately, rather than at one hand-picked chunk, per
// the constraints doc's "test terrain must have the shape the bug needs":
// the bug this must catch is chunk.go sampling climate at chunk-local
// coordinates instead of global ones, and a local-coordinate bug produces
// the SAME biome at every chunk regardless of position (every chunk's local
// coordinates span the same [0, ChunkWidth) range). A single hand-picked
// chunk can accidentally land on that same biome by chance -- plains alone
// covers well over half the sampled area in TestEveryBiomeOccursInPlausibleProportion
// -- and silently prove nothing. A wide grid practically guarantees at least
// one sampled chunk's real (global-coordinate) biome differs from whatever a
// local-coordinate bug would produce everywhere.
func TestSurfaceAndFillerFollowBiome(t *testing.T) {
	blocks.ResetToVanilla()

	for seed := int64(0); seed < 2; seed++ {
		g := NewGenerator(seed)
		for cx := -30; cx <= 30; cx += 5 {
			for cz := -30; cz <= 30; cz += 5 {
				coord := world.ChunkCoord{X: cx, Z: cz}
				c := g.Generate(coord)

				for x := range config.ChunkWidth {
					gx := coord.X*config.ChunkWidth + x
					for z := range config.ChunkWidth {
						gz := coord.Z*config.ChunkWidth + z

						biome := g.selectBiome(gx, gz)
						height := g.SurfaceHeight(gx, gz)

						if got := c.GetBlock(x, height-1, z); got != biome.Surface {
							t.Fatalf("seed %d chunk (%d,%d): column (%d,%d) [global (%d,%d)]: surface block is %v, want biome %q's surface %v",
								seed, cx, cz, x, z, gx, gz, got, biome.ID, biome.Surface)
						}
						for d := 2; d <= dirtDepth+1; d++ {
							y := height - d
							if got := c.GetBlock(x, y, z); got != biome.Filler {
								t.Fatalf("seed %d chunk (%d,%d): column (%d,%d) [global (%d,%d)]: filler block at y=%d is %v, want biome %q's filler %v",
									seed, cx, cz, x, z, gx, gz, y, got, biome.ID, biome.Filler)
							}
						}
					}
				}
			}
		}
	}
}

// TestFeaturesFollowRootBiome is criterion 6's feature half: every Wood or
// Leaves block in a generated chunk must be explained by some root within
// range whose OWN biome (selected at that root's global column) declares
// the root's kind. A mutation that let a feature ignore its root biome --
// e.g. checking the biome at the block being painted, or a neighbouring
// chunk's biome, instead of the root's own column -- would place blocks no
// permitting root can explain, which this test would catch as a failure to
// find any explaining root at all (dunes, with zero declared features,
// never contributes an explaining root, so a tree rooted in dunes could
// only be explained by a permitting root elsewhere -- exactly the case a
// wrong-column mutation produces).
func TestFeaturesFollowRootBiome(t *testing.T) {
	blocks.ResetToVanilla()

	checked := 0
	for seed := int64(0); seed < 3; seed++ {
		g := NewGenerator(seed)
		for cx := -4; cx <= 4; cx++ {
			for cz := -4; cz <= 4; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				roots := g.rootsAffecting(coord)
				c := g.Generate(coord)

				for x := range config.ChunkWidth {
					gx := coord.X*config.ChunkWidth + x
					for z := range config.ChunkWidth {
						gz := coord.Z*config.ChunkWidth + z
						for y := range config.ChunkHeight {
							b := c.GetBlock(x, y, z)
							if b != blocks.Wood && b != blocks.Leaves {
								continue
							}
							checked++

							explained := false
							for _, r := range roots {
								radius := bushRadius
								if r.kind == featureTree {
									radius = treeRadius
								}
								if absOffset(gx-r.globalX()) > radius || absOffset(gz-r.globalZ()) > radius {
									continue
								}
								rootBiome := g.selectBiome(r.globalX(), r.globalZ())
								if _, ok := rootBiome.featureDensity(r.kind); ok {
									explained = true
									break
								}
							}
							if !explained {
								t.Fatalf("seed %d chunk (%d,%d): %s at global (%d,%d,%d) has no explaining root whose "+
									"own root-column biome permits its kind", seed, cx, cz, b.ID, gx, y, gz)
							}
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no feature blocks were found across the sampled seeds/chunks -- test is not exercising anything")
	}
}

// TestNoFeatureRootsInDunes is a sharper, more direct restatement of the
// same property TestFeaturesFollowRootBiome checks generally: dunes declares
// zero features, so across a large sample no accepted root whose OWN column
// selects dunes should ever be painted.
func TestNoFeatureRootsInDunes(t *testing.T) {
	blocks.ResetToVanilla()

	checkedDunesRoots := 0
	for seed := int64(0); seed < 5; seed++ {
		g := NewGenerator(seed)
		for cx := -6; cx <= 6; cx++ {
			for cz := -6; cz <= 6; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				for _, r := range g.rootsAffecting(coord) {
					biome := g.selectBiome(r.globalX(), r.globalZ())
					if biome.ID != "minae/dunes" {
						continue
					}
					checkedDunesRoots++
					if g.biomeAllows(r) {
						t.Fatalf("seed %d: root at (%d,%d) selects dunes (no declared features) yet biomeAllows accepted it",
							seed, r.globalX(), r.globalZ())
					}
				}
			}
		}
	}
	if checkedDunesRoots == 0 {
		t.Fatal("no candidate roots landed in dunes across the sampled area -- test is not exercising anything")
	}
}

// Criterion 7 for biomes (sample at global, never chunk-local, coordinates)
// is checked by TestSurfaceAndFillerFollowBiome above: it compares Generate's
// actually-placed surface block, at a chunk chosen far from the origin
// specifically so local and global coordinates diverge, against
// g.selectBiome computed independently at the correct global coordinates. A
// chunk.go call site that sampled climate at (localX, localZ) instead would
// disagree with that independently-computed expectation almost everywhere
// in such a chunk, exactly as M17's SurfaceHeight tests already reason about
// the same hazard for terrain height.
