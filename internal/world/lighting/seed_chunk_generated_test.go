package lighting_test

import (
	"math/rand"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
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
	// allOrders marks the cases worth re-running reversed and shuffled as
	// well as forward.
	//
	// Equivalence and order-independence are two different properties, and
	// running every terrain shape through every order multiplies them for
	// little return: order-independence has its own dedicated test
	// (TestSeedChunk_OrderIndependent), and the shapes where order could
	// plausibly interact with terrain are the ones whose geometry spans a
	// seam. Those get all three orders; the rest get forward only.
	//
	// This is a real cost, not tidiness. Under -race with coverage this
	// package runs roughly 25x slower than plain, and M18 made it worse in a
	// way that is easy to miss: leaves are light-transparent, so light now
	// propagates *through* every canopy rather than stopping at it, and the
	// generated terrain every case starts from has trees in it now. The suite
	// crossed Go's ten-minute default timeout and failed CI outright.
	cases := []struct {
		name      string
		build     func(t *testing.T) *world.World
		allOrders bool
	}{
		{"varied heights, no carving", buildVariedHeightWorld, false},
		{"carved cave/tunnel", buildGeneratedCaveWorld, false},
		{"overhang across a seam", buildGeneratedOverhangWorld, true},
		{"tree canopy across a seam", buildGeneratedTreeWorld, false},
		{"canopy above the neighbourhood ceiling", buildCanopyAboveCeilingWorld, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			template := tc.build(t)

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

			orders := map[string][]world.ChunkCoord{"forward": forward}
			if tc.allOrders {
				orders["reverse"] = reversed
				orders["shuffle"] = shuffled
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

// buildGeneratedTreeWorld is generated terrain with a synthetic tree -- a
// Wood trunk and a Leaves canopy -- planted straddling the seam between
// chunk (0,0) and (1,0). This is the M18 extension the milestone calls for:
// the safety net for the whole transparency change needs terrain WITH trees,
// and specifically at a chunk boundary, since every seam bug this project
// has hit (M3's original black wall, M15's follow-up) lived exactly there.
//
// The tree is placed by hand rather than through worldgen's own feature
// placement, so this test does not depend on a seed happening to produce a
// tree in the right spot. The root column is chosen deliberately at the
// boundary (x=15, next to x=16 in chunk (1,0)), and the trunk's base is read
// from the real Generator.SurfaceHeight at that column, so it is genuinely
// rooted on the generated ground rather than floating at a hand-picked
// height that might not match the real terrain there.
func buildGeneratedTreeWorld(t *testing.T) *world.World {
	t.Helper()
	w := generatedGrid(t)

	g := worldgen.NewGenerator(genSeed)
	const rootX, rootZ = 15, 4 // straddles the chunk (0,0)/(1,0) seam at x=15/16
	groundY := g.SurfaceHeight(rootX, rootZ)

	const trunkHeight = 5
	for dy := 0; dy < trunkHeight; dy++ {
		w.SetBlock(rootX, groundY+dy, rootZ, blocks.Wood)
	}

	// A canopy loose enough to cross the seam in every direction, skipping
	// the trunk column and never overwriting a cell that already holds
	// something (matching how features.go's placeIfInChunk never overwrites
	// existing terrain).
	top := groundY + trunkHeight - 1
	for dx := -2; dx <= 2; dx++ {
		for dz := -2; dz <= 2; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			for dy := -1; dy <= 1; dy++ {
				gx, gy, gz := rootX+dx, top+dy, rootZ+dz
				if w.GetBlock(gx, gy, gz) == nil {
					w.SetBlock(gx, gy, gz, blocks.Leaves)
				}
			}
		}
	}

	return w
}

// buildCanopyAboveCeilingWorld plants a canopy that provably sits above the
// highest terrain anywhere in the region, with open air beneath it.
//
// This exists because buildGeneratedTreeWorld does not exercise the case it
// looks like it does. Its canopy tops out about five blocks above its own
// root column, and generated terrain spans roughly twenty blocks of relief,
// so the canopy usually ends up *below* the neighbourhood's highest solid
// block. Every canopy cell is then enqueued regardless and the sky-ceiling
// optimisation is never actually put under strain.
//
// The strain matters. `SeedChunk` skips enqueueing sky cells above the
// highest light-opaque block in the 3x3 neighbourhood, on the argument that
// every column up there is open sky at full brightness. That argument holds
// only while "light-opaque" means the same thing to `Chunk.highestSolidY` as
// it does to the light engine. Break that tie — let something block light
// without raising the ceiling — and the cells it shades from above are never
// queued, so light never reaches them sideways. Verified: with leaves made
// light-opaque while excluded from the ceiling, this case fails and the four
// above it pass.
//
// The canopy height is computed from the terrain rather than hardcoded, so
// the test keeps testing this even if the generator is retuned.
func buildCanopyAboveCeilingWorld(t *testing.T) *world.World {
	t.Helper()
	w := generatedGrid(t)

	g := worldgen.NewGenerator(genSeed)

	// The real ceiling is the highest solid block across the whole region, so
	// find it rather than guessing.
	highest := 0
	for x := -config.ChunkWidth; x < 2*config.ChunkWidth; x++ {
		for z := -config.ChunkWidth; z < 2*config.ChunkWidth; z++ {
			if h := g.SurfaceHeight(x, z); h > highest {
				highest = h
			}
		}
	}

	// Well clear of it, leaving a band of open air between the terrain and
	// the canopy. That band is the part that goes dark when the tie breaks:
	// it is above the ceiling, and it is in the canopy's shadow.
	const clearance = 8
	canopyY := highest + clearance
	if canopyY+1 >= config.ChunkHeight {
		t.Fatalf("canopy at y=%d does not fit below the world height %d", canopyY, config.ChunkHeight)
	}

	// Straddling the (0,0)/(1,0) seam, because a seam is where every lighting
	// bug this project has had actually lived.
	for x := 12; x <= 20; x++ {
		for z := 2; z <= 8; z++ {
			w.SetBlock(x, canopyY, z, blocks.Leaves)
		}
	}

	return w
}
