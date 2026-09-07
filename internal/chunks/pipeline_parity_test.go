package chunks_test

import (
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// The three tests here are the ones that decide whether the pipeline is
// trustworthy: it must produce the same world the synchronous path did, it
// must not depend on the order workers happen to finish in, and it must be
// free of races while genuinely under concurrent load.

const parityRegion = 1 // chunks either side of the origin: a 3x3 region

func regionCoords() []world.ChunkCoord {
	var out []world.ChunkCoord
	for x := -parityRegion; x <= parityRegion; x++ {
		for z := -parityRegion; z <= parityRegion; z++ {
			out = append(out, world.ChunkCoord{X: x, Z: z})
		}
	}
	return out
}

// worldState is everything about a finished world that can be compared.
type worldState struct {
	blocks map[world.ChunkCoord][]blocks.NumID
	sky    map[world.ChunkCoord][]uint8
	block  map[world.ChunkCoord][]uint8
}

func captureWorld(w *world.World) worldState {
	s := worldState{
		blocks: map[world.ChunkCoord][]blocks.NumID{},
		sky:    map[world.ChunkCoord][]uint8{},
		block:  map[world.ChunkCoord][]uint8{},
	}
	for coord, c := range w.Chunks {
		s.blocks[coord] = append([]blocks.NumID(nil), c.Blocks[:]...)
		s.sky[coord] = append([]uint8(nil), c.SkyLight[:]...)
		s.block[coord] = append([]uint8(nil), c.BlockLight[:]...)
	}
	return s
}

// buildSynchronously produces the region the way the game did before the
// pipeline existed: generate everything, light it all at once, then mesh.
func buildSynchronously(t *testing.T) (*world.World, map[world.ChunkCoord]*mesh.ChunkMeshData) {
	t.Helper()
	blocks.ResetToVanilla()

	w := world.NewWorld()
	gen := chunks.FlatGenerator{}
	for _, coord := range regionCoords() {
		w.Chunks[coord] = gen.Generate(coord)
	}

	lighting.NewEngine(w).RecomputeAll()

	meshes := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	for _, coord := range regionCoords() {
		meshes[coord] = mesh.GenerateChunkMeshData(w.GetChunk(coord.X, coord.Z), w, nil)
	}
	return w, meshes
}

// buildThroughPipeline drives the pipeline until every requested chunk has
// produced a mesh, or fails if it stalls.
func buildThroughPipeline(t *testing.T, workers int, want []world.ChunkCoord) (*world.World, map[world.ChunkCoord]*mesh.ChunkMeshData) {
	t.Helper()
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	p := chunks.NewPipeline(w, light, chunks.FlatGenerator{}, nil, workers)
	defer p.Close()

	for _, coord := range want {
		p.Request(coord)
	}

	meshes := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	budget := chunks.Budget{Light: 64, Mesh: 64}

	// Bounded by wall time, not iterations. Update is non-blocking by design,
	// so a tight loop spins through thousands of calls in a few milliseconds --
	// far less time than the workers need to generate and mesh real chunks.
	// Counting iterations would report a stall that is only impatience.
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		for _, r := range p.Update(budget) {
			meshes[r.Coord] = r.Data
		}
		if len(meshes) == len(want) {
			return w, meshes
		}
		runtime.Gosched()
	}
	t.Fatalf("pipeline did not finish within the deadline: produced %d of %d meshes", len(meshes), len(want))
	return nil, nil
}

func compareWorlds(t *testing.T, label string, got, want worldState) {
	t.Helper()

	if len(got.blocks) != len(want.blocks) {
		t.Fatalf("%s: %d chunks, want %d", label, len(got.blocks), len(want.blocks))
	}
	for coord := range want.blocks {
		if !reflect.DeepEqual(got.blocks[coord], want.blocks[coord]) {
			t.Errorf("%s: chunk %+v blocks differ", label, coord)
		}
		if !reflect.DeepEqual(got.sky[coord], want.sky[coord]) {
			t.Errorf("%s: chunk %+v skylight differs", label, coord)
		}
		if !reflect.DeepEqual(got.block[coord], want.block[coord]) {
			t.Errorf("%s: chunk %+v block light differs", label, coord)
		}
	}
}

