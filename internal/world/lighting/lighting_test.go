package lighting

import (
	"strconv"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// TestEngine_FlatTerrain checks the simplest possible world: one flat chunk
// with nothing but open sky above it.
func TestEngine_FlatTerrain(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(20).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	tests := []struct {
		name    string
		x, y, z int
		want    uint8
	}{
		{"surface block itself is opaque and dark", 8, 20, 8, 0},
		{"air directly above the surface is lit", 8, 21, 8, 15},
		{"deep underground is dark", 8, 0, 8, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := w.GetSkyLight(tt.x, tt.y, tt.z); got != tt.want {
				t.Errorf("GetSkyLight(%d, %d, %d) = %d, want %d", tt.x, tt.y, tt.z, got, tt.want)
			}
		})
	}
}

// TestEngine_Overhang checks that light entering through a single hole in a
// roof decays by one for every horizontal step away from the hole, while
// staying at full strength directly beneath it.
func TestEngine_Overhang(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(10).
		Fill(testutil.Box{MinX: 0, MinY: 15, MinZ: 0, MaxX: 15, MaxY: 15, MaxZ: 15}, blocks.Stone).
		Clear(testutil.Box{MinX: 8, MinY: 15, MinZ: 8, MaxX: 8, MaxY: 15, MaxZ: 8}).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	tests := []struct {
		name string
		z    int
		want uint8
	}{
		{"directly under the opening", 8, 15},
		{"one step away", 9, 14},
		{"two steps away", 10, 13},
		{"three steps away", 11, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := w.GetSkyLight(8, 12, tt.z); got != tt.want {
				t.Errorf("GetSkyLight(8, 12, %d) = %d, want %d", tt.z, got, tt.want)
			}
		})
	}
}

// TestEngine_VerticalShaft checks that a narrow shaft carved straight down
// through solid terrain stays lit at full strength all the way to the
// bottom. If skylight decayed on the way down like it does horizontally, a
// 60-block shaft would go dark long before reaching the bottom.
func TestEngine_VerticalShaft(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(60).
		Clear(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 60, MaxZ: 8}).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	for _, y := range []int{60, 40, 20, 1, 0} {
		t.Run("y="+strconv.Itoa(y), func(t *testing.T) {
			if got := w.GetSkyLight(8, y, 8); got != 15 {
				t.Errorf("GetSkyLight(8, %d, 8) = %d, want 15", y, got)
			}
		})
	}
}

// TestEngine_VerticalShaft_Incremental carves the same kind of shaft one
// block at a time through OnBlockChanged instead of RecomputeAll, exercising
// the add walk's full-strength-downward branch of expected directly.
func TestEngine_VerticalShaft_Incremental(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(60).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	for y := 60; y >= 0; y-- {
		w.SetBlock(8, y, 8, nil)
		e.OnBlockChanged(8, y, 8)
	}

	for _, y := range []int{60, 40, 20, 1, 0} {
		t.Run("y="+strconv.Itoa(y), func(t *testing.T) {
			if got := w.GetSkyLight(8, y, 8); got != 15 {
				t.Errorf("GetSkyLight(8, %d, 8) = %d, want 15", y, got)
			}
		})
	}
}

// TestEngine_SealedCave checks that an air pocket with no path to the sky
// stays completely dark.
func TestEngine_SealedCave(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 0, MinY: 0, MinZ: 0, MaxX: 15, MaxY: 63, MaxZ: 15}, blocks.Stone).
		Clear(testutil.Box{MinX: 5, MinY: 10, MinZ: 5, MaxX: 10, MaxY: 15, MaxZ: 10}).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	for x := 5; x <= 10; x++ {
		for y := 10; y <= 15; y++ {
			for z := 5; z <= 10; z++ {
				if got := w.GetSkyLight(x, y, z); got != 0 {
					t.Errorf("GetSkyLight(%d, %d, %d) = %d, want 0 (sealed cave)", x, y, z, got)
				}
			}
		}
	}
}

// TestEngine_CrossSeam checks that light spreading from a shaft near a
// chunk boundary reaches the neighbouring chunk at the correctly decayed
// level, in both crossing directions.
func TestEngine_CrossSeam(t *testing.T) {
	t.Run("positive: chunk (0,0) into chunk (1,0)", func(t *testing.T) {
		w := testutil.NewWorld(t).
			Chunks(0, 0, 1, 0).
			Flat(10).
			Fill(testutil.Box{MinX: 0, MinY: 15, MinZ: 0, MaxX: 31, MaxY: 15, MaxZ: 15}, blocks.Stone).
			Clear(testutil.Box{MinX: 15, MinY: 15, MinZ: 8, MaxX: 15, MaxY: 15, MaxZ: 8}).
			Build()

		e := NewEngine(w)
		e.RecomputeAll()

		if got := w.GetSkyLight(15, 12, 8); got != 15 {
			t.Errorf("GetSkyLight(15, 12, 8) = %d, want 15 (at the hole)", got)
		}
		if got := w.GetSkyLight(16, 12, 8); got != 14 {
			t.Errorf("GetSkyLight(16, 12, 8) = %d, want 14 (one step into the neighbour chunk)", got)
		}
		if got := w.GetSkyLight(17, 12, 8); got != 13 {
			t.Errorf("GetSkyLight(17, 12, 8) = %d, want 13 (two steps into the neighbour chunk)", got)
		}
	})

	t.Run("negative: chunk (1,0) into chunk (0,0)", func(t *testing.T) {
		w := testutil.NewWorld(t).
			Chunks(0, 0, 1, 0).
			Flat(10).
			Fill(testutil.Box{MinX: 0, MinY: 15, MinZ: 0, MaxX: 31, MaxY: 15, MaxZ: 15}, blocks.Stone).
			Clear(testutil.Box{MinX: 16, MinY: 15, MinZ: 8, MaxX: 16, MaxY: 15, MaxZ: 8}).
			Build()

		e := NewEngine(w)
		e.RecomputeAll()

		if got := w.GetSkyLight(16, 12, 8); got != 15 {
			t.Errorf("GetSkyLight(16, 12, 8) = %d, want 15 (at the hole)", got)
		}
		if got := w.GetSkyLight(15, 12, 8); got != 14 {
			t.Errorf("GetSkyLight(15, 12, 8) = %d, want 14 (one step into the neighbour chunk)", got)
		}
		if got := w.GetSkyLight(14, 12, 8); got != 13 {
			t.Errorf("GetSkyLight(14, 12, 8) = %d, want 13 (two steps into the neighbour chunk)", got)
		}
	})
}

