# M3 — Rewrite the light engine

**Status:** ✅ Done
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

Replace `CalculateChunkLighting` with an incremental engine that is correct
across chunk seams, can darken as well as brighten, and reports which chunks
need re-meshing.

Note this milestone does **not** put light on screen. The engine now computes
correct values, but the renderer still discards them — see
[M4](M4-render-pipeline.md).

## What was wrong

`CalculateChunkLighting` was a whole-chunk recompute that only ever *added*
light. It wiped its own chunk before reseeding from neighbour borders, had no
removal pass, and decayed skylight downward.

- `World.GetLight` returned **15** for a missing chunk. The BFS read that as
  "already brighter than anything I could contribute" and halted — the reported
  cross-chunk propagation failure.
- The BFS wrote into neighbouring chunks but never marked them for re-meshing.
- Skylight decayed in all six directions, so caves under overhangs were not dark.

## Steps

- [x] Rename `Chunk.LightMap` to `Chunk.SkyLight`, and the accessors to
      `GetSkyLight` / `SetSkyLight`. No `BlockLight` until light sources exist.
- [x] Fix `World.GetSkyLight`: unloaded chunks return 0, are opaque, and are
      never written to. Added `HasChunkAt`.
- [x] Cosmetic fallback in the *mesh builder*, not the engine: a face whose
      neighbour cell is in an unloaded chunk samples 15, so the world edge does
      not render as a black wall.
- [x] Column scan, propagation with the correct decay rule, removal walk,
      `OnBlockChanged`, automatic dirty tracking.
- [x] Wire into `game/app.go`.
- [x] Delete `CalculateChunkLighting`.
- [x] Raise `.coverage-floor` from 21.5 to 29.0.

## The engine

```go
func NewEngine(w *world.World) *Engine
func (e *Engine) RecomputeAll()
func (e *Engine) OnBlockChanged(x, y, z int)
func (e *Engine) DirtyChunks() []world.ChunkCoord
```

### The decay rule

Skylight falls **straight down at full strength** and decays by one in every
other direction:

```
expected(L, d) = 15      if d is straight down AND L == 15
                 L - 1   otherwise
```

This is what makes an open shaft bright to bedrock while a cave under an
overhang goes black. It is implemented once and used by both the add and the
remove walk, so the two cannot drift apart.

### Why removal compares against `expected`, not `nl < level`

The textbook removal walk asks whether a neighbour is *dimmer* than the cell
being removed, on the reasoning that a dimmer neighbour must have been fed by
it. That is wrong here. Because skylight falls downward losslessly, the cell
directly beneath a removed level-15 cell is itself 15, so `nl < level` is false
and the textbook test classifies it as an independent light source — leaving the
entire column lit underneath a freshly placed block.

Comparing against `expected(level, d)` handles the lossless downward case. A
cell that merely coincides with `expected` while genuinely fed by another source
is safe: it gets zeroed, the adjacent real source lands in the re-add set, and
re-propagation restores it.

This was the predicted failure mode, and mutation testing confirmed it: swapping
the comparison for `nl < node.Level` makes the property test fail with
`incremental=15 full-recompute=14`.

## Design decisions worth recording

**`RecomputeAll`, not `SeedChunk`.** The milestone originally specified a
per-chunk seed. That is order-dependent — seeding chunk B wipes light chunk A
already pushed into it, and recovery depends on which chunk is seeded next,
which in Go means map iteration order. `RecomputeAll` zeroes everything, scans
all columns into one queue, and propagates once, so it is order-independent by
construction. That is what makes it usable as the property test's ground truth.
A per-chunk seed is only genuinely needed for chunk streaming, which is backlog.

**Air is exactly `nil`.** `FromNumericID` short-circuits on numeric ID 0, and
both `nil` and `blocks.Air` map to 0, so `GetBlock` can never return a block
whose ID is `minae/air`. The old code's `ID == "minae/air" || Name == "Air"`
branches were unreachable. Transparency is now a single named helper.

