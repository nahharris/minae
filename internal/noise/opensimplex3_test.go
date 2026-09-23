package noise

import (
	"math"
	"math/rand"
	"testing"
)

// Criterion 1 (M16): the same seed and coordinate always produce the same
// value, across evaluation orders and independent constructions. Mirrors
// TestEval2DDeterministic exactly, generalized to three axes.
func TestEval3DDeterministic(t *testing.T) {
	seed := NewSeed(42)
	rng := rand.New(rand.NewSource(1))

	type sample struct{ x, y, z float64 }
	samples := make([]sample, 200)
	for i := range samples {
		samples[i] = sample{
			x: rng.Float64()*2000 - 1000,
			y: rng.Float64()*2000 - 1000,
			z: rng.Float64()*2000 - 1000,
		}
	}

	forward := make([]float64, len(samples))
	for i, s := range samples {
		forward[i] = seed.Eval3D(s.x, s.y, s.z)
	}

	for i := len(samples) - 1; i >= 0; i-- {
		if got := seed.Eval3D(samples[i].x, samples[i].y, samples[i].z); got != forward[i] {
			t.Fatalf("Eval3D(%v, %v, %v) = %v evaluated backwards, want %v (same as the forward pass)",
				samples[i].x, samples[i].y, samples[i].z, got, forward[i])
		}
	}

	order := rng.Perm(len(samples))
	for _, i := range order {
		if got := seed.Eval3D(samples[i].x, samples[i].y, samples[i].z); got != forward[i] {
			t.Fatalf("Eval3D(%v, %v, %v) = %v evaluated out of order, want %v",
				samples[i].x, samples[i].y, samples[i].z, got, forward[i])
		}
	}

	seed2 := NewSeed(42)
	for i, s := range samples {
		if got := seed2.Eval3D(s.x, s.y, s.z); got != forward[i] {
			t.Fatalf("a second Seed(42) gave Eval3D(%v,%v,%v) = %v, want %v (from the first Seed(42))",
				s.x, s.y, s.z, got, forward[i])
		}
	}
}

// Criterion 3: Eval3D never leaves [-1, 1], and the field is not degenerate
// (a function pinned at zero would trivially "stay in range" -- the second
// half of the range criterion the package's own constraints doc insists on).
// The bound noiseScale3D relies on (gradientMaxContribution3D, a triangle-
// inequality bound rather than a tight numeric supremum -- see
// opensimplex3.go's comment) is deliberately loose, so this does not assert
// the field reaches close to the edges the way TestEval2DUsesMostOfItsRange
// does for 2D; it only asserts the range is real, not a name for zero.
func TestEval3DStaysInRange(t *testing.T) {
	rng := rand.New(rand.NewSource(20260920))

	lo, hi := math.Inf(1), math.Inf(-1)
	for seed := int64(0); seed < 6; seed++ {
		s := NewSeed(seed)
		for i := 0; i < 20000; i++ {
			x := madd(rng.Float64(), 4000, -2000)
			y := madd(rng.Float64(), 4000, -2000)
			z := madd(rng.Float64(), 4000, -2000)
			v := s.Eval3D(x, y, z)
			if v < -1 || v > 1 {
				t.Fatalf("seed %d: Eval3D(%v, %v, %v) = %v, outside [-1, 1]", seed, x, y, z, v)
			}
			lo = math.Min(lo, v)
			hi = math.Max(hi, v)
		}
	}

	// A field that only ever returns exactly 0 would pass the bound above
	// vacuously. Measured, this field reaches roughly +-0.25 over this
	// sample; requiring a much smaller fraction keeps the test from being
	// re-broken by a legitimate future retune while still catching a
	// collapsed-to-zero regression outright.
	const minSpan = 0.05
	if lo > -minSpan || hi < minSpan {
		t.Fatalf("Eval3D's sampled range [%.6f, %.6f] does not reach +-%.2f; the field looks degenerate", lo, hi, minSpan)
	}
}

// Criterion 2 (implicit): sampling at chunk-adjacent but distinct coordinates
// must not collapse to a repeating pattern -- Eval3D must actually depend on
// all three axes, not silently ignore one (e.g. a copy-paste bug that used x
// twice and never read z).
func TestEval3DDependsOnEveryAxis(t *testing.T) {
	s := NewSeed(7)
	base := s.Eval3D(1.23, 4.56, 7.89)

	if v := s.Eval3D(9.87, 4.56, 7.89); v == base {
		t.Fatal("changing x alone did not change Eval3D's result")
	}
	if v := s.Eval3D(1.23, 9.87, 7.89); v == base {
		t.Fatal("changing y alone did not change Eval3D's result")
	}
	if v := s.Eval3D(1.23, 4.56, 9.87); v == base {
		t.Fatal("changing z alone did not change Eval3D's result")
	}
}

// Adjacent seeds must not produce correlated 3D fields either -- the same
// property NewSeed documents and M17 measured for Eval2D, checked here for
// Eval3D since it is a new consumer of the same permutation table.
func TestEval3DDifferentSeedsGiveDifferentFields(t *testing.T) {
	a := NewSeed(500)
	b := NewSeed(501)

	const samples = 300
	differences := 0
	for i := 0; i < samples; i++ {
		x := float64(i)*3.1 - 400
		y := float64(i)*1.7 + 20
		z := float64(i)*2.3 - 100
		if a.Eval3D(x, y, z) != b.Eval3D(x, y, z) {
			differences++
		}
	}
	if differences < samples/2 {
		t.Fatalf("Seed(500) and Seed(501) agree on %d/%d sampled Eval3D points; expected most to differ", samples-differences, samples)
	}
}
