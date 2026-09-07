package worldgen

import (
	"math"
	"math/rand"
	"testing"
)

// maxAdjacentSlope bounds how much SurfaceHeight may change between two
// horizontally adjacent columns. It is deliberately generous: the
// frequencies in generator.go make gently rolling terrain, and measured
// adjacent deltas across 160,000 samples spanning 8 seeds never exceeded 1 --
// but the point of this bound, per the milestone's criterion 6, is to catch
// the generator drifting into hills, not to pin the exact number today's
// tuning happens to produce.
const maxAdjacentSlope = 4

// Criterion 1: determinism.
func TestSurfaceHeightIsDeterministic(t *testing.T) {
	g := NewGenerator(42)
	coords := [][2]int{{0, 0}, {5, -5}, {1000, -1000}, {-37, 91}, {123456, -654321}}
	for _, c := range coords {
		want := g.SurfaceHeight(c[0], c[1])
		for i := 0; i < 5; i++ {
			if got := g.SurfaceHeight(c[0], c[1]); got != want {
				t.Fatalf("SurfaceHeight(%d, %d) = %d on call %d, want %d (same seed and coordinate must always agree)",
					c[0], c[1], got, i, want)
			}
		}
	}

	// A fresh Generator built from the same seed must reproduce every value
	// exactly -- determinism across independent constructions, not just
	// within one instance's lifetime.
	g2 := NewGenerator(42)
	for _, c := range coords {
		want := g.SurfaceHeight(c[0], c[1])
		if got := g2.SurfaceHeight(c[0], c[1]); got != want {
			t.Fatalf("SurfaceHeight(%d, %d) = %d from a fresh Generator(42), want %d", c[0], c[1], got, want)
		}
	}
}

// Criterion 3 (the pure-function half): height must not jump across a chunk
// boundary by more than the terrain's own slope allows. This is testable
// without generating a single chunk, per the milestone's design decision --
// see chunk_test.go for the complementary check that Generate's actual
// output agrees with this and is not merely coincidentally consistent with
// it.
func TestSurfaceHeightIsContinuousAcrossChunkBoundaries(t *testing.T) {
	g := NewGenerator(7)

	for chunkX := -20; chunkX <= 20; chunkX++ {
		boundary := chunkX*16 - 1 // last column of one chunk
		for z := -50; z <= 50; z++ {
			h0 := g.SurfaceHeight(boundary, z)
			h1 := g.SurfaceHeight(boundary+1, z)
			if d := iabs(h1 - h0); d > maxAdjacentSlope {
				t.Fatalf("SurfaceHeight jumps by %d across the chunk seam at x=%d/%d, z=%d (%d -> %d); want at most %d",
					d, boundary, boundary+1, z, h0, h1, maxAdjacentSlope)
			}
		}
	}
}

// Criterion 5: height stays in range for a large sample of coordinates and
// seeds. There is no bedrock block, so the floor is y=1, not y=0; the
// ceiling is config.ChunkHeight, spelled out numerically here to avoid
// importing config for a single constant already fixed at 256 by the
// project's own definition.
func TestSurfaceHeightStaysInRange(t *testing.T) {
	const chunkHeight = 256

	for seed := int64(0); seed < 12; seed++ {
		g := NewGenerator(seed)
		rng := rand.New(rand.NewSource(seed))
		for i := 0; i < 5000; i++ {
			x := rng.Intn(200000) - 100000
			z := rng.Intn(200000) - 100000
			h := g.SurfaceHeight(x, z)
			if h < 1 {
				t.Fatalf("seed %d: SurfaceHeight(%d, %d) = %d, at or below the world floor -- there is no bedrock to rest on",
					seed, x, z, h)
			}
			if h >= chunkHeight {
				t.Fatalf("seed %d: SurfaceHeight(%d, %d) = %d, at or above the chunk height", seed, x, z, h)
			}
		}
	}
}

