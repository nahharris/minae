# Design note — constraints every world-generation milestone inherits

**Applies to:** [M19](../milestones/M19-biome-selection.md) through
[M22](../milestones/M22-mysticness.md), and anything after them that generates
terrain.

M14 through M18 each paid for one of these. They are collected here so the
remaining milestones reference one list instead of rediscovering it, and so a
spec that breaks one has to say so explicitly.

## 1. Generation is a pure function of `(seed, chunk coordinate)`

No world access, no dependence on which chunks exist or the order they were
generated in. [M14](../milestones/M14-async-chunk-pipeline.md) runs generation on
a worker pool precisely because it is pure; anything that reads a neighbour
reintroduces locking or makes the world depend on thread scheduling.
[M18](../milestones/M18-vegetation-features.md) kept features pure by having each
chunk *recompute* what its neighbours would place, rather than asking them.

Every milestone needs a test that generates a chunk in isolation and after its
neighbours, and asserts the results are byte-identical.

## 2. Every sampler takes global coordinates

The most likely bug in any generator is sampling noise at chunk-local
coordinates: the result looks plausible in isolation and repeats identically in
every chunk. [M17](../milestones/M17-plains-terrain.md) established the test —
assert both that a shared chunk edge is continuous *and* that two different
chunks are not identical. Continuity alone passes against the bug, because
identical chunks have identical edges.

## 3. Floating-point accumulation must be identical on every architecture

Go fuses `a*b + c` into a single instruction on arm64 and not on amd64, and
`float64(a*b) + c` does **not** prevent it — the conversion is a no-op the
compiler drops. [M16](../milestones/M16-noise-foundation.md) found both the
hard way. Every product that feeds a sum goes through `math.FMA` (`noise.madd`,
`noise.dot2`, `worldgen.addProduct`), so it fuses identically everywhere.

`internal/noise` enforces this by compiling itself for arm64 and reading the
instructions. `internal/worldgen` does not yet, because M17 had a single such
site. **3D density changes that** — it is accumulation-heavy — so M20 must
extend the codegen guard to `internal/worldgen` before adding any.

## 4. A range criterion has two halves

"Output stays within its documented range" is satisfied by a function that
returns zero everywhere. M16's noise reached only ±0.365 of a documented ±1;
M17's height formula was never checked because a clamp made the range test
true regardless. Both passed every test.

So any criterion about a range, a distribution or a proportion must also assert
the range is *used*: values reach near its edges, every declared biome actually
occurs, a feature appears at roughly its declared density. A biome that never
appears is the same defect as a noise field using a third of its range.

## 5. Test terrain must have the shape the bug needs

Three milestones running, the weak part of a test was its terrain rather than
its assertions:

- M15's lighting equivalence needed an overhang to catch a wrong-chunk bug;
  flat fixtures caught nothing.
- M17 needed non-uniform heights.
- M18's tie between the light predicate and the sky ceiling was reported as
  tested and was not: the test's canopy sat *below* the ceiling it was meant to
  strain.

Before writing a spatial test, ask what geometry the bug requires to show up,
and build exactly that — computed from the terrain rather than hardcoded, so
retuning the generator cannot quietly move the case out of range.

## 6. Every milestone reports generation *and* lighting cost

M14 and M15 chose their per-frame budgets against a generator that filled a
constant. M17 benchmarked generation and not lighting, and lighting turned out
to cost 103 ms per chunk on the main thread — the freeze that
[M15's follow-up](../milestones/M15-chunk-streaming.md#follow-up-streaming-was-too-slow-to-play)
fixed. M18's transparent leaves then raised lighting cost by 17–44%, because
light now flows *through* canopies.

Terrain shape changes lighting cost directly: overhangs, caves and floating
islands all add shaded geometry that light has to propagate around. Each
milestone reports, before and after:

- `BenchmarkGenerateChunk` (worker pool; currently ~516 µs)
- `BenchmarkSeedChunk` and `BenchmarkSeedChunkColdNeighbours` (main thread;
  currently ~3.1 and ~3.3 ms, against a 4 ms per-frame light budget)

A regression is not automatically a blocker, but it is never a surprise.

**Measure before and after interleaved, not in batches.** Compile both builds
once, then alternate them for several rounds so background load lands on both
equally. M19's review produced a plausible 14% lighting regression twice over —
once in the implementation's report, once in review — by running each build in
its own batch while machine load drifted. Interleaved, the difference vanished.
A regression reported from batch runs is a hypothesis, not a finding.

## 7. The light predicate and the sky ceiling stay one predicate

`Chunk.highestSolidY` and the light engine both derive "opaque" from
`blocks.OpaqueToLight`. The sky-ceiling optimisation skips enqueueing cells
above the neighbourhood's highest opaque block on the argument that everything
up there is open sky at full brightness. Any new block type must answer
`LightTransparent` deliberately, and any terrain feature that puts opaque
geometry high in the world — floating islands, above all — puts that
optimisation under strain. See the risk recorded in M22.

## 8. Mutation-test every criterion that can be mutation-tested

A test never observed to fail proves nothing. Every milestone's report carries a
table of mutations applied and whether each was caught, and a criterion that
cannot be made observable is said to be unobservable rather than claimed as
covered. That has been right every time it was invoked.
