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
			setRightFaceLightPatch(neighbor.SetBlockLight, 0, 8, 8, level)
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
	setRightFaceLightPatch(neighbor.SetSkyLight, 0, 8, 8, 3)
	setRightFaceLightPatch(neighbor.SetBlockLight, 0, 8, 8, 12)
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

// TestVertexColor_AllChannelsAcrossEveryVertex checks, for each of a block's
// six faces in turn, that every single vertex of that face (not just the
// first) carries the correct skylight/block-light pair, with AO still at the
// unoccluded constant and alpha still fully opaque.
//
// Before smooth lighting, one neighbouring cell was the whole story for a
// face, so a single test block with six distinct, direction-identifiable
// neighbour values could check all six faces at once. Now every corner
// averages over a 3x3 patch of cells (see setRightFaceLightPatch), and a
// real block's face patches overlap at the diagonals -- the top face's patch
// and the right face's patch share the cell above and to the right of the
// block, for instance -- so six distinct simultaneous values are no longer
// achievable. Each face is therefore its own case with its own fresh world,
// patch-lit uniformly so the face still reads as one flat value; the
// per-vertex, not-just-first-vertex check is preserved as-is.
func TestVertexColor_AllChannelsAcrossEveryVertex(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	cases := []struct {
		name       string
		face       int
		sky, block uint8
		light      func(c *world.Chunk, sky, block uint8)
	}{
		{"right", faceRight, 15, 0, func(c *world.Chunk, sky, block uint8) {
			for dy := -1; dy <= 1; dy++ {
				for dz := -1; dz <= 1; dz++ {
					c.SetSkyLight(9, 8+dy, 8+dz, sky)
					c.SetBlockLight(9, 8+dy, 8+dz, block)
				}
			}
		}},
		{"left", faceLeft, 0, 15, func(c *world.Chunk, sky, block uint8) {
			for dy := -1; dy <= 1; dy++ {
				for dz := -1; dz <= 1; dz++ {
					c.SetSkyLight(7, 8+dy, 8+dz, sky)
					c.SetBlockLight(7, 8+dy, 8+dz, block)
				}
			}
		}},
		{"top", faceTop, 8, 3, func(c *world.Chunk, sky, block uint8) {
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					c.SetSkyLight(8+dx, 9, 8+dz, sky)
					c.SetBlockLight(8+dx, 9, 8+dz, block)
				}
			}
		}},
		{"bottom", faceBottom, 3, 8, func(c *world.Chunk, sky, block uint8) {
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					c.SetSkyLight(8+dx, 7, 8+dz, sky)
					c.SetBlockLight(8+dx, 7, 8+dz, block)
				}
			}
		}},
		{"front", faceFront, 1, 14, func(c *world.Chunk, sky, block uint8) {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					c.SetSkyLight(8+dx, 8+dy, 9, sky)
					c.SetBlockLight(8+dx, 8+dy, 9, block)
				}
			}
		}},
		{"back", faceBack, 14, 1, func(c *world.Chunk, sky, block uint8) {
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					c.SetSkyLight(8+dx, 8+dy, 7, sky)
					c.SetBlockLight(8+dx, 8+dy, 7, block)
				}
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := world.NewWorld()
			c := world.NewChunk(0, 0)
			w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

			c.SetBlock(8, 8, 8, stone)
			tc.light(c, tc.sky, tc.block)

			data := mesh.GenerateChunkMeshData(c, w, nil)
			if data == nil {
				t.Fatal("expected mesh data, got nil")
			}

			const bytesPerFace = 6 * 4
			off := tc.face * bytesPerFace
			for v := 0; v < 6; v++ {
				i := off + v*4
				r, g, b, a := data.Colors[i], data.Colors[i+1], data.Colors[i+2], data.Colors[i+3]
				if want := tc.sky * 17; r != want {
					t.Errorf("vertex %d: R = %d, want %d", v, r, want)
				}
				if want := tc.block * 17; g != want {
					t.Errorf("vertex %d: G = %d, want %d", v, g, want)
				}
				if b != 255 {
					t.Errorf("vertex %d: B = %d, want 255", v, b)
				}
				if a != 255 {
					t.Errorf("vertex %d: A = %d, want 255", v, a)
				}
			}
		})
	}
}
