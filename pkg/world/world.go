package world

import (
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
)

// World manages the collection of chunks and global world logic.
// It also holds the saveable state like player data and time.
type World struct {
	Chunks      map[ChunkCoord]*Chunk
	PlayerState *PlayerState
	TimeOfDay   *TimeOfDay
}

// ChunkCoord represents the grid coordinates of a chunk.
type ChunkCoord struct {
	X, Z int
}

// NewWorld creates a new empty World with initialized state.
func NewWorld() *World {
	return &World{
		Chunks:      make(map[ChunkCoord]*Chunk),
		PlayerState: NewPlayerState(),
		TimeOfDay:   NewTimeOfDay(),
	}
}

// GenerateFixedGrid generates a 3x3 grid of chunks centered at 0,0.
// This is for initial testing/debugging.
func (w *World) GenerateFixedGrid() {
	for x := -1; x <= 1; x++ {
		for z := -1; z <= 1; z++ {
			chunk := NewChunk(x, z)
			// Fill with some data
			w.fillChunkDebug(chunk)
			w.Chunks[ChunkCoord{X: x, Z: z}] = chunk
		}
	}
}

// fillChunkDebug fills the chunk with random noise for testing.
// TODO: Move this to generator.go when refactoring is complete
func (w *World) fillChunkDebug(c *Chunk) {
	for x := range config.ChunkWidth {
		for z := range config.ChunkWidth {
			height := 32
			for y := 0; y < height; y++ {
				if y < height-3 {
					c.SetBlock(x, y, z, blocks.Stone)
				} else if y < height-1 {
					c.SetBlock(x, y, z, blocks.Dirt)
				} else {
					c.SetBlock(x, y, z, blocks.Grass)
				}
			}
		}
	}
}

// GetBlock returns the block type at the global world coordinates.
func (w *World) GetBlock(x, y, z int) *blocks.Block {
	chunkX, localX := worldToLocal(x)
	chunkZ, localZ := worldToLocal(z)

	chunk, exists := w.Chunks[ChunkCoord{X: chunkX, Z: chunkZ}]
	if !exists {
		return nil
	}
	return chunk.GetBlock(localX, y, localZ)
}

// GetBlockState returns the block type and per-instance metadata at the given global coordinates.
// Returns (nil, 0) if the chunk is missing or the position is air/out of bounds.
func (w *World) GetBlockState(x, y, z int) (*blocks.Block, uint8) {
	chunkX, localX := worldToLocal(x)
	chunkZ, localZ := worldToLocal(z)

	chunk, exists := w.Chunks[ChunkCoord{X: chunkX, Z: chunkZ}]
	if !exists {
		return nil, 0
	}
	return chunk.GetBlockState(localX, y, localZ)
}

// GetLight returns the light level at the global world coordinates.
func (w *World) GetLight(x, y, z int) uint8 {
	chunkX, localX := worldToLocal(x)
	chunkZ, localZ := worldToLocal(z)

	chunk, exists := w.Chunks[ChunkCoord{X: chunkX, Z: chunkZ}]
	if !exists {
		return 15 // Default to full light if chunk is missing? Or 0? 15 implies open sky.
	}
	return chunk.GetLight(localX, y, localZ)
}

// SetLight sets the light level at the global world coordinates.
func (w *World) SetLight(x, y, z int, level uint8) {
	chunkX, localX := worldToLocal(x)
	chunkZ, localZ := worldToLocal(z)

	chunk, exists := w.Chunks[ChunkCoord{X: chunkX, Z: chunkZ}]
	if !exists {
		return
	}
	chunk.SetLight(localX, y, localZ, level)
}

// GetChunk returns the chunk at the given chunk coordinates.
func (w *World) GetChunk(x, z int) *Chunk {
	return w.Chunks[ChunkCoord{X: x, Z: z}]
}

// SetBlock sets the block type at the global world coordinates.
// Returns a list of chunk coordinates that need to be re-meshed.
func (w *World) SetBlock(x, y, z int, b *blocks.Block) []ChunkCoord {
	return w.SetBlockState(x, y, z, b, 0)
}

// SetBlockState sets the block type and per-instance metadata at the given global coordinates.
// Returns a list of chunk coordinates that need to be re-meshed.
func (w *World) SetBlockState(x, y, z int, b *blocks.Block, meta uint8) []ChunkCoord {
	if y < 0 || y >= config.ChunkHeight {
		return nil
	}

	chunkX, localX := worldToLocal(x)
	chunkZ, localZ := worldToLocal(z)

	coord := ChunkCoord{X: chunkX, Z: chunkZ}
	chunk, exists := w.Chunks[coord]
	if !exists {
		return nil
	}

	if !chunk.SetBlockState(localX, y, localZ, b, meta) {
		return nil
	}

	// Pre-allocate slice for chunk + four potential neighbors.
	affected := make([]ChunkCoord, 1, 5)
	affected[0] = coord

	// Check neighbors if on border
	switch localX {
	case 0:
		affected = append(affected, ChunkCoord{X: chunkX - 1, Z: chunkZ})
	case config.ChunkWidth - 1:
		affected = append(affected, ChunkCoord{X: chunkX + 1, Z: chunkZ})
	}

	switch localZ {
	case 0:
		affected = append(affected, ChunkCoord{X: chunkX, Z: chunkZ - 1})
	case config.ChunkWidth - 1:
		affected = append(affected, ChunkCoord{X: chunkX, Z: chunkZ + 1})
	}

	return affected
}

// worldToLocal converts a global coordinate to chunk and local coordinates.
// It correctly handles negative coordinates using floor division logic.
func worldToLocal(v int) (chunk, local int) {
	chunk = v / config.ChunkWidth
	local = v % config.ChunkWidth

	if local < 0 {
		chunk--
		local += config.ChunkWidth
	}
	return
}
