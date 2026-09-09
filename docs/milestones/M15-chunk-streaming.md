# M15 — Chunk streaming

**Status:** ✅ Done
**Depends on:** [M14](M14-async-chunk-pipeline.md)

## Objective

The world follows the player. Chunks within view distance load, chunks outside
it unload, and the fixed 3×3 grid goes away.

Terrain is still the flat plane. Streaming and generation are kept apart on
purpose: debugging a moving frontier is much easier when the thing being
streamed is uniform.

## The shape

A ring of chunk coordinates around the player's chunk, ordered nearest-first so
what you are about to see is produced before what you might see later. Each
frame:

1. Compute the desired set from the player's position and view distance.
2. Enqueue missing chunks into the M14 pipeline, nearest first.
3. Unload chunks outside the set plus a hysteresis margin.
4. Upload whatever finished this frame, within a budget.

**Hysteresis matters more than it sounds.** Loading at exactly the view distance
and unloading at exactly the view distance means a player standing on a chunk
boundary loads and unloads the same ring every frame. The unload radius must
exceed the load radius by at least a chunk.

## What unloading has to get right

Unloading is where this milestone will actually go wrong, because it is the
direction nobody tests.

- **GPU meshes must be released.** `SceneRenderer.RemoveMesh` exists; the leak
  is silent and only shows up as memory climbing over a long session.
- **In-flight work for an unloaded chunk must be discarded**, not applied to a
  chunk that no longer exists.
- **Lighting must not chase unloaded chunks.** The light engine already treats
  an unloaded chunk as opaque and never writes to it, which is exactly right
  here — see `GetSkyLight`'s doc comment. Collision takes the opposite view for
  its own good reasons, documented in `world/collision.go`. Both stay as they
  are.
- **The player must not fall through a chunk that has not arrived yet.**
  Collision treats missing chunks as empty, so a player outrunning the loader
  drops into the void. Needs an explicit answer: hold the player until the chunk
  beneath them exists, or keep a small guaranteed-loaded radius that generation
  is never allowed to fall behind.

That last point is the one most likely to be discovered by falling out of the
world.

## Lighting at a moving frontier

The hard part, and worth stating plainly before implementation.

Skylight propagates between chunks. When a new chunk arrives beside a lit one,
light must flow into it — and the newly lit chunk invalidates its neighbour's
mesh, exactly as the seam fix in [M3](M3-light-engine.md) established. So:

- A newly loaded chunk must be lit *and* trigger a re-mesh of its neighbours.
- `Engine.DirtyChunks` already reports mesh invalidation rather than raw light
  changes, and already includes seam neighbours. Streaming should consume that
  rather than reinventing it.
- A chunk at the edge of the loaded world is lit against opaque unloaded
  neighbours. When those neighbours arrive, its light is wrong until
  recomputed. Either light lazily once neighbours exist (the pipeline's stage
  rule already expresses this), or accept a visible seam and fix it on arrival.
  The stage rule is the better answer and is why M14 has one.

## Configuration

`view_distance` in chunks, in `config.yaml` alongside the other tunables. A
default around 8 gives a 17×17 region — 289 chunks, versus the current 9.

That is a large jump in memory: each chunk is currently 64 KB of blocks plus 128
KB of light. 289 chunks is roughly 55 MB, which is acceptable but worth knowing
before it surprises someone. Nibble-packing the two light arrays, deferred in
M6, halves the light half and becomes worth revisiting here.

## Design decisions

Made before implementation, because each one is a place the obvious choice is
wrong.

### The streamer is separate from the pipeline

`Pipeline` knows how to advance a chunk through stages. It does not know where
the player is, and should not. A `Streamer` owns the desired set and drives the
pipeline through `Request` and a new `Release`.

That split is what makes the desired-set logic testable without workers: ring
computation, hysteresis and ordering are pure functions of (player chunk,
radii) and deserve tests that do not spin up goroutines.

### `Release` must undo everything `Request` set up

Unloading is the direction nobody tests, so its contract is written out in
full. Releasing a coord must:

- delete it from `World.Chunks`,
- delete it from `stages` and `epoch`,
- remove it from the pending `requested` queue if it never started,
- bump its epoch first, so an in-flight mesh job's result is discarded,
- and invalidate its neighbours' meshes, because their face culling assumed it
  was there.

