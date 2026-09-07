package noise

import "testing"

// Criterion 1, for Warp2D: same inputs, same displaced coordinate.
func TestWarp2DDeterministic(t *testing.T) {
	warp := NewSeed(51)
	x1, y1 := Warp2D(warp, 10, -3, 4)
	x2, y2 := Warp2D(warp, 10, -3, 4)
	if x1 != x2 || y1 != y2 {
		t.Fatalf("Warp2D(10, -3, 4) = (%v, %v) then (%v, %v); same inputs must agree", x1, y1, x2, y2)
	}
}

// Zero amplitude must be a true no-op: the whole point of Warp2D is that a
// caller can dial the effect in or out, and dialing it to zero should return
// exactly the input coordinate, not something merely close to it.
func TestWarp2DZeroAmplitudeIsIdentity(t *testing.T) {
	warp := NewSeed(52)
	wx, wy := Warp2D(warp, 7.25, -19.5, 0)
	if wx != 7.25 || wy != -19.5 {
		t.Fatalf("Warp2D with amplitude 0 = (%v, %v), want the input coordinate unchanged (7.25, -19.5)", wx, wy)
	}
}

// The x and y displacement must differ (in general) -- if Warp2D used the
// same sample for both axes, every point would be pushed along the x=y
// diagonal, which defeats the purpose of a 2D warp. warpDecorrelation exists
// specifically to avoid this.
func TestWarp2DDisplacesBothAxesDifferently(t *testing.T) {
	warp := NewSeed(53)

	sameDisplacement := 0
	const trials = 50
	for i := 0; i < trials; i++ {
		x, y := float64(i)*3.7, float64(i)*-2.1
		wx, wy := Warp2D(warp, x, y, 5)
		dx, dy := wx-x, wy-y
		if dx == dy {
			sameDisplacement++
		}
	}

	if sameDisplacement == trials {
		t.Fatal("Warp2D displaced x and y by exactly the same amount at every sample point; the two axes are not being decorrelated")
	}
}

// Larger amplitude must produce a larger (or at least not smaller)
// displacement from the original coordinate, on average -- a sanity check
// that amplitude actually scales the effect rather than being ignored.
func TestWarp2DAmplitudeScales(t *testing.T) {
	warp := NewSeed(54)

	var smallTotal, largeTotal float64
	const trials = 200
	for i := 0; i < trials; i++ {
		x, y := float64(i)*1.3, float64(i)*0.7
		sx, sy := Warp2D(warp, x, y, 1)
		lx, ly := Warp2D(warp, x, y, 10)
		smallTotal += absf(sx-x) + absf(sy-y)
		largeTotal += absf(lx-x) + absf(ly-y)
	}

	if largeTotal <= smallTotal {
		t.Fatalf("total displacement at amplitude 10 (%.4f) was not larger than at amplitude 1 (%.4f)", largeTotal, smallTotal)
	}
}
