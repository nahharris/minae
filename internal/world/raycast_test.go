// Package world_test exercises Raycast as an external (black-box) test.
//
// It cannot live in package world: internal/testutil imports internal/world
// to build test fixtures, so a same-package test file importing testutil
// would create an import cycle (world -> testutil -> world). Every symbol
// these tests need (World, NewWorld's builder via testutil, Raycast, Chunks)
// is already exported, so there is nothing white-box access buys here.
package world_test

import (
	"math"
	"testing"
	"time"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
)

// raycastTimeout bounds every Raycast call made by these tests. A regression
// that reintroduces the -0.0 infinite loop must show up as a test failure,
// not as a hang that only the outer `go test` timeout would catch.
const raycastTimeout = 2 * time.Second

// raycastResult bundles Raycast's four return values so they can travel
// through a channel and be compared with a single ==-style check.
type raycastResult struct {
	hit   bool
	pos   [3]int
	face  [3]int
	block *blocks.Block
}

// raycastSafe runs w.Raycast on a background goroutine and fails the test if
// it does not return within raycastTimeout, instead of hanging the test
// binary. The goroutine is intentionally leaked on timeout: the point is to
// fail fast and loudly, not to clean up after a bug that shouldn't exist.
func raycastSafe(t *testing.T, w *world.World, start, dir core.Vec3, maxDist float32) raycastResult {
	t.Helper()

	done := make(chan raycastResult, 1)
	go func() {
		hit, pos, face, block := w.Raycast(start, dir, maxDist)
		done <- raycastResult{hit, pos, face, block}
	}()

	select {
	case r := <-done:
		return r
	case <-time.After(raycastTimeout):
		t.Fatalf("Raycast(start=%+v, dir=%+v, maxDist=%v) did not return within %v; likely an infinite loop", start, dir, maxDist, raycastTimeout)
		return raycastResult{}
	}
}

// negZero is a float32 negative zero. The literal -0 is normalized away by
// the Go compiler, so it has to be built at runtime.
func negZero() float32 {
	return float32(math.Copysign(0, -1))
}

// buildCorridorWorld makes a single-block-wide, single-block-tall air
// corridor running along +Z from z=0, flanked by stone walls at x=7 and x=9,
// and capped by a single stone block at (8, 64, 8). A ray fired from inside
// the corridor along +Z travels exactly 7.5 world units of open air before
// reaching the near face of the cap.
func buildCorridorWorld(t *testing.T) *world.World {
	t.Helper()
	return testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 7, MinY: 64, MinZ: 0, MaxX: 7, MaxY: 64, MaxZ: 9}, blocks.Stone).
		Fill(testutil.Box{MinX: 9, MinY: 64, MinZ: 0, MaxX: 9, MaxY: 64, MaxZ: 9}, blocks.Stone).
		Fill(testutil.Box{MinX: 8, MinY: 64, MinZ: 8, MaxX: 8, MaxY: 64, MaxZ: 8}, blocks.Stone).
		Build()
}

// TestRaycast_AxisAlignedHit fires a ray straight down the corridor and
// checks not just that something is hit, but that it is the right block, at
// the right coordinates, with the right face normal. Termination alone does
// not prove the DDA stepping logic is correct.
func TestRaycast_AxisAlignedHit(t *testing.T) {
	w := buildCorridorWorld(t)
	start := core.Vec3{X: 8.5, Y: 64.5, Z: 0.5}
	dir := core.Vec3{X: 0, Y: 0, Z: 1}

	r := raycastSafe(t, w, start, dir, 10)

	if !r.hit {
		t.Fatalf("Raycast down the corridor: hit = false, want true")
	}
	if r.pos != [3]int{8, 64, 8} {
		t.Errorf("Raycast down the corridor: pos = %v, want [8 64 8]", r.pos)
	}
	if r.face != [3]int{0, 0, -1} {
		t.Errorf("Raycast down the corridor: face = %v, want [0 0 -1]", r.face)
	}
	if r.block != blocks.Stone {
		t.Errorf("Raycast down the corridor: block = %v, want Stone", r.block)
	}
}

