package worldgen

import (
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// Generate implements chunks.Generator: stone below, dirtDepth layers of
// dirt, one layer of grass on top, at every column's own SurfaceHeight.
//
// There is no water and no bedrock block registered (see
// internal/blocks/vanilla.go), and this milestone places neither: SeaLevel
// is a reference height only, and nothing here special-cases a column being
// above or below it.
//
// Generate is a pure function of coord for a given *Generator: every column
// samples SurfaceHeight at that column's GLOBAL coordinates
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

			height := g.SurfaceHeight(globalX, globalZ)
			fillColumn(c, localX, localZ, height)
		}
	}

	g.paintFeatures(c, coord)

	return c
}

// fillColumn fills one column from y=0 up to (not including) height:
// stone until the last dirtDepth+1 layers, then dirtDepth layers of dirt,
// then one layer of grass on top -- the same layering
// chunks.FlatGenerator.Generate uses, parameterized on height instead of a
// single constant. Everything at or above height is left air, which is a
// world.Chunk's zero value.
func fillColumn(c *world.Chunk, x, z, height int) {
	for y := 0; y < height; y++ {
		switch {
		case y < height-dirtDepth-1:
			c.SetBlock(x, y, z, blocks.Stone)
		case y < height-1:
			c.SetBlock(x, y, z, blocks.Dirt)
		default:
			c.SetBlock(x, y, z, blocks.Grass)
		}
	}
}
