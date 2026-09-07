package chunks

import (
	"sync"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// Stage is a chunk's position in the pipeline.
//
// A chunk only ever moves forward, with one exception: DirtyChunks can push a
// Meshed or Meshing chunk back to Lit so it is re-meshed with corrected light
// (see Update). It never regresses below Lit, because generation and lighting
// are never redone for a chunk that has reached them once.
type Stage int

const (
	// Absent is the zero value: a chunk that has not been requested, or one
	// requested but not yet picked up by a generation worker.
	Absent Stage = iota
	// Generating means a worker is currently producing the chunk's blocks.
	Generating
	// Generated means the chunk's blocks exist in World.Chunks.
	Generated
	// Lit means the chunk has been seeded with skylight and block light.
	Lit
	// Meshing means a worker is currently building the chunk's mesh from a
	// Snapshot.
	Meshing
	// Meshed means the chunk's mesh data is ready and has been returned to
	// the caller through Update.
	Meshed
)

// Generator produces a chunk's blocks. It must be a pure function of coord:
// no world access, no shared state, safe to call from many goroutines at
// once.
type Generator interface {
	// Generate returns a newly built chunk for coord.
	Generate(coord world.ChunkCoord) *world.Chunk
}

// FlatGenerator produces the same flat stone/dirt/grass terrain that
// world.GenerateFixedGrid built directly, up to y=32.
//
// It lives here rather than in internal/world because internal/chunks already
// imports internal/world for World and Chunk; the reverse import would be a
// cycle. M17 replaces FlatGenerator with real terrain generation.
type FlatGenerator struct{}

// Generate implements Generator with a flat surface at y=32: stone below,
// two layers of dirt, and grass on top.
func (FlatGenerator) Generate(coord world.ChunkCoord) *world.Chunk {
	c := world.NewChunk(coord.X, coord.Z)

	const height = 32
	for x := range config.ChunkWidth {
		for z := range config.ChunkWidth {
			for y := 0; y < height; y++ {
				switch {
				case y < height-3:
					c.SetBlock(x, y, z, blocks.Stone)
				case y < height-1:
					c.SetBlock(x, y, z, blocks.Dirt)
				default:
					c.SetBlock(x, y, z, blocks.Grass)
				}
			}
		}
	}
	return c
}

// Budget caps the work Update performs in a single call, so one frame can
// never stall waiting on the pipeline. A zero Budget makes Update do no
// lighting and return no meshes that call — generation and meshing already in
// flight keep running regardless, and their results are picked up once a
// non-zero budget is passed.
type Budget struct {
	// Light is the maximum number of chunks Update may seed with light this
	// call.
	Light int
	// Mesh is the maximum number of finished meshes Update may return this
	// call.
	Mesh int
}

// Ready is a chunk mesh the pipeline has finished building, waiting to be
// uploaded to the GPU by the caller.
type Ready struct {
	// Coord is the chunk the mesh belongs to.
	Coord world.ChunkCoord
	// Data is the mesh's CPU-side data. It may be nil for a chunk with no
	// visible faces (e.g. one buried on every side); Upload already treats a
	// nil receiver as a no-op, so callers can pass it straight through.
	Data *mesh.ChunkMeshData
}

// genResult is what a generation worker sends back for one coord.
type genResult struct {
	coord world.ChunkCoord
	chunk *world.Chunk
}

// meshJob is what the main thread sends a mesh worker: a coord, the epoch it
// was dispatched at (see Pipeline.epoch), and the snapshot to mesh from.
type meshJob struct {
	coord    world.ChunkCoord
	epoch    int
	snapshot *Snapshot
}

// meshResult is what a mesh worker sends back for one job.
type meshResult struct {
	coord    world.ChunkCoord
	epoch    int
	snapshot *Snapshot
	data     *mesh.ChunkMeshData
}

// queueFactor sets how many jobs each worker may have buffered ahead of it in
// a queue or results channel. It only affects how much work can be in flight
// without a worker or Update blocking; it has no bearing on correctness.
const queueFactor = 4

// Pipeline produces chunks off the main thread, advancing each one through
// Absent → Generating → Generated → Lit → Meshing → Meshed. A chunk advances
// past Generated or Lit only once every one of its eight neighbours that has
// been requested has itself reached that stage — neighbours never requested
// are not part of this world in this milestone (dynamic loading is M15) and
// so cannot block it. Create one with NewPipeline.
type Pipeline struct {
	w     *world.World
	light *lighting.Engine
	gen   Generator
	uv    mesh.UVLookup

	genQueue    chan world.ChunkCoord
	genResults  chan genResult
	meshQueue   chan meshJob
	meshResults chan meshResult

	stop chan struct{}
	wg   sync.WaitGroup

	// mu guards every field below it. It is needed because Request may be
	// called from a different goroutine than Update; nothing else in Pipeline
	// touches these fields, since worker goroutines only ever see the channels
	// above.
	mu        sync.Mutex
	stages    map[world.ChunkCoord]Stage
	requested []world.ChunkCoord
	epoch     map[world.ChunkCoord]int

	// nextEpoch is a single monotonically increasing counter that every
	// epoch value is drawn from, so no two mesh jobs ever share one — not
	// even jobs for the same coord separated by a Release.
	//
	// Per-coord counters would be enough if a coord's history were
	// unbroken, but Release deletes its epoch entry, so a re-requested coord
	// would start counting from zero again and its first job would collide
	// with a job still in flight from the coord's previous life. That job
	// was meshed against a completely different neighbourhood — the player
	// walked away and came back — and drainMeshed would see a matching
	// epoch and accept it. Drawing from one counter makes a reused epoch
	// impossible by construction rather than by argument.
	nextEpoch int

	closeOnce sync.Once
}

// NewPipeline returns a pipeline that generates chunks with gen, lights them
// with light, and meshes them against w, using workers goroutines to run
// generation and meshing jobs. uv resolves texture keys to atlas coordinates
// for meshing; it is read-only after construction (internal/gfx/atlas.Atlas
// satisfies it), so sharing it across mesh workers is safe.
//
// workers below 1 is treated as 1: a pipeline with no workers could never
// finish anything it started.
func NewPipeline(w *world.World, light *lighting.Engine, gen Generator, uv mesh.UVLookup, workers int) *Pipeline {
	if workers < 1 {
		workers = 1
	}

	queueSize := workers * queueFactor
	p := &Pipeline{
		w:           w,
		light:       light,
		gen:         gen,
		uv:          uv,
		genQueue:    make(chan world.ChunkCoord, queueSize),
		genResults:  make(chan genResult, queueSize),
		meshQueue:   make(chan meshJob, queueSize),
		meshResults: make(chan meshResult, queueSize),
		stop:        make(chan struct{}),
		stages:      make(map[world.ChunkCoord]Stage),
		epoch:       make(map[world.ChunkCoord]int),
	}

	p.wg.Add(workers)
	for range workers {
		go p.worker()
	}

	return p
}

// worker runs generation and meshing jobs until Close stops it. One pool
// handles both job kinds because the two are never both busy in the same
// proportions: generation dominates while chunks first arrive, meshing
// dominates once neighbourhoods complete, and a single pool adapts to
// whichever is currently the bottleneck instead of leaving half of two fixed
// pools idle.
func (p *Pipeline) worker() {
	defer p.wg.Done()

	for {
		select {
		case <-p.stop:
			return

		case coord := <-p.genQueue:
			chunk := p.gen.Generate(coord)
			select {
			case p.genResults <- genResult{coord: coord, chunk: chunk}:
			case <-p.stop:
				return
			}

		case job := <-p.meshQueue:
			data := mesh.GenerateChunkMeshData(job.snapshot.Center(), job.snapshot, p.uv)
			select {
			case p.meshResults <- meshResult{coord: job.coord, epoch: job.epoch, snapshot: job.snapshot, data: data}:
			case <-p.stop:
				return
			}
		}
	}
}

// Request marks coord as wanted. It is safe to call repeatedly for the same
// coord: only the first call has any effect, so a caller never has to track
// what it has already asked for.
func (p *Pipeline) Request(coord world.ChunkCoord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, known := p.stages[coord]; known {
		return
	}
	p.stages[coord] = Absent
	p.requested = append(p.requested, coord)
}

// Stage reports coord's current position in the pipeline. A coord that was
// never requested reports Absent, the same as one that was requested but has
// not yet been picked up by a worker.
func (p *Pipeline) Stage(coord world.ChunkCoord) Stage {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stages[coord]
}

// Release undoes everything Request set up for coord: the pipeline no longer
// wants it, and every trace of it is removed so nothing can resurrect it
// later. This is the streaming counterpart to Request, and it is the
// direction M15 singles out as the one nobody tests — a leak here is silent
// and only shows up as memory climbing over a long session.
//
// Release must, in full:
//
//   - delete coord from stages, which is what discards a mesh job already in
//     flight: drainMeshed treats a result whose coord is no longer Meshing as
//     stale, and a released coord reads back as Absent,
//   - delete coord from epoch, so a later Request for the same coord starts
//     completely fresh with no memory of this one. That is safe only because
//     epochs come from a monotonic counter — see Pipeline.nextEpoch. Were
//     they per-coord counters, deleting here would reset the coord to zero
//     and its next job would collide with one still in flight from this
//     life, which drainMeshed would then accept as current,
//   - remove it from the pending requested queue if a worker never picked it
//     up, so dispatchGenerating never spends a worker generating a chunk
//     nobody wants any more,
//   - delete it from World.Chunks, so it is no longer part of the world at
//     all, and
//   - invalidate its neighbours' meshes, because their face culling assumed
//     this chunk was there and it no longer is — the exact mirror of what
//     invalidateNeighbourMeshesLocked does when a chunk arrives, run here
//     because a chunk disappearing changes seam geometry exactly as much as
//     one appearing does.
//
// Missing any one of these is a leak or a stale mesh. It is safe to call for
// a coord that was never requested; every step is then a no-op.
func (p *Pipeline) Release(coord world.ChunkCoord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.stages, coord)
	delete(p.epoch, coord)

	for i, c := range p.requested {
		if c == coord {
			p.requested = append(p.requested[:i], p.requested[i+1:]...)
			break
		}
	}

	delete(p.w.Chunks, coord)

	p.invalidateNeighbourMeshesLocked(coord)
}

