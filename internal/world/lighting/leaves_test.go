package lighting_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/testutil"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
)

// TestLeaves_PassLightUnattenuated makes the M18 design decision "leaves
// pass light unattenuated, for now" directly observable: a sealed shaft
// carved through solid rock and filled with Leaves must carry block light to
// exactly the same levels, at every cell, as the identical shaft left as
// plain air.
//
// This works because of what the design decision explicitly forbids
// changing: kind.expected (lighting.go) is a function of (level, direction)
// only, never the destination block, and isTransparent treats leaves and air
// identically (both are !blocks.OpaqueToLight). So the two profiles below
// are not merely "both nonzero" -- they must be bit-for-bit identical, and a
// mutation that made leaves decay light by more than one level per step
// (attenuation) would diverge from the air profile immediately.
func TestLeaves_PassLightUnattenuated(t *testing.T) {
	const surface = 100
	const shaftY, shaftZ = 20, 8

	profileFor := func(fill *blocks.Block) []uint8 {
		b := testutil.NewWorld(t).Chunks(0, 0, 0, 0).Flat(surface).
			Clear(testutil.Box{MinX: 0, MaxX: 15, MinY: shaftY, MaxY: shaftY, MinZ: shaftZ, MaxZ: shaftZ})
		if fill != nil {
			b.Fill(testutil.Box{MinX: 1, MaxX: 15, MinY: shaftY, MaxY: shaftY, MinZ: shaftZ, MaxZ: shaftZ}, fill)
		}
		w := b.Fill(testutil.Box{MinX: 0, MaxX: 0, MinY: shaftY, MaxY: shaftY, MinZ: shaftZ, MaxZ: shaftZ}, blocks.Glowstone).
			Build()

		e := lighting.NewEngine(w)
		e.RecomputeAll()

		profile := make([]uint8, config.ChunkWidth)
		for x := range profile {
			profile[x] = w.GetBlockLight(x, shaftY, shaftZ)
		}
		return profile
	}

	air := profileFor(nil)
	leaves := profileFor(blocks.Leaves)

	if air[0] == 0 {
		t.Fatal("precondition failed: the source cell itself should be lit")
	}
	if air[len(air)-1] != 0 {
		t.Fatal("precondition failed: a 16-block air shaft from a single glowstone (level 15) should not " +
			"reach the far end -- this test would not be exercising decay at all")
	}

	for x := range air {
		if air[x] != leaves[x] {
			t.Fatalf("x=%d: air-shaft block light = %d, leaf-filled shaft = %d -- "+
				"leaves must pass light exactly like air, unattenuated", x, air[x], leaves[x])
		}
	}
}

// buildSeedTestWorldWithCanopy is buildSeedTestWorld's tunnel scenario (see
// seed_chunk_test.go) plus a canopy of Leaves straddling the seam between
// chunk (0,*) and (1,*), well above the flat surface. It exercises the M18
// transparency change specifically at a chunk boundary: the case most likely
// to expose a mistake in the light-transparent, self-culling combination,
// since a mesh- or light-only bug at a seam is exactly what M3's original
// seam fix and M15's follow-up were both about.
func buildSeedTestWorldWithCanopy(t *testing.T) *world.World {
	t.Helper()
	const surface = 240

	return testutil.NewWorld(t).
		Chunks(-1, -1, 1, 1).
		Flat(surface).
		Clear(testutil.Box{MinX: -16, MaxX: 31, MinY: 20, MaxY: 20, MinZ: 0, MaxZ: 0}).
		Fill(testutil.Box{MinX: 0, MaxX: 0, MinY: 20, MaxY: 20, MinZ: 0, MaxZ: 0}, blocks.Glowstone).
		Fill(testutil.Box{MinX: 13, MaxX: 18, MinY: surface + 1, MaxY: surface + 4, MinZ: -3, MaxZ: 3}, blocks.Leaves).
		Build()
}

// TestSeedChunk_MatchesFullRecompute_WithLeaves extends the load-bearing
// equivalence check (see TestSeedChunk_MatchesFullRecompute) to terrain that
// includes leaves straddling a chunk seam -- the safety net the M18
// transparency change needs, since isTransparent is exactly what every
// seeding path in this package goes through. The canopy sits above the
// tunnel, so this exercises skylight falling through leaves on its way down
// as well as the existing tunnel's block light.
func TestSeedChunk_MatchesFullRecompute_WithLeaves(t *testing.T) {
	w := buildSeedTestWorldWithCanopy(t)

	e := lighting.NewEngine(w)
	for _, c := range sortedCoords(w) {
		e.SeedChunk(c)
	}
	incremental := snapshot(w)

	truth := lighting.NewEngine(w)
	truth.RecomputeAll()
	expected := snapshot(w)

	if diff := firstDifference(incremental, expected); diff != "" {
		t.Fatalf("seeding chunk by chunk diverged from a full recompute on terrain with leaves:\n%s", diff)
	}
}

// TestSeedChunk_OrderIndependent_WithLeaves is
// TestSeedChunk_OrderIndependent run against the leaf-canopy fixture:
// seeding in different orders must still agree once a light-transparent
// block sits on the seam being crossed.
func TestSeedChunk_OrderIndependent_WithLeaves(t *testing.T) {
	seedInOrder := func(coords []world.ChunkCoord) map[world.ChunkCoord]lightState {
		w := buildSeedTestWorldWithCanopy(t)
		e := lighting.NewEngine(w)
		for _, c := range coords {
			e.SeedChunk(c)
		}
		return snapshot(w)
	}

	forward := sortedCoords(buildSeedTestWorldWithCanopy(t))
	reference := seedInOrder(forward)

	reversed := make([]world.ChunkCoord, len(forward))
	copy(reversed, forward)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}

	if diff := firstDifference(seedInOrder(reversed), reference); diff != "" {
		t.Errorf("seeding in reverse order changed the result on terrain with leaves:\n%s", diff)
	}
}
