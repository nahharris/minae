package physics_test

import (
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
)

// testGrid is a sparse set of solid cells. Each solid cell contributes one unit
// box; half cells contribute a bottom half box, standing in for a slab.
type testGrid struct {
	solid map[[3]int]bool
	half  map[[3]int]bool
}

func newGrid() *testGrid {
	return &testGrid{solid: map[[3]int]bool{}, half: map[[3]int]bool{}}
}

func (g *testGrid) set(x, y, z int)     { g.solid[[3]int{x, y, z}] = true }
func (g *testGrid) setHalf(x, y, z int) { g.half[[3]int{x, y, z}] = true }

func (g *testGrid) CollisionBoxes(dst []physics.AABB, x, y, z int) []physics.AABB {
	fx, fy, fz := float32(x), float32(y), float32(z)
	switch {
	case g.solid[[3]int{x, y, z}]:
		return append(dst, physics.AABB{
			Min: core.Vec3{X: fx, Y: fy, Z: fz},
			Max: core.Vec3{X: fx + 1, Y: fy + 1, Z: fz + 1},
		})
	case g.half[[3]int{x, y, z}]:
		return append(dst, physics.AABB{
			Min: core.Vec3{X: fx, Y: fy, Z: fz},
			Max: core.Vec3{X: fx + 1, Y: fy + 0.5, Z: fz + 1},
		})
	}
	return dst
}

// overlapTolerance is how far a body may sit inside a box before it counts as a
// real intersection. Resting on a surface is contact, not penetration, and a
// resolver that leaves a small epsilon gap must not be reported as failing.
// 1e-4 blocks is far below anything visible and far above float32 noise.
const overlapTolerance = 1e-4

func overlaps(a, b physics.AABB) bool {
	pen := func(aMin, aMax, bMin, bMax float32) float32 {
		return float32(math.Min(float64(aMax), float64(bMax)) - math.Max(float64(aMin), float64(bMin)))
	}
	return pen(a.Min.X, a.Max.X, b.Min.X, b.Max.X) > overlapTolerance &&
		pen(a.Min.Y, a.Max.Y, b.Min.Y, b.Max.Y) > overlapTolerance &&
		pen(a.Min.Z, a.Max.Z, b.Min.Z, b.Max.Z) > overlapTolerance
}

// firstOverlap returns a description of a cell the body is inside, or "".
func firstOverlap(g *testGrid, b physics.Body) string {
	box := b.Box()
	var boxes []physics.AABB

	for x := int(math.Floor(float64(box.Min.X))) - 1; x <= int(math.Floor(float64(box.Max.X)))+1; x++ {
		for y := int(math.Floor(float64(box.Min.Y))) - 1; y <= int(math.Floor(float64(box.Max.Y)))+1; y++ {
			for z := int(math.Floor(float64(box.Min.Z))) - 1; z <= int(math.Floor(float64(box.Max.Z)))+1; z++ {
				boxes = g.CollisionBoxes(boxes[:0], x, y, z)
				for _, solid := range boxes {
					if overlaps(box, solid) {
						return fmt.Sprintf("body %+v (box %+v) is inside the block at (%d,%d,%d) %+v",
							b.Position, box, x, y, z, solid)
					}
				}
			}
		}
	}
	return ""
}

func playerBody(pos core.Vec3) physics.Body {
	return physics.Body{
		Position: pos,
		Size:     core.Vec3{X: 0.6, Y: 1.8, Z: 0.6},
	}
}

// A floor at y=0 spanning a generous area, plus scattered obstacles.
func randomGrid(rng *rand.Rand) *testGrid {
	g := newGrid()
	for x := -8; x <= 8; x++ {
		for z := -8; z <= 8; z++ {
			g.set(x, 0, z)
		}
	}
	for range 25 {
		x := rng.Intn(17) - 8
		z := rng.Intn(17) - 8
		y := 1 + rng.Intn(3)
		if rng.Intn(3) == 0 {
			g.setHalf(x, y, z)
		} else {
			g.set(x, y, z)
		}
	}
	return g
}

