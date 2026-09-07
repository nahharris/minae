# M17 — Plains terrain

**Status:** 📋 Planned
**Depends on:** [M16](M16-noise-foundation.md)

## Objective

Replace the flat plane with generated plains: gently rolling terrain, stone
under dirt under grass, a sea level. One biome, no caves, no features, and — per
the decision recorded below — no fantasy yet.

## The decision to keep it plain

The magi-tech identity is deliberately absent from this pass. The generator gets
honest and boring first; the fantasy arrives once the biome framework in
[M19](M19-data-driven-biomes.md) exists to hold it data-drivenly, rather than
being hardcoded here and then unpicked.

That is worth stating because the first thing anyone sees will look generic, and
that is the intent rather than an oversight.

## Shape of the generator

Even for one biome, the terrain is worth driving through the parameter-and-spline
structure the later biome work needs, rather than a single height noise. It
costs almost nothing now and means M19 adds axes rather than replacing the
generator.

Minecraft's post-1.18 approach is the reference: sample a handful of independent
noise fields, run each through a spline, and combine them into a height (later,
a density). The fields worth having from the start:

- **Continentalness** — how far inland. Drives the base height and, later,
  oceans. Even with one biome it is what gives large-scale structure rather than
  uniform bumpiness.
- **Erosion** — high erosion flattens. This is what makes plains read as plains
  instead of as small hills everywhere.
- **Peaks and valleys** — local ridging, mostly suppressed here, present so the
  machinery exists.

See the [Minecraft world generation reference](https://minecraft.wiki/w/World_generation).

**Height, not density, for now.** A 2D heightmap cannot produce overhangs or
caves, and 3D density can. Caves are explicitly out of scope and overhangs are
not wanted in plains, so a heightmap is the simpler correct choice here — but
the interface should not assume it, because M19's underground and floating-island
biomes will need density.

## Generation must be a pure function

`generate(seed, chunkCoord) -> blocks`, with no access to the world and no
dependence on which chunks already exist.

This is not a style preference. [M14](M14-async-chunk-pipeline.md) runs
generation on a worker pool precisely because it is pure, and any dependence on
neighbouring chunks would either reintroduce locking or make the result depend
on load order. It is also what makes criterion 2 below testable at all.

## Validation criteria

1. **Deterministic.** The same seed and coordinate always produce the same
   blocks.
2. **Independent of load order.** Generating a chunk in isolation and generating
   it after its neighbours give identical results. This is what makes it safe to
   run on the pipeline.
3. **Seamless across chunk boundaries.** Height is continuous across every seam:
   sample the columns either side of a boundary and assert the step is within
   what the terrain's own slope allows. A visible wall at every chunk edge is
   the classic failure and it is trivially detectable.
4. **Layering is correct.** Grass on top, dirt beneath it, stone below that,
   everywhere, at every height.
5. **Height stays in range** — never below bedrock, never above the chunk
   height, for a large sample of coordinates and seeds.
6. **Plains are actually flat-ish.** Assert a bound on slope over a large
   sample. Without this, "plains" is an unverified claim and the generator can
   drift into hills unnoticed.
7. **Different seeds give different worlds**, and the same seed on two runs does
   not.

## Manual verification

- [ ] The horizon reads as a landscape rather than as noise.
- [ ] No visible seams or walls at chunk boundaries.
- [ ] Walking across it feels like ground, not like a bumpy floor — worth
      checking on foot now that M13 exists, since flying hides slope entirely.

## Explicitly out of scope

Caves, ores, water and oceans, trees and vegetation, multiple biomes, and
anything fantastical.
