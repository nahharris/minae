package world

import (
	"testing"
)

func TestChunk_SetGetBlock(t *testing.T) {
	chunk := NewChunk(0, 0)

	// Test in bounds
	if !chunk.SetBlock(0, 0, 0, BlockStone) {
		t.Error("Failed to set block at 0,0,0")
	}
	if chunk.GetBlock(0, 0, 0) != BlockStone {
		t.Errorf("Expected BlockStone, got %v", chunk.GetBlock(0, 0, 0))
	}

	// Test out of bounds
	if chunk.SetBlock(-1, 0, 0, BlockStone) {
		t.Error("SetBlock should fail for negative x")
	}
	if chunk.GetBlock(-1, 0, 0) != BlockAir {
		t.Error("GetBlock should return Air for out of bounds")
	}
}

func TestWorld_GetBlock_Global(t *testing.T) {
	w := NewWorld()
	// Manually add a chunk at 0,0
	chunk00 := NewChunk(0, 0)
	chunk00.SetBlock(0, 0, 0, BlockStone)
	w.Chunks[ChunkCoord{0, 0}] = chunk00

	// Manually add a chunk at 1,0 (x=16 to 31)
	chunk10 := NewChunk(1, 0)
	chunk10.SetBlock(0, 0, 0, BlockDirt) // Global 16,0,0
	w.Chunks[ChunkCoord{1, 0}] = chunk10

	// Test retrieval
	if w.GetBlock(0, 0, 0) != BlockStone {
		t.Errorf("Expected BlockStone at 0,0,0")
	}
	if w.GetBlock(16, 0, 0) != BlockDirt {
		t.Errorf("Expected BlockDirt at 16,0,0")
	}

	// Test non-existent chunk
	if w.GetBlock(32, 0, 0) != BlockAir {
		t.Errorf("Expected BlockAir at 32,0,0")
	}
}

