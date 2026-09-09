package world

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
)

func TestChunk_SetGetBlock(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	chunk := NewChunk(0, 0)

	// Test in bounds
	if !chunk.SetBlock(0, 0, 0, stone) {
		t.Error("Failed to set block at 0,0,0")
	}
	if chunk.GetBlock(0, 0, 0) != stone {
		t.Errorf("Expected Stone, got %v", chunk.GetBlock(0, 0, 0))
	}

	// Test out of bounds
	if chunk.SetBlock(-1, 0, 0, stone) {
		t.Error("SetBlock should fail for negative x")
	}
	if chunk.GetBlock(-1, 0, 0) != nil {
		t.Error("GetBlock should return nil for out of bounds")
	}
}

// TestChunk_HighestSolidY exercises the incremental maintenance
// lighting.seedSkyLight's ceiling optimization depends on: raising it is O(1)
// on every solid block placed above the current maximum, and lowering it —
// only possible by removing the single highest solid block in the chunk —
// must correctly find the next-highest one below it, including when another
// block at the same Y (a different column) is what that turns out to be.
func TestChunk_HighestSolidY(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	c := NewChunk(0, 0)

	if got := c.HighestSolidY(); got != -1 {
		t.Fatalf("HighestSolidY on a fresh chunk = %d, want -1", got)
	}

	c.SetBlock(0, 5, 0, stone)
	if got := c.HighestSolidY(); got != 5 {
		t.Fatalf("after placing at y=5, HighestSolidY = %d, want 5", got)
	}

	c.SetBlock(3, 10, 3, stone)
	if got := c.HighestSolidY(); got != 10 {
		t.Fatalf("after placing a taller block, HighestSolidY = %d, want 10", got)
	}

	// A second block at the same height as the current maximum, in a
	// different column: removing the first must fall back to this one rather
	// than to the completely wrong assumption that y=10 has become empty.
	c.SetBlock(7, 10, 7, stone)

	// Removing an interior block (not the tallest) must not move the
	// maximum at all.
	c.SetBlock(0, 5, 0, nil)
	if got := c.HighestSolidY(); got != 10 {
		t.Fatalf("after removing an unrelated lower block, HighestSolidY = %d, want unchanged 10", got)
	}

	// Removing one of the two blocks at the current maximum must find the
	// other one still at y=10, not rescan past it.
	c.SetBlock(3, 10, 3, nil)
	if got := c.HighestSolidY(); got != 10 {
		t.Fatalf("after removing one of two blocks at the maximum, HighestSolidY = %d, want still 10 (the other survives)", got)
	}

	// Removing the last block at the maximum must fall through every empty
	// layer above what remains and land on it.
	c.SetBlock(7, 10, 7, nil)
	if got := c.HighestSolidY(); got != -1 {
		t.Fatalf("after clearing the only remaining block, HighestSolidY = %d, want -1 (chunk is empty)", got)
	}
}

// TestChunk_HighestSolidY_LightTransparentBlockDoesNotRaiseIt is the M18
// sky-ceiling/light-predicate tie, checked directly rather than trusted: a
// light-transparent block (leaves) is a real, solid, non-air block, but
// highestSolidY is defined as "the highest block the light engine considers
// opaque" (blocks.OpaqueToLight), not "the highest non-air block". If a leaf
// raised this value, the ceiling optimization in
// world/lighting.seedSkyLight would stop doing its job around every tree
// without necessarily producing wrong *pixels* (see this test's sibling
// below and the design decision in
// docs/milestones/M18-vegetation-features.md) — so this asserts the value
// itself, which is the one place that mutation is directly observable.
//
// A mutation of SetBlock/SetBlockState back to deciding solidity from
// `block != nil` instead of blocks.OpaqueToLight makes this test fail: the
// leaf below would raise highestSolidY to 5 instead of leaving it at the
// opaque block's height of 2.
func TestChunk_HighestSolidY_LightTransparentBlockDoesNotRaiseIt(t *testing.T) {
	blocks.Reset()
	opaque := blocks.Register(&blocks.Block{ID: "test/opaque", Name: "Opaque"})
	leaf := blocks.Register(&blocks.Block{ID: "test/leaf", Name: "Leaf", LightTransparent: true})

	c := NewChunk(0, 0)

	c.SetBlock(0, 2, 0, opaque)
	if got := c.HighestSolidY(); got != 2 {
		t.Fatalf("after placing an opaque block at y=2, HighestSolidY = %d, want 2", got)
	}

	// A light-transparent block placed higher than the opaque one must NOT
	// raise the ceiling: a column through it is genuinely open sky at full
	// brightness all the way down to the real opaque block beneath it.
	c.SetBlock(0, 5, 0, leaf)
	if got := c.HighestSolidY(); got != 2 {
		t.Fatalf("after placing a light-transparent block at y=5, HighestSolidY = %d, want unchanged 2 "+
			"(a light-transparent block must never raise the sky ceiling)", got)
	}

	// Removing the opaque block leaves only the leaf, which the ceiling must
	// treat as if the chunk were entirely open air (-1), not fall back to the
	// leaf's own height.
	c.SetBlock(0, 2, 0, nil)
	if got := c.HighestSolidY(); got != -1 {
		t.Fatalf("after removing the only opaque block (leaving a light-transparent one at y=5), HighestSolidY = %d, want -1", got)
	}

	// SetBlockState must derive the same rule, not just SetBlock.
	c2 := NewChunk(1, 0)
	c2.SetBlockState(0, 8, 0, leaf, 0)
	if got := c2.HighestSolidY(); got != -1 {
		t.Fatalf("SetBlockState: after placing only a light-transparent block, HighestSolidY = %d, want -1", got)
	}
	c2.SetBlockState(0, 3, 0, opaque, 0)
	if got := c2.HighestSolidY(); got != 3 {
		t.Fatalf("SetBlockState: after placing an opaque block at y=3 below the leaf, HighestSolidY = %d, want 3", got)
	}
}

func TestWorld_GetBlock_Global(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	dirt := blocks.Dirt
	blocks.Register(stone)
	blocks.Register(dirt)

	w := NewWorld()
	// Manually add a chunk at 0,0
	chunk00 := NewChunk(0, 0)
	chunk00.SetBlock(0, 0, 0, stone)
	w.Chunks[ChunkCoord{0, 0}] = chunk00

	// Manually add a chunk at 1,0 (x=16 to 31)
	chunk10 := NewChunk(1, 0)
	chunk10.SetBlock(0, 0, 0, dirt) // Global 16,0,0
	w.Chunks[ChunkCoord{1, 0}] = chunk10

	// Test retrieval
	if w.GetBlock(0, 0, 0) != stone {
		t.Errorf("Expected Stone at 0,0,0")
	}
	if w.GetBlock(16, 0, 0) != dirt {
		t.Errorf("Expected Dirt at 16,0,0")
	}

	// Test non-existent chunk
	if w.GetBlock(32, 0, 0) != nil {
		t.Errorf("Expected nil at 32,0,0")
	}
}