// The body must never end a tick inside geometry.
//
// This is the criterion the whole resolver exists to satisfy, and it is stated
// as a property because the failures are all in configurations nobody thinks to
// enumerate — a corner between three blocks, a step-up that lands in a ceiling,
// a diagonal slide along two walls at once.
func TestBodyNeverEndsInsideGeometry(t *testing.T) {
	t.Parallel()

	cfg := physics.DefaultConfig()

	for seed := int64(0); seed < 40; seed++ {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			t.Parallel()

			rng := rand.New(rand.NewSource(seed))
			g := randomGrid(rng)

			b := playerBody(core.Vec3{X: 0.5, Y: 6, Z: 0.5})
			if diff := firstOverlap(g, b); diff != "" {
				t.Fatalf("the test itself started the body inside geometry: %s", diff)
			}

			for tick := range 400 {
				in := physics.Intent{
					Move: core.Vec3{
						X: (rng.Float32()*2 - 1) * 6,
						Z: (rng.Float32()*2 - 1) * 6,
					},
					Jump: rng.Intn(8) == 0,
				}
				physics.Step(&b, g, cfg, in, 1.0/60.0)

				if diff := firstOverlap(g, b); diff != "" {
					t.Fatalf("tick %d: %s", tick, diff)
				}
			}
		})
	}
}

// No velocity, however absurd, may carry the body through a solid wall.
//
// This is what the substep cap buys, so it is tested far beyond any speed the
// game produces. A resolver that merely clamps to the contact plane without
// substepping passes at walking pace and fails here.
func TestNoTunnelingAtAnySpeed(t *testing.T) {
	t.Parallel()

	speeds := []float32{10, 100, 1_000, 100_000, 1e7, 1e9, 1e12}

	for _, speed := range speeds {
		t.Run(fmt.Sprintf("speed-%g", speed), func(t *testing.T) {
			t.Parallel()

			// A one-block-thick wall at x=4, floor at y=0.
			g := newGrid()
			for x := -10; x <= 10; x++ {
				for z := -4; z <= 4; z++ {
					g.set(x, 0, z)
				}
			}
			for y := 1; y <= 4; y++ {
				for z := -4; z <= 4; z++ {
					g.set(4, y, z)
				}
			}

			b := playerBody(core.Vec3{X: 0, Y: 1, Z: 0.5})
			cfg := physics.DefaultConfig()

			for range 120 {
				physics.Step(&b, g, cfg, physics.Intent{Move: core.Vec3{X: speed}}, 1.0/60.0)

				if b.Box().Min.X > 5 {
					t.Fatalf("body tunneled through the wall at x=4: now at %+v", b.Position)
				}
			}

			// Sanity: it should have actually reached the wall, or the test is
			// asserting nothing.
			if b.Box().Max.X < 3.5 {
				t.Errorf("body never reached the wall (box max X %v); the test proves nothing",
					b.Box().Max.X)
			}
		})
	}
}

// A body at rest must hold exactly the same Y, tick after tick.
//
// Asserted on the exact float rather than an epsilon. Gravity is applied every
// tick and cancelled by the floor every tick, so any imprecision in that
// cancellation accumulates: an epsilon check would pass while the player sank
// through the world over a minute of standing still.
func TestRestingBodyIsExactlyStable(t *testing.T) {
	t.Parallel()

	g := newGrid()
	for x := -4; x <= 4; x++ {
		for z := -4; z <= 4; z++ {
			g.set(x, 0, z)
		}
	}

	b := playerBody(core.Vec3{X: 0.5, Y: 4, Z: 0.5})
	cfg := physics.DefaultConfig()

	// Let it fall and settle.
	for range 120 {
		physics.Step(&b, g, cfg, physics.Intent{}, 1.0/60.0)
	}
	if !b.Grounded {
		t.Fatalf("body never landed; Y = %v", b.Position.Y)
	}

	settled := b.Position.Y
	for tick := range 600 {
		physics.Step(&b, g, cfg, physics.Intent{}, 1.0/60.0)

		if b.Position.Y != settled {
			t.Fatalf("tick %d: resting Y drifted from %v to %v (delta %g)",
				tick, settled, b.Position.Y, b.Position.Y-settled)
		}
		if !b.Grounded {
			t.Fatalf("tick %d: a body resting on a floor reported not grounded", tick)
		}
	}
}

