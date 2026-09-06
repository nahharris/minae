# M3 — Rewrite the light engine

**Status:** 📋 Planned
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

Replace `CalculateChunkLighting` with an incremental engine that is correct
across chunk seams, can darken as well as brighten, and reports which chunks
need re-meshing.

## What is wrong today

`CalculateChunkLighting` is a whole-chunk recompute that only ever *adds*
light. It wipes its own chunk's light map before reseeding from neighbour
borders, and has no removal pass, so placing a block can never darken anything.

Three defects compound it:

- `World.GetLight` returns **15** for a missing chunk. The BFS reads that as
  "already brighter than anything I could contribute" and halts. This is the
  reported cross-chunk propagation failure.
- The BFS writes into neighbouring chunks through `w.SetLight` but never marks
  them for re-meshing, so cross-seam updates stay invisible.
- Skylight decays by one in every direction, including straight down. Real
  skylight falls at full strength, which is what makes an open shaft bright to
  bedrock while a cave under an overhang goes black.

## Target shape

```go
type Engine struct {
    world World                    // GetBlock, SkyLight, SetSkyLight, HasChunk
    dirty map[ChunkCoord]struct{}  // every write records its chunk
}

func (e *Engine) SeedChunk(c ChunkCoord)      // top-down column scan
func (e *Engine) OnBlockChanged(x, y, z int)  // incremental add + remove
func (e *Engine) DirtyChunks() []ChunkCoord   // returns and clears
```

## Steps

- [ ] Rename `Chunk.LightMap` to `Chunk.SkyLight`. Do not add `BlockLight`
      until light sources exist.
- [ ] Fix `World.GetLight` missing-chunk semantics: unloaded chunks are opaque,
      return 0, and are never written to. Add `HasChunk`.
- [ ] Add the cosmetic fallback in the *mesh builder*, not the engine: a face
      whose neighbour cell lies in an unloaded chunk samples 15, so the world
      edge does not render as a black wall.
- [ ] Implement `SeedChunk`: top-down column scan, full strength until the
      first opaque block.
- [ ] Implement propagation with the correct decay rule — full strength
      straight down through air, minus one in the other five directions.
- [ ] Implement the removal BFS: zero cells dimmer than the node being removed,
      re-queue brighter ones as re-propagation sources.
- [ ] Implement `OnBlockChanged` on top of add and remove.
- [ ] Add dirty tracking to every write, and wire `DirtyChunks` into
      `game/app.go` so cross-seam updates re-mesh the neighbour.
- [ ] Delete `CalculateChunkLighting`.
- [ ] Raise `.coverage-floor`.

## Tests

All pure, all fast, using `internal/testutil`:

- [ ] Flat terrain: surface is 15, one block below is 0.
- [ ] Overhang: cells underneath decay with distance from the opening.
- [ ] Vertical shaft: full 15 all the way down, proving downward no-decay.
- [ ] Sealed cave: interior is 0.
- [ ] Cross-seam: light from chunk A reaches chunk B at the correct level, and
      symmetrically in the opposite direction.
- [ ] Place then break a block: light returns to a state identical to a fresh
      full recompute.
- [ ] **Property test** — after any random sequence of block edits, the
      incremental result is byte-identical to a full recompute from scratch.
      This is the test that earns its keep; it covers the whole class of bugs
      currently in the file.
- [ ] Dirty set: every chunk whose light changed is reported, and no chunk
      whose light did not change is reported.

## Verification

```bash
mise run ci
```

The property test is the real gate. If it passes over a few thousand random
edit sequences, the engine is correct in a way that no hand-written case list
can establish on its own.
