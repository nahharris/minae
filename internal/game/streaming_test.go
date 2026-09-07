package game

import (
	"runtime"
	"testing"
	"time"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// fakeRenderer is a minimal meshRemover double that counts what it holds,
// standing in for *render.SceneRenderer so the unload path can be tested
// without a live raylib window -- constructing a real SceneRenderer needs a
// GPU context this test binary does not have.
type fakeRenderer struct {
	meshes map[world.ChunkCoord]struct{}
}

func newFakeRenderer() *fakeRenderer {
	return &fakeRenderer{meshes: make(map[world.ChunkCoord]struct{})}
}

func (r *fakeRenderer) RemoveMesh(coord world.ChunkCoord) {
	delete(r.meshes, coord)
}

// compile-time check that *fakeRenderer satisfies meshRemover.
var _ meshRemover = (*fakeRenderer)(nil)

// TestReleaseUnloaded_FreesExactlyTheReleasedMeshes covers validation
// criterion 3: unloading releases GPU meshes. This counts what the fake
// renderer holds before and after, rather than merely checking that
// RemoveMesh was invoked, per the M15 instruction to verify by count.
func TestReleaseUnloaded_FreesExactlyTheReleasedMeshes(t *testing.T) {
	t.Parallel()

	renderer := newFakeRenderer()
	kept := world.ChunkCoord{X: 5, Z: 5}
	renderer.meshes[world.ChunkCoord{X: 0, Z: 0}] = struct{}{}
	renderer.meshes[world.ChunkCoord{X: 1, Z: 0}] = struct{}{}
	renderer.meshes[kept] = struct{}{}

	releaseUnloaded(renderer, []world.ChunkCoord{{X: 0, Z: 0}, {X: 1, Z: 0}})

	if got, want := len(renderer.meshes), 1; got != want {
		t.Fatalf("renderer holds %d meshes after releasing 2 of 3, want %d", got, want)
	}
	if _, ok := renderer.meshes[kept]; !ok {
		t.Fatal("releaseUnloaded removed a mesh it was not told to release")
	}
}

// TestReleaseUnloaded_NoReleasesIsANoOp guards against a vacuous pass: an
// empty released list must leave the renderer untouched.
func TestReleaseUnloaded_NoReleasesIsANoOp(t *testing.T) {
	t.Parallel()

	renderer := newFakeRenderer()
	renderer.meshes[world.ChunkCoord{X: 0, Z: 0}] = struct{}{}

	releaseUnloaded(renderer, nil)

	if len(renderer.meshes) != 1 {
		t.Fatalf("renderer holds %d meshes after releasing nothing, want 1", len(renderer.meshes))
	}
}

// allAtLeastMeshed reports whether every coord within radius chunks
// (Chebyshev distance) of center has reached at least Meshed.
func allAtLeastMeshed(p *chunks.Pipeline, center world.ChunkCoord, radius int) bool {
	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			if p.Stage(world.ChunkCoord{X: center.X + dx, Z: center.Z + dz}) < chunks.Meshed {
				return false
			}
		}
	}
	return true
}

// TestStreaming_MemoryStaysAtSteadyStateOverALongWalk covers validation
// criterion 8's mesh-count half: walking a long straight path through a real
// Pipeline and Streamer, driving the exact upload/remove sequence Game.Update
// performs each frame against a fake renderer, must not leave the renderer
// holding more and more meshes over time. internal/chunks's own streamer
// test covers the loaded-chunk-count half with a fake pipeline; this one
// exercises the actual GPU-release wiring this package owns.
func TestStreaming_MemoryStaysAtSteadyStateOverALongWalk(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	// A small fixed worker count, not runtime.NumCPU(): this suite runs
	// alongside every other package's tests under `go test ./...`, and a
	// pipeline sized to the whole machine here competes for CPU with
	// unrelated packages instead of just this one.
	pipeline := chunks.NewPipeline(w, light, chunks.FlatGenerator{}, nil, 4)
	defer pipeline.Close()

	const loadRadius = 1
	const unloadRadius = loadRadius + 2
	streamer := chunks.NewStreamer(pipeline, loadRadius, unloadRadius)
	renderer := newFakeRenderer()

	budget := chunks.Budget{Light: time.Second, Mesh: time.Second}

	step := func(center world.ChunkCoord) {
		t.Helper()

		released := streamer.Update(center)
		releaseUnloaded(renderer, released)

		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			for _, r := range pipeline.Update(budget) {
				if r.Data == nil {
					renderer.RemoveMesh(r.Coord)
				} else {
					renderer.meshes[r.Coord] = struct{}{}
				}
			}
			if allAtLeastMeshed(pipeline, center, loadRadius) {
				return
			}
			runtime.Gosched()
		}
		t.Fatalf("pipeline did not settle around %+v within the deadline", center)
	}

	const steps = 20
	meshCounts := make([]int, 0, steps)
	for i := 0; i < steps; i++ {
		step(world.ChunkCoord{X: i, Z: 0})
		meshCounts = append(meshCounts, len(renderer.meshes))
	}

	// Skip the initial transient while the trail is still catching up to the
	// unload radius. From there on, walking a straight line is
	// translation-invariant, so the mesh count must be identical at every
	// remaining step -- a leak would show up as it climbing instead.
	const settleAfter = unloadRadius + 1
	want := meshCounts[settleAfter]
	for i := settleAfter; i < len(meshCounts); i++ {
		if meshCounts[i] != want {
			t.Fatalf("step %d: renderer holds %d meshes, want steady %d (all counts: %v)",
				i, meshCounts[i], want, meshCounts)
		}
	}
}