Missing any one of these is a leak or a stale mesh, and only the first is
likely to be noticed by hand.

### A late generation result must be dropped, but no generation epoch is needed

`drainGenerated` currently inserts unconditionally. After `Release` that would
resurrect an unloaded chunk — a genuine leak, and the loaded set would silently
stop matching the desired set.

The fix is a wanted-check: drop a result whose coord is no longer in `stages`,
or whose stage is no longer `Generating`.

It is tempting to add a generation epoch alongside the mesh epoch. **It is not
needed, and adding one would be cargo cult.** Consider the one sequence that
looks dangerous: request, release, request again, and only then the first
result arrives. The stage is `Generating` and the coord is known, so the result
is accepted — and that is *correct*, because `Generator.Generate` is required
to be a pure function of coord. The stale result and the one still in flight
are the same chunk. Meshes need an epoch because a mesh depends on a mutable
world; generation does not depend on anything mutable at all.

### Hysteresis, and the counts that prove it

Load radius is `view_distance`; unload radius is `view_distance + 2`, both in
Chebyshev distance so the region is square and matches the chunk grid.

A margin of one is not enough. One chunk of margin means a player stepping
back and forth across a single boundary still sits exactly on the load edge,
and rounding decides the outcome. Two gives the frontier somewhere to sit.

The test must assert on **load and unload counts across an oscillation**, not
on the final set — a thrashing implementation reaches the correct final state
every time, which is exactly why asserting on state alone would pass.

### The player is held, not dropped

`world/collision.go` treats a missing chunk as empty, deliberately and for
reasons documented there. That is the right call for collision and the wrong
outcome for streaming: a player who outruns the loader falls into the void.

The decision: **the player does not move while the chunk containing them is
below `Generated`.** Position is not integrated and vertical velocity is
zeroed, so they hold in place rather than accumulating fall speed that launches
them downward the instant terrain appears.

This is a visible hitch, and that is the honest trade. Falling through the
world is unrecoverable; a stutter is not. On foot it cannot happen at all —
walking speed is far below the loader — so this exists for teleports, for
absurd-speed testing, and for a machine slow enough to matter.

Note the interaction: zeroing vertical velocity is what the M12 test
`RestingBodyDoesNotAccumulateFallSpeed` was written to protect. The same
property is what makes holding safe here.

### The frontier is already handled — but must be tested, not assumed

M14 recorded that "all eight neighbours" cannot be read literally: an
unrequested neighbour vacuously satisfies the stage gate. M14 was safe because
nothing was ever requested after startup. **Streaming breaks that**, and it is
worth being precise about why the existing machinery covers it:

- **Geometry.** A frontier chunk is meshed against absent neighbours. When one
  arrives, `invalidateNeighbourMeshesLocked` demotes the frontier chunk and
  bumps its epoch. Already in place as of M14.
- **Light.** A frontier chunk is lit as though its absent neighbours were solid
  rock. When a neighbour arrives, `SeedChunk` runs an add-only propagation, and
  `propagateAdd` is not clamped to the seeded chunk — it walks into any loaded
  chunk. So light flows *back* into the frontier chunk where a path opened up,
  `setLight` marks it dirty, and `demoteDirty` re-meshes it.

That second one is the M14 note about `demoteDirty` being unobservable coming
due: this is the milestone where it earns its place. The case that exercises it
is a cave whose only route to the sky runs through a chunk that had not loaded
yet — dark on arrival at the frontier, correctly lit once the neighbour lands.
A test must construct exactly that, because nothing simpler distinguishes it.

### Unloading leaves a stale seam, and that is accepted

The reverse of arrival: unloading a chunk means its still-loaded neighbour was
meshed with seam faces culled against blocks that are now gone, so you can see
through into the void, and its light is now too bright for the same reason.

`Release` invalidating neighbour meshes fixes the geometry. The light is left
stale on purpose — recomputing it would mean a removal walk at the frontier
every time a chunk unloads, which is real cost for a chunk sitting two rings
beyond view distance. If it ever becomes visible, the fix is to shrink the
gap between rendering and the unload radius, not to make unloading expensive.

## Validation criteria

1. **The loaded set matches the desired set** after the player moves, in every
   direction including across negative coordinates.
