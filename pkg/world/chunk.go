package world

import (
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
)

// Chunk represents a 16x16x256 section of the world.
// It stores compact block IDs in a flat array for cache locality.
type Chunk struct {
	Blocks [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]blocks.NumericID
	X, Z   int // Chunk coordinates in the world grid (not world position)

	meshHint chunkMeshHint
}

// NewChunk creates a new Chunk at the specified grid coordinates.
func NewChunk(x, z int) *Chunk {
	c := &Chunk{
		X: x,
		Z: z,
	}
	// Initialize with Air (assuming air is nil or a specific block)
	// If we want explicit air blocks, we should set them here.
	// For now, let's assume nil means "Air" or default, but better to be explicit if possible.
	// However, since we are using pointers, nil is a valid state for "nothing".
	// But "Air" is usually a block type. Let's try to use the "minae/air" block if available,
	// or handle nil as air.
	// For safety, let's fill with nil and handle nil as Air in GetBlock.
	return c
}

// GetBlock returns the block type at the specified local coordinates.
// x, z: 0 to 15
// y: 0 to 255
// Returns nil (Air) if coordinates are out of bounds or block is nil.
func (c *Chunk) GetBlock(x, y, z int) *blocks.Block {
	if x < 0 || x >= config.ChunkWidth || y < 0 || y >= config.ChunkHeight || z < 0 || z >= config.ChunkWidth {
		return nil
	}
	index := c.getBlockIndex(x, y, z)
	return blocks.FromNumericID(c.Blocks[index])
}

// SetBlock sets the block type at the specified local coordinates.
// Returns true if successful, false if coordinates are out of bounds.
func (c *Chunk) SetBlock(x, y, z int, block *blocks.Block) bool {
	if x < 0 || x >= config.ChunkWidth || y < 0 || y >= config.ChunkHeight || z < 0 || z >= config.ChunkWidth {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	c.Blocks[index] = blocks.NumericIDOf(block)
	return true
}

// getBlockIndex calculates the flat array index for 3D coordinates.
// index = x + z*width + y*width*depth
func (c *Chunk) getBlockIndex(x, y, z int) int {
	return x + z*config.ChunkWidth + y*config.ChunkWidth*config.ChunkWidth
}

type chunkMeshHint struct {
	vertices  int
	texcoords int
	normals   int
	colors    int
}
