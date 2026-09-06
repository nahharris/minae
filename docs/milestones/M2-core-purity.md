# M2 — Purify the core

**Status:** ✅ Done
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

Remove raylib from the simulation layer, so the light engine can be tested at
speed with no GPU, no cgo and no window.

Before this milestone every package transitively imported raylib — including
`internal/world`, `internal/world/lighting` and even `internal/platform/logging`,
which pulled it in for the trace-callback hookup alone. raylib-go is cgo, so
testing a flood-fill algorithm required OpenGL, X11 and Wayland headers just to
compile.

## Steps

- [x] Add `internal/core` with `Vec3` and `RGBA`, plus tests.
- [x] Convert `internal/world/time.go` to `core.RGBA`.
- [x] Convert `internal/world/raycast.go` and `interaction.go` to `core.Vec3`.
- [x] Confirm `internal/blocks` is raylib-free. It already was; the guard test
      now holds it that way.
- [x] Move the raylib trace-callback hookup out of
      `internal/platform/logging` into `logging/raylog`, imported only by
      `cmd/minae`.
- [x] Convert at the boundary: `internal/gfx/convert.go` provides `ToColor`,
      `ToVector3` and `FromVector3`, the only place the two type systems meet.
- [x] Add an import guard test (`internal/archtest`).
- [x] Raise `.coverage-floor` from 21.0 to 21.5.

## Result

`internal/world` and `internal/world/lighting` no longer depend on raylib:

```
$ go list -deps ./internal/world | grep raylib
(no output)
```

The pure set is now `internal/core`, `internal/blocks`, `internal/blocks/model`,
`internal/world`, `internal/world/lighting`, `internal/platform/config`,
`internal/platform/logging` and `internal/testutil`. M3's lighting tests can run
against any of these with no GPU present.

## Design decisions worth recording

**`core` is deliberately tiny.** `Vec3` has `Length` and `Normalize`; `RGBA` has
no methods at all. No `Add`, `Sub`, `Dot`, `LengthSquared`, constructors or
`String()`. Nothing needs them yet, and the project's YAGNI rule is enforced. A
colour `Lerp` was also declined: M4 replaces colour interpolation with
linear-space float maths, so adding one now would be waste.

**`Normalize` returns the zero vector for zero-length input**, rather than NaN.
A NaN would propagate silently through every later calculation. Callers who must
distinguish "no direction" from a genuine unit vector check `Length` themselves.
This matches raylib's own behaviour, so the conversion changed nothing.

**Conversion lives in the render layer, not the simulation layer.** `internal/gfx`
owns `ToColor` / `ToVector3` / `FromVector3` because it is the raylib adapter.
`core` cannot import raylib, and scattering ad-hoc struct copies at each call
site would defeat the boundary.

**The guard test reads the dependency graph, not import lines.** A transitive
dependency introduced three packages away still fails it — which is exactly what
happened during verification: a deliberate raylib import in `internal/world` also
failed `internal/world/lighting`.

## Issues found and deliberately NOT fixed

Found while converting. All were left alone so the type refactor stayed a pure
type refactor; they are recorded in the [roadmap](../ROADMAP.md) backlog.

| Issue | Where | Why deferred |
|---|---|---|
| `dir` is never normalized despite the doc comment saying it is, so `maxDist` is measured in units of `\|dir\|` rather than world units | `world/raycast.go` | Works today only because the camera target sits at radius 1.0. Fragile, but out of scope. |
| Division by zero on axis-aligned rays: a `dir` component of exactly 0 yields `±Inf` deltas, and `NaN` when the start lands on a voxel boundary. `NaN` fails every comparison in the traversal loop, so the DDA takes a wrong branch | `world/raycast.go` | Not reachable from the current camera, which never produces an exactly-zero component. Real, unguarded. |
| `SunIntensity` is interpolated by `LerpColors`, discarded by `GetLightingState`, and read by nothing. Dead weight across 7 initialisers | `world/time.go` | M4 rewrites this file. |
| `lerpColor` truncates rather than rounds, and `t` is never clamped to `[0,1]`, so an out-of-range `t` would wrap a channel from 255 to 0 | `world/time.go` | Latent, not live — `getStateFromTime` currently keeps `t` in range. M4 replaces this with linear float maths. |
| `lightDir` is horizontal at every hour, so the sun never shines downward | `world/time.go` | Already the headline defect scheduled for [M4](M4-render-pipeline.md). Converted verbatim, bug intact. |

A defect was also found and fixed in new code during review: `core.Vec3.Length`
initially squared in float32 and only widened afterwards, overflowing to `+Inf`
for components above roughly 1e19. It now widens first, with a regression test
that was confirmed to fail against the old expression.

## Verification

```bash
mise run ci
```

At the close of M2:

- `go build ./...`, `go vet ./...` — clean
- `go test -race ./...` — all packages pass
- `golangci-lint run` — 0 issues
- Total coverage 21.5%, floor raised to 21.5. `internal/core` is at 100%.
- The purity guard was confirmed to fail when raylib is deliberately imported
  into a pure package — a guard that has never been seen to fail is not a guard.
