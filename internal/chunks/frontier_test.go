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

// Bounds of the sealed room caveThroughFrontierGenerator carves into chunk
// (0,0). Exported as constants so the test can sample a cell inside it.
const (
	frontierRoomFloor   = 10
	frontierRoomCeiling = 15
	frontierRoomZMin    = 1
	frontierRoomZMax    = 14
)

// caveThroughFrontierGenerator builds chunk (0,0) as solid rock riddled with
// a sealed room whose only route to the sky runs through chunk (1,0): the
// room opens only at local x==15, the seam shared with (1,0), and (1,0)
// carries a vertical shaft to open sky at local x==0, lined up with the
// room's height and depth. Until (1,0) is loaded, the room has no other way
// to receive skylight — everything else, in both chunks, is solid rock.
//
// This is the exact scenario the M15 design decisions call out: "a cave
// whose only route to the sky runs through a chunk that had not loaded yet
// — dark on arrival at the frontier, correctly lit once the neighbour
// lands." Flat terrain or a simpler cave cannot distinguish this from a
// pipeline that never re-lights a frontier chunk at all, because there would
// be no light to receive in the first place.
type caveThroughFrontierGenerator struct{}

func (caveThroughFrontierGenerator) Generate(coord world.ChunkCoord) *world.Chunk {
	c := world.NewChunk(coord.X, coord.Z)
	for lx := range config.ChunkWidth {
		for lz := range config.ChunkWidth {
			for y := range config.ChunkHeight {
				c.SetBlock(lx, y, lz, blocks.Stone)
			}
		}
	}

	switch coord {
	case world.ChunkCoord{X: 0, Z: 0}:
		// The sealed room: open everywhere within its box, walled in by the
		// solid rock around it except at lx==15, the seam with (1,0).
		for lx := 1; lx <= 15; lx++ {
			for lz := frontierRoomZMin; lz <= frontierRoomZMax; lz++ {
				for y := frontierRoomFloor; y <= frontierRoomCeiling; y++ {
					c.SetBlock(lx, y, lz, nil)
				}
			}
		}
	case world.ChunkCoord{X: 1, Z: 0}:
		// The shaft: open from the room's floor all the way to the top of
		// the world, at the single column facing the room across the seam.
		for lz := frontierRoomZMin; lz <= frontierRoomZMax; lz++ {
			for y := frontierRoomFloor; y < config.ChunkHeight; y++ {
				c.SetBlock(0, y, lz, nil)
			}
		}
	}
	return c
}

// TestPipeline_LateNeighbourLightsSealedCaveAndRemeshesIt covers validation
// criterion 5: a newly loaded chunk is lit, and lights its already-loaded
// neighbour's cave through the seam that was the neighbour's only route to
// the sky, re-meshing it so the seam is not left dark. This is the streaming
// form of the bug that produced a black wall in M3.
func TestPipeline_LateNeighbourLightsSealedCaveAndRemeshesIt(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	p := chunks.NewPipeline(w, light, caveThroughFrontierGenerator{}, nil, 2)
	defer p.Close()

	near := world.ChunkCoord{X: 0, Z: 0}
	far := world.ChunkCoord{X: 1, Z: 0}

	latest := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	drive := func(until func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			for _, r := range p.Update(chunks.Budget{Light: time.Second, Mesh: time.Second}) {
				latest[r.Coord] = r.Data
			}
			if until() {
				return
			}
			runtime.Gosched()
		}
		t.Fatal("pipeline did not reach the expected state in time")
	}

	p.Request(near)
	drive(func() bool { return latest[near] != nil })
	before := latest[near]
	if before == nil || len(before.Vertices) == 0 {
		t.Fatal("chunk (0,0) produced no geometry; the test would prove nothing")
	}

	roomX, roomY, roomZ := 8, (frontierRoomFloor+frontierRoomCeiling)/2, 8
	if got := w.GetSkyLight(roomX, roomY, roomZ); got != 0 {
		t.Fatalf("room skylight = %d before the frontier neighbour loaded, want 0 (sealed): "+
			"the test's own geometry is wrong if this fails", got)
	}

	// Now the neighbour carrying the room's only route to the sky arrives —
	// exactly what streaming does when the player walks toward it.
	p.Request(far)
	drive(func() bool { return latest[near] != nil && !reflect.DeepEqual(latest[near], before) })

	after := latest[near]
	if got := w.GetSkyLight(roomX, roomY, roomZ); got == 0 {
		t.Fatal("room is still dark after the neighbour carrying its only route to the sky loaded")
	}
	if reflect.DeepEqual(after.Colors, before.Colors) {
		t.Fatal("chunk (0,0)'s mesh colours are unchanged: it was not re-meshed after light reached its sealed room")
	}

	// The rebuilt mesh must match a fresh build of the finished world, not
	// merely differ from the stale one.
	fresh := mesh.GenerateChunkMeshData(w.GetChunk(near.X, near.Z), w, nil)
	if !reflect.DeepEqual(fresh.Colors, after.Colors) {
		t.Errorf("re-meshed colours do not match a fresh build of the finished world")
	}
}

