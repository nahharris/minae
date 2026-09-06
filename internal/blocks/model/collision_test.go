package model

import (
	"fmt"
	"testing"
)

// A full block and a sided block are both solid unit cubes; a sided block
// only varies which texture goes on which face, so its collision shape must
// be identical to FullBlock's.
func TestFullAndSidedBlockCollisionBoxes(t *testing.T) {
	t.Parallel()

	want := Box{Min: Vec3{0, 0, 0}, Max: Vec3{1, 1, 1}}

	models := map[string]BlockModel{
		"full":  NewFullBlock([6]string{}),
		"sided": NewSidedBlock("minae/test", "", "", "", ""),
	}

	for name, m := range models {
		t.Run(name, func(t *testing.T) {
			boxes := m.CollisionBoxes(nil, 0)
			if len(boxes) != 1 {
				t.Fatalf("CollisionBoxes returned %d boxes, want 1", len(boxes))
			}
			if boxes[0] != want {
				t.Errorf("CollisionBoxes = %+v, want %+v", boxes[0], want)
			}
		})
	}
}

// CollisionBoxes must append to the caller's slice rather than replacing it.
func TestFullBlockCollisionBoxes_Appends(t *testing.T) {
	t.Parallel()

	m := NewFullBlock([6]string{})
	existing := Box{Min: Vec3{5, 5, 5}, Max: Vec3{6, 6, 6}}
	dst := []Box{existing}

	got := m.CollisionBoxes(dst, 0)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != existing {
		t.Errorf("got[0] = %+v, want existing entry %+v preserved", got[0], existing)
	}
}

// A bottom slab occupies y=0..0.5, sitting on the bottom of the voxel; a top
// slab occupies y=0.5..1, sitting on the top. This asserts the actual Y
// values, not merely that a box exists.
func TestSlabBlockCollisionBoxes(t *testing.T) {
	t.Parallel()

	m := NewSlabBlock([6]string{})

	tests := []struct {
		name string
		meta uint8
		want Box
	}{
		{"bottom", 0, Box{Min: Vec3{0, 0, 0}, Max: Vec3{1, 0.5, 1}}},
		{"top", MetaSlabTopBit, Box{Min: Vec3{0, 0.5, 0}, Max: Vec3{1, 1, 1}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boxes := m.CollisionBoxes(nil, tt.meta)
			if len(boxes) != 1 {
				t.Fatalf("CollisionBoxes returned %d boxes, want 1", len(boxes))
			}
			if boxes[0] != tt.want {
				t.Errorf("CollisionBoxes(meta=%#b) = %+v, want %+v", tt.meta, boxes[0], tt.want)
			}
		})
	}
}

// Orientable must delegate to the wrapped model unchanged: every shape that
// exists today is symmetric under the 4-way Y rotation it applies, so the
// same box is correct at every facing.
func TestOrientableCollisionBoxes_SameAtEveryFacing(t *testing.T) {
	t.Parallel()

	base := NewSlabBlock([6]string{})
	orientable := NewOrientable(base)

	want := base.CollisionBoxes(nil, 0)

	for facing := uint8(0); facing < 4; facing++ {
		t.Run(fmt.Sprintf("facing_%d", facing), func(t *testing.T) {
			got := orientable.CollisionBoxes(nil, facing)
			if len(got) != len(want) {
				t.Fatalf("facing %d: len(got) = %d, want %d", facing, len(got), len(want))
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("facing %d: got[%d] = %+v, want %+v", facing, i, got[i], want[i])
				}
			}
		})
	}
}
