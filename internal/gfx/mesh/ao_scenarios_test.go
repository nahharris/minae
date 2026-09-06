package mesh_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/blocks/model"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

// A matrix of corner and edge configurations, checked as ambient-occlusion
// *levels* rather than colour bytes.
//
// Levels are what the geometry determines; the bytes they map to are a
// contrast setting that is documented as tunable. Asserting bytes would make
// every one of these fail the next time the ramp is adjusted, for a reason
// having nothing to do with corners.

// aoLevelOf inverts the ramp, turning an emitted B byte back into 0..3.
func aoLevelOf(t *testing.T, b uint8) int {
	t.Helper()
	for level, v := range mesh.AORampForTest() {
		if v == b {
			return level
		}
	}
	t.Fatalf("AO byte %d is not a value in the ramp %v", b, mesh.AORampForTest())
	return -1
}

// baseBlock is the block under test. Neighbours are placed around it.
var baseBlock = [3]int{8, 8, 8}

// aoLevelsForFace meshes the scene and returns the AO level at each corner of
// one face of the base block, keyed by the corner's world position.
//
// It locates the face by matching the emitted normal and checking the vertices
// fall inside the base block's own bounds, rather than by index arithmetic.
// Face indices shift whenever a neighbour sorts earlier in the chunk's x,y,z
// scan order, which several of these scenarios do.
func aoLevelsForFace(t *testing.T, build func(c *world.Chunk), nx, ny, nz float32) map[[3]float32]int {
	t.Helper()
	blocks.ResetToVanilla()

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c
	c.SetBlock(baseBlock[0], baseBlock[1], baseBlock[2], blocks.Stone)
	build(c)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	inBase := func(p [3]float32) bool {
		for axis := range 3 {
			lo := float32(baseBlock[axis])
			if p[axis] < lo || p[axis] > lo+1 {
				return false
			}
		}
		return true
	}

	out := map[[3]float32]int{}
	for v := 0; v < len(data.Vertices)/3; v++ {
		if data.Normals[v*3] != nx || data.Normals[v*3+1] != ny || data.Normals[v*3+2] != nz {
			continue
		}
		p := [3]float32{data.Vertices[v*3], data.Vertices[v*3+1], data.Vertices[v*3+2]}
		if !inBase(p) {
			continue
		}
		out[p] = aoLevelOf(t, data.Colors[v*4+2])
	}

	if len(out) != 4 {
		t.Fatalf("expected 4 distinct corners on the face with normal (%v,%v,%v), got %d: %v",
			nx, ny, nz, len(out), format(out))
	}
	return out
}

func format(ao map[[3]float32]int) string {
	keys := make([][3]float32, 0, len(ao))
	for k := range ao {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		for axis := range 3 {
			if keys[i][axis] != keys[j][axis] {
				return keys[i][axis] < keys[j][axis]
			}
		}
		return false
	})
	s := ""
	for _, k := range keys {
		s += fmt.Sprintf("(%.0f,%.0f,%.0f)=%d ", k[0], k[1], k[2], ao[k])
	}
	return s
}

func bottomSlab(c *world.Chunk, x, y, z int) { c.SetBlockState(x, y, z, blocks.StoneSlab, 0) }
func topSlab(c *world.Chunk, x, y, z int) {
	c.SetBlockState(x, y, z, blocks.StoneSlab, model.MetaSlabTopBit)
}

