package world

import (
	"math"

	"github.com/nahharris/minae/pkg/config"
)

// World manages the collection of chunks and global world logic.
type World struct {
	Chunks map[ChunkCoord]*Chunk
}

// ChunkCoord represents the grid coordinates of a chunk.
type ChunkCoord struct {
	X, Z int
}

// NewWorld creates a new empty World.
func NewWorld() *World {
	return &World{
		Chunks: make(map[ChunkCoord]*Chunk),
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
func (w *World) fillChunkDebug(c *Chunk) {
	for x := range config.ChunkWidth {
		for z := range config.ChunkWidth {
			// Simple terrain height
			height := 32 // flat for now, or maybe random?
			// Let's just fill up to 32 for solid ground
			for y := 0; y < height; y++ {
				if y < height-3 {
					c.SetBlock(x, y, z, BlockStone)
				} else {
					c.SetBlock(x, y, z, BlockDirt)
				}
			}
		}
	}
}

// GetBlock returns the block type at the global world coordinates.
func (w *World) GetBlock(x, y, z int) BlockType {
	chunkX := int(math.Floor(float64(x) / float64(config.ChunkWidth)))
	chunkZ := int(math.Floor(float64(z) / float64(config.ChunkWidth)))

	localX := x - chunkX*config.ChunkWidth
	localZ := z - chunkZ*config.ChunkWidth

	// Fix negative modulo issues if strictly using modulo
	// But the subtraction above handles it if chunkX is calculated with Floor correctly.
	// Example: x = -1. chunkX = -1. localX = -1 - (-16) = 15. Correct.

	chunk, exists := w.Chunks[ChunkCoord{X: chunkX, Z: chunkZ}]
	if !exists {
		return BlockAir
	}
	return chunk.GetBlock(localX, y, localZ)
}

// GetChunk returns the chunk at the given chunk coordinates.
func (w *World) GetChunk(x, z int) *Chunk {
	return w.Chunks[ChunkCoord{X: x, Z: z}]
}
