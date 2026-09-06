package world

import (
	"testing"

	"github.com/nahharris/minae/internal/platform/config"
)

// Go's integer division truncates toward zero, so the naive v/ChunkWidth is
// wrong for every negative coordinate. The light engine walks across chunk
// seams in both directions, so this conversion has to be exact.
func TestChunkAndLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		v         int
		wantChunk int
		wantLocal int
	}{
		{"origin", 0, 0, 0},
		{"first chunk interior", 7, 0, 7},
		{"last block of chunk 0", 15, 0, 15},
		{"first block of chunk 1", 16, 1, 0},
		{"chunk 2 interior", 33, 2, 1},
		{"block before origin", -1, -1, 15},
		{"first block of chunk -1", -16, -1, 0},
		{"last block of chunk -2", -17, -2, 15},
		{"chunk -2 interior", -30, -2, 2},
		{"far negative", -160, -10, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			chunk, local := ChunkAndLocal(tt.v)
			if chunk != tt.wantChunk || local != tt.wantLocal {
				t.Errorf("ChunkAndLocal(%d) = (%d, %d), want (%d, %d)",
					tt.v, chunk, local, tt.wantChunk, tt.wantLocal)
			}
		})
	}
}

// The conversion must round-trip: reassembling the global coordinate from the
// chunk index and local offset has to return the original value, and the local
// offset must always be a valid array index.
func TestChunkAndLocal_RoundTrips(t *testing.T) {
	t.Parallel()

	for v := -64; v <= 64; v++ {
		chunk, local := ChunkAndLocal(v)

		if local < 0 || local >= config.ChunkWidth {
			t.Fatalf("ChunkAndLocal(%d) local = %d, outside 0..%d", v, local, config.ChunkWidth-1)
		}
		if got := chunk*config.ChunkWidth + local; got != v {
			t.Fatalf("ChunkAndLocal(%d) round-tripped to %d", v, got)
		}
	}
}
