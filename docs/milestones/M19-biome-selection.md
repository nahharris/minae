# M19 — Biome selection

**Status:** ✅ Done
**Depends on:** [M17](M17-plains-terrain.md), [M18](M18-vegetation-features.md)
**Inherits:** [world-generation constraints](../design/worldgen-constraints.md)

## Where this fits

The original M19 was a sketch covering biomes, 3D density, caves, floating
islands and a data format in one milestone, and it said plainly that a precise
spec would have to wait for M14–M18 to expose the constraints. They have. It is
now four milestones, each independently shippable and each small enough to
review properly:

| | milestone | what it adds |
|---|---|---|
| **M19** | **Biome selection** | climate axes, data-driven biomes, per-biome surfaces and features |
| [M20](M20-density-terrain.md) | Density terrain | the heightmap becomes a 3D density field; overhangs |
| [M21](M21-caves.md) | Caves | noise caves carved as density subtraction |
| [M22](M22-mysticness.md) | Mysticness | the first magi-tech biome, and floating islands |

The order is deliberate. Biome selection goes first because it changes *what*
the ground is made of without changing its *shape*, so it is testable against
M17's terrain unchanged. Density comes next and is anchored to reproduce M17
exactly before it is allowed to do anything new. Caves and islands are both
density edits, so they come after density exists. Mysticness goes last because
it is the one milestone whose content is mostly taste, and taste is easier to
judge once the machinery underneath is trusted.

## Objective

Several biomes instead of one, chosen by where a column sits in a climate
parameter space, with each biome's surface, filler and features declared as
data. Terrain shape does not change.

## The finding that reshapes this milestone

The sketch planned for biomes to carry "terrain shape modifiers — spline
adjustments". That would have been a mistake, and the research is unambiguous
about why.

In Minecraft's post-1.18 generator, the climate parameters that choose a biome
"do not affect terrain shape, as terrain generation is defined in
final_density" ([Noise router](https://minecraft.wiki/w/Noise_router)). The
same continentalness, erosion and ridge noises feed *both* the terrain and the
biome choice, but the terrain is computed from the noise directly — never from
the biome the noise happened to select.

That one decision makes the hardest problem in the sketch disappear. The
sketch's first open question was how to blend terrain across biome boundaries,
because a per-biome height function steps wherever the biome changes. If
terrain never depends on the biome, there is no step to blend: the ground is
continuous because the noise is continuous, and a biome boundary is only ever
a change of surface block.

Mountains are therefore not a biome that raises the ground. They are a region
where erosion is low, which raises the ground *and*, separately, selects a
mountain biome. The biome describes what is there; it does not put it there.

## Design decisions

### The parameter space

Six axes, all 2D fields evaluated once per column:

| axis | status | drives |
|---|---|---|
| continentalness | exists (M17) | terrain height, biome |
| erosion | exists (M17) | terrain flatness, biome |
| peaks and valleys | exists (M17) | local ridging, biome |
| temperature | **new** | biome only |
| humidity | **new** | biome only |
| mysticness | **new** | biome only in M19; terrain from M22 |

Mysticness is added now, as an ordinary axis, even though nothing uses it until
M22. Adding an axis later would reshuffle every biome's nearest neighbour and
change every existing world; adding it now, while all biomes sit at low
mysticness, costs nothing and keeps worlds stable across M22.

The three new fields are derived from the world seed the same way M17's are.
M17 measured that deriving fields from `seed`, `seed+1`, `seed+2` produces no
correlation between adjacent-seed worlds, and that measurement carries over.

### Selection is nearest-neighbour, with deterministic ties

Each biome declares a point in the six-dimensional space. A column takes the
biome whose point is nearest — the Minecraft approach, and far easier to reason
about than a chain of conditionals, because adding a biome never requires
editing the others.

Ties must be broken by biome ID, not by declaration order or map iteration
order. Go randomises map iteration deliberately; a tie resolved by it would make
the same seed produce different worlds on different runs, and it would only
happen on the rare exact-tie columns, which is the worst kind of bug to chase.

Weighting per axis is allowed — temperature and humidity should dominate the
choice between ordinary biomes — but weights are part of the data, not the
code.

### Biomes are data, embedded in the binary