// TestEngine_UnloadedNeighbour checks that light stops dead at the edge of
// loaded space instead of leaking into an unloaded chunk, and that reading
// there never causes a chunk to spring into existence.
func TestEngine_UnloadedNeighbour(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(10).
		Fill(testutil.Box{MinX: 0, MinY: 15, MinZ: 0, MaxX: 15, MaxY: 15, MaxZ: 15}, blocks.Stone).
		Clear(testutil.Box{MinX: 15, MinY: 15, MinZ: 8, MaxX: 15, MaxY: 15, MaxZ: 8}).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	if w.HasChunkAt(16, 8) {
		t.Fatal("chunk (1,0) should not be loaded")
	}
	if got := w.GetSkyLight(16, 12, 8); got != 0 {
		t.Errorf("GetSkyLight(16, 12, 8) = %d, want 0 (unloaded neighbour)", got)
	}
	if _, exists := w.Chunks[world.ChunkCoord{X: 1, Z: 0}]; exists {
		t.Error("propagation created a chunk for the unloaded neighbour")
	}
}

// TestEngine_OnBlockChanged_PlaceDarkens checks that placing an opaque block
// in a previously lit shaft darkens the column beneath it. The old engine
// could only ever add light, so this case - a block placed after the world
// was already lit - is exactly what it got wrong.
func TestEngine_OnBlockChanged_PlaceDarkens(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(60).
		Clear(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 60, MaxZ: 8}).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	if got := w.GetSkyLight(8, 0, 8); got != 15 {
		t.Fatalf("precondition failed: GetSkyLight(8, 0, 8) = %d, want 15", got)
	}

	w.SetBlock(8, 30, 8, blocks.Stone)
	e.OnBlockChanged(8, 30, 8)

	if got := w.GetSkyLight(8, 30, 8); got != 0 {
		t.Errorf("GetSkyLight(8, 30, 8) = %d, want 0 (now solid)", got)
	}
	if got := w.GetSkyLight(8, 20, 8); got != 0 {
		t.Errorf("GetSkyLight(8, 20, 8) = %d, want 0 (shaded by the new block above)", got)
	}
	if got := w.GetSkyLight(8, 0, 8); got != 0 {
		t.Errorf("GetSkyLight(8, 0, 8) = %d, want 0 (shaded by the new block above)", got)
	}
	if got := w.GetSkyLight(8, 40, 8); got != 15 {
		t.Errorf("GetSkyLight(8, 40, 8) = %d, want 15 (still open above the new block)", got)
	}
}

// TestEngine_OnBlockChanged_RemoveRestores checks that removing the block
// placed in TestEngine_OnBlockChanged_PlaceDarkens restores light to the
// column below, using the cell directly beneath a level-15 source (itself
// still 15) as the case the naive removal test gets wrong.
func TestEngine_OnBlockChanged_RemoveRestores(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(60).
		Clear(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 60, MaxZ: 8}).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	w.SetBlock(8, 30, 8, blocks.Stone)
	e.OnBlockChanged(8, 30, 8)

	w.SetBlock(8, 30, 8, nil)
	e.OnBlockChanged(8, 30, 8)

	for _, y := range []int{60, 30, 20, 0} {
		t.Run("y="+strconv.Itoa(y), func(t *testing.T) {
			if got := w.GetSkyLight(8, y, 8); got != 15 {
				t.Errorf("GetSkyLight(8, %d, 8) = %d, want 15", y, got)
			}
		})
	}
}

// TestEngine_DirtyChunks checks that DirtyChunks reports each affected
// chunk exactly once and clears the set for the next call.
func TestEngine_DirtyChunks(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 1, 0).
		Flat(10).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	dirty := e.DirtyChunks()
	seen := map[world.ChunkCoord]int{}
	for _, c := range dirty {
		seen[c]++
	}
	if seen[world.ChunkCoord{X: 0, Z: 0}] != 1 {
		t.Errorf("chunk (0,0) reported %d times, want 1", seen[world.ChunkCoord{X: 0, Z: 0}])
	}
	if seen[world.ChunkCoord{X: 1, Z: 0}] != 1 {
		t.Errorf("chunk (1,0) reported %d times, want 1", seen[world.ChunkCoord{X: 1, Z: 0}])
	}

	if again := e.DirtyChunks(); again != nil {
		t.Errorf("DirtyChunks after clearing = %v, want nil", again)
	}

	w.SetBlock(0, 11, 0, blocks.Stone)
	e.OnBlockChanged(0, 11, 0)

	dirty = e.DirtyChunks()
	if len(dirty) != 1 || dirty[0] != (world.ChunkCoord{X: 0, Z: 0}) {
		t.Errorf("DirtyChunks after a single-chunk change = %v, want [{0 0}]", dirty)
	}
}
