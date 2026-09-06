package testutil

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/world"
)

func TestChunks_AllocatesInclusiveRange(t *testing.T) {
	w := NewWorld(t).Chunks(-1, -1, 1, 1).Build()

	if got, want := len(w.Chunks), 9; got != want {
		t.Errorf("chunk count = %d, want %d", got, want)
	}
	for _, coord := range []world.ChunkCoord{{X: -1, Z: -1}, {X: 0, Z: 0}, {X: 1, Z: 1}} {
		if w.GetChunk(coord.X, coord.Z) == nil {
			t.Errorf("chunk %+v was not allocated", coord)
		}
	}
	if w.GetChunk(2, 0) != nil {
		t.Error("chunk (2,0) is outside the requested range but was allocated")
	}
}

func TestFlat_LayersAndAirAbove(t *testing.T) {
	const surface = 32
	w := NewWorld(t).Chunks(0, 0, 0, 0).Flat(surface).Build()

	tests := []struct {
		name string
		y    int
		want *blocks.Block
	}{
		{"surface is grass", surface, blocks.Grass},
		{"one below is dirt", surface - 1, blocks.Dirt},
		{"two below is dirt", surface - 2, blocks.Dirt},
		{"three below is stone", surface - 3, blocks.Stone},
		{"bedrock depth is stone", 0, blocks.Stone},
		{"above surface is air", surface + 1, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := w.GetBlock(3, tt.y, 5); got != tt.want {
				t.Errorf("block at y=%d = %v, want %v", tt.y, got, tt.want)
			}
		})
	}
}

func TestFlat_CoversEveryAllocatedChunk(t *testing.T) {
	const surface = 8
	w := NewWorld(t).Chunks(-1, -1, 0, 0).Flat(surface).Build()

	// One position in each of the four chunks, including negative coordinates.
	for _, pos := range [][2]int{{-16, -16}, {-1, -1}, {0, 0}, {15, 15}} {
		if got := w.GetBlock(pos[0], surface, pos[1]); got != blocks.Grass {
			t.Errorf("surface at x=%d z=%d = %v, want Grass", pos[0], pos[1], got)
		}
	}
}

func TestFillAndClear_CarveAndPlace(t *testing.T) {
	const surface = 16
	w := NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(surface).
		Clear(Box{MinX: 4, MinY: surface - 3, MinZ: 4, MaxX: 6, MaxY: surface, MaxZ: 6}).
		Fill(Box{MinX: 4, MinY: surface + 4, MinZ: 4, MaxX: 6, MaxY: surface + 4, MaxZ: 6}, blocks.Stone).
		Build()

	if got := w.GetBlock(5, surface, 5); got != nil {
		t.Errorf("cleared position = %v, want air", got)
	}
	if got := w.GetBlock(5, surface-4, 5); got == nil {
		t.Error("position below the cleared box should still be solid")
	}
	if got := w.GetBlock(5, surface+4, 5); got != blocks.Stone {
		t.Errorf("overhang = %v, want Stone", got)
	}
	if got := w.GetBlock(7, surface, 7); got != blocks.Grass {
		t.Errorf("position outside the cleared box = %v, want Grass", got)
	}
}

func TestFill_SpansChunkBoundary(t *testing.T) {
	w := NewWorld(t).
		Chunks(-1, 0, 0, 0).
		Fill(Box{MinX: -2, MinY: 4, MinZ: 0, MaxX: 1, MaxY: 4, MaxZ: 0}, blocks.Stone).
		Build()

	for x := -2; x <= 1; x++ {
		if got := w.GetBlock(x, 4, 0); got != blocks.Stone {
			t.Errorf("block at x=%d = %v, want Stone", x, got)
		}
	}
	if got := w.GetBlock(-3, 4, 0); got != nil {
		t.Errorf("block at x=-3 = %v, want air (outside the box)", got)
	}
}

func TestNewWorld_RestoresVanillaRegistry(t *testing.T) {
	// A previous test may have left the registry cleared, which would make
	// every stored block silently collapse to air.
	blocks.Reset()

	w := NewWorld(t).Chunks(0, 0, 0, 0).Flat(4).Build()

	if got := w.GetBlock(0, 4, 0); got != blocks.Grass {
		t.Errorf("block = %v, want Grass; registry was not restored", got)
	}
}
