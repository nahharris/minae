package chunks_test

import (
	"reflect"
	"sync"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/chunks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// scene builds a 3x3 chunk world around the origin with terrain, a carved
// pocket and varied light, so snapshots have something non-uniform to copy.
func scene(t *testing.T) *world.World {
	t.Helper()

	const surface = 40
	w := testutil.NewWorld(t).
		Chunks(-1, -1, 1, 1).
		Flat(surface).
		Clear(testutil.Box{MinX: -4, MinY: surface - 2, MinZ: -4, MaxX: 4, MaxY: surface, MaxZ: 4}).
		Fill(testutil.Box{MinX: 2, MinY: surface - 1, MinZ: 2, MaxX: 2, MaxY: surface - 1, MaxZ: 2}, blocks.Glowstone).
		Build()

	// Light that is not uniform, so a snapshot copying the wrong array shows.
	for x := -20; x <= 20; x++ {
		for z := -20; z <= 20; z++ {
			w.SetSkyLight(x, surface+1, z, uint8((x+z)&15))
			w.SetBlockLight(x, surface, z, uint8((x*3+z)&15))
		}
	}
	return w
}

func TestSnapshot_ReadsMatchTheLiveWorld(t *testing.T) {
	w := scene(t)

	for _, center := range []world.ChunkCoord{{X: 0, Z: 0}, {X: -1, Z: -1}, {X: 1, Z: 0}} {
		t.Run("", func(t *testing.T) {
			s := chunks.Take(w, center)
			defer s.Release()

			// Every column in the 3x3 neighbourhood, at several heights.
			for dx := -1; dx <= 1; dx++ {
				for dz := -1; dz <= 1; dz++ {
					baseX := (center.X + dx) * 16
					baseZ := (center.Z + dz) * 16

					for _, off := range [][2]int{{0, 0}, {15, 15}, {7, 3}} {
						x, z := baseX+off[0], baseZ+off[1]
						for _, y := range []int{0, 39, 40, 41} {
							wantBlock, wantMeta := w.GetBlockState(x, y, z)
							gotBlock, gotMeta := s.GetBlockState(x, y, z)
							if gotBlock != wantBlock || gotMeta != wantMeta {
								t.Fatalf("(%d,%d,%d): block state %v/%d, want %v/%d",
									x, y, z, gotBlock, gotMeta, wantBlock, wantMeta)
							}
							if got, want := s.GetSkyLight(x, y, z), w.GetSkyLight(x, y, z); got != want {
								t.Fatalf("(%d,%d,%d): skylight %d, want %d", x, y, z, got, want)
							}
							if got, want := s.GetBlockLight(x, y, z), w.GetBlockLight(x, y, z); got != want {
								t.Fatalf("(%d,%d,%d): block light %d, want %d", x, y, z, got, want)
							}
						}
					}
				}
			}
		})
	}
}

// The whole point: once taken, a snapshot must be unaffected by later writes
// to the live world. All four arrays are checked, because a shallow copy of
// any one of them is exactly the bug this guards against.
func TestSnapshot_IsIndependentOfTheLiveWorld(t *testing.T) {
	w := scene(t)

	const x, y, z = 3, 41, 3
	s := chunks.Take(w, world.ChunkCoord{X: 0, Z: 0})
	defer s.Release()

	beforeBlock, beforeMeta := s.GetBlockState(x, y, z)
	beforeSky := s.GetSkyLight(x, y, z)
	beforeBlockLight := s.GetBlockLight(x, y, z)

	w.SetBlockState(x, y, z, blocks.Stone, 3)
	w.SetSkyLight(x, y, z, 1)
	w.SetBlockLight(x, y, z, 2)

	// Sanity: the live world really did change, or this proves nothing.
	if gotBlock, _ := w.GetBlockState(x, y, z); gotBlock == beforeBlock {
		t.Fatal("the live world did not change; the test proves nothing")
	}

	if gotBlock, gotMeta := s.GetBlockState(x, y, z); gotBlock != beforeBlock || gotMeta != beforeMeta {
		t.Errorf("block state changed under the snapshot: %v/%d, want %v/%d",
			gotBlock, gotMeta, beforeBlock, beforeMeta)
	}
	if got := s.GetSkyLight(x, y, z); got != beforeSky {
		t.Errorf("skylight changed under the snapshot: %d, want %d", got, beforeSky)
	}
	if got := s.GetBlockLight(x, y, z); got != beforeBlockLight {
		t.Errorf("block light changed under the snapshot: %d, want %d", got, beforeBlockLight)
	}
}

func TestSnapshot_UnloadedAndOutOfRange(t *testing.T) {
	// One chunk only, so every neighbour is absent.
	w := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Flat(40).Build()

	s := chunks.Take(w, world.ChunkCoord{X: 0, Z: 0})
	defer s.Release()

	if !s.HasChunkAt(0, 0) {
		t.Error("the centre chunk should be loaded")
	}
	for _, pos := range [][2]int{
		{-1, 0},  // absent neighbour
		{16, 0},  // absent neighbour
		{0, -1},  // absent neighbour
		{40, 40}, // outside the 3x3 neighbourhood entirely
		{-40, 0},
	} {
		if s.HasChunkAt(pos[0], pos[1]) {
			t.Errorf("HasChunkAt(%d,%d) = true, want false", pos[0], pos[1])
		}
		if b := s.GetBlock(pos[0], 40, pos[1]); b != nil {
			t.Errorf("GetBlock at unloaded (%d,%d) = %v, want nil", pos[0], pos[1], b)
		}
	}
}

// Substituting the snapshot for the live world must be invisible to the
// mesher. If the two produce identical geometry, the copy is faithful in
// every way the renderer can observe.
func TestSnapshot_MeshingMatchesTheLiveWorld(t *testing.T) {
	w := scene(t)
	coord := world.ChunkCoord{X: 0, Z: 0}

	live := mesh.GenerateChunkMeshData(w.GetChunk(coord.X, coord.Z), w, nil)
	if live == nil {
		t.Fatal("expected mesh data from the live world")
	}

	s := chunks.Take(w, coord)
	defer s.Release()

	fromSnapshot := mesh.GenerateChunkMeshData(s.Center(), s, nil)
	if fromSnapshot == nil {
		t.Fatal("expected mesh data from the snapshot")
	}

	if !reflect.DeepEqual(live.Vertices, fromSnapshot.Vertices) {
		t.Errorf("vertices differ: %d from the world, %d from the snapshot",
			len(live.Vertices), len(fromSnapshot.Vertices))
	}
	if !reflect.DeepEqual(live.Normals, fromSnapshot.Normals) {
		t.Error("normals differ")
	}
	if !reflect.DeepEqual(live.Texcoords, fromSnapshot.Texcoords) {
		t.Error("texcoords differ")
	}
	if !reflect.DeepEqual(live.Colors, fromSnapshot.Colors) {
		t.Error("colours differ — light or ambient occlusion did not survive the copy")
	}
}

// The scenario the snapshot exists for: workers meshing while the main
// goroutine keeps mutating the world.
func TestSnapshot_SafeUnderConcurrentWorldMutation(t *testing.T) {
	w := scene(t)
	coord := world.ChunkCoord{X: 0, Z: 0}

	s := chunks.Take(w, coord)
	defer s.Release()

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A handful of builds per goroutine is enough for the race
			// detector to observe the access pattern; more only costs time,
			// and meshing a full terrain chunk is not cheap.
			for range 4 {
				if data := mesh.GenerateChunkMeshData(s.Center(), s, nil); data == nil {
					t.Error("expected mesh data")
					return
				}
			}
		}()
	}

	// Meanwhile the world keeps changing, which is what would race if the
	// snapshot shared anything with it.
	for i := range 200 {
		w.SetSkyLight(i%16, 41, 0, uint8(i&15))
		w.SetBlock(i%16, 41, 1, blocks.Stone)
	}

	wg.Wait()
}

// A recycled snapshot must carry nothing over. The second take has fewer
// loaded neighbours than the first, which is where leftovers would show.
func TestSnapshot_ReuseDoesNotLeakStaleNeighbours(t *testing.T) {
	full := scene(t)
	lone := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Flat(40).Build()

	// Take and release, so the same Snapshot is very likely handed back.
	first := chunks.Take(full, world.ChunkCoord{X: 0, Z: 0})
	if !first.HasChunkAt(-1, -1) {
		t.Fatal("precondition: the full world should have a neighbour at (-1,-1)")
	}
	first.Release()

	second := chunks.Take(lone, world.ChunkCoord{X: 0, Z: 0})
	defer second.Release()

	if second.HasChunkAt(-1, -1) {
		t.Error("a neighbour from the previous use survived into a recycled snapshot")
	}
	if b := second.GetBlock(-1, 40, -1); b != nil {
		t.Errorf("stale block data survived reuse: got %v, want nil", b)
	}
}
