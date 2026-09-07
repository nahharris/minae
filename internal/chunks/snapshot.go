// Package chunks holds the machinery for producing chunks off the main
// thread.
package chunks

import (
	"sync"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/world"
)

// span is the width of the neighbourhood a snapshot covers, in chunks: the
// centre plus one ring, which is everything the mesher reads.
const span = 3

// Snapshot is an immutable copy of a chunk and its eight neighbours — enough
// for the mesher to build that chunk's mesh without touching the live world.
//
// It exists because meshing moved onto a worker pool. The mesher reads
// neighbouring chunks for face culling, light sampling and ambient occlusion
// across seams, and World.Chunks is a plain map: a concurrent read while the
// main thread inserts a chunk is a Go runtime throw, not a subtle race.
// Locking the world for each mesh build would serialise away the parallelism,
// so a worker takes a copy instead and then runs entirely on its own data.
//
// A Snapshot satisfies mesh.WorldReader in global block coordinates. The
// centre chunk is reached through Center, because mesh.ChunkReader declares
// the same method names in *local* coordinates and one type cannot mean both.
type Snapshot struct {
	center world.ChunkCoord

	// storage holds the chunks by value, so the copy is deep without a
	// per-chunk allocation. loaded says which slots hold real data: an absent
	// neighbour must read as unloaded, not as air.
	storage [span * span]world.Chunk
	loaded  [span * span]bool
}

var pool = sync.Pool{New: func() any { return new(Snapshot) }}

// Take copies the chunk at coord and its eight neighbours out of w.
//
// The caller must ensure w is not mutated while this runs; it is meant to be
// called on the goroutine that owns the world. Once it returns, the result
// shares nothing with w and is safe to use from any goroutine.
//
// The snapshot comes from a pool. Pass it to Release when the mesh job is
// done, after which it must not be touched.
func Take(w *world.World, coord world.ChunkCoord) *Snapshot {
	s := pool.Get().(*Snapshot)
	s.center = coord

	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			i := index(dx, dz)

			src := w.GetChunk(coord.X+dx, coord.Z+dz)
			if src == nil {
				// Clearing matters: this Snapshot may have been used before,
				// and a stale neighbour surviving reuse would be read as real.
				s.loaded[i] = false
				continue
			}

			// Chunk holds its data in arrays, so assigning the struct copies
			// every one of them. Nothing is shared with the live chunk.
			s.storage[i] = *src
			s.loaded[i] = true
		}
	}
	return s
}

// Release returns a snapshot to the pool. It must not be used afterwards.
func (s *Snapshot) Release() {
	pool.Put(s)
}

// Center returns the chunk the snapshot was taken around, as a
// mesh.ChunkReader in local coordinates.
func (s *Snapshot) Center() *world.Chunk {
	return &s.storage[index(0, 0)]
}

// GetBlock returns the block at the given global coordinates, or nil where
// there is none or the chunk is outside the snapshot.
func (s *Snapshot) GetBlock(x, y, z int) *blocks.Block {
	block, _ := s.GetBlockState(x, y, z)
	return block
}

// GetBlockState returns the block and its metadata at the given global
// coordinates.
func (s *Snapshot) GetBlockState(x, y, z int) (*blocks.Block, uint8) {
	chunk, lx, lz, ok := s.locate(x, z)
	if !ok {
		return nil, 0
	}
	return chunk.GetBlockState(lx, y, lz)
}

// GetSkyLight returns the skylight at the given global coordinates, or 0
// where the chunk is outside the snapshot.
func (s *Snapshot) GetSkyLight(x, y, z int) uint8 {
	chunk, lx, lz, ok := s.locate(x, z)
	if !ok {
		return 0
	}
	return chunk.GetSkyLight(lx, y, lz)
}

// GetBlockLight returns the block light at the given global coordinates, or 0
// where the chunk is outside the snapshot.
func (s *Snapshot) GetBlockLight(x, y, z int) uint8 {
	chunk, lx, lz, ok := s.locate(x, z)
	if !ok {
		return 0
	}
	return chunk.GetBlockLight(lx, y, lz)
}

// HasChunkAt reports whether the snapshot holds the chunk containing the
// given global block coordinates.
//
// Anything beyond the nine chunks copied reads as unloaded, exactly as an
// ungenerated chunk does in the live world. A snapshot is not a window onto
// the world; it is all the world the mesher gets, and the mesher's
// full-bright edge fallback depends on being told so.
func (s *Snapshot) HasChunkAt(x, z int) bool {
	_, _, _, ok := s.locate(x, z)
	return ok
}

// locate resolves global x/z to the chunk holding them and the local
// coordinates within it.
func (s *Snapshot) locate(x, z int) (chunk *world.Chunk, lx, lz int, ok bool) {
	cx, lx := world.ChunkAndLocal(x)
	cz, lz := world.ChunkAndLocal(z)

	dx, dz := cx-s.center.X, cz-s.center.Z
	if dx < -1 || dx > 1 || dz < -1 || dz > 1 {
		return nil, 0, 0, false
	}

	i := index(dx, dz)
	if !s.loaded[i] {
		return nil, 0, 0, false
	}
	return &s.storage[i], lx, lz, true
}

// index maps a neighbour offset in -1..1 onto a slot.
func index(dx, dz int) int {
	return (dx+1)*span + (dz + 1)
}
