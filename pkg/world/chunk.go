package world

import (
	"github.com/nahharris/minae/pkg/config"
)

// BlockType represents the type of a voxel.
type BlockType uint8

const (
	// BlockAir represents an empty block.
	BlockAir BlockType = iota
	// BlockStone represents a stone block.
	BlockStone
	// BlockDirt represents a dirt block.
	BlockDirt
)

// Chunk represents a 16x16x256 section of the world.
// It stores block data in a flat array for cache locality.
type Chunk struct {
	Blocks [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]BlockType
	X, Z   int // Chunk coordinates in the world grid (not world position)
}

// NewChunk creates a new Chunk at the specified grid coordinates.
func NewChunk(x, z int) *Chunk {
	return &Chunk{
		X: x,
		Z: z,
	}
}

// GetBlock returns the block type at the specified local coordinates.
// x, z: 0 to 15
// y: 0 to 255
// Returns BlockAir if coordinates are out of bounds.
func (c *Chunk) GetBlock(x, y, z int) BlockType {
	if x < 0 || x >= config.ChunkWidth || y < 0 || y >= config.ChunkHeight || z < 0 || z >= config.ChunkWidth {
		return BlockAir
	}
	index := c.getBlockIndex(x, y, z)
	return c.Blocks[index]
}

// SetBlock sets the block type at the specified local coordinates.
// Returns true if successful, false if coordinates are out of bounds.
func (c *Chunk) SetBlock(x, y, z int, block BlockType) bool {
	if x < 0 || x >= config.ChunkWidth || y < 0 || y >= config.ChunkHeight || z < 0 || z >= config.ChunkWidth {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	c.Blocks[index] = block
	return true
}

// getBlockIndex calculates the flat array index for 3D coordinates.
// index = x + z*width + y*width*depth
func (c *Chunk) getBlockIndex(x, y, z int) int {
	return x + z*config.ChunkWidth + y*config.ChunkWidth*config.ChunkWidth
}
