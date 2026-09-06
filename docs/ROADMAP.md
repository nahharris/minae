# Minae Roadmap

An index of milestones. Each links to its own document holding the goal, the
ordered steps, and how the milestone is verified. Steps are checked off in the
same commit that implements them.

Current focus: making the project verifiable, then fixing the lighting engine.
The design behind milestones 1–4 is
[2026-09-05 lighting and foundations](superpowers/specs/2026-09-05-lighting-and-foundations-design.md).

All four milestones are code-complete. M4 awaits visual sign-off — the shader is the one thing no test can judge.

| # | Milestone | Status | Goal |
|---|-----------|--------|------|
| [M1](milestones/M1-foundations.md) | Verifiable foundations | ✅ Done | CI, lint config, coverage ratchet, test harness, docs structure |
| [M2](milestones/M2-core-purity.md) | Purify the core | ✅ Done | Remove raylib from the simulation layer so lighting is testable without a GPU |
| [M3](milestones/M3-light-engine.md) | Rewrite the light engine | ✅ Done | Incremental skylight with removal, correct cross-chunk propagation, dirty tracking |
| [M4](milestones/M4-render-pipeline.md) | Get light onto the screen | 🚧 Awaiting visual sign-off | Vertex-packed light, face bias, `skyTint` day cycle |

Status legend: 📋 Planned · 🚧 In progress · ✅ Done

## Known defects, not yet scheduled

Found during M2 while converting types, and deliberately left alone so the
refactor stayed a pure type refactor. Recorded here so they are not lost.

- **`world/raycast.go` divides by zero on axis-aligned rays.** A `dir` component
  of exactly 0 produces `±Inf` deltas, and `NaN` when the ray start lands on a
  voxel boundary. `NaN` fails every comparison in the traversal loop, so the DDA
  silently takes a wrong branch. Not reachable from the current camera, which
  never produces an exactly-zero component — but unguarded.
- **`world/raycast.go` never normalizes `dir`**, despite the doc comment saying
  it does. `maxDist` is therefore measured in units of `|dir|` rather than world
  units, so `PlayerArmLength` would scale with the camera's target distance. It
  works today only because the camera target sits at radius 1.0.
- **`world/time.go` computes `SunIntensity` and throws it away.** `LerpColors`
  interpolates it, `GetLightingState` discards it with `_`, and nothing reads it.
  Dead weight across 7 initialisers. M4 touches this file and should remove it.
- **`world/time.go`'s `lerpColor` truncates rather than rounds**, and `t` is
  never clamped to `[0, 1]`. An out-of-range `t` would wrap a channel from 255 to
  0. Latent rather than live, since `getStateFromTime` currently keeps `t` in
  range. M4 replaces this with linear float maths.

## Backlog

Not scheduled, and deliberately excluded from M1–M4. Roughly in the order that
would make the game most playable soonest.

**Gameplay foundations**
- Player collision and gravity (currently noclip flight)
- Noise-based terrain generation (currently a flat plane at y=32)
- Infinite world: chunk streaming based on player position (currently a fixed 3×3 grid)
- World persistence: save and load chunks, player state and time to disk

**Lighting, once M3 and M4 land**
- Block light sources: torches, glowstone. The vertex `G` channel and the
  `blockTint` uniform are already reserved for this.
- Ambient occlusion. The vertex `B` channel is reserved for it.
- Smooth lighting: per-vertex rather than per-face light sampling
- A gentle directional sun tint layered over the baked light

**Rendering performance**
- Frustum culling
- Greedy meshing
- Background-thread mesh generation
- Level of detail for distant chunks

**Content**
- Transparent blocks (glass, leaves) — the vertex alpha channel is freed for this in M4
- Liquids, stairs, fences, doors
- Tool tiers, block hardness, item drops
- Entities, mobs, dropped items
- Audio
- Main menu, settings screen, inventory screen
- Multiplayer
