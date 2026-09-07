package lighting

import (
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// viewSpan is the width, in chunks, of the neighbourhood a chunkView covers:
// the centre plus one ring in every direction.
const viewSpan = 3

// chunkView is a fixed nine-pointer window onto the chunk being seeded and
// its eight neighbours, used to resolve every read and write during
// SeedChunk without a map lookup per cell.
//
// It exists because a CPU profile of SeedChunk against real terrain showed
// mapaccess2 at 55% of the time: every light read and write went through
// World.GetSkyLight/SetSkyLight/etc, each of which resolves
// map[ChunkCoord]*Chunk from scratch, on the order of 860,000 times per
// chunk. Skylight and block light both cap at 15 and lose at least one level
// per horizontal step, so light originating in the centre chunk can reach at
// most 15 blocks past its border - strictly inside this nine-chunk
// neighbourhood, since a chunk is 16 blocks wide. That bound is what makes a
// fixed nine-pointer view provably sufficient for every read and write a
// seeding walk performs: anything the walk would need outside it is
// guaranteed to already hold a value at least as bright as what the walk
// could produce, so treating it as absent (this view's behaviour for
// anything outside its nine chunks) never changes the result. See
// docs/milestones/M15-chunk-streaming.md's "Follow-up" section for the full
// argument.
//
// Unlike chunks.Snapshot, which copies each chunk by value because mesh
// workers must not race with the main thread mutating World.Chunks, a
// chunkView holds pointers: SeedChunk always runs on the goroutine that owns
// the world (Pipeline.Update's doc comment says so), so there is no
// concurrent mutation to guard against, and lighting writes need to land on
// the real chunks.
type chunkView struct {
	center world.ChunkCoord
	chunks [viewSpan * viewSpan]*world.Chunk
}

// newChunkView builds a view of coord and its eight neighbours as they exist
// in w right now. A neighbour that is not loaded leaves its slot nil, which
// every accessor below treats exactly as an absent chunk in the live world:
// reads as dark/absent, writes are no-ops.
func newChunkView(w *world.World, coord world.ChunkCoord) *chunkView {
	v := &chunkView{center: coord}
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			v.chunks[viewIndex(dx, dz)] = w.GetChunk(coord.X+dx, coord.Z+dz)
		}
	}
	return v
}

// viewIndex maps a neighbour offset in -1..1 onto a slot.
func viewIndex(dx, dz int) int {
	return (dx+1)*viewSpan + (dz + 1)
}

// locate resolves a global (x, z) to the chunk holding it and the local
// coordinates within it. ok is false when the chunk is outside the view (at
// most one ring past the centre) or not loaded - both cases the view treats
// identically to an absent chunk in the live world.
func (v *chunkView) locate(x, z int) (chunk *world.Chunk, lx, lz int, ok bool) {
	cx, lx := world.ChunkAndLocal(x)
	cz, lz := world.ChunkAndLocal(z)

	dx, dz := cx-v.center.X, cz-v.center.Z
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		return nil, 0, 0, false
	}

	c := v.chunks[viewIndex(dx, dz)]
	if c == nil {
		return nil, 0, 0, false
	}
	return c, lx, lz, true
}

// GetSkyLight implements lightSurface.
func (v *chunkView) GetSkyLight(x, y, z int) uint8 {
	if y < 0 || y >= config.ChunkHeight {
		return 0
	}
	c, lx, lz, ok := v.locate(x, z)
	if !ok {
		return 0
	}
	return c.GetSkyLight(lx, y, lz)
}

// SetSkyLight implements lightSurface.
func (v *chunkView) SetSkyLight(x, y, z int, level uint8) {
	c, lx, lz, ok := v.locate(x, z)
	if !ok {
		return
	}
	c.SetSkyLight(lx, y, lz, level)
}

// GetBlockLight implements lightSurface.
func (v *chunkView) GetBlockLight(x, y, z int) uint8 {
	if y < 0 || y >= config.ChunkHeight {
		return 0
	}
	c, lx, lz, ok := v.locate(x, z)
	if !ok {
		return 0
	}
	return c.GetBlockLight(lx, y, lz)
}

// SetBlockLight implements lightSurface.
func (v *chunkView) SetBlockLight(x, y, z int, level uint8) {
	c, lx, lz, ok := v.locate(x, z)
	if !ok {
		return
	}
	c.SetBlockLight(lx, y, lz, level)
}

// GetBlock implements lightSurface.
func (v *chunkView) GetBlock(x, y, z int) *blocks.Block {
	c, lx, lz, ok := v.locate(x, z)
	if !ok {
		return nil
	}
	return c.GetBlock(lx, y, lz)
}

// HasChunkAt implements lightSurface.
func (v *chunkView) HasChunkAt(x, z int) bool {
	_, _, _, ok := v.locate(x, z)
	return ok
}

// hasNeighbourChunk reports whether the chunk at offset (dx, dz) from the
// view's centre - each in -1..1 - is loaded. It is the chunk-level
// equivalent of HasChunkAt, used where the caller already has a chunk-grid
// offset rather than a global coordinate (enqueueNeighbourBorders).
func (v *chunkView) hasNeighbourChunk(dx, dz int) bool {
	return v.chunks[viewIndex(dx, dz)] != nil
}

// neighbourhoodCeiling returns the highest Y, across every loaded chunk in
// the view, at which any block is solid - or -1 if every loaded chunk in the
// view is entirely air.
//
// This is the bound seedSkyLight uses to decide which open-sky cells are
// worth enqueueing: above the highest solid block anywhere in the
// neighbourhood, every column in every loaded chunk here is open sky at full
// brightness, so no horizontal step from one to another can ever raise
// anything. Using only the centre chunk's own highest solid block instead
// would be wrong the moment a neighbour is taller - an overhang whose
// underside sits above the centre chunk's own terrain but below a
// neighbour's needs exactly the cells this would wrongly skip. See
// TestSeedChunk_MatchesFullRecompute_GeneratedTerrain's overhang case, which
// is built specifically to fail if this uses the wrong chunk's height.
func (v *chunkView) neighbourhoodCeiling() int {
	ceiling := -1
	for _, c := range v.chunks {
		if c == nil {
			continue
		}
		if h := c.HighestSolidY(); h > ceiling {
			ceiling = h
		}
	}
	return ceiling
}
