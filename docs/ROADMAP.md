# Minae Roadmap

An index of milestones. Each links to its own document holding the goal, the
ordered steps, and how the milestone is verified. Steps are checked off in the
same commit that implements them.

The design behind milestones 1-4 is
[2026-09-05 lighting and foundations](superpowers/specs/2026-09-05-lighting-and-foundations-design.md).

All milestones are complete and the known-defect list is empty. What comes next is in the backlog below.

| # | Milestone | Status | Goal |
|---|-----------|--------|------|
| [M1](milestones/M1-foundations.md) | Verifiable foundations | ✅ Done | CI, lint config, coverage ratchet, test harness, docs structure |
| [M2](milestones/M2-core-purity.md) | Purify the core | ✅ Done | Remove raylib from the simulation layer so lighting is testable without a GPU |
| [M3](milestones/M3-light-engine.md) | Rewrite the light engine | ✅ Done | Incremental skylight with removal, correct cross-chunk propagation, dirty tracking |
| [M4](milestones/M4-render-pipeline.md) | Get light onto the screen | ✅ Done | Vertex-packed light, face bias, `skyTint` day cycle |
| [M5](milestones/M5-known-defects.md) | Clear the known-defect list | ✅ Done | Fixed a raycast freeze on negative-zero directions; normalized `dir` so `maxDist` is in world units |

Status legend: 📋 Planned · 🚧 In progress · ✅ Done

## Known defects, not yet scheduled

None outstanding. See [M5](milestones/M5-known-defects.md).

The two `world/time.go` entries that used to sit here — dead `SunIntensity`, and
a `lerpColor` that truncated without clamping — were fixed by M4's rewrite of
that file, but were left listed here afterwards. Retired 2026-09-05.

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
