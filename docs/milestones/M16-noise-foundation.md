# M16 — Noise foundation

**Status:** ✅ Done

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

## Design decisions

Made before implementation. The first one is the reason this milestone is not
simply "write some noise functions".

### Floating-point fusion breaks determinism across architectures

"The same seed produces the same world on every machine, forever" is stated as
criterion 1, and Go does not give it to us for free.

The Go spec permits an implementation to fuse `x*y + z` into a single FMA
operation, skipping the intermediate rounding and producing a *different*
result from doing the two operations separately. This is not hypothetical:
Go emits FMA on arm64, ppc64, s390x and riscv64, and does **not** on amd64. So
the identical source, the identical seed and the identical coordinate can
produce different noise on an Apple Silicon Mac than on an x86 Linux box —
which means different terrain, at the same seed, on two players' machines.

Every accumulation in the noise code must therefore force the intermediate
rounding with an explicit conversion — `float64(a*b) + c` — which the spec
guarantees prevents fusion. This is ugly, it is load-bearing, and it must be
commented at each site or someone will tidy it away.

**CI gets an arm64 job.** This is the only thing that actually verifies the
claim rather than asserting it: the same golden vectors, checked on an
architecture that fuses and one that does not. The repository is public, so
`ubuntu-24.04-arm` runners are free. Without this, the guarantee is a comment.

### Golden vectors are what "forever" actually means

We are writing OpenSimplex2 rather than importing it, and there are no
reference vectors to check against — so "correct" cannot mean "matches
KdotJPG's implementation bit for bit", and it is more honest to say so than to
imply a fidelity we cannot test.

What the product requirement actually needs is weaker and achievable: that
*our* implementation, once it satisfies the property tests below, never changes
its output again. So once the properties pass, freeze a table of
(seed, x, y) → value into a committed golden file. Any later refactor,
optimisation or "cleanup" that shifts a single value then fails loudly instead
of silently reshaping every world ever generated.

The property tests say the noise is good. The golden file says it is *fixed*.
Both are needed and neither substitutes for the other.

### The axis-alignment test needs a negative control

Criterion 5 is the whole reason for choosing OpenSimplex2 over Perlin, and a
statistical test that passes is worthless unless it is known to be capable of
failing.

So the test suite must contain a deliberately axis-aligned noise — a plain
square-grid value or Perlin noise, in test code only — and assert that the
same statistic **rejects** it. This mirrors what
`TestGuardCoversRenderingPackages` does for the purity guard: prove the
detector detects.

Without the control, "variance along the axes is close to variance along the
diagonals" is a threshold nobody can calibrate, and the likely outcome is a
threshold quietly widened until it passes.

### Seeding must mix, not truncate

Criterion 2 exists because the classic mistake is folding the seed in by
truncation or a low-quality shuffle, so seeds 1000 and 1001 give visibly
related worlds. Derive the permutation from a proper 64-bit mixer
(splitmix64 is four lines and well understood), and test adjacent seeds
specifically, not just distant ones.

### Scope of the API

`float64` throughout, and `float64` on the boundary. The rest of the engine is
`float32`, and the conversion belongs at the point of use in the generator —
noise accumulates over octaves and domain warps, and doing that in `float32`
would give up precision exactly where the determinism argument is most
delicate.

Splines are declared as control points and evaluated; monotonicity is a
property the constructor validates and the tests assert, because a spline that
is accidentally non-monotone produces terrain that folds back on itself and is
very hard to diagnose from the output.

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

8. **Golden vectors hold.** A committed table of (seed, coordinate) → value
   reproduces exactly. This is what turns "deterministic" from a property of
   one build into a promise across every future one.
9. **The golden vectors hold on another architecture.** Verified in CI on
   arm64 as well as amd64 — one fuses floating-point operations and the other
   does not, so this is the only check that catches the fusion hazard above.

Criterion 7 is cheap to write and catches an entire category of terrain bug.
Criteria 8 and 9 are what make criterion 1 mean anything beyond a single
machine.

## Explicitly out of scope

Terrain, biomes, caves, features, and any use of the noise. This milestone
produces a library and its tests, nothing visible.

## Result

`internal/noise` is in: OpenSimplex2 2D on a simplex lattice, fBm, domain
warping and monotone splines, importing nothing outside the standard library
and guarded by an archtest that says so. 98.7% covered; total 62.1%.

CI gained an arm64 job running this package's tests, which is what turns the
determinism claim from a comment into something checked.

### The scale was correct and useless

The first implementation derived its scale from a proved bound: three corners,
each no larger than `gradientMaxContribution` by Cauchy-Schwarz. Sound, and
every test passed.

It also meant the field only ever reached **±0.365**. Criterion 3 asks that
output stay *within* its documented range, and a field pinned at zero
satisfies that perfectly — so nothing caught it. The three corners cannot sit
at their individual optimum radius simultaneously, so the triangle inequality
overestimates by a factor of 2.74.

That is not cosmetic. Splines are how a designer says "this range of
continentalness is coastline". A curve authored across `[-1, 1]` and only ever
fed a third of its domain wastes two thirds of the control the interface
exists to give, and every consumer would end up inventing its own correction
factor — not all the same one.

The scale now comes from a tight bound found by searching sample positions and
giving each corner its best gradient, independently per corner because the
hash picks them independently. That makes it an upper bound regardless of
seed. It is attained in practice: 32 million samples across 40 seeds reach
0.99 of it, and the maximum sits at (√3/2, √3/2), the deep interior of a cell.

