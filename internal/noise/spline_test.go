package noise

import (
	"math/rand"
	"testing"
)

// Criterion 6, first half: evaluation passes exactly through the control
// points.
func TestSplinePassesThroughControlPoints(t *testing.T) {
	points := []SplinePoint{{X: 0, Y: 0}, {X: 1, Y: 0.2}, {X: 2, Y: 0.9}, {X: 5, Y: 1}}
	sp, err := NewSpline(points)
	if err != nil {
		t.Fatalf("NewSpline(%v): %v", points, err)
	}

	for _, p := range points {
		if got := sp.Eval(p.X); got != p.Y {
			t.Errorf("Eval(%v) = %v, want exactly %v", p.X, got, p.Y)
		}
	}
}

// Criterion 6, second half: the constructor validates monotonicity where
// declared, for both increasing and decreasing curves, and evaluation
// between control points never overshoots into non-monotone territory --
// the specific failure mode a naive (non-limited) cubic spline has, which is
// exactly what monotoneTangents's Fritsch-Carlson limiter exists to prevent.
func TestSplineEvalIsMonotoneBetweenControlPoints(t *testing.T) {
	cases := []struct {
		name   string
		points []SplinePoint
	}{
		{"increasing", []SplinePoint{{0, 0}, {1, 0.05}, {2, 0.9}, {3, 0.95}, {4, 1}}},
		{"decreasing", []SplinePoint{{0, 10}, {1, 9.9}, {2, 1}, {3, 0.5}, {4, 0}}},
		{"flat run then rise", []SplinePoint{{0, 0}, {1, 0}, {2, 0}, {3, 5}}},
		{"steep then shallow", []SplinePoint{{0, 0}, {1, 100}, {2, 100.1}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sp, err := NewSpline(c.points)
			if err != nil {
				t.Fatalf("NewSpline(%v): %v", c.points, err)
			}

			increasing := c.points[len(c.points)-1].Y >= c.points[0].Y

			const steps = 2000
			minX, maxX := c.points[0].X, c.points[len(c.points)-1].X
			prev := sp.Eval(minX)
			for i := 1; i <= steps; i++ {
				x := minX + (maxX-minX)*float64(i)/steps
				v := sp.Eval(x)
				if increasing && v < prev-1e-9 {
					t.Fatalf("spline decreased at x=%v: %v -> %v, want non-decreasing", x, prev, v)
				}
				if !increasing && v > prev+1e-9 {
					t.Fatalf("spline increased at x=%v: %v -> %v, want non-increasing", x, prev, v)
				}
				prev = v
			}
		})
	}
}

// The constructor must reject non-monotone control points rather than
// silently building a curve that folds back on itself.
func TestNewSplineRejectsNonMonotonePoints(t *testing.T) {
	_, err := NewSpline([]SplinePoint{{0, 0}, {1, 1}, {2, 0.5}, {3, 2}})
	if err == nil {
		t.Fatal("NewSpline accepted non-monotone control points (up, down, up); want an error")
	}
}

func TestNewSplineRejectsTooFewPoints(t *testing.T) {
	if _, err := NewSpline(nil); err == nil {
		t.Fatal("NewSpline(nil) did not return an error")
	}
	if _, err := NewSpline([]SplinePoint{{0, 0}}); err == nil {
		t.Fatal("NewSpline with a single point did not return an error")
	}
}

func TestNewSplineRejectsDuplicateX(t *testing.T) {
	_, err := NewSpline([]SplinePoint{{0, 0}, {1, 1}, {1, 2}})
	if err == nil {
		t.Fatal("NewSpline accepted two control points with the same X; want an error")
	}
}

// A constant Y is trivially both non-decreasing and non-increasing, and must
// be accepted rather than rejected as neither.
func TestNewSplineAcceptsConstantY(t *testing.T) {
	sp, err := NewSpline([]SplinePoint{{0, 5}, {1, 5}, {2, 5}})
	if err != nil {
		t.Fatalf("NewSpline with constant Y: %v", err)
	}
	if v := sp.Eval(0.5); v != 5 {
		t.Fatalf("Eval(0.5) on a constant spline = %v, want 5", v)
	}
}

// Points need not be supplied in X order; NewSpline sorts them.
func TestNewSplineSortsUnorderedInput(t *testing.T) {
	sp, err := NewSpline([]SplinePoint{{2, 1}, {0, 0}, {1, 0.5}})
	if err != nil {
		t.Fatalf("NewSpline with out-of-order points: %v", err)
	}
	if v := sp.Eval(0); v != 0 {
		t.Fatalf("Eval(0) = %v, want 0", v)
	}
	if v := sp.Eval(2); v != 1 {
		t.Fatalf("Eval(2) = %v, want 1", v)
	}
}

// Outside the control points' range, Eval clamps to the nearest endpoint.
func TestSplineEvalClampsOutsideRange(t *testing.T) {
	sp, err := NewSpline([]SplinePoint{{0, 1}, {1, 2}, {2, 4}})
	if err != nil {
		t.Fatalf("NewSpline: %v", err)
	}
	if v := sp.Eval(-10); v != 1 {
		t.Fatalf("Eval(-10) = %v, want 1 (clamped to the first point)", v)
	}
	if v := sp.Eval(10); v != 4 {
		t.Fatalf("Eval(10) = %v, want 4 (clamped to the last point)", v)
	}
}

// Criterion 1, for Spline: same input, same output, evaluated repeatedly and
// out of order.
func TestSplineDeterministic(t *testing.T) {
	sp, err := NewSpline([]SplinePoint{{0, 0}, {1, 0.3}, {2, 0.8}, {3, 1}})
	if err != nil {
		t.Fatalf("NewSpline: %v", err)
	}

	rng := rand.New(rand.NewSource(61))
	xs := make([]float64, 200)
	for i := range xs {
		xs[i] = rng.Float64() * 3
	}

	first := make([]float64, len(xs))
	for i, x := range xs {
		first[i] = sp.Eval(x)
	}
	for i := len(xs) - 1; i >= 0; i-- {
		if got := sp.Eval(xs[i]); got != first[i] {
			t.Fatalf("Eval(%v) = %v evaluated a second time out of order, want %v", xs[i], got, first[i])
		}
	}
}
