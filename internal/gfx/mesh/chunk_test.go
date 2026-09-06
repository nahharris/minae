package mesh_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/blocks/model"
	"github.com/nahharris/minae/internal/gfx/atlas"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

func TestGenerateChunkMeshData_Culling(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	// Place a single block at 8,8,8
	// It should have all 6 faces visible (since neighbors are air)
	c.SetBlock(8, 8, 8, stone)

	data := mesh.GenerateChunkMeshData(c, w, nil)

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
	data = mesh.GenerateChunkMeshData(c, w, nil)

	// Bottom block (8,8,8): Top face hidden. 5 faces.
	// Top block (8,9,8): Bottom face hidden. 5 faces.
	// Total 10 faces * 6 verts = 60 vertices.
	expectedVertices = 10 * 6
	if len(data.Vertices)/3 != expectedVertices {
		t.Errorf("Expected %d vertices, got %d", expectedVertices, len(data.Vertices)/3)
	}
}

func TestGenerateChunkMeshData_Empty(t *testing.T) {
	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data != nil {
		t.Error("Expected nil mesh for empty chunk")
	}
}

func TestGenerateChunkMeshData_Slab_CullingDependsOnOverlap(t *testing.T) {
	blocks.Reset()

	slab := blocks.Register(&blocks.Block{
		ID:    "test/slab",
		Name:  "Slab",
		Color: 0xFFFFFFFF,
		ModelSpec: model.ModelSpec{
			Type: "slab",
		},
	})

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	set := func(x int, meta uint8) {
		ok := c.SetBlockState(x, 8, 8, slab, meta)
		if !ok {
			t.Fatalf("failed to set slab at %d,8,8", x)
		}
	}

	t.Run("bottom+bottom_sharedFaceCulled", func(t *testing.T) {
		set(8, 0)
		set(9, 0)

		data := mesh.GenerateChunkMeshData(c, w, nil)
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
		c = world.NewChunk(0, 0)
		w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

		set = func(x int, meta uint8) {
			ok := c.SetBlockState(x, 8, 8, slab, meta)
			if !ok {
				t.Fatalf("failed to set slab at %d,8,8", x)
			}
		}

		set(8, 0)
		set(9, model.MetaSlabTopBit)

		data := mesh.GenerateChunkMeshData(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}

		// Shared boundary is only partially occluded => both quads remain => 72 verts.
		if got, want := len(data.Vertices)/3, 72; got != want {
			t.Fatalf("expected %d vertices, got %d", want, got)
		}
	})
}

// faceColor returns the RGBA color bytes recorded for the first vertex of
// the face at the given index within data.Colors, assuming a "full" cube
// model where AppendQuads emits the six faces in the fixed order Right,
// Left, Top, Bottom, Front, Back and every vertex of a face shares the same
// color (see addQuad in builder.go).
func faceColor(data *mesh.ChunkMeshData, faceIndex int) (r, g, b, a uint8) {
	const bytesPerFace = 6 * 4 // 6 vertices, 4 color bytes each
	off := faceIndex * bytesPerFace
	return data.Colors[off], data.Colors[off+1], data.Colors[off+2], data.Colors[off+3]
}

// faceSkylight returns the R channel (skylight) recorded for the face at
// the given index within data.Colors.
func faceSkylight(data *mesh.ChunkMeshData, faceIndex int) uint8 {
	r, _, _, _ := faceColor(data, faceIndex)
	return r
}

const (
	faceRight = iota
	faceLeft
	faceTop
	faceBottom
	faceFront
	faceBack
)

// The light engine reports 0 for a chunk it hasn't loaded, since unloaded
// space must never look like open sky. But that correctness would render
// the outward-facing edge of the loaded world as a black wall, so the mesh
// builder substitutes full-bright (15) whenever a face's neighbor cell
// falls in a chunk that isn't loaded, while still using the engine's real
// value for faces whose neighbor is inside a loaded chunk.
func TestGenerateChunkMeshData_UnloadedNeighborFallsBackToFullBright(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	t.Run("neighbor_chunk_unloaded_getsFullBright", func(t *testing.T) {
		w := world.NewWorld()
		c := world.NewChunk(0, 0)
		w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

		// Block at the +X edge of chunk (0,0): its right-face neighbor
		// (global x=16) falls in chunk (1,0), which is never loaded.
		c.SetBlock(15, 8, 8, stone)

		data := mesh.GenerateChunkMeshData(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}

		if got, want := faceSkylight(data, faceRight), uint8(255); got != want {
			t.Errorf("right face (unloaded neighbor) R = %d, want %d (full bright)", got, want)
		}
		// The left-face neighbor (global x=14) is inside the same loaded
		// chunk and was never lit, so it keeps the engine's real value (0).
		if got, want := faceSkylight(data, faceLeft), uint8(0); got != want {
			t.Errorf("left face (loaded neighbor) R = %d, want %d (engine value)", got, want)
		}
	})

	t.Run("neighbor_chunk_loaded_usesEngineValue", func(t *testing.T) {
		w := world.NewWorld()
		c := world.NewChunk(0, 0)
		w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

		// Now also load the neighbor chunk and give the boundary cell a
		// known skylight value via the engine's own storage.
		neighbor := world.NewChunk(1, 0)
		neighbor.SetSkyLight(0, 8, 8, 9)
		w.Chunks[world.ChunkCoord{X: 1, Z: 0}] = neighbor

		c.SetBlock(15, 8, 8, stone)

		data := mesh.GenerateChunkMeshData(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}

		want := uint8(9 * 17)
		if got := faceSkylight(data, faceRight); got != want {
			t.Errorf("right face (loaded neighbor) R = %d, want %d (engine value for skylight 9)", got, want)
		}
	})
}