// takeEpochLocked returns an epoch value that has never been used before and
// never will be again. p.mu must already be held.
//
// Every site that invalidates a mesh — dispatching a new job, a neighbour
// arriving or leaving, an external edit — draws from here rather than
// incrementing a per-coord counter, so "this result is stale" is decided by
// identity rather than by ordering.
func (p *Pipeline) takeEpochLocked() int {
	p.nextEpoch++
	return p.nextEpoch
}

// Update advances the pipeline by one step and returns the meshes finished
// this call, ready for the caller to upload. It must only be called from the
// main thread: it is the only place World.Chunks is written, the only place
// lighting.Engine is touched, and the only place a Snapshot is taken.
//
// It never blocks. Every channel operation is non-blocking, so a worker queue
// that is currently full simply leaves its work for the next call rather than
// stalling this one.
func (p *Pipeline) Update(budget Budget) []Ready {
	p.drainGenerated()
	p.seedLit(budget.Light)
	p.demoteDirty()
	p.dispatchMeshing()
	ready := p.drainMeshed(budget.Mesh)
	p.dispatchGenerating()
	return ready
}

// drainGenerated inserts every chunk a generation worker has finished into
// World.Chunks and marks it Generated, unless coord is no longer wanted. It
// is unbounded: draining a finished result only ever does two map writes, so
// there is no per-frame cost worth budgeting here the way there is for
// lighting and meshing.
//
// The wanted-check exists because of Release (M15): a result can arrive for
// a coord that was released while its generation job was still in flight.
// Inserting it unconditionally, as this did before dynamic unloading existed,
// would resurrect a chunk nobody wants — a genuine leak, and the loaded set
// would silently stop matching the desired set. The check is simply that
// coord is still known and still Generating: Release deletes coord from
// stages entirely, so a released coord fails "still known", and a coord that
// was released and then re-requested before this result arrived is back at
// Absent, which also fails "still Generating".
//
// This uses no generation epoch, and deliberately so — adding one would be
// cargo cult. The one sequence that looks like it needs one is: request,
// release, request again, and only then the original result arrives. At that
// point the stage is Generating (the second Request put it there) and the
// coord is known, so the wanted-check accepts the result — and that is
// correct, not a near-miss the epoch happened to paper over. Generator.Generate
// is required to be a pure function of coord, so the stale-looking result and
// the one actually in flight for the second request are computed from
// identical inputs and are the same chunk byte for byte. An epoch earns its
// keep only when a result can be wrong depending on when it was computed —
// true for meshing, which reads the mutable world, and false for generation,
// which reads nothing at all.
func (p *Pipeline) drainGenerated() {
	for {
		select {
		case res := <-p.genResults:
			p.mu.Lock()
			if stage, known := p.stages[res.coord]; !known || stage != Generating {
				// Released (and possibly not yet re-requested) since this job
				// was dispatched: applying it would write World.Chunks for a
				// coord the pipeline no longer considers loaded.
				p.mu.Unlock()
				continue
			}

			p.w.Chunks[res.coord] = res.chunk
			p.stages[res.coord] = Generated
			p.invalidateNeighbourMeshesLocked(res.coord)
			p.mu.Unlock()
		default:
			return
		}
	}
}

