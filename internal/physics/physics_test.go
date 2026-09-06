package physics

import (
	"testing"

	"github.com/nahharris/minae/internal/core"
)

// testGrid is a fake Grid built from two predicates, so tests can compose
// floors, walls, slabs and steps without ever touching internal/world.
type testGrid struct {
	full func(x, y, z int) bool
	slab func(x, y, z int) bool
}

func (g testGrid) CollisionBoxes(dst []AABB, x, y, z int) []AABB {
	fx, fy, fz := float32(x), float32(y), float32(z)
	if g.full != nil && g.full(x, y, z) {
		dst = append(dst, AABB{
			Min: core.Vec3{X: fx, Y: fy, Z: fz},
			Max: core.Vec3{X: fx + 1, Y: fy + 1, Z: fz + 1},
		})
	}
	if g.slab != nil && g.slab(x, y, z) {
		dst = append(dst, AABB{
			Min: core.Vec3{X: fx, Y: fy, Z: fz},
			Max: core.Vec3{X: fx + 1, Y: fy + 0.5, Z: fz + 1},
		})
	}
	return dst
}

// flatFloor is solid for every column at grid-y 0, i.e. world Y in [0, 1).
func flatFloor(x, y, z int) bool { return y == 0 }

// standardBody returns a body of the default 0.6x1.8x0.6 dimensions resting
// exactly on a floor whose top face is at worldY, with everything else at
// rest.
func standardBody(x, worldY, z float32) Body {
	return Body{
		Position: core.Vec3{X: x, Y: worldY + epsilon, Z: z},
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
		Grounded: true,
	}
}

func runTicks(b *Body, g Grid, cfg Config, in Intent, dt float32, ticks int) {
	for i := 0; i < ticks; i++ {
		Step(b, g, cfg, in, dt)
	}
}

func TestFallsAndLandsExactlyOnFloor(t *testing.T) {
	t.Parallel()

	grid := testGrid{full: flatFloor}
	cfg := DefaultConfig()
	b := Body{
		Position: core.Vec3{X: 0, Y: 10, Z: 0},
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
	}

	runTicks(&b, grid, cfg, Intent{}, 1.0/60, 300)

	const wantY = 1 + epsilon
	if b.Position.Y != wantY {
		t.Fatalf("Position.Y = %v, want exactly %v", b.Position.Y, wantY)
	}
	if !b.Grounded {
		t.Fatalf("Grounded = false after landing, want true")
	}
	if b.Velocity.Y != 0 {
		t.Fatalf("Velocity.Y = %v after landing, want 0", b.Velocity.Y)
	}
}

func TestRestingBodyHoldsExactYOverManyTicks(t *testing.T) {
	t.Parallel()

	grid := testGrid{full: flatFloor}
	cfg := DefaultConfig()
	b := standardBody(0, 1, 0)

	const wantY = 1 + epsilon
	for i := 0; i < 500; i++ {
		Step(&b, grid, cfg, Intent{}, 1.0/60)
		if b.Position.Y != wantY {
			t.Fatalf("tick %d: Position.Y = %v, want exactly %v (no sinking, no jitter)", i, b.Position.Y, wantY)
		}
		if !b.Grounded {
			t.Fatalf("tick %d: Grounded = false, want true", i)
		}
	}
}

func TestWalkingIntoWallStopsOnlyThatAxis(t *testing.T) {
	t.Parallel()

	// A wall at x == 5 spans the body's full height, on top of an
	// everywhere floor.
	grid := testGrid{full: func(x, y, z int) bool {
		return y == 0 || (x == 5 && y >= 1 && y <= 2)
	}}
	cfg := DefaultConfig()
	b := standardBody(3, 1, 0)

	in := Intent{Move: core.Vec3{X: 5, Z: 2}}
	runTicks(&b, grid, cfg, in, 1.0/120, 600)

	const wantMaxX = 5 - epsilon
	gotMaxX := b.Box().Max.X
	if gotMaxX > wantMaxX+1e-6 {
		t.Fatalf("Box().Max.X = %v, want <= %v (stopped at the wall)", gotMaxX, wantMaxX)
	}
	if b.Velocity.X != 0 {
		t.Fatalf("Velocity.X = %v after hitting the wall, want 0", b.Velocity.X)
	}
	if b.Velocity.Z != 2 {
		t.Fatalf("Velocity.Z = %v, want 2 (unaffected by the X collision)", b.Velocity.Z)
	}
	if b.Position.Z <= 0 {
		t.Fatalf("Position.Z = %v, want > 0 (Z movement should be unobstructed)", b.Position.Z)
	}
}

