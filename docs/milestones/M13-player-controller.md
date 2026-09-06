# M13 — Player controller

**Status:** ✅ Done — pending visual confirmation
**Depends on:** [M12](M12-collision-core.md)

## Objective

Give the player a body. Walking under gravity becomes the default, flight
becomes a toggle, and the camera stops being the thing that moves.

This is the milestone where the project stops being a renderer you fly through.

## The structural change

**Today the camera position *is* the player position.** `Player.Update` moves
`Camera.Position` directly and syncs it into `PlayerState`. There is no body.

Physics needs one: a 0.6 × 1.8 box whose feet rest on the ground, with the
camera at eye height above it. So the body becomes the source of truth and the
camera is derived from it each frame, rather than the other way round.

That inversion is most of the work in this milestone, and it is worth doing
carefully — a half-converted version where two things both believe they own the
position is how "the camera drifts from the body" bugs start.

### A latent bug this exposes

`placingInsidePlayer` in `internal/world/interaction.go` tests whether the
placement position equals the *camera's* voxel:

```go
return int(math.Floor(float64(cameraPos.X))) == placePos[0] && ...
```

With a 1.8-tall body that is wrong — it guards one cell out of the two or three
the player occupies, so a block can be placed inside your own legs. Today
nothing bad follows, because there is no collision. Once there is, it means
placing a block that traps you.

It must become an overlap test between the placement box and the player's body,
which is also the first real consumer of M12 outside the resolver itself.

## Movement modes

Walking is the default. Flight remains, behind a toggle, with collision off —
it is how the lighting has been visually verified all along, and the manual
checklists in earlier milestones assume you can fly into a cave to look at it.

| | walking | flying |
|---|---|---|
| Gravity | yes | no |
| Collision | yes | no |
| Space | jump when grounded | ascend |
| Ctrl | (unused for now) | descend |

The mode is runtime state, not saved. It is the seed of a creative/survival
distinction later, but no such distinction is introduced here.

## Input becomes intent

`Player.Update` currently reads the keyboard and mutates the position in the
same breath. It should instead produce a movement *intent* — a desired
horizontal direction plus jump and mode flags — and hand that to the physics
step. Keeping input separate from resolution is what lets the interesting half
stay testable, and it is the same boundary M12 draws.

## Configuration

`config.Current.PlayerSpeed` is currently 10.0 and drives everything. Walking
and flying want different speeds — 10 blocks/s on foot is roughly twice a run.
Split into `walk_speed` (≈4.5) and `fly_speed` (keeping 10.0), both in
`config.yaml` alongside the existing keys, since these are the values most
likely to be tuned by feel.

## Validation criteria

1. **The camera follows the body, always.** After any sequence of movement,
   the camera sits exactly at the body's position plus eye height. No path
   through the update can move one without the other.
2. **Walking cannot enter geometry.** The M12 property re-asserted through the
   controller, so the wiring cannot reintroduce what the resolver prevents.
3. **Flight ignores collision, walking does not.** Toggling modes changes
   behaviour and nothing else; toggling twice returns the original behaviour.
4. **Jump only when grounded.** Holding space mid-air does not climb. Jumping
   from ground clears a one-block step and does not clear a two-block one,
   which pins the jump constant to something meaningful rather than arbitrary.
5. **`placingInsidePlayer` covers the whole body.** Placing a block into any
   cell the player's box overlaps is refused — head, feet, and the cell between
   them. Test each individually; a test that only covers the feet would pass
   against today's buggy version.
6. **Spawn is safe.** The player does not begin inside terrain or fall through
   the world at startup. With a flat plane this is nearly free; it will matter
   as soon as world generation lands, so the assertion belongs here.
7. **Existing interaction still works.** Break and place still target what the
   crosshair points at, with the ray still originating at the eye rather than
   the feet.

## Manual verification

Automated tests cover the body; only eyes can judge the feel.

- [ ] Walking feels neither floaty nor sluggish; stopping does not slide.
- [ ] Jumping clears exactly one block.
- [ ] Walking onto a slab steps up smoothly, without a visible hop or camera
      snap.
- [ ] Walking into a wall stops cleanly, with no stutter or sticking while
      strafing along it.
- [ ] Falling from height lands without clipping through the floor.
- [ ] Toggling flight and back leaves the player in a sane position.
- [ ] A block cannot be placed inside yourself, at any part of the body.

## Explicitly out of scope

Sprinting, crouching, fall damage, health, swimming, and any general entity
system. The player is the only body here.

## Result

`internal/player` went from 0% to 57.5% coverage and `internal/world` from 53.8%
to 74.5%; total 53.8%.

That jump in `internal/player` is the point of the restructure rather than a
side effect. The package was previously untestable — every path ran through
`rl.IsKeyDown` and a live camera. Pulling intent construction, camera derivation
and mode switching out into functions that touch no raylib made the logic
reachable, and the raylib-only remainder is now mouse-look and key polling.

### Ownership

`Player.Body` is the sole source of truth. The camera is derived through one
function, called once per frame immediately after `physics.Step`, and nothing
else assigns `Camera.Position`. `PlayerState.Position` now stores the body's feet
rather than the eye.

The fly toggle is F3 (F1, F2 and Escape were taken).

### Verification of the placement fix

`placingInsidePlayer` now overlaps the placement block against the player's box.
The test asserts the feet cell, the cell between, and the head cell
**individually** — reverting to the old single-cell check fails all three. A test
that only covered the feet would have passed against the buggy version, which is
exactly the trap the criterion was written to avoid.

### A feel question that turned out fine

The implementer's controller-level "jump clears a one-block step" test raises
walk speed to 10, on the grounds that clearing a one-block-*wide* step needs
enough horizontal speed to cross it inside the jump arc. That is true, but it
left open whether the real default of 4.5 can climb onto terrain at all — the
most basic action in the game.

Checked directly against a raised plateau: 3.0, 4.5 and 6.0 all climb it
comfortably. Landing on a single isolated one-block-wide pillar is the harder
case and genuinely wants more speed, but ordinary terrain is fine at the default.
No tuning needed.

## Manual verification

The checklist above is unticked on purpose: it is the procedure to re-run
whenever the movement constants change, not a record of a past sign-off.

The constants most likely to want tuning are `walk_speed` and `fly_speed` in
`config.yaml`, and `JumpVelocity`, `Gravity` and `StepHeight` in
`physics.DefaultConfig`.
