package player

import (
	"testing"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/world"
)

// abs32 returns the absolute value of v, for epsilon comparisons against
// float32 arithmetic that is not expected to be bit-exact.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// Update itself calls rl.GetMouseDelta, rl.IsKeyDown and friends, which need
// a live raylib window and so cannot run in this test binary. Everything
// Update does that does not touch raylib -- deriving the camera from the
// body, building an Intent from input, and toggling flight -- is pulled out
// into the plain functions and methods exercised below.

// TestDeriveCameraPosition_IsBodyPositionPlusEyeHeight pins the one
// computation the "camera follows the body, always" invariant rests on: eye
// height above the body's feet, nothing else.
func TestDeriveCameraPosition_IsBodyPositionPlusEyeHeight(t *testing.T) {
	t.Parallel()

	bodyPos := core.Vec3{X: 3, Y: 10, Z: -5}
	want := core.Vec3{X: 3, Y: 10 + eyeHeight, Z: -5}

	if got := deriveCameraPosition(bodyPos); got != want {
		t.Fatalf("deriveCameraPosition(%+v) = %+v, want %+v", bodyPos, got, want)
	}
}

// TestSyncCameraToBody_PositionAlwaysMatchesBody exercises the exact
// function Update calls every frame to keep the camera glued to the body.
// After any body position, the derived camera position must be exactly
// body.Position + eye height, and the target must sit lookDir away from that
// position -- never anywhere else.
func TestSyncCameraToBody_PositionAlwaysMatchesBody(t *testing.T) {
	t.Parallel()

	positions := []core.Vec3{
		{X: 0, Y: 0, Z: 0},
		{X: 12.5, Y: 40, Z: -8.25},
		{X: -100, Y: 5, Z: 100},
	}
	lookDir := core.Vec3{X: 0.3, Y: -0.1, Z: 0.9}

	for _, pos := range positions {
		body := physics.Body{Position: pos}
		gotPos, gotTarget := syncCameraToBody(body, lookDir)

		wantPos := deriveCameraPosition(pos)
		if gotPos != wantPos {
			t.Errorf("syncCameraToBody(%+v).position = %+v, want %+v", pos, gotPos, wantPos)
		}

		gotDir := core.Vec3{X: gotTarget.X - gotPos.X, Y: gotTarget.Y - gotPos.Y, Z: gotTarget.Z - gotPos.Z}
		const eps = 1e-3
		if abs32(gotDir.X-lookDir.X) > eps || abs32(gotDir.Y-lookDir.Y) > eps || abs32(gotDir.Z-lookDir.Z) > eps {
			t.Errorf("syncCameraToBody(%+v): target-position = %+v, want lookDir %+v", pos, gotDir, lookDir)
		}
	}
}

// TestNewPlayer_CameraStartsAtBodyPlusEyeHeight checks the invariant holds
// from construction, not just after an Update.
func TestNewPlayer_CameraStartsAtBodyPlusEyeHeight(t *testing.T) {
	t.Parallel()

	state := &world.PlayerState{Position: [3]float32{1, 20, 2}}
	p := NewPlayer(state)

	want := deriveCameraPosition(p.Body.Position)
	got := toCoreVector3(p.Camera.Position)
	if got != want {
		t.Fatalf("Camera.Position = %+v, want %+v (Body.Position + eye height)", got, want)
	}
}

// TestSyncFromState_MovesBodyAndCameraTogether checks that loading a saved
// position updates both the body and the derived camera, so the two never
// disagree even right after a load.
func TestSyncFromState_MovesBodyAndCameraTogether(t *testing.T) {
	t.Parallel()

	state := &world.PlayerState{Position: [3]float32{0, 0, 0}}
	p := NewPlayer(state)

	state.Position = [3]float32{50, 12, -3}
	p.SyncFromState()

	wantBody := core.Vec3{X: 50, Y: 12, Z: -3}
	if p.Body.Position != wantBody {
		t.Fatalf("Body.Position = %+v, want %+v", p.Body.Position, wantBody)
	}
	wantCam := deriveCameraPosition(wantBody)
	if got := toCoreVector3(p.Camera.Position); got != wantCam {
		t.Fatalf("Camera.Position = %+v, want %+v", got, wantCam)
	}
}

