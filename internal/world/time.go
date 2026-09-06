package world

import (
	"math"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/platform/config"
)

// keyframe is a single point in the day/night cycle: an hour in [0, 1)
// paired with the sky background colour and the skylight tint that apply at
// that instant.
type keyframe struct {
	hour    float32
	sky     core.RGBA
	skyTint core.RGB
}

// keyframes are the day/night cycle's fixed points, ordered by hour
// ascending. The ring is circular: sampleKeyframes wraps from the last entry
// back to the first, treating keyframes[0] as if it also sat at hour 1.0, so
// every hour in [0, 1) has a defined, continuous value.
var keyframes = []keyframe{
	{hour: 0.00, sky: core.RGBA{R: 10, G: 10, B: 30, A: 255}, skyTint: core.RGB{R: 0.16, G: 0.18, B: 0.32}},
	{hour: 0.22, sky: core.RGBA{R: 60, G: 50, B: 80, A: 255}, skyTint: core.RGB{R: 0.45, G: 0.35, B: 0.42}},
	{hour: 0.27, sky: core.RGBA{R: 255, G: 150, B: 90, A: 255}, skyTint: core.RGB{R: 1.00, G: 0.60, B: 0.35}},
	{hour: 0.35, sky: core.RGBA{R: 200, G: 220, B: 255, A: 255}, skyTint: core.RGB{R: 1.00, G: 0.90, B: 0.75}},
	{hour: 0.50, sky: core.RGBA{R: 135, G: 206, B: 235, A: 255}, skyTint: core.RGB{R: 1.00, G: 0.98, B: 0.92}},
	{hour: 0.68, sky: core.RGBA{R: 255, G: 225, B: 170, A: 255}, skyTint: core.RGB{R: 1.00, G: 0.92, B: 0.78}},
	{hour: 0.76, sky: core.RGBA{R: 255, G: 110, B: 55, A: 255}, skyTint: core.RGB{R: 1.00, G: 0.55, B: 0.30}},
	{hour: 0.82, sky: core.RGBA{R: 50, G: 40, B: 70, A: 255}, skyTint: core.RGB{R: 0.40, G: 0.30, B: 0.40}},
}

// TimeOfDay manages the game day cycle.
type TimeOfDay struct {
	Time          float32 // Current time in seconds
	CycleDuration float32 // Total duration of a day in seconds
}

// NewTimeOfDay creates a new time manager.
func NewTimeOfDay() *TimeOfDay {
	return &TimeOfDay{
		Time:          config.Current.DayCycleDuration * 0.25, // Start at 6am (Sunrise approx)
		CycleDuration: config.Current.DayCycleDuration,
	}
}

// Update advances the time.
func (t *TimeOfDay) Update(dt float32) {
	t.Time += dt
	for t.Time >= t.CycleDuration {
		t.Time -= t.CycleDuration
	}
}

// GetLightingState returns the background sky colour and the tint applied to
// baked skylight for the current time of day.
func (t *TimeOfDay) GetLightingState() (sky core.RGBA, skyTint core.RGB) {
	hour := normalizeHour(t.Time / t.CycleDuration)
	return sampleKeyframes(hour)
}

// normalizeHour wraps hour into [0, 1), regardless of any drift or negative
// input, so sampleKeyframes never sees a value outside the range its ring
// covers.
func normalizeHour(hour float32) float32 {
	h := float32(math.Mod(float64(hour), 1.0))
	if h < 0 {
		h += 1.0
	}
	return h
}

// sampleKeyframes interpolates the sky colour and skylight tint for hour,
// which must be in [0, 1). The keyframe ring is circular: after the last
// keyframe it interpolates back to the first, treated as hour 1.0, so there
// is no discontinuity anywhere in the cycle, including across the wrap from
// the last keyframe back to midnight.
func sampleKeyframes(hour float32) (sky core.RGBA, skyTint core.RGB) {
	n := len(keyframes)
	for i := range n {
		cur := keyframes[i]
		next := keyframes[(i+1)%n]
		nextHour := next.hour
		if i == n-1 {
			nextHour = 1.0
		}

		if hour < cur.hour || hour >= nextHour {
			continue
		}

		span := nextHour - cur.hour
		var t float32
		if span > 0 {
			t = (hour - cur.hour) / span
		}

		return lerpRGBA(cur.sky, next.sky, t), lerpRGB(cur.skyTint, next.skyTint, t)
	}

	// Unreachable for hour in [0, 1) since keyframes[0].hour == 0, but return
	// the first keyframe's values rather than a zero value if it ever is.
	return keyframes[0].sky, keyframes[0].skyTint
}

// lerpRGB interpolates two linear-space colours by t. t is clamped to [0, 1]
// defensively before use.
func lerpRGB(a, b core.RGB, t float32) core.RGB {
	t = clamp01(t)
	return core.RGB{
		R: lerpFloat(a.R, b.R, t),
		G: lerpFloat(a.G, b.G, t),
		B: lerpFloat(a.B, b.B, t),
	}
}

// lerpRGBA interpolates two 8-bit colours by t. The interpolation happens in
// float space and each channel is rounded, not truncated, when converting
// back to uint8, and clamped to [0, 255]. t is clamped to [0, 1] defensively
// before use.
func lerpRGBA(a, b core.RGBA, t float32) core.RGBA {
	t = clamp01(t)
	return core.RGBA{
		R: lerpChannel(a.R, b.R, t),
		G: lerpChannel(a.G, b.G, t),
		B: lerpChannel(a.B, b.B, t),
		A: lerpChannel(a.A, b.A, t),
	}
}

// lerpChannel interpolates one 8-bit channel in float space and rounds the
// result back to uint8, clamping to [0, 255].
func lerpChannel(c1, c2 uint8, t float32) uint8 {
	return clampByte(lerpFloat(float32(c1), float32(c2), t))
}

// clampByte rounds v to the nearest integer and clamps it to [0, 255].
func clampByte(v float32) uint8 {
	rounded := math.Round(float64(v))
	switch {
	case rounded < 0:
		return 0
	case rounded > 255:
		return 255
	default:
		return uint8(rounded)
	}
}

// clamp01 clamps t to [0, 1].
func clamp01(t float32) float32 {
	switch {
	case t < 0:
		return 0
	case t > 1:
		return 1
	default:
		return t
	}
}

// lerpFloat linearly interpolates between f1 and f2 by t.
func lerpFloat(f1, f2, t float32) float32 {
	return f1 + (f2-f1)*t
}
