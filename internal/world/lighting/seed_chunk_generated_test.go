package lighting_test

import (
	"math/rand"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
	"github.com/nahharris/minae/internal/worldgen"
)

// TestSeedChunk_MatchesFullRecompute_GeneratedTerrain is the equivalence
// check TestSeedChunk_MatchesFullRecompute runs, but against real generated
// terrain instead of flat/handmade fixtures.
//
// This distinction matters specifically for the M15 follow-up's second
// change: skipping the enqueue of open-sky cells above the tallest terrain in
// the loaded neighbourhood. Flat terrain can never expose a bug in that
// optimisation, because every chunk's own highest solid block already equals
// the neighbourhood's - there is nothing for the "wrong ceiling" mutation to
// get wrong. Real relief (and, deliberately, an overhang taller than its
// neighbour) is what can tell the correct rule apart from the buggy one that
// uses only the seeded chunk's own height.
//
// Every case below is checked in three seeding orders - forward, reverse, and
// one shuffle - each compared against one RecomputeAll computed over the same
// finished world, for both skylight and block light.
//
// Each case's terrain (real fBm noise plus whatever carving the case adds) is
// built exactly once and then cloned for the truth pass and for each order:
// generating terrain and re-running the carve loops for every one of the
// twelve (case x order, plus one truth pass each) combinations this test
// drives cost real wall time under -race and coverage instrumentation, for
// no benefit — the terrain is a pure function of the seed, so the fourth copy
// is exactly as good as regenerating it a fourth time.
func TestSeedChunk_MatchesFullRecompute_GeneratedTerrain(t *testing.T) {
	cases := map[string]func(t *testing.T) *world.World{
		"varied heights, no carving": buildVariedHeightWorld,
		"carved cave/tunnel":         buildGeneratedCaveWorld,
		"overhang across a seam":     buildGeneratedOverhangWorld,
	}

	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			template := build(t)

			truthWorld := cloneWorld(template)
			truth := lighting.NewEngine(truthWorld)
			truth.RecomputeAll()
			expected := snapshot(truthWorld)

			forward := sortedCoords(template)

			reversed := make([]world.ChunkCoord, len(forward))
			copy(reversed, forward)
			for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
				reversed[i], reversed[j] = reversed[j], reversed[i]
			}

			shuffled := make([]world.ChunkCoord, len(forward))
			copy(shuffled, forward)
			rand.New(rand.NewSource(7)).Shuffle(len(shuffled), func(i, j int) {
				shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
			})

			orders := map[string][]world.ChunkCoord{
				"forward": forward,
				"reverse": reversed,
				"shuffle": shuffled,
			}

			for orderName, order := range orders {
				t.Run(orderName, func(t *testing.T) {
					w := cloneWorld(template)
					e := lighting.NewEngine(w)
					for _, c := range order {
						e.SeedChunk(c)
					}

					if diff := firstDifference(snapshot(w), expected); diff != "" {
						t.Fatalf("seeding chunk by chunk (order=%s) diverged from a full recompute:\n%s", orderName, diff)
					}
				})
			}
		})
	}
}

// cloneWorld returns a deep copy of src: every chunk is copied by value, so
// mutating the clone's blocks or light never touches src. It exists so a
// terrain template built once (worldgen's noise plus a case's carving) can
// be reused across a truth pass and every seeding order without regenerating
// or re-carving it each time.
func cloneWorld(src *world.World) *world.World {
	w := world.NewWorld()
	for coord, c := range src.Chunks {
		clone := new(world.Chunk)
		*clone = *c
		w.Chunks[coord] = clone
	}
	return w
}

// genSeed is fixed so every case below reproduces the same terrain from run
// to run - these tests are about the lighting engine, not about hunting for
// interesting noise.
const genSeed = 1234

// generatedGrid returns a freshly generated 3x3 chunk world centred on the
// origin, using the real worldgen generator - the same one internal/game/app.go
// wires into the pipeline - so the terrain has genuine relief instead of a
// constant height.
func generatedGrid(t *testing.T) *world.World {
	t.Helper()
	blocks.ResetToVanilla()

	g := worldgen.NewGenerator(genSeed)
	w := world.NewWorld()
	for cx := -1; cx <= 1; cx++ {
		for cz := -1; cz <= 1; cz++ {
			coord := world.ChunkCoord{X: cx, Z: cz}
			w.Chunks[coord] = g.Generate(coord)
		}
	}
	return w
}

// buildVariedHeightWorld is generated terrain with no further carving: the
// baseline "real relief" case.
func buildVariedHeightWorld(t *testing.T) *world.World {
	t.Helper()
	w := generatedGrid(t)

	// A glowstone buried well below worldgen's minimum surface height (32),
	// so it sits under solid rock in every column regardless of the terrain
	// above it. Without an emitter here, block light would trivially match
	// (everything zero) and this case would exercise skylight only.
	w.SetBlock(4, 10, 4, blocks.Glowstone)
	return w
}

// buildGeneratedCaveWorld is generated terrain with a sealed, lit tunnel
// carved along z=0 through all three x-chunks, far enough underground (y in
// 5..15) to stay below worldgen's minSurfaceHeight (32) in every column - so
// the tunnel never breaks through to open sky no matter what the surface
// noise does above it.
func buildGeneratedCaveWorld(t *testing.T) *world.World {
	t.Helper()
	w := generatedGrid(t)

	const y = 10
	for x := -16; x <= 31; x++ {
		w.SetBlock(x, y, 0, nil)
	}
	w.SetBlock(0, y, 0, blocks.Glowstone)
	return w
}

// buildGeneratedOverhangWorld is generated terrain plus a stone roof spanning
// part of chunk (1,0) and (1,-1), well above the tallest terrain anywhere in
// the grid (worldgen never exceeds maxSurfaceHeight=128), with the pocket
// underneath it cleared down to at least y=40 - below the natural surface in
// every column the roof covers, so the pocket's floor is always solid rock
// rather than open air continuing further down.
//
// That pocket has no direct path to the sky - the roof blocks straight down
// - so it can only be lit by skylight arriving sideways from chunk (0,0),
// which has no roof and is genuinely open sky at the same height. Seeding
// chunk (0,0) is what has to enqueue those open-sky cells for that light to
// ever cross the seam, and the only way to know they are worth enqueueing is
// comparing against the neighbourhood's ceiling - chunk (1,0)'s roof - not
// chunk (0,0)'s own, much lower, terrain. A ceiling rule that looks only at
// the chunk being seeded gets this case wrong.
func buildGeneratedOverhangWorld(t *testing.T) *world.World {
	t.Helper()
	w := generatedGrid(t)

	const roofY = 140 // above worldgen's maxSurfaceHeight (128) everywhere
	const floorY = 40 // below worldgen's minSurfaceHeight (32) plus margin
	for x := 16; x <= 23; x++ {
		for z := -16; z <= 15; z++ {
			w.SetBlock(x, roofY, z, blocks.Stone)
		}
	}

	// Clear the pocket under the roof, and light one corner of it with block
	// light too, so the overhang exercises both channels.
	for x := 16; x <= 23; x++ {
		for z := -16; z <= 15; z++ {
			for y := floorY; y < roofY; y++ {
				w.SetBlock(x, y, z, nil)
			}
		}
	}
	w.SetBlock(23, floorY, 0, blocks.Glowstone)

	return w
}
