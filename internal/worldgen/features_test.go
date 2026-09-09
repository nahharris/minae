package worldgen

import (
	"sort"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// TestFeatureRadiusIsEnforced is the milestone's "not review, asserted"
// requirement checked at the shape level: treeShape and bushShape are pure
// functions from a root offset to a set of blocks, so every offset either of
// them can ever produce is checked directly against the feature's declared
// radius, for every trunk height the generator can actually produce.
func TestFeatureRadiusIsEnforced(t *testing.T) {
	for h := minTrunkHeight; h < minTrunkHeight+trunkHeightRange; h++ {
		for _, fb := range treeShape(h) {
			if absOffset(fb.dx) > treeRadius || absOffset(fb.dz) > treeRadius {
				t.Fatalf("treeShape(%d) produced offset (%d,%d,%d), horizontal distance %d exceeds treeRadius=%d",
					h, fb.dx, fb.dy, fb.dz, max(absOffset(fb.dx), absOffset(fb.dz)), treeRadius)
			}
		}
	}

	for _, fb := range bushShape() {
		if absOffset(fb.dx) > bushRadius || absOffset(fb.dz) > bushRadius {
			t.Fatalf("bushShape produced offset (%d,%d,%d), horizontal distance %d exceeds bushRadius=%d",
				fb.dx, fb.dy, fb.dz, max(absOffset(fb.dx), absOffset(fb.dz)), bushRadius)
		}
	}
}

// withinSomeRootRadius reports whether global column (gx, gz) lies within
// the declared radius of at least one of roots.
func withinSomeRootRadius(roots []rootCandidate, gx, gz int) (ok bool) {
	for _, r := range roots {
		radius := bushRadius
		if r.kind == featureTree {
			radius = treeRadius
		}
		dx := absOffset(gx - r.globalX())
		dz := absOffset(gz - r.globalZ())
		if dx <= radius && dz <= radius {
			return true
		}
	}
	return false
}

// TestGenerate_FeaturesNeverWriteOutsideDeclaredRadius is the integration
// half of radius enforcement: every Wood or Leaves block anywhere in a
// generated chunk -- terrain itself never places either, see
// internal/worldgen/chunk.go's fillColumn -- must fall within the declared
// radius of one of the roots that rootsAffecting says could touch that
// chunk. A shape function that quietly grew one ring past its declared
// radius (the exact mutation this test exists to catch) would place a block
// no root's radius covers.
func TestGenerate_FeaturesNeverWriteOutsideDeclaredRadius(t *testing.T) {
	blocks.ResetToVanilla()

	checked := 0
	for seed := int64(0); seed < 6; seed++ {
		g := NewGenerator(seed)
		for cx := -3; cx <= 3; cx++ {
			for cz := -3; cz <= 3; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				roots := g.rootsAffecting(coord)
				c := g.Generate(coord)

				for x := range config.ChunkWidth {
					gx := coord.X*config.ChunkWidth + x
					for z := range config.ChunkWidth {
						gz := coord.Z*config.ChunkWidth + z
						for y := range config.ChunkHeight {
							b := c.GetBlock(x, y, z)
							if b != blocks.Wood && b != blocks.Leaves {
								continue
							}
							checked++
							if !withinSomeRootRadius(roots, gx, gz) {
								t.Fatalf("seed %d chunk (%d,%d): %s at global (%d,%d,%d) is outside every known root's declared radius",
									seed, cx, cz, b.ID, gx, y, gz)
							}
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no feature blocks were found across the sampled seeds/chunks -- test is not exercising anything")
	}
}

// TestFeatures_TreeCrossesChunkBoundaryIntact is validation criterion 3,
// checked directly and deterministically: a tree rooted at each of the four
// boundary positions (west, east, north, south edge of its chunk) must have
// its full canopy realized across the two chunks its geometry spans, with no
// clipped branches and no half-tree.
//
// This drives placeIfInChunk and treeShape directly with a hand-chosen root
// and a fixed, flat ground height, rather than searching real generated
// terrain for a naturally-occurring edge case. That is deliberate: on sloped
// terrain a canopy cell can legitimately land inside a taller neighbouring
// column and be skipped (placeIfInChunk never overwrites existing terrain) --
// correct behaviour, not clipping, but indistinguishable from a boundary bug
// by a test that only compares block-for-block against the ideal shape. A
// flat, hand-chosen ground height removes that confound and isolates exactly
// the property this criterion is about: does content near an edge reach the
// neighbouring chunk at all.
func TestFeatures_TreeCrossesChunkBoundaryIntact(t *testing.T) {
	blocks.ResetToVanilla()

	const groundY = 64
	const trunkHeight = 5

	cases := []struct {
		name       string
		lx, lz     int
		neighbourX int
		neighbourZ int
	}{
		{"west edge", 0, 8, -1, 0},
		{"east edge", config.ChunkWidth - 1, 8, 1, 0},
		{"north edge", 8, 0, 0, -1},
		{"south edge", 8, config.ChunkWidth - 1, 0, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rootCoord := world.ChunkCoord{X: 10, Z: -10}
			root := rootCandidate{coord: rootCoord, lx: tc.lx, lz: tc.lz, kind: featureTree, trunkHeight: trunkHeight}
			neighbourCoord := world.ChunkCoord{X: rootCoord.X + tc.neighbourX, Z: rootCoord.Z + tc.neighbourZ}

			cRoot := world.NewChunk(rootCoord.X, rootCoord.Z)
			cNeighbour := world.NewChunk(neighbourCoord.X, neighbourCoord.Z)

			shape := treeShape(trunkHeight)
			for _, fb := range shape {
				gx, gz, y := root.globalX()+fb.dx, root.globalZ()+fb.dz, groundY+fb.dy
				placeIfInChunk(cRoot, rootCoord, gx, y, gz, fb.block)
				placeIfInChunk(cNeighbour, neighbourCoord, gx, y, gz, fb.block)
			}

			for _, fb := range shape {
				gx, gz, y := root.globalX()+fb.dx, root.globalZ()+fb.dz, groundY+fb.dy
				cx, lx := world.ChunkAndLocal(gx)
				cz, lz := world.ChunkAndLocal(gz)

				var got *blocks.Block
				switch {
				case cx == rootCoord.X && cz == rootCoord.Z:
					got = cRoot.GetBlock(lx, y, lz)
				case cx == neighbourCoord.X && cz == neighbourCoord.Z:
					got = cNeighbour.GetBlock(lx, y, lz)
				default:
					t.Fatalf("offset (%d,%d,%d) from a root at the %s lands outside both the root chunk and its neighbour -- "+
						"treeRadius (%d) or featureChunkRadius (%d) is too small to cover it",
						fb.dx, fb.dy, fb.dz, tc.name, treeRadius, featureChunkRadius)
					continue
				}

				if got != fb.block {
					t.Fatalf("offset (%d,%d,%d) from a root at the %s: want %v, got %v -- the canopy was clipped at the chunk boundary",
						fb.dx, fb.dy, fb.dz, tc.name, fb.block, got)
				}
			}
		})
	}
}

// TestFeatures_LoadOrderIndependence is validation criterion 2, run across a
// broad sweep of seeds and coordinates (rather than one hand-picked case) so
// it also exercises whatever roots naturally land near an edge: a chunk
// generated in isolation must be byte-identical to the same chunk generated
// after all eight of its neighbours.
//
// Generate never reads World, so this should hold by construction -- but
// that is exactly the property worth asserting rather than trusting, per the
// milestone's own framing of this as the load-bearing criterion for safety
// under the M14 worker pool.
func TestFeatures_LoadOrderIndependence(t *testing.T) {
	blocks.ResetToVanilla()

	for seed := int64(0); seed < 6; seed++ {
		g := NewGenerator(seed)
		for cx := -2; cx <= 2; cx++ {
			for cz := -2; cz <= 2; cz++ {
				target := world.ChunkCoord{X: cx, Z: cz}

				isolated := g.Generate(target)

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
					t.Fatalf("seed %d chunk (%d,%d): blocks differ depending on whether its neighbours were generated first",
						seed, cx, cz)
				}
			}
		}
	}
}

// acceptedOwnRoots queries g.rootsAffecting(queryFrom) and returns the sorted
// order indices of the roots rooted in home that it reports accepted. Used by
// TestFeatures_AcceptanceConsistentAcrossQueryOrigin below.
func acceptedOwnRoots(g *Generator, queryFrom, home world.ChunkCoord) []int {
	var out []int
	for _, r := range g.rootsAffecting(queryFrom) {
		if r.coord == home {
			out = append(out, r.order)
		}
	}
	sort.Ints(out)
	return out
}

func intSliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestFeatures_AcceptanceConsistentAcrossQueryOrigin is the load-order
// independence property at the level where it actually lives: whether a
// chunk's own accepted roots are the same regardless of which nearby chunk's
// generation triggered the computation.
//
// This is a sharper check than TestFeatures_LoadOrderIndependence and
// TestGenerateIsIndependentOfLoadOrder, which only confirm that calling
// Generate(target) twice gives the same answer -- true by construction for
// any pure function with no shared state, regardless of whether poolRadius
// is wide enough. What those tests cannot see is a narrower pool silently
// giving two *different* nearby chunks two different verdicts about the same
// candidate root: home's own query sees candidates rootsAffecting's pool
// argument says it needs, but a neighbour's query -- with home's roots
// inside its declared featureChunkRadius output range, entitling it to agree
// with home about them -- computes its OWN pool centred on itself, which may
// not reach far enough to see the same earlier-ordered rejectors home saw.
// That is precisely what a too-small poolRadius breaks, and precisely what
// this test is built to catch: for every home chunk, its own accepted-roots
// verdict (queried from itself) must match the verdict every neighbour
// within featureChunkRadius reports for those exact same roots.
func TestFeatures_AcceptanceConsistentAcrossQueryOrigin(t *testing.T) {
	blocks.ResetToVanilla()

	for seed := int64(0); seed < 6; seed++ {
		g := NewGenerator(seed)
		for cx := -4; cx <= 4; cx++ {
			for cz := -4; cz <= 4; cz++ {
				home := world.ChunkCoord{X: cx, Z: cz}
				fromHome := acceptedOwnRoots(g, home, home)

				for ddx := -featureChunkRadius; ddx <= featureChunkRadius; ddx++ {
					for ddz := -featureChunkRadius; ddz <= featureChunkRadius; ddz++ {
						if ddx == 0 && ddz == 0 {
							continue
						}
						neighbour := world.ChunkCoord{X: home.X + ddx, Z: home.Z + ddz}
						fromNeighbour := acceptedOwnRoots(g, neighbour, home)

						if !intSliceEqual(fromHome, fromNeighbour) {
							t.Fatalf("seed %d: chunk (%d,%d)'s accepted roots are %v queried from itself, "+
								"but %v queried from neighbour (%d,%d) -- acceptance depends on which chunk asked",
								seed, home.X, home.Z, fromHome, fromNeighbour, neighbour.X, neighbour.Z)
						}
					}
				}
			}
		}
	}
}

// TestFeatures_Deterministic is validation criterion 1: two independently
// constructed Generators built from the same seed must place the same
// features in the same chunk.
func TestFeatures_Deterministic(t *testing.T) {
	blocks.ResetToVanilla()

	coord := world.ChunkCoord{X: 7, Z: -3}
	a := NewGenerator(99).Generate(coord)
	b := NewGenerator(99).Generate(coord)

	if a.Blocks != b.Blocks {
		t.Fatal("two Generators built from the same seed produced different blocks for the same chunk")
	}
}

// TestFeatures_TreesAreRootedNotFloating is validation criterion 4: every
// tree's trunk base must rest on solid ground, never on air left by terrain
// that sloped away underneath it. It checks the lowest Wood block in every
// column that has one, since a trunk is always painted from the ground up
// (see treeShape), so the lowest Wood block in a column is always the base.
func TestFeatures_TreesAreRootedNotFloating(t *testing.T) {
	blocks.ResetToVanilla()

	checked := 0
	for seed := int64(0); seed < 10; seed++ {
		g := NewGenerator(seed)
		for cx := -4; cx <= 4; cx++ {
			for cz := -4; cz <= 4; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				c := g.Generate(coord)

				for x := range config.ChunkWidth {
					for z := range config.ChunkWidth {
						base := -1
						for y := range config.ChunkHeight {
							if c.GetBlock(x, y, z) == blocks.Wood {
								base = y
								break
							}
						}
						if base < 0 {
							continue
						}
						checked++

						if base == 0 {
							t.Fatalf("seed %d chunk (%d,%d) column (%d,%d): trunk base sits at y=0, nothing beneath it to check",
								seed, cx, cz, x, z)
						}
						if below := c.GetBlock(x, base-1, z); below == nil {
							t.Fatalf("seed %d chunk (%d,%d) column (%d,%d): trunk base at y=%d is floating -- air at y=%d beneath it",
								seed, cx, cz, x, z, base, base-1)
						}
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no trees were found across the sampled seeds/chunks -- test is not exercising anything")
	}
}

// TestFeatures_MinimumSpacingRespected is validation criterion 5: no two
// accepted roots, gathered across a wide area, may sit closer than
// minSpacing -- which is also what keeps their declared radii (each well
// under half of minSpacing) from ever overlapping.
func TestFeatures_MinimumSpacingRespected(t *testing.T) {
	blocks.ResetToVanilla()

	for seed := int64(0); seed < 8; seed++ {
		g := NewGenerator(seed)

		type key struct{ x, z, order int }
		seen := map[key]bool{}
		var accepted []rootCandidate

		for cx := -5; cx <= 5; cx++ {
			for cz := -5; cz <= 5; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				for _, r := range g.rootsAffecting(coord) {
					if r.coord != coord {
						continue
					}
					k := key{r.coord.X, r.coord.Z, r.order}
					if seen[k] {
						continue
					}
					seen[k] = true
					accepted = append(accepted, r)
				}
			}
		}

		for i := range accepted {
			for j := i + 1; j < len(accepted); j++ {
				if tooClose(accepted[i], accepted[j]) {
					t.Fatalf("seed %d: roots at global (%d,%d) and (%d,%d) are closer than minSpacing=%.1f",
						seed, accepted[i].globalX(), accepted[i].globalZ(),
						accepted[j].globalX(), accepted[j].globalZ(), minSpacing)
				}
			}
		}
	}
}

// TestFeatures_DensityIsPlausible is validation criterion 6: the average
// number of accepted roots per chunk, across many seeds, must land in a
// plausible band. Measured empirically against these constants at roughly
// 1.9 roots/chunk; the band here is deliberately wide so retuning the
// generator's *feel* doesn't make this test brittle, while still catching a
// change that silently turns plains into empty steppe (near 0) or solid
// forest (many times denser than a chunk can even fit given minSpacing).
func TestFeatures_DensityIsPlausible(t *testing.T) {
	blocks.ResetToVanilla()

	const minAvg, maxAvg = 0.3, 4.0

	total, chunks := 0, 0
	for seed := int64(0); seed < 15; seed++ {
		g := NewGenerator(seed)
		for cx := 0; cx < 8; cx++ {
			for cz := 0; cz < 8; cz++ {
				coord := world.ChunkCoord{X: cx, Z: cz}
				for _, r := range g.rootsAffecting(coord) {
					if r.coord == coord {
						total++
					}
				}
				chunks++
			}
		}
	}

	avg := float64(total) / float64(chunks)
	if avg < minAvg || avg > maxAvg {
		t.Fatalf("average roots per chunk = %.3f, want within [%.1f, %.1f] for plausible density", avg, minAvg, maxAvg)
	}
}
