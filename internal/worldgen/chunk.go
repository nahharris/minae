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

	// cache is M20's cell-corner cache (density.go): every `detail` corner
	// value this chunk's columns could possibly interpolate between,
	// computed once instead of per block. Built from coord's GLOBAL origin,
	// so a shared chunk edge's corners land on the exact same lattice points
	// as the neighbouring chunk's own cache (criterion 2).
	cache := newCornerCache(g, coord)

	for localX := range config.ChunkWidth {
		globalX := coord.X*config.ChunkWidth + localX
		for localZ := range config.ChunkWidth {
			globalZ := coord.Z*config.ChunkWidth + localZ

			// heightAndClimate, not SurfaceHeight+selectBiome separately: it
			// computes the same two answers (see its doc comment for the
			// bit-for-bit equivalence) while sampling the three shared
			// terrain fields once instead of twice -- worth doing here
			// specifically, since this loop runs for every one of a chunk's
			// 256 columns. The height it returns is density's 2D base term,
			// h in density.go's own vocabulary -- not the final column
			// shape, which fillColumn now derives cell by cell from the
			// density field itself.
			height, climate := g.heightAndClimate(globalX, globalZ)
			biome := g.biomes.Select(climate)
			fillColumn(c, cache, localX, localZ, height, biome)
		}
	}

	g.paintFeatures(c, coord)

	return c
}

// fillColumn fills one column of c from the density field, scanning top-down
// from h+surfaceHeightBand (above which density.go's solidAt guarantees air)
// down to y=0.
//
// Layering generalizes M19's "stone, then dirtDepth filler, then one surface
// layer" to a column that need not be a single solid run: depth counts blocks
// since the last air-to-solid transition scanning downward, resetting to -1
// every time an air cell is seen, so each locally-exposed top -- whether the
// column's true top or the lip of an overhang -- gets its own surface block
// and filler layers, and stone otherwise. With `detail` at zero every column
// is exactly one solid run from y=0 to h-1, and this reduces to precisely
// M19's rule: surface at h-1, filler at h-2 and h-3 (dirtDepth=2), stone
// below -- see TestGenerateReproducesHeightmapExactly for the exact-equality
// check this is built to satisfy (criterion 1).
//
// The deep layer stays Stone regardless of biome: only the top dirtDepth+1
// layers vary (the milestone's biome table declares a surface and a filler
// per biome, nothing deeper), which is also all two new blocks --
// blocks.Sand and blocks.Sandstone -- are needed for.
func fillColumn(c *world.Chunk, cache *cornerCache, x, z, h int, biome *Biome) {
	top := h + surfaceHeightBand
	if top >= config.ChunkHeight {
		top = config.ChunkHeight - 1
	}

	depth := -1 // -1: the cell above (or nothing, at the very top) was air.
	for y := top; y >= 0; y-- {
		if !cache.solidAt(x, y, z, h) {
			depth = -1
			continue
		}
		depth++
		switch {
		case depth == 0:
			c.SetBlock(x, y, z, biome.Surface)
		case depth <= dirtDepth:
			c.SetBlock(x, y, z, biome.Filler)
		default:
			c.SetBlock(x, y, z, blocks.Stone)
		}
	}
}
