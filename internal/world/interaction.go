package world

import (
	"math"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/blocks/model"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/platform/config"
)

// InteractionAction defines the type of interaction.
type InteractionAction int

const (
	ActionNone InteractionAction = iota
	ActionBreak
	ActionPlace
)

// InteractionResult holds the outcome of an interaction.
type InteractionResult struct {
	AffectedChunks []ChunkCoord
	TargetBlock    [3]int
	HasTarget      bool

	// ChangedBlock is the position whose block actually changed, which is not
	// the same as TargetBlock: breaking changes the targeted block, placing
	// changes the empty cell next to it. The lighting engine needs the position
	// that changed, so it cannot use TargetBlock.
	ChangedBlock [3]int
	Changed      bool
}

func raycastTarget(w *World, cameraPos, cameraDir core.Vec3) (hit bool, pos [3]int, face [3]int) {
	hit, pos, face, _ = w.Raycast(cameraPos, cameraDir, config.Current.PlayerArmLength)
	return
}

// placingInsidePlayer reports whether the block cell at placePos overlaps the
// player's body box. It is an overlap test rather than a single-cell equality
// check because the body spans multiple vertical cells (feet, head, and
// anything between), and a check against only one of them would let a block
// be placed inside another. Boxes that only touch at a shared face do not
// overlap, matching internal/physics's own definition of overlap.
func placingInsidePlayer(playerBox physics.AABB, placePos [3]int) bool {
	fx, fy, fz := float32(placePos[0]), float32(placePos[1]), float32(placePos[2])
	blockBox := physics.AABB{
		Min: core.Vec3{X: fx, Y: fy, Z: fz},
		Max: core.Vec3{X: fx + 1, Y: fy + 1, Z: fz + 1},
	}
	return boxesOverlap(playerBox, blockBox)
}

// boxesOverlap reports whether a and b intersect with positive volume. Boxes
// that only touch at a shared face do not overlap. This mirrors the overlap
// test internal/physics uses internally, kept here rather than exported from
// physics so that package's surface stays focused on the resolver.
func boxesOverlap(a, b physics.AABB) bool {
	return a.Min.X < b.Max.X && a.Max.X > b.Min.X &&
		a.Min.Y < b.Max.Y && a.Max.Y > b.Min.Y &&
		a.Min.Z < b.Max.Z && a.Max.Z > b.Min.Z
}

func applySlabMeta(base uint8, block *blocks.Block, face [3]int) uint8 {
	if block.ModelSpec.Type != "slab" {
		return base
	}
	if face[1] == -1 {
		return base | model.MetaSlabTopBit
	}
	return base
}

func applyOrientableMeta(base uint8, block *blocks.Block, viewDir core.Vec3) uint8 {
	if !block.ModelSpec.Orientable {
		return base
	}

	view := viewDir
	view.Y = 0
	view.X = -view.X
	view.Z = -view.Z

	if view.Length() > 0 {
		view = view.Normalize()
	}

	ax := float32(math.Abs(float64(view.X)))
	az := float32(math.Abs(float64(view.Z)))

	var facing uint8 // 0:+Z, 1:+X, 2:-Z, 3:-X
	if az >= ax {
		if view.Z >= 0 {
			facing = 0
		} else {
			facing = 2
		}
	} else {
		if view.X >= 0 {
			facing = 1
		} else {
			facing = 3
		}
	}

	return (base &^ model.MetaFacingMask) | (facing & model.MetaFacingMask)
}

// ProcessBlockInteraction handles block breaking and placing logic.
//
// cameraPos and cameraDir originate at the eye, not the feet: the targeting
// ray must still come from where the player is looking. playerBox is the
// player's body box, used only to keep a placement from landing inside the
// player -- a separate concern from where the ray starts.
func ProcessBlockInteraction(
	w *World,
	cameraPos, cameraDir core.Vec3,
	playerBox physics.AABB,
	action InteractionAction,
	blockToPlace *blocks.Block,
	placeMeta uint8,
) InteractionResult {
	result := InteractionResult{}

	// Raycast to find target
	hit, pos, face := raycastTarget(w, cameraPos, cameraDir)
	result.HasTarget = hit
	if hit {
		result.TargetBlock = pos
	} else {
		return result
	}

	if action == ActionNone {
		return result
	}

	if action == ActionBreak {
		// Break block
		result.AffectedChunks = w.SetBlock(pos[0], pos[1], pos[2], blocks.Air)
		result.ChangedBlock = pos
		result.Changed = len(result.AffectedChunks) > 0
		return result
	}

	if action == ActionPlace && blockToPlace != nil {
		// Place block
		placePos := [3]int{pos[0] + face[0], pos[1] + face[1], pos[2] + face[2]}

		// Don't place inside the player.
		if placingInsidePlayer(playerBox, placePos) {
			return result
		}

		// Calculate placement metadata (orientation, slabs, etc)
		meta := applySlabMeta(placeMeta, blockToPlace, face)
		meta = applyOrientableMeta(meta, blockToPlace, cameraDir)

		result.AffectedChunks = w.SetBlockState(placePos[0], placePos[1], placePos[2], blockToPlace, meta)
		result.ChangedBlock = placePos
		result.Changed = len(result.AffectedChunks) > 0
		return result
	}

	return result
}