Biome definitions live in YAML, following the precedent `blocks.Load` already
sets for blocks. The vanilla set is embedded with `go:embed`, so tests and a
fresh install have the same biomes without depending on the player's data
folder.

The line from the sketch stands: **data describes parameters, code implements
behaviours.** A biome can say "surface is sand, trees at this density, at this
point in climate space". It cannot contain logic. Once configuration becomes a
programming language it is a worse one than the language it is written in, and
it stops being testable.

A definition is validated when loaded, and invalid data is a startup error with
a message that names the file and the field — not a silently wrong world.
Unknown block names, duplicate biome IDs, two biomes at the identical point,
missing axes and out-of-range values are all rejected.

### The first biomes are ordinary on purpose

Three, all recognisably real:

| biome | climate | surface | features |
|---|---|---|---|
| plains | temperate, middling humidity | grass | sparse bushes, rare trees |
| forest | temperate, humid | grass | dense trees |
| dunes | hot, dry | sand over sandstone | none |

Dunes need two new blocks, sand and sandstone. That is deliberate: a biome whose
surface differs visibly from its neighbours is the only way to *see* that
selection works, and the ordinary biomes should stay ordinary so that M22's
strange ones land as strange. No mystic biome exists yet; every biome sits low
on mysticness.

### Features belong to the biome at their root

M18's features move from the generator into biome definitions. A tree is placed
if the biome *at its root column* allows trees, and it may then grow across a
biome boundary — a forest's edge trees overhanging the plains beside it is
exactly what a real forest edge looks like.

The global feature radius stays the maximum over every feature in every biome.
M18's radius test must now run across all biomes, since a biome with a larger
feature silently widens the region every chunk has to evaluate.

### Surface boundaries are hard, and that is accepted

Blending terrain across biomes is no longer needed. Blending *surface blocks* is
a separate, smaller question, and the answer for this milestone is not to: grass
meets sand along a line. The line follows noise contours, so it is irregular
rather than chunk-aligned, and Minecraft's own borders are hard in the same way.
If it reads badly in play, dithering the boundary is a contained follow-up.

### The FMA guard reaches worldgen here

M17 decided `internal/worldgen` did not need its own codegen guard because it
had a single accumulation site. Biome distance is a sum of weighted squares —
exactly the shape that fuses on arm64 — and a biome choice that differs across
architectures means a different world on a different machine. The guard from
`internal/noise` extends to `internal/worldgen` in this milestone, ahead of the
much heavier accumulation M20 brings.

## Validation criteria

1. **Terrain shape is unchanged by biomes.** `SurfaceHeight` returns the same
   value at every column whatever the biome definitions say. Test it by
   generating with the real biome set and with a deliberately absurd one, and
   asserting identical heights. This is the architectural decision above,
   written as an assertion.
2. **Selection is nearest-neighbour.** Checked against a brute-force reference
   over a large sample, including constructed exact ties resolved by ID.