// Walking into a wall and away again must not accumulate drift along it.
//
// Repeated snapping to a contact plane is where creep comes from: each tick
// nudges the body a fraction further, and after a minute against a wall the
// player has slid somewhere they never walked.
func TestBlockedMovementDoesNotDrift(t *testing.T) {
	t.Parallel()

	g := newGrid()
	for x := -6; x <= 6; x++ {
		for z := -6; z <= 6; z++ {
			g.set(x, 0, z)
		}
	}
	for y := 1; y <= 3; y++ {
		for z := -6; z <= 6; z++ {
			g.set(3, y, z)
		}
	}

	b := playerBody(core.Vec3{X: 0.5, Y: 1, Z: 0.5})
	cfg := physics.DefaultConfig()

	for range 60 {
		physics.Step(&b, g, cfg, physics.Intent{Move: core.Vec3{X: 5}}, 1.0/60.0)
	}
	against := b.Position

	// Keep pushing into the wall: nothing should change at all.
	for tick := range 300 {
		physics.Step(&b, g, cfg, physics.Intent{Move: core.Vec3{X: 5}}, 1.0/60.0)

		if b.Position.X != against.X {
			t.Fatalf("tick %d: X crept from %v to %v while pressed against a wall",
				tick, against.X, b.Position.X)
		}
		if b.Position.Z != against.Z {
			t.Fatalf("tick %d: Z drifted from %v to %v while walking straight into a wall",
				tick, against.Z, b.Position.Z)
		}
	}
}

// A body that starts inside geometry must get out, not jam.
//
// This is reachable in the real game: a block can be placed into the space the
// player occupies, and falling blocks will make it routine. A body that can
// never escape is a worse outcome than one that briefly clips.
func TestOverlappedStartEscapes(t *testing.T) {
	t.Parallel()

	g := newGrid()
	for x := -4; x <= 4; x++ {
		for z := -4; z <= 4; z++ {
			g.set(x, 0, z)
			g.set(x, 1, z) // a solid layer the body starts buried in
		}
	}

	b := playerBody(core.Vec3{X: 0.5, Y: 1.2, Z: 0.5})
	if firstOverlap(g, b) == "" {
		t.Fatal("the test did not actually start the body inside geometry")
	}

	cfg := physics.DefaultConfig()
	for range 600 {
		physics.Step(&b, g, cfg, physics.Intent{}, 1.0/60.0)
		if firstOverlap(g, b) == "" {
			return // escaped
		}
	}
	t.Errorf("body never escaped geometry it started inside; ended at %+v", b.Position)
}

// Identical inputs from an identical state must produce an identical result,
// or none of the property tests above are reproducible from their seed.
func TestStepIsDeterministic(t *testing.T) {
	t.Parallel()

	run := func() physics.Body {
		rng := rand.New(rand.NewSource(7))
		g := randomGrid(rng)
		b := playerBody(core.Vec3{X: 0.5, Y: 6, Z: 0.5})
		cfg := physics.DefaultConfig()

		for range 300 {
			in := physics.Intent{
				Move: core.Vec3{X: (rng.Float32()*2 - 1) * 6, Z: (rng.Float32()*2 - 1) * 6},
				Jump: rng.Intn(8) == 0,
			}
			physics.Step(&b, g, cfg, in, 1.0/60.0)
		}
		return b
	}

	first, second := run(), run()
	if first != second {
		t.Errorf("identical input produced different results:\n first: %+v\nsecond: %+v", first, second)
	}
}

