package mesh

// This file is compiled only for tests, so nothing here widens the package's
// real API.

// SetAORampForTest replaces the ambient-occlusion ramp and returns a function
// that restores the previous one.
//
// It exists so a test can prove the triangulation flip is independent of the
// ramp's spacing. That property is invisible while the ramp is evenly spaced,
// which it is today, so it can only be tested by making it uneven.
func SetAORampForTest(ramp [4]uint8) (restore func()) {
	previous := aoRamp
	aoRamp = ramp
	return func() { aoRamp = previous }
}