// invalidateNeighbourMeshesLocked sends any already-meshed neighbour of coord
// back to be meshed again. Callers must hold p.mu.
//
// A chunk arriving changes its neighbours' geometry even when it changes none
// of their light. The mesher culls faces against whatever is next door, so a
// neighbour meshed while this chunk was absent drew the seam faces that are
// now hidden, and it will keep drawing them: demoteDirty cannot help, because
// it reacts to light changing and nothing here did.
//
// Flat terrain arriving beside flat terrain is exactly that case, and it does
// not arise while the whole region is requested at once — every chunk is
// present before anything is meshed. It arises the moment chunks arrive one at
// a time, which is what streaming does.
func (p *Pipeline) invalidateNeighbourMeshesLocked(coord world.ChunkCoord) {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}

			neighbour := world.ChunkCoord{X: coord.X + dx, Z: coord.Z + dz}
			switch p.stages[neighbour] {
			case Meshed, Meshing:
				p.stages[neighbour] = Lit
				// Take a fresh epoch so an in-flight job for this neighbour,
				// built against a world without coord in it, is discarded on
				// arrival.
				p.epoch[neighbour] = p.takeEpochLocked()
			}
		}
	}
}

// seedLit lights every Generated chunk whose eight requested neighbours have
// all reached at least Generated, up to limit chunks. limit <= 0 lights
// nothing this call.
func (p *Pipeline) seedLit(limit int) {
	if limit <= 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	lit := 0
	for coord, stage := range p.stages {
		if lit >= limit {
			return
		}
		if stage != Generated || !p.neighborsAtLeastLocked(coord, Generated) {
			continue
		}

		p.light.SeedChunk(coord)
		p.stages[coord] = Lit
		lit++
	}
}

