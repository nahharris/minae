# M15 — Chunk streaming

**Status:** 📋 Planned
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
