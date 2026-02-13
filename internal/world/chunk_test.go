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