// The pipeline changed *when* chunks are produced, not *what* is produced.
//
// Blocks, both light channels and the mesh geometry must all match what the
// old single-threaded path produced. Light is the interesting half: the
// synchronous path lights everything at once with RecomputeAll, while the
// pipeline seeds chunk by chunk as they arrive, so agreement here is also a
// statement that per-chunk seeding converges to the same answer under a real
// arrival order rather than a contrived one.
func TestPipeline_MatchesSynchronousOutput(t *testing.T) {
	syncWorld, syncMeshes := buildSynchronously(t)
	asyncWorld, asyncMeshes := buildThroughPipeline(t, 4, regionCoords())

	compareWorlds(t, "world", captureWorld(asyncWorld), captureWorld(syncWorld))

	for _, coord := range regionCoords() {
		want, got := syncMeshes[coord], asyncMeshes[coord]
		if (want == nil) != (got == nil) {
			t.Errorf("chunk %+v: mesh presence differs (sync nil=%v, async nil=%v)",
				coord, want == nil, got == nil)
			continue
		}
		if want == nil {
			continue
		}
		if !reflect.DeepEqual(want.Vertices, got.Vertices) {
			t.Errorf("chunk %+v: vertices differ (%d sync, %d async)",
				coord, len(want.Vertices), len(got.Vertices))
		}
		if !reflect.DeepEqual(want.Colors, got.Colors) {
			t.Errorf("chunk %+v: colours differ — light or ambient occlusion diverged", coord)
		}
		if !reflect.DeepEqual(want.Normals, got.Normals) || !reflect.DeepEqual(want.Texcoords, got.Texcoords) {
			t.Errorf("chunk %+v: normals or texcoords differ", coord)
		}
	}
}

// Workers finish in whatever order the scheduler chooses, and the finished
// world must not depend on it.
//
// Varying the worker count is the sharper half: one worker completes jobs in
// dispatch order, eight complete them in an order that changes run to run. A
// result that depends on completion order would reproduce roughly never, and
// then only on someone else's machine.
func TestPipeline_DeterministicRegardlessOfCompletionOrder(t *testing.T) {
	// A smaller region than parity uses, because what this test needs is
	// repeated builds rather than large ones: a scheduler-dependent result shows
	// up across runs, not within one. Four chunks still gives every chunk loaded
	// neighbours on two sides.
	//
	// Serial and highly-parallel worker counts are the two that matter. One
	// worker completes jobs in dispatch order; eight complete them in an order
	// that differs run to run, which is the condition being tested.
	small := []world.ChunkCoord{{X: 0, Z: 0}, {X: 1, Z: 0}, {X: 0, Z: 1}, {X: 1, Z: 1}}

	reference, _ := buildThroughPipeline(t, 1, small)
	want := captureWorld(reference)

	for _, workers := range []int{1, 8} {
		t.Run("", func(t *testing.T) {
			for run := range 1 {
				got, _ := buildThroughPipeline(t, workers, small)
				compareWorlds(t, "run", captureWorld(got), want)
				if t.Failed() {
					t.Fatalf("diverged with %d workers on run %d", workers, run)
				}
			}
		})
	}
}

// The pipeline under genuine concurrent load, with the world being edited
// while workers are mid-flight.
//
// The race detector only reports races it actually observes, so a test that
// merely enables the flag proves nothing. This one keeps workers busy while
// the owning goroutine mutates blocks, relights and invalidates, which is the
// interleaving the ownership rules exist to make safe.
func TestPipeline_RaceUnderConcurrentEditing(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	p := chunks.NewPipeline(w, light, chunks.FlatGenerator{}, nil, 6)
	defer p.Close()

	var requested []world.ChunkCoord
	for x := 0; x <= 1; x++ {
		for z := 0; z <= 1; z++ {
			coord := world.ChunkCoord{X: x, Z: z}
			requested = append(requested, coord)
			p.Request(coord)
		}
	}

	// A burst of edits while work is in flight, then quiet. Editing forever
	// would be pathological rather than realistic: every Invalidate demotes a
	// chunk, so a fast enough edit stream keeps the pipeline permanently
	// behind and nothing would ever reach Meshed. What matters is that the
	// overlap is safe, and that the pipeline converges once the burst stops.
	budget := chunks.Budget{Light: 2, Mesh: 2}
	edits := 0

	burst := time.Now().Add(1500 * time.Millisecond)
	for frame := 0; time.Now().Before(burst); frame++ {
		p.Update(budget)

		if frame%40 == 0 {
			x, z := frame%24, (frame*3)%24
			if w.HasChunkAt(x, z) {
				w.SetBlock(x, 40, z, blocks.Stone)
				light.OnBlockChanged(x, 40, z)
				p.Invalidate(world.ChunkCoord{X: x >> 4, Z: z >> 4})
				edits++
			}
		}
		runtime.Gosched()
	}

	if edits == 0 {
		t.Fatal("no edits were applied; the test did not exercise concurrent mutation")
	}

	// With editing stopped, everything must settle.
	settle := time.Now().Add(30 * time.Second)
	for time.Now().Before(settle) {
		p.Update(budget)
		if allMeshedAtLeastOnce(p, requested) {
			return
		}
		runtime.Gosched()
	}
	t.Error("the pipeline never settled after editing stopped; chunks are stuck below Meshed")
}

