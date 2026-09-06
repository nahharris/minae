# M12 — Collision and gravity core

**Status:** 📋 Planned

## Objective

A pure, GPU-free physics core: move an axis-aligned body through a voxel world
without ever ending up inside it. Gravity, jumping, ground detection and a 0.6
step-up.

This milestone changes nothing the player can see. It is the library;
[M13](M13-player-controller.md) wires it to the camera and the keyboard.

## Why it is its own milestone

The split follows the boundary M2 established. Collision is pure arithmetic over
a grid of boxes: no raylib, no GPU, no window. Kept separate it can be property
tested exhaustively in milliseconds, which is the only practical way to gain
confidence in a resolver — the bugs are all in the cases nobody thought to try.

Bolting it directly onto the raylib `Player` would put the interesting logic
behind a camera and an input poll, where none of that is reachable.

## The package

`internal/physics`, importing only the standard library and `internal/core`.
Notably it does **not** import `internal/world`: it asks for collision boxes
through an interface defined where it is used, so tests drive it with a trivial
fake grid rather than building worlds.

```go
// Grid supplies the solid boxes of the world to the resolver.
type Grid interface {
    // CollisionBoxes appends the solid boxes of the block at the given
    // coordinates, in world space, to dst and returns it. Air contributes
    // nothing.
    CollisionBoxes(dst []AABB, x, y, z int) []AABB
}
```

Appending into a caller-owned slice keeps the resolver allocation-free in its
inner loop, which matters because it runs per substep per axis.

**Why boxes rather than a `SolidAt(x, y, z) bool`.** Step-up is set at 0.6
blocks so that slabs are walkable. If every non-air block collided as a unit
cube, a slab would be a full-height wall and 0.6 would clear nothing — the
decision would be self-defeating. A block's shape already exists in its
`ModelSpec` and meta bits; the adapter in M13 reads it. For now that means a
unit box for full blocks and a half box for slabs, which is enough to make the
step-up meaningful and leaves stairs a natural extension rather than a rewrite.

## Resolution strategy

**Substepped, per-axis, with a hard substep cap.** Not a swept solve.

Movement is divided into substeps no larger than `maxSubstep` (0.4 blocks) on
any axis, so a body can never cross a one-block-thick wall within a single
substep. Tunneling is then impossible by construction rather than by careful
maths, which is the simpler thing to get right and to keep right.

Within a substep, axes resolve independently, in the order Y, X, Z:

1. Apply gravity, clamped to terminal velocity.
2. Move on Y. If the body now overlaps a box, snap it to the contact plane and
   zero the Y velocity. Landing while moving downward sets *grounded*.
3. Move on X, then Z, each snapping and zeroing on contact.

Resolving Y first means grounded state is known before horizontal movement,
which is what step-up needs.

**Step-up.** If a horizontal move was blocked and the body is grounded, lift it
by up to `stepHeight`, retry the horizontal move, and accept the result only if
the body ends up unobstructed and supported. Otherwise discard the attempt
entirely and keep the blocked result. A step-up must never move the body
vertically by more than `stepHeight`, and must never place it somewhere it could
not have stood.

**Starting overlapped.** A body can legitimately start inside geometry — a block
placed into it, and later falling blocks. The resolver must define a behaviour
and never deadlock: it resolves outward rather than refusing to move. A body
that can never escape is worse than one that briefly clips.

## Constants

Starting values, to be tuned once M13 makes them visible.

| | value | note |
|---|---|---|
| Body | 0.6 × 1.8 × 0.6 | width × height × depth |
| Gravity | −32 blocks/s² | |
| Terminal velocity | −78 blocks/s | |
| Jump velocity | 9.0 blocks/s | `v²/2g` ≈ 1.27 blocks, clears one block |
| Step height | 0.6 blocks | walkable slabs, not walkable full blocks |
| Max substep | 0.4 blocks | tunneling guard |

## Validation criteria

The first two are the load-bearing ones and are properties, not cases.

1. **Never inside geometry.** After any sequence of moves from a valid start,
   the body's box never overlaps a collision box. Randomised over positions,
   velocities and grids.
2. **No tunneling at any speed.** A body starting on one side of a
   one-block-thick wall can never end up on the other side, including at
   absurd velocities. This is what the substep cap buys, so it must be tested
   at velocities far beyond anything the game produces.
3. **Resting is stable.** A grounded body with no input holds *exactly* the same
   Y across many ticks: no slow sinking, no jitter between grounded and falling.
   Assert on the exact float, not an epsilon — drift here is a real bug that an
   epsilon would hide.
4. **Step-up is bounded.** A step-up never raises the body more than
   `stepHeight`, and never leaves it unsupported or overlapping.
5. **Slabs are walkable, full blocks are not.** Directly encodes the decision
   that motivated collision boxes.
6. **No deadlock from an overlapped start.** A body starting inside geometry
   always reaches a non-overlapping position; it never becomes permanently
   stuck.
7. **Blocked movement does not drift.** Walking into a wall and back out
   returns the body to a sane position, with no accumulated creep along the
   wall from repeated snapping.
8. **Determinism.** Identical inputs from an identical state produce an
   identical result, so the property tests are reproducible from a seed.

## Explicitly out of scope

Fall damage, health, swimming, sprinting, crouching, entity-vs-entity
collision, and any general entity system. The player is the only body; a second
one is what would justify generalising, and there isn't one.
