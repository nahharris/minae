package chunks_test

import (
	"testing"
	"time"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// fakeRequester is a lightweight double for the pipeline a Streamer drives.
// It only records what it is told, so the desired-set logic (ring
// computation, ordering, hysteresis) can be tested without a real Pipeline,
// which is the entire reason the M15 design splits Streamer out from
// Pipeline in the first place.
type fakeRequester struct {
	requests []world.ChunkCoord
	releases []world.ChunkCoord
}

func (f *fakeRequester) Request(coord world.ChunkCoord) { f.requests = append(f.requests, coord) }
func (f *fakeRequester) Release(coord world.ChunkCoord) { f.releases = append(f.releases, coord) }

// squareSize returns how many chunks a Chebyshev square of the given radius
// contains.
func squareSize(radius int) int {
	side := 2*radius + 1
	return side * side
}

// TestStreamer_LoadedSetMatchesDesiredSquare covers validation criterion 1:
// the loaded set matches the desired set after the player moves, including
// across negative chunk coordinates, where a sign or truncation bug in the
// square computation would be most likely to show up.
func TestStreamer_LoadedSetMatchesDesiredSquare(t *testing.T) {
	t.Parallel()

	const loadRadius = 2
	const unloadRadius = loadRadius + 4 // generous margin: isolate criterion 1 from hysteresis

	fake := &fakeRequester{}
	s := chunks.NewStreamer(fake, loadRadius, unloadRadius)

	origin := world.ChunkCoord{X: 0, Z: 0}
	s.Update(origin)

	if got, want := s.Len(), squareSize(loadRadius); got != want {
		t.Fatalf("Len() = %d after first Update, want %d (a %dx%d square)", got, want, 2*loadRadius+1, 2*loadRadius+1)
	}
	for dx := -loadRadius; dx <= loadRadius; dx++ {
		for dz := -loadRadius; dz <= loadRadius; dz++ {
			coord := world.ChunkCoord{X: dx, Z: dz}
			if !s.Loaded(coord) {
				t.Errorf("%+v not loaded, but is within loadRadius %d of origin", coord, loadRadius)
			}
		}
	}

	// Move somewhere far away and across negative coordinates in both axes.
	// Everything from the origin square is now well beyond unloadRadius, so
	// it should be fully replaced.
	far := world.ChunkCoord{X: -50, Z: -37}
	s.Update(far)

	if got, want := s.Len(), squareSize(loadRadius); got != want {
		t.Fatalf("Len() = %d after moving to %+v, want %d", got, far, want)
	}
	for dx := -loadRadius; dx <= loadRadius; dx++ {
		for dz := -loadRadius; dz <= loadRadius; dz++ {
			coord := world.ChunkCoord{X: far.X + dx, Z: far.Z + dz}
			if !s.Loaded(coord) {
				t.Errorf("%+v not loaded, but is within loadRadius %d of %+v", coord, loadRadius, far)
			}
		}
	}
	if s.Loaded(origin) {
		t.Errorf("origin chunk still loaded after moving far away across negative coordinates")
	}

	// Every original chunk must have actually been released, not merely
	// absent from Loaded due to a bookkeeping bug.
	if len(fake.releases) != squareSize(loadRadius) {
		t.Errorf("%d releases after moving away, want %d (the entire old square)", len(fake.releases), squareSize(loadRadius))
	}
}

// TestStreamer_RequestsNearestFirst covers the ordering half of the M15
// design decisions: missing chunks are requested nearest first by squared
// Euclidean distance, so what the player is about to see arrives before what
// they might see later.
func TestStreamer_RequestsNearestFirst(t *testing.T) {
	t.Parallel()

	fake := &fakeRequester{}
	s := chunks.NewStreamer(fake, 4, 6)

	center := world.ChunkCoord{X: 5, Z: -3}
	s.Update(center)

	if len(fake.requests) == 0 {
		t.Fatal("no requests recorded; the test would prove nothing")
	}

	sq := func(c world.ChunkCoord) int {
		dx, dz := c.X-center.X, c.Z-center.Z
		return dx*dx + dz*dz
	}
	for i := 1; i < len(fake.requests); i++ {
		if sq(fake.requests[i]) < sq(fake.requests[i-1]) {
			t.Fatalf("request %d (%+v, sqDist %d) is closer than request %d (%+v, sqDist %d): not nearest-first",
				i, fake.requests[i], sq(fake.requests[i]), i-1, fake.requests[i-1], sq(fake.requests[i-1]))
		}
	}
	// The centre chunk itself must be requested first of all.
	if fake.requests[0] != center {
		t.Errorf("first request = %+v, want the centre chunk %+v", fake.requests[0], center)
	}
}

// TestStreamer_HysteresisPreventsThrashingAcrossOscillation covers
// validation criterion 2. It asserts on load and unload *counts* across many
// oscillations, not on the final state: a thrashing implementation reaches
// the same final loaded set every time, which is exactly why a state-only
// assertion would pass against the bug.
func TestStreamer_HysteresisPreventsThrashingAcrossOscillation(t *testing.T) {
	t.Parallel()

	const loadRadius = 3
	const unloadRadius = loadRadius + 2 // the M15 hysteresis margin

	a := world.ChunkCoord{X: 0, Z: 0}
	b := world.ChunkCoord{X: 1, Z: 0} // one chunk over: straddling a single boundary

	// Reference: what one visit to each side costs, with no oscillation.
	oneVisitEach := &fakeRequester{}
	s1 := chunks.NewStreamer(oneVisitEach, loadRadius, unloadRadius)
	s1.Update(a)
	s1.Update(b)
	wantRequests := len(oneVisitEach.requests)
	wantReleases := len(oneVisitEach.releases)

	fake := &fakeRequester{}
	s := chunks.NewStreamer(fake, loadRadius, unloadRadius)
	s.Update(a)

	const oscillations = 25
	for i := 0; i < oscillations; i++ {
		s.Update(b)
		s.Update(a)
	}

	if len(fake.releases) != wantReleases {
		t.Fatalf("%d releases after %d oscillations across one boundary, want exactly %d (hysteresis should prevent any repeated unload)",
			len(fake.releases), oscillations, wantReleases)
	}
	if len(fake.requests) != wantRequests {
		t.Fatalf("%d total requests after %d oscillations, want exactly %d (only the first visit to each side should ever request anything)",
			len(fake.requests), oscillations, wantRequests)
	}
}

// TestStreamer_UpdateNeverBlocks covers validation criterion 7 at the
// streaming layer: recomputing the desired set and driving one Pipeline
// Update call must stay bounded even when the desired set is the full
// default-view-distance square (289 chunks), because nothing here may wait
// on generation, lighting or meshing to finish.
func TestStreamer_UpdateNeverBlocks(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	// A small fixed worker count, not runtime.NumCPU(): this test (and the
	// suite around it) already runs alongside every other package's tests
	// under `go test ./...`, and a pipeline sized to the whole machine here
	// competes for CPU with unrelated packages instead of just this one.
	p := chunks.NewPipeline(w, light, chunks.FlatGenerator{}, nil, 4)
	defer p.Close()

	const viewDistance = 8 // the M15 default: a 17x17, 289-chunk square
	s := chunks.NewStreamer(p, viewDistance, viewDistance+2)

	const budget = 200 * time.Millisecond
	start := time.Now()
	s.Update(world.ChunkCoord{})
	p.Update(chunks.Budget{Light: time.Second, Mesh: time.Second})
	elapsed := time.Since(start)

	if elapsed > budget {
		t.Fatalf("Streamer.Update + Pipeline.Update took %v against a fresh 289-chunk desired set, want under %v", elapsed, budget)
	}
}

// TestStreamer_MemoryStaysAtSteadyStateOverALongWalk covers validation
// criterion 8's loaded-chunk-count half: walking a long straight path must
// not make the streamer's resident set grow without bound. The mesh-count
// half (what the renderer holds) is covered in internal/game, where the
// meshRemover wiring lives.
func TestStreamer_MemoryStaysAtSteadyStateOverALongWalk(t *testing.T) {
	fake := &fakeRequester{}
	const loadRadius = 2
	const unloadRadius = loadRadius + 2
	s := chunks.NewStreamer(fake, loadRadius, unloadRadius)

	const steps = 40
	counts := make([]int, 0, steps)
	for i := 0; i < steps; i++ {
		s.Update(world.ChunkCoord{X: i, Z: 0})
		counts = append(counts, s.Len())
	}

	// Skip the initial transient while the trailing edge is still catching
	// up to the unload radius; from there on the count must be identical at
	// every step, not merely bounded -- walking in a straight line is
	// translation-invariant, so a fixed steady-state value is the correct
	// property, and a value that keeps climbing is exactly the leak this
	// criterion exists to catch.
	const settleAfter = unloadRadius + 1
	want := counts[settleAfter]
	for i := settleAfter; i < len(counts); i++ {
		if counts[i] != want {
			t.Fatalf("step %d: loaded count = %d, want steady %d (all counts: %v)", i, counts[i], want, counts)
		}
	}
}
