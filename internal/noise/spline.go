package noise

import (
	"fmt"
	"math"
	"sort"
)

// SplinePoint is one control point a Spline is built from.
type SplinePoint struct {
	X, Y float64
}

// Spline is a monotone piecewise cubic curve through a set of control points,
// evaluated with monotone cubic Hermite interpolation (Fritsch-Carlson).
//
// This is the piece that makes Minecraft-style terrain control work: rather
// than writing arithmetic that turns a noise value into a height, a designer
// declares a handful of control points -- "this range of continentalness is
// coastline, this range is inland plateau" -- and the curve interpolates
// between them. Monotonicity is what makes that declaration trustworthy: a
// spline that is accidentally non-monotone folds back on itself, so a small
// increase in the input can produce a decrease in the output, which produces
// terrain that folds back on itself in a way that is very hard to diagnose
// from the generated output alone. NewSpline validates it at construction
// instead, so a bad set of control points fails loudly with the points
// responsible rather than shipping.
type Spline struct {
	points   []SplinePoint
	tangents []float64
}

// NewSpline builds a monotone spline through points, sorted by X.
//
// It returns an error if there are fewer than two points, if two points
// share an X (the curve would not be a function of X), or if the Y values are
// not monotone -- neither non-decreasing nor non-increasing -- across the
// sorted points. A constant Y (every point equal) is accepted: it is
// trivially both.
func NewSpline(points []SplinePoint) (*Spline, error) {
	if len(points) < 2 {
		return nil, fmt.Errorf("noise: spline needs at least 2 control points, got %d", len(points))
	}

	sorted := make([]SplinePoint, len(points))
	copy(sorted, points)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].X < sorted[j].X })

	for i := 1; i < len(sorted); i++ {
		if sorted[i].X == sorted[i-1].X {
			return nil, fmt.Errorf("noise: spline control points must have distinct X, got two points at X=%v", sorted[i].X)
		}
	}

	increasing, decreasing := true, true
	for i := 1; i < len(sorted); i++ {
		switch {
		case sorted[i].Y > sorted[i-1].Y:
			decreasing = false
		case sorted[i].Y < sorted[i-1].Y:
			increasing = false
		}
	}
	if !increasing && !decreasing {
		return nil, fmt.Errorf("noise: spline control points are not monotone in Y")
	}

	return &Spline{
		points:   sorted,
		tangents: monotoneTangents(sorted),
	}, nil
}

// monotoneTangents computes one tangent slope per control point using the
// Fritsch-Carlson method: an initial estimate from the secant slopes on
// either side of each point, then limited so the resulting Hermite cubic
// cannot overshoot and violate monotonicity between any pair of knots. A
// naive cubic spline through monotone points can still overshoot between
// them -- this limiting step is what a plain "smooth curve through the
// points" would be missing, and is exactly why monotonicity has to be a
// property of the interpolation method, not just of the input points.
func monotoneTangents(points []SplinePoint) []float64 {
	n := len(points)
	secants := make([]float64, n-1)
	for i := range secants {
		dx := points[i+1].X - points[i].X
		dy := points[i+1].Y - points[i].Y
		secants[i] = dy / dx
	}

	tangents := make([]float64, n)
	tangents[0] = secants[0]
	tangents[n-1] = secants[n-2]
	for i := 1; i < n-1; i++ {
		left, right := secants[i-1], secants[i]
		if left*right <= 0 {
			// A local extremum (or a flat run): forcing a non-zero slope
			// through it is exactly what would make the curve overshoot and
			// stop being monotone, so the tangent here is flat instead.
			tangents[i] = 0
			continue
		}
		// Pure addition of two already-computed secants; no multiply is
		// attached to this operation.
		tangents[i] = (left + right) / 2
	}

	// Fritsch-Carlson limiter: clamp each pair of adjacent tangents so the
	// cubic between points[i] and points[i+1] cannot overshoot the secant.
	for i := 0; i < n-1; i++ {
		delta := secants[i]
		if delta == 0 {
			tangents[i] = 0
			tangents[i+1] = 0
			continue
		}

		alpha := tangents[i] / delta
		beta := tangents[i+1] / delta

		// alpha and beta are both >= 0 here: the averaging step above only
		// ever produces a tangent with the same sign as the secants
		// surrounding it (or zero), so a tangent can never point the wrong
		// way relative to its own segment's secant.
		//
		// dot2(alpha, beta, alpha, beta) computes alpha*alpha + beta*beta:
		// dot2(ax, ay, bx, by) returns ax*bx + ay*by, so pairing each
		// variable with itself in the "a" and "b" argument slots is what
		// turns it into a sum of squares rather than the cross term
		// alpha*beta + alpha*beta a naive dot2(alpha, alpha, beta, beta)
		// call would silently compute instead.
		magnitudeSq := dot2(alpha, beta, alpha, beta)
		if magnitudeSq > 9 {
			tau := 3 / math.Sqrt(magnitudeSq)
			// Each of these is a pure product chain (tau*alpha, then that
			// times delta) with no addition in it, so it carries no fusion
			// risk regardless of grouping.
			tangents[i] = tau * alpha * delta
			tangents[i+1] = tau * beta * delta
		}
	}

	return tangents
}

