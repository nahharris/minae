// Package core holds the small value types shared by the simulation layer.
//
// Every other package in this project pulls in raylib, which is cgo and so
// needs OpenGL and X11 headers just to compile a test binary. These types
// exist so the pure simulation logic can be written, and tested, without any
// of that: core depends on nothing outside the standard library.
package core

import "math"

// Vec3 is a 3-component vector.
type Vec3 struct{ X, Y, Z float32 }

// Length returns the Euclidean length of v.
//
// The components are widened to float64 before squaring. Squaring in float32
// first would overflow to +Inf for components above roughly 1e19, even though
// the resulting length is perfectly representable.
func (v Vec3) Length() float32 {
	x, y, z := float64(v.X), float64(v.Y), float64(v.Z)
	return float32(math.Sqrt(x*x + y*y + z*z))
}

// Normalize returns the unit vector pointing in the same direction as v.
//
// A zero-length vector has no direction, so there is no correct answer to
// return; dividing by the length would yield NaN components that then spread
// silently through every later calculation. Normalize deliberately returns the
// zero vector in that case, so callers get a harmless value instead of a
// poisoned one. Callers that need to distinguish "no direction" from a genuine
// unit vector must check Length themselves.
func (v Vec3) Normalize() Vec3 {
	length := v.Length()
	if length == 0 {
		return Vec3{}
	}
	return Vec3{X: v.X / length, Y: v.Y / length, Z: v.Z / length}
}

// RGBA is an 8-bit-per-channel colour.
type RGBA struct{ R, G, B, A uint8 }
