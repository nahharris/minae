package lighting

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/config"
)

// DayState represents the lighting configuration at a specific time of day.
type DayState struct {
	SkyColor     rl.Color
	SunColor     rl.Color
	AmbientColor rl.Color
	SunIntensity float32 // Used to scale lightColor if needed, or passed to shader
	PeakTime     float32
	NextState    *DayState
}

// lerpColor interpolates between two colors
func lerpColor(c1, c2 rl.Color, t float32) rl.Color {
	return rl.Color{
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

// Lerp returns a new DayState interpolated between s and it's next state by t (0.0 to 1.0)
func (s *DayState) Lerp(hour float32) *DayState {
	if s.NextState == nil {
		return s
	}

	t := (hour - s.PeakTime) / (s.NextState.PeakTime - s.PeakTime)
	target := s.NextState
	return &DayState{
		SkyColor:     lerpColor(s.SkyColor, target.SkyColor, t),
		SunColor:     lerpColor(s.SunColor, target.SunColor, t),
		AmbientColor: lerpColor(s.AmbientColor, target.AmbientColor, t),
		SunIntensity: lerpFloat(s.SunIntensity, target.SunIntensity, t),
		PeakTime:     lerpFloat(s.PeakTime, target.PeakTime, t),
		NextState:    s.NextState,
	}
}

var (
	// Key States
	nightState = &DayState{
		SkyColor:     rl.NewColor(10, 10, 30, 255),
		SunColor:     rl.NewColor(20, 20, 40, 255), // Moon light
		AmbientColor: rl.NewColor(10, 10, 20, 255),
		SunIntensity: 0.2,
		PeakTime:     0.8,
		NextState:    nil,
	}
	sunsetState = &DayState{
		SkyColor:     rl.NewColor(255, 100, 50, 255),
		SunColor:     rl.NewColor(255, 150, 100, 255),
		AmbientColor: rl.NewColor(80, 50, 50, 255),
		SunIntensity: 0.6,
		PeakTime:     0.75,
		NextState:    nightState,
	}
	afternoonState = &DayState{
		SkyColor:     rl.NewColor(255, 210, 140, 255), // Soft orange-yellow sky
		SunColor:     rl.NewColor(255, 230, 180, 255), // Pale orange sun
		AmbientColor: rl.NewColor(200, 170, 110, 255), // Warm orange ambient
		SunIntensity: 1.0,
		PeakTime:     0.7,
		NextState:    sunsetState,
	}
	noonState = &DayState{
		SkyColor:     rl.NewColor(135, 206, 235, 255),
		SunColor:     rl.NewColor(255, 255, 255, 255),
		AmbientColor: rl.NewColor(100, 100, 100, 255),
		SunIntensity: 1.0,
		PeakTime:     0.5,
		NextState:    afternoonState,
	}
	morningState = &DayState{
		SkyColor:     rl.NewColor(200, 220, 255, 255), // Soft white-blue morning sky
		SunColor:     rl.NewColor(230, 220, 200, 255), // Gentle, warmer morning sun
		AmbientColor: rl.NewColor(120, 140, 170, 255), // Softer blue ambient for morning
		SunIntensity: 0.7,
		PeakTime:     0.3,
		NextState:    noonState,
	}
	sunriseState = &DayState{
		SkyColor:     rl.NewColor(255, 180, 100, 255),
		SunColor:     rl.NewColor(255, 200, 100, 255),
		AmbientColor: rl.NewColor(80, 60, 60, 255),
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

type Manager struct {
	Time          float32 // Current time in seconds
	CycleDuration float32 // Total duration of a day in seconds
}

func NewManager() *Manager {
	return &Manager{
		Time:          config.Current.DayCycleDuration * 0.25, // Start at 6am (Sunrise approx)
		CycleDuration: config.Current.DayCycleDuration,
	}
}

func (m *Manager) Update(dt float32) {
	m.Time += dt
	for m.Time >= m.CycleDuration {
		m.Time -= m.CycleDuration
	}
}

func (m *Manager) GetState() (skyColor, lightColor, ambientColor rl.Color, lightDir rl.Vector3) {
	// Map time to 0-24 hour scale for easier reasoning
	hour := (m.Time / m.CycleDuration)

	lerpedState := getStateFromTime(hour).Lerp(hour)
	skyColor = lerpedState.SkyColor
	lightColor = lerpedState.SunColor
	ambientColor = lerpedState.AmbientColor
	// Calculate Sun Direction
	// Angle 0 at 6am.
	angle := hour * 2.0 * math.Pi

	sinVal := float32(math.Sin(float64(angle)))
	cosVal := float32(math.Cos(float64(angle)))

	lightDir = rl.Vector3{X: cosVal, Y: sinVal, Z: 0.2} // Slight Z tilt
	lightDir = rl.Vector3Normalize(lightDir)

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
