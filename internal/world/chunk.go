package world

import (
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
)

// Chunk represents a 16x16x256 section of the world.
// It stores block data in a flat array for cache locality.
type Chunk struct {
	Blocks   [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]blocks.NumID
	Meta     [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]uint8
	LightMap [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]uint8
	X, Z     int // Chunk coordinates in the world grid (not world position)
	meshHint chunkMeshHint
}

// NewChunk creates a new Chunk at the specified grid coordinates.
func NewChunk(x, z int) *Chunk {
	c := &Chunk{
		X: x,
		Z: z,
	}
	return c
}

// ChunkX returns the X coordinate of the chunk.
func (c *Chunk) ChunkX() int {
	return c.X
}

// ChunkZ returns the Z coordinate of the chunk.
func (c *Chunk) ChunkZ() int {
	return c.Z
}

func (c *Chunk) InBounds(x, y, z int) bool {
	return x >= 0 && x < config.ChunkWidth && y >= 0 && y < config.ChunkHeight && z >= 0 && z < config.ChunkWidth
}

// GetBlock returns the block type at the specified local coordinates.
// x, z: 0 to 15
// y: 0 to 255
// Returns nil (Air) if coordinates are out of bounds or block is nil.
func (c *Chunk) GetBlock(x, y, z int) *blocks.Block {
	if !c.InBounds(x, y, z) {
		return nil
	}
	index := c.getBlockIndex(x, y, z)
	return blocks.FromNumericID(c.Blocks[index])
}

// GetBlockMeta returns the block instance metadata at the specified local coordinates.
// Returns 0 if coordinates are out of bounds or the block is air.
func (c *Chunk) GetBlockMeta(x, y, z int) uint8 {
	if !c.InBounds(x, y, z) {
		return 0
	}
	index := c.getBlockIndex(x, y, z)
	if blocks.FromNumericID(c.Blocks[index]) == nil {
		return 0
	}
	return c.Meta[index]
}

// GetBlockState returns the block type and per-instance metadata at the specified local coordinates.
// Returns (nil, 0) (Air) if coordinates are out of bounds or the block is air.
func (c *Chunk) GetBlockState(x, y, z int) (*blocks.Block, uint8) {
	if !c.InBounds(x, y, z) {
		return nil, 0
	}
	index := c.getBlockIndex(x, y, z)
	b := blocks.FromNumericID(c.Blocks[index])
	if b == nil {
		return nil, 0
	}
	return b, c.Meta[index]
}

// SetBlock sets the block type at the specified local coordinates.
// Returns true if successful, false if coordinates are out of bounds.
func (c *Chunk) SetBlock(x, y, z int, block *blocks.Block) bool {
	if !c.InBounds(x, y, z) {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	c.Blocks[index] = blocks.NumericIDOf(block)
	// Reset meta for convenience. Per-instance meta should be set via SetBlockState.
	c.Meta[index] = 0
	return true
}

// SetBlockMeta sets per-instance metadata for the specified local coordinates.
// Returns false if out of bounds or the block is air.
func (c *Chunk) SetBlockMeta(x, y, z int, meta uint8) bool {
	if !c.InBounds(x, y, z) {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	if blocks.FromNumericID(c.Blocks[index]) == nil {
		c.Meta[index] = 0
		return false
	}
	c.Meta[index] = meta
	return true
}

// SetBlockState sets the block type and per-instance metadata at the specified local coordinates.
// If block is air, meta is cleared to 0.
func (c *Chunk) SetBlockState(x, y, z int, block *blocks.Block, meta uint8) bool {
	if !c.InBounds(x, y, z) {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	id := blocks.NumericIDOf(block)
	c.Blocks[index] = id
	if id == blocks.InvalidNumericID {
		c.Meta[index] = 0
		return true
	}
	c.Meta[index] = meta
	return true
}

// GetLight returns the light level at the specified local coordinates.
// x, z: 0 to 15
// y: 0 to 255
// Returns 0 if coordinates are out of bounds.
func (c *Chunk) GetLight(x, y, z int) uint8 {
	if !c.InBounds(x, y, z) {
		return 0
	}
	index := c.getBlockIndex(x, y, z)
	return c.LightMap[index]
}

// SetLight sets the light level at the specified local coordinates.
// Returns true if successful, false if coordinates are out of bounds.
func (c *Chunk) SetLight(x, y, z int, level uint8) bool {
	if !c.InBounds(x, y, z) {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	c.LightMap[index] = level
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