// demoteDirty pushes every Meshed or Meshing chunk whose light changed since
// the last call back to Lit, so it is re-meshed with the corrected light.
//
// This is not optional. Light crosses chunk seams (see
// lighting.Engine.markMeshDirty), so seeding or editing one chunk routinely
// changes what a neighbour's mesh should look like even though nothing inside
// the neighbour itself changed. Skipping this leaves that neighbour rendering
// stale darkness — the exact seam bug M3 exists to fix, recreated one layer up
// if this step is dropped.
func (p *Pipeline) demoteDirty() {
	dirty := p.light.DirtyChunks()
	if len(dirty) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	for _, coord := range dirty {
		switch p.stages[coord] {
		case Meshed, Meshing:
			p.stages[coord] = Lit
		}
	}
}

// dispatchMeshing takes a snapshot and starts a mesh job for every Lit chunk
// whose eight requested neighbours have all reached at least Lit. It stops at
// the first full meshQueue and leaves the rest for the next call.
func (p *Pipeline) dispatchMeshing() {
	p.mu.Lock()
	var candidates []world.ChunkCoord
	for coord, stage := range p.stages {
		if stage == Lit && p.neighborsAtLeastLocked(coord, Lit) {
			candidates = append(candidates, coord)
		}
	}
	p.mu.Unlock()

	for _, coord := range candidates {
		// Take runs on this goroutine, as its own doc requires: Update is the
		// main thread, and it owns w.
		snap := Take(p.w, coord)

		p.mu.Lock()
		if p.stages[coord] != Lit {
			// Something else moved this coord on (demoteDirty running again
			// on a later call cannot, but a defensive check here costs
			// nothing and means this function never depends on being the
			// only writer of Lit->Meshing).
			p.mu.Unlock()
			snap.Release()
			continue
		}

		p.epoch[coord] = p.takeEpochLocked()
		job := meshJob{coord: coord, epoch: p.epoch[coord], snapshot: snap}

		select {
		case p.meshQueue <- job:
			p.stages[coord] = Meshing
			p.mu.Unlock()
		default:
			// The epoch stays where it is. There is nothing to undo: epochs
			// are drawn from a counter that only ever moves forward, so a
			// value spent on a job that was never sent is simply skipped.
			// Leaving it also happens to be the safe direction — if a job
			// for this coord is still in flight from before a demotion, the
			// value taken here already tells drainMeshed to discard it.
			p.mu.Unlock()
			snap.Release()
			return
		}
	}
}