// TestSyncToState_SavesBodyPositionNotCamera checks that the persisted
// position is the body's feet, not the eye -- the state should not encode
// eye height, or reloading would keep adding it.
func TestSyncToState_SavesBodyPositionNotCamera(t *testing.T) {
	t.Parallel()

	state := &world.PlayerState{Position: [3]float32{0, 0, 0}}
	p := NewPlayer(state)
	p.Body.Position = core.Vec3{X: 7, Y: 8, Z: 9}

	p.SyncToState()

	want := [3]float32{7, 8, 9}
	if state.Position != want {
		t.Fatalf("State.Position = %+v, want %+v (feet, not eye)", state.Position, want)
	}
}

// TestToggleFlight_FlipsAndReturns checks criterion 3's "toggling twice
// returns the original behaviour" at the mode level.
func TestToggleFlight_FlipsAndReturns(t *testing.T) {
	t.Parallel()

	p := &Player{}
	if p.Flying {
		t.Fatalf("new Player starts Flying = true, want false (walking is the default)")
	}

	p.ToggleFlight()
	if !p.Flying {
		t.Fatalf("Flying = false after one ToggleFlight, want true")
	}

	p.ToggleFlight()
	if p.Flying {
		t.Fatalf("Flying = true after two ToggleFlight calls, want false")
	}
}

// TestBuildIntent_WalkForward checks the basic mapping: pressing forward with
// the camera looking down +Z produces a walking Intent moving along +Z at
// exactly walkSpeed, with Fly unset.
func TestBuildIntent_WalkForward(t *testing.T) {
	t.Parallel()

	in := MovementInput{Forward: true}
	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	got := BuildIntent(in, lookDir, false, 4.5, 10)

	want := physics.Intent{Move: core.Vec3{X: 0, Z: 4.5}}
	if got != want {
		t.Fatalf("BuildIntent = %+v, want %+v", got, want)
	}
}

// TestBuildIntent_WalkIgnoresLookPitch checks that a look direction with a
// vertical component (looking up or down) does not add a vertical component
// to walking movement or change its horizontal speed -- walking is always
// horizontal regardless of where the camera is pointed.
func TestBuildIntent_WalkIgnoresLookPitch(t *testing.T) {
	t.Parallel()

	in := MovementInput{Forward: true}
	lookDir := core.Vec3{X: 0, Y: 5, Z: 1} // looking steeply upward, but still toward +Z

	got := BuildIntent(in, lookDir, false, 4.5, 10)

	if got.Move.Y != 0 {
		t.Fatalf("Move.Y = %v, want 0 (walking never moves vertically)", got.Move.Y)
	}
	const eps = 1e-4
	speed := got.Move.Length()
	if speed < 4.5-eps || speed > 4.5+eps {
		t.Fatalf("|Move| = %v, want %v", speed, 4.5)
	}
}

// TestBuildIntent_WalkDiagonalDoesNotExceedSpeed checks that pressing two
// movement keys at once is normalized rather than summed, so diagonal
// movement is not faster than straight movement.
func TestBuildIntent_WalkDiagonalDoesNotExceedSpeed(t *testing.T) {
	t.Parallel()

	in := MovementInput{Forward: true, Right: true}
	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	got := BuildIntent(in, lookDir, false, 4.5, 10)

	const eps = 1e-4
	speed := got.Move.Length()
	if speed < 4.5-eps || speed > 4.5+eps {
		t.Fatalf("|Move| = %v after pressing two directions, want exactly walkSpeed %v", speed, 4.5)
	}
}

// TestBuildIntent_WalkOppositeKeysCancel checks that pressing both keys for
// an axis (e.g. forward and back) cancels out to no movement, rather than
// leaving a residual direction from the order the flags happen to be
// checked in.
func TestBuildIntent_WalkOppositeKeysCancel(t *testing.T) {
	t.Parallel()

	in := MovementInput{Forward: true, Back: true, Left: true, Right: true}
	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	got := BuildIntent(in, lookDir, false, 4.5, 10)

	want := physics.Intent{Move: core.Vec3{}}
	if got != want {
		t.Fatalf("BuildIntent with all opposite pairs held = %+v, want zero movement %+v", got, want)
	}
}

// TestBuildIntent_WalkJumpPassesThrough checks that Jump is forwarded
// verbatim while walking, since only physics.Step knows whether the body is
// actually grounded.
func TestBuildIntent_WalkJumpPassesThrough(t *testing.T) {
	t.Parallel()

	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	got := BuildIntent(MovementInput{Jump: true}, lookDir, false, 4.5, 10)
	if !got.Jump {
		t.Fatalf("Jump = false, want true")
	}
	if got.Fly {
		t.Fatalf("Fly = true while walking, want false")
	}

	got = BuildIntent(MovementInput{Jump: false}, lookDir, false, 4.5, 10)
	if got.Jump {
		t.Fatalf("Jump = true, want false")
	}
}

