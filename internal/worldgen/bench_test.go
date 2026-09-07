package worldgen

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/world"
)

// BenchmarkGenerateChunk measures the cost of generating one chunk: 256
// columns, each sampling three fBm fields (see fieldConfig's four octaves) and
// evaluating three splines. The milestone's own design decision is explicit
// that this is worth measuring rather than assuming -- M14 and M15 chose
// their per-frame budgets against chunks.FlatGenerator, which fills a
// constant, and math.FMA is a software routine on amd64, which has no
// hardware FMA instruction.
func BenchmarkGenerateChunk(b *testing.B) {
	blocks.ResetToVanilla()

	g := NewGenerator(1)
	coord := world.ChunkCoord{X: 0, Z: 0}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		coord.X = i // vary the coordinate so the compiler cannot fold the loop
		_ = g.Generate(coord)
	}
}