func allMeshedAtLeastOnce(p *chunks.Pipeline, coords []world.ChunkCoord) bool {
	for _, c := range coords {
		if p.Stage(c) < chunks.Meshed {
			return false
		}
	}
	return true
}

// Guard against the parity helpers silently testing nothing.
func TestParityHelpersProduceWork(t *testing.T) {
	_, meshes := buildSynchronously(t)
	if len(meshes) != len(regionCoords()) {
		t.Fatalf("synchronous build produced %d meshes, want %d", len(meshes), len(regionCoords()))
	}
	for coord, m := range meshes {
		if m == nil || len(m.Vertices) == 0 {
			t.Fatalf("chunk %+v produced an empty mesh; the comparison would be vacuous", coord)
		}
	}
}

// pitGenerator makes terrain that is *not* uniform: alternate chunks hold a
// deep shaft, so skylight varies sharply across chunk seams.
//
// Flat terrain cannot distinguish a pipeline that re-meshes after light
// changes from one that does not, because every column is lit identically and
// a stale mesh looks the same as a fresh one. Light has to differ across a
// seam for the difference to be observable at all.
type pitGenerator struct{}

func (pitGenerator) Generate(coord world.ChunkCoord) *world.Chunk {
	c := chunks.FlatGenerator{}.Generate(coord)

	// A shaft in every other chunk, at its edge, so the light it admits spills
	// across the seam into the neighbour.
	if (coord.X+coord.Z)%2 != 0 {
		return c
	}
	for y := 0; y <= 32; y++ {
		for lx := 0; lx <= 2; lx++ {
			for lz := 0; lz <= 2; lz++ {
				c.SetBlock(lx, y, lz, nil)
			}
		}
	}
	return c
}

// The parity check again, over terrain whose light genuinely varies across
// seams.
//
// This is the version that can observe whether the pipeline re-meshes a chunk
// after a neighbour's arrival changes the light it samples. With flat terrain
// the question is unanswerable: every column is lit the same, so a stale mesh
// is indistinguishable from a fresh one.
func TestPipeline_MatchesSynchronousOutputOnVariedTerrain(t *testing.T) {
	coords := regionCoords()

	// Synchronous reference.
	blocks.ResetToVanilla()
	syncWorld := world.NewWorld()
	for _, coord := range coords {
		syncWorld.Chunks[coord] = pitGenerator{}.Generate(coord)
	}
	lighting.NewEngine(syncWorld).RecomputeAll()

	syncMeshes := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	for _, coord := range coords {
		syncMeshes[coord] = mesh.GenerateChunkMeshData(syncWorld.GetChunk(coord.X, coord.Z), syncWorld, nil)
	}

	// Through the pipeline.
	blocks.ResetToVanilla()
	asyncWorld := world.NewWorld()
	light := lighting.NewEngine(asyncWorld)
	p := chunks.NewPipeline(asyncWorld, light, pitGenerator{}, nil, 4)
	defer p.Close()

	for _, coord := range coords {
		p.Request(coord)
	}

	asyncMeshes := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) && len(asyncMeshes) < len(coords) {
		for _, r := range p.Update(chunks.Budget{Light: 64, Mesh: 64}) {
			asyncMeshes[r.Coord] = r.Data
		}
		runtime.Gosched()
	}
	if len(asyncMeshes) != len(coords) {
		t.Fatalf("pipeline produced %d of %d meshes", len(asyncMeshes), len(coords))
	}

	compareWorlds(t, "varied terrain", captureWorld(asyncWorld), captureWorld(syncWorld))

	for _, coord := range coords {
		want, got := syncMeshes[coord], asyncMeshes[coord]
		if want == nil || got == nil {
			continue
		}
		if !reflect.DeepEqual(want.Colors, got.Colors) {
			t.Errorf("chunk %+v: vertex colours differ — a mesh was not rebuilt after the light it samples changed", coord)
		}
		if !reflect.DeepEqual(want.Vertices, got.Vertices) {
			t.Errorf("chunk %+v: vertices differ", coord)
		}
	}
}

