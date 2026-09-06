package lighting

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// TestEngine_BlockLight_RadialDecay checks that block light spreads evenly in
// all six directions from an emitter and decays by exactly one per step,
// including straight down. The cell directly below the emitter reading 14,
// not 15, is what separates block light's propagation rule from skylight's:
// skylight would leave it at full strength.
func TestEngine_BlockLight_RadialDecay(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 4, MinY: 16, MinZ: 4, MaxX: 12, MaxY: 24, MaxZ: 12}, blocks.Stone).
		Clear(testutil.Box{MinX: 5, MinY: 17, MinZ: 5, MaxX: 11, MaxY: 23, MaxZ: 11}).
		Fill(testutil.Box{MinX: 8, MinY: 20, MinZ: 8, MaxX: 8, MaxY: 20, MaxZ: 8}, blocks.Glowstone).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	if got := w.GetBlockLight(8, 20, 8); got != 15 {
		t.Fatalf("GetBlockLight at the emitter = %d, want 15", got)
	}

	tests := []struct {
		name    string
		x, y, z int
		want    uint8
	}{
		{"+X neighbour", 9, 20, 8, 14},
		{"-X neighbour", 7, 20, 8, 14},
		{"+Z neighbour", 8, 20, 9, 14},
		{"-Z neighbour", 8, 20, 7, 14},
		{"above", 8, 21, 8, 14},
		{"directly below (14, not 15 - unlike skylight)", 8, 19, 8, 14},
		{"two steps down", 8, 18, 8, 13},
		{"three steps down", 8, 17, 8, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := w.GetBlockLight(tt.x, tt.y, tt.z); got != tt.want {
				t.Errorf("GetBlockLight(%d, %d, %d) = %d, want %d", tt.x, tt.y, tt.z, got, tt.want)
			}
		})
	}
}

// TestEngine_SkyAndBlockLight_AreIndependent checks that the two channels
// never perturb each other: a cell can hold a non-trivial skylight level and
// a non-trivial block light level at the same time, and a change confined to
// one channel leaves the other exactly as it was.
func TestEngine_SkyAndBlockLight_AreIndependent(t *testing.T) {
	// Flat(60) matters here, not just Clear: the neighbouring columns must
	// stay solid well above the plug so sealing the shaft actually seals it,
	// rather than leaving skylight free to bypass the plug sideways through
	// already-open neighbouring columns above a shallower surface.
	t.Run("changing skylight does not perturb block light", func(t *testing.T) {
		w := testutil.NewWorld(t).
			Chunks(0, 0, 0, 0).
			Flat(60).
			Clear(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 60, MaxZ: 8}).
			Fill(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 0, MaxZ: 8}, blocks.Glowstone).
			Build()

		e := NewEngine(w)
		e.RecomputeAll()

		wantBlock := w.GetBlockLight(8, 5, 8)
		if wantBlock == 0 {
			t.Fatal("precondition failed: block light should reach partway up the shaft")
		}
		if got := w.GetSkyLight(8, 5, 8); got != 15 {
			t.Fatalf("precondition failed: GetSkyLight(8, 5, 8) = %d, want 15", got)
		}

		// A change that only affects skylight: seal the shaft from the sky.
		w.SetBlock(8, 30, 8, blocks.Stone)
		e.OnBlockChanged(8, 30, 8)

		if got := w.GetSkyLight(8, 5, 8); got != 0 {
			t.Fatalf("precondition failed: skylight should now be shaded, got %d", got)
		}
		if got := w.GetBlockLight(8, 5, 8); got != wantBlock {
			t.Errorf("GetBlockLight(8, 5, 8) = %d, want %d (perturbed by a skylight-only change)", got, wantBlock)
		}
	})

	t.Run("changing block light does not perturb skylight", func(t *testing.T) {
		w := testutil.NewWorld(t).
			Chunks(0, 0, 0, 0).
			Flat(60).
			Clear(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 60, MaxZ: 8}).
			Build()

		e := NewEngine(w)
		e.RecomputeAll()

		wantSky := w.GetSkyLight(8, 5, 8)
		if wantSky != 15 {
			t.Fatalf("precondition failed: GetSkyLight(8, 5, 8) = %d, want 15", wantSky)
		}

		// A change that only affects block light: drop a torch into the shaft.
		w.SetBlock(8, 0, 8, blocks.Glowstone)
		e.OnBlockChanged(8, 0, 8)

		if got := w.GetBlockLight(8, 5, 8); got == 0 {
			t.Fatal("precondition failed: block light should now reach this cell")
		}
		if got := w.GetSkyLight(8, 5, 8); got != wantSky {
			t.Errorf("GetSkyLight(8, 5, 8) = %d, want %d (perturbed by a block-light-only change)", got, wantSky)
		}
	})
}

