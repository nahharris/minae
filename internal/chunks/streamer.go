package chunks

import (
	"sort"

	"github.com/nahharris/minae/internal/world"
)

// requester is the subset of Pipeline's API a Streamer drives. It exists so
// the desired-set logic — ring computation, ordering and hysteresis — can be
// tested against a lightweight fake instead of a real Pipeline with real
// workers. See the M15 design decision "the streamer is separate from the
// pipeline": that separation is what makes this interface possible at all.
type requester interface {
	Request(coord world.ChunkCoord)
	Release(coord world.ChunkCoord)
}

// compile-time check that *Pipeline satisfies requester.
var _ requester = (*Pipeline)(nil)

// Streamer owns the desired set of loaded chunks around a moving player and
// drives a Pipeline to match it, via Request and Release.
//
// It knows nothing about workers, generation or meshing, or even about the
// concrete Pipeline type — only coordinates, the two radii, and requester —
// which is what makes ring computation, ordering and hysteresis testable
// without a single goroutine. See streamer_test.go for tests that construct
// a Streamer around a fake requester and never touch a real Pipeline.
type Streamer struct {
	p            requester
	loadRadius   int
	unloadRadius int

	// loaded is every coord the Streamer currently considers requested: it
	// has called Request and not yet called Release for it. This is the
	// Streamer's own bookkeeping, independent of what stage the pipeline has
	// actually reached — Update needs to know what it has already asked for,
	// not what has arrived.
	loaded map[world.ChunkCoord]struct{}
}

// NewStreamer returns a Streamer that drives p, loading every chunk within
// loadRadius chunks (Chebyshev distance) of the player's chunk and unloading
// anything beyond unloadRadius.
//
// unloadRadius should exceed loadRadius by at least 2 chunks; that margin is
// what the M15 design decisions call hysteresis, and it is what keeps a
// player standing on a chunk boundary from loading and unloading the same
// ring every frame. NewStreamer does not enforce the margin itself — the
// radii are just numbers to this type — but a caller that ignores it will
// see the frontier thrash on the very first playtest.
func NewStreamer(p requester, loadRadius, unloadRadius int) *Streamer {
	return &Streamer{
		p:            p,
		loadRadius:   loadRadius,
		unloadRadius: unloadRadius,
		loaded:       make(map[world.ChunkCoord]struct{}),
	}
}

// Update recomputes the desired set around the player's current chunk and
// drives the pipeline to match it: every chunk within loadRadius that is not
// already loaded is requested, nearest first, and every loaded chunk beyond
// unloadRadius is released. It returns the coords released this call, so a
// caller can free anything tied to them outside the pipeline — a GPU mesh,
// most notably; see app.go.
func (s *Streamer) Update(center world.ChunkCoord) []world.ChunkCoord {
	for _, coord := range coordsToRequest(center, s.loadRadius, s.loaded) {
		s.p.Request(coord)
		s.loaded[coord] = struct{}{}
	}

	released := coordsToRelease(center, s.unloadRadius, s.loaded)
	for _, coord := range released {
		s.p.Release(coord)
		delete(s.loaded, coord)
	}
	return released
}

// Loaded reports whether coord is currently in the Streamer's desired set —
// it has been requested and not yet released.
func (s *Streamer) Loaded(coord world.ChunkCoord) bool {
	_, ok := s.loaded[coord]
	return ok
}

// Len returns how many coords the Streamer currently considers loaded.
func (s *Streamer) Len() int {
	return len(s.loaded)
}

// coordsToRequest returns every coord within loadRadius chunks (Chebyshev
// distance, so the region is a square matching the chunk grid) of center
// that is not already in loaded, ordered nearest-first by squared Euclidean
// distance so that what the player is about to see is requested before what
// they might see later.
//
// This is a pure function of its arguments — no pipeline, no goroutines — so
// the desired-set and ordering logic can be tested directly, which is the
// entire point of keeping Streamer separate from Pipeline.
func coordsToRequest(center world.ChunkCoord, loadRadius int, loaded map[world.ChunkCoord]struct{}) []world.ChunkCoord {
	var out []world.ChunkCoord
	for dx := -loadRadius; dx <= loadRadius; dx++ {
		for dz := -loadRadius; dz <= loadRadius; dz++ {
			coord := world.ChunkCoord{X: center.X + dx, Z: center.Z + dz}
			if _, ok := loaded[coord]; ok {
				continue
			}
			out = append(out, coord)
		}
	}

	// Stable, not sort.Slice: many coords sit at the same distance from the
	// centre, and an unstable sort orders those ties however the algorithm
	// happens to land. The loop above emits candidates in a fixed dx/dz
	// order, so a stable sort turns that into a total order and two runs
	// from the same state request chunks in exactly the same sequence.
	sort.SliceStable(out, func(i, j int) bool {
		return sqDist(center, out[i]) < sqDist(center, out[j])
	})
	return out
}

// coordsToRelease returns every coord in loaded whose Chebyshev distance from
// center exceeds unloadRadius — the chunks hysteresis says have drifted far
// enough from the player to give up.
func coordsToRelease(center world.ChunkCoord, unloadRadius int, loaded map[world.ChunkCoord]struct{}) []world.ChunkCoord {
	var out []world.ChunkCoord
	for coord := range loaded {
		if chebyshev(center, coord) > unloadRadius {
			out = append(out, coord)
		}
	}

	// Sorted because the range above walks a map, and Go deliberately
	// randomises that order. The released set is the same either way, but
	// Update hands this slice back to the caller to free GPU meshes with, and
	// a caller that behaves differently run to run is the kind of thing that
	// reproduces once a month. Ordering it costs nothing at these sizes.
	sort.Slice(out, func(i, j int) bool {
		if out[i].X != out[j].X {
			return out[i].X < out[j].X
		}
		return out[i].Z < out[j].Z
	})
	return out
}

// chebyshev returns the Chebyshev distance between two chunk coordinates:
// the radius of the smallest square, centred on a, that contains b. Both the
// load and unload regions use this rather than Euclidean distance so the
// loaded area is a square matching the chunk grid, not a circle that would
// load some chunks at a given "radius" and not others at the same radius
// along a diagonal.
func chebyshev(a, b world.ChunkCoord) int {
	dx := absInt(a.X - b.X)
	dz := absInt(a.Z - b.Z)
	if dx > dz {
		return dx
	}
	return dz
}

// sqDist returns the squared Euclidean distance between two chunk
// coordinates. Squared, not the true distance, because request ordering only
// needs a consistent ranking, and every candidate is compared every call —
// one fewer square root each time is worth taking.
func sqDist(a, b world.ChunkCoord) int {
	dx := a.X - b.X
	dz := a.Z - b.Z
	return dx*dx + dz*dz
}

// absInt returns the absolute value of v.
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