// drainMeshed collects up to limit finished mesh jobs, releasing each
// snapshot and marking the chunk Meshed. limit <= 0 returns nothing this
// call, though the results already sitting in the channel are not lost — they
// are picked up by a later call with a positive budget.
//
// A result is discarded, its snapshot still released, when the chunk it
// belongs to is no longer Meshing at the epoch the job was dispatched at.
// That is demoteDirty's doing: a chunk demoted back to Lit while its old mesh
// job was still running would otherwise have that stale job overwrite the
// fresh one queued after it.
func (p *Pipeline) drainMeshed(limit int) []Ready {
	if limit <= 0 {
		return nil
	}

	ready := make([]Ready, 0, limit)
	for len(ready) < limit {
		select {
		case res := <-p.meshResults:
			res.snapshot.Release()

			p.mu.Lock()
			stale := p.stages[res.coord] != Meshing || p.epoch[res.coord] != res.epoch
			if !stale {
				p.stages[res.coord] = Meshed
			}
			p.mu.Unlock()

			if !stale {
				ready = append(ready, Ready{Coord: res.coord, Data: res.data})
			}
		default:
			return ready
		}
	}
	return ready
}

// dispatchGenerating sends every Absent chunk still waiting in the request
// queue to a generation worker, oldest first, stopping at the first full
// genQueue and leaving the rest queued for the next call.
func (p *Pipeline) dispatchGenerating() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for len(p.requested) > 0 {
		coord := p.requested[0]

		select {
		case p.genQueue <- coord:
			p.requested = p.requested[1:]
			p.stages[coord] = Generating
		default:
			return
		}
	}
}

// neighborsAtLeastLocked reports whether every one of coord's eight
// neighbours that has been requested has reached at least min. p.mu must
// already be held.
//
// A neighbour that was never requested is not in p.stages at all and is
// treated as satisfying any stage: in this milestone nothing is ever
// requested after startup (dynamic loading is M15), so a neighbour that is
// not present now can never later appear and invalidate this chunk's light or
// mesh. Requiring literally all eight positions to be requested would leave
// every chunk on the edge of a finite region — such as the fixed 3x3 area
// wired up in this milestone — stuck below Lit forever, since chunks outside
// that area are never requested.
func (p *Pipeline) neighborsAtLeastLocked(coord world.ChunkCoord, min Stage) bool {
	for dx := -1; dx <= 1; dx++ {
		for dz := -1; dz <= 1; dz++ {
			if dx == 0 && dz == 0 {
				continue
			}
			n := world.ChunkCoord{X: coord.X + dx, Z: coord.Z + dz}
			if stage, known := p.stages[n]; known && stage < min {
				return false
			}
		}
	}
	return true
}

// Invalidate tells the pipeline that coord's mesh is stale — for instance
// because a block edit changed it outside the pipeline. If a mesh job for
// coord is in flight, its result is discarded when it arrives rather than
// applied, matching the ownership rule that a chunk being meshed must be
// treated as immutable: the caller that changed the chunk, not the pipeline,
// owns getting the new state on screen immediately, but the pipeline must not
// be allowed to overwrite that with a mesh built from the old blocks.
//
// It is a no-op for a coord that is not Meshing or Meshed; there is nothing
// to invalidate before a chunk has been meshed once, and Absent/Generating/
// Generated/Lit chunks have no mesh yet for an in-flight job to overwrite.
func (p *Pipeline) Invalidate(coord world.ChunkCoord) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.stages[coord] {
	case Meshing:
		// The in-flight job's epoch is now stale, so drainMeshed will discard
		// its result instead of marking the chunk Meshed with old data.
		p.epoch[coord] = p.takeEpochLocked()
		p.stages[coord] = Lit
	case Meshed:
		p.stages[coord] = Lit
	}
}

// Close stops every worker goroutine and waits for them to exit. It is safe
// to call once; a second call is a no-op.
func (p *Pipeline) Close() {
	p.closeOnce.Do(func() {
		close(p.stop)
		p.wg.Wait()
	})
}
