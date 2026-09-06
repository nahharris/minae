package mesh_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

// The triangulation flip must split a quad along the diagonal through its
// *darker* corners, not its brighter ones.
//
// The artifact this prevents has a precise signature. A quad is drawn as two
// triangles, and light is interpolated across each. If the split leaves one
// triangle with every corner fully unoccluded, that triangle renders at full
// brightness right up to the edge it shares with the occluded half — a hard
// bright wedge where a soft corner shadow should be. Splitting through the
// occluded corner instead puts it in both triangles, so the shadow radiates
// from it.
//
// Stated as an invariant: if any corner of a quad is occluded, no emitted
// triangle may consist entirely of unoccluded corners.
func TestAO_NoFullyUnoccludedTriangleBesideAnOccludedCorner(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	// The block under test, and a single block diagonally above one of its top
	// face's corners. That occludes exactly one corner: the diagonal cell is
	// the corner sample for one vertex and touches no other.
	c.SetBlock(8, 8, 8, stone)
	c.SetBlock(9, 9, 9, stone)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// (8,8,8) sorts first in the x,y,z scan order, so its six faces occupy
	// vertex indices 0..35 and its top face is the third of them.
	const bytesPerFace = 6 * 4
	off := faceTop * bytesPerFace

	var ao [6]uint8
	for v := range ao {
		ao[v] = data.Colors[off+v*4+2]
	}

	occluded := 0
	for _, a := range ao {
		if a != 255 {
			occluded++
		}
	}
	if occluded == 0 {
		t.Fatalf("setup produced no occlusion at all (AO bytes %v); the test proves nothing", ao)
	}

	for tri := range 2 {
		a, b, cc := ao[tri*3], ao[tri*3+1], ao[tri*3+2]
		if a == 255 && b == 255 && cc == 255 {
			t.Errorf(
				"triangle %d has every corner unoccluded (%d,%d,%d) while the quad is occluded elsewhere (all six AO bytes: %v).\n"+
					"That triangle renders at full brightness against the shaded half, which is the hard bright wedge "+
					"the triangulation flip exists to prevent. The quad must split along the diagonal through its darker corners.",
				tri, a, b, cc, ao,
			)
		}
	}
}

// A light-emitting block must not cast ambient occlusion.
//
// AO approximates how much surrounding geometry blocks incoming light. A
// glowstone is not blocking light, it is light, and letting it occlude puts a
// dark halo on exactly the surfaces it is illuminating — the lamp casting its
// own shadow.
//
// Transparency is a separate question and is unchanged: an emitter is solid, so
// light still does not pass through it.
func TestAO_EmittingBlocksDoNotOcclude(t *testing.T) {
	unoccluded := mesh.AORampForTest()[3]

	build := func(t *testing.T, neighbour *blocks.Block) [4]uint8 {
		t.Helper()

		blocks.ResetToVanilla()

		w := world.NewWorld()
		c := world.NewChunk(0, 0)
		w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

		c.SetBlock(8, 8, 8, blocks.Stone)
		c.SetBlock(9, 9, 9, neighbour) // diagonal corner neighbour of the top face

		data := mesh.GenerateChunkMeshData(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}

		// Distinct AO values across the top face, keyed by corner position so
		// the result does not depend on which diagonal the quad split along.
		const bytesPerFace = 6 * 4
		posOff := faceTop * 6 * 3
		colOff := faceTop * bytesPerFace

		byCorner := make(map[[3]float32]uint8, 4)
		for i := range 6 {
			pos := [3]float32{
				data.Vertices[posOff+i*3],
				data.Vertices[posOff+i*3+1],
				data.Vertices[posOff+i*3+2],
			}
			byCorner[pos] = data.Colors[colOff+i*4+2]
		}

		var out [4]uint8
		i := 0
		for _, v := range byCorner {
			out[i] = v
			i++
		}
		return out
	}

	t.Run("a solid non-emitter occludes", func(t *testing.T) {
		ao := build(t, blocks.Stone)

		occluded := 0
		for _, v := range ao {
			if v != unoccluded {
				occluded++
			}
		}
		if occluded != 1 {
			t.Errorf("stone diagonal neighbour produced %d occluded corners (AO %v), want exactly 1.\n"+
				"Without this the emitter case below would pass vacuously.", occluded, ao)
		}
	})

	t.Run("an emitter does not", func(t *testing.T) {
		ao := build(t, blocks.Glowstone)

		for _, v := range ao {
			if v != unoccluded {
				t.Errorf("glowstone diagonal neighbour darkened a corner to %d (all corners %v), want all %d.\n"+
					"A light source must not cast ambient occlusion on what it lights.", v, ao, unoccluded)
			}
		}
	})
}
