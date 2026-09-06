package world

import (
	"testing"

	"github.com/nahharris/minae/internal/platform/config"
)

// Chunk.GetBlockLight/SetBlockLight must behave exactly like the skylight
// pair: store and retrieve, and report failure the same way out of bounds.

func TestChunk_SetGetBlockLight(t *testing.T) {
	t.Parallel()

	chunk := NewChunk(0, 0)

	if !chunk.SetBlockLight(1, 2, 3, 9) {
		t.Fatal("SetBlockLight(1, 2, 3, 9) = false, want true")
	}
	if got := chunk.GetBlockLight(1, 2, 3); got != 9 {
		t.Errorf("GetBlockLight(1, 2, 3) = %d, want 9", got)
	}

	// A neighboring position that was never set stays at the zero value.
	if got := chunk.GetBlockLight(2, 2, 3); got != 0 {
		t.Errorf("GetBlockLight(2, 2, 3) = %d, want 0", got)
	}
}

func TestChunk_BlockLight_OutOfBounds(t *testing.T) {
	t.Parallel()

	chunk := NewChunk(0, 0)

	if chunk.SetBlockLight(-1, 0, 0, 15) {
		t.Error("SetBlockLight(-1, 0, 0, 15) = true, want false")
	}
	if got := chunk.GetBlockLight(-1, 0, 0); got != 0 {
		t.Errorf("GetBlockLight(-1, 0, 0) = %d, want 0", got)
	}
}

// A shared-backing-array bug between SkyLight and BlockLight would pass every
// single-channel test above, so exercise both channels together at the same
// position and confirm neither write bleeds into the other.
func TestChunk_SkyLightAndBlockLight_AreIndependent(t *testing.T) {
	t.Parallel()

	chunk := NewChunk(0, 0)

	chunk.SetSkyLight(5, 5, 5, 15)
	if got := chunk.GetBlockLight(5, 5, 5); got != 0 {
		t.Errorf("GetBlockLight after SetSkyLight = %d, want 0", got)
	}

	chunk.SetBlockLight(5, 5, 5, 7)
	if got := chunk.GetSkyLight(5, 5, 5); got != 15 {
		t.Errorf("GetSkyLight after SetBlockLight = %d, want 15 (unaffected)", got)
	}
	if got := chunk.GetBlockLight(5, 5, 5); got != 7 {
		t.Errorf("GetBlockLight(5, 5, 5) = %d, want 7", got)
	}
}

// World.GetBlockLight/SetBlockLight mirror the skylight accessors' missing-
// chunk and out-of-range-Y semantics.

func TestWorld_GetBlockLight_MissingChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()

	if got := w.GetBlockLight(0, 0, 0); got != 0 {
		t.Errorf("GetBlockLight on missing chunk = %d, want 0", got)
	}
}

func TestWorld_GetBlockLight_OutOfRangeY(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = NewChunk(0, 0)

	tests := []struct {
		name string
		y    int
	}{
		{"negative", -1},
		{"at height", config.ChunkHeight},
		{"far above", config.ChunkHeight + 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := w.GetBlockLight(0, tt.y, 0); got != 0 {
				t.Errorf("GetBlockLight(0, %d, 0) = %d, want 0", tt.y, got)
			}
		})
	}
}

func TestWorld_GetBlockLight_LoadedChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	chunk := NewChunk(0, 0)
	chunk.SetBlockLight(3, 10, 5, 12)
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = chunk

	if got := w.GetBlockLight(3, 10, 5); got != 12 {
		t.Errorf("GetBlockLight(3, 10, 5) = %d, want 12", got)
	}

	// A neighboring position in the same loaded chunk that was never set
	// stays at the zero value.
	if got := w.GetBlockLight(4, 10, 5); got != 0 {
		t.Errorf("GetBlockLight(4, 10, 5) = %d, want 0", got)
	}
}

// SetBlockLight on a missing chunk has nowhere to store the value, so it must
// be a silent no-op rather than panicking or creating the chunk.
func TestWorld_SetBlockLight_MissingChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()

	w.SetBlockLight(0, 0, 0, 15)

	if _, exists := w.Chunks[ChunkCoord{X: 0, Z: 0}]; exists {
		t.Error("SetBlockLight on a missing chunk created a chunk")
	}
	if got := w.GetBlockLight(0, 0, 0); got != 0 {
		t.Errorf("GetBlockLight after SetBlockLight on missing chunk = %d, want 0", got)
	}
}

func TestWorld_SetBlockLight_LoadedChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = NewChunk(0, 0)

	w.SetBlockLight(1, 2, 3, 9)

	if got := w.GetBlockLight(1, 2, 3); got != 9 {
		t.Errorf("GetBlockLight(1, 2, 3) = %d, want 9", got)
	}
}

// World-level independence: writing one channel through the World accessors
// must not disturb the other, exercised across a chunk boundary case too.
func TestWorld_SkyLightAndBlockLight_AreIndependent(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = NewChunk(0, 0)

	w.SetSkyLight(8, 8, 8, 15)
	w.SetBlockLight(8, 8, 8, 5)

	if got := w.GetSkyLight(8, 8, 8); got != 15 {
		t.Errorf("GetSkyLight(8, 8, 8) = %d, want 15", got)
	}
	if got := w.GetBlockLight(8, 8, 8); got != 5 {
		t.Errorf("GetBlockLight(8, 8, 8) = %d, want 5", got)
	}
}
