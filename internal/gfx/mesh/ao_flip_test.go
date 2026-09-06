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