// Eval returns the spline's value at x. Outside the control points' range it
// is clamped to the nearest endpoint's Y rather than extrapolated, since a
// cubic extrapolated past its last constraint has no basis for staying
// monotone or even bounded.
//
// Eval passes exactly through every control point: at x == points[i].X, t
// (see below) is exactly 0 or 1, which zeroes every basis term except the one
// carrying that point's own Y.
func (sp *Spline) Eval(x float64) float64 {
	points := sp.points
	last := len(points) - 1

	if x <= points[0].X {
		return points[0].Y
	}
	if x >= points[last].X {
		return points[last].Y
	}

	i := sp.segmentFor(x)
	p0, p1 := points[i], points[i+1]
	m0, m1 := sp.tangents[i], sp.tangents[i+1]

	h := p1.X - p0.X
	t := (x - p0.X) / h

	t2 := t * t
	t3 := t2 * t

	// The four Hermite basis functions, each an explicit accumulation built
	// with madd so every coefficient*power product's rounding is forced
	// before it is combined with the next term -- see the FMA design
	// decision. Plain "+t" and "+0" terms below carry no multiplication and
	// so need no forcing.
	h00 := madd(-3, t2, 1.0)
	h00 = madd(2, t3, h00)

	h10 := t
	h10 = madd(-2, t2, h10)
	h10 = madd(1, t3, h10)

	h01 := madd(3, t2, 0.0)
	h01 = madd(-2, t3, h01)

	h11 := madd(-1, t2, 0.0)
	h11 = madd(1, t3, h11)

	// h*m0 and h*m1 are pure products with no addition attached directly;
	// they are each multiplied again by a basis function below, and that
	// final product is what feeds the sum, so it is forced there instead.
	hm0 := h * m0
	hm1 := h * m1

	value := madd(h00, p0.Y, 0.0)
	value = madd(h10, hm0, value)
	value = madd(h01, p1.Y, value)
	value = madd(h11, hm1, value)
	return value
}

// segmentFor returns the index i such that x is in [points[i].X,
// points[i+1].X). Callers must already have clamped x to the spline's range.
func (sp *Spline) segmentFor(x float64) int {
	points := sp.points
	// sort.Search finds the first index whose X is > x; the segment x
	// belongs in starts one index before that. Points is small (a curve
	// with hundreds of control points would be unusual), so the O(log n)
	// here is not a performance concern -- it exists for correctness on
	// unevenly spaced points, not speed.
	i := sort.Search(len(points), func(i int) bool { return points[i].X > x })
	if i == 0 {
		return 0
	}
	return i - 1
}
