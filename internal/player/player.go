package player

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// Body dimensions and eye height, per the M13 player controller.
const (
	bodyWidth  float32 = 0.6
	bodyHeight float32 = 1.8
	bodyDepth  float32 = 0.6
	eyeHeight  float32 = 1.62
)

// Player represents the first-person character in the game.
// It handles camera movement and input processing.
//
// Body is the single source of truth for the player's position; Camera is
// derived from it every Update, at Body.Position plus eye height. Nothing
// else is allowed to write Camera.Position directly -- that is what keeps
// the camera from ever drifting away from the body it is supposed to
// represent.
type Player struct {
	State  *world.PlayerState // Reference to world's saveable state.
	Body   physics.Body       // Source of truth for position.
	Camera rl.Camera3D        // Runtime-only; derived from Body each Update.

	// Flying is runtime movement-mode state, not persisted in PlayerState.
	// While true, gravity and collision are off and Space/Ctrl ascend and
	// descend instead of jumping.
	Flying bool

	WalkSpeed        float32
	FlySpeed         float32
	MouseSensitivity float32
	PhysicsConfig    physics.Config

	// Runtime Interaction State
	SelectedBlockIndex int
	TargetBlock        rl.Vector3
	HasTarget          bool
}

// NewPlayer creates a new Player instance wrapping the given state. The body
// is spawned at the state's saved position; the camera is derived from it.
func NewPlayer(state *world.PlayerState) *Player {
	body := physics.Body{
		Position: core.Vec3{X: state.Position[0], Y: state.Position[1], Z: state.Position[2]},
		Size:     core.Vec3{X: bodyWidth, Y: bodyHeight, Z: bodyDepth},
	}

	p := &Player{
		State:            state,
		Body:             body,
		WalkSpeed:        config.Current.WalkSpeed,
		FlySpeed:         config.Current.FlySpeed,
		MouseSensitivity: config.Current.MouseSens,
		PhysicsConfig:    physics.DefaultConfig(),
	}

	camPos, camTarget := syncCameraToBody(p.Body, core.Vec3{X: 1.0, Y: 0.0, Z: 0.0}) // Looking along +X
	p.Camera.Position = toRlVector3(camPos)
	p.Camera.Target = toRlVector3(camTarget)
	p.Camera.Up = rl.NewVector3(0.0, 1.0, 0.0)
	p.Camera.Fovy = config.Current.FOV
	p.Camera.Projection = rl.CameraPerspective

	return p
}

// deriveCameraPosition returns the eye position for a body whose feet are at
// bodyPos. This is the only place camera position is computed from body
// position, so it is the one path every caller must go through -- there is
// no other way to move the camera.
func deriveCameraPosition(bodyPos core.Vec3) core.Vec3 {
	return core.Vec3{X: bodyPos.X, Y: bodyPos.Y + eyeHeight, Z: bodyPos.Z}
}

// syncCameraToBody derives the eye position and look-at target for body,
// preserving lookDir as the direction the camera faces. It is the single
// computation Update relies on to keep the camera glued to the body every
// frame; pulling it out as a plain function of values (no raylib, no
// receiver mutation) is what makes that invariant directly testable.
func syncCameraToBody(body physics.Body, lookDir core.Vec3) (position, target core.Vec3) {
	position = deriveCameraPosition(body.Position)
	target = core.Vec3{X: position.X + lookDir.X, Y: position.Y + lookDir.Y, Z: position.Z + lookDir.Z}
	return position, target
}

// SyncFromState updates the body and camera position from the saveable
// state. Call this when loading a game.
func (p *Player) SyncFromState() {
	p.Body.Position = core.Vec3{X: p.State.Position[0], Y: p.State.Position[1], Z: p.State.Position[2]}
	p.Camera.Position = toRlVector3(deriveCameraPosition(p.Body.Position))
}

// SyncToState updates the saveable state from the body position. Call this
// before saving.
func (p *Player) SyncToState() {
	p.State.Position = [3]float32{p.Body.Position.X, p.Body.Position.Y, p.Body.Position.Z}
}

// ToggleFlight flips the player's movement mode between walking and flying.
// The mode is runtime-only: it is never written to PlayerState.
func (p *Player) ToggleFlight() {
	p.Flying = !p.Flying
}

// MovementInput is the raw per-tick input the controller needs, independent
// of any input backend. Keeping it a plain struct (rather than reading
// rl.IsKeyDown inline) is what lets BuildIntent be tested without raylib.
type MovementInput struct {
	Forward, Back, Left, Right bool
	Jump                       bool
	// Ascend and Descend only apply in fly mode.
	Ascend, Descend bool
}

