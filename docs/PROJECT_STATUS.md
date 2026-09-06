# Minae — Project Status

**Minae** is a voxel game engine written in Go on top of raylib.

Last reviewed: 2026-09-05, at the close of [M5](milestones/M5-known-defects.md).
For what happens next, see the [roadmap](ROADMAP.md).

This document describes what is *actually true today*, including what is
broken. Aspirations belong in the roadmap.

## Layout

```
cmd/minae/          Entry point
internal/
  archtest/         Architectural constraints the compiler cannot express
  blocks/           Block definitions, registry, models (full, sided, slab, orientable)
  core/             Vec3 and RGBA — the pure value types the simulation speaks
  game/             Game coordinator and states
  gfx/              Scene renderer, texture atlas, chunk meshing, type conversion
  platform/         Config, logging, resource loading
  platform/logging/raylog/   raylib trace-log bridge, isolated from logging
  player/           Camera and input
  testutil/         Synthetic world builder for tests
  ui/core/          UI framework (panels, buttons, labels, layout)
  ui/game/          HUD, pause menu, debug overlay
  world/            World, chunks, terrain, time, raycasting, interaction
  world/lighting/   Skylight propagation and shaders
```

19 packages, ~4,200 lines of non-test Go, ~950 lines of test.

### The raylib boundary

raylib is cgo and needs OpenGL, X11 and Wayland headers to compile. Since M2 the
simulation layer is free of it, so it can be tested with no GPU and no window:

```
cmd/minae ──► game ──► gfx          (raylib, GPU, shaders)
                 ├──► player, ui    (raylib)
                 └──► world ──► world/lighting    ◄── pure Go
                          └──► blocks ──► core    ◄── pure Go
```

`internal/gfx/convert.go` holds `ToColor`, `ToVector3` and `FromVector3` — the
only place the two type systems meet. `internal/archtest` fails the build if any
pure package regains a raylib dependency, directly or transitively.

## Known broken

Nothing in the lighting pipeline. M4 was visually verified on 2026-09-05.

Fixed across M3 and M4: light can be removed as well as added, so placing a
block darkens what is beneath it; unloaded chunks are opaque rather than
reporting full sky, so propagation crosses chunk seams; every chunk whose light
changed is reported for re-meshing; light travels in vertex RGB rather than
alpha, so dark blocks render dark instead of see-through; per-face bias replaced
a directional sun that never pointed downward; and the day cycle ring is
circular, so colour eases rather than snapping.

The roadmap's known-defect list is empty as of M5.

## Working

- **Blocks** — thread-safe registry with numeric IDs; full, sided, slab and
  4-way orientable models; per-instance metadata; face-culling occlusion tests;
  YAML block definitions loaded from disk.
- **World** — 16×16×256 chunks in flat arrays; global coordinates with correct
  negative-coordinate handling; 3D DDA raycasting for block targeting.
- **Rendering** — face-culled mesh generation, runtime texture atlas with UV
  mapping, chunk mesh upload and cleanup.
- **Player** — first-person camera with pitch clamping, WASD + Space/Ctrl
  movement, scroll-selected hotbar.
- **Interaction** — break and place, with slab orientation from the clicked
  face and 4-way facing from view direction.
- **UI** — custom panel/stack layout framework; crosshair, hotbar, pause menu,
  debug overlay.
- **Platform** — YAML config with defaults, structured logrus logging,
  centralised resource loader.

## Partially working

| Area | Today | Missing |
|---|---|---|
| Terrain | Flat plane: stone/dirt/grass at y=32 | Noise, caves, biomes, structures, ores |
| Chunk management | Fixed 3×3 grid at spawn | Streaming based on player position |
| Physics | None — noclip flight | Collision, gravity, jumping |
| Persistence | Data model supports it | No serialisation to disk |
| Textures | Three PNGs, colour fallback otherwise | Full block texture set |
| Blocks | 6 types | Transparency, liquids, stairs, fences, doors |
| Lighting | Skylight, rendered and verified | Block light sources, ambient occlusion, smooth lighting |

## Verification

```bash
mise run ci
```

Runs build, vet, race-enabled tests with the coverage floor, and lint — the
same four gates CI enforces on every push and pull request.

Coverage by package, at the close of M5 (total 35.5%, floor 35.0):

| Package | Coverage |
|---|---|
| `core` | 100% |
| `world/lighting` | 94.2% |
| `gfx/mesh` | 86.0% |
| `testutil` | 85.4% |
| `platform/config` | 78.6% |
| `blocks` | 58.7% |
| `ui/core` | 37.5% |
| `world` | 50.2% |
| `blocks/model`, `game`, `gfx`, `gfx/atlas`, `platform/logging`, `platform/logging/raylog`, `platform/resources`, `player`, `ui/game` | 0% |

(`archtest` reports no statements; it contains tests only.)

The remaining gaps are all raylib-facing packages that need a GPU to exercise.
`internal/game` at 0% is the most worthwhile of them to attack next.

## Dependencies

raylib (graphics), logrus (logging), yaml.v3 (config). Tool versions are pinned
in `mise.toml`, which is the single source of truth — including for CI.