// mixedGenerator makes chunk (0,0) hollow and everything else solid rock with
// no air at all.
//
// A fully solid neighbour is the sharp case: arriving, it changes nothing
// about its neighbour's light — every cell involved is already 0 — while
// changing its geometry, because the seam faces that were drawn against
// absent space must now be culled.
type mixedGenerator struct{}

func (mixedGenerator) Generate(coord world.ChunkCoord) *world.Chunk {
	c := world.NewChunk(coord.X, coord.Z)

	// Solid to the very top, so there is no open sky anywhere. That matters:
	// a chunk with sky above it writes skylight when it is seeded, and those
	// writes dirty the seam and would trigger a re-mesh for the wrong reason.
	// Filling the full height means seeding this chunk writes nothing at all.
	for lx := range 16 {
		for lz := range 16 {
			for y := range config.ChunkHeight {
				c.SetBlock(lx, y, lz, blocks.Stone)
			}
		}
	}
	if coord == (world.ChunkCoord{X: 0, Z: 0}) {
		// A hollow room, so this chunk has faces to draw at its seams.
		for lx := 1; lx < 15; lx++ {
			for lz := 1; lz < 15; lz++ {
				for y := 20; y < 30; y++ {
					c.SetBlock(lx, y, lz, nil)
				}
			}
		}
		// Open one wall right up to the seam.
		for lz := 1; lz < 15; lz++ {
			for y := 20; y < 30; y++ {
				c.SetBlock(15, y, lz, nil)
			}
		}
	}
	return c
}

// A meshed chunk must be re-meshed when a neighbour arrives, even if that
// neighbour changes none of its light.
//
// The parity tests cannot see this: their stage rule means every neighbour is
// present before anything is meshed. It only arises when chunks arrive one at
// a time, which is what streaming does — and it is not covered by reacting to
// light changes, because a solid neighbour arriving writes no light at all.
// What changes is face culling: the seam faces drawn against absent space are
// now hidden.
func TestPipeline_LateSolidNeighbourForcesReMesh(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	p := chunks.NewPipeline(w, light, mixedGenerator{}, nil, 2)
	defer p.Close()

	first := world.ChunkCoord{X: 0, Z: 0}
	late := world.ChunkCoord{X: 1, Z: 0}

	// Collect every Ready as it is produced. Discarding them and looking only
	// at stages would miss a re-mesh that has already been delivered.
	latest := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	drive := func(until func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			for _, r := range p.Update(chunks.Budget{Light: 64, Mesh: 64}) {
				latest[r.Coord] = r.Data
			}
			if until() {
				return
			}
			runtime.Gosched()
		}
		t.Fatal("pipeline did not reach the expected state in time")
	}

	p.Request(first)
	drive(func() bool { return latest[first] != nil })

	before := latest[first]
	if before == nil || len(before.Vertices) == 0 {
		t.Fatal("the first chunk produced no geometry; the test would prove nothing")
	}

	p.Request(late)
	drive(func() bool { return latest[late] != nil && latest[first] != before })

	after := latest[first]
	if after == before {
		t.Fatal("the first chunk was never re-meshed after a solid neighbour arrived; " +
			"its seam faces are still drawn against space that is now rock")
	}

	// And the rebuilt mesh must match a fresh build of the finished world.
	fresh := mesh.GenerateChunkMeshData(w.GetChunk(first.X, first.Z), w, nil)
	if !reflect.DeepEqual(fresh.Vertices, after.Vertices) {
		t.Errorf("re-meshed geometry does not match a fresh build: %d vertices vs %d",
			len(after.Vertices), len(fresh.Vertices))
	}
	if len(after.Vertices) >= len(before.Vertices) {
		t.Errorf("expected fewer vertices after the neighbour arrived and culled the seam: "+
			"%d before, %d after", len(before.Vertices), len(after.Vertices))
	}
}
