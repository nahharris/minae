# Lighting Engine Rewrite and Project Foundations

**Date:** 2026-09-05
**Status:** Approved

## Problem

Minae renders, but its lighting is decorative rather than functional, and the project
has no way to prove any change is correct.

### The lighting is disconnected end to end

The mesh builder computes a skylight level per face and encodes it into the vertex
**alpha** channel, leaving RGB at 255 (`internal/gfx/mesh/builder.go`). The fragment
shader then multiplies `texelColor * fragColor`. Multiplying alpha by alpha never
touches RGB, so the entire skylight BFS is computed every frame and discarded before
it reaches a pixel.

The only surviving term is a directional diffuse, and it is also wrong.
`TimeOfDay.GetLightingState` builds `lightDir` as `(cos(h·2π), sin(h·2π), 0.2)`, which
at noon (`h = 0.5`) evaluates to `(-1, 0, 0.2)` — horizontal. The shader negates it and
dots it against the surface normal, so top faces receive `diff ≈ 0` at every hour of
the day. The sun never shines downward. The constant `Z = 0.2` tilt is what makes the
−Z faces respond only to ambient.

### The light engine cannot represent change

`CalculateChunkLighting` is a whole-chunk recompute that only ever *adds* light. It
wipes its own chunk's light map before reseeding from neighbour borders, and it has no
removal pass at all, so placing a block can never darken anything.

Three further defects compound this:

- `World.GetLight` returns **15** for a missing chunk. The BFS reads that as "already
  brighter than anything I could contribute" and halts. This is the reported
  cross-chunk propagation failure.
- The BFS writes into neighbouring chunks through `w.SetLight` but never marks them for
  re-meshing, so cross-seam updates are invisible until something else dirties them.
- `getStateFromTime` has a dead zone for `hour ∈ [0, 0.2)`: no day state matches, it
  falls through to `nightState`, whose `NextState` is `nil`, so colour snaps instead of
  interpolating.

### Nothing is verified

12 test functions across ~4,400 lines. `internal/world` sits at 10% coverage;
`internal/world/lighting`, `internal/blocks/model`, `internal/gfx/atlas` and
`internal/gfx` are all at 0%. There is no `.github/`, no CI, and no `.golangci.yml`
despite `mise.toml` defining a `lint` task.

Every package — including `internal/world` and `internal/platform/logging` — transitively
imports raylib. raylib-go is cgo, so testing the flood-fill algorithm currently requires
OpenGL and X11 headers present just to compile.

## Goals

Fix the lighting engine so that baked voxel light reaches the screen, caves are dark,
light crosses chunk seams, and the day/night cycle reads as realistic colour. Do it on
top of foundations that prove it stays fixed.

## Non-Goals

Deliberately excluded, and tracked in the roadmap backlog rather than here: block light
sources (torches, glowstone), ambient occlusion, smooth lighting, a directional sun
tint, greedy meshing, and every item already listed under "not implemented" in the
project status document.

## Architecture

Three layers, with raylib confined to the outermost:

```
cmd/minae ──► internal/game ──► internal/gfx  (raylib, GPU, shaders)
                    │
                    ├──► internal/player, internal/ui  (raylib)
                    │
                    └──► internal/world ──► internal/world/lighting   ◄── pure Go
                              └──► internal/blocks                    ◄── pure Go
                                        └──► internal/core            ◄── pure Go (new)
```

`internal/core` is a new package holding `Vec3` and `RGBA`. The simulation layer speaks
only in those types; `internal/gfx` converts to `rl.Vector3` and `rl.Color` at the
boundary. This is what allows the light engine to be tested at speed with no GPU
present.

## Milestones

### M1 — Verifiable foundations

No gameplay change. Everything here exists so the following milestones can be proven.

- `.golangci.yml` checked in, enabling `errcheck`, `staticcheck`, `govet`,
  `ineffassign`, `unused` and `gosimple` per the project's Go rules.
- `.github/workflows/ci.yml` with two jobs:
  - **build-test** on ubuntu, with `libgl1-mesa-dev`, `xorg-dev` and `libasound2-dev`
    installed for cgo: `go build ./...`, `go vet ./...`,
    `go test -race -covermode=atomic -coverprofile ./...`, then the coverage floor check.
  - **lint** running `golangci-lint`.
  - Go version resolved from `go.mod` via `setup-go`, with the module cache enabled.
- A checked-in `.coverage-floor` file holding the minimum acceptable total statement
  coverage, seeded at the measured value on the day M1 lands and ratcheted upward by
  hand thereafter. CI fails when total coverage drops below it.
- `internal/testutil`: synthetic world builders (`FlatWorld`, `WithOverhang`,
  `WithShaft`, `WithCave`) so lighting tests read as intent rather than as forty lines
  of `SetBlock` calls. Consumed by the `lighting` and `mesh` test packages; `world`'s
  own in-package tests keep building inline to avoid an import cycle.
- Docs: `docs/ROADMAP.md` as a thin index of milestones and their status, with
  `docs/milestones/M<n>-<slug>.md` holding each milestone's goal, ordered steps as
  checkboxes, and a verification section. Steps are checked off in the same commit that
  implements them. `PROJECT_STATUS.md` is rewritten against the real `internal/` layout;
  `REORGANIZATION.md` is deleted, since git history already records that refactor.