func TestJumpFromGroundRisesAndReturns(t *testing.T) {
	t.Parallel()

	grid := testGrid{full: flatFloor}
	cfg := DefaultConfig()
	b := standardBody(0, 1, 0)
	startY := b.Position.Y

	Step(&b, grid, cfg, Intent{Jump: true}, 1.0/60)

	if b.Grounded {
		t.Fatalf("Grounded = true immediately after jumping, want false")
	}
	if b.Velocity.Y <= 0 {
		t.Fatalf("Velocity.Y = %v after jumping, want > 0", b.Velocity.Y)
	}
	if b.Position.Y <= startY {
		t.Fatalf("Position.Y = %v after jumping, want > %v (should have risen)", b.Position.Y, startY)
	}

	// Let it fall back; it should land on the exact same spot it started
	// from.
	runTicks(&b, grid, cfg, Intent{}, 1.0/60, 300)

	if b.Position.Y != startY {
		t.Fatalf("Position.Y = %v after landing, want exactly %v", b.Position.Y, startY)
	}
	if !b.Grounded {
		t.Fatalf("Grounded = false after landing, want true")
	}
}

func TestJumpingMidAirDoesNothing(t *testing.T) {
	t.Parallel()

	grid := testGrid{full: flatFloor}
	cfg := DefaultConfig()

	withJump := standardBody(0, 1, 0)
	withoutJump := standardBody(0, 1, 0)

	// Leave the ground under both bodies identically (one real jump each),
	// then compare a body that keeps holding Jump against one that never
	// presses it again.
	Step(&withJump, grid, cfg, Intent{Jump: true}, 1.0/60)
	Step(&withoutJump, grid, cfg, Intent{Jump: true}, 1.0/60)

	for i := 0; i < 20; i++ {
		Step(&withJump, grid, cfg, Intent{Jump: true}, 1.0/60)
		Step(&withoutJump, grid, cfg, Intent{}, 1.0/60)

		if withJump.Position.Y != withoutJump.Position.Y {
			t.Fatalf("tick %d: holding Jump mid-air changed the trajectory: %v vs %v",
				i, withJump.Position.Y, withoutJump.Position.Y)
		}
		if withJump.Velocity.Y != withoutJump.Velocity.Y {
			t.Fatalf("tick %d: holding Jump mid-air changed Velocity.Y: %v vs %v",
				i, withJump.Velocity.Y, withoutJump.Velocity.Y)
		}
	}
}

// wallOfHeight builds a grid with an everywhere floor and a wall at x == 5
// that is blocks tall blocks, standing on the floor.
func wallOfHeight(blocks int) testGrid {
	return testGrid{full: func(x, y, z int) bool {
		if y == 0 {
			return true
		}
		return x == 5 && y >= 1 && y <= blocks
	}}
}

func TestJumpClearsOneBlockStep(t *testing.T) {
	t.Parallel()

	grid := wallOfHeight(1)
	cfg := DefaultConfig()
	b := standardBody(2, 1, 0)

	in := Intent{Move: core.Vec3{X: 10}, Jump: true}
	Step(&b, grid, cfg, in, 1.0/120)
	runTicks(&b, grid, cfg, Intent{Move: core.Vec3{X: 10}}, 1.0/120, 240)

	if b.Position.X <= 6.5 {
		t.Fatalf("Position.X = %v, want > 6.5 (should have cleared the one-block step)", b.Position.X)
	}
	if !b.Grounded {
		t.Fatalf("Grounded = false at the end, want true (should have landed)")
	}
}

func TestJumpDoesNotClearTwoBlockStep(t *testing.T) {
	t.Parallel()

	grid := wallOfHeight(2)
	cfg := DefaultConfig()
	b := standardBody(2, 1, 0)

	in := Intent{Move: core.Vec3{X: 10}, Jump: true}
	Step(&b, grid, cfg, in, 1.0/120)
	runTicks(&b, grid, cfg, Intent{Move: core.Vec3{X: 10}}, 1.0/120, 240)

	if b.Position.X >= 5 {
		t.Fatalf("Position.X = %v, want < 5 (should be blocked by the two-block step)", b.Position.X)
	}
	const wantY = 1 + epsilon
	if b.Position.Y != wantY {
		t.Fatalf("Position.Y = %v, want exactly %v (should have fallen back to the floor)", b.Position.Y, wantY)
	}
	if !b.Grounded {
		t.Fatalf("Grounded = false at the end, want true")
	}
}

