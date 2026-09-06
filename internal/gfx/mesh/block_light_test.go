package mesh_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

// faceBlockLight returns the G channel (block light) recorded for the face
// at the given index within data.Colors.
func faceBlockLight(data *mesh.ChunkMeshData, faceIndex int) uint8 {
	_, g, _, _ := faceColor(data, faceIndex)
	return g
}

// TestVertexColor_BlockLightMapping checks that the G channel is exactly
// blockLight*17 for a range of block light levels, including the boundaries
// 0 and 15, sampled from the same neighboring cell used for skylight.
func TestVertexColor_BlockLightMapping(t *testing.T) {
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
			neighbor.SetBlockLight(0, 8, 8, level)
			w.Chunks[world.ChunkCoord{X: 1, Z: 0}] = neighbor

			c.SetBlock(15, 8, 8, stone)

			data := mesh.GenerateChunkMeshData(c, w, nil)
			if data == nil {
				t.Fatal("expected mesh data, got nil")
			}

			want := level * 17
			if got := faceBlockLight(data, faceRight); got != want {
				t.Errorf("block light %d: G = %d, want %d (light*17)", level, got, want)
			}
		})
	}
}

// TestGenerateChunkMeshData_UnloadedNeighborBlockLightStaysZero checks the
// asymmetry between the two light channels at the edge of the loaded world:
// skylight substitutes a cosmetic full-bright fallback (15) for an unloaded
// neighbor chunk, but block light has no such fallback and must stay 0,
// since there is no reason to assume an unloaded chunk contains an emitter.
func TestGenerateChunkMeshData_UnloadedNeighborBlockLightStaysZero(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	// Block at the +X edge of chunk (0,0): its right-face neighbor (global
	// x=16) falls in chunk (1,0), which is never loaded.
	c.SetBlock(15, 8, 8, stone)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	if got, want := faceSkylight(data, faceRight), uint8(255); got != want {
		t.Errorf("right face (unloaded neighbor) R = %d, want %d (skylight fallback)", got, want)
	}
	if got, want := faceBlockLight(data, faceRight), uint8(0); got != want {
		t.Errorf("right face (unloaded neighbor) G = %d, want %d (no block light fallback)", got, want)
	}
}

// TestVertexColor_SkyAndBlockLight_Independent checks that the two channels
// are packed independently: giving a neighboring cell distinct skylight and
// block light values must not let one bleed into the other, across every
// vertex of the affected face (not just the first).
func TestVertexColor_SkyAndBlockLight_Independent(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	neighbor := world.NewChunk(1, 0)
	neighbor.SetSkyLight(0, 8, 8, 3)
	neighbor.SetBlockLight(0, 8, 8, 12)
	w.Chunks[world.ChunkCoord{X: 1, Z: 0}] = neighbor

	c.SetBlock(15, 8, 8, stone)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	const bytesPerFace = 6 * 4
	off := faceRight * bytesPerFace
	for v := 0; v < 6; v++ {
		i := off + v*4
		r, g, b, a := data.Colors[i], data.Colors[i+1], data.Colors[i+2], data.Colors[i+3]
		if r != 3*17 {
			t.Errorf("vertex %d: R = %d, want %d (skylight 3)", v, r, 3*17)
		}
		if g != 12*17 {
			t.Errorf("vertex %d: G = %d, want %d (block light 12)", v, g, 12*17)
		}
		if b != 255 {
			t.Errorf("vertex %d: B = %d, want 255", v, b)
		}
		if a != 255 {
			t.Errorf("vertex %d: A = %d, want 255", v, a)
		}
	}
}

// TestVertexColor_AllChannelsAcrossEveryVertex places a block away from any
// chunk edge, gives every one of its six neighboring cells a distinct,
// direction-identifiable skylight/block-light pair, and then checks every
// single vertex of every face (not just the first vertex of each) so that a
// bug affecting only later vertices in a face's 6-vertex run cannot slip by.
func TestVertexColor_AllChannelsAcrossEveryVertex(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, stone)

	type expect struct{ sky, block uint8 }
	// faceRight..faceBack order matches AppendQuads' fixed emission order.
	expected := map[int]expect{
		faceRight:  {sky: 15, block: 0},
		faceLeft:   {sky: 0, block: 15},
		faceTop:    {sky: 8, block: 3},
		faceBottom: {sky: 3, block: 8},
		faceFront:  {sky: 1, block: 14},
		faceBack:   {sky: 14, block: 1},
	}

	c.SetSkyLight(9, 8, 8, expected[faceRight].sky)
	c.SetBlockLight(9, 8, 8, expected[faceRight].block)
	c.SetSkyLight(7, 8, 8, expected[faceLeft].sky)
	c.SetBlockLight(7, 8, 8, expected[faceLeft].block)
	c.SetSkyLight(8, 9, 8, expected[faceTop].sky)
	c.SetBlockLight(8, 9, 8, expected[faceTop].block)
	c.SetSkyLight(8, 7, 8, expected[faceBottom].sky)
	c.SetBlockLight(8, 7, 8, expected[faceBottom].block)
	c.SetSkyLight(8, 8, 9, expected[faceFront].sky)
	c.SetBlockLight(8, 8, 9, expected[faceFront].block)
	c.SetSkyLight(8, 8, 7, expected[faceBack].sky)
	c.SetBlockLight(8, 8, 7, expected[faceBack].block)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	const bytesPerFace = 6 * 4
	for face, exp := range expected {
		off := face * bytesPerFace
		for v := 0; v < 6; v++ {
			i := off + v*4
			r, g, b, a := data.Colors[i], data.Colors[i+1], data.Colors[i+2], data.Colors[i+3]
			if want := exp.sky * 17; r != want {
				t.Errorf("face %d vertex %d: R = %d, want %d", face, v, r, want)
			}
			if want := exp.block * 17; g != want {
				t.Errorf("face %d vertex %d: G = %d, want %d", face, v, g, want)
			}
			if b != 255 {
				t.Errorf("face %d vertex %d: B = %d, want 255", face, v, b)
			}
			if a != 255 {
				t.Errorf("face %d vertex %d: A = %d, want 255", face, v, a)
			}
		}
	}
}
