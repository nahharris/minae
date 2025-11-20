package player

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
)

// Player represents the first-person character in the game.
// It handles camera movement and input processing.
type Player struct {
	Camera      rl.Camera3D
	MovementSpeed float32
	MouseSensitivity float32
}

// NewPlayer creates a new Player instance with default settings.
// position: The starting position of the player.
func NewPlayer(position rl.Vector3) *Player {
	camera := rl.Camera3D{}
	camera.Position = position
	camera.Target = rl.Vector3Add(position, rl.NewVector3(1.0, 0.0, 0.0)) // Looking along +X
	camera.Up = rl.NewVector3(0.0, 1.0, 0.0)
	camera.Fovy = 60.0
	camera.Projection = rl.CameraPerspective

	return &Player{
		Camera:           camera,
		MovementSpeed:    10.0, // Units per second
		MouseSensitivity: 0.003,
	}
}

// Update handles player input and updates the camera position and orientation.
// dt: Delta time since the last frame.
func (p *Player) Update(dt float32) {
	// Mouse input for looking around
	mouseDelta := rl.GetMouseDelta()
	
	// Raylib Camera update mode only works if we use UpdateCamera.
	// However, for custom FPS control, we might want to manipulate target/position directly.
	// For simplicity and "fly mode", we can use rl.UpdateCameraPro or manual calculation.
	// Let's implement manual calculation for clarity and control.
	
	// Rotate the camera based on mouse movement
	p.rotateCamera(-mouseDelta.X*p.MouseSensitivity, -mouseDelta.Y*p.MouseSensitivity)

	// Keyboard input for movement
	var forward = rl.Vector3Subtract(p.Camera.Target, p.Camera.Position)
	forward.Y = 0 // Keep movement horizontal for now (except space/ctrl)
	forward = rl.Vector3Normalize(forward)
	
	var right = rl.Vector3CrossProduct(forward, p.Camera.Up)
	right = rl.Vector3Normalize(right)

	var moveDir = rl.Vector3Zero()

	if rl.IsKeyDown(rl.KeyW) {
		moveDir = rl.Vector3Add(moveDir, forward)
	}
	if rl.IsKeyDown(rl.KeyS) {
		moveDir = rl.Vector3Subtract(moveDir, forward)
	}
	if rl.IsKeyDown(rl.KeyD) {
		moveDir = rl.Vector3Add(moveDir, right)
	}
	if rl.IsKeyDown(rl.KeyA) {
		moveDir = rl.Vector3Subtract(moveDir, right)
	}
	if rl.IsKeyDown(rl.KeySpace) {
		moveDir = rl.Vector3Add(moveDir, rl.NewVector3(0, 1, 0))
	}
	if rl.IsKeyDown(rl.KeyLeftControl) {
		moveDir = rl.Vector3Subtract(moveDir, rl.NewVector3(0, 1, 0))
	}

	if rl.Vector3Length(moveDir) > 0 {
		moveDir = rl.Vector3Normalize(moveDir)
		velocity := rl.Vector3Scale(moveDir, p.MovementSpeed*dt)
		p.Camera.Position = rl.Vector3Add(p.Camera.Position, velocity)
		p.Camera.Target = rl.Vector3Add(p.Camera.Target, velocity)
	}
}

// rotateCamera rotates the camera's view direction.
// yaw: Horizontal rotation (radians).
// pitch: Vertical rotation (radians).
func (p *Player) rotateCamera(yaw, pitch float32) {
	// Vector from position to target
	direction := rl.Vector3Subtract(p.Camera.Target, p.Camera.Position)
	
	// Convert to spherical coordinates
	r := rl.Vector3Length(direction)
	theta := float32(math.Atan2(float64(direction.X), float64(direction.Z)))
	phi := float32(math.Asin(float64(direction.Y / r)))

	// Apply rotation
	theta += yaw // Yaw rotates around Y axis (modifies theta in XZ plane)
	phi += pitch // Pitch rotates up/down

	// Clamp pitch to avoid flipping
	maxPitch := float32(89.0 * math.Pi / 180.0)
	if phi > maxPitch {
		phi = maxPitch
	} else if phi < -maxPitch {
		phi = -maxPitch
	}

	// Convert back to Cartesian
	direction.X = r * float32(math.Sin(float64(theta))) * float32(math.Cos(float64(phi)))
	direction.Y = r * float32(math.Sin(float64(phi)))
	direction.Z = r * float32(math.Cos(float64(theta))) * float32(math.Cos(float64(phi)))

	p.Camera.Target = rl.Vector3Add(p.Camera.Position, direction)
}

