package world

import (
	"math"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/platform/config"
)

// DayState represents the lighting configuration at a specific time of day.
type DayState struct {
	SkyColor     core.RGBA
	SunColor     core.RGBA
	AmbientColor core.RGBA
	SunIntensity float32
	PeakTime     float32
	NextState    *DayState
}

// lerpColor interpolates between two colors
func lerpColor(c1, c2 core.RGBA, t float32) core.RGBA {
	return core.RGBA{
		R: uint8(lerpFloat(float32(c1.R), float32(c2.R), t)),
		G: uint8(lerpFloat(float32(c1.G), float32(c2.G), t)),
		B: uint8(lerpFloat(float32(c1.B), float32(c2.B), t)),
		A: uint8(lerpFloat(float32(c1.A), float32(c2.A), t)),
	}
}

// lerpFloat interpolates between two floats
func lerpFloat(f1, f2 float32, t float32) float32 {
	return f1 + (f2-f1)*t
}

// LerpColors interpolates the colors between this state and the next state.
func (s *DayState) LerpColors(hour float32) (sky, sun, ambient core.RGBA, intensity float32) {
	if s.NextState == nil {
		return s.SkyColor, s.SunColor, s.AmbientColor, s.SunIntensity
	}

	next := s.NextState
	den := next.PeakTime - s.PeakTime
	if den == 0 {
		return next.SkyColor, next.SunColor, next.AmbientColor, next.SunIntensity
	}

	t := (hour - s.PeakTime) / den
	return lerpColor(s.SkyColor, next.SkyColor, t),
		lerpColor(s.SunColor, next.SunColor, t),
		lerpColor(s.AmbientColor, next.AmbientColor, t),
		lerpFloat(s.SunIntensity, next.SunIntensity, t)
}

var (
	// Key States
	nightState = &DayState{
		SkyColor:     core.RGBA{R: 10, G: 10, B: 30, A: 255},
		SunColor:     core.RGBA{R: 20, G: 20, B: 40, A: 255}, // Moon light
		AmbientColor: core.RGBA{R: 10, G: 10, B: 20, A: 255},
		SunIntensity: 0.2,
		PeakTime:     0.8,
		NextState:    nil,
	}
	sunsetState = &DayState{
		SkyColor:     core.RGBA{R: 255, G: 100, B: 50, A: 255},
		SunColor:     core.RGBA{R: 255, G: 150, B: 100, A: 255},
		AmbientColor: core.RGBA{R: 80, G: 50, B: 50, A: 255},
		SunIntensity: 0.6,
		PeakTime:     0.75,
		NextState:    nightState,
	}
	afternoonState = &DayState{
		SkyColor:     core.RGBA{R: 255, G: 210, B: 140, A: 255}, // Soft orange-yellow sky
		SunColor:     core.RGBA{R: 255, G: 230, B: 180, A: 255}, // Pale orange sun
		AmbientColor: core.RGBA{R: 200, G: 170, B: 110, A: 255}, // Warm orange ambient
		SunIntensity: 1.0,
		PeakTime:     0.7,
		NextState:    sunsetState,
	}
	noonState = &DayState{
		SkyColor:     core.RGBA{R: 135, G: 206, B: 235, A: 255},
		SunColor:     core.RGBA{R: 255, G: 255, B: 255, A: 255},
		AmbientColor: core.RGBA{R: 100, G: 100, B: 100, A: 255},
		SunIntensity: 1.0,
		PeakTime:     0.5,
		NextState:    afternoonState,
	}
	morningState = &DayState{
		SkyColor:     core.RGBA{R: 200, G: 220, B: 255, A: 255}, // Soft white-blue morning sky
		SunColor:     core.RGBA{R: 230, G: 220, B: 200, A: 255}, // Gentle, warmer morning sun
		AmbientColor: core.RGBA{R: 120, G: 140, B: 170, A: 255}, // Softer blue ambient for morning
		SunIntensity: 0.7,
		PeakTime:     0.3,
		NextState:    noonState,
	}
	sunriseState = &DayState{
		SkyColor:     core.RGBA{R: 255, G: 180, B: 100, A: 255},
		SunColor:     core.RGBA{R: 255, G: 200, B: 100, A: 255},
		AmbientColor: core.RGBA{R: 80, G: 60, B: 60, A: 255},
		SunIntensity: 0.6,
		PeakTime:     0.25,
		NextState:    morningState,
	}
	dawnState = &DayState{
		SkyColor:     nightState.SkyColor,
		SunColor:     nightState.SunColor,
		AmbientColor: nightState.AmbientColor,
		SunIntensity: nightState.SunIntensity,
		PeakTime:     0.2,
		NextState:    sunriseState,
	}
)

var dayStates = []*DayState{
	dawnState,
	sunriseState,
	morningState,
	noonState,
	afternoonState,
	sunsetState,
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

// GetLightingState returns the current lighting configuration based on time.
func (t *TimeOfDay) GetLightingState() (skyColor, lightColor, ambientColor core.RGBA, lightDir core.Vec3) {
	// Map time to 0-24 hour scale for easier reasoning (0.0 - 1.0)
	hour := (t.Time / t.CycleDuration)

	state := getStateFromTime(hour)
	skyColor, lightColor, ambientColor, _ = state.LerpColors(hour)

	// Calculate Sun Direction
	// Angle 0 at 6am.
	angle := hour * 2.0 * math.Pi

	sinVal := float32(math.Sin(float64(angle)))
	cosVal := float32(math.Cos(float64(angle)))

	lightDir = core.Vec3{X: cosVal, Y: sinVal, Z: 0.2} // Slight Z tilt
	lightDir = lightDir.Normalize()

	return
}

func getStateFromTime(hour float32) *DayState {
	for _, state := range dayStates {
		if hour >= state.PeakTime && (state.NextState == nil || hour < state.NextState.PeakTime) {
			return state
		}
	}
	return nightState
}
