package core

import (
	"math"
	"testing"
)

// float32 carries a 24-bit mantissa, so about 1.2e-7 of relative precision.
// Length runs the components through a sqrt and Normalize adds a division on
// top, which costs a few units in the last place. 1e-6 sits an order of
// magnitude above that accumulated error while still being far tighter than
// any deviation that would signal a real bug.
const epsilon = 1e-6

// closeEnough reports whether got and want agree to within epsilon. NaN is
// never close to anything, including itself, so this also fails loudly if a
// calculation degenerates.
func closeEnough(got, want float32) bool {
	return math.Abs(float64(got-want)) <= epsilon
}

func TestVec3Length(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    Vec3
		want float32
	}{
		{"zero vector", Vec3{}, 0},
		{"3-4-5 triangle in xy", Vec3{X: 3, Y: 4}, 5},
		{"3-4-5 triangle in yz", Vec3{Y: 3, Z: 4}, 5},
		{"3-4-5 triangle in xz", Vec3{X: 4, Z: 3}, 5},
		{"1-2-2 gives 3", Vec3{X: 1, Y: 2, Z: 2}, 3},
		{"2-3-6 gives 7", Vec3{X: 2, Y: 3, Z: 6}, 7},
		{"unit x", Vec3{X: 1}, 1},
		{"unit y", Vec3{Y: 1}, 1},
		{"unit z", Vec3{Z: 1}, 1},
		{"negative components", Vec3{X: -3, Y: -4}, 5},
		{"mixed signs", Vec3{X: -1, Y: 2, Z: -2}, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.v.Length(); !closeEnough(got, tt.want) {
				t.Errorf("%+v.Length() = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

func TestVec3Normalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    Vec3
		want Vec3
	}{
		{"unit x is unchanged", Vec3{X: 1}, Vec3{X: 1}},
		{"unit y is unchanged", Vec3{Y: 1}, Vec3{Y: 1}},
		{"unit z is unchanged", Vec3{Z: 1}, Vec3{Z: 1}},
		{"negative unit z is unchanged", Vec3{Z: -1}, Vec3{Z: -1}},
		{"axis-aligned scaled down", Vec3{X: 10}, Vec3{X: 1}},
		{"3-4-5 in xy", Vec3{X: 3, Y: 4}, Vec3{X: 0.6, Y: 0.8}},
		{"1-2-2 in all axes", Vec3{X: 1, Y: 2, Z: 2}, Vec3{X: 1.0 / 3, Y: 2.0 / 3, Z: 2.0 / 3}},
		{"negatives keep their sign", Vec3{X: -3, Y: -4}, Vec3{X: -0.6, Y: -0.8}},
		{"very small vector", Vec3{X: 3e-6, Y: 4e-6}, Vec3{X: 0.6, Y: 0.8}},
		{"very large vector", Vec3{X: 3e12, Y: 4e12}, Vec3{X: 0.6, Y: 0.8}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.v.Normalize()
			if !closeEnough(got.X, tt.want.X) || !closeEnough(got.Y, tt.want.Y) || !closeEnough(got.Z, tt.want.Z) {
				t.Errorf("%+v.Normalize() = %+v, want %+v", tt.v, got, tt.want)
			}

			// Whatever the input magnitude, the result must be a unit vector.
			if length := got.Length(); !closeEnough(length, 1) {
				t.Errorf("%+v.Normalize() has length %v, want 1", tt.v, length)
			}
		})
	}
}

// The zero vector has no direction, so Normalize returns the zero vector
// rather than dividing by zero. Getting this wrong produces NaN components
// that propagate silently, so assert on NaN directly instead of relying on an
// equality check that NaN would fail for the wrong reason.
func TestVec3NormalizeZeroVector(t *testing.T) {
	t.Parallel()

	got := Vec3{}.Normalize()

	components := []struct {
		axis  string
		value float32
	}{
		{"X", got.X},
		{"Y", got.Y},
		{"Z", got.Z},
	}

	for _, c := range components {
		if math.IsNaN(float64(c.value)) {
			t.Errorf("Vec3{}.Normalize().%s is NaN, want 0", c.axis)
		}
		if math.IsInf(float64(c.value), 0) {
			t.Errorf("Vec3{}.Normalize().%s is Inf, want 0", c.axis)
		}
	}

	if got != (Vec3{}) {
		t.Errorf("Vec3{}.Normalize() = %+v, want %+v", got, Vec3{})
	}
}

// Normalizing an already-normalized vector must not drift. Direction vectors
// get renormalized repeatedly as the camera moves, so any per-call error would
// compound over frames.
func TestVec3NormalizeIsStable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    Vec3
	}{
		{"unit x", Vec3{X: 1}},
		{"negative unit y", Vec3{Y: -1}},
		{"3-4-5 direction", Vec3{X: 0.6, Y: 0.8}},
		{"1-2-2 direction", Vec3{X: 1.0 / 3, Y: 2.0 / 3, Z: 2.0 / 3}},
		{"diagonal", Vec3{X: 1, Y: 1, Z: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			once := tt.v.Normalize()

			got := once
			for range 10 {
				got = got.Normalize()
			}

			if !closeEnough(got.X, once.X) || !closeEnough(got.Y, once.Y) || !closeEnough(got.Z, once.Z) {
				t.Errorf("normalizing %+v 11 times = %+v, want %+v after one", tt.v, got, once)
			}
			if length := got.Length(); !closeEnough(length, 1) {
				t.Errorf("repeatedly normalized %+v has length %v, want 1", tt.v, length)
			}
		})
	}
}

// Squaring in float32 before widening overflows to +Inf for large components,
// even when the resulting length is representable. Length widens first, so
// these must come back finite and correct.
func TestVec3Length_LargeComponentsDoNotOverflow(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		v    Vec3
		want float32
	}{
		{"1e20 on one axis", Vec3{X: 1e20}, 1e20},
		{"1e30 on one axis", Vec3{Y: 1e30}, 1e30},
		{"3-4-5 scaled to 1e20", Vec3{X: 3e20, Y: 4e20}, 5e20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.v.Length()
			if math.IsInf(float64(got), 0) || math.IsNaN(float64(got)) {
				t.Fatalf("Length() = %v, want a finite %v", got, tt.want)
			}
			// Relative comparison: absolute epsilon is meaningless at this scale.
			if diff := math.Abs(float64(got-tt.want) / float64(tt.want)); diff > epsilon {
				t.Errorf("Length() = %v, want %v (relative difference %g)", got, tt.want, diff)
			}
		})
	}
}
