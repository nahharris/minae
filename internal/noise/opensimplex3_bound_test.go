package noise

import (
	"math"
	"testing"
)

// TestRawSumSupremum3DIsSound re-derives rawSumSupremum3D, the constant
// noiseScale3D is built on, so it cannot drift away from the lattice geometry
// it describes.
//
// It mirrors Eval3D's own corner selection but, instead of hashing a gradient,
// gives every corner its best case over all sixteen independently, which is
// what makes the result an upper bound whatever gradients a seed selects.
func TestRawSumSupremum3DIsSound(t *testing.T) {
	best, bx, by, bz := 0.0, 0.0, 0.0, 0.0

	// A coarse grid over more than one lattice period in every direction, then
	// local refinement. The maximum sits in the interior of a cell, so the
	// coarse pass only has to land near it.
	const steps = 48
	const span = 3.2
	for i := 0; i <= steps; i++ {
		for j := 0; j <= steps; j++ {
			for k := 0; k <= steps; k++ {
				x := float64(i)/steps*span - 0.1
				y := float64(j)/steps*span - 0.1
				z := float64(k)/steps*span - 0.1
				if v := rawCornerSumUpperBound3D(x, y, z); v > best {
					best, bx, by, bz = v, x, y, z
				}
			}
		}
	}
	step := span / steps
	for pass := 0; pass < 30; pass++ {
		for i := -4; i <= 4; i++ {
			for j := -4; j <= 4; j++ {
				for k := -4; k <= 4; k++ {
					x, y, z := bx+float64(i)*step/4, by+float64(j)*step/4, bz+float64(k)*step/4
					if v := rawCornerSumUpperBound3D(x, y, z); v > best {
						best, bx, by, bz = v, x, y, z
					}
				}
			}
		}
		step /= 2
	}

	if best > rawSumSupremum3D {
		t.Fatalf("search found a raw sum of %.12g, above the claimed supremum %.12g; "+
			"noiseScale3D is built on this and Eval3D could exceed its documented range",
			best, rawSumSupremum3D)
	}
	// The other direction matters as much: a supremum far above anything a
	// position can reach is safe but wastes range, which is the exact defect
	// this constant replaced.
	if best < rawSumSupremum3D*0.99 {
		t.Fatalf("search reached only %.12g against a claimed supremum of %.12g; "+
			"the bound has become loose and Eval3D is wasting its range",
			best, rawSumSupremum3D)
	}
}

// TestNumericalSupremum3DRespectsTheProvedBound cross-checks the numerical
// search against the triangle-inequality bound, which owes nothing to
// sampling. A search that ever landed above it would be broken.
func TestNumericalSupremum3DRespectsTheProvedBound(t *testing.T) {
	if rawSumSupremum3D > rawSumBound3DProved {
		t.Fatalf("rawSumSupremum3D (%.12g) exceeds the proved bound (%.12g); the numerical search is wrong",
			rawSumSupremum3D, rawSumBound3DProved)
	}
	// And it must be at least one corner's proved maximum, since a single
	// corner at its optimal radius with the others out of range is achievable.
	if rawSumSupremum3D < gradientMaxContribution3D {
		t.Fatalf("rawSumSupremum3D (%.12g) is below one corner's own maximum (%.12g)",
			rawSumSupremum3D, gradientMaxContribution3D)
	}
}

// TestEval3DUsesMostOfItsRange is the half of the range criterion that
// TestEval3DStaysInRange cannot supply. A field pinned at zero stays inside
// [-1, 1] perfectly; M20 first shipped a 3D field that peaked at 0.2475, and
// every staying-in-range test passed.
func TestEval3DUsesMostOfItsRange(t *testing.T) {
	const wantAtLeast = 0.85

	peak := 0.0
	for _, seedValue := range []int64{1, 7, 99, -5, 12345} {
		s := NewSeed(seedValue)
		for i := range 60 {
			for j := range 60 {
				for k := range 60 {
					v := math.Abs(s.Eval3D(float64(i)*0.137, float64(j)*0.113, float64(k)*0.127))
					peak = math.Max(peak, v)
				}
			}
		}
	}

	if peak < wantAtLeast {
		t.Errorf("Eval3D peaks at %.4f, well inside its documented [-1, 1]; "+
			"noiseScale3D is too conservative and every consumer loses resolution", peak)
	}
	if peak > 1 {
		t.Errorf("Eval3D reached %.4f, outside its documented [-1, 1]", peak)
	}
}

// rawCornerSumUpperBound3D returns the largest four-corner sum any seed could
// produce at (x, y, z), by giving every corner its most favourable gradient.
func rawCornerSumUpperBound3D(x, y, z float64) float64 {
	skew := (x + y + z) * f3
	i, j, k := math.Floor(x+skew), math.Floor(y+skew), math.Floor(z+skew)
	unskew := (i + j + k) * g3
	x0, y0, z0 := x-(i-unskew), y-(j-unskew), z-(k-unskew)

	// The same six-way simplex selection Eval3D makes.
	var i1, j1, k1, i2, j2, k2 float64
	switch {
	case x0 >= y0 && y0 >= z0:
		i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 1, 0
	case x0 >= y0 && x0 >= z0:
		i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 0, 1
	case x0 >= y0:
		i1, j1, k1, i2, j2, k2 = 0, 0, 1, 1, 0, 1
	case y0 < z0:
		i1, j1, k1, i2, j2, k2 = 0, 0, 1, 0, 1, 1
	case x0 < z0:
		i1, j1, k1, i2, j2, k2 = 0, 1, 0, 0, 1, 1
	default:
		i1, j1, k1, i2, j2, k2 = 0, 1, 0, 1, 1, 0
	}

	offsets := [4][3]float64{
		{x0, y0, z0},
		{x0 - i1 + g3, y0 - j1 + g3, z0 - k1 + g3},
		{x0 - i2 + g3x2, y0 - j2 + g3x2, z0 - k2 + g3x2},
		{x0 - 1 + g3x3, y0 - 1 + g3x3, z0 - 1 + g3x3},
	}

	total := 0.0
	for _, d := range offsets {
		tt := 0.5 - d[0]*d[0] - d[1]*d[1] - d[2]*d[2]
		if tt <= 0 {
			continue
		}
		t2 := tt * tt
		total += t2 * t2 * bestGradientDot3D(d[0], d[1], d[2])
	}
	return total
}

// bestGradientDot3D returns the largest dot(g, d) over the sixteen 3D
// gradients.
func bestGradientDot3D(dx, dy, dz float64) float64 {
	best := 0.0
	for _, g := range gradients3D {
		if v := g[0]*dx + g[1]*dy + g[2]*dz; v > best {
			best = v
		}
	}
	return best
}