3. **Deterministic and load-order independent** — per the
   [constraints](../design/worldgen-constraints.md#1-generation-is-a-pure-function-of-seed-chunk-coordinate).
4. **Every biome occurs, in plausible proportion.** Over a large sample of
   columns and several seeds, each declared biome covers a meaningful fraction
   of the world, none dominates absurdly. The range lesson applied to biomes.
5. **Biome regions are coherent.** A world that alternates biome every few
   columns reads as noise, not as a place. Assert a bound on how often
   horizontally adjacent columns disagree, and a minimum typical region size.
6. **Surfaces and features follow the biome.** Every column's surface and
   filler match its biome; features appear only where their root biome allows
   them, at roughly their declared density.
7. **Chunk seams are continuous in both terrain and biome**, and distinct
   chunks differ — the M17 pair of assertions.
8. **Invalid biome data is rejected at load** with an error naming the file and
   field, for each class of error listed above.
9. **No unintended fused multiply-add in `internal/worldgen`**, enforced by the
   codegen guard with its own control test.
10. **Costs reported**: generation and lighting benchmarks before and after.

## Manual verification

- [x] Biome regions read as places — a forest, a stretch of plains, a desert —
      rather than as a patchwork. Confirmed 2026-09-23: "it's looking pretty dope".
      The two items below were not separately confirmed, so they stay open.
- [ ] Forest edges look like forest edges.
- [ ] Walking from one biome into another feels like arriving somewhere.

## Explicitly out of scope

Any change to terrain shape, caves, 3D density, a mystic biome, biome-tinted
grass or foliage colours, surface blending, water, and anything where a biome
affects behaviour rather than appearance.

## Result

Three biomes — plains, forest, dunes — chosen by weighted nearest-neighbour
selection across six climate axes, defined in embedded YAML and validated at
load. Terrain shape is untouched. Sand and sandstone are registered for dunes.
Coverage 66.5%, `internal/worldgen` at 93%.

Every design decision survived implementation as written.

### The new guard's first catch was in M18's code

Extending the FMA codegen guard to `internal/worldgen` failed immediately, and
not on anything this milestone wrote. M18's tree spacing check, `tooClose`,
computed `dx*dx + dz*dz` bare — the exact shape arm64 fuses and amd64 does not.

So since M18, **the same seed could place trees differently on Apple Silicon
than on x86.** Only near the spacing threshold, and only for some candidates,
which is precisely the kind of divergence nobody would notice until two players
compared screenshots. It now routes through `addProduct`.

This is constraint 3 working as intended, and it is the argument for the guard
over the comment. M17 decided the package needed no guard because it had a
single accumulation site "visible by inspection"; M18 then added a second one
that nobody inspected. A rule that depends on remembering to look is a rule
that eventually is not followed.

Side effect worth knowing: fusing that expression changes its rounding on amd64
too, so a few tree placements near the spacing threshold may shift for existing
seeds. There is no world persistence yet, so no saved world is affected.

### Criterion 1 is guarded on both height paths

Generation now computes height and climate together (`heightAndClimate`) to
share the three terrain samples between them, while spawn and the tests call
`SurfaceHeight`. That is two paths to the same number, and biome-dependent
terrain could creep into either.

The implementation's mutation targeted `SurfaceHeight`. Review repeated it on
the path that actually fills chunks — a height offset applied only in forest
columns — and it is caught by `TestSurfaceAndFillerFollowBiome`, which checks
generated blocks against `SurfaceHeight`. So the guarantee holds on both paths,
by agreement rather than by criterion 1's own test, which is adequate.

### A lighting regression that was not there

The implementation reported cold-neighbour lighting slowing 14%, from 3.35 to
3.81 ms — uncomfortably close to the 4 ms per-frame budget — and attributed it
to generation cost leaking into the benchmark. That attribution cannot be
right, since the benchmark stops its timer while generating.

Checking the actual input made it stranger: the benchmark region now holds 50
leaf blocks instead of 439, because biome selection placed it in plains with
sparse bushes rather than trees. The lighting code is unchanged and its input
got *simpler*. A slowdown would need an explanation that did not exist.

Measured properly — both builds compiled once, then run alternately for four
rounds so machine load hits both equally — there is no regression:

| | before | after |
|---|---|---|
| warm | 2.33 ms | 2.43 ms |
| cold | 3.15 ms | 3.15 ms |

The "after" build wins three of the four cold rounds. The reported 14%, and a
first review measurement that seemed to confirm it, both came from running each
build in a separate batch while background load drifted. **Before/after
benchmarks in this project should be interleaved from now on** — a batch-by-batch
comparison produced a plausible-looking regression twice over.

### What did cost something

Generation went from 274 µs to 367 µs per chunk, +34%. Three more fBm fields
per column is a genuine cost of six-axis climate; sharing the terrain samples
between height and biome selection brought a naive +67% down to that. It runs on
the worker pool, so it shows up as chunks arriving slightly later, not as frame
time.

### Biomes are large, perhaps too large to find

Climate fields use a wavelength of 1,000 blocks so regions read as places, and
the coherence criterion is comfortably met: adjacent columns disagree under 2%
of the time. The flip side is that dunes, the rarest biome, can sit 700 or more
blocks from spawn on some seeds.

That is a play question, not a test one, and it is on the manual checklist
rather than decided here. If deserts feel like a rumour, the climate frequency
is a single constant.

### One criterion is tested only synthetically

Dropping the mysticness axis from the distance changes nothing for the vanilla
set, because all three biomes sit at the same low mysticness with the same
weight. The mutation is caught by the randomised biome-set test instead, not by
the vanilla one — correct, and expected until M22 gives mysticness something to
separate.
