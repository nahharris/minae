package lighting_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// buildSeedTestWorld returns a fresh 3x3 chunk world: flat terrain with a
// horizontal, fully enclosed tunnel running along z=0 through all three
// x-chunks, lit by a single glowstone near its middle. The tunnel gives block
// light something to carry across two seams, and the flat terrain's open sky
// gives skylight the same cross-seam continuity every other test in this
// package relies on.
//
// A high surface, like the property test's, keeps the open-sky portion of
// each column thin so the queues stay small under -race.
func buildSeedTestWorld(t *testing.T) *world.World {
	t.Helper()
	const surface = 240

	return testutil.NewWorld(t).
		Chunks(-1, -1, 1, 1).
		Flat(surface).
		Clear(testutil.Box{MinX: -16, MaxX: 31, MinY: 20, MaxY: 20, MinZ: 0, MaxZ: 0}).
		Fill(testutil.Box{MinX: 0, MaxX: 0, MinY: 20, MaxY: 20, MinZ: 0, MaxZ: 0}, blocks.Glowstone).
		Build()
}

// sortedCoords returns every loaded chunk's coordinates in a fixed order, so
// a "forward" run has something deterministic to be the baseline for the
// reversed and shuffled runs.
func sortedCoords(w *world.World) []world.ChunkCoord {
	coords := make([]world.ChunkCoord, 0, len(w.Chunks))
	for c := range w.Chunks {
		coords = append(coords, c)
	}
	sort.Slice(coords, func(i, j int) bool {
		if coords[i].X != coords[j].X {
			return coords[i].X < coords[j].X
		}
		return coords[i].Z < coords[j].Z
	})
	return coords
}

// TestSeedChunk_MatchesFullRecompute is the load-bearing equivalence check:
// lighting a world one chunk at a time through SeedChunk must produce exactly
// what RecomputeAll produces over the same finished world, for both light
// channels.
func TestSeedChunk_MatchesFullRecompute(t *testing.T) {
	w := buildSeedTestWorld(t)

	e := lighting.NewEngine(w)
	for _, c := range sortedCoords(w) {
		e.SeedChunk(c)
	}
	incremental := snapshot(w)

	truth := lighting.NewEngine(w)
	truth.RecomputeAll()
	expected := snapshot(w)

	if diff := firstDifference(incremental, expected); diff != "" {
		t.Fatalf("seeding chunk by chunk diverged from a full recompute:\n%s", diff)
	}
}

// TestSeedChunk_OrderIndependent seeds the same finished world's chunks in
// several different orders - forward, reversed, and two shuffles - and
// requires the result to be identical every time.
//
// This is exactly the property the previous SeedChunk (deleted in M3) got
// wrong: it wiped its own chunk before reseeding from neighbours, so the
// result depended on which chunk was seeded next. An add-only seed has no
// wipe to make order matter, and this test is what actually holds it to
// that rather than assuming it from the reasoning alone.
func TestSeedChunk_OrderIndependent(t *testing.T) {
	seedInOrder := func(coords []world.ChunkCoord) map[world.ChunkCoord]lightState {
		w := buildSeedTestWorld(t)
		e := lighting.NewEngine(w)
		for _, c := range coords {
			e.SeedChunk(c)
		}
		return snapshot(w)
	}

	forward := sortedCoords(buildSeedTestWorld(t))
	reference := seedInOrder(forward)

	reversed := make([]world.ChunkCoord, len(forward))
	copy(reversed, forward)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	shuffled1 := make([]world.ChunkCoord, len(forward))
	copy(shuffled1, forward)
	rand.New(rand.NewSource(1)).Shuffle(len(shuffled1), func(i, j int) {
		shuffled1[i], shuffled1[j] = shuffled1[j], shuffled1[i]
	})

	shuffled2 := make([]world.ChunkCoord, len(forward))
	copy(shuffled2, forward)
	rand.New(rand.NewSource(2)).Shuffle(len(shuffled2), func(i, j int) {
		shuffled2[i], shuffled2[j] = shuffled2[j], shuffled2[i]
	})

	cases := map[string][]world.ChunkCoord{
		"reverse":    reversed,
		"shuffled-1": shuffled1,
		"shuffled-2": shuffled2,
	}

	for name, order := range cases {
		t.Run(name, func(t *testing.T) {
			if diff := firstDifference(seedInOrder(order), reference); diff != "" {
				t.Errorf("seeding in a different order changed the result:\n%s", diff)
			}
		})
	}
}

