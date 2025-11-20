package world

import (
	"testing"
)

func TestCalculateChunkMesh_Culling(t *testing.T) {
	w := NewWorld()
	c := NewChunk(0, 0)
	w.Chunks[ChunkCoord{0, 0}] = c

	// Place a single block at 8,8,8
	// It should have all 6 faces visible (since neighbors are air)
	c.SetBlock(8, 8, 8, BlockStone)

	data := CalculateChunkMesh(c, w)
	
	if data == nil {
		t.Fatal("Expected mesh data, got nil")
	}

	// 6 faces * 2 triangles * 3 vertices = 36 vertices
	expectedVertices := 6 * 6
	if len(data.Vertices)/3 != expectedVertices {
		t.Errorf("Expected %d vertices, got %d", expectedVertices, len(data.Vertices)/3)
	}

	// Now place a block on top (8,9,8)
	c.SetBlock(8, 9, 8, BlockStone)
	
	// Recalculate
	data = CalculateChunkMesh(c, w)
	
	// Bottom block (8,8,8): Top face hidden. 5 faces.
	// Top block (8,9,8): Bottom face hidden. 5 faces.
	// Total 10 faces * 6 verts = 60 vertices.
	expectedVertices = 10 * 6
	if len(data.Vertices)/3 != expectedVertices {
		t.Errorf("Expected %d vertices, got %d", expectedVertices, len(data.Vertices)/3)
	}
}

func TestCalculateChunkMesh_Empty(t *testing.T) {
	w := NewWorld()
	c := NewChunk(0, 0)
	w.Chunks[ChunkCoord{0, 0}] = c
	
	data := CalculateChunkMesh(c, w)
	if data != nil {
		t.Error("Expected nil mesh for empty chunk")
	}
}

