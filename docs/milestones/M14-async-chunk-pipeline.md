# M14 — Async chunk pipeline

**Status:** ✅ Done

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

## Progress

Two foundations landed. The pipeline itself is still to come.

### `Engine.SeedChunk` — per-chunk lighting

M3 deleted the previous `SeedChunk` for being order-dependent: it wiped its own
chunk before reseeding from neighbour borders, so seeding chunk B destroyed
light chunk A had pushed into it, and recovery depended on which came next.

This one has no wipe, and that is the whole argument for bringing it back.
**Adding a chunk can only add light.** An unloaded chunk reads as opaque, so
before it arrived its neighbours were lit as though every cell in it were solid
rock. Air appearing where rock was assumed can only open new paths; it cannot
invalidate a value the neighbours already hold, because that value never
depended on this chunk. So nothing needs removing, and an add-only propagation
converges regardless of arrival order.

Its load-bearing test is equivalence: seeding chunks one at a time ends
byte-identical to `RecomputeAll` over the finished world, in both channels.
Mutation-tested — dropping the neighbour-border seeding leaves light stopping
at the seam.

### `internal/chunks.Snapshot` — an immutable view for workers

Mesh workers cannot read the live world: `World.Chunks` is a plain map, and a
concurrent read during a write is a Go runtime throw, not a subtle race.
Locking per mesh build would serialise away the parallelism it exists for.

So a worker gets a copy of the 3×3 neighbourhood, taken on the owning goroutine
and then read with no locking at all. It costs about 2.9 MB and a memcpy, which
is cheap against a mesh build, and snapshots are pooled.

This needed no change to the mesh package, because the mesher already reads
through `mesh.WorldReader`. One wrinkle: `WorldReader` and `ChunkReader`
declare the same method names in *global* and *local* coordinates
respectively, so one type cannot satisfy both. The snapshot is the
`WorldReader`; `Center()` hands back the copied centre chunk as the
`ChunkReader`.

Its load-bearing test is that meshing through the snapshot produces
byte-identical geometry to meshing through the live world — if the substitution
is invisible, the copy is faithful in every way the renderer can observe.
Mutation-tested twice: omitting the light arrays fails the read comparison, and
failing to clear an absent neighbour on reuse leaks stale data into a recycled
snapshot.

### A sharp edge removed from the test harness

`testutil.Flat` rewrites *every* allocated chunk, so calling it to add terrain
for a newly arrived chunk silently refilled tunnels an earlier `Clear` had
carved. It cost a misleading test failure that looked like a lighting bug.

`Flat` now refuses to run after any `Fill` or `Clear`, which immediately caught
two further tests building the wrong world and passing anyway. `FlatChunk` was
added for the case that actually needed expressing — a single chunk arriving
into a world that is already built and lit, which streaming tests will need
constantly.

## Result

The pipeline is in and wired into the game. Generation and meshing run on a
worker pool; lighting and GPU upload stay on the main thread. `internal/chunks`
is at ~92% coverage, total 58.2%.

Parity, determinism and race-freedom are all verified. Parity is the one that
matters most: for the same region, the async result is byte-identical to what
the synchronous path produced — blocks, both light channels, and mesh geometry.

### A chunk arriving invalidates its neighbours' meshes, even when it changes no light

The design reacted to `DirtyChunks`, which reports *light* changes. That misses
a whole category: **a neighbour appearing changes face culling**. Seam faces
drawn against absent space must be culled once something is actually there, and
no light needs to change for that to be true.

A chunk that is solid to the top of the world is the pure case — seeding it
writes nothing at all, so it produces no dirt, and its neighbour would keep
drawing seam faces into rock forever.

`invalidateNeighbourMeshesLocked` now demotes any already-meshed neighbour when
a chunk arrives, bumping its epoch so an in-flight job built against the older
world is discarded. Mutation-tested: removing it fails the test.

This was flagged in passing during implementation as an M15 concern. It is
cheaper to close now than to hit while debugging streaming.

### `demoteDirty` cannot be observed within this milestone's scope

Worth recording honestly. Removing the re-mesh-on-light-change entirely does not
change any output this milestone can produce, on flat *or* varied terrain,
because the stage rule already guarantees every neighbour is lit before anything
is meshed. Light is final by meshing time.

It becomes load-bearing the moment chunks arrive one at a time, which is M15. It
is kept and exercised, but the parity tests do not prove it — the direct
late-arrival test does.

### "All eight neighbours" cannot be taken literally

Read strictly, every chunk on the edge of any finite region would wait forever
for neighbours nobody asked for. The rule is therefore: a neighbour gates a
transition only if it has been requested; an unrequested neighbour vacuously
satisfies it.

That is safe here only because M14 has no dynamic loading — nothing can appear
later and invalidate a chunk meshed against assumed-absent space. **M15 breaks
that assumption**, and the neighbour-invalidation above is what will carry it.

### Testing a non-blocking API

`Update` never blocks, so a tight loop burns thousands of iterations in
microseconds and gives workers no chance to finish. Two tests were written
against iteration counts and reported stalls that were only impatience. Every
drive loop is now bounded by wall time.

The race test also had to stop editing eventually: every `Invalidate` demotes a
chunk, so a fast enough edit stream keeps the pipeline permanently behind and
nothing ever reaches `Meshed`. It now applies a burst and then asserts the
pipeline settles — which is a better property than the one originally intended.
