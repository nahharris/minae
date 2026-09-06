package lighting_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// The incremental engine and a full recompute must agree exactly.
//
// This is the test that actually guards the engine. Hand-written cases check
// the situations someone thought of; this one checks the situations nobody did.
// Every bug the previous engine shipped with — light that could brighten but
// never darken, light that stopped at a chunk seam, downward propagation that
// decayed when it should not — shows up here as a divergence.
//
// The ground truth is RecomputeAll, which is order-independent by construction:
// it zeroes every loaded chunk, scans all columns into one queue, and
// propagates once.
func TestIncrementalMatchesFullRecompute(t *testing.T) {
	// Kept deliberately small. Each edit is followed by a full recompute of
	// every loaded chunk, and a chunk is 16*16*256 cells, so the cost climbs
	// fast — especially under -race, which CI runs. A 2x2 grid still has seams
	// in both axes and on both sides of the origin, which is where the
	// cross-chunk bugs live; more chunks would buy coverage of nothing new.
	const (
		seeds        = 8
		editsPerSeed = 8
	)

	for seed := int64(0); seed < seeds; seed++ {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			rng := rand.New(rand.NewSource(seed))

			w := randomWorld(t, rng)

			engine := lighting.NewEngine(w)
			engine.RecomputeAll()

			for i := range editsPerSeed {
				x, y, z := randomEditPos(rng)

				var placed *blocks.Block
				if rng.Intn(2) == 0 {
					placed = blocks.Stone
				}
				w.SetBlock(x, y, z, placed)
				engine.OnBlockChanged(x, y, z)

				// Comparing after every edit rather than only at the end means a
				// failure names the edit that broke it, instead of leaving 40
				// candidates.
				incremental := snapshot(w)

				truth := lighting.NewEngine(w)
				truth.RecomputeAll()
				expected := snapshot(w)

				if diff := firstDifference(incremental, expected); diff != "" {
					t.Fatalf("after edit %d at (%d,%d,%d) placing %v:\n%s",
						i, x, y, z, blockName(placed), diff)
				}

				// RecomputeAll just overwrote the world's light with the same
				// values, so the incremental engine may continue from here.
			}
		})
	}
}

// Breaking a block and putting it back must return the world to exactly the
// state it started in. An engine that can brighten but not darken passes every
// "is the cave lit" test and fails this one.
func TestPlaceThenBreakRoundTrips(t *testing.T) {
	const surface = 32

	w := testutil.NewWorld(t).Chunks(-1, -1, 1, 1).Flat(surface).Build()

	engine := lighting.NewEngine(w)
	engine.RecomputeAll()
	before := snapshot(w)

	// A spot on a chunk seam, so the round trip has to survive cross-chunk work.
	const x, y, z = 15, surface + 1, 15

	w.SetBlock(x, y, z, blocks.Stone)
	engine.OnBlockChanged(x, y, z)

	if diff := firstDifference(snapshot(w), before); diff == "" {
		t.Fatal("placing a block above the surface changed no light at all; the engine is not darkening anything")
	}

	w.SetBlock(x, y, z, nil)
	engine.OnBlockChanged(x, y, z)

	if diff := firstDifference(snapshot(w), before); diff != "" {
		t.Errorf("light did not return to its original state after break:\n%s", diff)
	}
}

// Every chunk whose light changed must be reported, and no chunk whose light
// did not change may be. Under-reporting leaves stale meshes on screen, which
// is exactly the "cross-chunk updates are invisible" symptom.
func TestDirtyChunksMatchesActualChanges(t *testing.T) {
	const surface = 32

	w := testutil.NewWorld(t).Chunks(-1, -1, 1, 1).Flat(surface).Build()

	engine := lighting.NewEngine(w)
	engine.RecomputeAll()
	engine.DirtyChunks() // drain the seeding churn

	before := snapshot(w)

	// Carve a shaft on the seam between chunk (0,0) and chunk (-1,0) so light
	// spills sideways into the neighbour.
	const x, z = 0, 8
	for y := surface; y > surface-6; y-- {
		w.SetBlock(x, y, z, nil)
		engine.OnBlockChanged(x, y, z)
	}

	reported := make(map[world.ChunkCoord]bool)
	for _, coord := range engine.DirtyChunks() {
		if reported[coord] {
			t.Errorf("chunk %+v reported dirty twice", coord)
		}
		reported[coord] = true
	}

	after := snapshot(w)
	for coord, lightAfter := range after {
		changed := lightAfter != before[coord]
		switch {
		case changed && !reported[coord]:
			t.Errorf("chunk %+v changed but was not reported dirty; its mesh would go stale", coord)
		case !changed && reported[coord]:
			t.Errorf("chunk %+v was reported dirty but its light is identical; that is a wasted re-mesh", coord)
		}
	}
}

