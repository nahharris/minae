# M12 — Collision and gravity core

**Status:** ✅ Done

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

## Result

`internal/physics` at 91.3% coverage; total 47.3%. The package imports only the
standard library and `internal/core`, and is in `archtest`'s `purePackages`.

### What the implementation chose

**Contact snapping is computed from the obstacle, not from the body.** When a
move is blocked, the body is placed at a fixed offset from the obstacle's face
rather than backed off by its penetration depth. The landing position is
therefore a pure function of the static box, independent of entry velocity — so
a resting body recomputes the identical float every tick and is bit-exact
stable, rather than merely close.

That turned out to matter more than the epsilon gap itself: setting `epsilon` to
zero leaves every property passing. Stability comes from the snap formula, not
from the gap.

**An overlapped body pushes out along the shortest axis**, bounded to eight
iterations so a pathological grid cannot hang. A body that briefly clips is a
better outcome than one that can never escape.

**A substep-count cap** guards against unbounded work at degenerate velocities.
It does not weaken the no-tunneling property: the resolver clamps within each
substep regardless of substep size, and the property test confirms no tunneling
at 10¹² blocks/s while also asserting the body actually reached the wall, so it
cannot pass by standing still.

### Verification

Eight hand-written cases plus eight properties. Mutation testing on the
properties:

| Mutation | Caught by |
|---|---|
| No substepping | `NoTunnelingAtAnySpeed` — tunnels at 100 blocks/s |
| X axis never collides | `BodyNeverEndsInsideGeometry` — inside a block by tick 160 |
| Landing does not zero Y velocity | *nothing, initially* |

That third row is the useful one. Position stays perfectly stable without
zeroing the velocity, because the floor snaps the body back every tick — so the
existing properties all passed while fall speed climbed without bound. The bug
would only appear the moment the player stepped off a ledge, dropping at
terminal velocity from standing.

`RestingBodyDoesNotAccumulateFallSpeed` was added to close that, and it catches
the mutation at 64 blocks/s of accumulated speed on tick 0.

### A test that was wrong, not the code

`StepUpIsBounded` initially failed, and the implementation was correct. The slab
in it is one block wide, so standing on it is transient: the body steps up,
walks across, and drops off the far side. Asserting on the *final* Y measured
the wrong moment. It now tracks the peak.

Worth recording because the reflex on a red test is to change the code.