Two tests now hold it in place — one re-runs the search so the constant cannot
drift from the geometry, another cross-checks it against the closed-form
triangle-inequality bound, so a search that landed somewhere absurd shows up
as violating something proved by hand. And a new
`TestEval2DUsesMostOfItsRange` states the property that was missing: the range
must be *used*, not merely respected.

The golden vectors caught the rescale loudly and were regenerated
deliberately. That is the mechanism working, not a nuisance.

### `madd` and `dot2` had no tests

The two functions the entire cross-architecture guarantee rests on were
untested. `math.FMA` makes the property directly checkable, and the tests now
distinguish two cases that behave differently:

- Rewriting `madd` to call `math.FMA` fails everywhere, amd64 included.
  Verified by mutation.
- Rewriting it as a bare `a*b + c` fails only where the compiler actually
  fuses — which is the arm64 job's entire reason for existing.

Neither check substitutes for the other, and the file says so.

`TestDot2IsTheDotProduct` guards the argument order, which is not
hypothetical: the spline limiter called `dot2(alpha, alpha, beta, beta)`
intending α²+β² and got 2αβ, silently producing non-monotone splines. The
signature is worth keeping — it reads well at the noise call sites — but it
needed a test naming which argument is which.

### Isotropy comes from the kernel, not the lattice

Worth recording because it contradicts the folklore this milestone was written
on. Four mutations aimed at the lattice geometry — always picking the same
corner, an axis-only gradient table, asymmetric kernel weighting, and zeroing
the skew constants outright — **none** moved the axis-bias statistic.

The isotropy is carried by the radial falloff `t = 0.5 - dx² - dy²`, which is
rotationally symmetric by construction, rather than by the triangular tiling.
The mutation that *was* caught replaced the corner-kernel sum with separable
tensor-product interpolation — the actual mechanism behind Perlin's
axis-aligned artefacts — moving bias from 0.0019 to 0.0184 against a 0.010
threshold.

The negative control works: the square-grid value-noise control scores 0.0184
against OpenSimplex2's 0.0019, a 9.6× separation. The statistic can reject the
defect it exists to catch.

### Criterion 7 tests less than it appears to

A `math.Mod(x, 16)` mutation inside `Eval2D` is *not* caught, and the reason is
structural: the test computes the same global-coordinate formula on both
paths, so any pure function of that argument — however broken — still agrees
with itself.

The property criterion 7 actually protects is that a future caller builds
global rather than chunk-local coordinates, and that caller does not exist
until M17. The test carries a negative control proving the comparison
mechanism works, and says this in its own comments rather than implying
coverage it does not have.

## The arm64 job failed on its first run, and it was right to

Both of the following were found by that job. Neither was findable on amd64,
and both were shipped in the first version of this milestone.

### `float64(a*b) + c` does not prevent fusion

The mitigation this milestone was designed around does not work.

The Go spec says an explicit floating-point conversion "rounds to the
precision of the target type, preventing fusion that would discard that
rounding", which reads like a licence to write `float64(a*b) + c` and be safe.
It is not. For float64 to float64 the conversion changes no value, so the
compiler drops it before the fused-multiply-add rewrite runs. `madd` compiled
to a single `FMADDD` on arm64 regardless, and `madd` is used by fBm, domain
warping and splines.

`Eval2D` escaped, and only by luck worth recording: its one multiply feeds two
separate additions, so the compiler had to materialise the product anyway.
Relying on that is relying on a register allocator.

**The fix inverts the goal.** "Never fuse" is not expressible in Go; "always
fuse" is, because `math.FMA` is *defined* as the exactly rounded fused result.
Every architecture computes the same bits, hardware instruction or software
fallback. Determinism stops depending on defeating an optimiser and starts
depending on an operation that is specified.

`TestNoUnintendedFusedOperations` now compiles the package for arm64 and
asserts that every fused instruction in the output is attributed to a source
line in `fma.go` — inlining preserves the attribution, so a fused operation
anywhere else is named precisely. It runs on any host, needs no arm64
hardware, and has a control test proving a bare `a*b + c` really does fuse, so
the guard cannot go quietly vacuous.

That guard is the durable outcome here. The arm64 CI job verifies the real
toolchain end to end and is worth keeping, but a property only checkable in CI
is a property nobody checks while writing code.

### A golden file whose inputs are recomputed is not frozen

The actual CI failure was not in the noise at all. The golden test read only
the value column from `testdata/golden.csv` and regenerated its coordinates
from a seeded RNG — and the generator expression was
`rng.Float64()*4000 - 2000`, a multiply feeding a subtract, which arm64 fuses.
The *inputs* differed between architectures, so of course the outputs did.

The evidence was in the failure pattern and was misread at first: all thirteen
fixed cases passed and all forty generated ones failed. That is not what a
broken noise function looks like.

The file now carries both halves, and the test reads inputs from it and never
from the case table. The case table decides what to freeze; once frozen, the
file is the only authority. The generator uses `madd` too, so regenerating on
a different machine cannot silently change which coordinates are frozen.

Failure messages now print all seventeen significant digits rather than Go's
shortest representation — the shortest form is what made the first diagnosis
of this take a wrong turn, since the printed coordinate did not parse back to
the coordinate in the file.
