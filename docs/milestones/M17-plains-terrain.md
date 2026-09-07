# M17 — Plains terrain

**Status:** ✅ Done
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

## Design decisions

### `SurfaceHeight` is exported, and that is what makes this testable

The generator computes a height per column and then fills it. Exposing
`SurfaceHeight(x, z int) int` as a pure function of *global* coordinates
splits every interesting property in two, and both halves become easy:

- Criteria 3 and 6 — seams and slope — are properties of the height function
  alone, testable without generating a single chunk.
- Criterion 4 — layering — is a property of the fill, testable by asserting a
  generated chunk's columns agree with `SurfaceHeight` at the matching global
  coordinate.

Testing "are the chunks seamless" through generated chunks alone would work
but would say much less when it failed.

### This is where M16's criterion 7 finally has teeth

M16 recorded, honestly, that its chunk-boundary test could not observe a
periodicity bug inside `Eval2D`: both paths computed the same global
coordinate, so any pure function of it agreed with itself. The property it
actually protects is that *a caller builds global rather than chunk-local
coordinates*, and that caller did not exist yet.

It exists now. The single most likely bug in this milestone is a generator
that samples noise at `(localX, localZ)` instead of
`(chunkX*16 + localX, chunkZ*16 + localZ)` — which produces terrain that
looks entirely plausible in isolation and repeats identically in every chunk,
with a wall at every seam.

A test must fail specifically for that, not merely for terrain being
discontinuous in general. Assert that two different chunks are not identical,
as well as that their shared edge is continuous.

### Sea level is a reference height, not water

Water is out of scope and there is no water block registered. Sea level exists
here only as the datum the splines are shaped around, so that when oceans
arrive the land does not have to be re-tuned around a different zero. Nothing
in this milestone should place a water block, and nothing should special-case
being below sea level.

### Spawning on the surface is part of this milestone

The player spawns at a hardcoded `(0, 40, 0)`. That was fine against a flat
plane at y=32. Against generated terrain it means spawning inside a hill or
far above one, and it will be the first thing anyone sees.

The spawn Y must be derived from `SurfaceHeight` at the spawn column. This is
in scope precisely because it is invisible in unit tests and obvious on
launch — the same category as M15's player-hold.

Note the interaction with M15: the player is held until the chunk beneath them
reaches `Generated`, so a spawn placed correctly is not enough on its own if
the spawn column's chunk has not arrived. Placing the player from
`SurfaceHeight` — a pure function needing no chunk — sidesteps that entirely,
which is a second reason to prefer it over inspecting generated blocks.

### Parameters and splines now, even for one biome

Three noise fields — continentalness, erosion, peaks-and-valleys — each mapped
through a declared spline and combined into a height. For a single flat biome
this is more machinery than the output justifies, and it is still the right
call: M19 then adds axes to an existing structure rather than replacing a
single height noise with one.

The splines must be declared as control-point data, not arithmetic, since
being able to reshape terrain by moving control points is the entire reason
M16 built splines.

Keep the height computation behind that one function rather than building an
abstract density interface today. M19's underground and floating-island biomes
will need 3D density, but designing that interface before there is a second
implementation would be guessing; isolating the height computation means the
change lands in one place when it is actually understood.

### `FlatGenerator` stays

It is the fixture most of the existing chunk and lighting tests are built on,
and a test that wants predictable terrain should not have to reason about
noise. The new generator is added alongside it, not in place of it.

### Generation cost is worth measuring, not assuming

M14 and M15 chose their per-frame budgets against a generator that filled a
constant. This one evaluates three fBm fields per column, 256 columns per
chunk, and `math.FMA` is a software routine on amd64 without the hardware
feature — which M16 chose deliberately for determinism.

Benchmark a chunk generation and report the number. This is not a request to
optimise; it is a request to know, because "generation is cheap" is currently
an assumption two milestones are resting on.

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

## Result

`internal/worldgen` replaces the flat plane. Three splined noise fields —
continentalness, erosion, peaks-and-valleys — combine into a height per
column, filled as stone under two dirt under grass. `world_seed` in
`config.yaml`, default 1337. The package is pure and in the raylib guard;
`FlatGenerator` stays as the fixture the existing chunk and lighting tests are
built on. Coverage 62.5%, the package at 87.2%.

Measured shape, seed 1337: heights span 57–76 over a 1200-block square, with
the largest step between adjacent columns being a single block. Relief of
roughly ten to twenty blocks plays out over hundreds of blocks, which is what
"gently rolling" was meant to mean.

### The clamp was doing the work, and the test could not tell

`SurfaceHeight` clamps to [32, 128] as a backstop, and
`TestSurfaceHeightStaysInRange` asserted the result lands inside it. That
assertion is true whatever the formula does, because the clamp makes it true —
a generator whose noise drifted to a height of 5000 would pass while producing
a world pinned flat against a ceiling. Silent, and visually catastrophic.

Measured across 540,000 samples and six seeds, the clamp never fires at all.
So the range guarantee genuinely rests on the tuning, and nothing was testing
the tuning.

`rawSurfaceHeight` is now the unclamped computation, with
`TestRawSurfaceHeightNeedsNoClamp` asserting it stays in range *with margin*,
so a formula that merely grazes the limit fails while there is still room to
fix it. The demonstration that this matters: raising `peakAmplitude` from 4 to
90 leaves the original range test **passing** and fails the new one.

This is the same shape as M16's scale bug — a bound that is satisfied without
being meaningful — and it is worth naming as a pattern. "Output is within
range" is nearly always the weaker half of what a range criterion means.

### A concern that measurement dismissed

The three noise fields are seeded from `seed`, `seed+1` and `seed+2`, so
worlds from adjacent seeds share two of their three underlying fields. That
looked like the classic seed-folding defect M16's criterion 2 exists to catch.

It is not, and the measurement is unambiguous: mean absolute height difference
between seeds 1337 and 1338 is 4.80 blocks, and between 1337 and 918273645 it
is 4.59. Adjacent-seed worlds are no more similar than distant-seed ones,
because each field is sampled at a different frequency and through a different
spline, so sharing a raw field between two roles produces no visible
correspondence. The derivation stays as it is.

Recorded because the instinct to "fix" it was strong and would have been
churn.

### Walking, quantified

7% of adjacent column pairs differ in height, so a straight walk meets a step
roughly every 14 blocks. Physics step height is 0.6, so a one-block rise has
to be jumped rather than walked. That is ordinary for the genre, and it is the
number to have in hand when judging the manual "feels like ground, not a bumpy
floor" check rather than guessing at it afterwards.

### Generation cost

235 µs to generate one chunk, one allocation. At the pipeline's budget that is
around 2 ms of worker-pool CPU per frame's worth of generation — comfortably
inside budget. M14 and M15 chose their budgets against a generator that filled
a constant, so this is now known rather than assumed.

### No codegen guard here, for now

`internal/worldgen` has exactly one place where a product feeds a sum, and it
already routes through an explicit `math.FMA`. Everything else is a pure
product chain or delegates to `internal/noise`, which carries its own guard. A
second guard would check one call site visible by inspection. M19's 3D density
is the point to revisit.

Noted in passing, not acted on: `physics.Step` integrates position with
`b.Position.X += b.Velocity.X * dt`, which fuses on arm64 and not amd64. That
is harmless today — nothing claims physics is bit-reproducible, and there is
no replay or networking — but it would matter the moment either exists.
