package noise

import (
	"math"
	"math/rand"
	"testing"
)

// Criterion 1: the same seed and coordinate always produce the same value,
// across runs and independent of evaluation order. Determinism is what a
// world seed rests on: if this test can fail, nothing else in this file
// means anything.
func TestEval2DDeterministic(t *testing.T) {
	seed := NewSeed(42)
	rng := rand.New(rand.NewSource(1))

	type sample struct{ x, y float64 }
	samples := make([]sample, 200)
	for i := range samples {
		samples[i] = sample{
			x: rng.Float64()*2000 - 1000,
			y: rng.Float64()*2000 - 1000,
		}
	}

	// Evaluate forwards, then backwards, then in a freshly shuffled order.
	// Backwards catches any hidden mutable state that only breaks when the
	// previous call touched different coordinates; forwards alone would not.
	forward := make([]float64, len(samples))
	for i, s := range samples {
		forward[i] = seed.Eval2D(s.x, s.y)
	}

	for i := len(samples) - 1; i >= 0; i-- {
		if got := seed.Eval2D(samples[i].x, samples[i].y); got != forward[i] {
			t.Fatalf("Eval2D(%v, %v) = %v evaluated backwards, want %v (same as the forward pass)",
				samples[i].x, samples[i].y, got, forward[i])
		}
	}

	order := rng.Perm(len(samples))
	for _, i := range order {
		if got := seed.Eval2D(samples[i].x, samples[i].y); got != forward[i] {
			t.Fatalf("Eval2D(%v, %v) = %v evaluated out of order, want %v",
				samples[i].x, samples[i].y, got, forward[i])
		}
	}

	// A second, independently constructed Seed from the same value must
	// agree exactly: determinism has to survive process/instance boundaries,
	// not just repeated calls on one Seed value.
	again := NewSeed(42)
	for i, s := range samples {
		if got := again.Eval2D(s.x, s.y); got != forward[i] {
			t.Fatalf("a fresh NewSeed(42).Eval2D(%v, %v) = %v, want %v (same as the first Seed built from 42)",
				s.x, s.y, got, forward[i])
		}
	}
}

// Criterion 3: output stays within its documented [-1, 1] range for a large
// random sample. TestConservativeScaleGuaranteesRange (in
// opensimplex2_bound_test.go) proves this holds for every possible input;
// this test is the sanity check that the proof and the implementation
// actually agree, over a sample large enough that a wiring mistake (a
// dropped noiseScale, an inverted sign) would show up.
func TestEval2DRangeIsBounded(t *testing.T) {
	seed := NewSeed(7)
	rng := rand.New(rand.NewSource(2))

	const samples = 200_000
	for i := 0; i < samples; i++ {
		x := rng.Float64()*20000 - 10000
		y := rng.Float64()*20000 - 10000
		v := seed.Eval2D(x, y)
		if v < -1 || v > 1 {
			t.Fatalf("Eval2D(%v, %v) = %v, outside the documented [-1, 1] range", x, y, v)
		}
	}
}

// Criterion 4: adjacent samples differ by less than a bound; the field has no
// discontinuities. A seam here becomes a visible cliff in terrain.
//
// Eval2D is a sum of three corner kernels, each identically zero outside its
// own falloff radius (see corner), so crossing the boundary where the active
// corner set changes replaces a zero contribution with a continuously-grown
// one rather than jumping between two unrelated values. That is what this
// test actually checks, at a step size much smaller than a lattice cell.
func TestEval2DContinuity(t *testing.T) {
	seed := NewSeed(99)
	rng := rand.New(rand.NewSource(3))

	const (
		samples = 5_000
		eps     = 1e-4
		// A single-corner kernel has bounded slope (see
		// gradientMaxContribution's derivation, which bounds the kernel
		// itself, not just its value), and noiseScale scales that down
		// further, so a step of 1e-4 producing a jump anywhere near 1 would
		// mean a genuine seam rather than ordinary smooth variation.
		maxDelta = 0.05
	)

	for i := 0; i < samples; i++ {
		x := rng.Float64()*2000 - 1000
		y := rng.Float64()*2000 - 1000

		base := seed.Eval2D(x, y)
		dx := seed.Eval2D(x+eps, y)
		dy := seed.Eval2D(x, y+eps)

		if d := absf(dx - base); d > maxDelta {
			t.Fatalf("Eval2D jumped %.6g over a step of %g in x at (%v, %v): %v -> %v", d, eps, x, y, base, dx)
		}
		if d := absf(dy - base); d > maxDelta {
			t.Fatalf("Eval2D jumped %.6g over a step of %g in y at (%v, %v): %v -> %v", d, eps, x, y, base, dy)
		}
	}
}

