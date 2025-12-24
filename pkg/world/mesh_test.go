package world

import (
	"testing"

	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/render/atlas"
)

func TestCalculateChunkMesh_Culling(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := NewWorld()
	c := NewChunk(0, 0)
	w.Chunks[ChunkCoord{0, 0}] = c

	// Place a single block at 8,8,8
	// It should have all 6 faces visible (since neighbors are air)
	c.SetBlock(8, 8, 8, stone)

	data := CalculateChunkMesh(c, w, nil)

	if data == nil {
		t.Fatal("Expected mesh data, got nil")
	}

	// 6 faces * 2 triangles * 3 vertices = 36 vertices
	expectedVertices := 6 * 6
	if len(data.Vertices)/3 != expectedVertices {
		t.Errorf("Expected %d vertices, got %d", expectedVertices, len(data.Vertices)/3)
	}

	// Now place a block on top (8,9,8)
	c.SetBlock(8, 9, 8, stone)

	// Recalculate
	data = CalculateChunkMesh(c, w, nil)

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

	data := CalculateChunkMesh(c, w, nil)
	if data != nil {
		t.Error("Expected nil mesh for empty chunk")
	}
}

func TestCalculateChunkMesh_Slab_CullingDependsOnOverlap(t *testing.T) {
	blocks.Reset()

	slab := blocks.Register(&blocks.Block{
		ID:    "test/slab",
		Name:  "Slab",
		Color: 0xFFFFFFFF,
		ModelSpec: blocks.ModelSpec{
			Type: "slab",
		},
	})

	w := NewWorld()
	c := NewChunk(0, 0)
	w.Chunks[ChunkCoord{0, 0}] = c

	set := func(x int, meta uint8) {
		ok := c.SetBlockState(x, 8, 8, slab, meta)
		if !ok {
			t.Fatalf("failed to set slab at %d,8,8", x)
		}
	}

	t.Run("bottom+bottom_sharedFaceCulled", func(t *testing.T) {
		set(8, 0)
		set(9, 0)

		data := CalculateChunkMesh(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}

		// Two slabs (6 quads each => 72 verts) minus 2 culled quads on shared boundary => 60 verts.
		if got, want := len(data.Vertices)/3, 60; got != want {
			t.Fatalf("expected %d vertices, got %d", want, got)
		}
	})

	t.Run("bottom+top_sharedFaceVisible", func(t *testing.T) {
		// Clear the chunk.
		c = NewChunk(0, 0)
		w.Chunks[ChunkCoord{0, 0}] = c

		set = func(x int, meta uint8) {
			ok := c.SetBlockState(x, 8, 8, slab, meta)
			if !ok {
				t.Fatalf("failed to set slab at %d,8,8", x)
			}
		}

		set(8, 0)
		set(9, blocks.MetaSlabTopBit)

		data := CalculateChunkMesh(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}

		// Shared boundary is only partially occluded => both quads remain => 72 verts.
		if got, want := len(data.Vertices)/3, 72; got != want {
			t.Fatalf("expected %d vertices, got %d", want, got)
		}
	})
}

type dummyUVLookup struct {
	uv atlas.UV
}

func (d dummyUVLookup) UV(key string) (atlas.UV, bool) {
	if key == "test/tex" {
		return d.uv, true
	}
	return atlas.UV{}, false
}

func TestCalculateChunkMesh_UsesUVLookup(t *testing.T) {
	blocks.Reset()
	texBlock := blocks.Register(&blocks.Block{
		ID:        "test/tex",
		Name:      "Tex",
		Color:     0xFFFFFFFF,
		ModelSpec: blocks.ModelSpec{Type: "full"},
	})

	w := NewWorld()
	c := NewChunk(0, 0)
	w.Chunks[ChunkCoord{0, 0}] = c

	c.SetBlock(0, 0, 0, texBlock)

	lookup := dummyUVLookup{
		uv: atlas.UV{U0: 0.25, V0: 0.5, U1: 0.5, V1: 0.75},
	}

	data := CalculateChunkMesh(c, w, lookup)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// Verify the mesh used the UV lookup by asserting the first quad's UVs are within the returned rect.
	// We don't assert exact ordering because per-face orientation can flip/rotate UVs.
	if len(data.Texcoords) < 12 {
		t.Fatalf("expected at least 12 texcoords, got %d", len(data.Texcoords))
	}
	got := data.Texcoords[:12]

	eps := float32(1e-6)
	for i := 0; i < len(got); i += 2 {
		u := got[i]
		v := got[i+1]
		if u < lookup.uv.U0-eps || u > lookup.uv.U1+eps {
			t.Fatalf("u out of range at %d: got %.5f, want within [%.5f, %.5f]", i, u, lookup.uv.U0, lookup.uv.U1)
		}
		if v < lookup.uv.V0-eps || v > lookup.uv.V1+eps {
			t.Fatalf("v out of range at %d: got %.5f, want within [%.5f, %.5f]", i+1, v, lookup.uv.V0, lookup.uv.V1)
		}
	}
}
