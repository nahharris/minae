package lighting_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
	"github.com/nahharris/minae/internal/worldgen"
)

// BenchmarkSeedChunk measures the cost the M15 follow-up singles out as the
// freeze: lighting one newly generated chunk on real terrain.
//
// The setup mirrors what the pipeline actually does when a chunk streams in:
// a 3x3 neighbourhood of real worldgen terrain is generated once, and the
// benchmark repeatedly re-seeds the centre chunk. Only the centre chunk's own
// light is cleared between iterations - its neighbours keep whatever they
// were seeded with, which is exactly the "loaded but already lit" state the
// centre chunk seeds against in the real pipeline.
//
// Before the M15 follow-up this cost 17.5ms per chunk. Note that is NOT the
// 103ms the milestone doc quotes: that figure is the cold-neighbour case,
// measured by BenchmarkSeedChunkColdNeighbours below, and the two differ by
// almost six times. See that benchmark for why the distinction matters.
// Both had the same cause, with a CPU profile blaming mapaccess2 for 55%.
// See docs/milestones/M15-chunk-streaming.md
func BenchmarkSeedChunk(b *testing.B) {
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
	e := lighting.NewEngine(w)

	// Seed every neighbour once, up front, so the centre chunk is always
	// seeded against already-lit neighbours - the common case once the
	// pipeline is past its very first chunk.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			e.SeedChunk(world.ChunkCoord{X: dx, Z: dz})
		}
	}
	e.DirtyChunks()

	centerChunk := w.GetChunk(center.X, center.Z)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		clear(centerChunk.SkyLight[:])
		clear(centerChunk.BlockLight[:])
		e.SeedChunk(center)
	}
}

// BenchmarkSeedChunkColdNeighbours lights a chunk whose neighbours exist but
// have not been lit yet.
//
// This is the case that matters, and it is not the one BenchmarkSeedChunk
// measures. There, the neighbours are already lit, so propagation stops dead
// at every border: nothing next door can be raised. Here light floods into all
// eight, which is what actually happens at startup and whenever the player
// walks into terrain that has just streamed in — the two situations the
// original "the game freezes" report was about.
//
// The gap between the two is large enough to mislead. Measured on the same
// machine before this optimisation pass: 17.5 ms warm against 103 ms cold. An
// optimisation judged only against the warm case would have looked five times
// better than it was, and would have been tuned for the wrong workload.
func BenchmarkSeedChunkColdNeighbours(b *testing.B) {
	blocks.ResetToVanilla()

	g := worldgen.NewGenerator(1)

	for b.Loop() {
		b.StopTimer()
		// A fresh, entirely unlit region each iteration. Rebuilding it is the
		// reason for the timer dance: generation is real work, and it is not
		// what this benchmark is about.
		w := world.NewWorld()
		for dx := -1; dx <= 1; dx++ {
			for dz := -1; dz <= 1; dz++ {
				coord := world.ChunkCoord{X: dx, Z: dz}
				w.Chunks[coord] = g.Generate(coord)
			}
		}
		e := lighting.NewEngine(w)
		b.StartTimer()

		e.SeedChunk(world.ChunkCoord{X: 0, Z: 0})
	}
}