// TestVertexColor_SkylightMapping checks that the R channel is exactly
// light*17 for a range of skylight levels, including the boundaries 0 and 15.
func TestVertexColor_SkylightMapping(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	levels := []uint8{0, 1, 2, 7, 14, 15}
	for _, level := range levels {
		t.Run("", func(t *testing.T) {
			w := world.NewWorld()
			c := world.NewChunk(0, 0)
			w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

			neighbor := world.NewChunk(1, 0)
			neighbor.SetSkyLight(0, 8, 8, level)
			w.Chunks[world.ChunkCoord{X: 1, Z: 0}] = neighbor

			c.SetBlock(15, 8, 8, stone)

			data := mesh.GenerateChunkMeshData(c, w, nil)
			if data == nil {
				t.Fatal("expected mesh data, got nil")
			}

			want := level * 17
			if got := faceSkylight(data, faceRight); got != want {
				t.Errorf("skylight %d: R = %d, want %d (light*17)", level, got, want)
			}
		})
	}
}

// TestVertexColor_GBAConstant checks that every vertex in a mesh with varied
// skylight levels across its faces still has G=0, B=255, A=255 -- those
// channels never vary with light.
func TestVertexColor_GBAConstant(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	// Right-face neighbor chunk is unloaded (fallback full bright, R=255).
	// Left-face neighbor is in the same loaded chunk and keeps the engine's
	// default (unlit, R=0). This gives us two distinct light levels across
	// the mesh's vertices.
	c.SetBlock(15, 8, 8, stone)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	if len(data.Colors)%4 != 0 {
		t.Fatalf("Colors length %d is not a multiple of 4", len(data.Colors))
	}

	for i := 0; i < len(data.Colors); i += 4 {
		g, b, a := data.Colors[i+1], data.Colors[i+2], data.Colors[i+3]
		if g != 0 {
			t.Errorf("vertex %d: G = %d, want 0", i/4, g)
		}
		if b != 255 {
			t.Errorf("vertex %d: B = %d, want 255", i/4, b)
		}
		if a != 255 {
			t.Errorf("vertex %d: A = %d, want 255", i/4, a)
		}
	}
}

// TestVertexColor_FaceBiasNotBaked checks that a block lit uniformly on all
// sides produces identical vertex colors on the top face and a side face.
// Face brightness bias belongs in the shader, not the mesh, so the builder
// must not encode any face-dependent variation.
func TestVertexColor_FaceBiasNotBaked(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	// Place the block away from any chunk edge so every face's neighbor is
	// inside this same loaded chunk, then explicitly light every neighbor
	// cell to the same level.
	c.SetBlock(8, 8, 8, stone)
	c.SetSkyLight(9, 8, 8, 15) // +X (right)
	c.SetSkyLight(7, 8, 8, 15) // -X (left)
	c.SetSkyLight(8, 9, 8, 15) // +Y (top)
	c.SetSkyLight(8, 7, 8, 15) // -Y (bottom)
	c.SetSkyLight(8, 8, 9, 15) // +Z (front)
	c.SetSkyLight(8, 8, 7, 15) // -Z (back)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	topR, topG, topB, topA := faceColor(data, faceTop)
	sideR, sideG, sideB, sideA := faceColor(data, faceFront)

	if topR != sideR || topG != sideG || topB != sideB || topA != sideA {
		t.Errorf("top face color (%d,%d,%d,%d) != side face color (%d,%d,%d,%d); face bias must not be baked into vertex color",
			topR, topG, topB, topA, sideR, sideG, sideB, sideA)
	}

	// Sanity: the shared light level should show up as full-bright R.
	if topR != 255 {
		t.Errorf("expected uniform full-bright R=255, got %d", topR)
	}
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

func TestGenerateChunkMeshData_UsesUVLookup(t *testing.T) {
	blocks.Reset()
	texBlock := blocks.Register(&blocks.Block{
		ID:        "test/tex",
		Name:      "Tex",
		Color:     0xFFFFFFFF,
		ModelSpec: model.ModelSpec{Type: "full"},
	})

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(0, 0, 0, texBlock)

	lookup := dummyUVLookup{
		uv: atlas.UV{U0: 0.25, V0: 0.5, U1: 0.5, V1: 0.75},
	}

	data := mesh.GenerateChunkMeshData(c, w, lookup)
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