// TestSeedChunk_LightFlowsAcrossNewSeam checks both directions a new seam can
// carry light: an already-lit chunk brightening a dark chunk that just
// arrived beside it, and a newly-arrived emitter lighting the already-present
// chunk next to it.
func TestSeedChunk_LightFlowsAcrossNewSeam(t *testing.T) {
	const surface = 240

	t.Run("an existing chunk lights the dark new chunk beside it", func(t *testing.T) {
		// Chunk (0,0) has a lit tunnel reaching its own edge. Chunk (1,0) does
		// not exist yet.
		b := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Flat(surface).
			Clear(testutil.Box{MinX: 8, MaxX: 15, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8}).
			Fill(testutil.Box{MinX: 8, MaxX: 8, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8}, blocks.Glowstone)
		w := b.Build()

		e := lighting.NewEngine(w)
		e.SeedChunk(world.ChunkCoord{X: 0, Z: 0})

		if got := w.GetBlockLight(15, 20, 8); got == 0 {
			t.Fatal("precondition failed: chunk (0,0)'s tunnel should already be lit at its own edge")
		}

		// Now chunk (1,0) arrives: same tunnel continues into it, but it has
		// not been seeded yet, so it must start dark even though its terrain
		// is already in place.
		b.Chunks(1, 0, 1, 0).FlatChunk(1, 0, surface).
			Clear(testutil.Box{MinX: 16, MaxX: 20, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8})

		if got := w.GetBlockLight(16, 20, 8); got != 0 {
			t.Fatalf("precondition failed: the new chunk should start dark before being seeded, got %d", got)
		}

		e.SeedChunk(world.ChunkCoord{X: 1, Z: 0})

		if got := w.GetBlockLight(16, 20, 8); got == 0 {
			t.Error("light did not flow from the existing chunk into the new chunk across the seam")
		}
	})

	t.Run("a new chunk's glowstone lights the chunk already beside it", func(t *testing.T) {
		// Both chunks exist and the terrain is laid before anything is carved:
		// Flat rewrites every allocated chunk, so carving first and flattening
		// afterwards would refill the tunnel with stone.
		//
		// Only chunk (1,0) is seeded. Chunk (0,0) stands in for a neighbour that
		// is present but dark, which is what the arriving chunk must light.
		w := testutil.NewWorld(t).Chunks(0, 0, 1, 0).Flat(surface).
			Clear(testutil.Box{MinX: 8, MaxX: 20, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8}).
			Fill(testutil.Box{MinX: 20, MaxX: 20, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8}, blocks.Glowstone).
			Build()

		if got := w.GetBlockLight(15, 20, 8); got != 0 {
			t.Fatalf("precondition failed: nothing is lit before seeding, got %d", got)
		}

		e := lighting.NewEngine(w)
		e.SeedChunk(world.ChunkCoord{X: 1, Z: 0})

		if got := w.GetBlockLight(15, 20, 8); got == 0 {
			t.Error("the new chunk's glowstone did not light the chunk already beside it")
		}
	})
}

// TestSeedChunk_DirtyTracking checks that SeedChunk's writes flow through the
// engine's normal dirty tracking rather than bypassing it: seeding a new
// chunk that brightens its neighbour's border cell must report both chunks,
// exactly as markMeshDirty's border rule intends.
func TestSeedChunk_DirtyTracking(t *testing.T) {
	const surface = 240

	b := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Flat(surface).
		Clear(testutil.Box{MinX: 8, MaxX: 15, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8}).
		Fill(testutil.Box{MinX: 8, MaxX: 8, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8}, blocks.Glowstone)
	w := b.Build()

	e := lighting.NewEngine(w)
	e.SeedChunk(world.ChunkCoord{X: 0, Z: 0})
	e.DirtyChunks() // drain the initial seeding churn

	b.Chunks(1, 0, 1, 0).FlatChunk(1, 0, surface).
		Clear(testutil.Box{MinX: 16, MaxX: 20, MinY: 20, MaxY: 20, MinZ: 8, MaxZ: 8})

	e.SeedChunk(world.ChunkCoord{X: 1, Z: 0})

	dirty := make(map[world.ChunkCoord]bool)
	for _, c := range e.DirtyChunks() {
		if dirty[c] {
			t.Errorf("chunk %+v reported dirty twice", c)
		}
		dirty[c] = true
	}

	if !dirty[world.ChunkCoord{X: 1, Z: 0}] {
		t.Error("the newly seeded chunk was not reported dirty")
	}
	if !dirty[world.ChunkCoord{X: 0, Z: 0}] {
		t.Error("the existing neighbour was not reported dirty, even though light crossed the seam into a " +
			"cell its own mesh samples from")
	}
}

// TestSeedChunk_Idempotent checks that seeding an already-seeded set of
// chunks a second time is a no-op: no light value moves and nothing is
// reported dirty. A seed that wiped anything before reseeding - like the one
// M3 deleted - would fail this by construction.
func TestSeedChunk_Idempotent(t *testing.T) {
	w := buildSeedTestWorld(t)
	e := lighting.NewEngine(w)

	coords := sortedCoords(w)
	for _, c := range coords {
		e.SeedChunk(c)
	}
	e.DirtyChunks() // drain the first pass's churn

	before := snapshot(w)

	for _, c := range coords {
		e.SeedChunk(c)
	}

	if diff := firstDifference(snapshot(w), before); diff != "" {
		t.Fatalf("seeding the same chunks twice changed light:\n%s", diff)
	}

	if dirty := e.DirtyChunks(); len(dirty) != 0 {
		t.Errorf("re-seeding an already-seeded world reported chunks dirty: %v", dirty)
	}
}
