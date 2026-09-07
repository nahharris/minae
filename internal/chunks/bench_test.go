package chunks_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/worldgen"
)

// BenchmarkMeshTerrainChunk measures the cost of meshing one chunk of real
// generated terrain, for comparison against BenchmarkSeedChunk in
// internal/world/lighting: the M15 follow-up measured meshing at 4.7ms/chunk
// against generate's 235µs and light's 103ms, which is why light — not mesh —
// was the one moved onto a time budget in this pass. See
// docs/milestones/M15-chunk-streaming.md's "Follow-up" section.
func BenchmarkMeshTerrainChunk(b *testing.B) {
	blocks.ResetToVanilla()

	g := worldgen.NewGenerator(1)
	w := world.NewWorld()
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			coord := world.ChunkCoord{X: dx, Z: dz}
			w.Chunks[coord] = g.Generate(coord)
		}
	}

	center := world.ChunkCoord{X: 0, Z: 0}
	snap := chunks.Take(w, center)
	defer snap.Release()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = mesh.GenerateChunkMeshData(snap.Center(), snap, nil)
	}
}
