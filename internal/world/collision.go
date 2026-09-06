package world

import (
	"github.com/nahharris/minae/internal/blocks/model"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/platform/config"
)

// compile-time check that *World implements physics.Grid.
var _ physics.Grid = (*World)(nil)

// CollisionBoxes appends the solid boxes of the block at the given global
// block coordinates, translated into world space, to dst and returns it. It
// implements physics.Grid.
//
// Air, a y outside 0..config.ChunkHeight-1, and an unloaded chunk all
// contribute nothing. Treating a missing chunk as empty rather than solid is
// deliberate, and it is the opposite of the light engine's treatment of
// unloaded chunks (see GetSkyLight): the light engine must treat missing data
// as opaque so light never appears to leak in from nowhere, but collision
// doing the same would stop the player at an invisible wall the moment they
// reached the edge of the loaded world. Falling through ungenerated terrain
// for an instant is the lesser problem, so here a missing chunk is air, not
// rock.
//
// This does not allocate once its internal scratch buffer has warmed up:
// CollisionBoxes is called once per axis per substep, per block coordinate,
// from the physics resolver's inner loop. It is not safe for concurrent use
// on the same *World, matching the resolver's own single-body-at-a-time use.
func (w *World) CollisionBoxes(dst []physics.AABB, x, y, z int) []physics.AABB {
	if y < 0 || y >= config.ChunkHeight {
		return dst
	}

	block, meta := w.GetBlockState(x, y, z)
	if block == nil {
		return dst
	}

	blockModel := block.Model
	if blockModel == nil {
		blockModel = model.CompileModel(block.ID, block.ModelSpec)
	}

	// A local scratch slice rather than one cached on World. Caching it would
	// save a small allocation per call, but it would put mutable state on the
	// single object the renderer, the light engine, the UI and the game loop all
	// share -- a poor trade for an allocation this size, and a trap the first
	// time anything calls this off the main thread.
	var scratch []model.Box
	scratch = blockModel.CollisionBoxes(scratch, meta)

	fx, fy, fz := float32(x), float32(y), float32(z)
	for _, box := range scratch {
		dst = append(dst, physics.AABB{
			Min: core.Vec3{X: fx + box.Min.X, Y: fy + box.Min.Y, Z: fz + box.Min.Z},
			Max: core.Vec3{X: fx + box.Max.X, Y: fy + box.Max.Y, Z: fz + box.Max.Z},
		})
	}
	return dst
}
