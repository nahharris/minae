# M20 — Density terrain

**Status:** 📋 Planned
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

## Risks worth naming now

**Lighting cost.** Every overhang is shaded geometry that light must propagate
under. This is the first milestone where terrain shape itself changes the main
thread's per-chunk cost. Constraint 6 applies with force.

**Generation cost.** Even interpolated, density is substantially more work
than a heightmap. Generation runs on workers, so it is throughput rather than
frame time — but chunks arriving slower is visible as a horizon that fills in
late.

## Validation criteria

1. **Reproduces M17 exactly with the 3D term off** — every block of every chunk
   in a large sample, several seeds.
2. **Continuous across cell boundaries and chunk seams**, sampled from global
   coordinates; distinct chunks differ.
3. **Deterministic and load-order independent.**
4. **No floating debris.** No solid component smaller than a threshold is
   disconnected from the ground. The failure mode of density terrain, asserted
   rather than eyeballed.
5. **Plains stay plains.** M17's slope bound still holds where erosion is high.
6. **Overhangs occur**, somewhere, at a bounded rate — the range lesson: a 3D
   term that never produces an overhang is an expensive no-op.
7. **`SurfaceHeight` agrees with the generated blocks** in every column.
8. **Lighting equivalence holds on overhang terrain**, with test terrain built
   so that shaded cells sit under an overhang lip that crosses a chunk seam.
9. **Codegen guard passes** — density is accumulation-heavy.
10. **Costs reported**, and the cell size justified by the numbers.

## Explicitly out of scope

Caves, floating islands, aquifers and water, and any biome influence on shape.
