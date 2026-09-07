package chunks_test

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// maxDriveIterations bounds every loop in this file that repeatedly calls
// Update waiting for a condition. Each iteration is a handful of non-blocking
// channel operations, so it never legitimately takes more than a few hundred
// of them; a run that does is a hang, and failing loudly beats the test
// suite blocking forever.
const maxDriveIterations = 10_000

// generousBudget lights and meshes far more than nine chunks can ever need in
// one call, so tests that are not specifically about budgeting can drive the
// pipeline to completion in the fewest possible calls.
var generousBudget = chunks.Budget{Light: 64, Mesh: 64}

// stubGenerator is a Generator a test can inspect and, per coord, hold up
// deliberately. It always eventually returns an empty chunk (air throughout,
// via world.NewChunk) — these tests are about the stage machine, not terrain.
type stubGenerator struct {
	mu    sync.Mutex
	calls map[world.ChunkCoord]int
	gate  map[world.ChunkCoord]<-chan struct{}
}

func newStubGenerator() *stubGenerator {
	return &stubGenerator{calls: make(map[world.ChunkCoord]int)}
}

// Generate implements chunks.Generator.
func (g *stubGenerator) Generate(coord world.ChunkCoord) *world.Chunk {
	g.mu.Lock()
	g.calls[coord]++
	gate := g.gate[coord]
	g.mu.Unlock()

	if gate != nil {
		<-gate
	}
	return world.NewChunk(coord.X, coord.Z)
}

// callCount reports how many times Generate has been called for coord.
func (g *stubGenerator) callCount(coord world.ChunkCoord) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls[coord]
}

// holdBack makes Generate block for coord until the returned func is called.
func (g *stubGenerator) holdBack(coord world.ChunkCoord) (release func()) {
	ch := make(chan struct{})
	g.mu.Lock()
	if g.gate == nil {
		g.gate = make(map[world.ChunkCoord]<-chan struct{})
	}
	g.gate[coord] = ch
	g.mu.Unlock()

	var once sync.Once
	return func() { once.Do(func() { close(ch) }) }
}

// newTestPipeline builds a pipeline over a fresh, empty world and lighting
// engine, using testutil so the global block registry is reset first.
func newTestPipeline(t *testing.T, gen chunks.Generator, workers int) (*chunks.Pipeline, *world.World) {
	t.Helper()
	w := testutil.NewWorld(t).Build()
	light := lighting.NewEngine(w)
	return chunks.NewPipeline(w, light, gen, nil, workers), w
}

// drainUntil repeatedly calls Update with budget until cond reports true,
// failing the test if that takes more than maxDriveIterations calls.
func drainUntil(t *testing.T, p *chunks.Pipeline, budget chunks.Budget, cond func() bool) {
	t.Helper()
	for i := 0; i < maxDriveIterations; i++ {
		p.Update(budget)
		if cond() {
			return
		}
	}
	t.Fatal("condition never became true within maxDriveIterations Update calls")
}

// atLeast reports whether every coord in coords has reached at least min.
func atLeast(p *chunks.Pipeline, coords []world.ChunkCoord, min chunks.Stage) bool {
	for _, c := range coords {
		if p.Stage(c) < min {
			return false
		}
	}
	return true
}

// grid3x3 returns the nine chunk coordinates centred on the origin.
func grid3x3() []world.ChunkCoord {
	coords := make([]world.ChunkCoord, 0, 9)
	for x := -1; x <= 1; x++ {
		for z := -1; z <= 1; z++ {
			coords = append(coords, world.ChunkCoord{X: x, Z: z})
		}
	}
	return coords
}

// neighbours8 returns coord's eight axis- and diagonally-adjacent chunks.
func neighbours8(coord world.ChunkCoord) []world.ChunkCoord {
	out := make([]world.ChunkCoord, 0, 8)
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			out = append(out, world.ChunkCoord{X: coord.X + dx, Z: coord.Z + dz})
		}
	}
	return out
}