// TestRaycast_ZeroLengthDirection checks that a direction with no length
// reports no hit, and does not hang, even when the ray starts inside a solid
// block (where a naive implementation might special-case "hit at distance
// zero").
func TestRaycast_ZeroLengthDirection(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 8, MinY: 64, MinZ: 8, MaxX: 8, MaxY: 64, MaxZ: 8}, blocks.Stone).
		Build()

	tests := []struct {
		name  string
		start core.Vec3
	}{
		{"outside any block", core.Vec3{X: 2.5, Y: 64.5, Z: 2.5}},
		{"inside a solid block", core.Vec3{X: 8.5, Y: 64.5, Z: 8.5}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := raycastSafe(t, w, tt.start, core.Vec3{}, 10)
			if r.hit || r.pos != [3]int{} || r.face != [3]int{} || r.block != nil {
				t.Errorf("Raycast with zero-length dir = %+v, want the zero result", r)
			}
		})
	}
}

// TestRaycast_Terminates fires rays with every combination of zero, +0 and
// -0 components across the axes that used to hang the loop, plus a few
// ordinary directions, and requires every one of them to return within
// raycastTimeout. This is the direct reproduction of the reported freeze: a
// dir.X of -0.0 (and, by the same bug, -0.0 on Y or Z) sends tMax to -Inf
// forever.
func TestRaycast_Terminates(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 8, MinY: 64, MinZ: 8, MaxX: 8, MaxY: 64, MaxZ: 8}, blocks.Stone).
		Build()
	start := core.Vec3{X: 2.5, Y: 64.5, Z: 2.5}
	nz := negZero()

	dirs := map[string]core.Vec3{
		"-0 on X":           {X: nz, Y: 0.3, Z: 0.3},
		"-0 on Y":           {X: 0.3, Y: nz, Z: 0.3},
		"-0 on Z":           {X: 0.3, Y: 0.3, Z: nz},
		"+0 on X":           {X: 0, Y: 0.3, Z: 0.3},
		"+0 on Y":           {X: 0.3, Y: 0, Z: 0.3},
		"+0 on Z":           {X: 0.3, Y: 0.3, Z: 0},
		"all -0":            {X: nz, Y: nz, Z: nz},
		"ordinary diagonal": {X: 0.5, Y: 0.5, Z: 0.5},
	}

	for name, dir := range dirs {
		t.Run(name, func(t *testing.T) {
			// The result isn't asserted here; TestRaycast_SignOfZeroInvariance
			// and TestRaycast_AxisAlignedHit cover correctness. This test only
			// requires that Raycast returns at all.
			raycastSafe(t, w, start, dir, 50)
		})
	}
}

