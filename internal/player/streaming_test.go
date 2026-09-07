package player

import (
	"testing"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
)

// These tests cover validation criterion 6 of M15: the player cannot fall
// through unloaded terrain, including when moving faster than chunks can
// load. They exercise stepBody directly -- the same function Update calls --
// rather than reimplementing its held branch, so a regression in production
// code is what these tests actually observe. See stepBody's doc comment for
// why Update itself cannot be called from this test binary.

// TestStepBody_HeldFreezesPositionAndZeroesVerticalVelocity pins the
// mechanical contract directly: held means physics.Step is not called at
// all, and vertical velocity is zeroed.
func TestStepBody_HeldFreezesPositionAndZeroesVerticalVelocity(t *testing.T) {
	t.Parallel()

	grid := fakeGrid{} // solid nowhere: falling would be unobstructed if Step ran
	b := &physics.Body{
		Position: core.Vec3{X: 10, Y: 50, Z: 0},
		Velocity: core.Vec3{X: 3, Y: -40, Z: -2},
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
	}
	cfg := physics.DefaultConfig()
	intent := physics.Intent{Move: core.Vec3{X: 100, Z: 100}}

	stepBody(b, grid, cfg, intent, 1.0/60, true)

	if b.Position != (core.Vec3{X: 10, Y: 50, Z: 0}) {
		t.Fatalf("Position = %+v after a held step, want unchanged (position must not integrate)", b.Position)
	}
	if b.Velocity.Y != 0 {
		t.Fatalf("Velocity.Y = %v after a held step, want 0", b.Velocity.Y)
	}
}

// TestStepBody_NotHeldStepsNormally checks the other side of the branch:
// held=false must behave exactly like calling physics.Step directly, so the
// hold rule never changes ordinary, on-the-ground behaviour.
func TestStepBody_NotHeldStepsNormally(t *testing.T) {
	t.Parallel()

	grid := fakeGrid{solid: flatFloor}
	cfg := physics.DefaultConfig()
	intent := physics.Intent{Move: core.Vec3{X: 2}}
	const dt = 1.0 / 60

	viaStepBody := &physics.Body{Position: core.Vec3{X: 0, Y: 5, Z: 0}, Size: core.Vec3{X: 0.6, Y: 1.8, Z: 0.6}}
	viaStep := &physics.Body{Position: core.Vec3{X: 0, Y: 5, Z: 0}, Size: core.Vec3{X: 0.6, Y: 1.8, Z: 0.6}}

	for i := 0; i < 120; i++ {
		stepBody(viaStepBody, grid, cfg, intent, dt, false)
		physics.Step(viaStep, grid, cfg, intent, dt)
	}

	if viaStepBody.Position != viaStep.Position {
		t.Fatalf("stepBody(held=false) diverged from physics.Step: %+v vs %+v", viaStepBody.Position, viaStep.Position)
	}
}

// TestHeldPlayerDoesNotFallThroughUnloadedTerrain covers criterion 6 end to
// end, at absurd speed: a floor exists only for x in [0, loadedMaxX] --
// exactly what an unloaded chunk looks like to collision, which treats a
// missing chunk as air (see world/collision.go). A player held whenever
// their current position's "chunk" (here, whether x is past the loaded
// edge) has not loaded must never accumulate enough fall speed to drop
// through when it does load, because that fall speed was never allowed to
// build up in the first place.
func TestHeldPlayerDoesNotFallThroughUnloadedTerrain(t *testing.T) {
	t.Parallel()

	const loadedMaxX = 32
	grid := fakeGrid{solid: func(x, y, z int) bool {
		return y == 0 && x >= 0 && x <= loadedMaxX
	}}

	b := &physics.Body{
		Position: core.Vec3{X: 2, Y: 1, Z: 0},
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
		Grounded: true,
	}
	cfg := physics.DefaultConfig()
	const dt = 1.0 / 60
	// An absurd walking speed: far faster than any streamed chunk could
	// possibly load, which is exactly the scenario the hold rule exists for.
	const absurdSpeed = 100000.0

	minY := b.Position.Y
	for i := 0; i < 900; i++ {
		held := b.Position.X > loadedMaxX
		intent := physics.Intent{Move: core.Vec3{X: absurdSpeed}}
		stepBody(b, grid, cfg, intent, dt, held)
		if b.Position.Y < minY {
			minY = b.Position.Y
		}
	}

	// physics.Step's dt is fixed real time per call regardless of how many
	// substeps a huge horizontal speed forces internally, so even the one
	// tick spent crossing loadedMaxX before held engages can only lose a
	// fraction of a block to gravity -- nowhere close to falling out of the
	// world. A generous 5-block tolerance leaves no doubt this is about
	// falling through, not a rounding nitpick.
	const wantMinY = -5.0
	if minY < wantMinY {
		t.Fatalf("player fell through unloaded terrain: min Y reached %v (started at %v)", minY, 1.0)
	}
}
