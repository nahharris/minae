package world

import (
	"math"
	"testing"

	"github.com/nahharris/minae/internal/core"
)

// continuityEpsilon bounds how much any single skyTint channel may move
// between two hours 0.001 apart. The steepest ramp in the keyframe table is
// midnight (0.16) to dawn (0.45) over 0.22 of the cycle, a slope of about
// 1.3 per unit hour, so a 0.001 step should move a channel by roughly
// 0.0013. This is an order of magnitude looser than that, leaving room for
// float32 error while still catching a real discontinuity, which would jump
// by tenths or more.
const continuityEpsilon = 0.02

func newTimeOfDayAt(hour float32) *TimeOfDay {
	return &TimeOfDay{Time: hour, CycleDuration: 1}
}

// Every hour across the cycle must produce a usable, finite, in-range state.
// Sampling finely also exercises every keyframe segment, including the wrap
// segment from the last keyframe back to the first.
func TestTimeOfDay_GetLightingState_AllHoursFinite(t *testing.T) {
	for i := 0; i < 1000; i++ {
		hour := float32(i) / 1000
		tod := newTimeOfDayAt(hour)

		sky, tint := tod.GetLightingState()

		channels := []float32{tint.R, tint.G, tint.B}
		for _, c := range channels {
			if math.IsNaN(float64(c)) || math.IsInf(float64(c), 0) {
				t.Fatalf("hour %v: skyTint channel is %v, want finite", hour, c)
			}
			if c < 0 {
				t.Fatalf("hour %v: skyTint channel = %v, want >= 0", hour, c)
			}
		}

		// sky is uint8, so it is range-safe by construction; just confirm the
		// call didn't panic and returned a value (A should stay opaque).
		if sky.A != 255 {
			t.Errorf("hour %v: sky.A = %d, want 255", hour, sky.A)
		}
	}
}

// Stepping across the whole cycle in small increments must never produce a
// jump larger than continuityEpsilon in any skyTint channel. This is the
// test that pins the original snapping bug: getStateFromTime's dayStates
// slice omitted nightState and had no entry for hour in [0, 0.2), so the
// cycle fell through to a state whose NextState was nil and colours jumped
// instead of easing. It also specifically checks the wrap from hour 0.99
// back to 0.00, which only stays continuous if the keyframe ring treats the
// last keyframe and the first as neighbours instead of leaving a gap after
// the last keyframe.
func TestTimeOfDay_GetLightingState_Continuous(t *testing.T) {
	const step = float32(0.001)

	_, prevTint := newTimeOfDayAt(0).GetLightingState()

	for i := 1; i <= 1000; i++ {
		hour := float32(i) * step
		_, tint := newTimeOfDayAt(normalizeHour(hour)).GetLightingState()

		if d := math.Abs(float64(tint.R - prevTint.R)); d > continuityEpsilon {
			t.Errorf("hour %.3f: skyTint.R jumped by %v from previous step, want <= %v", hour, d, continuityEpsilon)
		}
		if d := math.Abs(float64(tint.G - prevTint.G)); d > continuityEpsilon {
			t.Errorf("hour %.3f: skyTint.G jumped by %v from previous step, want <= %v", hour, d, continuityEpsilon)
		}
		if d := math.Abs(float64(tint.B - prevTint.B)); d > continuityEpsilon {
			t.Errorf("hour %.3f: skyTint.B jumped by %v from previous step, want <= %v", hour, d, continuityEpsilon)
		}

		prevTint = tint
	}
}

// The 0.99 -> 0.00 wrap specifically, isolated from the general sweep above
// so a failure here points straight at the ring not being circular.
func TestTimeOfDay_GetLightingState_WrapsAcrossMidnight(t *testing.T) {
	_, beforeWrap := newTimeOfDayAt(0.999).GetLightingState()
	_, atWrap := newTimeOfDayAt(0.000).GetLightingState()

	if d := math.Abs(float64(atWrap.R - beforeWrap.R)); d > continuityEpsilon {
		t.Errorf("skyTint.R jumped by %v across the 0.999 -> 0.000 wrap, want <= %v", d, continuityEpsilon)
	}
	if d := math.Abs(float64(atWrap.G - beforeWrap.G)); d > continuityEpsilon {
		t.Errorf("skyTint.G jumped by %v across the 0.999 -> 0.000 wrap, want <= %v", d, continuityEpsilon)
	}
	if d := math.Abs(float64(atWrap.B - beforeWrap.B)); d > continuityEpsilon {
		t.Errorf("skyTint.B jumped by %v across the 0.999 -> 0.000 wrap, want <= %v", d, continuityEpsilon)
	}
}

