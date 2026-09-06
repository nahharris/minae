# M2 — Purify the core

**Status:** 📋 Planned
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

Remove raylib from the simulation layer, so the light engine can be tested at
speed with no GPU, no cgo and no window.

Today every package transitively imports raylib — including `internal/world`,
`internal/world/lighting` and even `internal/platform/logging`, which pulls it
in for the trace-callback hookup. raylib-go is cgo, so testing a flood-fill
algorithm currently requires OpenGL and X11 headers just to compile.

The coupling is thinner than it looks. `internal/world` uses raylib for exactly
`rl.Color` and `rl.Vector3`, in `time.go`, `raycast.go` and `interaction.go`.

## Steps

- [ ] Add `internal/core` with `Vec3` and `RGBA`, plus tests.
- [ ] Convert `internal/world/time.go` to `core.RGBA`.
- [ ] Convert `internal/world/raycast.go` and `interaction.go` to `core.Vec3`.
- [ ] Confirm `internal/blocks` is raylib-free (it should already be; verify
      rather than assume).
- [ ] Move the raylib trace-callback hookup out of
      `internal/platform/logging` into `logging/raylog`, imported only by
      `cmd/minae`.
- [ ] Convert at the boundary in `internal/gfx` and `internal/player`:
      `core.Vec3` ⇄ `rl.Vector3`, `core.RGBA` ⇄ `rl.Color`.
- [ ] Add an import guard test asserting that no package in the pure set
      depends on raylib, so the boundary cannot rot silently.
- [ ] Raise `.coverage-floor`.

## Notes

The pure set is `internal/core`, `internal/blocks`, `internal/blocks/model`,
`internal/world`, `internal/world/lighting`, `internal/platform/config`,
`internal/platform/logging` and `internal/testutil`.

The guard test should read the dependency graph rather than grep for import
lines, so a transitive dependency introduced three packages away still fails
it. `go list -deps` output, or `golang.org/x/tools/go/packages`, both work.

## Verification

```bash
mise run ci
```

Plus: the import guard test fails when raylib is deliberately imported into a
pure package. Confirm that by temporarily adding such an import — a guard that
has never been seen to fail is not yet a guard.
