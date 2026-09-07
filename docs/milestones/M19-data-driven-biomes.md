# M19 — Data-driven biomes

**Status:** 📋 Planned — sketch, not yet a spec
**Depends on:** [M17](M17-plains-terrain.md), [M18](M18-vegetation-features.md)

## Objective

Turn the single hardcoded plains generator into a biome system: a set of world
conditions selects a biome, and each biome declares its own terrain shape,
blocks and features in data rather than in code.

This is where the magi-tech identity finally lands, because this is the first
point at which it can be expressed as content instead of being welded into the
generator.

Deliberately a sketch. Writing a precise spec now would be guessing at
constraints that M14 through M18 will make obvious.

## The architecture the research points to

Minecraft's post-1.18 design is a close match for what was described, and worth
following rather than inventing: terrain is driven by a handful of independent
noise fields, each passed through a spline, and biomes are chosen by
**nearest-neighbour lookup in that multi-dimensional parameter space** rather
than by a chain of conditionals. See
[World generation](https://minecraft.wiki/w/World_generation) and
[Custom world generation](https://minecraft.wiki/w/Tutorial:Custom_world_generation).

Its axes are continentalness, erosion, peaks-and-valleys, weirdness, temperature
and humidity. The proposed set here:

| axis | drives |
|---|---|
| continentalness | ocean ↔ inland; base height |
| erosion | flat ↔ mountainous |
| peaks and valleys | local ridging |
| temperature | biome selection |
| humidity | biome selection |
| **mysticness** | biome selection, and how strange the world is allowed to be |

**Mysticness is the interesting one**, and it drops into this architecture as
simply one more axis. It is what makes the world get progressively stranger in
some regions rather than being uniformly magical — which is the difference
between a world with a fantasy setting and a world that is merely weird
everywhere.

## Verticality

Surface, underground and sky biomes cannot come from a 2D heightmap. This is
where terrain moves from a height function to a **3D density field**: solid
where density is positive, air where it is not. Caves are then a matter of
subtracting density rather than a separate carving pass, and floating islands
are islands of positive density with nothing beneath them.

[M17](M17-plains-terrain.md) deliberately keeps its interface from assuming a
heightmap for this reason.

## Data-driven, and how far

A biome definition should be able to declare, in YAML alongside the block
definitions:

- where it sits in the parameter space
- surface, filler and underwater blocks
- terrain shape modifiers — spline adjustments rather than arbitrary code
- features and their densities

The line worth holding: **data describes parameters, code implements
behaviours.** A biome can say "spruce trees at this density"; it cannot contain
a script. Once configuration becomes a programming language it is a worse
programming language than the one it is written in, and it stops being
testable.

## On restraint with the fantasy

Worth writing down before there is a framework tempting us to use all of it.

The world should read as a *place* first. Impossible things land harder when
they are rare: a single floating island on the horizon is remarkable, a sky full
of them is wallpaper. Mysticness as an axis is the mechanism for exactly that —
most of the world sits low on it and looks like a world, and the strange regions
are strange because they are the exception.

Concretely, that suggests fantastical features being gated behind high
mysticness rather than sprinkled everywhere, and the ordinary biomes staying
recognisably ordinary.

## Open questions for when this is specified

- How are biome boundaries blended? Hard edges between biomes look wrong, and
  the standard answers — interpolating surface blocks, blending height across a
  margin — each have costs.
- Do caves get their own biome axis, or are they a density subtraction that
  ignores surface biomes entirely?
- How are structures (as opposed to scattered features) placed, given they may
  span many chunks?
- Does mysticness affect gameplay — mob spawns, resource availability, magical
  effects — or only appearance? That decision reaches well beyond world
  generation.

## Explicitly out of scope for the first pass

Structures, villages, dungeons, ore distribution, weather, and anything that
requires the biome to affect behaviour rather than appearance.
