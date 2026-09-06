package world

import (
	"testing"

	"github.com/nahharris/minae/internal/platform/config"
)

// A missing chunk must read as opaque darkness (0), never as open sky (15).
// The flood fill treats "brighter than or equal to what I'd contribute" as
// "nothing to do here", so a wrong default of 15 would silently stop light
// from ever crossing into an unloaded chunk's neighbors.
func TestWorld_GetSkyLight_MissingChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()

	if got := w.GetSkyLight(0, 0, 0); got != 0 {
		t.Errorf("GetSkyLight on missing chunk = %d, want 0", got)
	}
}

func TestWorld_GetSkyLight_OutOfRangeY(t *testing.T) {
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

			if got := w.GetSkyLight(0, tt.y, 0); got != 0 {
				t.Errorf("GetSkyLight(0, %d, 0) = %d, want 0", tt.y, got)
			}
		})
	}
}

func TestWorld_GetSkyLight_LoadedChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	chunk := NewChunk(0, 0)
	chunk.SetSkyLight(3, 10, 5, 12)
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = chunk

	if got := w.GetSkyLight(3, 10, 5); got != 12 {
		t.Errorf("GetSkyLight(3, 10, 5) = %d, want 12", got)
	}

	// A neighboring position in the same loaded chunk that was never set
	// stays at the zero value.
	if got := w.GetSkyLight(4, 10, 5); got != 0 {
		t.Errorf("GetSkyLight(4, 10, 5) = %d, want 0", got)
	}
}

// SetSkyLight on a missing chunk has nowhere to store the value, so it must
// be a silent no-op rather than panicking or creating the chunk.
func TestWorld_SetSkyLight_MissingChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()

	w.SetSkyLight(0, 0, 0, 15)

	if _, exists := w.Chunks[ChunkCoord{X: 0, Z: 0}]; exists {
		t.Error("SetSkyLight on a missing chunk created a chunk")
	}
	if got := w.GetSkyLight(0, 0, 0); got != 0 {
		t.Errorf("GetSkyLight after SetSkyLight on missing chunk = %d, want 0", got)
	}
}

func TestWorld_SetSkyLight_LoadedChunk(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = NewChunk(0, 0)

	w.SetSkyLight(1, 2, 3, 9)

	if got := w.GetSkyLight(1, 2, 3); got != 9 {
		t.Errorf("GetSkyLight(1, 2, 3) = %d, want 9", got)
	}
}

func TestWorld_HasChunkAt(t *testing.T) {
	t.Parallel()

	w := NewWorld()
	w.Chunks[ChunkCoord{X: 0, Z: 0}] = NewChunk(0, 0)
	w.Chunks[ChunkCoord{X: -1, Z: -1}] = NewChunk(-1, -1)

	tests := []struct {
		name string
		x, z int
		want bool
	}{
		{"loaded chunk origin", 0, 0, true},
		{"loaded chunk interior", 15, 15, true},
		{"unloaded chunk", 16, 0, false},
		{"negative coords inside loaded chunk", -1, -1, true},
		{"negative coords inside loaded chunk far edge", -16, -16, true},
		{"negative coords in unloaded chunk", -17, -17, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := w.HasChunkAt(tt.x, tt.z); got != tt.want {
				t.Errorf("HasChunkAt(%d, %d) = %v, want %v", tt.x, tt.z, got, tt.want)
			}
		})
	}
}
