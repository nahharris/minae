# Minae Roadmap

An index of milestones. Each links to its own document holding the goal, the
ordered steps, and how the milestone is verified. Steps are checked off in the
same commit that implements them.

The design behind milestones 1-4 is
[2026-09-05 lighting and foundations](superpowers/specs/2026-09-05-lighting-and-foundations-design.md).

The lighting backlog is finished (M1-M7 done, M8 skipped). M12 landed the physics
core and M13 wired it to the player. Next is the world itself: M14-M19 cover
streaming, generation and biomes.

| # | Milestone | Status | Goal |
|---|-----------|--------|------|
| [M1](milestones/M1-foundations.md) | Verifiable foundations | ✅ Done | CI, lint config, coverage ratchet, test harness, docs structure |
| [M2](milestones/M2-core-purity.md) | Purify the core | ✅ Done | Remove raylib from the simulation layer so lighting is testable without a GPU |
| [M3](milestones/M3-light-engine.md) | Rewrite the light engine | ✅ Done | Incremental skylight with removal, correct cross-chunk propagation, dirty tracking |
| [M4](milestones/M4-render-pipeline.md) | Get light onto the screen | ✅ Done | Vertex-packed light, face bias, `skyTint` day cycle |
| [M5](milestones/M5-known-defects.md) | Clear the known-defect list | ✅ Done | Fixed a raycast freeze on negative-zero directions; normalized `dir` so `maxDist` is in world units |
| [M6](milestones/M6-block-light.md) | Block light sources | ✅ Done | Glowstone emits; one BFS parameterized over two light channels |
| [M7](milestones/M7-smooth-lighting.md) | Smooth lighting and ambient occlusion | ✅ Done | Per-vertex light sampling and corner darkening |
| [M8](milestones/M8-sun-tint.md) | Gentle directional sun tint | ⏭️ Skipped | Declined: reintroduces the bug class M4 fixed, for an effect nobody asked to see |
| [M12](milestones/M12-collision-core.md) | Collision and gravity core | ✅ Done | Pure, GPU-free AABB resolver: gravity, jump, 0.6 step-up |
| [M13](milestones/M13-player-controller.md) | Player controller | ✅ Done | Give the player a body; walking by default, flight as a toggle |
| [M14](milestones/M14-async-chunk-pipeline.md) | Async chunk pipeline | 📋 Planned | Generation and meshing off the main thread, behind a stage pipeline |
| [M15](milestones/M15-chunk-streaming.md) | Chunk streaming | 📋 Planned | The world follows the player; the fixed 3×3 grid goes away |
| [M16](milestones/M16-noise-foundation.md) | Noise foundation | 📋 Planned | Deterministic OpenSimplex2, fBm, domain warping, splines |
| [M17](milestones/M17-plains-terrain.md) | Plains terrain | 📋 Planned | Real ground: continentalness, erosion, peaks-and-valleys through splines |
| [M18](milestones/M18-vegetation-features.md) | Trees and bushes | 📋 Planned | The first features, and the first transparent block |
| [M19](milestones/M19-data-driven-biomes.md) | Data-driven biomes | 📋 Planned | Multi-noise parameter space including mysticness; 3D density |

Status legend: 📋 Planned · 🚧 In progress · ✅ Done · ⏭️ Skipped

## Known defects, not yet scheduled

None outstanding. See [M5](milestones/M5-known-defects.md).

The two `world/time.go` entries that used to sit here — dead `SunIntensity`, and
a `lerpColor` that truncated without clamping — were fixed by M4's rewrite of
that file, but were left listed here afterwards. Retired 2026-09-05.

## Backlog

Not scheduled, and deliberately excluded from M1–M4. Roughly in the order that
would make the game most playable soonest.

**Gameplay foundations**
- Player collision and gravity: done in [M12](milestones/M12-collision-core.md) and
  [M13](milestones/M13-player-controller.md). Sprinting, crouching and fall damage
  are still open, as is a general entity system.
- Noise-based terrain generation: planned as [M16](milestones/M16-noise-foundation.md),
  [M17](milestones/M17-plains-terrain.md) and [M18](milestones/M18-vegetation-features.md).
- Chunk streaming: planned as [M14](milestones/M14-async-chunk-pipeline.md) and
  [M15](milestones/M15-chunk-streaming.md).
- World persistence: save and load chunks, player state and time to disk

**Lighting**
- Block light sources: done in [M6](milestones/M6-block-light.md). A torch with a
  proper thin cross-shaped model is still outstanding; glowstone is a full cube.
- Ambient occlusion and smooth lighting: done in [M7](milestones/M7-smooth-lighting.md).
- A gentle directional sun tint: skipped, see [M8](milestones/M8-sun-tint.md).
- Banding on large untextured surfaces is inherent to per-vertex lighting at
  one-block resolution; see the note at the end of [M7](milestones/M7-smooth-lighting.md).
  Dithering in the fragment shader would hide most of it. Not scheduled.
- AO strength is tunable in two places: `aoRamp` in the mesh builder (contrast)
  and `blockAOStrength` in the fragment shader (how much AO applies to torch
  light). Both are worth revisiting once real textures exist.

**Rendering performance**
- Frustum culling
- Greedy meshing
- Background-thread mesh generation
- Level of detail for distant chunks

**Content**
- Multiple biomes, caves, oceans and floating islands: sketched in
  [M19](milestones/M19-data-driven-biomes.md), which is where the magi-tech identity
  is meant to land.
- Transparent blocks (glass, leaves) — the vertex alpha channel is freed for this in M4
- Liquids, stairs, fences, doors
- Tool tiers, block hardness, item drops
- Entities, mobs, dropped items
- Audio
- Main menu, settings screen, inventory screen
- Multiplayer
