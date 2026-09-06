// Package world_test exercises World.CollisionBoxes as an external
// (black-box) test.
//
// It cannot live in package world: internal/testutil imports internal/world
// to build test fixtures, so a same-package test file importing testutil
// would create an import cycle (world -> testutil -> world). Every symbol
// these tests need is already exported, so there is nothing white-box access
// buys here.
package world_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/blocks/model"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// A full block yields exactly one unit box at the right world position,
// including at negative coordinates, where floor division is a known trap.
func TestWorldCollisionBoxes_FullBlock(t *testing.T) {

	tests := []struct {
		name    string
		x, y, z int
	}{
		{"positive", 5, 10, 3},
		{"negative x and z", -5, 10, -3},
		{"negative across chunk seam", -17, 4, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := testutil.NewWorld(t).
				Chunks(-2, -2, 2, 2).
				Fill(testutil.Box{MinX: tt.x, MinY: tt.y, MinZ: tt.z, MaxX: tt.x, MaxY: tt.y, MaxZ: tt.z}, blocks.Stone).
				Build()

			got := w.CollisionBoxes(nil, tt.x, tt.y, tt.z)
			if len(got) != 1 {
				t.Fatalf("CollisionBoxes returned %d boxes, want 1", len(got))
			}

			fx, fy, fz := float32(tt.x), float32(tt.y), float32(tt.z)
			if got[0].Min.X != fx || got[0].Min.Y != fy || got[0].Min.Z != fz {
				t.Errorf("Min = %+v, want (%v, %v, %v)", got[0].Min, fx, fy, fz)
			}
			if got[0].Max.X != fx+1 || got[0].Max.Y != fy+1 || got[0].Max.Z != fz+1 {
				t.Errorf("Max = %+v, want (%v, %v, %v)", got[0].Max, fx+1, fy+1, fz+1)
			}
		})
	}
}

// A bottom slab yields a box of height 0.5 sitting on the bottom of the
// voxel; a top slab yields one sitting on the top. The actual Y values are
// asserted, not just that a box exists.
func TestWorldCollisionBoxes_Slab(t *testing.T) {

	const x, y, z = 4, 8, 4

	tests := []struct {
		name     string
		meta     uint8
		wantMinY float32
		wantMaxY float32
	}{
		{"bottom", 0, 0, 0.5},
		{"top", model.MetaSlabTopBit, 0.5, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Build()
			w.SetBlockState(x, y, z, blocks.StoneSlab, tt.meta)

			got := w.CollisionBoxes(nil, x, y, z)
			if len(got) != 1 {
				t.Fatalf("CollisionBoxes returned %d boxes, want 1", len(got))
			}

			wantMinY := float32(y) + tt.wantMinY
			wantMaxY := float32(y) + tt.wantMaxY
			if got[0].Min.Y != wantMinY {
				t.Errorf("Min.Y = %v, want %v", got[0].Min.Y, wantMinY)
			}
			if got[0].Max.Y != wantMaxY {
				t.Errorf("Max.Y = %v, want %v", got[0].Max.Y, wantMaxY)
			}
			// Horizontal extent is unaffected by the slab's Y split.
			if got[0].Min.X != x || got[0].Max.X != x+1 || got[0].Min.Z != z || got[0].Max.Z != z+1 {
				t.Errorf("horizontal extent = %+v, want full voxel width", got[0])
			}
		})
	}
}

// Air, an unloaded chunk, and a y outside 0..config.ChunkHeight-1 all
// contribute nothing.
func TestWorldCollisionBoxes_ContributesNothing(t *testing.T) {

	w := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Build()
	// Position (0,0,0) is left as air by default.

	tests := []struct {
		name    string
		x, y, z int
	}{
		{"air", 0, 0, 0},
		{"unloaded chunk", 1000, 0, 1000},
		{"y below range", 0, -1, 0},
		{"y above range", 0, config.ChunkHeight, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := w.CollisionBoxes(nil, tt.x, tt.y, tt.z)
			if len(got) != 0 {
				t.Errorf("CollisionBoxes(%d,%d,%d) returned %d boxes, want 0", tt.x, tt.y, tt.z, len(got))
			}
		})
	}
}

// CollisionBoxes must append to the caller's slice rather than replacing it.
func TestWorldCollisionBoxes_Appends(t *testing.T) {

	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 1, MinY: 1, MinZ: 1, MaxX: 1, MaxY: 1, MaxZ: 1}, blocks.Stone).
		Build()

	existing := physics.AABB{
		Min: core.Vec3{X: 99, Y: 99, Z: 99},
		Max: core.Vec3{X: 100, Y: 100, Z: 100},
	}
	dst := []physics.AABB{existing}

	got := w.CollisionBoxes(dst, 1, 1, 1)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != existing {
		t.Errorf("got[0] = %+v, want the pre-existing entry %+v preserved", got[0], existing)
	}
}

// *World satisfies physics.Grid: the resolver only needs a value that
// implements the interface, and this pins the compile-time check that it
// keeps doing so.
func TestWorld_ImplementsPhysicsGrid(t *testing.T) {

	var _ physics.Grid = (*world.World)(nil)
}
