# M5 — Clear the known-defect list

**Status:** ✅ Done

## Goal

Close out the defects recorded during M2 but deliberately left unfixed, so the
roadmap's known-defect list is empty and honest.

## What the list actually contained

Four entries were listed. Checking them against the code first turned up that
the list was wrong in both directions.

### Two were already fixed

M4's rewrite of `world/time.go` removed the dead `SunIntensity` field and
replaced the truncating, unclamped `lerpColor` with `lerpRGBA`, which
interpolates in float space, rounds rather than truncates, and clamps both the
factor and the resulting channel. Neither defect survived M4; the roadmap simply
was not updated when it landed. Retired without code changes.

**Process note:** these entries were written during M2 and outlived their fix by
a whole milestone. When a milestone touches a file with recorded defects, the
list needs re-checking as part of closing that milestone, not later.

### One was misdiagnosed

The recorded entry read:

> `world/raycast.go` divides by zero on axis-aligned rays. A `dir` component of
> exactly 0 produces `±Inf` deltas, and `NaN` when the ray start lands on a
> voxel boundary. `NaN` fails every comparison in the traversal loop, so the DDA
> silently takes a wrong branch.

Two claims in that are false, and the real bug is worse than the one described.

**No `NaN` is reachable.** `NaN` would need `0/0`. In the `dir < 0` branch the
numerator `floor(p) - p` can be zero — when the ray starts exactly on a voxel
boundary — but that branch is only taken when the divisor is non-zero. In the
other branch the numerator `floor(p) + 1 - p` lies in `(0, 1]` and is never
zero. Verified empirically across mid-voxel and on-boundary starts for both
signs of zero.

**`+0.0` is handled correctly, by accident.** It yields `+Inf` for both `tDelta`
and `tMax`, and `+Inf` is never the minimum, so that axis simply never advances
— which is exactly the desired behaviour for a ray with no component along it.
This path runs on every startup: the camera's initial direction is exactly
`(1, 0, 0)`.

**The real defect is `-0.0`, and it hangs the game.** Negative zero fails the
`dir < 0` test, so it takes the positive branch and computes `-Inf` for both
`tDelta` and `tMax`. A `tMax` of `-Inf` is smaller than every other axis, so that
axis is chosen on every iteration, and `dist` becomes `-Inf`, which is forever
less than `maxDist`. The loop never terminates. Reproduced: a `Raycast` with
`dir.X = -0.0` times out rather than returning.

That is a freeze, not a "wrong branch" — the worst failure mode a game can have,
and it was recorded as a minor unguarded edge case.

### One was accurate

`world/raycast.go` never normalizes `dir` despite its doc comment saying it
should, so `maxDist` is measured in units of `|dir|` rather than world units.
Correct as recorded.

## Validation criteria

Set before implementation. The two invariants are the load-bearing ones: they
hold for inputs nobody enumerated.

1. **Sign-of-zero invariance** — replacing any `+0.0` component of a direction
   with `-0.0` must not change the result in any respect.
2. **Positive-scale invariance** — `Raycast(start, dir, maxDist)` and
   `Raycast(start, k*dir, maxDist)` agree for every `k > 0`.
3. **Termination** for every axis-aligned case, failing loudly rather than
   hanging the suite.
4. **`maxDist` is in world units**, independent of `|dir|`.
5. **Zero-length direction returns no hit** and does not hang, even when the ray
   starts inside a solid block.
6. **Axis-aligned rays hit the right block, face and coordinates** — termination
   is not correctness.
7. **No regression**, including unchanged `PlayerArmLength` reach for the
   unit-length direction the game actually passes.

Criteria 1 and 2 must be demonstrated to fail against the unfixed code. A test
that has never been seen to fail is not evidence.

## Steps

- [x] Verify the recorded defects against the code; correct the two that were
      wrong and retire the two already fixed.
- [x] Fix the `-0.0` hang and normalize `dir`.
- [x] Establish the seven validation criteria as tests.
- [x] Raise `.coverage-floor` from 31.0 to 35.0.

## Verification

```bash
mise run ci
```

## Result

Both fixes landed in `internal/world/raycast.go`:

- A component of exactly zero is now handled before the sign check, routing both
  `+0.0` and `-0.0` to `+Inf`, so that axis is never chosen. No iteration cap
  was added; a cap would have masked this and any future numeric pathology
  instead of fixing it.
- `dir` is normalized on entry, so `maxDist` is in world units. A zero-length
  direction returns no hit, including when the ray starts inside a solid block —
  a ray with no direction cannot travel to a block, and reporting the voxel it
  happens to sit in would be a surprising special case.

All seven criteria hold. Criteria 1 and 2 were verified against the unfixed code
independently of the implementer: every `-0.0` axis case failed with
`did not return within 2s; likely an infinite loop`, and positive-scale
invariance failed with `k=0.25` missing a block that `k=1, 2, 100` all hit.

The termination test runs `Raycast` on its own goroutine with an internal
two-second deadline, so a hang fails that one test rather than stalling the whole
suite on `go test`'s global timeout.

`internal/world` coverage went from 20.1% to 50.2%; total 31.6% to 35.5%.

## A note on process

Two of the four recorded defects had already been fixed a milestone earlier, and
one of the remaining two was misdescribed in a way that undersold it — a freeze
recorded as a minor unguarded edge case. Both problems come from the same habit:
the entries were written from a report rather than from the code, and never
re-checked.

Recording a defect you decided not to fix is worth doing. Recording it without
verifying it, and then not re-reading it when a later milestone touches the same
file, is how a list stops being trustworthy.