// TestEngine_BlockLight_EmitterRemoval checks that breaking a glowstone
// darkens every cell it fed back to exactly what a full recompute produces -
// which here is all zero, since the emitter was the only source of block
// light in the sealed cave.
func TestEngine_BlockLight_EmitterRemoval(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 4, MinY: 16, MinZ: 4, MaxX: 12, MaxY: 24, MaxZ: 12}, blocks.Stone).
		Clear(testutil.Box{MinX: 5, MinY: 17, MinZ: 5, MaxX: 11, MaxY: 23, MaxZ: 11}).
		Fill(testutil.Box{MinX: 8, MinY: 20, MinZ: 8, MaxX: 8, MaxY: 20, MaxZ: 8}, blocks.Glowstone).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	if got := w.GetBlockLight(9, 20, 8); got == 0 {
		t.Fatal("precondition failed: the emitter's neighbour should be lit")
	}

	w.SetBlock(8, 20, 8, nil)
	e.OnBlockChanged(8, 20, 8)

	for x := 5; x <= 11; x++ {
		for y := 17; y <= 23; y++ {
			for z := 5; z <= 11; z++ {
				if got := w.GetBlockLight(x, y, z); got != 0 {
					t.Errorf("GetBlockLight(%d, %d, %d) = %d, want 0 after breaking the only emitter", x, y, z, got)
				}
			}
		}
	}

	// Belt and suspenders: the incremental result must match a full
	// recompute cell for cell, not merely "all zero" by the coincidence of
	// this scenario having no other source.
	incremental := w.GetChunk(0, 0).BlockLight

	truth := NewEngine(w)
	truth.RecomputeAll()
	fullRecompute := w.GetChunk(0, 0).BlockLight

	if incremental != fullRecompute {
		t.Error("incremental removal does not match a full recompute")
	}
}

// TestEngine_BlockLight_TwoEmitters_TakeMaxThenIndependentRemoval checks
// that a cell reachable from two emitters holds the maximum of their two
// contributions, and that removing the nearer emitter leaves the farther
// one's contribution exactly intact rather than leaving the cell dark or
// stale.
func TestEngine_BlockLight_TwoEmitters_TakeMaxThenIndependentRemoval(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 5, MinY: 20, MinZ: 8, MaxX: 5, MaxY: 20, MaxZ: 8}, blocks.Glowstone).
		Fill(testutil.Box{MinX: 9, MinY: 20, MinZ: 8, MaxX: 9, MaxY: 20, MaxZ: 8}, blocks.Glowstone).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	// (8,20,8) is 3 steps from the emitter at x=5 (would-be level 12) and 1
	// step from the emitter at x=9 (would-be level 14). The stored value must
	// be the maximum of the two.
	if got := w.GetBlockLight(8, 20, 8); got != 14 {
		t.Fatalf("GetBlockLight(8, 20, 8) = %d, want 14 (max of the two contributions)", got)
	}

	w.SetBlock(9, 20, 8, nil)
	e.OnBlockChanged(9, 20, 8)

	if got := w.GetBlockLight(8, 20, 8); got != 12 {
		t.Errorf("GetBlockLight(8, 20, 8) after removing the near emitter = %d, want 12 (the far emitter's contribution)", got)
	}
	if got := w.GetBlockLight(5, 20, 8); got != 15 {
		t.Errorf("GetBlockLight(5, 20, 8) = %d, want 15 (the surviving emitter itself, untouched)", got)
	}
}

// TestEngine_BlockLight_SealedEmitter checks that a glowstone with all six
// neighbours opaque lights only its own cell.
func TestEngine_BlockLight_SealedEmitter(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 0, MinY: 0, MinZ: 0, MaxX: 15, MaxY: 30, MaxZ: 15}, blocks.Stone).
		Fill(testutil.Box{MinX: 8, MinY: 15, MinZ: 8, MaxX: 8, MaxY: 15, MaxZ: 8}, blocks.Glowstone).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	if got := w.GetBlockLight(8, 15, 8); got != 15 {
		t.Fatalf("GetBlockLight at the emitter = %d, want 15", got)
	}

	for _, d := range directions {
		x, y, z := 8+d.DX, 15+d.DY, 8+d.DZ
		if got := w.GetBlockLight(x, y, z); got != 0 {
			t.Errorf("GetBlockLight(%d, %d, %d) = %d, want 0 (sealed by opaque stone)", x, y, z, got)
		}
	}
}

// TestEngine_BlockLight_NoLeakIntoUnloadedChunk checks that block light stops
// dead at the edge of loaded space, the same as skylight, and that reading
// there never causes a chunk to spring into existence.
func TestEngine_BlockLight_NoLeakIntoUnloadedChunk(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 15, MinY: 10, MinZ: 8, MaxX: 15, MaxY: 10, MaxZ: 8}, blocks.Glowstone).
		Build()

	e := NewEngine(w)
	e.RecomputeAll()

	if w.HasChunkAt(16, 8) {
		t.Fatal("chunk (1,0) should not be loaded")
	}
	if got := w.GetBlockLight(16, 10, 8); got != 0 {
		t.Errorf("GetBlockLight(16, 10, 8) = %d, want 0 (unloaded neighbour)", got)
	}
	if _, exists := w.Chunks[world.ChunkCoord{X: 1, Z: 0}]; exists {
		t.Error("propagation created a chunk for the unloaded neighbour")
	}
}
