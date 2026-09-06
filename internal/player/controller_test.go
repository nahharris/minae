package player

import (
	"testing"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// These tests re-assert the M12 physics properties through the M13
// controller wiring -- BuildIntent feeding physics.Step exactly as Update
// does -- rather than through physics.Step directly, so a wiring mistake
// (e.g. swapped axes, a dropped speed, Flying not actually reaching Step)
// would show up here even though internal/physics's own tests are untouched
// and still pass.

// fakeGrid is a minimal physics.Grid built from a predicate, so these tests
// can compose obstacles without touching internal/world.
type fakeGrid struct {
	solid func(x, y, z int) bool
}

func (g fakeGrid) CollisionBoxes(dst []physics.AABB, x, y, z int) []physics.AABB {
	if g.solid == nil || !g.solid(x, y, z) {
		return dst
	}
	fx, fy, fz := float32(x), float32(y), float32(z)
	return append(dst, physics.AABB{
		Min: core.Vec3{X: fx, Y: fy, Z: fz},
		Max: core.Vec3{X: fx + 1, Y: fy + 1, Z: fz + 1},
	})
}

// flatFloor is solid for every column at grid-y 0, i.e. world Y in [0, 1).
func flatFloor(x, y, z int) bool { return y == 0 }

// stepOneTick drives exactly the sequence Update uses to turn raw input into
// a resolved body: build an Intent from in and lookDir, then hand it to
// physics.Step. This is the wiring under test, not physics.Step in
// isolation.
func stepOneTick(p *Player, grid physics.Grid, in MovementInput, lookDir core.Vec3, dt float32) {
	intent := BuildIntent(in, lookDir, p.Flying, p.WalkSpeed, p.FlySpeed)
	physics.Step(&p.Body, grid, p.PhysicsConfig, intent, dt)
}

// newTestPlayer builds a Player around a fresh PlayerState at pos, with
// deterministic speeds and physics tuning independent of config.Current.
func newTestPlayer(pos core.Vec3) *Player {
	state := &world.PlayerState{Position: [3]float32{pos.X, pos.Y, pos.Z}}
	p := NewPlayer(state)
	p.WalkSpeed = 4.5
	p.FlySpeed = 10
	p.PhysicsConfig = physics.DefaultConfig()
	return p
}

// TestWalkingCannotEnterGeometry_ThroughController re-asserts criterion 2:
// the M12 collision guarantee must still hold once movement is expressed as
// BuildIntent -> physics.Step rather than a direct physics.Intent literal.
func TestWalkingCannotEnterGeometry_ThroughController(t *testing.T) {
	t.Parallel()

	// Floor everywhere, plus a wall at x==5 taller than StepHeight so it
	// cannot be stepped over.
	grid := fakeGrid{solid: func(x, y, z int) bool {
		return y == 0 || (x == 5 && y >= 1 && y <= 4)
	}}

	p := newTestPlayer(core.Vec3{X: 2, Y: 5, Z: 0})
	lookDir := core.Vec3{X: 1, Z: 0}
	in := MovementInput{Forward: true}

	const dt = 1.0 / 60
	for i := 0; i < 600; i++ {
		stepOneTick(p, grid, in, lookDir, dt)
	}

	const wantMaxX = float32(5)
	if got := p.Body.Box().Max.X; got > wantMaxX+1e-3 {
		t.Fatalf("Box().Max.X = %v, want <= %v (walking through the controller should stop at the wall)", got, wantMaxX)
	}
	if !p.Body.Grounded {
		t.Fatalf("Grounded = false at the end, want true (should have landed on the floor)")
	}
}

// TestFlightThenWalkingAgain_TogglingTwiceRestoresBehaviour covers
// criterion 3: flying ignores the wall and gravity that stopped walking, and
// toggling back to walking makes the very same wall block movement again --
// "toggling twice returns the original behaviour."
func TestFlightThenWalkingAgain_TogglingTwiceRestoresBehaviour(t *testing.T) {
	t.Parallel()

	grid := fakeGrid{solid: func(x, y, z int) bool {
		return y == 0 || (x == 5 && y >= 1 && y <= 4)
	}}
	lookDir := core.Vec3{X: 1, Z: 0}
	const dt = 1.0 / 60

	p := newTestPlayer(core.Vec3{X: 2, Y: 5, Z: 0})

	// Phase A: walking into the wall stops at it (same property as the test
	// above, established here as this test's baseline "original behaviour").
	for i := 0; i < 600; i++ {
		stepOneTick(p, grid, MovementInput{Forward: true}, lookDir, dt)
	}
	if got := p.Body.Box().Max.X; got > 5+1e-3 {
		t.Fatalf("phase A (walking): Box().Max.X = %v, want <= 5", got)
	}

	// Phase B: flying passes straight through the same wall.
	p.ToggleFlight()
	for i := 0; i < 200; i++ {
		stepOneTick(p, grid, MovementInput{Forward: true}, lookDir, dt)
	}
	if p.Body.Grounded {
		t.Fatalf("phase B (flying): Grounded = true, want false")
	}
	if got := p.Body.Position.X; got < 7 {
		t.Fatalf("phase B (flying): Position.X = %v, want > 7 (should have flown through the wall)", got)
	}

	// Phase C: toggling back to walking, the wall blocks movement again from
	// the other side.
	p.ToggleFlight()
	xBeforePhaseC := p.Body.Position.X
	for i := 0; i < 600; i++ {
		stepOneTick(p, grid, MovementInput{Back: true}, lookDir, dt)
	}
	if got := p.Body.Box().Min.X; got < 6-1e-3 {
		t.Fatalf("phase C (walking again): Box().Min.X = %v, want >= 6 (blocked by the wall from the other side)", got)
	}
	if p.Body.Position.X >= xBeforePhaseC {
		t.Fatalf("phase C (walking again): Position.X = %v did not move back from %v; walking input had no effect", p.Body.Position.X, xBeforePhaseC)
	}
}

// TestJumpOnlyWhenGrounded_ThroughController covers the mid-air half of
// criterion 4: holding Jump while airborne must not add lift, whether the
// Intent comes from BuildIntent or a literal physics.Intent.
func TestJumpOnlyWhenGrounded_ThroughController(t *testing.T) {
	t.Parallel()

	grid := fakeGrid{solid: flatFloor}
	lookDir := core.Vec3{X: 1, Z: 0}
	const dt = 1.0 / 60

	holdsJump := newTestPlayer(core.Vec3{X: 0, Y: 1, Z: 0})
	holdsJump.Body.Grounded = true
	releasesJump := newTestPlayer(core.Vec3{X: 0, Y: 1, Z: 0})
	releasesJump.Body.Grounded = true

	// Both leave the ground with one real jump.
	stepOneTick(holdsJump, grid, MovementInput{Jump: true}, lookDir, dt)
	stepOneTick(releasesJump, grid, MovementInput{Jump: true}, lookDir, dt)

	// Bounded well under the ~34-tick flight time of a 9 blocks/s jump under
	// -32 blocks/s^2 gravity: both bodies must still be airborne throughout,
	// so any divergence is Jump leaking mid-air rather than an expected
	// second jump immediately on landing.
	for i := 0; i < 25; i++ {
		stepOneTick(holdsJump, grid, MovementInput{Jump: true}, lookDir, dt)
		stepOneTick(releasesJump, grid, MovementInput{}, lookDir, dt)

		if holdsJump.Body.Position.Y != releasesJump.Body.Position.Y {
			t.Fatalf("tick %d: holding Jump mid-air through the controller changed the trajectory: %v vs %v",
				i, holdsJump.Body.Position.Y, releasesJump.Body.Position.Y)
		}
	}
}

// TestJumpClearsOneBlockNotTwo_ThroughController covers the rest of
// criterion 4: a jump built through BuildIntent clears a one-block step and
// does not clear a two-block one.
func TestJumpClearsOneBlockNotTwo_ThroughController(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		wallHeight  int
		wantCleared bool
	}{
		{"one block step is cleared", 1, true},
		{"two block step is not cleared", 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			grid := fakeGrid{solid: func(x, y, z int) bool {
				if y == 0 {
					return true
				}
				return x == 5 && y >= 1 && y <= tt.wallHeight
			}}

			p := newTestPlayer(core.Vec3{X: 2, Y: 1, Z: 0})
			p.Body.Grounded = true
			// A fast horizontal speed, matching internal/physics's own test
			// for this exact scenario: clearing a one-block step by jumping
			// over it needs enough horizontal speed to cross the gap before
			// the jump arc brings the body back down, which is a property of
			// the jump/gravity constants rather than of any particular walk
			// speed the game happens to configure.
			p.WalkSpeed = 10
			lookDir := core.Vec3{X: 1, Z: 0}
			const dt = 1.0 / 120

			stepOneTick(p, grid, MovementInput{Forward: true, Jump: true}, lookDir, dt)
			for i := 0; i < 240; i++ {
				stepOneTick(p, grid, MovementInput{Forward: true}, lookDir, dt)
			}

			cleared := p.Body.Position.X > 6.5
			if cleared != tt.wantCleared {
				t.Fatalf("cleared = %v, want %v (final Position.X = %v)", cleared, tt.wantCleared, p.Body.Position.X)
			}
		})
	}
}

