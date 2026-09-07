# M16 — Noise foundation

**Status:** 📋 Planned

## Objective

A pure, deterministic noise library: the arithmetic every later generator is
built from. No terrain yet.

## Why write it rather than import it

Two reasons, and neither is "not invented here".

**Determinism is a product requirement.** A seed must produce the same world on
every machine and every build, forever. A dependency can change its gradient
tables or its hashing in a minor release and silently reshape every existing
world. Owning ~200 lines removes that risk entirely.

**It belongs in the pure core.** `internal/noise` importing only the standard
library keeps it inside the boundary M2 drew and the purity guard enforces, so
it is property-testable with no GPU and no world.

## What to implement

**OpenSimplex2, not Perlin.** Classic Perlin is built on a square grid and its
artefacts are axis-aligned — visible in terrain as faint north-south and
east-west structure that reads as unnatural once you know to look for it.
OpenSimplex2 suppresses that directional bias for a small performance cost that
does not matter at chunk-generation rates. See
[The Perlin Problem](https://noiseposti.ng/posts/2022-01-16-The-Perlin-Problem-Moving-Past-Square-Noise.html).

On top of the base noise:

- **fBm / octaves** — summed layers at doubling frequency and halving amplitude.
  The standard way to turn one smooth field into something with detail at
  several scales.
- **Domain warping** — offsetting sample coordinates by another noise field.
  This is what turns visibly noise-shaped terrain into something that looks
  eroded, and it is the cheapest single technique for making generated land
  stop looking generated.
- **Splines** — a monotone piecewise curve mapping one value to another. This
  is the piece that makes Minecraft-style terrain control work: rather than
  writing arithmetic that turns a noise value into a height, you draw a curve.
  It is what lets a designer say "this range of continentalness is coastline,
  this range is inland plateau" without touching the generator.

Ridged and billowed variants can wait until something needs them.

## Validation criteria

1. **Determinism.** The same seed and coordinate always produce the same value,
   across runs and independent of evaluation order. This is the criterion the
   whole idea of a world seed rests on.
2. **Seeds are independent.** Different seeds produce uncorrelated fields;
   nearby seeds do not produce nearly-identical worlds, which is the classic
   symptom of folding the seed in badly.
3. **Range is bounded.** Output stays within its documented range for a large
   random sample — including fBm, where naive octave summing overflows the
   expected range unless normalised.
4. **Continuity.** Adjacent samples differ by less than a bound; the field has
   no discontinuities. A seam here becomes a visible cliff in terrain.
5. **No axis-aligned bias.** Sample a large grid and compare variance along the
   axes against variance along the diagonals. This is the specific defect
   OpenSimplex2 exists to avoid, so it is worth asserting rather than assuming.
6. **Splines are monotone** where declared, and pass exactly through their
   control points.
7. **Chunk-boundary agreement.** Sampling a coordinate directly and sampling it
   as part of a neighbouring chunk's range give identical results. Generation is
   per-chunk; a mismatch here produces a visible wall at every chunk seam.

Criterion 7 is cheap to write and catches an entire category of terrain bug.

## Explicitly out of scope

Terrain, biomes, caves, features, and any use of the noise. This milestone
produces a library and its tests, nothing visible.