// Criterion 7: sampling a coordinate directly and sampling it as part of a
// neighbouring chunk's range must give identical results. Generation is
// per-chunk; a mismatch here is the classic bug of building sample
// coordinates from a chunk-local index instead of the chunk's global offset,
// which produces a visible wall at every chunk seam because each chunk's
// noise field silently restarts from local (0, 0) instead of continuing the
// global field.
func TestEval2DChunkBoundaryAgreement(t *testing.T) {
	const chunkWidth = 16

	seed := NewSeed(1234)

	// "Directly": Eval2D over a stretch of global coordinates spanning two
	// chunks (chunk 0: local 0..15, chunk 1: local 0..15 at global
	// offset 16).
	direct := make([]float64, chunkWidth*2)
	for gx := 0; gx < chunkWidth*2; gx++ {
		direct[gx] = seed.Eval2D(float64(gx), 3.5)
	}

	// "As part of a neighbouring chunk's range": the same values, but
	// computed the way a per-chunk generator actually would -- iterating a
	// chunk index and a local coordinate, and building the global coordinate
	// from them, for both chunk 0 and chunk 1.
	viaChunks := make([]float64, chunkWidth*2)
	for chunk := 0; chunk < 2; chunk++ {
		for local := 0; local < chunkWidth; local++ {
			global := chunk*chunkWidth + local
			viaChunks[global] = seed.Eval2D(float64(chunk*chunkWidth+local), 3.5)
		}
	}

	for i := range direct {
		if direct[i] != viaChunks[i] {
			t.Fatalf("global x=%d: direct sampling gave %v, chunked sampling gave %v; a per-chunk generator using this field would show a seam here",
				i, direct[i], viaChunks[i])
		}
	}

	// Eval2D itself has no notion of a chunk -- it is just a pure function of
	// a continuous coordinate, so the comparison above holds for any correct
	// caller by construction and cannot, on its own, prove this test is
	// capable of catching the bug it is named for. This is the same gap the
	// milestone doc calls out for criterion 5's statistic, so it gets the
	// same fix: build the actual broken pattern (using the chunk-local index
	// with no global offset, so every chunk restarts its sampling from 0)
	// and confirm it disagrees with direct sampling everywhere chunk 1
	// overlaps chunk 0's local range.
	broken := make([]float64, chunkWidth*2)
	for chunk := 0; chunk < 2; chunk++ {
		for local := 0; local < chunkWidth; local++ {
			global := chunk*chunkWidth + local
			broken[global] = seed.Eval2D(float64(local), 3.5) // missing chunk*chunkWidth
		}
	}
	disagreements := 0
	for i := chunkWidth; i < len(direct); i++ {
		if direct[i] != broken[i] {
			disagreements++
		}
	}
	if disagreements == 0 {
		t.Fatal("the local-coordinate-only sampling pattern agreed with direct sampling everywhere in chunk 1; this comparison is not actually capable of detecting the chunk-boundary bug it exists to catch")
	}
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// TestEval2DUsesMostOfItsRange is the other half of criterion 3, and the half
// that was missing.
//
// "Output stays within its documented range" is satisfied perfectly by a
// function that returns zero everywhere, so bounding alone says nothing about
// whether the range is honest. The first implementation of this package
// scaled by the triangle-inequality bound and reached only ±0.365 — inside
// [-1, 1], passing every test, and quietly costing every downstream consumer
// two thirds of the resolution the interface advertises.
//
// Splines are the concrete victim: a curve authored across [-1, 1] and fed a
// third of its domain wastes the control it exists to provide, and each
// consumer would end up inventing its own correction factor.
func TestEval2DUsesMostOfItsRange(t *testing.T) {
	// A shallow floor: this is asking whether the scale is roughly right, not
	// pinning down the exact peak, which TestRawSumSupremumIsSound does.
	const wantAtLeast = 0.85

	peak := 0.0
	for _, seedValue := range []int64{1, 7, 12345, -99, 1 << 40} {
		s := NewSeed(seedValue)
		for i := range 400 {
			for j := range 400 {
				// Irrational-ish strides so the samples do not land on the
				// lattice in a repeating pattern and miss the extremes.
				v := math.Abs(s.Eval2D(float64(i)*0.0713, float64(j)*0.0917))
				if v > peak {
					peak = v
				}
			}
		}
	}

	if peak < wantAtLeast {
		t.Errorf("the field peaks at %.4f, well inside its documented [-1, 1]; "+
			"noiseScale is too conservative and consumers are losing resolution", peak)
	}
	if peak > 1 {
		t.Errorf("the field reached %.4f, outside its documented [-1, 1]", peak)
	}
}
