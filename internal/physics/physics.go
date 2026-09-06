// Package physics is a pure, GPU-free collision and gravity core.
//
// It moves an axis-aligned body through a grid of solid boxes without ever
// leaving it inside one. It knows nothing about raylib, chunks, or block
// types: callers supply solid geometry through the Grid interface, which
// keeps this package testable with trivial fakes instead of real worlds.
//
// The resolver is substepped and per-axis rather than swept: movement within
// a tick is divided into pieces no larger than Config.MaxSubstep on any axis,
// so a body can never cross a one-block-thick wall inside a single substep.
// Within a substep, axes resolve independently in the order Y, X, Z, so that
// ground state is known before horizontal movement is attempted (step-up
// needs this).
package physics

import (
	"math"

	"github.com/nahharris/minae/internal/core"
)

// epsilon is the gap left between a body and whatever it just collided with.
//
// Snapping a body exactly onto a contact plane (zero gap) makes the very next
// tick's overlap test ambiguous at the boundary, and floating point makes
// "exactly touching" unreliable to detect consistently. Leaving a tiny gap
// instead means a resting body's next-tick collision check reliably reports
// "not yet touching", so gravity moves it by a tiny amount, the collision
// re-triggers, and it snaps back to the exact same epsilon-gapped position
// every time: no sinking, no jitter, and the resulting Y is bit-for-bit
// identical across ticks because it is always recomputed from the same
// obstacle geometry rather than accumulated from the previous position.
const epsilon float32 = 1e-4

// axis identifies one of the three coordinate axes for the internal resolver.
type axis int

const (
	axisX axis = iota
	axisY
	axisZ
)

// AABB is an axis-aligned box in world space.
type AABB struct{ Min, Max core.Vec3 }

// Grid supplies the solid boxes of the world to the resolver.
type Grid interface {
	// CollisionBoxes appends the solid boxes of the block at the given block
	// coordinates, in world space, to dst and returns it. Air contributes
	// nothing.
	CollisionBoxes(dst []AABB, x, y, z int) []AABB
}

// Body is a moving axis-aligned box.
type Body struct {
	// Position is the centre of the box's bottom face — the point between
	// the feet, not the centre of the box.
	Position core.Vec3
	Velocity core.Vec3
	Size     core.Vec3 // width, height, depth
	Grounded bool
}

// Box returns the body's current bounding box.
func (b Body) Box() AABB {
	halfWidth := b.Size.X / 2
	halfDepth := b.Size.Z / 2
	return AABB{
		Min: core.Vec3{X: b.Position.X - halfWidth, Y: b.Position.Y, Z: b.Position.Z - halfDepth},
		Max: core.Vec3{X: b.Position.X + halfWidth, Y: b.Position.Y + b.Size.Y, Z: b.Position.Z + halfDepth},
	}
}

// Intent is what the controller wants this tick.
type Intent struct {
	// Move is the desired horizontal velocity in blocks/second. Y is ignored,
	// except in fly mode (see Fly), where it is the desired vertical
	// velocity too.
	Move core.Vec3
	Jump bool
	// Fly disables gravity and collision entirely.
	Fly bool
}

// Config holds the tuning constants for Step.
type Config struct {
	Gravity          float32
	TerminalVelocity float32
	JumpVelocity     float32
	StepHeight       float32
	MaxSubstep       float32
}

// DefaultConfig returns the starting tuning constants: -32 blocks/s² gravity,
// -78 blocks/s terminal velocity, a 9.0 blocks/s jump (clears one block), a
// 0.6 block step height (walkable slabs, not walkable full blocks), and a 0.4
// block substep cap (tunneling guard).
func DefaultConfig() Config {
	return Config{
		Gravity:          -32,
		TerminalVelocity: -78,
		JumpVelocity:     9.0,
		StepHeight:       0.6,
		MaxSubstep:       0.4,
	}
}

// maxSubstepIterations bounds the number of substeps a single Step call can
// take. The substep cap that guarantees no tunneling makes the substep count
// proportional to velocity, which is unbounded in principle; this is a
// best-effort safety valve against a pathological or non-finite velocity
// hanging the caller, not a physical limit any reasonable input should hit.
const maxSubstepIterations = 1 << 20

// minRemainingTime is the point at which the substep loop in Step treats the
// remaining tick time as spent, to avoid looping forever on floating point
// remainders that never quite reach zero.
const minRemainingTime float32 = 1e-7

// maxDepenetrateIterations bounds how many times Step tries to push an
// overlapping body clear of geometry before giving up for this tick. Each
// iteration makes progress against at least one overlap, so this is a
// generous bound rather than a tuned constant; it exists purely so a
// pathological grid can never hang Step.
const maxDepenetrateIterations = 8