// Criterion 6: plains are flat-ish. Bound the slope over a large sample so
// "plains" is a checked property, not an unverified claim.
func TestPlainsAreFlat(t *testing.T) {
	for seed := int64(0); seed < 8; seed++ {
		g := NewGenerator(seed)
		rng := rand.New(rand.NewSource(seed + 1000))
		for i := 0; i < 5000; i++ {
			x := rng.Intn(20000) - 10000
			z := rng.Intn(20000) - 10000
			h := g.SurfaceHeight(x, z)

			if d := iabs(g.SurfaceHeight(x+1, z) - h); d > maxAdjacentSlope {
				t.Fatalf("seed %d: slope in x at (%d,%d) is %d, want at most %d for plains", seed, x, z, d, maxAdjacentSlope)
			}
			if d := iabs(g.SurfaceHeight(x, z+1) - h); d > maxAdjacentSlope {
				t.Fatalf("seed %d: slope in z at (%d,%d) is %d, want at most %d for plains", seed, x, z, d, maxAdjacentSlope)
			}
		}
	}
}

// Criterion 7: different seeds give different worlds, and the same seed on
// two runs does not.
func TestDifferentSeedsGiveDifferentWorlds(t *testing.T) {
	gA := NewGenerator(1)
	gB := NewGenerator(2)

	const samples = 500
	differences := 0
	for i := 0; i < samples; i++ {
		x, z := i*13-3000, i*7-1000
		if gA.SurfaceHeight(x, z) != gB.SurfaceHeight(x, z) {
			differences++
		}
	}

	// Requiring every sample to differ would be too strong -- two unrelated
	// fields can coincide at a handful of points by chance -- but a
	// generator that ignored the seed entirely would agree on all of them.
	if differences < samples/2 {
		t.Fatalf("seed 1 and seed 2 agree on %d/%d sampled columns; expected most to differ", samples-differences, samples)
	}
}

func TestSameSeedIsReproducibleAcrossRuns(t *testing.T) {
	const samples = 500
	points := make([][2]int, samples)
	for i := range points {
		points[i] = [2]int{i*17 - 4000, i*11 - 2000}
	}

	run := func() []int {
		g := NewGenerator(99)
		out := make([]int, samples)
		for i, p := range points {
			out[i] = g.SurfaceHeight(p[0], p[1])
		}
		return out
	}

	first := run()
	second := run()
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("seed 99 produced %d at point %d on the first run and %d on the second", first[i], i, second[i])
		}
	}
}

func iabs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// TestRawSurfaceHeightNeedsNoClamp checks the range guarantee where it is
// actually made.
//
// TestSurfaceHeightStaysInRange cannot distinguish "the terrain formula is
// well behaved" from "clampHeight works", because the clamp makes the
// assertion true either way. A generator whose noise drifted to a height of
// 5000 would pass it while producing a world pinned flat against a ceiling —
// visually catastrophic, and silent.
//
// So this asserts the stronger, real property: the unclamped formula stays
// inside the range on its own, with room to spare. The clamp remains as a
// backstop for future retuning; it is simply not what the guarantee rests on
// today.
func TestRawSurfaceHeightNeedsNoClamp(t *testing.T) {
	// A margin, not the clamp bounds themselves: a formula that just barely
	// grazes the limit is one retune away from being pinned, and this should
	// fail while there is still room to fix it.
	const margin = 4.0

	for _, seed := range []int64{1337, 1, -1, 999999, -424242, 77777} {
		g := NewGenerator(seed)

		lo, hi := math.Inf(1), math.Inf(-1)
		for i := range 300 {
			for j := range 300 {
				// Coprime-ish strides over a wide span, so the sample covers
				// whole wavelengths of the broadest field rather than one
				// patch of it.
				h := g.rawSurfaceHeight(i*7-1000, j*11-1000)
				lo = math.Min(lo, h)
				hi = math.Max(hi, h)
			}
		}

		if lo < minSurfaceHeight+margin || hi > maxSurfaceHeight-margin {
			t.Errorf("seed %d: unclamped height spans [%.2f, %.2f], want within [%.1f, %.1f]; "+
				"the terrain formula is relying on clampHeight rather than staying in range itself",
				seed, lo, hi, minSurfaceHeight+margin, maxSurfaceHeight-margin)
		}
	}
}
