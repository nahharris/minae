// Package world holds white-box tests for the unexported placement-safety
// helpers in interaction.go. These need no *World and so have no reason to
// pull in internal/testutil (which would create an import cycle for a
// same-package test file: testutil imports world).
package world

import (
	"testing"

	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/physics"
)

// TestPlacingInsidePlayer_CoversWholeBody is the direct regression test for
// the M13 latent bug: the old check compared placePos against only the
// camera's single voxel, which guards one cell out of the two or three a
// 1.8-tall body occupies. Each cell the body's box actually overlaps is
// asserted individually -- a check that only covered the feet would pass
// this test's "feet cell" case while still failing "middle cell" and "head
// cell", which is exactly the bug being fixed.
func TestPlacingInsidePlayer_CoversWholeBody(t *testing.T) {
	// Feet at fractional Y = 10.5 so the 1.8-tall box spans three vertical
	// cells: 10 (feet), 11 (the cell between), and 12 (head).
	playerBox := physics.AABB{
		Min: core.Vec3{X: 4.7, Y: 10.5, Z: 4.7},
		Max: core.Vec3{X: 5.3, Y: 12.3, Z: 5.3},
	}

	tests := []struct {
		name string
		pos  [3]int
		want bool
	}{
		{"feet cell", [3]int{5, 10, 5}, true},
		{"middle cell", [3]int{5, 11, 5}, true},
		{"head cell", [3]int{5, 12, 5}, true},
		{"below feet", [3]int{5, 9, 5}, false},
		{"above head", [3]int{5, 13, 5}, false},
		{"beside the body, level with the feet", [3]int{6, 10, 5}, false},
		{"beside the body, level with the head", [3]int{6, 12, 5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := placingInsidePlayer(playerBox, tt.pos); got != tt.want {
				t.Errorf("placingInsidePlayer(%+v, %v) = %v, want %v", playerBox, tt.pos, got, tt.want)
			}
		})
	}
}

// TestPlacingInsidePlayer_TouchingCellIsAllowed checks the boundary: a cell
// that only shares a face with the body, without any volume overlap, is a
// legal placement (e.g. placing a block directly underfoot to stand on).
func TestPlacingInsidePlayer_TouchingCellIsAllowed(t *testing.T) {
	// Feet exactly on an integer boundary, so the cell below only touches.
	playerBox := physics.AABB{
		Min: core.Vec3{X: 4.7, Y: 10, Z: 4.7},
		Max: core.Vec3{X: 5.3, Y: 11.8, Z: 5.3},
	}

	if placingInsidePlayer(playerBox, [3]int{5, 9, 5}) {
		t.Errorf("placingInsidePlayer = true for a cell only touching the body's feet, want false")
	}
}

// TestBoxesOverlap_TouchingFacesDoNotOverlap pins boxesOverlap's overlap
// semantics: two boxes sharing only a face have zero-volume intersection and
// must not be reported as overlapping. This mirrors internal/physics's own
// aabbOverlap, which the M12 resolver's epsilon-gap design depends on.
func TestBoxesOverlap_TouchingFacesDoNotOverlap(t *testing.T) {
	a := physics.AABB{Min: core.Vec3{X: 0, Y: 0, Z: 0}, Max: core.Vec3{X: 1, Y: 1, Z: 1}}
	b := physics.AABB{Min: core.Vec3{X: 1, Y: 0, Z: 0}, Max: core.Vec3{X: 2, Y: 1, Z: 1}}

	if boxesOverlap(a, b) {
		t.Errorf("boxesOverlap(%+v, %+v) = true, want false (boxes only touch at a shared face)", a, b)
	}
}

// TestBoxesOverlap_Overlapping checks the positive case alongside the
// touching-faces negative case above, so a trivial "always false" stub would
// not pass either test.
func TestBoxesOverlap_Overlapping(t *testing.T) {
	a := physics.AABB{Min: core.Vec3{X: 0, Y: 0, Z: 0}, Max: core.Vec3{X: 1, Y: 1, Z: 1}}
	b := physics.AABB{Min: core.Vec3{X: 0.5, Y: 0, Z: 0}, Max: core.Vec3{X: 1.5, Y: 1, Z: 1}}

	if !boxesOverlap(a, b) {
		t.Errorf("boxesOverlap(%+v, %+v) = false, want true", a, b)
	}
}