// Step advances the body by dt.
//
// In fly mode (Intent.Fly), gravity and collision are skipped entirely: the
// body moves freely along Intent.Move, including vertically, and Grounded is
// cleared.
//
// Otherwise, a body that starts this tick already overlapping geometry (a
// block placed into it, or a block that fell onto it) is pushed clear along
// whichever direction requires the smallest movement before the tick's
// ordinary movement is resolved. Refusing to move such a body would let it
// get permanently stuck, which is worse than the brief, bounded clipping this
// produces.
func Step(b *Body, g Grid, cfg Config, in Intent, dt float32) {
	if dt <= 0 {
		return
	}

	if in.Fly {
		b.Grounded = false
		b.Velocity = in.Move
		b.Position.X += b.Velocity.X * dt
		b.Position.Y += b.Velocity.Y * dt
		b.Position.Z += b.Velocity.Z * dt
		return
	}

	b.Velocity.X = in.Move.X
	b.Velocity.Z = in.Move.Z
	if in.Jump && b.Grounded {
		b.Velocity.Y = cfg.JumpVelocity
		b.Grounded = false
	}

	var buf []AABB
	depenetrate(b, g, &buf)

	remaining := dt
	for i := 0; i < maxSubstepIterations && remaining > minRemainingTime; i++ {
		subDt := computeSubDt(b.Velocity, cfg, remaining)

		b.Velocity.Y += cfg.Gravity * subDt
		if b.Velocity.Y < cfg.TerminalVelocity {
			b.Velocity.Y = cfg.TerminalVelocity
		}

		resolveY(b, g, &buf, b.Velocity.Y*subDt)
		resolveX(b, g, cfg, &buf, b.Velocity.X*subDt)
		resolveZ(b, g, cfg, &buf, b.Velocity.Z*subDt)

		remaining -= subDt
	}
}

// computeSubDt returns how much of the remaining tick time can be simulated
// in one substep without any axis moving further than Config.MaxSubstep.
//
// It bounds the substep using the largest velocity magnitude the body could
// have at any point in [0, remaining]: its velocity now, and what gravity
// would bring Y to by the end of the interval. Gravity changes Y velocity
// monotonically over that window, so those two endpoints bound every value in
// between, and therefore bound the distance actually travelled once gravity
// is applied for the (possibly smaller) chosen substep.
func computeSubDt(vel core.Vec3, cfg Config, remaining float32) float32 {
	tentativeVY := vel.Y + cfg.Gravity*remaining
	if tentativeVY < cfg.TerminalVelocity {
		tentativeVY = cfg.TerminalVelocity
	}

	maxSpeed := abs32(vel.X)
	if s := abs32(vel.Y); s > maxSpeed {
		maxSpeed = s
	}
	if s := abs32(tentativeVY); s > maxSpeed {
		maxSpeed = s
	}
	if s := abs32(vel.Z); s > maxSpeed {
		maxSpeed = s
	}

	if maxSpeed <= 0 {
		return remaining
	}

	limited := cfg.MaxSubstep / maxSpeed
	if limited < remaining {
		return limited
	}
	return remaining
}

// resolveY moves the body along Y by delta and resolves any collision.
//
// Landing (delta < 0 and blocked) sets Grounded. Any other outcome — hitting
// a ceiling, or moving without obstruction in either direction — clears it:
// the body is airborne unless it just came to rest on top of something.
func resolveY(b *Body, g Grid, buf *[]AABB, delta float32) {
	if delta == 0 {
		return
	}

	result, blocked := moveAndCollide(b.Box(), axisY, delta, g, buf)
	b.Position.Y = result.Min.Y

	if !blocked {
		b.Grounded = false
		return
	}
	b.Velocity.Y = 0
	b.Grounded = delta < 0
}

// resolveX moves the body along X by delta, resolving collisions and, when
// blocked while grounded, attempting a step-up before giving up and zeroing
// Velocity.X.
func resolveX(b *Body, g Grid, cfg Config, buf *[]AABB, delta float32) {
	if delta == 0 {
		return
	}

	result, blocked := moveAndCollide(b.Box(), axisX, delta, g, buf)
	if !blocked {
		b.Position.X = result.Min.X + b.Size.X/2
		return
	}
	if b.Grounded && tryStepUp(b, g, cfg, buf, axisX, delta) {
		return
	}
	b.Position.X = result.Min.X + b.Size.X/2
	b.Velocity.X = 0
}