func TestAO_CornerAndEdgeScenarios(t *testing.T) {
	const up, down = 1, -1

	tests := []struct {
		name    string
		build   func(c *world.Chunk)
		normal  [3]float32
		want    map[[3]float32]int
		because string
	}{
		{
			name:   "open face is unoccluded everywhere",
			build:  func(*world.Chunk) {},
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 3, {8, 9, 9}: 3, {9, 9, 9}: 3,
			},
			because: "nothing neighbours the face",
		},
		{
			name:   "one edge neighbour shades the two corners along that edge equally",
			build:  func(c *world.Chunk) { c.SetBlock(9, 9, 8, blocks.Stone) },
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 2, {8, 9, 9}: 3, {9, 9, 9}: 2,
			},
			because: "both corners on the +X edge see the same single occluder, so the edge between them is flat",
		},
		{
			name:   "a diagonal neighbour shades only the corner it touches",
			build:  func(c *world.Chunk) { c.SetBlock(9, 9, 9, blocks.Stone) },
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 3, {8, 9, 9}: 3, {9, 9, 9}: 2,
			},
			because: "a diagonal contributes to exactly one corner",
		},
		{
			name: "an inside corner is fully occluded even with the diagonal open",
			build: func(c *world.Chunk) {
				c.SetBlock(9, 9, 8, blocks.Stone)
				c.SetBlock(8, 9, 9, blocks.Stone)
			},
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 2, {8, 9, 9}: 2, {9, 9, 9}: 0,
			},
			because: "where two neighbours meet along an edge the diagonal touches the vertex at a point only, " +
				"contributing no solid angle — so level 1 is skipped and the corner goes straight to 0",
		},
		{
			name: "an inside corner with a solid diagonal is also fully occluded",
			build: func(c *world.Chunk) {
				c.SetBlock(9, 9, 8, blocks.Stone)
				c.SetBlock(8, 9, 9, blocks.Stone)
				c.SetBlock(9, 9, 9, blocks.Stone)
			},
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 2, {8, 9, 9}: 2, {9, 9, 9}: 0,
			},
			because: "filling the diagonal changes nothing, which is what makes the override consistent",
		},
		{
			name: "a corridor shades both edges equally, leaving the middle flat",
			build: func(c *world.Chunk) {
				for dz := -1; dz <= 1; dz++ {
					c.SetBlock(7, 9, 8+dz, blocks.Stone)
					c.SetBlock(9, 9, 8+dz, blocks.Stone)
				}
			},
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 1, {9, 9, 8}: 1, {8, 9, 9}: 1, {9, 9, 9}: 1,
			},
			because: "every corner sees one wall plus its diagonal, so the floor of a corridor is uniformly shaded",
		},

		// Slabs. These are the cases that were wrong: occlusion used to be a
		// per-block boolean, so a half-height slab shaded like a full cube.
		{
			name:   "a bottom slab beside a floor does shade it",
			build:  func(c *world.Chunk) { bottomSlab(c, 9, 9, 8) },
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 2, {8, 9, 9}: 3, {9, 9, 9}: 2,
			},
			because: "the slab fills the half of its cell that sits against the floor",
		},
		{
			name:   "a top slab beside a floor does not shade it",
			build:  func(c *world.Chunk) { topSlab(c, 9, 9, 8) },
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 3, {8, 9, 9}: 3, {9, 9, 9}: 3,
			},
			because: "the slab sits in the upper half of its cell and never touches the floor plane",
		},
		{
			name:   "a bottom slab beside a ceiling does not shade it",
			build:  func(c *world.Chunk) { bottomSlab(c, 9, 7, 8) },
			normal: [3]float32{0, down, 0},
			want: map[[3]float32]int{
				{8, 8, 8}: 3, {9, 8, 8}: 3, {8, 8, 9}: 3, {9, 8, 9}: 3,
			},
			because: "seen from below, the slab is a half cell away from the face",
		},
		{
			name:   "a top slab beside a ceiling does shade it",
			build:  func(c *world.Chunk) { topSlab(c, 9, 7, 8) },
			normal: [3]float32{0, down, 0},
			want: map[[3]float32]int{
				{8, 8, 8}: 3, {9, 8, 8}: 2, {8, 8, 9}: 3, {9, 8, 9}: 2,
			},
			because: "the slab fills the half of its cell that sits against the ceiling",
		},
		{
			name:   "a bottom slab beside a wall shades only the wall's lower corners",
			build:  func(c *world.Chunk) { bottomSlab(c, 9, 8, 9) },
			normal: [3]float32{0, 0, 1},
			want: map[[3]float32]int{
				{8, 8, 9}: 3, {9, 8, 9}: 2, {8, 9, 9}: 3, {9, 9, 9}: 3,
			},
			because: "the slab only reaches halfway up, so the wall's top corners are clear of it",
		},

		{
			name:   "an emitting neighbour never occludes",
			build:  func(c *world.Chunk) { c.SetBlock(9, 9, 8, blocks.Glowstone) },
			normal: [3]float32{0, up, 0},
			want: map[[3]float32]int{
				{8, 9, 8}: 3, {9, 9, 8}: 3, {8, 9, 9}: 3, {9, 9, 9}: 3,
			},
			because: "a light source is not blocking light, it is light",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aoLevelsForFace(t, tt.build, tt.normal[0], tt.normal[1], tt.normal[2])

			for pos, wantLevel := range tt.want {
				gotLevel, ok := got[pos]
				if !ok {
					t.Errorf("no corner at %v; got %s", pos, format(got))
					continue
				}
				if gotLevel != wantLevel {
					t.Errorf("corner %v: AO level %d, want %d\n  because: %s\n  full face: %s",
						pos, gotLevel, wantLevel, tt.because, format(got))
				}
			}
		})
	}
}
