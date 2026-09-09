package world

import (
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
)

// Chunk represents a 16x16x256 section of the world.
// It stores block data in a flat array for cache locality.
type Chunk struct {
	Blocks     [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]blocks.NumID
	Meta       [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]uint8
	SkyLight   [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]uint8
	BlockLight [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]uint8
	X, Z       int // Chunk coordinates in the world grid (not world position)

	// highestSolidY is the Y of the highest block anywhere in the chunk that
	// the light engine considers opaque (blocks.OpaqueToLight), or -1 if no
	// such block exists. It is a single scalar for the whole chunk, not per
	// column: lighting's column-scan ceiling optimization (see
	// lighting.seedSkyLight) only needs "how high does this chunk's terrain
	// reach at most", not a per-column height map.
	//
	// It is deliberately defined as "opaque to light", not "non-air": M18
	// added leaves, which are a real, solid, non-air block that light passes
	// through anyway. A column through a canopy is genuinely full-brightness
	// sky all the way down, so a leaf must not raise this value — if it did,
	// the ceiling optimisation would still be *correct* (it only ever widens
	// the set of cells enqueued, never narrows it below the true ceiling) but
	// it would stop doing its job, since a leaf-block-tall "ceiling" would
	// reappear in every chunk with a tree in its neighbourhood. Both
	// SetBlock and SetBlockState below derive solidity from
	// blocks.OpaqueToLightID -- a lock-free cache of the exact same
	// blocks.OpaqueToLight function world/lighting.isTransparent negates,
	// see that function's doc comment for why the cache exists -- rather
	// than from a local `block != nil` check, specifically so the two
	// cannot silently disagree about what "opaque" means. See
	// docs/milestones/M18-vegetation-features.md's "light predicate and the
	// sky ceiling must be the same predicate" design decision.
	//
	// It is maintained incrementally by SetBlock/SetBlockState rather than
	// scanned on demand, since SeedChunk reads it on the hot chunk-loading
	// path. Raising it is O(1); lowering it (only possible by removing the
	// single highest opaque block in the whole chunk) rescans, which is rare
	// enough in practice not to matter.
	highestSolidY int
}

// NewChunk creates a new Chunk at the specified grid coordinates.
func NewChunk(x, z int) *Chunk {
	c := &Chunk{
		X:             x,
		Z:             z,
		highestSolidY: -1,
	}
	return c
}

// HighestSolidY returns the Y of the highest non-air block in the chunk, or
// -1 if the chunk is entirely air.
func (c *Chunk) HighestSolidY() int {
	return c.highestSolidY
}

// noteBlockChange updates highestSolidY after a block at height y changed
// from wasSolid to isSolid.
func (c *Chunk) noteBlockChange(y int, wasSolid, isSolid bool) {
	if isSolid == wasSolid {
		return
	}
	if isSolid {
		if y > c.highestSolidY {
			c.highestSolidY = y
		}
		return
	}
	// A solid block was removed. Only the chunk-wide maximum can be affected,
	// and only if the removed block was sitting at it.
	if y == c.highestSolidY {
		c.recomputeHighestSolidBelow(y)
	}
}

// recomputeHighestSolidBelow scans downward from from (inclusive) for the new
// chunk-wide highest solid block, after the block that used to sit at from
// was removed. from itself must still be checked, not skipped: another
// column can hold a solid block at that same Y, and removing one block does
// not clear the whole layer.
func (c *Chunk) recomputeHighestSolidBelow(from int) {
	for y := from; y >= 0; y-- {
		base := y * config.ChunkWidth * config.ChunkWidth
		for i := range config.ChunkWidth * config.ChunkWidth {
			if blocks.OpaqueToLightID(c.Blocks[base+i]) {
				c.highestSolidY = y
				return
			}
		}
	}
	c.highestSolidY = -1
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
	wasSolid := blocks.OpaqueToLightID(c.Blocks[index])
	c.Blocks[index] = blocks.NumericIDOf(block)
	// Reset meta for convenience. Per-instance meta should be set via SetBlockState.
	c.Meta[index] = 0
	c.noteBlockChange(y, wasSolid, blocks.OpaqueToLight(block))
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
	wasSolid := blocks.OpaqueToLightID(c.Blocks[index])
	id := blocks.NumericIDOf(block)
	c.Blocks[index] = id
	c.noteBlockChange(y, wasSolid, blocks.OpaqueToLight(block))
	if id == blocks.InvalidNumericID {
		c.Meta[index] = 0
		return true
	}
	c.Meta[index] = meta
	return true
}

// GetSkyLight returns the skylight level at the specified local coordinates.
// x, z: 0 to 15
// y: 0 to 255
// Returns 0 if coordinates are out of bounds.
func (c *Chunk) GetSkyLight(x, y, z int) uint8 {
	if !c.InBounds(x, y, z) {
		return 0
	}
	index := c.getBlockIndex(x, y, z)
	return c.SkyLight[index]
}

// SetSkyLight sets the skylight level at the specified local coordinates.
// Returns true if successful, false if coordinates are out of bounds.
func (c *Chunk) SetSkyLight(x, y, z int, level uint8) bool {
	if !c.InBounds(x, y, z) {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	c.SkyLight[index] = level
	return true
}

// GetBlockLight returns the block light level at the specified local coordinates.
// x, z: 0 to 15
// y: 0 to 255
// Returns 0 if coordinates are out of bounds.
func (c *Chunk) GetBlockLight(x, y, z int) uint8 {
	if !c.InBounds(x, y, z) {
		return 0
	}
	index := c.getBlockIndex(x, y, z)
	return c.BlockLight[index]
}

// SetBlockLight sets the block light level at the specified local coordinates.
// Returns true if successful, false if coordinates are out of bounds.
func (c *Chunk) SetBlockLight(x, y, z int, level uint8) bool {
	if !c.InBounds(x, y, z) {
		return false
	}
	index := c.getBlockIndex(x, y, z)
	c.BlockLight[index] = level
	return true
}

// getBlockIndex calculates the flat array index for 3D coordinates.
// index = x + z*width + y*width*depth
func (c *Chunk) getBlockIndex(x, y, z int) int {
	return x + z*config.ChunkWidth + y*config.ChunkWidth*config.ChunkWidth
}