2. **Hysteresis prevents thrashing.** A player oscillating across a chunk
   boundary does not repeatedly load and unload the same chunk. Assert on load
   and unload *counts*, not just final state.
3. **Unloading releases GPU meshes**, verified by counting what the renderer
   holds rather than trusting that it was called.
4. **In-flight work for an unloaded chunk is discarded** and never applied.
5. **A newly loaded chunk is lit and re-meshes its neighbours**, so no seam is
   left dark. This is the streaming form of the bug that produced a black wall
   in M3.
6. **The player cannot fall through unloaded terrain**, including when moving
   faster than chunks can load. Test at absurd speed, as the physics
   no-tunneling property does.
7. **Frame time stays bounded** while streaming, with the budget honoured.
8. **Memory is stable over a long traverse** — walk a long path and confirm the
   loaded chunk count and mesh count return to steady state rather than
   climbing.

Criterion 8 is the one that catches leaks, and it is the one that only fails
after minutes of play, so it belongs in an automated test rather than a manual
checklist.

## Explicitly out of scope

Terrain generation, level of detail for distant chunks, frustum culling,
persistence to disk, and mesh compression.

## Result

The world follows the player. The fixed 3×3 grid is gone; `Streamer` owns the
desired set and drives the pipeline through `Request` and a new `Release`,
with `view_distance` defaulting to 8 — a 17×17 region, 289 chunks. Terrain is
still the flat plane, as planned. `internal/chunks` is at 97.1%, total 59.7%.

Every design decision above survived implementation unchanged. Two things did
not, and both are recorded below rather than quietly fixed.

### An epoch collision across unload and reload

Found in review, not by a test. `Release` deletes the coord's epoch entry, and
epochs were per-coord counters starting at 1 — so a chunk unloaded and
re-loaded began counting again from 1, and its first mesh job drew exactly the
epoch a job still in flight from the chunk's *previous* life was carrying.

`drainMeshed` decides staleness with `stage != Meshing || epoch != res.epoch`.
The re-requested chunk is legitimately `Meshing`, and now the epochs match, so
the stale result is **accepted** — a mesh built against a neighbourhood the
player has since walked away from and back to. Worse, accepting it marks the
chunk `Meshed`, so the correct result arriving moments later is the one
discarded.

This is precisely the failure criterion 4 exists to prevent, and it was
invisible while chunks were never released, which is why M14 could not have
caught it.

Epochs now come from one monotonic counter, `takeEpochLocked`, shared by every
site that invalidates a mesh. A reused epoch becomes impossible by
construction instead of by argument, and `Release` can still delete the entry —
so the epoch map does not grow without bound over a long traverse, which
criterion 8 would not have caught either.

The regression test is white-box, and deliberately so. Reproducing the failure
end-to-end needs a mesh job held in flight across a release, a re-request, a
regeneration, a re-lighting and a second dispatch, with the stale result
landing first. Nothing exported can hold a mesh worker at a chosen point, so a
black-box version would depend on winning a race rather than on the bug being
present. The test asserts the invariant in terms of `drainMeshed`'s own
staleness expression, and states in its own comment what it does not cover.

### Determinism leaks in the streamer

`coordsToRelease` ranged over a map and `coordsToRequest` used an unstable
sort, so both the request order among equidistant chunks and the released set
handed back to the caller varied run to run. Neither changes the final loaded
set, which is why the tests passed.

That still matters here: `Update` returns the released slice for the caller to
free GPU meshes with, so a caller could behave differently on identical input.
Determinism regardless of ordering is a property this project has asserted
since M14, and leaving a known source of it in place because nothing currently
observes it is how the once-a-month bug gets written. Both are now ordered.

### `demoteDirty` finally earns its place — but not where expected

M14 recorded that `demoteDirty` could not be observed and predicted this
milestone would make it load-bearing. That turned out to be **half right**, and
the correction is worth keeping.

The predicted scenario — a chunk arriving and lighting its neighbour through a
seam — does not exercise it. `invalidateNeighbourMeshesLocked` fires from
`drainGenerated` and demotes the already-meshed neighbour *before* its light
could change, because the arriving chunk must itself reach `Lit` before the
stage gate lets the neighbour re-mesh. Geometry invalidation structurally
pre-empts light invalidation for every "new chunk arrives" case. Both were
verified by mutation: disabling either one alone leaves the sealed-cave test
passing, so they are genuine redundancy rather than a gap.

