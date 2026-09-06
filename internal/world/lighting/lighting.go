// Package lighting computes and incrementally maintains chunk skylight.
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

// isTransparent reports whether a block lets skylight pass through it. Air
// always reads back as nil from World.GetBlock and Chunk.GetBlock, so this is
// the one place to extend when translucent blocks such as glass or leaves are
// added.
func isTransparent(b *blocks.Block) bool {
	return b == nil
}

// expected returns the skylight level a neighbour reached by moving from a
// cell at level from in direction d should hold. Skylight falls straight
// down at full strength; it decays by one in every other direction,
// including diagonally-adjacent-but-still-vertical cases, since d is always
// one of the six axis directions.
func expected(from uint8, d direction) uint8 {
	if d.isDown() && from == MaxSkyLight {
		return MaxSkyLight
	}
	if from == 0 {
		return 0
	}
	return from - 1
}

// Engine incrementally maintains the skylight of every loaded chunk in a
// World. Create one with NewEngine.
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

// RecomputeAll rebuilds the skylight of every loaded chunk from scratch. It
// is order-independent: every loaded chunk is zeroed first, then a single
// top-down column scan runs over every chunk collecting into one queue, and
// only then does that queue propagate. Seeding and propagating chunk by
// chunk would make the result depend on map iteration order.
func (e *Engine) RecomputeAll() {
	for _, chunk := range e.w.Chunks {
		// clear compiles to a single memclr. The equivalent element-by-element
		// loop is 65536 separate writes per chunk, which the race detector and
		// the coverage counters both instrument individually — enough to
		// dominate the test suite's runtime.
		clear(chunk.SkyLight[:])
	}

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

	e.markChunksDirty()
	e.propagateAdd()
}

// OnBlockChanged updates the lighting after the block at the given global
// coordinates changed. Callers are expected to have already written the new
// block into the world before calling this.
func (e *Engine) OnBlockChanged(x, y, z int) {
	block := e.w.GetBlock(x, y, z)

	if !isTransparent(block) {
		old := e.w.GetSkyLight(x, y, z)
		if old == 0 {
			return
		}
		e.removeQueue = e.removeQueue[:0]
		e.reAdd = e.reAdd[:0]
		e.setSkyLight(x, y, z, 0)
		e.removeQueue = append(e.removeQueue, lightNode{x, y, z, old})
		e.runRemove()
		return
	}

	e.addQueue = e.addQueue[:0]

	if y == config.ChunkHeight-1 {
		e.setSkyLight(x, y, z, MaxSkyLight)
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

	e.propagateAdd()
}

// DirtyChunks returns the chunks whose skylight changed since the last call,
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

// propagateAdd drains e.addQueue, propagating light outward from every
// queued cell according to expected.
func (e *Engine) propagateAdd() {
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

			want := expected(node.Level, d)
			if want <= e.w.GetSkyLight(nx, ny, nz) {
				continue
			}
			if !isTransparent(e.w.GetBlock(nx, ny, nz)) {
				continue
			}

			e.setSkyLight(nx, ny, nz, want)
			e.addQueue = append(e.addQueue, lightNode{nx, ny, nz, want})
		}
	}
	e.addQueue = e.addQueue[:0]
}

// runRemove drains e.removeQueue, darkening cells that were lit solely by
// the light being removed, then re-propagates from every cell found to have
// an independent source (e.reAdd).
func (e *Engine) runRemove() {
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

			nl := e.w.GetSkyLight(nx, ny, nz)
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
			// a freshly placed block. Comparing against expected(level, d)
			// instead correctly recognizes that the cell below was lit BY
			// this one even though it holds the same level.
			if nl == expected(node.Level, d) {
				e.setSkyLight(nx, ny, nz, 0)
				e.removeQueue = append(e.removeQueue, lightNode{nx, ny, nz, nl})
			} else {
				e.reAdd = append(e.reAdd, lightNode{nx, ny, nz, nl})
			}
		}
	}

	e.addQueue = e.addQueue[:0]
	e.addQueue = append(e.addQueue, e.reAdd...)
	e.propagateAdd()
}

// setSkyLight routes every skylight write through one place: it is a no-op
// if the chunk is not loaded or the value is unchanged, and otherwise writes
// the new level and records the owning chunk as dirty.
func (e *Engine) setSkyLight(x, y, z int, level uint8) {
	if !e.w.HasChunkAt(x, z) {
		return
	}
	if e.w.GetSkyLight(x, y, z) == level {
		return
	}
	e.w.SetSkyLight(x, y, z, level)

	cx, _ := world.ChunkAndLocal(x)
	cz, _ := world.ChunkAndLocal(z)
	e.dirty[world.ChunkCoord{X: cx, Z: cz}] = struct{}{}
}

// markChunksDirty marks every loaded chunk dirty. Used by RecomputeAll,
// which rewrites every loaded chunk's skylight.
func (e *Engine) markChunksDirty() {
	for coord := range e.w.Chunks {
		e.dirty[coord] = struct{}{}
	}
}
