package noise

import (
	"math"
	"math/rand"
	"testing"
)

// findFusionSensitiveTriple returns a, b, c for which the fused and unfused
// evaluations of a*b + c genuinely disagree, so a test built on them is
// testing something. Most products happen to be exactly representable, in
// which case the two agree and prove nothing, so this searches rather than
// assuming.
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

// TestMaddIsAlwaysFused states the contract the package's cross-architecture
// determinism rests on.
//
// The contract is deliberately the opposite of what it first looks like it
// should be. The intuitive goal is "never fuse", so every machine does the
// two roundings — but Go gives no reliable way to say that: float64(a*b) + c
// reads like a barrier and is not one, because the conversion changes no
// value and the compiler drops it before the fusion rewrite runs. This
// package shipped that spelling and arm64 fused it anyway.
//
// "Always fuse" is achievable, because math.FMA is *defined* as the exactly
// rounded fused result. Both spellings give one deterministic answer; only
// this one is enforceable.
func TestMaddIsAlwaysFused(t *testing.T) {
	a, b, c := findFusionSensitiveTriple(t)

	if got, want := madd(a, b, c), math.FMA(a, b, c); got != want {
		t.Errorf("madd(%v, %v, %v) = %v, want the fused %v", a, b, c, got, want)
	}

	// And confirm the triple actually distinguishes the two, so a future
	// change to the search cannot leave this test passing vacuously.
	if math.FMA(a, b, c) == float64(a*b)+c {
		t.Fatal("the chosen triple does not distinguish fused from unfused; the test proves nothing")
	}
}

// TestDot2IsAlwaysFused is the same contract for the two-product shape.
//
// dot2 folds its left product into the addition and leaves the right as a
// plain multiply. Which side is folded is arbitrary; that it is fixed here,
// rather than decided by whichever architecture compiled the code, is not.
func TestDot2IsAlwaysFused(t *testing.T) {
	ax, bx, ay := findFusionSensitiveTriple(t)
	by := ax

	if got, want := dot2(ax, ay, bx, by), math.FMA(ax, bx, ay*by); got != want {
		t.Errorf("dot2 = %v, want the fused %v", got, want)
	}
}

// TestDot2IsTheDotProduct guards the ordering of dot2's parameters.
//
// This is not hypothetical. The spline limiter called
// dot2(alpha, alpha, beta, beta) intending alpha^2 + beta^2 and got 2*alpha*beta,
// because the signature interleaves the two vectors rather than taking them
// as pairs. That silently broke monotonicity. The signature is worth keeping
// — it reads naturally at the noise call sites — but it deserves a test that
// states plainly which argument is which.
func TestDot2IsTheDotProduct(t *testing.T) {
	// dot2(ax, ay, bx, by) is (ax, ay) . (bx, by) = ax*bx + ay*by.
	if got, want := dot2(2, 3, 5, 7), 2.0*5.0+3.0*7.0; got != want {
		t.Errorf("dot2(2, 3, 5, 7) = %v, want %v (ax*bx + ay*by)", got, want)
	}
	if got, want := dot2(1, 0, 0, 1), 0.0; got != want {
		t.Errorf("dot2 of perpendicular unit vectors = %v, want %v", got, want)
	}
}