// TestSpawnIsSafe covers criterion 6 against a world shaped like the real
// game's: NewPlayerState's default spawn, over terrain built the same way
// GenerateFixedGrid's flat ground is (a solid surface with air above it).
// The player must not start embedded in that terrain, and must come to rest
// on it rather than falling through an unloaded or missing chunk.
func TestSpawnIsSafe(t *testing.T) {
	const surfaceY = 31 // grass top face sits at world Y = surfaceY+1 = 32.
	w := testutil.NewWorld(t).Chunks(-1, -1, 1, 1).Flat(surfaceY).Build()

	state := world.NewPlayerState()
	if state.Position[1] <= surfaceY+1 {
		t.Fatalf("test precondition failed: default spawn Y = %v is not above the terrain surface %v", state.Position[1], surfaceY+1)
	}

	p := NewPlayer(state)
	lookDir := core.Vec3{X: 1, Z: 0}
	const dt = 1.0 / 60

	for i := 0; i < 600; i++ {
		stepOneTick(p, w, MovementInput{}, lookDir, dt)
	}

	if !p.Body.Grounded {
		t.Fatalf("player never landed after spawning: Grounded = false, Position.Y = %v (fell out of the world?)", p.Body.Position.Y)
	}
	const wantY = float32(surfaceY + 1)
	if diff := p.Body.Position.Y - wantY; diff < -0.01 || diff > 0.01 {
		t.Fatalf("landed at Position.Y = %v, want approximately %v (the terrain surface)", p.Body.Position.Y, wantY)
	}
}
