package worldgen

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// topSolidY returns the y coordinate one above the highest terrain block in
// column (x, z) of c -- i.e. what SurfaceHeight should equal for the global
// column that local column represents, if Generate filled it correctly.
//
// "Terrain block" deliberately excludes Wood and Leaves. M18 added features
// painted on top of terrain (features.go), so a column under a tree can have
// non-air blocks well above its actual SurfaceHeight; these tests are about
// the terrain layering Generate produces independently of whether a tree
// happens to grow there, so they look past feature blocks to the ground
// underneath, exactly as SurfaceHeight itself does (it has no notion of
// trees at all).
func topSolidY(c *world.Chunk, x, z int) int {
	for y := config.ChunkHeight - 1; y >= 0; y-- {
		if b := c.GetBlock(x, y, z); b != nil && b != blocks.Wood && b != blocks.Leaves {
			return y + 1
		}
	}
	return 0
}

// Criterion 4, plus the SurfaceHeight/Generate agreement the milestone calls
// out explicitly ("that is what connects the two halves"): every column of a
// generated chunk must have grass on top, dirtDepth layers of dirt beneath
// it, stone below that, everywhere, with the surface sitting exactly at
// SurfaceHeight for that column's global coordinates.
//
// Since M20, a column need not be a single solid run -- an overhang gives it
// more than one -- so this checks the general property fillColumn is built to
// satisfy (see its own doc comment) rather than assuming exactly one run:
// every block's solidity must agree with density.go's own (uncached)
// Generator.solidAt, and within each contiguous solid run scanning downward
// from the top, the first block is the biome's surface, the next dirtDepth
// are its filler, and the rest are stone. This is also criterion 7 exercised
// directly against Generate's actual output: solidAt is the SAME function
// SurfaceHeight's band scan calls, so if Generate's cached cornerCache path
// ever disagreed with it, this would be the test that notices.
func TestGeneratedChunkLayeringAgreesWithSurfaceHeight(t *testing.T) {
	blocks.ResetToVanilla()

	g := NewGenerator(3)
	coord := world.ChunkCoord{X: 2, Z: -3}
	c := g.Generate(coord)

	for x := range config.ChunkWidth {
		gx := coord.X*config.ChunkWidth + x
		for z := range config.ChunkWidth {
			gz := coord.Z*config.ChunkWidth + z
			h := g.clampHeight(g.rawSurfaceHeight(gx, gz))
			biome := g.selectBiome(gx, gz)

			want := g.SurfaceHeight(gx, gz)
			if got := topSolidY(c, x, z); got != want {
				t.Fatalf("column (%d,%d) [global (%d,%d)]: top solid block implies height %d, SurfaceHeight says %d",
					x, z, gx, gz, got, want)
			}

			depth := -1
			top := h + surfaceHeightBand
			if top >= config.ChunkHeight {
				top = config.ChunkHeight - 1
			}
			for y := top; y >= 0; y-- {
				wantSolid := g.solidAt(gx, y, gz, h)
				b := c.GetBlock(x, y, z)
				gotSolid := b != nil && b != blocks.Wood && b != blocks.Leaves

				if gotSolid != wantSolid {
					t.Fatalf("column (%d,%d) y=%d: block solidity is %v, density.solidAt says %v", x, z, y, gotSolid, wantSolid)
				}

				if !wantSolid {
					depth = -1
					continue
				}
				depth++
				switch {
				case depth == 0:
					if b != biome.Surface {
						t.Fatalf("column (%d,%d) y=%d: expected surface block %v (top of a solid run), got %v", x, z, y, biome.Surface, b)
					}
				case depth <= dirtDepth:
					if b != biome.Filler {
						t.Fatalf("column (%d,%d) y=%d: expected filler block %v (depth %d), got %v", x, z, y, biome.Filler, depth, b)
					}
				default:
					if b != blocks.Stone {
						t.Fatalf("column (%d,%d) y=%d: expected stone (depth %d), got %v", x, z, y, depth, b)
					}
				}
			}
		}
	}
}

// Criterion 2: generating a chunk in isolation and generating it after its
// neighbours must give identical results. Generate takes no world and no
// neighbour state, so this should hold trivially if that purity is actually
// respected -- this test is what turns "should" into "does".
func TestGenerateIsIndependentOfLoadOrder(t *testing.T) {
	blocks.ResetToVanilla()

	g := NewGenerator(11)
	target := world.ChunkCoord{X: 4, Z: -2}

	isolated := g.Generate(target)

	// Generate every neighbour first, then the target.
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			g.Generate(world.ChunkCoord{X: target.X + dx, Z: target.Z + dz})
		}
	}
	afterNeighbours := g.Generate(target)

	if isolated.Blocks != afterNeighbours.Blocks {
		t.Fatalf("chunk (%d,%d)'s blocks differ depending on whether its neighbours were generated first",
			target.X, target.Z)
	}
}

// Criterion 3, at the level the milestone specifically calls for: a test
// against Generate's actual chunk output, not merely against SurfaceHeight,
// because a bug could live in how Generate computes its coordinates even
// while SurfaceHeight itself is correct.
//
// It asserts BOTH halves the design decision insists on: the shared edge is
// continuous, AND the two chunks are not identical. A test that only checked
// continuity would pass against the classic bug -- sampling noise at
// (localX, localZ) instead of global coordinates -- because a chunk that is
// internally identical to every other chunk also agrees perfectly with its
// neighbour at their shared edge.
func TestGeneratedChunksAreSeamlessAndDistinct(t *testing.T) {
	blocks.ResetToVanilla()

	g := NewGenerator(5)
	west := g.Generate(world.ChunkCoord{X: 0, Z: 0})
	east := g.Generate(world.ChunkCoord{X: 1, Z: 0})

	for z := range config.ChunkWidth {
		hWest := topSolidY(west, config.ChunkWidth-1, z)
		hEast := topSolidY(east, 0, z)
		if d := iabs(hEast - hWest); d > maxAdjacentSlope {
			t.Fatalf("seam at z=%d: chunk (0,0)'s x=15 column has height %d, chunk (1,0)'s x=0 column has height %d (delta %d > %d)",
				z, hWest, hEast, d, maxAdjacentSlope)
		}
	}

	if west.Blocks == east.Blocks {
		t.Fatal("chunk (0,0) and chunk (1,0) have byte-for-byte identical blocks; " +
			"this is exactly what sampling noise at chunk-local coordinates instead of global " +
			"ones produces -- every chunk would repeat the same terrain")
	}
}

// A second pair, further from the origin, so the distinctness check above is
// not relying on some coincidence specific to chunks (0,0) and (1,0).
func TestGeneratedChunksAreDistinctFarFromOrigin(t *testing.T) {
	blocks.ResetToVanilla()

	g := NewGenerator(5)
	a := g.Generate(world.ChunkCoord{X: 100, Z: -40})
	b := g.Generate(world.ChunkCoord{X: 101, Z: -40})

	if a.Blocks == b.Blocks {
		t.Fatal("chunk (100,-40) and chunk (101,-40) have byte-for-byte identical blocks")
	}
}