// TestBuildIntent_NoInputIsZeroMove checks that no keys held produces no
// movement and no jump in either mode.
func TestBuildIntent_NoInputIsZeroMove(t *testing.T) {
	t.Parallel()

	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	for _, flying := range []bool{false, true} {
		got := BuildIntent(MovementInput{}, lookDir, flying, 4.5, 10)
		if got.Move != (core.Vec3{}) {
			t.Errorf("flying=%v: Move = %+v, want zero", flying, got.Move)
		}
		if got.Jump {
			t.Errorf("flying=%v: Jump = true, want false", flying)
		}
		if got.Fly != flying {
			t.Errorf("flying=%v: Fly = %v, want %v", flying, got.Fly, flying)
		}
	}
}

// TestBuildIntent_FlyAscendDescend checks that Space and Ctrl become pure
// vertical movement while flying, using Move.Y as physics.Step's doc comment
// says fly mode requires.
func TestBuildIntent_FlyAscendDescend(t *testing.T) {
	t.Parallel()

	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	up := BuildIntent(MovementInput{Ascend: true}, lookDir, true, 4.5, 10)
	want := physics.Intent{Fly: true, Move: core.Vec3{Y: 10}}
	if up != want {
		t.Fatalf("ascend Intent = %+v, want %+v", up, want)
	}

	down := BuildIntent(MovementInput{Descend: true}, lookDir, true, 4.5, 10)
	want = physics.Intent{Fly: true, Move: core.Vec3{Y: -10}}
	if down != want {
		t.Fatalf("descend Intent = %+v, want %+v", down, want)
	}
}

// TestBuildIntent_FlyAscendAndDescendCancel checks Space+Ctrl held together
// produces no vertical movement.
func TestBuildIntent_FlyAscendAndDescendCancel(t *testing.T) {
	t.Parallel()

	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}
	got := BuildIntent(MovementInput{Ascend: true, Descend: true}, lookDir, true, 4.5, 10)

	if got.Move.Y != 0 {
		t.Fatalf("Move.Y = %v with Ascend and Descend both held, want 0", got.Move.Y)
	}
}

// TestBuildIntent_FlyDiagonalDoesNotExceedSpeed checks that combining
// horizontal and vertical flight input is normalized to flySpeed rather than
// summed, mirroring the walking diagonal case.
func TestBuildIntent_FlyDiagonalDoesNotExceedSpeed(t *testing.T) {
	t.Parallel()

	in := MovementInput{Forward: true, Ascend: true}
	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}

	got := BuildIntent(in, lookDir, true, 4.5, 10)

	const eps = 1e-3
	speed := got.Move.Length()
	if speed < 10-eps || speed > 10+eps {
		t.Fatalf("|Move| = %v, want exactly flySpeed %v", speed, 10.0)
	}
	if !got.Fly {
		t.Fatalf("Fly = false while flying, want true")
	}
}

// TestBuildIntent_FlyIgnoresJump checks that holding Jump while flying does
// not leak into the fly Intent -- ascending uses Ascend, not Jump.
func TestBuildIntent_FlyIgnoresJump(t *testing.T) {
	t.Parallel()

	lookDir := core.Vec3{X: 0, Y: 0, Z: 1}
	got := BuildIntent(MovementInput{Jump: true}, lookDir, true, 4.5, 10)

	if got.Jump {
		t.Fatalf("Jump = true in a fly Intent, want false (Step ignores Jump in fly mode, but this keeps Intent's meaning consistent)")
	}
	if got.Move != (core.Vec3{}) {
		t.Fatalf("Move = %+v, want zero (Jump alone is not Ascend)", got.Move)
	}
}

// TestBuildIntent_ZeroLookDirDoesNotProduceNaN checks the degenerate case
// where lookDir has no horizontal component (looking straight up or down):
// Vec3.Normalize returns the zero vector rather than NaN, so forward/right
// become zero and movement input produces no motion instead of a poisoned
// value silently propagating into the physics step.
func TestBuildIntent_ZeroLookDirDoesNotProduceNaN(t *testing.T) {
	t.Parallel()

	in := MovementInput{Forward: true, Right: true}
	lookDir := core.Vec3{X: 0, Y: 1, Z: 0} // straight up: no horizontal component

	got := BuildIntent(in, lookDir, false, 4.5, 10)

	if got.Move != (core.Vec3{}) {
		t.Fatalf("Move = %+v, want zero when look direction has no horizontal component", got.Move)
	}
}
