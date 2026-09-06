package lighting_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// A light change must invalidate the mesh of every chunk that renders it, not
// only the chunk the change happened in.
//
// A block's faces are lit from the cells *around* it, so a wall standing on a
// chunk border is lit by air on the far side of that border. Light the room and
// the wall's chunk contains no changed cell at all — every cell in it is solid
// and stays at 0 — yet its faces must be redrawn.
//
// Reported symptom: placing a glowstone in a dark room left one wall pitch
// black until a block in it was destroyed, which forced that chunk to re-mesh
// for an unrelated reason.
func TestDirtyChunks_IncludesNeighbourAcrossASeam(t *testing.T) {
	const y = 40

	// Chunk (0,0) is solid rock; chunk (0,1) is the open room next to it. The
	// seam runs along z=15/z=16.
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 1).
		Fill(testutil.Box{MinX: 0, MinY: 0, MinZ: 0, MaxX: 15, MaxY: 60, MaxZ: 15}, blocks.Stone).
		Build()

	wall := world.ChunkCoord{X: 0, Z: 0}
	room := world.ChunkCoord{X: 0, Z: 1}

	engine := lighting.NewEngine(w)
	engine.RecomputeAll()
	engine.DirtyChunks() // drain the initial churn

	// Sanity: the wall block at the seam is lit by the air cell across it, and
	// that cell is dark to begin with.
	if got := w.GetBlockLight(8, y, 16); got != 0 {
		t.Fatalf("expected the room to start dark, got block light %d", got)
	}

	// Put a glowstone in the room, one block clear of the seam.
	w.SetBlock(8, y, 18, blocks.Glowstone)
	engine.OnBlockChanged(8, y, 18)

	// The cell the wall's face samples is now lit...
	if got := w.GetBlockLight(8, y, 16); got == 0 {
		t.Fatal("expected the glowstone to light the cell adjacent to the wall")
	}
	// ...while nothing inside the wall's own chunk changed, since it is solid.
	if got := w.GetBlockLight(8, y, 15); got != 0 {
		t.Fatalf("the wall chunk is solid and should hold no light, got %d", got)
	}

	dirty := make(map[world.ChunkCoord]bool)
	for _, c := range engine.DirtyChunks() {
		dirty[c] = true
	}

	if !dirty[room] {
		t.Errorf("the room chunk %+v was not reported dirty, but its light changed", room)
	}
	if !dirty[wall] {
		t.Errorf("the wall chunk %+v was not reported dirty.\n"+
			"No light changed inside it — it is solid — but its faces are lit from the "+
			"air cells across the seam, which did change. Its mesh still carries the old "+
			"darkness, so the wall renders black until something else forces a re-mesh.",
			wall)
	}
}