What does pin `demoteDirty` down is a light change crossing a seam between two
chunks that are *both already loaded* — no stage transition, so geometry
invalidation never fires. That case has its own test, and it is the only one
that fails when `demoteDirty` is disabled.

### Coverage of the player-hold decision

The mechanical half is tested: `stepBody` freezes position and zeroes vertical
velocity when held, mutation-verified. The *decision* —
`Stage(playerChunk) < Generated` in `app.go` — is not, because `Game` needs a
live raylib context to construct. That matches the existing convention for
`Player.Update` from M13, and it is a real gap rather than an acceptable one:
the predicate could be inverted and no test would notice.

One further limit worth naming: the hold tests the chunk containing the
player's centre, while the player's AABB has width and can span two chunks at
a boundary. A player held at a seam with one unloaded neighbour is approximated
rather than handled exactly. It has no visible effect at walking speed and is
the kind of thing that would matter to a teleport landing exactly on a chunk
edge.

## Follow-up: streaming was too slow to play

Reported after M17 landed: the initial region takes long enough to be
unplayable, and walking into new chunks freezes the game.

### Measured, not guessed

Per chunk, against real generated terrain:

| stage | cost | thread |
|---|---|---|
| generate | 235 µs | worker |
| snapshot | 211 µs | main |
| mesh | 4.7 ms | worker |
| **light (`SeedChunk`)** | **103 ms** | **main** |

Lighting is 440× generation and 22× meshing, and it is the one expensive thing
M14 deliberately left on the main thread. At the per-frame budget of 9, that is
roughly 930 ms of main-thread work in a single frame. There is no mystery left
to solve; that is the freeze.

A CPU profile says where it goes: `mapaccess2` is **55%** of the time, with
`aeshashbody` alone at 24%. Every light read and write resolves
`map[ChunkCoord]*Chunk` from scratch, and propagation performs on the order of
860,000 of those per chunk.

Worth noting why this only surfaced now: against the flat plane, terrain sat at
y=32 with no relief, so there was far less air to seed and no shaded geometry
to propagate through. M14 and M15 chose a budget of 9 against a generator that
filled a constant. M17 did not make lighting slow — it revealed it.

### Three changes

**Resolve chunk pointers once per operation, not per cell.** Skylight and
block light both cap at 15 and lose at least one level per horizontal step, so
light originating in a chunk can reach at most 15 blocks past its border —
strictly inside the 3×3 neighbourhood. That bound makes a nine-pointer view
provably sufficient, and it is the same argument `chunks.Snapshot` rests on,
except this one holds pointers because lighting writes.

**Stop enqueueing sky cells that cannot propagate anywhere.** The column walk
sets every open-sky cell to full brightness and enqueues all of them — about
190 per column with terrain near y=64 in a 256-tall world. Above the highest
solid block in the whole 3×3 neighbourhood, every column is open sky at full
brightness, so no horizontal step can raise anything and no such cell can
contribute. Only cells below that ceiling need to be queued, which with M17's
relief of roughly twenty blocks is an order-of-magnitude fewer.

The subtle case, and the reason this needs the equivalence test rather than an
argument: a neighbour chunk that is loaded but *not yet lit* reads as dark, so
skipping propagation into it looks unsafe. It is safe, because that chunk is
seeded in turn and its own column walk sets exactly those cells — the pipeline's
stage rule guarantees every Generated chunk is lit before anything is meshed.
The final state is identical, which is precisely what
`TestSeedChunk_MatchesFullRecompute` checks.

**Budget by time, not by count.** A count of 9 was chosen when a chunk was
cheap and uniform; it cannot adapt to a chunk that turns out to cost more.
A frame should spend at most a stated number of milliseconds on lighting and
uploads and defer the rest, so the worst case is a slower fill rather than a
stall.

### What this is not

Moving lighting off the main thread is the real long-term answer and remains
M14's deferred research problem. It is not attempted here: the cost is being
paid at the wrong order of magnitude, and fixing that is worth doing before
rearchitecting around it.

### Result

All three changes landed as designed, and none of the three turned out to be
unsound.