// The property world is a 2x2 chunk grid spanning block coordinates -16..15 in
// both axes, so seams fall on the origin and on negative coordinates.
const (
	propMinBlock = -16
	propMaxBlock = 15
	// A high surface keeps the empty sky above it thin. Most of a 256-tall
	// chunk would otherwise be lit cells that the recompute must enqueue and
	// walk, which costs real time under -race and coverage instrumentation and
	// exercises nothing: the interesting behaviour is all at the terrain
	// boundary and in the deep column beneath it.
	propSurface = 240
)

// randomWorld builds flat terrain, then carves and stacks a few random boxes so
// there are overhangs, sealed pockets and shafts for light to interact with.
// Flat terrain alone would exercise almost none of the propagation rules.
func randomWorld(t *testing.T, rng *rand.Rand) *world.World {
	t.Helper()

	b := testutil.NewWorld(t).Chunks(-1, -1, 0, 0).Flat(propSurface)

	for range 6 {
		box := randomBox(rng)
		if rng.Intn(2) == 0 {
			b = b.Clear(box)
		} else {
			b = b.Fill(box, blocks.Stone)
		}
	}
	return b.Build()
}

// randomBox returns a small box clamped inside the allocated grid. The builder
// fails the test on an out-of-range box, so the clamping is load-bearing.
func randomBox(rng *rand.Rand) testutil.Box {
	span := propMaxBlock - propMinBlock

	x := propMinBlock + rng.Intn(span)
	z := propMinBlock + rng.Intn(span)
	y := propSurface - 4 + rng.Intn(10)

	return testutil.Box{
		MinX: x, MaxX: min(x+rng.Intn(5), propMaxBlock),
		MinY: y, MaxY: y + rng.Intn(3),
		MinZ: z, MaxZ: min(z+rng.Intn(5), propMaxBlock),
	}
}

// randomEditPos returns a position inside the grid, in the vertical band where
// terrain and air meet — edits far above or below it would be no-ops.
func randomEditPos(rng *rand.Rand) (x, y, z int) {
	span := propMaxBlock - propMinBlock + 1
	return propMinBlock + rng.Intn(span),
		propSurface - 4 + rng.Intn(10),
		propMinBlock + rng.Intn(span)
}

// lightState is a copy of one chunk's skylight array.
type lightState = [config.ChunkWidth * config.ChunkWidth * config.ChunkHeight]uint8

func snapshot(w *world.World) map[world.ChunkCoord]lightState {
	out := make(map[world.ChunkCoord]lightState, len(w.Chunks))
	for coord, chunk := range w.Chunks {
		out[coord] = chunk.SkyLight
	}
	return out
}

// firstDifference returns a human-readable description of the first cell where
// the two snapshots disagree, or "" if they are identical. It reports a single
// cell with its coordinates rather than dumping two 64KB arrays.
func firstDifference(got, want map[world.ChunkCoord]lightState) string {
	if len(got) != len(want) {
		return fmt.Sprintf("chunk count differs: got %d, want %d", len(got), len(want))
	}

	for coord, wantLight := range want {
		gotLight, ok := got[coord]
		if !ok {
			return fmt.Sprintf("chunk %+v missing from result", coord)
		}
		if gotLight == wantLight {
			continue
		}

		for i := range wantLight {
			if gotLight[i] == wantLight[i] {
				continue
			}
			lx, y, lz := unindex(i)
			return fmt.Sprintf(
				"chunk %+v local (%d,%d,%d) global (%d,%d,%d): incremental=%d full-recompute=%d",
				coord, lx, y, lz,
				coord.X*config.ChunkWidth+lx, y, coord.Z*config.ChunkWidth+lz,
				gotLight[i], wantLight[i],
			)
		}
	}
	return ""
}

// unindex inverts Chunk's flat index: x + z*width + y*width*width.
func unindex(i int) (x, y, z int) {
	const w = config.ChunkWidth
	return i % w, i / (w * w), (i / w) % w
}

func blockName(b *blocks.Block) string {
	if b == nil {
		return "air"
	}
	return b.ID
}