// Step-up must be bounded: it may lift the body at most StepHeight, and only
// onto somewhere it can actually stand.
func TestStepUpIsBounded(t *testing.T) {
	t.Parallel()

	cfg := physics.DefaultConfig()

	g := newGrid()
	for x := -6; x <= 6; x++ {
		for z := -6; z <= 6; z++ {
			g.set(x, 0, z)
		}
	}
	// A half-height ledge (a slab) at x=3, and a full block behind it at x=5.
	for z := -6; z <= 6; z++ {
		g.setHalf(3, 1, z)
		g.set(5, 1, z)
	}

	b := playerBody(core.Vec3{X: 0.5, Y: 1, Z: 0.5})

	// The slab is one block wide, so standing on it is transient: the body steps
	// up, walks across, and drops off the far side. Track the peak rather than
	// the final position.
	var maxRise, peakY float32
	for range 240 {
		before := b.Position.Y
		physics.Step(&b, g, cfg, physics.Intent{Move: core.Vec3{X: 4}}, 1.0/60.0)

		if rise := b.Position.Y - before; rise > maxRise {
			maxRise = rise
		}
		if b.Position.Y > peakY {
			peakY = b.Position.Y
		}
	}

	if maxRise > cfg.StepHeight+overlapTolerance {
		t.Errorf("a single tick raised the body by %v, more than StepHeight %v", maxRise, cfg.StepHeight)
	}
	// It should have climbed onto the slab at some point...
	if peakY < 1.5 {
		t.Errorf("body never stepped onto the half-height ledge; peak Y = %v", peakY)
	}
	// ...and been stopped by the full-height block, never climbing it.
	if b.Box().Min.X > 5 {
		t.Errorf("body climbed a full-height block without jumping; X = %v", b.Position.X)
	}
	if b.Position.Y > 1.5 {
		t.Errorf("body ended up on top of the full block (Y = %v); step-up must not clear it", b.Position.Y)
	}
}

// Standing still must not build up fall speed.
//
// Position alone does not catch this. Gravity is applied every tick and the
// floor snaps the body back to the same place every tick, so a resolver that
// forgets to zero the vertical velocity on contact still looks perfectly
// stable — while the velocity climbs without bound. The bug only shows the
// moment you walk off a ledge, and then you drop at terminal velocity from a
// standing start.
func TestRestingBodyDoesNotAccumulateFallSpeed(t *testing.T) {
	t.Parallel()

	g := newGrid()
	for x := -6; x <= 6; x++ {
		for z := -6; z <= 6; z++ {
			g.set(x, 0, z)
		}
	}

	b := playerBody(core.Vec3{X: 0.5, Y: 4, Z: 0.5})
	cfg := physics.DefaultConfig()

	for range 120 {
		physics.Step(&b, g, cfg, physics.Intent{}, 1.0/60.0)
	}
	if !b.Grounded {
		t.Fatalf("body never landed; Y = %v", b.Position.Y)
	}

	// One tick of gravity is expected: it is applied before the floor cancels
	// it. Anything beyond that is accumulation.
	maxRested := float32(math.Abs(float64(cfg.Gravity))) / 60 * 1.5

	for tick := range 600 {
		physics.Step(&b, g, cfg, physics.Intent{}, 1.0/60.0)

		if speed := float32(math.Abs(float64(b.Velocity.Y))); speed > maxRested {
			t.Fatalf("tick %d: a body standing still has built up %v blocks/s of fall speed (limit %v).\n"+
				"Position looks stable because the floor keeps snapping it back, but stepping off a "+
				"ledge would now drop it at that speed from standing.",
				tick, speed, maxRested)
		}
	}
}
