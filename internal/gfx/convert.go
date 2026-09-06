package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/core"
)

// ToColor converts a simulation colour into the raylib colour used by the
// renderer. This is the only place colours cross that boundary.
func ToColor(c core.RGBA) rl.Color {
	return rl.Color{R: c.R, G: c.G, B: c.B, A: c.A}
}

// ToVector3 converts a simulation vector into the raylib vector used by the
// renderer. This is the only place vectors cross that boundary.
func ToVector3(v core.Vec3) rl.Vector3 {
	return rl.Vector3{X: v.X, Y: v.Y, Z: v.Z}
}

// FromVector3 converts a raylib vector into the simulation vector, for values
// produced by the renderer and handed back to the simulation.
func FromVector3(v rl.Vector3) core.Vec3 {
	return core.Vec3{X: v.X, Y: v.Y, Z: v.Z}
}