// resolveZ is resolveX's twin for the Z axis.
func resolveZ(b *Body, g Grid, cfg Config, buf *[]AABB, delta float32) {
	if delta == 0 {
		return
	}

	result, blocked := moveAndCollide(b.Box(), axisZ, delta, g, buf)
	if !blocked {
		b.Position.Z = result.Min.Z + b.Size.Z/2
		return
	}
	if b.Grounded && tryStepUp(b, g, cfg, buf, axisZ, delta) {
		return
	}
	b.Position.Z = result.Min.Z + b.Size.Z/2
	b.Velocity.Z = 0
}

// tryStepUp attempts to carry a blocked horizontal move over an obstacle no
// taller than Config.StepHeight.
//
// It lifts the body by as much as StepHeight allows (less, if something
// lower overhead stops it), retries the horizontal move at that height, and
// then settles back down by the same amount so the body ends up resting on
// whatever it stepped onto rather than floating. The attempt is accepted only
// if every stage succeeds and the final position does not overlap anything;
// otherwise the body is left untouched and the caller keeps its normal
// blocked result. This bounds the rise to at most StepHeight and guarantees
// the accepted result is not overlapping.
func tryStepUp(b *Body, g Grid, cfg Config, buf *[]AABB, ax axis, delta float32) bool {
	box := b.Box()

	lifted, _ := moveAndCollide(box, axisY, cfg.StepHeight, g, buf)
	actualLift := lifted.Min.Y - box.Min.Y
	if actualLift <= 0 {
		return false
	}

	moved, blocked := moveAndCollide(lifted, ax, delta, g, buf)
	if blocked {
		return false
	}

	settled, landed := moveAndCollide(moved, axisY, -actualLift, g, buf)
	if boxOverlapsGrid(settled, g, buf) {
		return false
	}

	switch ax {
	case axisX:
		b.Position.X = settled.Min.X + b.Size.X/2
	case axisZ:
		b.Position.Z = settled.Min.Z + b.Size.Z/2
	}
	b.Position.Y = settled.Min.Y
	b.Grounded = landed
	return true
}

// depenetrate pushes b clear of any geometry it currently overlaps.
//
// Each iteration finds one overlapping obstacle and moves the body the
// shortest distance that clears it (out through the nearest face), which is
// the smallest disturbance that makes progress. The loop is capped rather
// than run until clear, so a pathological grid can never hang the caller; in
// practice ordinary geometry clears in one or two iterations.
func depenetrate(b *Body, g Grid, buf *[]AABB) {
	for i := 0; i < maxDepenetrateIterations; i++ {
		box := b.Box()
		obstacle, ok := firstOverlap(box, g, buf)
		if !ok {
			return
		}
		pushOut(b, box, obstacle)
	}
}

// pushOut moves b's position by the shortest displacement that clears box
// from obstacle, out through whichever of the six faces is nearest.
func pushOut(b *Body, box, obstacle AABB) {
	pushPosX := obstacle.Max.X - box.Min.X
	pushNegX := box.Max.X - obstacle.Min.X
	pushPosY := obstacle.Max.Y - box.Min.Y
	pushNegY := box.Max.Y - obstacle.Min.Y
	pushPosZ := obstacle.Max.Z - box.Min.Z
	pushNegZ := box.Max.Z - obstacle.Min.Z

	best := pushPosX
	choice := 0
	if pushNegX < best {
		best, choice = pushNegX, 1
	}
	if pushPosY < best {
		best, choice = pushPosY, 2
	}
	if pushNegY < best {
		best, choice = pushNegY, 3
	}
	if pushPosZ < best {
		best, choice = pushPosZ, 4
	}
	if pushNegZ < best {
		best, choice = pushNegZ, 5
	}

	switch choice {
	case 0:
		b.Position.X += best + epsilon
	case 1:
		b.Position.X -= best + epsilon
	case 2:
		b.Position.Y += best + epsilon
	case 3:
		b.Position.Y -= best + epsilon
	case 4:
		b.Position.Z += best + epsilon
	case 5:
		b.Position.Z -= best + epsilon
	}
}

