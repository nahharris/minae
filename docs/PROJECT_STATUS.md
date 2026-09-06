# Minae — Project Status

**Minae** is a voxel game engine written in Go on top of raylib.

Last reviewed: 2026-09-05, at the close of [M1](milestones/M1-foundations.md).
For what happens next, see the [roadmap](ROADMAP.md).

This document describes what is *actually true today*, including what is
broken. Aspirations belong in the roadmap.

## Layout

```
cmd/minae/          Entry point
internal/
  blocks/           Block definitions, registry, models (full, sided, slab, orientable)
  game/             Game coordinator and states
  gfx/              Scene renderer, texture atlas, chunk meshing
  platform/         Config, logging, resource loading
  player/           Camera and input
  testutil/         Synthetic world builder for tests
  ui/core/          UI framework (panels, buttons, labels, layout)
  ui/game/          HUD, pause menu, debug overlay
  world/            World, chunks, terrain, time, raycasting, interaction
  world/lighting/   Skylight propagation and shaders
```

16 packages, ~4,100 lines of non-test Go, ~680 lines of test.

## Known broken

**Lighting does not reach the screen.** The mesh builder writes skylight into
the vertex alpha channel; the fragment shader multiplies by it in a way that
cannot affect RGB. The entire flood-fill is computed and discarded. Separately,
the directional sun vector is horizontal at every hour, so top faces never
light up. Scheduled for [M3](milestones/M3-light-engine.md) and
[M4](milestones/M4-render-pipeline.md).

**Light cannot be removed.** `CalculateChunkLighting` only ever brightens.
Placing a block never darkens anything.

**Cross-chunk light propagation is unreliable.** `World.GetLight` returns 15
for unloaded chunks, which halts the BFS; and light written into a neighbouring
chunk never marks that chunk for re-meshing.

**The day cycle snaps.** `getStateFromTime` has no matching state for
`hour ∈ [0, 0.2)` and falls through to a terminal night state, so colour jumps
rather than interpolating.

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
| Lighting | Skylight only, and it does not render | Block light sources; see *Known broken* |

## Verification

```bash
mise run ci
```

Runs build, vet, race-enabled tests with the coverage floor, and lint — the
same four gates CI enforces on every push and pull request.

Coverage by package, at the close of M1 (total 21.3%, floor 21.0):

| Package | Coverage |
|---|---|
| `gfx/mesh` | 85.6% |
| `testutil` | 85.4% |
| `platform/config` | 78.6% |
| `blocks` | 58.7% |
| `ui/core` | 37.5% |
| `world` | 11.2% |
| `blocks/model`, `game`, `gfx`, `gfx/atlas`, `platform/logging`, `platform/resources`, `player`, `ui/game`, `world/lighting` | 0% |

`world/lighting` at 0% is the gap that matters most, and it is what
[M3](milestones/M3-light-engine.md) closes.

## Dependencies

raylib (graphics), logrus (logging), yaml.v3 (config). Tool versions are pinned
in `mise.toml`, which is the single source of truth — including for CI.