**The nine-pointer view** is `chunkView` in `internal/world/lighting/chunkview.go`:
a fixed array of nine `*world.Chunk`, built once per `SeedChunk` call and
threaded through `propagateAdd`, `runRemove`, `setLight`, `markMeshDirty` and
`enqueueNeighbourBorders` behind a new `lightSurface` interface. `*world.World`
satisfies the same interface unchanged, so `RecomputeAll` and `OnBlockChanged`
still read and write through the live map exactly as before — only
`SeedChunk`'s call path resolves against the view.

**The ceiling** is `chunkView.neighbourhoodCeiling`: the highest solid Y across
every *loaded* chunk in the nine-chunk view (chunks that are not loaded are not
part of the neighbourhood and are simply skipped, matching how propagation
already stops at an unloaded chunk's edge). `world.Chunk` grew a
`highestSolidY` field, maintained incrementally by `SetBlock`/`SetBlockState`
rather than rescanned per call: raising it is O(1), and lowering it — only
possible by removing the single highest block in a 16×256×16 chunk — rescans
that one Y layer downward, which is rare enough not to matter.

**The budget** is a `time.Duration` pair. Production uses
`chunks.Budget{Light: 4ms, Mesh: 2ms}` in `internal/game/app.go` — chosen
against the *post-fix* `SeedChunk` cost (see below), with headroom for a
chunk whose neighbours are not yet lit (more cross-seam enqueueing) plus a
generous allowance for draining finished meshes, while keeping both well
under a 16ms frame at 60 FPS. Both fields check their deadline only *after*
completing a whole unit of work (one `SeedChunk` call, one drained mesh
result), which guarantees a positive budget always makes some progress —
never a full stall — at the cost of a call occasionally finishing slightly
past its nominal deadline.

One nuance the design section did not anticipate: draining a finished mesh
result is a channel receive plus two map writes, cheap enough that on an
ordinary machine's clock resolution (measured around 500µs on the development
machine) a whole frame's worth of backlog drains inside a single clock tick
regardless of the Mesh budget's value. `TestPipeline_BudgetsAreHonoured`'s
Light half asserts "a budget too small to finish in one call defers the
rest"; the Mesh half deliberately does not make that same assertion — an
experiment pushing 1200 pending results through the same loop confirmed it is
not reliably observable by wall clock at this cost per item. What it does
still assert, deterministically, is that a zero budget drains nothing and a
positive one eventually drains everything exactly once. The real cost this
budget exists to bound — the GPU upload — happens in `app.go` after `Update`
returns, outside what either budget measures; shrinking that gap is future
work, not something this pass claims to have solved.

**Numbers**, on the development machine (`BenchmarkSeedChunk`,
`internal/world/lighting`, real worldgen terrain, seeded neighbours already
lit — the common case once the pipeline is past its first chunk):

| | before | after |
|---|---|---|
| `SeedChunk` | 17.5 ms/chunk (measured against this benchmark) | 3.0 ms/chunk |

The 103ms/chunk figure earlier in this document was measured under the full
running pipeline rather than this isolated benchmark and is not directly
comparable in absolute terms, but the mechanism it names — `mapaccess2`
dominating every light read and write — is exactly what the nine-pointer view
removes; the ~5-6x measured here and the ~34x implied against the original
103ms figure are two views of the same fix. `BenchmarkMeshTerrainChunk` (new,
`internal/chunks`) measured 4.5ms/chunk, matching the 4.7ms this document
already cites meshing at. Filling the initial 289-chunk region (17×17,
default `view_distance`) at the post-fix cost is now light-bound by neither
lighting nor meshing but by the 4ms/2ms per-frame budget itself: roughly one
to a few chunks lit per frame depending on how many neighbours are already
lit, so on the order of 1-2 seconds to fully light the region rather than the
several-hundred-frame stall the 103ms figure implied.

**Mutation testing**, each reverted immediately after confirming the failure:

| mutation | caught |
|---|---|
| nine-pointer view resolves the wrong chunk for one offset | yes — `TestSeedChunk_MatchesFullRecompute_GeneratedTerrain`'s overhang case, 2 of 3 seeding orders (the existing flat/tunnel fixtures did not catch it) |
| ceiling uses the seeded chunk's own height instead of the neighbourhood's | yes — the same test's overhang case, all 3 orders |
| time budget's deadline check is skipped (`seedLit`) | yes — `TestPipeline_BudgetsAreHonoured`'s Light half |
| time budget's deadline check is skipped (`drainMeshed`) | yes — the Mesh half, once it waits for the worker pool to actually finish before draining |