// moveAndCollide translates box by delta along ax and resolves it against
// the grid.
//
// If nothing blocks the move, it returns the translated box and false. If
// something does, it returns box with only the ax component adjusted so the
// box sits epsilon clear of the nearest obstacle on the side it moved into,
// and true. Only the block coordinates the swept box could actually touch
// are queried.
func moveAndCollide(box AABB, ax axis, delta float32, g Grid, buf *[]AABB) (AABB, bool) {
	newBox := box
	switch ax {
	case axisX:
		newBox.Min.X += delta
		newBox.Max.X += delta
	case axisY:
		newBox.Min.Y += delta
		newBox.Max.Y += delta
	case axisZ:
		newBox.Min.Z += delta
		newBox.Max.Z += delta
	}

	xLo, xHi := blockRange(box.Min.X, box.Max.X)
	yLo, yHi := blockRange(box.Min.Y, box.Max.Y)
	zLo, zHi := blockRange(box.Min.Z, box.Max.Z)
	switch ax {
	case axisX:
		xLo, xHi = blockRange(min(box.Min.X, newBox.Min.X), max(box.Max.X, newBox.Max.X))
	case axisY:
		yLo, yHi = blockRange(min(box.Min.Y, newBox.Min.Y), max(box.Max.Y, newBox.Max.Y))
	case axisZ:
		zLo, zHi = blockRange(min(box.Min.Z, newBox.Min.Z), max(box.Max.Z, newBox.Max.Z))
	}

	*buf = (*buf)[:0]
	*buf = collectBoxes(g, *buf, xLo, xHi, yLo, yHi, zLo, zHi)

	blocked := false
	var contact float32
	for _, obstacle := range *buf {
		if !aabbOverlap(newBox, obstacle) {
			continue
		}

		var c float32
		switch ax {
		case axisX:
			if delta > 0 {
				c = obstacle.Min.X
			} else {
				c = obstacle.Max.X
			}
		case axisY:
			if delta > 0 {
				c = obstacle.Min.Y
			} else {
				c = obstacle.Max.Y
			}
		case axisZ:
			if delta > 0 {
				c = obstacle.Min.Z
			} else {
				c = obstacle.Max.Z
			}
		}

		if !blocked {
			contact, blocked = c, true
			continue
		}
		if delta > 0 {
			contact = min(contact, c)
		} else {
			contact = max(contact, c)
		}
	}

	if !blocked {
		return newBox, false
	}

	result := box
	switch ax {
	case axisX:
		size := box.Max.X - box.Min.X
		if delta > 0 {
			result.Max.X = contact - epsilon
			result.Min.X = result.Max.X - size
		} else {
			result.Min.X = contact + epsilon
			result.Max.X = result.Min.X + size
		}
	case axisY:
		size := box.Max.Y - box.Min.Y
		if delta > 0 {
			result.Max.Y = contact - epsilon
			result.Min.Y = result.Max.Y - size
		} else {
			result.Min.Y = contact + epsilon
			result.Max.Y = result.Min.Y + size
		}
	case axisZ:
		size := box.Max.Z - box.Min.Z
		if delta > 0 {
			result.Max.Z = contact - epsilon
			result.Min.Z = result.Max.Z - size
		} else {
			result.Min.Z = contact + epsilon
			result.Max.Z = result.Min.Z + size
		}
	}
	return result, true
}

// firstOverlap returns the first solid box overlapping box, if any.
func firstOverlap(box AABB, g Grid, buf *[]AABB) (AABB, bool) {
	xLo, xHi := blockRange(box.Min.X, box.Max.X)
	yLo, yHi := blockRange(box.Min.Y, box.Max.Y)
	zLo, zHi := blockRange(box.Min.Z, box.Max.Z)

	*buf = (*buf)[:0]
	*buf = collectBoxes(g, *buf, xLo, xHi, yLo, yHi, zLo, zHi)

	for _, obstacle := range *buf {
		if aabbOverlap(box, obstacle) {
			return obstacle, true
		}
	}
	return AABB{}, false
}

// boxOverlapsGrid reports whether box overlaps any solid geometry.
func boxOverlapsGrid(box AABB, g Grid, buf *[]AABB) bool {
	_, ok := firstOverlap(box, g, buf)
	return ok
}

// collectBoxes appends every solid box in the given inclusive block
// coordinate range to dst and returns it.
func collectBoxes(g Grid, dst []AABB, xLo, xHi, yLo, yHi, zLo, zHi int) []AABB {
	for x := xLo; x <= xHi; x++ {
		for y := yLo; y <= yHi; y++ {
			for z := zLo; z <= zHi; z++ {
				dst = g.CollisionBoxes(dst, x, y, z)
			}
		}
	}
	return dst
}

// blockRange returns the inclusive range of block coordinates spanning
// [lo, hi], a half-open interval per block boundary.
func blockRange(lo, hi float32) (int, int) {
	return int(math.Floor(float64(lo))), int(math.Floor(float64(hi)))
}

// aabbOverlap reports whether a and b intersect with positive volume. Boxes
// that only touch at a shared face do not overlap.
func aabbOverlap(a, b AABB) bool {
	return a.Min.X < b.Max.X && a.Max.X > b.Min.X &&
		a.Min.Y < b.Max.Y && a.Max.Y > b.Min.Y &&
		a.Min.Z < b.Max.Z && a.Max.Z > b.Min.Z
}

// abs32 returns the absolute value of v.
func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