// TestPipeline_StageRuleHolds drives a full 3x3 neighbourhood to completion
// while continuously checking, after every single Update call, that no chunk
// has ever gotten ahead of its neighbours: Lit implies every neighbour is at
// least Generated, and Meshing/Meshed implies every neighbour is at least
// Lit. The check runs on observable Stage values, not on any output, exactly
// as the milestone asks.
func TestPipeline_StageRuleHolds(t *testing.T) {
	gen := newStubGenerator()
	p, _ := newTestPipeline(t, gen, 4)
	defer p.Close()

	coords := grid3x3()
	requested := make(map[world.ChunkCoord]bool, len(coords))
	for _, c := range coords {
		p.Request(c)
		requested[c] = true
	}

	// The gate is defined in terms of requested neighbours only (see
	// neighborsAtLeastLocked's doc comment): a neighbour outside the 3x3 was
	// never requested and, by design, does not hold anything back. So the
	// invariant below is checked only against neighbours that are themselves
	// part of this requested set.
	checkInvariant := func() {
		t.Helper()
		for _, c := range coords {
			stage := p.Stage(c)
			if stage < chunks.Lit {
				continue
			}
			for _, n := range neighbours8(c) {
				if requested[n] && p.Stage(n) < chunks.Generated {
					t.Fatalf("%v reached Lit while requested neighbour %v is only %v", c, n, p.Stage(n))
				}
			}
			if stage < chunks.Meshing {
				continue
			}
			for _, n := range neighbours8(c) {
				if requested[n] && p.Stage(n) < chunks.Lit {
					t.Fatalf("%v reached %v while requested neighbour %v is only %v", c, stage, n, p.Stage(n))
				}
			}
		}
	}

	for i := 0; i < maxDriveIterations; i++ {
		p.Update(generousBudget)
		checkInvariant()
		if atLeast(p, coords, chunks.Meshed) {
			return
		}
	}
	t.Fatal("the 3x3 neighbourhood never finished meshing")
}

// TestPipeline_RequestEventuallyYieldsMesh is the minimal end-to-end case: a
// single requested chunk, with no neighbours requested at all, still reaches
// Meshed and is handed back through Update's return value. It also exercises
// that an unrequested neighbour does not block progress: every one of this
// chunk's eight neighbours is unrequested, and the design treats that as
// trivially satisfied (see Pipeline's doc comment on neighborsAtLeastLocked).
func TestPipeline_RequestEventuallyYieldsMesh(t *testing.T) {
	gen := newStubGenerator()
	p, _ := newTestPipeline(t, gen, 2)
	defer p.Close()

	coord := world.ChunkCoord{X: 5, Z: -3}
	p.Request(coord)

	var got []chunks.Ready
	for i := 0; i < maxDriveIterations; i++ {
		got = append(got, p.Update(generousBudget)...)
		if p.Stage(coord) == chunks.Meshed {
			break
		}
	}

	if p.Stage(coord) != chunks.Meshed {
		t.Fatalf("Stage(%v) = %v after %d Update calls, want Meshed", coord, p.Stage(coord), maxDriveIterations)
	}

	found := false
	for _, r := range got {
		if r.Coord == coord {
			found = true
		}
	}
	if !found {
		t.Errorf("Update never returned a Ready for %v, though it reached Meshed", coord)
	}
}

// TestPipeline_Close_StopsEveryGoroutine builds and closes several pipelines
// and checks the goroutine count settles back down, allowing generously for
// the runtime's own background goroutines and scheduling noise.
func TestPipeline_Close_StopsEveryGoroutine(t *testing.T) {
	baseline := goroutineCountSettled()

	const workers = 6
	gen := newStubGenerator()
	p, _ := newTestPipeline(t, gen, workers)

	// Give the workers something to do before shutting down, so this
	// exercises Close under real activity rather than an idle pool.
	for _, c := range grid3x3() {
		p.Request(c)
	}
	drainUntil(t, p, generousBudget, func() bool { return atLeast(p, grid3x3(), chunks.Meshed) })

	p.Close()
	p.Close() // must be safe to call twice

	after := goroutineCountSettled()
	if after > baseline+2 {
		t.Errorf("goroutine count after Close = %d, baseline was %d (workers = %d): workers appear to have leaked", after, baseline, workers)
	}
}

// goroutineCountSettled samples runtime.NumGoroutine after giving the
// scheduler a chance to finish anything in flight, to keep the leak check
// above from being flaky about goroutines that are merely mid-exit.
func goroutineCountSettled() int {
	var last int
	for i := 0; i < 20; i++ {
		runtime.Gosched()
		last = runtime.NumGoroutine()
		time.Sleep(time.Millisecond)
	}
	return last
}

