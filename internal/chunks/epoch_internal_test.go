package chunks

import (
	"testing"

	"github.com/nahharris/minae/internal/world"
)

// This file is deliberately in package chunks rather than chunks_test, which
// every other test here uses. The property under test — that no two mesh jobs
// ever carry the same epoch — is not reachable from outside the package, and
// the end-to-end failure it causes needs an interleaving the public API
// cannot schedule: a mesh job must still be in flight while its coord is
// released, re-requested, regenerated, re-lit and dispatched again, and then
// its result must arrive before the second job's. Nothing exported can hold a
// mesh worker at a chosen point, so a black-box version would depend on
// winning a race rather than on the bug being present.
//
// Testing the invariant directly is the honest alternative. It is stated in
// terms of drainMeshed's own staleness expression so it cannot drift away
// from the thing it protects.
//
// What this does not cover, stated plainly: the test mirrors dispatchMeshing's
// two lines rather than calling it, because dispatchMeshing needs workers,
// channels and a lit world. So it verifies the counter and its interaction
// with Release, and it would not catch someone changing a single production
// call site back to a per-coord increment while leaving takeEpochLocked
// intact. Every site that assigns an epoch must draw from takeEpochLocked;
// that is a rule the reader has to keep, not one this test enforces.

// TestEpochIsNeverReusedAcrossRelease pins down why epochs come from a single
// monotonic counter instead of a per-coord one.
//
// The sequence is the one streaming produces whenever a player walks away
// from a chunk and comes back while its mesh is still being built: dispatch,
// release, request, dispatch again. With per-coord counters, Release deletes
// the coord's entry, the re-requested coord starts from zero, and its first
// job draws the same epoch the still-in-flight job from the previous life is
// carrying. drainMeshed then sees a matching epoch on a chunk that is legitimately
// Meshing and accepts a mesh built against a neighbourhood that no longer
// exists — and, because that marks the chunk Meshed, the correct result
// arriving afterwards is the one discarded.
func TestEpochIsNeverReusedAcrossRelease(t *testing.T) {
	p := &Pipeline{
		w:      world.NewWorld(),
		stages: make(map[world.ChunkCoord]Stage),
		epoch:  make(map[world.ChunkCoord]int),
	}

	coord := world.ChunkCoord{X: 3, Z: -2}

	// The chunk's first life: requested, and a mesh job dispatched for it.
	// This mirrors dispatchMeshing's two lines rather than calling it, so the
	// test needs no workers, no world content and no channels.
	p.Request(coord)
	p.mu.Lock()
	p.epoch[coord] = p.takeEpochLocked()
	p.stages[coord] = Meshing
	inFlight := p.epoch[coord]
	p.mu.Unlock()

	// The player walks away, then comes back, and the chunk is dispatched
	// again — all while the first job is still meshing.
	p.Release(coord)
	p.Request(coord)

	p.mu.Lock()
	p.epoch[coord] = p.takeEpochLocked()
	p.stages[coord] = Meshing
	redispatched := p.epoch[coord]
	p.mu.Unlock()

	if redispatched == inFlight {
		t.Fatalf("a mesh job dispatched after Release reused epoch %d from a job "+
			"still in flight; epochs must come from a monotonic counter", inFlight)
	}
	if redispatched <= inFlight {
		t.Errorf("epoch went backwards across Release: %d then %d", inFlight, redispatched)
	}

	// The consequence, written exactly as drainMeshed decides it. This is the
	// assertion that actually matters: the invariant above is only worth
	// having because it makes this come out true.
	p.mu.Lock()
	stale := p.stages[coord] != Meshing || p.epoch[coord] != inFlight
	p.mu.Unlock()

	if !stale {
		t.Error("the in-flight job's result would be accepted as current after " +
			"its coord was released and re-requested; it was meshed against a " +
			"neighbourhood that no longer exists")
	}
}

// TestTakeEpochNeverRepeats is the narrow guard under the test above: every
// value the counter hands out is distinct, whichever call site asked for it.
func TestTakeEpochNeverRepeats(t *testing.T) {
	p := &Pipeline{
		w:      world.NewWorld(),
		stages: make(map[world.ChunkCoord]Stage),
		epoch:  make(map[world.ChunkCoord]int),
	}

	seen := make(map[int]bool)
	p.mu.Lock()
	defer p.mu.Unlock()
	for range 1000 {
		e := p.takeEpochLocked()
		if seen[e] {
			t.Fatalf("takeEpochLocked returned %d twice", e)
		}
		seen[e] = true
	}
}