### M2 — Purify the core

- New `internal/core` package providing `Vec3` and `RGBA`.
- `internal/world` (`time.go`, `raycast.go`, `interaction.go`) and `internal/blocks`
  drop raylib entirely.
- `internal/platform/logging` splits: the raylib trace-callback hookup moves to
  `logging/raylog`, imported only by `cmd/minae`. Logging itself becomes pure.
- An **import guard test** walks the package import graph and fails if any package in
  the pure set gains a raylib dependency. This keeps the boundary from silently rotting.

### M3 — Rewrite the light engine

`CalculateChunkLighting` is replaced by an incremental engine that owns its own dirty
tracking:

```go
type Engine struct {
    world World                    // GetBlock, SkyLight, SetSkyLight, HasChunk
    dirty map[ChunkCoord]struct{}  // every write records its chunk
}

func (e *Engine) SeedChunk(c ChunkCoord)      // top-down column scan
func (e *Engine) OnBlockChanged(x, y, z int)  // incremental add + remove
func (e *Engine) DirtyChunks() []ChunkCoord   // returns and clears
```

Four correctness rules the current code violates:

1. **Skylight falls at full strength.** Travelling straight down through air, level 15
   stays 15; it decays by one only sideways and upward. This is what makes an open shaft
   bright to bedrock while a cave under an overhang goes properly black.
2. **Removal is a real algorithm.** Placing a block must darken. A second BFS zeroes
   cells dimmer than the node being removed and re-queues brighter cells as
   re-propagation sources.
3. **Unloaded chunks are opaque.** They block light and are never written to. The
   cosmetic exception lives in the mesh builder, not the engine: when sampling light for
   a face whose neighbour cell lies in an unloaded chunk, the builder falls back to 15 so
   the world edge does not render as a black wall. Engine correctness and edge cosmetics
   stay separate, documented concerns.
4. **Dirty tracking is automatic.** Every light write records its chunk, so cross-seam
   updates re-mesh the neighbour.

Storage: `Chunk.LightMap` is renamed `SkyLight`. A `BlockLight` array is *not* added
until light sources exist.

Tests, all pure and fast: flat terrain, overhang decay, vertical shaft, sealed cave,
cross-seam symmetry in both directions, and place-then-break round-tripping to an
identical state. The test that earns its keep is a **property test** asserting that after
any random sequence of block edits, the incremental result is byte-identical to a full
recompute from scratch. A separate test asserts every chunk whose light changed is
reported dirty.

### M4 — Get light onto the screen

**Vertex payload.** `R = skylight × 17` (mapping 0–15 onto 0–255), `G = blocklight × 17`
(always 0 until torches exist), `B = 255` (ambient-occlusion channel reserved but
unimplemented), `A = 255` (alpha freed for real transparency later).

**Fragment shader:**

```glsl
vec3 sky   = skyTint   * fragColor.r;
vec3 block = blockTint * fragColor.g;
vec3 light = max(sky, block) * faceBias(normal) * fragColor.b;
finalColor = vec4(texel.rgb * max(light, minAmbient), texel.a * fragColor.a);
```

Face bias lives in the shader rather than being baked into vertices, so it is tunable
without re-meshing: top 1.00, ±X 0.80, ±Z 0.60, bottom 0.50. This constant per-face step
supplies the sense of depth and directly replaces the broken N·L term, which is the fix
for "−Z faces receive only ambient".

**Day cycle.** `skyTint` is a single uniform, so time of day costs zero re-meshing and
enclosed spaces correctly ignore it. Three changes to `time.go`: make the state ring
circular (`night → dawn`) to eliminate the dead zone at `hour ∈ [0, 0.2)`; interpolate in
linear float space rather than sRGB `uint8`, which is why the current ramps read muddy;
and delete `lightDir` entirely.

Keyframes, tunable live against the running game:

| hour | phase    | skyTint                       |
|------|----------|-------------------------------|
| 0.00 | midnight | deep blue `(0.16, 0.18, 0.32)` |
| 0.27 | sunrise  | orange `(1.00, 0.60, 0.35)`    |
| 0.50 | noon     | warm white `(1.00, 0.98, 0.92)`|
| 0.76 | sunset   | orange-red `(1.00, 0.55, 0.30)`|
| 0.82 | dusk     | violet `(0.40, 0.30, 0.40)`    |

The vertex shader's per-vertex `transpose(inverse(matModel))` is dropped; chunk
transforms are pure translation, so it is wasted work every frame.

Shaders cannot be unit tested. M4's milestone document therefore carries an explicit
manual verification checklist — stand in a cave, watch a sunrise, break a ceiling block —
to be run before the milestone is marked done.

## Error Handling

The light engine performs no I/O and returns no errors; out-of-range coordinates and
unloaded chunks are defined, silent no-ops as described above. Existing error handling
conventions are unchanged: errors are returned last, wrapped with `%w`, and never
discarded with `_`.

## Verification

M1 through M3 are verified by `mise exec -- go test -race ./...` passing under the CI
gates, with the coverage floor rising as each milestone lands. M4 is verified by those
same gates plus its manual visual checklist, since no automated test can observe a
fragment shader.