// BuildIntent constructs the physics.Intent for one tick from raw input and
// the current horizontal look direction. It is pure: no raylib, no globals,
// no I/O, so the mapping from input to intent is directly testable.
//
// lookDir need not be normalized or horizontal; only its X and Z components
// are used. Diagonal movement (e.g. forward+right, or forward+ascend while
// flying) is normalized so the resulting speed never exceeds walkSpeed or
// flySpeed.
func BuildIntent(in MovementInput, lookDir core.Vec3, flying bool, walkSpeed, flySpeed float32) physics.Intent {
	forward := core.Vec3{X: lookDir.X, Z: lookDir.Z}.Normalize()
	right := core.Vec3{X: -forward.Z, Z: forward.X}

	var horizontal core.Vec3
	if in.Forward {
		horizontal.X += forward.X
		horizontal.Z += forward.Z
	}
	if in.Back {
		horizontal.X -= forward.X
		horizontal.Z -= forward.Z
	}
	if in.Right {
		horizontal.X += right.X
		horizontal.Z += right.Z
	}
	if in.Left {
		horizontal.X -= right.X
		horizontal.Z -= right.Z
	}

	if !flying {
		if horizontal.Length() > 0 {
			horizontal = horizontal.Normalize()
		}
		return physics.Intent{
			Move: core.Vec3{X: horizontal.X * walkSpeed, Z: horizontal.Z * walkSpeed},
			Jump: in.Jump,
		}
	}

	full := core.Vec3{X: horizontal.X, Z: horizontal.Z}
	if in.Ascend {
		full.Y++
	}
	if in.Descend {
		full.Y--
	}
	if full.Length() > 0 {
		full = full.Normalize()
	}
	return physics.Intent{
		Fly:  true,
		Move: core.Vec3{X: full.X * flySpeed, Y: full.Y * flySpeed, Z: full.Z * flySpeed},
	}
}

// Update handles player input and updates the body and camera.
// dt: Delta time since the last frame.
// grid supplies the collision geometry the physics step resolves against.
func (p *Player) Update(dt float32, grid physics.Grid) {
	// Mouse input for looking around.
	mouseDelta := rl.GetMouseDelta()
	p.rotateCamera(-mouseDelta.X*p.MouseSensitivity, -mouseDelta.Y*p.MouseSensitivity)

	if rl.IsKeyPressed(rl.KeyF3) {
		p.ToggleFlight()
	}

	lookDir := toCoreVector3(rl.Vector3Subtract(p.Camera.Target, p.Camera.Position))

	in := MovementInput{
		Forward: rl.IsKeyDown(rl.KeyW),
		Back:    rl.IsKeyDown(rl.KeyS),
		Right:   rl.IsKeyDown(rl.KeyD),
		Left:    rl.IsKeyDown(rl.KeyA),
		Jump:    rl.IsKeyDown(rl.KeySpace),
		Ascend:  rl.IsKeyDown(rl.KeySpace),
		Descend: rl.IsKeyDown(rl.KeyLeftControl),
	}

	intent := BuildIntent(in, lookDir, p.Flying, p.WalkSpeed, p.FlySpeed)
	physics.Step(&p.Body, grid, p.PhysicsConfig, intent, dt)

	// The camera is derived from the body every frame through this one path;
	// nothing else in this method (or anywhere else) assigns Camera.Position.
	camPos, camTarget := syncCameraToBody(p.Body, lookDir)
	p.Camera.Position = toRlVector3(camPos)
	p.Camera.Target = toRlVector3(camTarget)

	p.SyncToState()

	// Inventory Selection
	mouseWheel := rl.GetMouseWheelMove()
	if mouseWheel != 0 {
		p.SelectedBlockIndex -= int(mouseWheel)
		if p.SelectedBlockIndex < 0 {
			p.SelectedBlockIndex = len(p.State.Inventory) - 1
		} else if p.SelectedBlockIndex >= len(p.State.Inventory) {
			p.SelectedBlockIndex = 0
		}
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

// toRlVector3 converts a simulation vector into the raylib vector the camera
// uses. This and toCoreVector3 are the only places Player crosses that
// boundary.
func toRlVector3(v core.Vec3) rl.Vector3 {
	return rl.Vector3{X: v.X, Y: v.Y, Z: v.Z}
}

// toCoreVector3 converts a raylib vector into the simulation vector used by
// BuildIntent and the physics step.
func toCoreVector3(v rl.Vector3) core.Vec3 {
	return core.Vec3{X: v.X, Y: v.Y, Z: v.Z}
}
