// Package lighting computes and incrementally maintains chunk skylight and
// block light.
package lighting

import (
	_ "embed"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

//go:embed shaders/vertex.glsl
var VsCode string

//go:embed shaders/fragment.glsl
var FsCode string

// MaxSkyLight is the brightest possible skylight level: direct, unobstructed
// sky.
const MaxSkyLight uint8 = 15

// lightNode is a queued position and the light level it was enqueued at.
type lightNode struct {
	X, Y, Z int
	Level   uint8
}

// direction is one of the six axis-aligned neighbour offsets.
type direction struct {
	DX, DY, DZ int
}

var directions = [6]direction{
	{1, 0, 0}, {-1, 0, 0},
	{0, 1, 0}, {0, -1, 0},
	{0, 0, 1}, {0, 0, -1},
}

// isDown reports whether d points straight down.
func (d direction) isDown() bool {
	return d.DX == 0 && d.DY == -1 && d.DZ == 0
}

// isTransparent reports whether a block lets light pass through it. Air
// always reads back as nil from World.GetBlock and Chunk.GetBlock, so this is
// the one place to extend when translucent blocks such as glass or leaves are
// added. The rule is the same for skylight and block light: a source cell may
// itself be opaque (a glowstone still emits from its own cell), but light
// never spreads through an opaque neighbour.
func isTransparent(b *blocks.Block) bool {
	return b == nil
}

// decayByOne returns from-1, saturating at 0. It is the falloff every
// direction uses for block light, and every direction except straight down
// uses for skylight.
func decayByOne(from uint8) uint8 {
	if from == 0 {
		return 0
	}
	return from - 1
}

// skyExpected returns the skylight level a neighbour reached by moving from a
// cell at level from in direction d should hold. Skylight falls straight
// down at full strength; it decays by one in every other direction,
// including diagonally-adjacent-but-still-vertical cases, since d is always
// one of the six axis directions.
func skyExpected(from uint8, d direction) uint8 {
	if d.isDown() && from == MaxSkyLight {
		return MaxSkyLight
	}
	return decayByOne(from)
}

// blockExpected returns the block light level a neighbour reached by moving
// from a cell at level from in direction d should hold. Unlike skylight,
// block light has no lossless direction: it decays by one everywhere,
// including straight down. Applying skylight's lossless-downward rule to a
// torch would make every torch cast a bright shaft to bedrock.
func blockExpected(from uint8, _ direction) uint8 {
	return decayByOne(from)
}

// lightKind describes one propagation channel to the shared BFS: how to read
// a cell's current level, how to write it, and how a level decays across a
// step in a given direction. Skylight and block light are identical in every
// other respect, so a single add walk and a single removal walk, each
// parameterized over a lightKind, replace what would otherwise be two nearly
// identical copies of both.
type lightKind struct {
	get      func(w *world.World, x, y, z int) uint8
	set      func(w *world.World, x, y, z int, level uint8)
	expected func(from uint8, d direction) uint8
}

// skyKind is the skylight propagation channel.
var skyKind = lightKind{
	get:      (*world.World).GetSkyLight,
	set:      (*world.World).SetSkyLight,
	expected: skyExpected,
}

// blockKind is the block light propagation channel.
var blockKind = lightKind{
	get:      (*world.World).GetBlockLight,
	set:      (*world.World).SetBlockLight,
	expected: blockExpected,
}

// Engine incrementally maintains the skylight and block light of every
// loaded chunk in a World. Create one with NewEngine.
type Engine struct {
	w *world.World

	addQueue    []lightNode
	removeQueue []lightNode
	reAdd       []lightNode

	dirty map[world.ChunkCoord]struct{}
}

// NewEngine returns an engine that lights w.
func NewEngine(w *world.World) *Engine {
	return &Engine{
		w:     w,
		dirty: make(map[world.ChunkCoord]struct{}),
	}
}

// RecomputeAll rebuilds the skylight and block light of every loaded chunk
// from scratch. It is order-independent: every loaded chunk is zeroed first,
// then a single top-down column scan seeds skylight and a single full-volume
// scan seeds block light from every emitting block, and only then does each
// queue propagate. Seeding and propagating chunk by chunk would make the
// result depend on map iteration order.
func (e *Engine) RecomputeAll() {
	for _, chunk := range e.w.Chunks {
		// clear compiles to a single memclr. The equivalent element-by-element
		// loop is 65536 separate writes per chunk, which the race detector and
		// the coverage counters both instrument individually — enough to
		// dominate the test suite's runtime.
		clear(chunk.SkyLight[:])
		clear(chunk.BlockLight[:])
	}

	e.markChunksDirty()

	e.addQueue = e.addQueue[:0]

	for _, chunk := range e.w.Chunks {
		baseX := chunk.X * config.ChunkWidth
		baseZ := chunk.Z * config.ChunkWidth

		for lx := range config.ChunkWidth {
			for lz := range config.ChunkWidth {
				x := baseX + lx
				z := baseZ + lz

				// Walk down only as far as the first opaque block. Everything
				// below it is shaded, and the clear above already left it at 0,
				// so continuing would write zeros over zeros for the rest of a
				// 256-tall column.
				for y := config.ChunkHeight - 1; y >= 0; y-- {
					if !isTransparent(chunk.GetBlock(lx, y, lz)) {
						break
					}
					chunk.SetSkyLight(lx, y, lz, MaxSkyLight)
					e.addQueue = append(e.addQueue, lightNode{x, y, z, MaxSkyLight})
				}
			}
		}
	}

	e.propagateAdd(skyKind)

	e.addQueue = e.addQueue[:0]

	for _, chunk := range e.w.Chunks {
		baseX := chunk.X * config.ChunkWidth
		baseZ := chunk.Z * config.ChunkWidth

		// Unlike the skylight scan above, an emitter can sit anywhere in the
		// column, not just under the open sky, so the whole volume has to be
		// visited: there is no shortcut analogous to stopping at the first
		// opaque block.
		for lx := range config.ChunkWidth {
			for lz := range config.ChunkWidth {
				for y := range config.ChunkHeight {
					blk := chunk.GetBlock(lx, y, lz)
					if blk == nil || blk.LightLevel == 0 {
						continue
					}

					x := baseX + lx
					z := baseZ + lz
					chunk.SetBlockLight(lx, y, lz, blk.LightLevel)
					e.addQueue = append(e.addQueue, lightNode{x, y, z, blk.LightLevel})
				}
			}
		}
	}

	e.propagateAdd(blockKind)
}

// OnBlockChanged updates the lighting after the block at the given global
// coordinates changed. Callers are expected to have already written the new
// block into the world before calling this.
func (e *Engine) OnBlockChanged(x, y, z int) {
	block := e.w.GetBlock(x, y, z)

	e.updateSkyLight(x, y, z, block)
	e.updateBlockLight(x, y, z, block)
}

// updateSkyLight applies the skylight half of OnBlockChanged: an opaque
// block darkens whatever skylight it used to hold, propagating the removal
// outward; a transparent block picks up light from the open sky above (if
// any) and from every already-lit neighbour.
func (e *Engine) updateSkyLight(x, y, z int, block *blocks.Block) {
	if !isTransparent(block) {
		old := e.w.GetSkyLight(x, y, z)
		if old == 0 {
			return
		}
		e.removeQueue = e.removeQueue[:0]
		e.reAdd = e.reAdd[:0]
		e.setLight(skyKind, x, y, z, 0)
		e.removeQueue = append(e.removeQueue, lightNode{x, y, z, old})
		e.runRemove(skyKind)
		return
	}

	e.addQueue = e.addQueue[:0]

	if y == config.ChunkHeight-1 {
		e.setLight(skyKind, x, y, z, MaxSkyLight)
		e.addQueue = append(e.addQueue, lightNode{x, y, z, MaxSkyLight})
	}

	for _, d := range directions {
		nx, ny, nz := x+d.DX, y+d.DY, z+d.DZ
		if ny < 0 || ny >= config.ChunkHeight {
			continue
		}
		level := e.w.GetSkyLight(nx, ny, nz)
		if level > 0 {
			e.addQueue = append(e.addQueue, lightNode{nx, ny, nz, level})
		}
	}

	e.propagateAdd(skyKind)
}

// updateBlockLight applies the block-light half of OnBlockChanged. It first
// removes whatever the previous occupant of the cell was contributing to its
// own cell (this also covers one emitter being replaced directly by
// another: the old contribution is fully unwound before the new one is
// seeded). If the new block is itself an emitter, its cell is seeded at its
// LightLevel. If the cell is transparent, every already-lit neighbour is
// re-enqueued so light can flow into the newly opened space. Both cases then
// share one propagation.
func (e *Engine) updateBlockLight(x, y, z int, block *blocks.Block) {
	old := e.w.GetBlockLight(x, y, z)
	if old > 0 {
		e.removeQueue = e.removeQueue[:0]
		e.reAdd = e.reAdd[:0]
		e.setLight(blockKind, x, y, z, 0)
		e.removeQueue = append(e.removeQueue, lightNode{x, y, z, old})
		e.runRemove(blockKind)
	}

	e.addQueue = e.addQueue[:0]

	if block != nil && block.LightLevel > 0 {
		e.setLight(blockKind, x, y, z, block.LightLevel)
		e.addQueue = append(e.addQueue, lightNode{x, y, z, block.LightLevel})
	}

	if isTransparent(block) {
		for _, d := range directions {
			nx, ny, nz := x+d.DX, y+d.DY, z+d.DZ
			if ny < 0 || ny >= config.ChunkHeight {
				continue
			}
			level := e.w.GetBlockLight(nx, ny, nz)
			if level > 0 {
				e.addQueue = append(e.addQueue, lightNode{nx, ny, nz, level})
			}
		}
	}

	e.propagateAdd(blockKind)
}

// SeedChunk lights a chunk that has just been added to the world, and lets
// light flow between it and its already-lit neighbours.
//
// M3 deleted the previous SeedChunk because it was order-dependent: it wiped
// its own chunk before reseeding from neighbour borders, so seeding chunk B
// destroyed light chunk A had already pushed into it, and recovery depended
// on which chunk was seeded next - in Go, map iteration order.
//
// This version has no wipe, and that is not a simplification so much as the
// whole argument for why it is safe to bring back. Adding a chunk can only
// ever add light, never remove it: World.GetSkyLight's doc comment spells out
// that an unloaded chunk reads as opaque, so before this chunk arrived its
// neighbours were necessarily lit as though every cell in it were solid rock.
// Air appearing where the engine had assumed rock can only open new paths for
// light that is already there - it can never invalidate a value the
// neighbours already hold, because that value never depended on this chunk in
// the first place. So there is nothing to remove, nothing to wipe, and an
// add-only propagation converges to the same answer no matter which chunk in
// a batch is seeded first. That is precisely the property the old
// implementation lacked.
//
// SeedChunk seeds the new chunk's own skylight column scan and emitting
// blocks exactly as RecomputeAll does, enqueues every already-lit cell on a
// loaded neighbour's border that faces this chunk, and runs the ordinary add
// walk. It is a no-op if coord is not loaded.
func (e *Engine) SeedChunk(coord world.ChunkCoord) {
	chunk := e.w.GetChunk(coord.X, coord.Z)
	if chunk == nil {
		return
	}

	e.seedSkyLight(coord, chunk)
	e.seedBlockLight(coord, chunk)
}

// seedSkyLight is the skylight half of SeedChunk: a top-down column scan of
// the new chunk identical to RecomputeAll's, plus every already-lit skylight
// cell on a neighbouring chunk's border that can still shed light across the
// seam, then one propagation.
func (e *Engine) seedSkyLight(coord world.ChunkCoord, chunk *world.Chunk) {
	e.addQueue = e.addQueue[:0]

	baseX := coord.X * config.ChunkWidth
	baseZ := coord.Z * config.ChunkWidth

	for lx := range config.ChunkWidth {
		for lz := range config.ChunkWidth {
			x := baseX + lx
			z := baseZ + lz

			// Walk down only as far as the first opaque block, exactly like
			// RecomputeAll: everything below it is shaded and already 0 in a
			// fresh chunk.
			for y := config.ChunkHeight - 1; y >= 0; y-- {
				if !isTransparent(chunk.GetBlock(lx, y, lz)) {
					break
				}
				e.setLight(skyKind, x, y, z, MaxSkyLight)
				e.addQueue = append(e.addQueue, lightNode{x, y, z, MaxSkyLight})
			}
		}
	}

	e.enqueueNeighbourBorders(coord, skyKind)
	e.propagateAdd(skyKind)
}

// seedBlockLight is the block-light half of SeedChunk: a full-volume scan of
// the new chunk for emitting blocks identical to RecomputeAll's, plus every
// already-lit block-light cell on a neighbouring chunk's border that can
// still shed light across the seam, then one propagation.
func (e *Engine) seedBlockLight(coord world.ChunkCoord, chunk *world.Chunk) {
	e.addQueue = e.addQueue[:0]

	baseX := coord.X * config.ChunkWidth
	baseZ := coord.Z * config.ChunkWidth

	// Unlike the skylight scan, an emitter can sit anywhere in the column, so
	// the whole volume has to be visited.
	for lx := range config.ChunkWidth {
		for lz := range config.ChunkWidth {
			for y := range config.ChunkHeight {
				blk := chunk.GetBlock(lx, y, lz)
				if blk == nil || blk.LightLevel == 0 {
					continue
				}

				x := baseX + lx
				z := baseZ + lz
				e.setLight(blockKind, x, y, z, blk.LightLevel)
				e.addQueue = append(e.addQueue, lightNode{x, y, z, blk.LightLevel})
			}
		}
	}

	e.enqueueNeighbourBorders(coord, blockKind)
	e.propagateAdd(blockKind)
}

// enqueueNeighbourBorders appends, for every already-loaded chunk that
// borders coord, the cells on its edge facing coord whose light of the given
// kind can still cross the seam into the new chunk. Chunks are neighbours
// only in the four horizontal directions, so the two vertical entries in
// directions are skipped.
//
// A cell is worth enqueuing only if kind.expected reports a non-zero level
// for the step back into the new chunk. That excludes an unlit border cell
// for free - decayByOne(0) is 0 - and also a cell too dim to survive one more
// step, which would only write a 0 into a new chunk that already reads 0
// everywhere. propagateAdd would discard such a node on its first iteration
// anyway; filtering here just means the walk never has to look at it.
func (e *Engine) enqueueNeighbourBorders(coord world.ChunkCoord, kind lightKind) {
	baseX := coord.X * config.ChunkWidth
	baseZ := coord.Z * config.ChunkWidth

	for _, d := range directions {
		if d.DY != 0 {
			continue
		}
		if e.w.GetChunk(coord.X+d.DX, coord.Z+d.DZ) == nil {
			continue
		}

		// into is the direction light travels crossing the seam from the
		// neighbour's border cell back into the new chunk - the reverse of d,
		// which points from the new chunk out to the neighbour.
		into := direction{DX: -d.DX, DY: -d.DY, DZ: -d.DZ}

		for a := range config.ChunkWidth {
			var x, z int
			switch {
			case d.DX == 1:
				x, z = baseX+config.ChunkWidth-1, baseZ+a
			case d.DX == -1:
				x, z = baseX, baseZ+a
			case d.DZ == 1:
				x, z = baseX+a, baseZ+config.ChunkWidth-1
			default: // d.DZ == -1
				x, z = baseX+a, baseZ
			}

			// The neighbour's cell just across the seam from (x, z).
			nx, nz := x+d.DX, z+d.DZ

			for y := range config.ChunkHeight {
				level := kind.get(e.w, nx, y, nz)
				if kind.expected(level, into) == 0 {
					continue
				}
				e.addQueue = append(e.addQueue, lightNode{nx, y, nz, level})
			}
		}
	}
}

// DirtyChunks returns the chunks whose meshes the light changes since the last
// call have invalidated, and clears the set.
//
// This is deliberately "meshes invalidated" rather than "light changed". A
// block is lit by the cells around it, so a change on a chunk border also
// invalidates the neighbouring chunk even though no light inside it changed —
// see markMeshDirty. Reporting only the chunks that changed leaves walls
// standing on seams rendering stale darkness.
func (e *Engine) DirtyChunks() []world.ChunkCoord {
	if len(e.dirty) == 0 {
		return nil
	}
	out := make([]world.ChunkCoord, 0, len(e.dirty))
	for c := range e.dirty {
		out = append(out, c)
		delete(e.dirty, c)
	}
	return out
}

// propagateAdd drains e.addQueue, propagating light of the given kind
// outward from every queued cell according to kind.expected.
func (e *Engine) propagateAdd(kind lightKind) {
	for head := 0; head < len(e.addQueue); head++ {
		node := e.addQueue[head]
		if node.Level == 0 {
			continue
		}

		for _, d := range directions {
			nx, ny, nz := node.X+d.DX, node.Y+d.DY, node.Z+d.DZ
			if ny < 0 || ny >= config.ChunkHeight {
				continue
			}
			if !e.w.HasChunkAt(nx, nz) {
				continue
			}

			want := kind.expected(node.Level, d)
			if want <= kind.get(e.w, nx, ny, nz) {
				continue
			}
			if !isTransparent(e.w.GetBlock(nx, ny, nz)) {
				continue
			}

			e.setLight(kind, nx, ny, nz, want)
			e.addQueue = append(e.addQueue, lightNode{nx, ny, nz, want})
		}
	}
	e.addQueue = e.addQueue[:0]
}

// runRemove drains e.removeQueue for the given kind, darkening cells that
// were lit solely by the light being removed, then re-propagates from every
// cell found to have an independent source (e.reAdd).
func (e *Engine) runRemove(kind lightKind) {
	for head := 0; head < len(e.removeQueue); head++ {
		node := e.removeQueue[head]

		for _, d := range directions {
			nx, ny, nz := node.X+d.DX, node.Y+d.DY, node.Z+d.DZ
			if ny < 0 || ny >= config.ChunkHeight {
				continue
			}
			if !e.w.HasChunkAt(nx, nz) {
				continue
			}
			if !isTransparent(e.w.GetBlock(nx, ny, nz)) {
				continue
			}

			nl := kind.get(e.w, nx, ny, nz)
			if nl == 0 {
				continue
			}

			// The textbook removal test is nl < level: a neighbour dimmer
			// than the level we are erasing was necessarily lit by us, so
			// it also gets erased. That test is wrong here because
			// skylight falls straight down losslessly: the cell directly
			// beneath a removed level-15 source is itself 15, so nl < level
			// is false there, and the textbook test would wrongly treat it
			// as an independent source, leaving the whole column lit under
			// a freshly placed block. Comparing against kind.expected(level, d)
			// instead correctly recognizes that the cell below was lit BY
			// this one even though it holds the same level. Block light has
			// no lossless direction, so for it this reduces to the textbook
			// nl == level-1 case - the same comparison still applies because
			// it is phrased in terms of kind.expected rather than hard-coded
			// to skylight's rule.
			if nl == kind.expected(node.Level, d) {
				e.setLight(kind, nx, ny, nz, 0)
				e.removeQueue = append(e.removeQueue, lightNode{nx, ny, nz, nl})
			} else {
				e.reAdd = append(e.reAdd, lightNode{nx, ny, nz, nl})
			}
		}
	}

	e.addQueue = e.addQueue[:0]
	e.addQueue = append(e.addQueue, e.reAdd...)
	e.propagateAdd(kind)
}

// setLight routes every light write of the given kind through one place: it
// is a no-op if the chunk is not loaded or the value is unchanged, and
// otherwise writes the new level and records the owning chunk as dirty.
func (e *Engine) setLight(kind lightKind, x, y, z int, level uint8) {
	if !e.w.HasChunkAt(x, z) {
		return
	}
	if kind.get(e.w, x, y, z) == level {
		return
	}
	kind.set(e.w, x, y, z, level)

	e.markMeshDirty(x, z)
}

// markMeshDirty records every loaded chunk whose mesh depends on the light in
// the global column (x, z).
//
// That is not only the chunk containing the cell. A block's faces are lit from
// the cells around it — since smooth lighting, from a 2x2 patch per corner, so
// up to one step away including diagonally — which means a block just across a
// chunk seam is lit by this cell. A change on a border therefore invalidates
// the neighbour's mesh even though no light inside it changed.
//
// A solid wall standing on a seam is the case that makes this visible: light a
// room on one side and the wall's own chunk contains no changed cell at all,
// because every cell in it is rock sitting at 0. Marking only the changed
// cell's chunk leaves that wall rendering its old darkness until something
// else happens to re-mesh it.
func (e *Engine) markMeshDirty(x, z int) {
	cx, lx := world.ChunkAndLocal(x)
	cz, lz := world.ChunkAndLocal(z)

	e.dirty[world.ChunkCoord{X: cx, Z: cz}] = struct{}{}

	dxs, nx := borderSpan(lx)
	dzs, nz := borderSpan(lz)
	if nx == 1 && nz == 1 {
		// Interior cell: no other chunk renders it. This is the overwhelming
		// majority of writes, and it costs the same single map insert as before.
		return
	}

	for i := range nx {
		for j := range nz {
			dx, dz := dxs[i], dzs[j]
			if dx == 0 && dz == 0 {
				continue
			}
			if e.w.GetChunk(cx+dx, cz+dz) == nil {
				continue
			}
			e.dirty[world.ChunkCoord{X: cx + dx, Z: cz + dz}] = struct{}{}
		}
	}
}

// borderSpan returns the chunk offsets a local coordinate can be seen from.
// An interior coordinate is only rendered by its own chunk; the two edge
// coordinates are also rendered by the neighbour on that side.
//
// It returns an array rather than a slice because it is called on every light
// write, and a slice would allocate in the BFS inner loop.
func borderSpan(local int) (offsets [2]int, n int) {
	switch local {
	case 0:
		return [2]int{0, -1}, 2
	case config.ChunkWidth - 1:
		return [2]int{0, 1}, 2
	default:
		return [2]int{0, 0}, 1
	}
}

// markChunksDirty marks every loaded chunk dirty. Used by RecomputeAll,
// which rewrites every loaded chunk's skylight and block light.
func (e *Engine) markChunksDirty() {
	for coord := range e.w.Chunks {
		e.dirty[coord] = struct{}{}
	}
}