// TestPipeline_DemoteDirtyReMeshesOnCrossSeamLightChangeAlone isolates
// demoteDirty from invalidateNeighbourMeshesLocked.
//
// The frontier test above cannot separate the two: a chunk arriving always
// makes its already-meshed neighbour's stage regress via
// invalidateNeighbourMeshesLocked before that neighbour's own light could
// possibly change, because the arriving chunk must itself reach Lit — which
// requires it to already be seeded — before the stage-gate lets its meshed
// neighbour be re-meshed at all. So for a brand new neighbour,
// invalidateNeighbourMeshesLocked always pre-empts demoteDirty, and disabling
// demoteDirty alone does not fail that test (confirmed while writing this
// one — see the mutation-testing notes in the M15 report).
//
// This test instead changes a block deep inside an already-loaded chunk
// pair, with no chunk transitioning to Generated at all, so
// invalidateNeighbourMeshesLocked never runs. If a chunk's mesh is stale
// after a cross-seam light change with no arrival involved, demoteDirty is
// the only thing in the pipeline that could have fixed it.
func TestPipeline_DemoteDirtyReMeshesOnCrossSeamLightChangeAlone(t *testing.T) {
	blocks.ResetToVanilla()

	w := world.NewWorld()
	light := lighting.NewEngine(w)
	p := chunks.NewPipeline(w, light, chunks.FlatGenerator{}, nil, 2)
	defer p.Close()

	west := world.ChunkCoord{X: 0, Z: 0}
	east := world.ChunkCoord{X: 1, Z: 0}
	p.Request(west)
	p.Request(east)

	latest := map[world.ChunkCoord]*mesh.ChunkMeshData{}
	drive := func(until func() bool) {
		t.Helper()
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			for _, r := range p.Update(chunks.Budget{Light: time.Second, Mesh: time.Second}) {
				latest[r.Coord] = r.Data
			}
			if until() {
				return
			}
			runtime.Gosched()
		}
		t.Fatal("pipeline did not reach the expected state in time")
	}
	drive(func() bool { return latest[west] != nil && latest[east] != nil })
	before := latest[west]

	// A block placed at chunk (1,0)'s local x==0 -- the seam with (0,0) --
	// at an interior z, above the flat surface. FlatGenerator's terrain has
	// open sky above y==31, so this blocks a previously fully-lit column.
	// markMeshDirty's border rule marks BOTH chunks sharing this seam
	// column dirty on every write there, regardless of whether (0,0)'s own
	// stored light values change -- exactly the "nothing inside the
	// neighbour itself changed" case demoteDirty's doc comment describes.
	const seamX, seamY, seamZ = 16, 32, 8
	w.SetBlock(seamX, seamY, seamZ, blocks.Stone)
	light.OnBlockChanged(seamX, seamY, seamZ) // bypasses p.Invalidate deliberately

	drive(func() bool { return latest[west] != nil && !reflect.DeepEqual(latest[west], before) })

	if reflect.DeepEqual(latest[west].Colors, before.Colors) {
		t.Fatal("chunk (0,0) was never re-meshed after a light-only change on its neighbour's side of the seam")
	}
}
