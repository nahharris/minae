package noise

import (
	"math"
	"testing"
)

// TestGradientMaxContributionIsTheSupremum numerically checks the closed-form
// claim in gradientMaxContribution's doc comment: that f(r) = (0.5-r^2)^4 * r
// is maximised at r = sqrt(1/18), not somewhere else the calculus in the
// comment got wrong. This is a proof-carrying check on the derivation
// noiseScale depends on for criterion 3 (range is bounded) -- if this test
// ever fails, the bound noiseScale relies on is unsound and Eval2D's [-1, 1]
// guarantee is not actually proved, only hoped for.
func TestGradientMaxContributionIsTheSupremum(t *testing.T) {
	f := func(r float64) float64 {
		t := 0.5 - r*r
		if t <= 0 {
			return 0
		}
		return t * t * t * t * r
	}

	const steps = 1_000_000
	rMax := math.Sqrt(0.5)
	var best float64
	for i := 0; i <= steps; i++ {
		r := rMax * float64(i) / steps
		if v := f(r); v > best {
			best = v
		}
	}

	if best > gradientMaxContribution*(1+1e-9) {
		t.Fatalf("numeric search found a larger single-corner contribution (%.17g) than the claimed bound gradientMaxContribution (%.17g); the derivation is wrong and noiseScale's [-1,1] guarantee does not hold",
			best, gradientMaxContribution)
	}
	// The numeric search should also get close to the claimed bound -- if it
	// is far below, the closed form is claiming a much looser (still safe,
	// but suspiciously so) bound than intended, which would be a sign the
	// formula was transcribed wrong even though it happens not to be unsafe.
	if best < gradientMaxContribution*0.999 {
		t.Fatalf("numeric search found only %.17g, well below the claimed supremum %.17g; check the closed-form derivation",
			best, gradientMaxContribution)
	}
}

// TestRawSumSupremumIsSound re-derives rawSumSupremum, the constant
// noiseScale is built on, so it cannot drift away from the lattice geometry
// it describes.
//
// The search mirrors Eval2D's own corner arithmetic, but instead of hashing
// a gradient it takes each corner's best case over all sixteen, independently
// per corner -- which is what makes the result an upper bound regardless of
// which gradients any particular seed's hash actually selects.
func TestRawSumSupremumIsSound(t *testing.T) {
	best := 0.0
	const steps = 1200
	for i := 0; i <= steps; i++ {
		for j := 0; j <= steps; j++ {
			x := float64(i) / float64(steps) * 2.0
			y := float64(j) / float64(steps) * 2.0
			if v := rawCornerSumUpperBound(x, y); v > best {
				best = v
			}
		}
	}

	if best > rawSumSupremum {
		t.Fatalf("search found a raw sum of %.10g, above the claimed supremum %.10g; "+
			"noiseScale is built on this and Eval2D could exceed its documented range",
			best, rawSumSupremum)
	}
	// Also guard the other direction. A supremum far above what any position
	// can reach would still be safe, but it would mean the field silently
	// stopped using its range -- the exact defect this constant replaced.
	if best < rawSumSupremum*0.99 {
		t.Fatalf("search reached only %.10g against a claimed supremum of %.10g; "+
			"the bound has become loose and Eval2D is wasting its range",
			best, rawSumSupremum)
	}
}

// TestNumericalSupremumRespectsTheProvedBound cross-checks the numerical
// search against the closed form derived by hand.
//
// rawSumSupremum comes from a grid search, and a grid search can land
// somewhere wrong. The triangle inequality gives an independent bound that
// owes nothing to sampling: three corners, each individually no larger than
// gradientMaxContribution. If the search ever returned something above that,
// the search is broken rather than the geometry surprising.
func TestNumericalSupremumRespectsTheProvedBound(t *testing.T) {
	proved := 3 * gradientMaxContribution
	if rawSumSupremum > proved {
		t.Fatalf("rawSumSupremum (%.10g) exceeds the proved triangle-inequality bound (%.10g); the numerical search is wrong",
			rawSumSupremum, proved)
	}
}

// TestScaledRangeIsGuaranteed states the consequence the two tests above
// exist for, in the terms criterion 3 is written in.
func TestScaledRangeIsGuaranteed(t *testing.T) {
	if peak := rawSumSupremum * noiseScale; peak > 1 {
		t.Fatalf("a raw sum at the supremum scales to %.10g, outside the documented [-1, 1]", peak)
	}
}

// rawCornerSumUpperBound returns the largest three-corner sum any seed could
// produce at (x, y), by giving every corner its most favourable gradient.
func rawCornerSumUpperBound(x, y float64) float64 {
	skew := (x + y) * f2
	ip := math.Floor(x + skew)
	jp := math.Floor(y + skew)
	unskew := (ip + jp) * g2
	x0 := x - (ip - unskew)
	y0 := y - (jp - unskew)

	var i1, j1 float64
	if x0 > y0 {
		i1, j1 = 1, 0
	} else {
		i1, j1 = 0, 1
	}

	offsets := [3][2]float64{
		{x0, y0},
		{x0 - i1 + g2, y0 - j1 + g2},
		{x0 - 1 + g2x2, y0 - 1 + g2x2},
	}

	total := 0.0
	for _, d := range offsets {
		tt := 0.5 - d[0]*d[0] - d[1]*d[1]
		if tt <= 0 {
			continue
		}
		t2 := tt * tt
		total += t2 * t2 * bestGradientDot(d[0], d[1])
	}
	return total
}

// bestGradientDot returns the largest dot(g, d) over the sixteen gradients:
// the most any corner at offset d could contribute for the best possible
// gradient choice.
func bestGradientDot(dx, dy float64) float64 {
	best := 0.0
	for _, g := range gradients2D {
		if v := g[0]*dx + g[1]*dy; v > best {
			best = v
		}
	}
	return best
}