The first two mutations were not caught by the pre-existing flat/handmade
fixtures, which is exactly the gap this milestone's generated-terrain tests
(`internal/world/lighting/seed_chunk_generated_test.go`) exist to close —
uniform terrain cannot distinguish "this chunk's own height" from "the
neighbourhood's height" because they are always equal.

### Correction: which "before" number is the right one

The implementation reported a 17.5 ms starting point and a 5.8× improvement.
That measurement is real but it is the wrong scenario, and the difference
matters enough to write down.

`BenchmarkSeedChunk` lights a chunk whose neighbours are *already lit*, so
propagation stops dead at every border — nothing next door can be raised. The
103 ms in the section above was measured with neighbours loaded but **unlit**,
so light floods into all eight. That is the startup case and the
walking-into-fresh-terrain case, which is to say it is the case the original
report was about.

Same code, same machine, both scenarios:

| | before | after | |
|---|---|---|---|
| warm — neighbours already lit | 17.5 ms | 2.2 ms | 8× |
| **cold — neighbours unlit** | **103 ms** | **2.8 ms** | **36×** |

Both benchmarks are now committed, with the distinction spelled out in
`BenchmarkSeedChunkColdNeighbours`. An optimisation judged only against the
warm case would have looked five times better than it was, and would have been
tuned against a workload the player never experiences.

### Two further wins the first profile could not see

Re-profiling after the three changes landed showed a completely different
shape, which is the argument for profiling again rather than assuming the plan
was complete.

**`markMeshDirty` had become the single largest cost at 22%** — larger than the
chunk resolution the whole pass was aimed at. Every light write did a map
insert to record its chunk as needing a re-mesh, and a chunk seed makes tens of
thousands of writes into the same handful of chunks. The set was being told the
same fact about the same chunk forty thousand times.

`Engine.dirtyMemo` is a small linear-scanned array of already-marked coords.
Linear beats a map because the count is tiny and *bounded*: light cannot leave
its 3×3 neighbourhood, and border writes mark one chunk further out, so at most
25 distinct chunks can be marked in one operation. Correctness rests on a
single invariant — skipping an insert is only safe while the coord is genuinely
still in `dirty` — so anything that empties or bypasses the set resets the
memo. 4.0 ms → 3.3 ms.

**`setLight` re-established what its caller had just proved.** `propagateAdd`
checks the chunk is loaded and reads the current level, then called `setLight`,
which checked both again — two of five chunk resolutions per write spent
re-deriving a fact one line old. `setLightKnown` drops them. 3.3 ms → 3.2 ms.

That second one is the honest remainder of "collapse four resolutions into
one". The larger version — hoisting the cell index itself into the hot loop —
was measured at roughly 13% and deliberately not taken: it would couple the
light engine to `Chunk`'s memory layout, blocking the nibble-packing deferred
in M6, and put the index arithmetic in a second place. That is the exact bug
class that produces subtly wrong lighting, which is what this project was
started to fix. See [the parallel lighting design
note](../design/parallel-lighting.md).

### Where this leaves multi-threaded lighting

Researched in that design note and, on these numbers, not needed yet.

The fastest known implementation of this problem — Starlight, ~35× vanilla
Minecraft — is entirely single-threaded, and post-1.20 lights a chunk in
0.75 ms. At 2.8 ms cold we are within about 4× of it, against the 140× the
original 103 ms represented. Threading offers 8× on top; a per-frame time
budget already prevents any single frame from stalling.

The 9-colour parallel schedule in the design note stays specced and ready. It
is a smaller change than M14's "research problem" framing implied, because
light provably cannot leave its 3×3 neighbourhood, so chunks 3 apart have
disjoint write footprints and need no locking at all. The real work is that
`Engine` holds mutable scratch which would have to become per-worker — and
that is worth doing when there is a second caller, not before.

### Confirmed in play

2026-09-07: "perf is good". The startup stall and the walking freeze are both
gone at the default view distance of 8.

That closes the report this follow-up was opened for. The GPU mesh upload in
`app.go` remains genuinely unmeasured — it runs after `Update` returns and no
budget covers it — so it stays the first suspect if hitching ever reappears.