// TestPipeline_DuplicateRequestDoesNotDuplicateWork calls Request twice for
// the same coord before the pipeline has done anything, then confirms the
// generator only ever ran once for it.
func TestPipeline_DuplicateRequestDoesNotDuplicateWork(t *testing.T) {
	gen := newStubGenerator()
	p, _ := newTestPipeline(t, gen, 2)
	defer p.Close()

	coord := world.ChunkCoord{X: 0, Z: 0}
	p.Request(coord)
	p.Request(coord)
	p.Request(coord)

	drainUntil(t, p, generousBudget, func() bool { return p.Stage(coord) == chunks.Meshed })

	if got := gen.callCount(coord); got != 1 {
		t.Errorf("generator called %d times for a chunk requested 3 times, want 1", got)
	}
}

// TestPipeline_BudgetsAreHonoured checks both halves of Budget against a
// fully-requested 3x3 neighbourhood, where every chunk becomes eligible for
// lighting (and, later, meshing) in the same Update call — the scenario that
// would most tempt an unbudgeted implementation to do all nine at once.
func TestPipeline_BudgetsAreHonoured(t *testing.T) {
	gen := newStubGenerator()
	p, _ := newTestPipeline(t, gen, 4)
	defer p.Close()

	coords := grid3x3()
	for _, c := range coords {
		p.Request(c)
	}

	t.Run("Light", func(t *testing.T) {
		drainUntil(t, p, chunks.Budget{Light: 0, Mesh: 0}, func() bool { return atLeast(p, coords, chunks.Generated) })

		for i := 0; i < 9; i++ {
			before := countAtLeast(p, coords, chunks.Lit)
			p.Update(chunks.Budget{Light: 1, Mesh: 0})
			after := countAtLeast(p, coords, chunks.Lit)

			if got := after - before; got > 1 {
				t.Fatalf("call %d: Light budget 1 lit %d chunks in one call, want at most 1", i, got)
			}
		}

		if !atLeast(p, coords, chunks.Lit) {
			t.Fatalf("after 9 calls with Light budget 1, not every one of 9 chunks reached Lit (lit count = %d)", countAtLeast(p, coords, chunks.Lit))
		}
	})

	t.Run("Mesh", func(t *testing.T) {
		seen := make(map[world.ChunkCoord]bool)
		for i := 0; i < maxDriveIterations && len(seen) < len(coords); i++ {
			ready := p.Update(chunks.Budget{Light: 0, Mesh: 1})
			if len(ready) > 1 {
				t.Fatalf("call %d: Mesh budget 1 returned %d meshes in one call, want at most 1", i, len(ready))
			}
			for _, r := range ready {
				if seen[r.Coord] {
					t.Errorf("coord %v returned twice by Update", r.Coord)
				}
				seen[r.Coord] = true
			}
		}

		if len(seen) != len(coords) {
			t.Fatalf("collected %d distinct meshes across many budget-1 calls, want %d", len(seen), len(coords))
		}
	})
}

// countAtLeast returns how many of coords are currently at or beyond min.
func countAtLeast(p *chunks.Pipeline, coords []world.ChunkCoord, min chunks.Stage) int {
	n := 0
	for _, c := range coords {
		if p.Stage(c) >= min {
			n++
		}
	}
	return n
}

// TestPipeline_StuckNeighbourDoesNotBlockOthers requests a chunk whose
// generation is deliberately held back, alongside an unrelated chunk far
// away. The unrelated chunk must still reach Meshed while the stuck one is
// held, proving the pipeline does not serialise unrelated work behind one
// chunk that never arrives. The held generator is released before the test
// ends so Close (via defer) does not itself hang waiting on that worker.
func TestPipeline_StuckNeighbourDoesNotBlockOthers(t *testing.T) {
	gen := newStubGenerator()
	p, _ := newTestPipeline(t, gen, 4)
	defer p.Close()

	stuck := world.ChunkCoord{X: 0, Z: 0}
	release := gen.holdBack(stuck)
	defer release() // safety net if an assertion fails before the explicit release below

	dependent := world.ChunkCoord{X: 1, Z: 0} // has stuck as a requested neighbour
	independent := world.ChunkCoord{X: 100, Z: 100}

	p.Request(stuck)
	p.Request(dependent)
	p.Request(independent)

	drainUntil(t, p, generousBudget, func() bool { return p.Stage(independent) == chunks.Meshed })

	if stage := p.Stage(stuck); stage >= chunks.Generated {
		t.Errorf("Stage(stuck) = %v, want it still short of Generated while its generator is held back", stage)
	}
	if stage := p.Stage(dependent); stage >= chunks.Lit {
		t.Errorf("Stage(dependent) = %v, want it blocked below Lit by its stuck neighbour", stage)
	}

	release()
	drainUntil(t, p, generousBudget, func() bool { return p.Stage(dependent) == chunks.Meshed })
}
