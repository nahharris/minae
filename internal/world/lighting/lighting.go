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

// DirtyChunks returns the chunks whose light changed since the last call,
// and clears the set.
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

	cx, _ := world.ChunkAndLocal(x)
	cz, _ := world.ChunkAndLocal(z)
	e.dirty[world.ChunkCoord{X: cx, Z: cz}] = struct{}{}
}

// markChunksDirty marks every loaded chunk dirty. Used by RecomputeAll,
// which rewrites every loaded chunk's skylight and block light.
func (e *Engine) markChunksDirty() {
	for coord := range e.w.Chunks {
		e.dirty[coord] = struct{}{}
	}
}
