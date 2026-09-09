package mesh_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

// TestGenerateChunkMeshData_LeafFacesCulledAgainstLeaves is validation
// criterion 8, checked on vertex count rather than by eye: two adjacent leaf
// blocks must not draw the shared face between them, or a tree's interior
// becomes a mass of hidden geometry.
func TestGenerateChunkMeshData_LeafFacesCulledAgainstLeaves(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, blocks.Leaves)
	c.SetBlock(9, 8, 8, blocks.Leaves)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// Two leaf cubes (6 quads each => 72 verts) minus the two quads on their
	// shared boundary (one from each cube's side) => 60 verts.
	if got, want := len(data.Vertices)/3, 60; got != want {
		t.Fatalf("expected %d vertices (interior leaf faces culled), got %d", want, got)
	}
}

// TestGenerateChunkMeshData_LeafAgainstAirIsDrawn checks the other half of
// the same criterion: a leaf block with nothing but air around it draws all
// six faces, exactly like any other block.
func TestGenerateChunkMeshData_LeafAgainstAirIsDrawn(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, blocks.Leaves)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	if got, want := len(data.Vertices)/3, 6*6; got != want {
		t.Fatalf("expected %d vertices (all six faces drawn against air), got %d", want, got)
	}
}

// TestGenerateChunkMeshData_LeafDoesNotHideOpaqueNeighborFace is the
// regression the milestone's "leaves hide faces only against other leaves"
// design decision exists to prevent: a solid block sitting behind
// translucent leaves must keep its own face, or a stone surface would vanish
// wherever leaves grow against it, showing a hole through the tree.
func TestGenerateChunkMeshData_LeafDoesNotHideOpaqueNeighborFace(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, blocks.Stone)
	c.SetBlock(9, 8, 8, blocks.Leaves)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// Both cubes keep all 6 faces: the stone's face is not culled by the
	// leaf beside it (leaves only hide faces of other leaves), and the
	// leaf's face towards the stone IS culled (stone is fully opaque and
	// really does hide it) -- 6 (stone) + 5 (leaf) = 11 faces, 66 verts.
	if got, want := len(data.Vertices)/3, 11*6; got != want {
		t.Fatalf("expected %d vertices (stone keeps its face behind the leaf; the leaf's face into stone is culled), got %d",
			want, got)
	}
}

// TestGenerateChunkMeshData_LeafDoesNotCastHardAO is validation criterion 7's
// "underside is not a hard black disc": a leaf neighbour must not push a
// corner's ambient occlusion below full brightness (level 3), the way an
// opaque block does.
func TestGenerateChunkMeshData_LeafDoesNotCastHardAO(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, blocks.Stone)
	// A leaf directly beside the top face's edge, the same placement that
	// TestAO_CornerAndEdgeScenarios uses to prove an opaque neighbour shades
	// two corners to level 2.
	c.SetBlock(9, 9, 8, blocks.Leaves)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// Every corner's B channel (ambient occlusion) on the top face must read
	// full brightness (255): the top face has no other neighbours, and the
	// only one present is a leaf, which must not occlude at all.
	const bytesPerFace = 6 * 4
	// Top face is emitted third by a FullBlock (Right, Left, Top, ...).
	const topFaceIndex = 2
	off := topFaceIndex * bytesPerFace
	for v := 0; v < 6; v++ {
		b := data.Colors[off+v*4+2]
		if b != 255 {
			t.Errorf("top face vertex %d: AO byte = %d, want 255 (a leaf neighbour must not cast ambient occlusion)", v, b)
		}
	}
}