**Engine correctness and edge cosmetics are separate.** The engine treats
unloaded chunks as opaque so light never originates from nowhere. The renderer
substitutes full-bright for faces against unloaded chunks so the world edge is
not a black wall. Putting the cosmetic rule in the engine would have corrupted
the propagation.

**`RecomputeAll` marks every loaded chunk dirty rather than diffing.** A full
recompute is a bulk operation where a safe over-approximation is correct and a
precise diff buys nothing. The strict "skip unchanged writes" behaviour is
reserved for the incremental paths, where spurious re-meshes would matter.

## Verification

The hand-written cases cover flat terrain, overhang decay, vertical shafts,
sealed caves, cross-seam propagation in both directions, unloaded neighbours,
place-darkens and break-restores.

The test that actually guards the engine is the **property test**: after every
edit in a random sequence, the incremental result must be byte-identical to a
full recompute. It reports the first divergent cell by both local and global
coordinate rather than dumping two 64KB arrays.

Both were checked by **mutation testing** — a test suite that has never been
seen to fail is not evidence of anything:

| Mutation | Caught by |
|---|---|
| Removal uses the textbook `nl < level` | property test, and `PlaceDarkens` |
| `expected` drops the downward full-strength case | property test, `VerticalShaft`, `PlaceDarkens`, `RemoveRestores` |

### Performance

The lighting suite initially took 139 s under CI's `-race -covermode=atomic`.
Two fixes brought it to 22 s:

- `RecomputeAll` zeroed light with an element-by-element loop — 65,536
  individually instrumented writes per chunk. Replaced with `clear`, which
  compiles to a single memclr.
- The column scan walked all 256 rows of every column even after the column was
  closed, writing zeros over the zeros `clear` had just written. It now breaks
  at the first opaque block.

Both are real improvements to production code, not test-only tuning.

```bash
mise run ci
```

At the close of M3: build, vet, `go test -race`, and lint all clean. Total
coverage 29.4%, floor raised to 29.0. `internal/world/lighting` went from 0% to
94.2%.

**Not verified:** nothing here has been seen on screen, because the renderer
still discards the light. M4 closes that loop and carries the manual visual
checklist.

## Follow-up fix: stale meshes on chunk seams

Reported from a screenshot: placing a glowstone in a dark room left one wall
pitch black. Destroying a block in that wall made it re-render correctly.

The dirty set recorded **the chunk each changed cell lives in**. But a block's
faces are lit by the cells *around* it — since M7's smooth lighting, by a 2×2
patch per corner, so up to one step away including diagonally. A block one step
across a chunk seam is therefore lit by cells in the neighbouring chunk.

A solid wall standing on a seam is the case that makes this visible, and it is
the worst case rather than an edge case: **no cell inside the wall's chunk can
ever change**, because every cell in it is rock sitting at 0. Light the room on
the other side and the engine correctly reports the room's chunk and nothing
else. The wall keeps rendering the darkness baked into its mesh until something
unrelated forces a re-mesh — which is exactly what breaking a block in it did.

`markMeshDirty` now records the changed cell's chunk plus, when the cell sits on
a border, the neighbouring chunks that render it. Interior cells — the
overwhelming majority of writes — still cost a single map insert, so the BFS
inner loop is unaffected.

`DirtyChunks` is now documented as "chunks whose meshes are invalidated" rather
than "chunks whose light changed". The distinction is the whole bug.

### A test that encoded the wrong contract

`TestDirtyChunksMatchesActualChanges` asserted that reporting a chunk whose
light did not change was "a wasted re-mesh". That was the old contract, and it
failed the moment the fix landed.

Rather than delete the check, it now bounds the tolerance: an unchanged chunk
may be reported only if it actually touches a chunk that did change. Reporting a
seam neighbour is correct; reporting an unrelated chunk is still a bug worth
catching.