// Sampling exactly at a keyframe's hour must return that keyframe's own
// values, not an interpolated blend with a neighbour.
func TestTimeOfDay_GetLightingState_KeyframesRoundTrip(t *testing.T) {
	for _, kf := range keyframes {
		t.Run("", func(t *testing.T) {
			tod := newTimeOfDayAt(kf.hour)
			sky, tint := tod.GetLightingState()

			if sky != kf.sky {
				t.Errorf("hour %v: sky = %+v, want %+v", kf.hour, sky, kf.sky)
			}
			if tint != kf.skyTint {
				t.Errorf("hour %v: skyTint = %+v, want %+v", kf.hour, tint, kf.skyTint)
			}
		})
	}
}

// Noon must be brighter than midnight on every skyTint channel, and midnight
// must be blue-dominant.
func TestTimeOfDay_GetLightingState_NoonBrighterThanMidnight(t *testing.T) {
	_, midnight := newTimeOfDayAt(0.0).GetLightingState()
	_, noon := newTimeOfDayAt(0.5).GetLightingState()

	if noon.R <= midnight.R {
		t.Errorf("noon.R = %v, midnight.R = %v, want noon brighter", noon.R, midnight.R)
	}
	if noon.G <= midnight.G {
		t.Errorf("noon.G = %v, midnight.G = %v, want noon brighter", noon.G, midnight.G)
	}
	if noon.B <= midnight.B {
		t.Errorf("noon.B = %v, midnight.B = %v, want noon brighter", noon.B, midnight.B)
	}

	if midnight.B <= midnight.R {
		t.Errorf("midnight = %+v, want blue-dominant (B > R)", midnight)
	}
}

// Sunrise and sunset are the warm ends of the cycle: red must exceed blue.
func TestTimeOfDay_GetLightingState_SunriseAndSunsetAreWarm(t *testing.T) {
	tests := []struct {
		name string
		hour float32
	}{
		{"sunrise", 0.27},
		{"sunset", 0.76},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, tint := newTimeOfDayAt(tt.hour).GetLightingState()

			if tint.R <= tint.B {
				t.Errorf("%s skyTint = %+v, want warm (R > B)", tt.name, tint)
			}
		})
	}
}

// lerpRGBA rounds rather than truncates, and clamps out-of-range t instead of
// letting a channel wrap.
func TestLerpRGBA(t *testing.T) {
	tests := []struct {
		name string
		a, b core.RGBA
		t    float32
		want core.RGBA
	}{
		{
			name: "t=0 returns a",
			a:    core.RGBA{R: 10, G: 20, B: 30, A: 255},
			b:    core.RGBA{R: 200, G: 200, B: 200, A: 255},
			t:    0,
			want: core.RGBA{R: 10, G: 20, B: 30, A: 255},
		},
		{
			name: "t=1 returns b",
			a:    core.RGBA{R: 10, G: 20, B: 30, A: 255},
			b:    core.RGBA{R: 200, G: 200, B: 200, A: 255},
			t:    1,
			want: core.RGBA{R: 200, G: 200, B: 200, A: 255},
		},
		{
			name: "rounds instead of truncating",
			a:    core.RGBA{R: 0, G: 0, B: 0, A: 0},
			b:    core.RGBA{R: 1, G: 1, B: 1, A: 1},
			t:    0.6,
			// 0.6 truncates to 0 but rounds to 1.
			want: core.RGBA{R: 1, G: 1, B: 1, A: 1},
		},
		{
			name: "t above 1 clamps instead of wrapping",
			a:    core.RGBA{R: 0, G: 0, B: 0, A: 0},
			b:    core.RGBA{R: 200, G: 200, B: 200, A: 200},
			t:    2,
			want: core.RGBA{R: 200, G: 200, B: 200, A: 200},
		},
		{
			name: "t below 0 clamps instead of wrapping",
			a:    core.RGBA{R: 50, G: 50, B: 50, A: 50},
			b:    core.RGBA{R: 200, G: 200, B: 200, A: 200},
			t:    -1,
			want: core.RGBA{R: 50, G: 50, B: 50, A: 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lerpRGBA(tt.a, tt.b, tt.t); got != tt.want {
				t.Errorf("lerpRGBA(%+v, %+v, %v) = %+v, want %+v", tt.a, tt.b, tt.t, got, tt.want)
			}
		})
	}
}