func TestStepUpOntoSlabWithoutJumping(t *testing.T) {
	t.Parallel()

	grid := testGrid{
		full: flatFloor,
		slab: func(x, y, z int) bool { return x == 5 && y == 1 },
	}
	cfg := DefaultConfig()
	b := standardBody(3, 1, 0)

	runTicks(&b, grid, cfg, Intent{Move: core.Vec3{X: 2}}, 1.0/60, 300)

	if b.Position.X <= 6.5 {
		t.Fatalf("Position.X = %v, want > 6.5 (should have stepped over the slab without jumping)", b.Position.X)
	}
}

func TestStepUpBlockedByFullHeightBlock(t *testing.T) {
	t.Parallel()

	grid := testGrid{full: func(x, y, z int) bool {
		return y == 0 || (x == 5 && y == 1)
	}}
	cfg := DefaultConfig()
	b := standardBody(3, 1, 0)

	runTicks(&b, grid, cfg, Intent{Move: core.Vec3{X: 2}}, 1.0/60, 200)

	if b.Position.X >= 5 {
		t.Fatalf("Position.X = %v, want < 5 (a full-height block is not step-up-able)", b.Position.X)
	}
	const wantY = 1 + epsilon
	if b.Position.Y != wantY {
		t.Fatalf("Position.Y = %v, want exactly %v (should never have risen)", b.Position.Y, wantY)
	}
}

func TestFlyIgnoresGravityAndCollision(t *testing.T) {
	t.Parallel()

	// Solid geometry directly in the flight path.
	grid := testGrid{full: func(x, y, z int) bool { return true }}
	cfg := DefaultConfig()
	b := Body{
		Position: core.Vec3{X: 0, Y: 0, Z: 0},
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
		Grounded: true, // should be cleared immediately
	}

	move := core.Vec3{X: 5, Y: 3, Z: -2}
	const dt = 1.0 / 60
	Step(&b, grid, cfg, Intent{Fly: true, Move: move}, dt)

	wantPos := core.Vec3{X: move.X * dt, Y: move.Y * dt, Z: move.Z * dt}
	if b.Position != wantPos {
		t.Fatalf("Position = %+v, want %+v (fly should move freely through geometry)", b.Position, wantPos)
	}
	if b.Grounded {
		t.Fatalf("Grounded = true in fly mode, want false")
	}
	if b.Velocity != move {
		t.Fatalf("Velocity = %+v, want %+v", b.Velocity, move)
	}

	for i := 0; i < 60; i++ {
		Step(&b, grid, cfg, Intent{Fly: true, Move: move}, dt)
	}
	if b.Grounded {
		t.Fatalf("Grounded = true after sustained flight through geometry, want false")
	}
}

func TestBodyStartingInsideGeometryEscapes(t *testing.T) {
	t.Parallel()

	// A single solid block at the origin; the body starts embedded in its
	// upper half with no floor anywhere else, so the only way out is the
	// depenetration pass.
	grid := testGrid{full: func(x, y, z int) bool { return x == 0 && y == 0 && z == 0 }}
	cfg := DefaultConfig()
	b := Body{
		Position: core.Vec3{X: 0.5, Y: 0.5, Z: 0.5},
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
	}
	obstacle := AABB{Min: core.Vec3{X: 0, Y: 0, Z: 0}, Max: core.Vec3{X: 1, Y: 1, Z: 1}}

	if !aabbOverlap(b.Box(), obstacle) {
		t.Fatalf("precondition failed: body does not start overlapping the obstacle")
	}

	Step(&b, grid, cfg, Intent{}, 1.0/60)

	if aabbOverlap(b.Box(), obstacle) {
		t.Fatalf("body box %+v still overlaps the obstacle after one Step", b.Box())
	}

	// It should never fall back in either: run it for a while and keep
	// checking.
	for i := 0; i < 100; i++ {
		Step(&b, grid, cfg, Intent{}, 1.0/60)
		if aabbOverlap(b.Box(), obstacle) {
			t.Fatalf("tick %d: body box %+v overlaps the obstacle again (jammed/oscillating)", i, b.Box())
		}
	}
}
