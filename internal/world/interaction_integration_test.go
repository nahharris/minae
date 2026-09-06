// Package world_test exercises ProcessBlockInteraction as an external
// (black-box) test, because it needs internal/testutil to build worlds and
// testutil imports internal/world -- a same-package test file here would
// create an import cycle.
package world_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// buildWallWorld places a single stone block at (5, 10, 5) with nothing else
// solid nearby, so a ray fired from (5.5, 10.5, 3) along +Z hits its near
// (-Z) face at distance 1.5, and the empty cell in front of it -- where a
// placement would land -- is (5, 10, 4).
func buildWallWorld(t *testing.T) *world.World {
	t.Helper()
	return testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 5, MinY: 10, MinZ: 5, MaxX: 5, MaxY: 10, MaxZ: 5}, blocks.Stone).
		Build()
}

var (
	wallCameraPos = core.Vec3{X: 5.5, Y: 10.5, Z: 3}
	wallCameraDir = core.Vec3{X: 0, Y: 0, Z: 1}
	wallPlacePos  = [3]int{5, 10, 4}
)

// farAwayBox is a body-sized box nowhere near the wall or the camera. Tests
// that pass it as playerBox and still see the raycast succeed are also proof
// that ProcessBlockInteraction rays from cameraPos, not from playerBox --
// the ray must still originate at the eye, not wherever the body happens to
// be.
var farAwayBox = physics.AABB{
	Min: core.Vec3{X: 50, Y: 50, Z: 50},
	Max: core.Vec3{X: 50.6, Y: 51.8, Z: 50.6},
}

func TestProcessBlockInteraction_PlaceRefusedWhenOverlappingPlayer(t *testing.T) {
	w := buildWallWorld(t)

	// A body box that fully overlaps the cell the placement would land in.
	playerBox := physics.AABB{
		Min: core.Vec3{X: 5, Y: 10, Z: 4},
		Max: core.Vec3{X: 6, Y: 11.8, Z: 5},
	}

	result := world.ProcessBlockInteraction(w, wallCameraPos, wallCameraDir, playerBox, world.ActionPlace, blocks.Dirt, 0)

	if !result.HasTarget {
		t.Fatalf("HasTarget = false, want true (the wall should still be targeted)")
	}
	if result.Changed {
		t.Errorf("Changed = true, want false (placement should be refused)")
	}
	if got := w.GetBlock(wallPlacePos[0], wallPlacePos[1], wallPlacePos[2]); got != nil {
		t.Errorf("block at placement cell = %v, want nil (air, unchanged)", got)
	}
}

func TestProcessBlockInteraction_PlaceAllowedWhenClearOfPlayer(t *testing.T) {
	w := buildWallWorld(t)

	result := world.ProcessBlockInteraction(w, wallCameraPos, wallCameraDir, farAwayBox, world.ActionPlace, blocks.Dirt, 0)

	if !result.Changed {
		t.Fatalf("Changed = false, want true (placement should succeed when clear of the player)")
	}
	if result.ChangedBlock != wallPlacePos {
		t.Errorf("ChangedBlock = %v, want %v", result.ChangedBlock, wallPlacePos)
	}
	if got := w.GetBlock(wallPlacePos[0], wallPlacePos[1], wallPlacePos[2]); got != blocks.Dirt {
		t.Errorf("block at placement cell = %v, want Dirt", got)
	}
}

// TestProcessBlockInteraction_BreakIgnoresPlayerBox checks that the
// player-box overlap guard applies only to placement. Breaking the block you
// are standing in (however that happened) must still work: nothing about
// breaking a block can trap the player, so there is no safety reason to
// restrict it, and restricting it would be an undocumented behaviour change.
func TestProcessBlockInteraction_BreakIgnoresPlayerBox(t *testing.T) {
	w := buildWallWorld(t)

	// A player box that overlaps the target block itself, not just the
	// placement cell.
	playerBox := physics.AABB{
		Min: core.Vec3{X: 5, Y: 10, Z: 5},
		Max: core.Vec3{X: 6, Y: 11.8, Z: 6},
	}

	result := world.ProcessBlockInteraction(w, wallCameraPos, wallCameraDir, playerBox, world.ActionBreak, nil, 0)

	if !result.Changed {
		t.Fatalf("Changed = false, want true (break should succeed regardless of playerBox)")
	}
	if got := w.GetBlock(5, 10, 5); got != nil {
		t.Errorf("target block = %v, want nil (air) after breaking", got)
	}
}

// TestProcessBlockInteraction_NoActionReportsTargetOnly checks that
// ActionNone still reports what the crosshair is over without mutating
// anything, independent of playerBox.
func TestProcessBlockInteraction_NoActionReportsTargetOnly(t *testing.T) {
	w := buildWallWorld(t)

	result := world.ProcessBlockInteraction(w, wallCameraPos, wallCameraDir, farAwayBox, world.ActionNone, nil, 0)

	if !result.HasTarget {
		t.Fatalf("HasTarget = false, want true")
	}
	if result.TargetBlock != [3]int{5, 10, 5} {
		t.Errorf("TargetBlock = %v, want [5 10 5]", result.TargetBlock)
	}
	if result.Changed {
		t.Errorf("Changed = true, want false")
	}
}