// TestRaycast_SignOfZeroInvariance establishes that replacing a +0.0
// direction component with -0.0 never changes the result, on any single
// axis or on two axes at once, and regardless of whether the ray starts
// mid-voxel or exactly on an integer voxel boundary.
//
// The world has a floor (solid for y<=5) and a free-standing stone pillar at
// x=8, z=8, y=0..15, so both vertical and horizontal rays have something to
// hit.
func TestRaycast_SignOfZeroInvariance(t *testing.T) {
	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Flat(5).
		Fill(testutil.Box{MinX: 8, MinY: 0, MinZ: 8, MaxX: 8, MaxY: 15, MaxZ: 8}, blocks.Stone).
		Build()

	nz := negZero()

	assertSame := func(t *testing.T, base, variant raycastResult) {
		t.Helper()
		if base != variant {
			t.Errorf("sign of zero changed the result: baseline = %+v, variant = %+v", base, variant)
		}
	}

	t.Run("X_mid_voxel", func(t *testing.T) {
		start := core.Vec3{X: 8.5, Y: 10.5, Z: 2.5}
		base := raycastSafe(t, w, start, core.Vec3{X: 0, Y: -1, Z: 0.3}, 20)
		variant := raycastSafe(t, w, start, core.Vec3{X: nz, Y: -1, Z: 0.3}, 20)
		assertSame(t, base, variant)
	})

	t.Run("X_on_boundary", func(t *testing.T) {
		start := core.Vec3{X: 8, Y: 10, Z: 2}
		base := raycastSafe(t, w, start, core.Vec3{X: 0, Y: -1, Z: 0.3}, 20)
		variant := raycastSafe(t, w, start, core.Vec3{X: nz, Y: -1, Z: 0.3}, 20)
		assertSame(t, base, variant)
	})

	t.Run("Z_mid_voxel", func(t *testing.T) {
		start := core.Vec3{X: 2.5, Y: 10.5, Z: 8.5}
		base := raycastSafe(t, w, start, core.Vec3{X: 0.3, Y: -1, Z: 0}, 20)
		variant := raycastSafe(t, w, start, core.Vec3{X: 0.3, Y: -1, Z: nz}, 20)
		assertSame(t, base, variant)
	})

	t.Run("Z_on_boundary", func(t *testing.T) {
		start := core.Vec3{X: 2, Y: 10, Z: 8}
		base := raycastSafe(t, w, start, core.Vec3{X: 0.3, Y: -1, Z: 0}, 20)
		variant := raycastSafe(t, w, start, core.Vec3{X: 0.3, Y: -1, Z: nz}, 20)
		assertSame(t, base, variant)
	})

	t.Run("Y_mid_voxel", func(t *testing.T) {
		start := core.Vec3{X: 2.5, Y: 10.5, Z: 8.5}
		base := raycastSafe(t, w, start, core.Vec3{X: 1, Y: 0, Z: 0.05}, 20)
		variant := raycastSafe(t, w, start, core.Vec3{X: 1, Y: nz, Z: 0.05}, 20)
		assertSame(t, base, variant)
	})

	t.Run("Y_on_boundary", func(t *testing.T) {
		start := core.Vec3{X: 2, Y: 10, Z: 8}
		base := raycastSafe(t, w, start, core.Vec3{X: 1, Y: 0, Z: 0.05}, 20)
		variant := raycastSafe(t, w, start, core.Vec3{X: 1, Y: nz, Z: 0.05}, 20)
		assertSame(t, base, variant)
	})

	t.Run("XY_two_axes_mid_voxel", func(t *testing.T) {
		start := core.Vec3{X: 8.5, Y: 10.5, Z: 2.5}
		base := raycastSafe(t, w, start, core.Vec3{X: 0, Y: 0, Z: 1}, 20)
		for name, dir := range map[string]core.Vec3{
			"--": {X: nz, Y: nz, Z: 1},
			"+-": {X: 0, Y: nz, Z: 1},
			"-+": {X: nz, Y: 0, Z: 1},
		} {
			t.Run(name, func(t *testing.T) {
				variant := raycastSafe(t, w, start, dir, 20)
				assertSame(t, base, variant)
			})
		}
	})

	t.Run("XY_two_axes_on_boundary", func(t *testing.T) {
		start := core.Vec3{X: 8, Y: 10, Z: 2}
		base := raycastSafe(t, w, start, core.Vec3{X: 0, Y: 0, Z: 1}, 20)
		for name, dir := range map[string]core.Vec3{
			"--": {X: nz, Y: nz, Z: 1},
			"+-": {X: 0, Y: nz, Z: 1},
			"-+": {X: nz, Y: 0, Z: 1},
		} {
			t.Run(name, func(t *testing.T) {
				variant := raycastSafe(t, w, start, dir, 20)
				assertSame(t, base, variant)
			})
		}
	})

	t.Run("XZ_two_axes_mid_voxel", func(t *testing.T) {
		start := core.Vec3{X: 3.5, Y: 10.5, Z: 3.5}
		base := raycastSafe(t, w, start, core.Vec3{X: 0, Y: -1, Z: 0}, 20)
		for name, dir := range map[string]core.Vec3{
			"--": {X: nz, Y: -1, Z: nz},
			"+-": {X: 0, Y: -1, Z: nz},
			"-+": {X: nz, Y: -1, Z: 0},
		} {
			t.Run(name, func(t *testing.T) {
				variant := raycastSafe(t, w, start, dir, 20)
				assertSame(t, base, variant)
			})
		}
	})

	t.Run("YZ_two_axes_mid_voxel", func(t *testing.T) {
		start := core.Vec3{X: 2.5, Y: 10.5, Z: 8.5}
		base := raycastSafe(t, w, start, core.Vec3{X: 1, Y: 0, Z: 0}, 20)
		for name, dir := range map[string]core.Vec3{
			"--": {X: 1, Y: nz, Z: nz},
			"+-": {X: 1, Y: 0, Z: nz},
			"-+": {X: 1, Y: nz, Z: 0},
		} {
			t.Run(name, func(t *testing.T) {
				variant := raycastSafe(t, w, start, dir, 20)
				assertSame(t, base, variant)
			})
		}
	})

	t.Run("YZ_two_axes_on_boundary", func(t *testing.T) {
		start := core.Vec3{X: 2, Y: 10, Z: 8}
		base := raycastSafe(t, w, start, core.Vec3{X: 1, Y: 0, Z: 0}, 20)
		for name, dir := range map[string]core.Vec3{
			"--": {X: 1, Y: nz, Z: nz},
			"+-": {X: 1, Y: 0, Z: nz},
			"-+": {X: 1, Y: nz, Z: 0},
		} {
			t.Run(name, func(t *testing.T) {
				variant := raycastSafe(t, w, start, dir, 20)
				assertSame(t, base, variant)
			})
		}
	})
}

