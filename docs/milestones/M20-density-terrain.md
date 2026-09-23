# M20 — Density terrain

**Status:** ✅ Done
**Depends on:** [M19](M19-biome-selection.md)
**Inherits:** [world-generation constraints](../design/worldgen-constraints.md)

## Objective

Replace the 2D heightmap with a 3D density field: a cell is solid where density
is positive, air where it is not. The first visible result is overhangs and
more sculpted relief; the real point is that caves (M21) and floating islands
(M22) become edits to one field instead of separate carving passes.

## Design decisions

### Anchored to M17 before it may do anything new

Density starts as the heightmap restated:

```
density(x, y, z) = surfaceHeight(x, z) - y + detail(x, y, z)
```

With `detail` at zero this is solid exactly below M17's surface and air above
it, so the density generator must reproduce M17's terrain **block for block**
when the 3D term is switched off. That is the regression anchor, and it lands
before the 3D term is allowed any amplitude at all. A density generator that
cannot reproduce the heightmap it replaces has a bug in its interpolation or its
coordinates, and that is far easier to find against a known answer than against
new terrain nobody has seen.

### Sampled on a coarse grid and interpolated

Evaluating 3D noise at every block is 65,536 samples per chunk against the 256
the heightmap needs — two orders of magnitude, on a path that already doubled
in M18. Minecraft samples density on a coarse cell grid and interpolates
within each cell ([Density function](https://minecraft.wiki/w/Density_function)),
and caches values that only vary horizontally so they are computed once per
column.

The plan here: cells 4 blocks wide and 8 tall, trilinear interpolation between
corners, and the 2D climate values (continentalness, erosion, peaks) computed
once per column rather than per sample. That is roughly 5×33×5 corner samples
per chunk — a few hundred, not sixty-five thousand. The cell size is a tuning
constant, and the benchmark decides it rather than the analogy.

Interpolation corners on a chunk boundary must be computed from global
coordinates and shared exactly by both chunks, or every seam gets a crease.
That is constraint 2 in a new form.

### `SurfaceHeight` survives, redefined

Spawn placement, the M17 tests and M18's feature placement all ask "where is
the ground in this column?" without a chunk existing. With density, the answer
is the highest solid cell in the column — computable from the density function
alone, so it stays a pure function of `(x, z)`.

Overhangs make the question subtler: the highest solid cell may be the lip of
an overhang with air beneath. Spawning there is fine; rooting a tree on it is
fine; the definition stays "highest solid", and anything needing "ground you
can walk from the base of" asks for it explicitly.

### Overhangs are rare and structural

Unbounded 3D detail produces the classic failure of density terrain: floating
debris and single-block pillars that read as noise rather than as rock. The 3D
term's amplitude is small and scaled by erosion, so plains stay plains and
sculpted relief appears where the ground is already rough. A connectivity test
guards against debris directly (below).

## Refinements before implementation

Written after M19 shipped, on re-reading this spec against what M19 found. The
first one corrects a contradiction in the decisions above.

### Only the 3D term is interpolated

As originally written, this spec asked for two things that cannot both hold:
sample the whole density field on a 4-block grid and interpolate, *and*
reproduce the heightmap block for block with the 3D term off.

With `detail` at zero, density is `surfaceHeight(x, z) - y`. That is linear in
`y`, so interpolating vertically is exact. But `surfaceHeight` is not linear in
`x` and `z`, so interpolating it across 4-block cells smooths the heightmap
horizontally, and the anchor fails wherever the ground is not flat. The easy
"fix" would be loosening criterion 1 to a tolerance, which would throw away the
one exact answer this milestone has to check against.

The resolution is the structure Minecraft uses anyway: values that vary only
horizontally are computed once per column, and only genuinely 3D noise goes
through the cell grid.

```
density(x, y, z) = (surfaceHeight(x, z) - y) + interpolate(detail)(x, y, z)
```

The 2D base is exact per column and costs nothing extra, since the column
fields are already computed per column. Only `detail` is sampled at cell corners
and interpolated. With `detail` at zero, criterion 1 then holds exactly, not
approximately.

### One field, one path

M19 found height computed on two paths — one for generating chunks, one for
`SurfaceHeight` — and had to confirm that both stayed independent of the biome.
Density raises the stakes. If `Generate` fills blocks from the *interpolated*
field while `SurfaceHeight` evaluates `detail` directly at full resolution, the
two disagree exactly where `detail` is non-zero. Those are the overhang columns,
the only interesting ones.

`SurfaceHeight` must read the same interpolated field `Generate` does, from the
same corner samples. The agreement test (criterion 7) must deliberately sample
columns where `detail` is non-zero. A test over flat columns would pass against
this exact bug, because there both paths reduce to the heightmap.

### `SurfaceHeight` scans a bounded band

Feature placement calls `SurfaceHeight` for every candidate root, including
roots in neighbouring chunks, and spawn calls it too. A full 256-cell scan per
call would make feature placement the new bottleneck.

It does not need one. `detail` is bounded by its amplitude `A`, so density can
only differ from the heightmap's answer within `A` blocks of it. The highest
solid cell therefore lies in `[h - A, h + A]` of the heightmap height `h`, and
the scan covers that band only.

### Interpolation is FMA-shaped

A lerp, `a + t*(b - a)`, is a product feeding a sum. Since M19 the codegen guard
covers `internal/worldgen`, so a bare lerp will fail it. Route every
interpolation step through `addProduct`. It is noted here so it is designed in
rather than discovered by a red CI run.

### Before/after benchmarks are interleaved

Per [worldgen-constraints](../design/worldgen-constraints.md#6-every-milestone-reports-generation-and-lighting-cost):
compile both builds once and alternate them. M19 produced a phantom 14% lighting
regression twice by measuring in batches.

## Risks worth naming now

**Lighting cost.** Every overhang is shaded geometry that light must propagate
under. This is the first milestone where terrain shape itself changes the main
thread's per-chunk cost. Constraint 6 applies with force.

**Generation cost.** Even interpolated, density is substantially more work
than a heightmap. Generation runs on workers, so it is throughput rather than
frame time — but chunks arriving slower is visible as a horizon that fills in
late.

## Validation criteria

1. **Reproduces the current terrain exactly with the 3D term off** — M17's
   shape with M19's surfaces, every block of every chunk in a large sample,
   several seeds. Exact, not within a tolerance; see "Only the 3D term is
   interpolated".
2. **Continuous across cell boundaries and chunk seams**, sampled from global
   coordinates; distinct chunks differ.
3. **Deterministic and load-order independent.**
4. **No floating debris.** No solid component smaller than a threshold is
   disconnected from the ground. The failure mode of density terrain, asserted
   rather than eyeballed.
5. **Plains stay plains.** M17's slope bound still holds where erosion is high.
6. **Overhangs occur**, somewhere, at a bounded rate — the range lesson: a 3D
   term that never produces an overhang is an expensive no-op.
7. **`SurfaceHeight` agrees with the generated blocks** in every column, with
   the sample deliberately weighted toward columns where the 3D term is
   non-zero. Flat columns cannot catch a two-path disagreement.
8. **Lighting equivalence holds on overhang terrain**, with test terrain built
   so that shaded cells sit under an overhang lip that crosses a chunk seam.
9. **Codegen guard passes** — density is accumulation-heavy.
10. **Costs reported**, and the cell size justified by the numbers.

## Explicitly out of scope

Caves, floating islands, aquifers and water, and any biome influence on shape.

## Result

The heightmap is now a 3D density field: `(surfaceHeight − y) + detail`, with
only `detail` sampled on 4×8 cells and interpolated, as the refinement required.
With the 3D term switched off, generation reproduces the previous terrain block
for block across ten seeds, exactly rather than within a tolerance. A 3D noise
function joins `internal/noise`, with golden vectors and the same
cross-architecture discipline as the 2D one.

The field moves the surface in 62% of columns, by up to six blocks. What it
barely produces is overhangs, and that is the finding worth reading first.

### Overhangs: criterion 6 met in letter, not in spirit

Thirteen overhang columns in 655,360, or 0.002%. At view distance 8 that is
about one overhang in the whole loaded world. The test asserts "more than zero",
so it passes, which is exactly the outcome this spec warned about: a 3D term
that never produces an overhang is an expensive no-op.

The cause is structural, and it is not a tuning slip. The height term falls by
one per block of height, so an overhang needs `detail` to rise faster than
that somewhere. With detail capped at 7.5 blocks in the roughest terrain and
interpolated over 8-block-tall cells, that only happens in the extreme tail.

More fundamentally, **the world has no rough terrain for overhangs to form
on.** M17 built plains on purpose, with about twenty blocks of total relief, and
no milestone since has added mountains or cliffs. Overhangs belong on steep
ground; sprinkling them on rolling hills would read as noise, not as rock.

The known fix, and what Minecraft does, is a squash factor: scale the height
term by `s(x, z)` in `(0, 1]`, small where terrain is rough, so the detail can
overpower it there.

```
density = s(x, z) · (surfaceHeight − y) + detail
```

It keeps criterion 1 exact, because multiplying by a positive factor never
changes a sign, so with detail off the blocks are unchanged. The cost is that
`SurfaceHeight`'s band widens to `detailBand / s` in rough columns. It belongs
together with real relief, and how tall and how rough that relief should be is a
design question rather than an implementation one. It is recorded here instead
of being quietly bolted on.

### The loose noise bound shipped a second time

The implementation added 3D simplex noise scaled by the triangle-inequality
bound: four corners, each at its own maximum. That bound wastes a factor of
exactly four, because at the true maximum only one corner is anywhere near its
own peak. The field reached 0.2475 of its documented ±1.

This is M16's scale bug again, and constraint 4 of
[worldgen-constraints](../design/worldgen-constraints.md) exists because of it.
The implementation chose the loose bound knowingly, reasoning that "nothing
downstream needs Eval3D's own normalisation to be tight". That was wrong in two
ways that compounded:

- worldgen compensated with a detail amplitude of 50 instead of 12.5, and then
  derived its search band from that amplitude times the *documented* range:
  31 blocks each side where 9 suffice. That band was the dominant cost of
  generating a chunk.
- M21 will threshold this noise to carve caves, and a rule like "carve where
  noise exceeds 0.6" would never fire on a field that tops out at 0.25.

The tight bound was found numerically, exactly as M16 did for 2D: 0.0130072,
a hair above one corner's proved maximum. It is held in place by three tests:
one re-derives the bound, one cross-checks it against the proved one, and
`TestEval3DUsesMostOfItsRange` asserts the range is actually used. Restoring
the loose bound fails that last test with the original 0.2475. The amplitude
dropped to 12.5, reproducing the same terrain to within 0.008%.

On the kernel: 3D simplex with radius² 0.5 leaves about 8.5% of samples near
zero, and Gustavson's 0.6 would fill that in. It was left at 0.5 on purpose,
because 0.6 is known to introduce small discontinuities, and continuity is a
guarantee this package already makes.

### Redundant corner evaluation, fixed in review

Separately from the band, every corner recomputed its column's erosion factor,
a four-octave noise sum that depends only on the column. A chunk has 825
corners but 25 corner columns. Chunk generation also computed all 33 vertical
corner levels, though cells far from the surface never read them.

Corners are now split into a per-column scale and a per-point noise, so the
scale is computed once. The chunk cache fills corners lazily, and
`SurfaceHeight` walks its column once, reusing corners between cells. All three
paths feed identical values through the same interpolation in the same order,
and a corner-order mutation in the new column walk is caught by three tests.

| chunk generation | per chunk | vs before M20 |
|---|---|---|
| before M20 | 394 µs | — |
| as implemented | 947 µs | 2.4× |
| after review fixes | ~512 µs | 1.3× |

### Mutations re-run in review

| mutation | caught by |
|---|---|
| interpolate the height term across cells too | `TestGenerateReproducesHeightmapExactly` |
| `SurfaceHeight` reads detail at full resolution | four tests, including `TestSolidAtAgreesCachedAndUncached` |
| swap two corners in the new column walk | three layering and agreement tests |
| restore the loose 3D noise bound | `TestEval3DUsesMostOfItsRange` |

The lighting test for an overhang across a chunk seam follows the precondition
rule: it searches generated terrain for a real one and fails if none exists.

### Lighting

| | before | after |
|---|---|---|
| warm | 2.35 ms | 2.31 ms |
| cold | 3.05 ms | 3.31 ms |

Four interleaved rounds each. Cold lighting is slower in every round, so the
+8.5% is real and matches the risk this spec named: relief adds shaded
geometry. That leaves 0.7 ms of the 4 ms per-frame budget, and caves in M21 will
spend some of it.

### The test suite came within 14 seconds of Go's timeout

Under `-race -covermode=atomic`, `internal/worldgen` took 586 s against Go's
600 s default. GitHub's runners are slower than the development machine, so it
would almost certainly have failed there. `-race` makes atomic coverage
counters expensive, because the race detector instruments the atomics too, so
every hot loop in the package paid twice.

It is now 261 s, and no assertion was weakened. Two tests were checking a
property the long way:

- The overhang-rate test generated 2,560 whole chunks to count columns. It now
  asks the density field directly, and finds exactly the same count, 13 in
  655,360. That doubles as a check that the column walk agrees with generation
  on exactly the columns that matter.
- The overhang search behind several tests generated chunks until it hit an
  overhang, about one chunk in two hundred. It now locates the column through
  the field and generates only that chunk, still confirming the overhang in the
  real blocks.

The rest are sample sizes on deterministic property tests that were far
larger than their property needed. The biome-proportion test sampled every 40
blocks for climate fields with a 1,000-block wavelength; every 160 keeps the
same area. Several tests dropped seeds. The overhang test keeps seeds 0 to 4,
which hold eight overhangs between them. Four of the ten original seeds have
**none at all** in a 256×256-block area, which is the plainest possible
picture of how rare they are. The trimmed tree test still catches trees placed
two blocks above the ground instantly.

The CI command now passes `-timeout 20m`. That is a safety net against a hung
test, not a budget: the default has nearly failed CI twice, for world/lighting
in M18 and worldgen here, both on suites that were merely slow.

### A process note

The implementation again stopped to wait on a background command, despite a
brief that made running everything in the foreground mandatory. It was stopped
before review began, and it never finished CI or raised the coverage floor.
Worth knowing when weighing how much to delegate: the rule has now been broken
in three runs out of five.
