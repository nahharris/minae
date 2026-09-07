package noise

import (
	"math"
	"math/rand"
	"testing"
)

// findFusionSensitiveTriple returns a, b, c for which the fused and unfused
// evaluations of a*b + c genuinely disagree, so a test built on them is
// testing something. Not every triple distinguishes them — most products
// happen to be exactly representable — so this searches rather than assuming.
func findFusionSensitiveTriple(t *testing.T) (a, b, c float64) {
	t.Helper()

	rng := rand.New(rand.NewSource(1))
	for range 100_000 {
		a = rng.NormFloat64()
		b = rng.NormFloat64()
		c = rng.NormFloat64()
		if math.FMA(a, b, c) != float64(a*b)+c {
			return a, b, c
		}
	}
	t.Fatal("found no triple where fused and unfused a*b+c differ; this test cannot prove anything")
	return 0, 0, 0
}

// TestMaddDoesNotFuse is the direct statement of the property the whole
// package's cross-architecture determinism rests on.
//
// madd must compute the product, round it to float64, and only then add —
// never the single fused operation that skips the intermediate rounding. Go
// emits FMA on arm64 and not on amd64, so a madd that fused would produce a
// different world from the same seed on an Apple Silicon machine than on an
// x86 one.
//
// What this catches, precisely, because the distinction matters:
//
//   - Rewriting madd to call math.FMA explicitly fails everywhere, including
//     amd64, since math.FMA fuses regardless of what the hardware would have
//     chosen. Verified by mutation.
//   - Rewriting madd as a bare `a*b + c` fails only where the compiler
//     actually fuses — arm64 and friends, not amd64. That case is exactly
//     why CI runs this package on arm64 as well.
//
// So this test is not a substitute for the arm64 job, and the arm64 job is
// not a substitute for it. The value of stating the property here is the
// failure message: a fused madd says so directly, where a golden-vector
// mismatch only reports that some number changed.
func TestMaddDoesNotFuse(t *testing.T) {
	a, b, c := findFusionSensitiveTriple(t)

	unfused := float64(a*b) + c
	fused := math.FMA(a, b, c)

	if got := madd(a, b, c); got != unfused {
		t.Errorf("madd(%v, %v, %v) = %v, want the unfused %v", a, b, c, got, unfused)
	}
	if got := madd(a, b, c); got == fused {
		t.Errorf("madd(%v, %v, %v) produced the fused result %v; the explicit "+
			"float64(...) rounding has been lost and this package is no longer "+
			"deterministic across architectures", a, b, c, got)
	}
}

// TestDot2DoesNotFuse is the same property for the two-product shape.
//
// dot2 needs both products rounded, not one: ax*bx + ay*by offers the
// compiler two possible fusions, so rounding only one of them would leave the
// other free to fuse. The test therefore checks against both single-sided
// fusions, not just the combined result.
func TestDot2DoesNotFuse(t *testing.T) {
	ax, bx, other := findFusionSensitiveTriple(t)
	// Reuse the sensitive pair for the left product and build a right-hand
	// product that is sensitive in the same way.
	ay, by := other, ax

	unfused := float64(ax*bx) + float64(ay*by)

	if got := dot2(ax, ay, bx, by); got != unfused {
		t.Errorf("dot2 = %v, want the fully unfused %v", got, unfused)
	}

	// Either product folding into the addition is a distinct bug, so name
	// them separately: a fix that rounds only one would otherwise look fine.
	if fusedLeft := math.FMA(ax, bx, float64(ay*by)); unfused != fusedLeft {
		if got := dot2(ax, ay, bx, by); got == fusedLeft {
			t.Error("dot2 fused its left product into the addition")
		}
	}
	if fusedRight := math.FMA(ay, by, float64(ax*bx)); unfused != fusedRight {
		if got := dot2(ax, ay, bx, by); got == fusedRight {
			t.Error("dot2 fused its right product into the addition")
		}
	}
}

// TestDot2IsTheDotProduct guards the ordering of dot2's parameters.
//
// This is not a hypothetical. The spline code called dot2(alpha, alpha, beta,
// beta) intending alpha^2 + beta^2 and got 2*alpha*beta, because dot2's
// signature interleaves the two vectors rather than taking them as pairs.
// That silently broke the monotonicity limiter. The signature is worth
// keeping — it reads naturally at the noise call sites — but it deserves a
// test that states plainly which argument is which.
func TestDot2IsTheDotProduct(t *testing.T) {
	// dot2(ax, ay, bx, by) is (ax, ay) . (bx, by) = ax*bx + ay*by.
	if got, want := dot2(2, 3, 5, 7), 2.0*5.0+3.0*7.0; got != want {
		t.Errorf("dot2(2, 3, 5, 7) = %v, want %v (ax*bx + ay*by)", got, want)
	}
	if got, want := dot2(1, 0, 0, 1), 0.0; got != want {
		t.Errorf("dot2 of perpendicular unit vectors = %v, want %v", got, want)
	}
}
