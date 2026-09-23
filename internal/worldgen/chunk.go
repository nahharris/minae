package worldgen

import (
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// Generate implements chunks.Generator: stone below, dirtDepth layers of
// the column's own biome's filler block, one layer of that biome's surface
// block on top, at every column's own SurfaceHeight.
//
// SurfaceHeight is computed first, from noise alone, exactly as in M17;
// selectBiome is consulted only afterwards, only to decide what the column
// looks like, never how tall it is. This is the milestone's central
// architectural decision (see SurfaceHeight's own doc comment and
// docs/milestones/M19-biome-selection.md's "the finding that reshapes this
// milestone") made visible in the one function that could most easily have
// violated it by threading a biome into the height calculation instead.
//
// There is no water and no bedrock block registered (see
// internal/blocks/vanilla.go), and this milestone places neither: SeaLevel
// is a reference height only, and nothing here special-cases a column being
// above or below it.
//
// Generate is a pure function of coord for a given *Generator: every column
// samples SurfaceHeight and selectBiome at that column's GLOBAL coordinates
// (coord.X*config.ChunkWidth + localX, coord.Z*config.ChunkWidth + localZ),
// never chunk-local (localX, localZ) alone. Getting this wrong is the single
// most likely bug in this package -- see SurfaceHeight's doc comment --
// which is why it is spelled out here as globalX/globalZ rather than folded
// into one expression.
//
// Once terrain is filled, paintFeatures (features.go) places every tree and
// bush whose declared radius reaches into coord, rooted in coord itself or
// in a neighbouring chunk. That call is likewise a pure function of
// (g.worldSeed, coord) -- see rootsAffecting's doc comment -- so Generate as
// a whole stays a pure function of coord, exactly as chunks.Generator
// requires.
func (g *Generator) Generate(coord world.ChunkCoord) *world.Chunk {
	c := world.NewChunk(coord.X, coord.Z)

	for localX := range config.ChunkWidth {
		globalX := coord.X*config.ChunkWidth + localX
		for localZ := range config.ChunkWidth {
			globalZ := coord.Z*config.ChunkWidth + localZ

			// heightAndClimate, not SurfaceHeight+selectBiome separately: it
			// computes the same two answers (see its doc comment for the
			// bit-for-bit equivalence) while sampling the three shared
			// terrain fields once instead of twice -- worth doing here
			// specifically, since this loop runs for every one of a chunk's
			// 256 columns.
			height, climate := g.heightAndClimate(globalX, globalZ)
			biome := g.biomes.Select(climate)
			fillColumn(c, localX, localZ, height, biome)
		}
	}

	g.paintFeatures(c, coord)

	return c
}

// fillColumn fills one column from y=0 up to (not including) height: stone
// until the last dirtDepth+1 layers, then dirtDepth layers of biome's filler
// block, then one layer of biome's surface block on top -- the same
// layering chunks.FlatGenerator.Generate uses, parameterized on height and
// biome instead of a single constant surface/filler pair. Everything at or
// above height is left air, which is a world.Chunk's zero value.
//
// The deep layer stays Stone regardless of biome: only the top dirtDepth+1
// layers vary (the milestone's biome table declares a surface and a filler
// per biome, nothing deeper), which is also all two new blocks --
// blocks.Sand and blocks.Sandstone -- are needed for.
func fillColumn(c *world.Chunk, x, z, height int, biome *Biome) {
	for y := 0; y < height; y++ {
		switch {
		case y < height-dirtDepth-1:
			c.SetBlock(x, y, z, blocks.Stone)
		case y < height-1:
			c.SetBlock(x, y, z, biome.Filler)
		default:
			c.SetBlock(x, y, z, biome.Surface)
		}
	}
}
