# M14 — Async chunk pipeline

**Status:** 📋 Planned

## Objective

Move chunk generation, lighting and meshing off the main thread, behind a stage
pipeline, without changing what the world looks like. Terrain stays the flat
plane; only *how* chunks come into existence changes.

This is the foundation [M15](M15-chunk-streaming.md) streams with. It is
deliberately first: building the concurrency architecture while the generator is
trivial means the terrain is not a variable while the threading is debugged, and
it avoids writing a synchronous generator and retrofitting it later.

## What makes this hard

Three pieces of shared mutable state stand between here and a worker pool.

**`World.Chunks` is a plain map.** Concurrent read during write is not a subtle
race in Go — the runtime throws `concurrent map read and map write` and the
process dies. Any worker touching the world while the main thread inserts a
chunk crashes the game.

**The light engine holds mutable scratch.** `Engine` owns `addQueue`,
`removeQueue`, `reAdd` and `dirty`, reused across calls. Two goroutines calling
`OnBlockChanged` corrupt each other silently.

**Lighting and meshing are inherently cross-chunk.** This is the real problem,
and no amount of locking removes it:

- Skylight propagates *between* chunks, so a chunk cannot be lit in isolation.
  Its neighbours must exist first, or light stops at the seam — the exact bug M3
  was written to fix.
- The mesh builder reads neighbouring chunks for face culling, light sampling
  and ambient occlusion across seams. Meshing a chunk requires its neighbours to
  exist *and* not change underneath it.

So "generate each chunk on a worker" is not achievable directly. What is
achievable is a pipeline where a chunk may only advance when its neighbours have
reached the previous stage.

## The pipeline

```
Empty ──► Generated ──► Lit ──► Meshed ──► Uploaded
```

| stage | needs | where it runs |
|---|---|---|
| Generated | nothing — a pure function of (seed, coord) | worker pool |
| Lit | all 8 neighbours Generated | main thread, budgeted |
| Meshed | all 8 neighbours Lit | worker pool, over immutable chunks |
| Uploaded | its own mesh data | main thread (GPU) |

A chunk advances only when every neighbour has reached the prior stage. That
single rule is what makes cross-chunk work safe: by the time a chunk is meshed,
everything it reads is stable.

### Why lighting stays on the main thread

Generation is pure and meshing is read-only, so both parallelise cleanly.
Lighting is neither: it *writes across chunk boundaries*, which is precisely
what M3 built and what the seam fix reinforced. Making it concurrent would mean
either locking the world per light write — in the BFS inner loop, which would
cost more than it saves — or partitioning light propagation with a border
exchange phase, which is a research problem, not a milestone.

Running it on the main thread with a per-frame budget keeps the expensive halves
(generation, meshing) parallel and leaves the subtle half single-threaded. If
lighting later proves to be the bottleneck, that is a measurement to act on, not
a guess to design around now.

## Ownership rules

Written down because concurrency bugs come from ambiguity about who may touch
what:

- **The main thread owns `World.Chunks`.** It is the only thing that inserts,
  removes, or looks up chunks. Workers never see the map.
- **Workers receive exactly what they need**, passed in explicitly: a seed and
  a coordinate for generation; a chunk plus its eight neighbours for meshing.
- **A chunk being meshed is immutable.** If a block edit targets a chunk with a
  mesh job in flight, the job's result is discarded and re-queued rather than
  applied. Stale meshes are the failure mode to design against here — see the
  seam bug in [M3](M3-light-engine.md).
- **Nothing shared is mutable without a stated owner.** The scratch buffer
  removed from `World` during M13 is the pattern: a cached buffer on a shared
  object is a race waiting for a second caller.

## Validation criteria

1. **`go test -race` is clean** with the pipeline under load — many chunks
   generating, lighting and meshing while blocks are edited. The race detector
   only reports races it actually observes, so the test must genuinely exercise
   concurrent access rather than merely enable the flag.
2. **The stage rule holds.** A chunk is never lit before its neighbours are
   generated, nor meshed before its neighbours are lit. Assert on the state
   machine directly rather than inferring it from output.
3. **Output is identical to the synchronous version.** For a fixed seed and
   chunk set, the resulting blocks, light and mesh data must match what the
   current single-threaded path produces, byte for byte. This is the property
   that says "we changed when, not what".
4. **Determinism regardless of completion order.** Workers finish in arbitrary
   order; the final world must not depend on it. Run the same set repeatedly and
   compare — a scheduler-dependent result is a bug that will reproduce once a
   month otherwise.
5. **A block edit during an in-flight mesh job does not produce a stale mesh.**
   The specific race this design is most likely to get wrong.
6. **No deadlock and no starvation.** A chunk whose neighbours never arrive must
   not block the pipeline; the test should fail loudly rather than hang.
7. **The main thread is not blocked.** Frame time stays bounded while chunks are
   being produced.

Criteria 1 and 3 are the load-bearing pair: one says it is safe, the other says
it is still correct.

## Explicitly out of scope

Dynamic loading and unloading (that is M15), any change to terrain, level of
detail, and mesh data compression.