// TestRaycast_PositiveScaleInvariance establishes that scaling dir by any
// positive constant does not change the result. Before dir was normalized
// inside Raycast, a larger-magnitude dir travelled further for the same
// maxDist, because maxDist was being compared against the DDA's t parameter
// rather than a true world distance.
func TestRaycast_PositiveScaleInvariance(t *testing.T) {
	w := buildCorridorWorld(t)
	start := core.Vec3{X: 8.5, Y: 64.5, Z: 0.5}
	const maxDist = 10 // comfortably more than the 7.5 units to the cap

	ks := []float32{0.25, 1, 2, 100}
	var baseline raycastResult
	for i, k := range ks {
		dir := core.Vec3{X: 0, Y: 0, Z: k}
		r := raycastSafe(t, w, start, dir, maxDist)
		if i == 0 {
			baseline = r
			continue
		}
		if r != baseline {
			t.Errorf("Raycast with dir scaled by k=%v = %+v, want same as k=%v: %+v", k, r, ks[0], baseline)
		}
	}

	if !baseline.hit {
		t.Fatalf("baseline raycast did not hit anything; test setup is broken")
	}
}

// TestRaycast_MaxDistInWorldUnits establishes that maxDist is measured in
// world units, not in units of |dir|. A block sits a known 7.5 world units
// down the corridor; maxDist comfortably above that distance must find it,
// comfortably below must not, and this must hold no matter how long or short
// dir is.
func TestRaycast_MaxDistInWorldUnits(t *testing.T) {
	w := buildCorridorWorld(t)
	start := core.Vec3{X: 8.5, Y: 64.5, Z: 0.5}

	magnitudes := []float32{0.1, 1, 3, 50}

	t.Run("maxDist well above the target distance hits", func(t *testing.T) {
		for _, k := range magnitudes {
			dir := core.Vec3{X: 0, Y: 0, Z: k}
			r := raycastSafe(t, w, start, dir, 20)
			if !r.hit {
				t.Errorf("|dir|=%v, maxDist=20 (target at 7.5): hit = false, want true", k)
			}
		}
	})

	t.Run("maxDist well below the target distance misses", func(t *testing.T) {
		for _, k := range magnitudes {
			dir := core.Vec3{X: 0, Y: 0, Z: k}
			r := raycastSafe(t, w, start, dir, 3)
			if r.hit {
				t.Errorf("|dir|=%v, maxDist=3 (target at 7.5): hit = true, want false", k)
			}
		}
	})
}

// TestRaycast_PlayerArmLengthUnchanged guards against a regression in the
// one real-world caller: the camera always passes a unit-length direction,
// so PlayerArmLength's reach must be exactly what it was before Raycast
// started normalizing internally (normalizing an already-unit vector is a
// no-op, modulo floating point noise well below one voxel).
func TestRaycast_PlayerArmLengthUnchanged(t *testing.T) {
	reach := config.Current.PlayerArmLength

	w := testutil.NewWorld(t).
		Chunks(0, 0, 0, 0).
		Fill(testutil.Box{MinX: 8, MinY: 64, MinZ: 8, MaxX: 8, MaxY: 64, MaxZ: 8}, blocks.Stone).
		Build()

	// The block's near face sits 1 unit inside PlayerArmLength.
	near := core.Vec3{X: float32(8) - (reach - 1), Y: 64.5, Z: 8.5}
	// The block's near face sits 1 unit beyond PlayerArmLength.
	far := core.Vec3{X: float32(8) - (reach + 1), Y: 64.5, Z: 8.5}
	dir := core.Vec3{X: 1, Y: 0, Z: 0} // already unit length, as the camera provides

	if r := raycastSafe(t, w, near, dir, reach); !r.hit {
		t.Errorf("target 1 unit inside PlayerArmLength (%v): hit = false, want true", reach)
	}
	if r := raycastSafe(t, w, far, dir, reach); r.hit {
		t.Errorf("target 1 unit beyond PlayerArmLength (%v): hit = true, want false", reach)
	}
}
