// Package testutil builds synthetic worlds for tests.
//
// Lighting and meshing tests are only readable if the world they run against is
// described in one or two lines. A test that opens with forty SetBlock calls
// hides the scenario it is actually checking. The builder here composes worlds
// from two primitives, Fill and Clear, so scenarios such as an overhang, a
// vertical shaft or a sealed cave are expressed directly at the call site.
package testutil

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// Box is an inclusive axis-aligned region in global world coordinates.
type Box struct {
	MinX, MinY, MinZ int
	MaxX, MaxY, MaxZ int
}

// WorldBuilder incrementally assembles a world for a test.
// The zero value is not usable; call NewWorld.
type WorldBuilder struct {
	t *testing.T
	w *world.World
}

// NewWorld starts a builder holding an empty world with no chunks.
// It resets the global block registry to the vanilla set, so tests are
// independent of registry mutations made by earlier tests in the same package.
//
// That reset mutates process-wide state, so tests using this builder must not
// call t.Parallel.
func NewWorld(t *testing.T) *WorldBuilder {
	t.Helper()
	blocks.ResetToVanilla()

	return &WorldBuilder{t: t, w: world.NewWorld()}
}

// Chunks allocates an inclusive rectangle of empty chunks in the chunk grid.
// Coordinates are chunk indices, not block positions.
func (b *WorldBuilder) Chunks(minCX, minCZ, maxCX, maxCZ int) *WorldBuilder {
	b.t.Helper()

	if maxCX < minCX || maxCZ < minCZ {
		b.t.Fatalf("Chunks: empty range (%d,%d)..(%d,%d)", minCX, minCZ, maxCX, maxCZ)
	}

	for cx := minCX; cx <= maxCX; cx++ {
		for cz := minCZ; cz <= maxCZ; cz++ {
			coord := world.ChunkCoord{X: cx, Z: cz}
			if _, exists := b.w.Chunks[coord]; exists {
				continue
			}
			b.w.Chunks[coord] = world.NewChunk(cx, cz)
		}
	}
	return b
}

// Flat lays stone/dirt/grass columns across every allocated chunk, with the
// topmost grass block at surfaceY. Everything above surfaceY is left as air.
func (b *WorldBuilder) Flat(surfaceY int) *WorldBuilder {
	b.t.Helper()

	if surfaceY < 0 || surfaceY >= config.ChunkHeight {
		b.t.Fatalf("Flat: surfaceY %d outside 0..%d", surfaceY, config.ChunkHeight-1)
	}
	if len(b.w.Chunks) == 0 {
		b.t.Fatal("Flat: no chunks allocated; call Chunks first")
	}

	for _, chunk := range b.w.Chunks {
		for x := range config.ChunkWidth {
			for z := range config.ChunkWidth {
				for y := 0; y <= surfaceY; y++ {
					chunk.SetBlock(x, y, z, blockForDepth(surfaceY-y))
				}
			}
		}
	}
	return b
}

// blockForDepth picks the terrain block that many blocks below the surface.
func blockForDepth(depth int) *blocks.Block {
	switch {
	case depth == 0:
		return blocks.Grass
	case depth <= 2:
		return blocks.Dirt
	default:
		return blocks.Stone
	}
}

// Fill sets every position in box to blk. Positions in unallocated chunks are
// a test authoring mistake and fail the test rather than being skipped.
func (b *WorldBuilder) Fill(box Box, blk *blocks.Block) *WorldBuilder {
	b.t.Helper()
	b.forEach(box, func(chunk *world.Chunk, lx, y, lz int) {
		chunk.SetBlock(lx, y, lz, blk)
	})
	return b
}

// Clear sets every position in box to air. Use it to carve caves, shafts and
// the space underneath an overhang.
func (b *WorldBuilder) Clear(box Box) *WorldBuilder {
	return b.Fill(box, nil)
}

// Build returns the assembled world.
func (b *WorldBuilder) Build() *world.World {
	return b.w
}

// forEach walks every position in box, resolving each to its owning chunk.
func (b *WorldBuilder) forEach(box Box, fn func(chunk *world.Chunk, lx, y, lz int)) {
	b.t.Helper()

	if box.MaxX < box.MinX || box.MaxY < box.MinY || box.MaxZ < box.MinZ {
		b.t.Fatalf("box has an empty range: %+v", box)
	}
	if box.MinY < 0 || box.MaxY >= config.ChunkHeight {
		b.t.Fatalf("box Y range %d..%d outside 0..%d", box.MinY, box.MaxY, config.ChunkHeight-1)
	}

	for x := box.MinX; x <= box.MaxX; x++ {
		cx, lx := world.ChunkAndLocal(x)
		for z := box.MinZ; z <= box.MaxZ; z++ {
			cz, lz := world.ChunkAndLocal(z)

			chunk := b.w.GetChunk(cx, cz)
			if chunk == nil {
				b.t.Fatalf("box covers unallocated chunk (%d,%d) at x=%d z=%d", cx, cz, x, z)
			}

			for y := box.MinY; y <= box.MaxY; y++ {
				fn(chunk, lx, y, lz)
			}
		}
	}
}
