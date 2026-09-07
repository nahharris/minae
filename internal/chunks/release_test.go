package chunks_test

import (
	"runtime"
	"testing"
	"time"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// blockingGenerator wraps another Generator and blocks the calling worker
// goroutine after signalling started, until told to proceed. It exists to
// pin down the exact "in flight" window a release-while-generating test
// needs: without it, FlatGenerator finishes so fast that a test could never
// reliably observe the coord mid-flight rather than already Generated.
type blockingGenerator struct {
	inner   chunks.Generator
	started chan world.ChunkCoord
	proceed chan struct{}
}

func (g *blockingGenerator) Generate(coord world.ChunkCoord) *world.Chunk {
	g.started <- coord
	<-g.proceed
	return g.inner.Generate(coord)
}

// TestPipeline_ReleaseDiscardsInFlightGeneration covers validation criterion
// 4 on the generation half of the pipeline, and is the direct test for the
// drainGenerated wanted-check the M15 design decisions call for: a coord
// released while a generation worker is still producing it must not be
// resurrected when that worker finally reports back.
func TestPipeline_ReleaseDiscardsInFlightGeneration(t *testing.T) {
	blocks.ResetToVanilla()

	gen := &blockingGenerator{
		inner:   chunks.FlatGenerator{},
		started: make(chan world.ChunkCoord, 1),
		proceed: make(chan struct{}),
	}

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	p := chunks.NewPipeline(w, light, gen, nil, 1)
	defer p.Close()

	target := world.ChunkCoord{X: 3, Z: -2}
	p.Request(target)

	// Update must only ever be called from one goroutine (see its own doc
	// comment), so synchronizing with the worker is done by polling
	// gen.started between Update calls in this same goroutine, rather than
	// blocking on it from a second one.
	started := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && !started {
		p.Update(chunks.Budget{Light: 8, Mesh: 8})
		select {
		case got := <-gen.started:
			if got != target {
				t.Fatalf("worker started generating %+v, want %+v", got, target)
			}
			started = true
		default:
			runtime.Gosched()
		}
	}
	if !started {
		t.Fatal("generation worker never started; the pipeline is stuck")
	}

	// Release while the job is still blocked inside Generate: exactly the
	// race window Release's wanted-check exists to close.
	p.Release(target)

	if p.Stage(target) != chunks.Absent {
		t.Fatalf("Stage(%+v) = %v immediately after Release, want Absent", target, p.Stage(target))
	}

	// Let the worker finish and report its result back.
	close(gen.proceed)

	// Drain for a short while: the worker's result reaches genResults and is
	// processed by drainGenerated in microseconds once unblocked, so this is
	// a generous margin rather than something the test should ever actually
	// wait out. If the wanted-check were missing, this result would
	// resurrect the coord into World.Chunks and Generated.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		p.Update(chunks.Budget{Light: 8, Mesh: 8})
		runtime.Gosched()
	}

	if _, exists := w.Chunks[target]; exists {
		t.Fatalf("World.Chunks contains %+v after Release discarded its in-flight generation result", target)
	}
	if p.Stage(target) != chunks.Absent {
		t.Fatalf("Stage(%+v) = %v after settling, want Absent (the late result must not resurrect it)", target, p.Stage(target))
	}

	// A fresh Request for the same coord afterwards must still work
	// correctly — Release must not have left the pipeline's bookkeeping in a
	// state that poisons a later request for the same coord.
	p.Request(target)
	deadline = time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		p.Update(chunks.Budget{Light: 8, Mesh: 8})
		if p.Stage(target) >= chunks.Generated {
			return
		}
		runtime.Gosched()
	}
	t.Fatalf("re-requesting %+v after Release never reached Generated", target)
}

// TestPipeline_ReleaseDiscardsInFlightMesh covers validation criterion 4 on
// the meshing half: a coord released while its mesh job is in flight must
// never surface a Ready for it, and must end up fully gone from the
// pipeline's bookkeeping and the world.
func TestPipeline_ReleaseDiscardsInFlightMesh(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	// A single worker serializes generation and meshing, which widens the
	// window a freshly dispatched mesh job spends at stage Meshing before
	// drainMeshed can collect its result — see the worker pool's doc comment
	// on why one pool adapts to whichever phase is the bottleneck.
	p := chunks.NewPipeline(w, light, chunks.FlatGenerator{}, nil, 1)
	defer p.Close()

	center := world.ChunkCoord{X: 0, Z: 0}
	var want []world.ChunkCoord
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			want = append(want, world.ChunkCoord{X: dx, Z: dz})
		}
	}
	for _, c := range want {
		p.Request(c)
	}

	budget := chunks.Budget{Light: 64, Mesh: 64}
	sawMeshing := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		p.Update(budget)
		if p.Stage(center) == chunks.Meshing {
			sawMeshing = true
			p.Release(center)
			break
		}
		runtime.Gosched()
	}
	if !sawMeshing {
		t.Fatal("never observed the centre chunk reach Meshing before completing; cannot exercise the in-flight window")
	}

	settle := time.Now().Add(2 * time.Second)
	for time.Now().Before(settle) {
		for _, r := range p.Update(budget) {
			if r.Coord == center {
				t.Fatalf("a Ready was produced for %+v after it was released mid-mesh", center)
			}
		}
		runtime.Gosched()
	}

	if p.Stage(center) != chunks.Absent {
		t.Errorf("Stage(%+v) = %v after settling, want Absent", center, p.Stage(center))
	}
	if _, exists := w.Chunks[center]; exists {
		t.Errorf("World.Chunks still contains %+v after it was released mid-mesh", center)
	}
}
