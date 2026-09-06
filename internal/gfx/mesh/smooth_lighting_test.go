package mesh_test

import (
	"testing"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

// vertexColorAt returns the RGBA color bytes for the given vertex index
// within the given face (both 0-based), where a face spans 6 vertices as
// emitted by addQuad in builder.go.
func vertexColorAt(data *mesh.ChunkMeshData, faceIndex, vertex int) (r, g, b, a uint8) {
	const bytesPerFace = 6 * 4
	i := faceIndex*bytesPerFace + vertex*4
	return data.Colors[i], data.Colors[i+1], data.Colors[i+2], data.Colors[i+3]
}

// TestSmoothLighting_GradientAcrossQuad checks validation criterion 2: near
// a light-level discontinuity, a single face's corners carry different R
// (skylight) values, brightest at the corner nearest the light.
//
// The block sits at (8,8,8); its top face patch (see setRightFaceLightPatch
// for the general idea, applied here to the X/Z tangent axes of the top
// face) is lit unevenly: (9,9,9) -- the corner diagonal touching vertex V2,
// local (1,1,1) -- and its two edge-adjacent cells are bright (15), the
// shared outward cell (8,9,8) is dim (5), and everything else is dark (0).
func TestSmoothLighting_GradientAcrossQuad(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, stone)

	// Top face patch: y is fixed at 9, x and z range over {7,8,9}.
	for x := 7; x <= 9; x++ {
		for z := 7; z <= 9; z++ {
			c.SetSkyLight(x, 9, z, 0)
		}
	}
	c.SetSkyLight(8, 9, 8, 5)  // A: the cell the face looks into
	c.SetSkyLight(9, 9, 8, 15) // B for the corner nearest (9,9,9)
	c.SetSkyLight(8, 9, 9, 15) // C for the corner nearest (9,9,9)
	c.SetSkyLight(9, 9, 9, 15) // D: the corner diagonal itself

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// No neighboring blocks exist, so AO is uniformly unoccluded (255) at
	// every corner and the default (V1,V2,V3),(V1,V3,V4) triangulation is
	// kept -- vertex indices 0,1,2 (and repeated 3,4,5) map directly onto
	// corners V1,V2,V3,(V1),(V3),V4.
	v1R, _, _, _ := vertexColorAt(data, faceTop, 0)
	v2R, _, _, _ := vertexColorAt(data, faceTop, 1)
	v3R, _, _, _ := vertexColorAt(data, faceTop, 2)
	v4R, _, _, _ := vertexColorAt(data, faceTop, 5)

	// sum(level*17)/count for each corner's transparent A,B,C,D cells:
	// V1 (local x=0,z=1): A=5,B=(7,9,8)=0,C=(8,9,9)=15,D=(7,9,9)=0 -> 340/4=85
	// V2 (local x=1,z=1): A=5,B=(9,9,8)=15,C=(8,9,9)=15,D=(9,9,9)=15 -> 850/4=212
	// V3 (local x=1,z=0): A=5,B=(9,9,8)=15,C=(8,9,7)=0,D=(9,9,7)=0 -> 340/4=85
	// V4 (local x=0,z=0): A=5,B=(7,9,8)=0,C=(8,9,7)=0,D=(7,9,7)=0 -> 85/4=21
	if v1R != 85 {
		t.Errorf("V1 R = %d, want 85", v1R)
	}
	if v3R != 85 {
		t.Errorf("V3 R = %d, want 85", v3R)
	}
	if v2R != 212 {
		t.Errorf("V2 R = %d, want 212", v2R)
	}
	if v4R != 21 {
		t.Errorf("V4 R = %d, want 21", v4R)
	}

	if v1R == v2R && v2R == v3R && v3R == v4R {
		t.Fatal("all four corners share the same R; expected a gradient")
	}
	if v2R <= v1R || v2R <= v3R || v2R <= v4R {
		t.Errorf("corner nearest the bright cell (V2, R=%d) is not the brightest of V1=%d, V3=%d, V4=%d", v2R, v1R, v3R, v4R)
	}
}

// TestSmoothLighting_InsideCornerDarkens checks validation criterion 3: at
// the shared corner where two solid blocks meet a face, that corner's AO
// (the B channel) is the lowest of the face's four corners, even at full
// skylight.
//
// The base block sits at (8,8,8); two solid "wall" blocks at (9,9,8) and
// (8,9,7) sit in the top face's outward patch. Per the AO rule, the corner
// whose two edge-adjacent cells (B and C) are both one of those walls -- V3,
// local (1,1,0) -- is fully occluded, while the others are only partially
// occluded or not occluded at all.
func TestSmoothLighting_InsideCornerDarkens(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, stone)
	c.SetBlock(9, 9, 8, stone)
	c.SetBlock(8, 9, 7, stone)

	// Full light everywhere relevant, to prove AO doesn't need darkness.
	for x := 7; x <= 9; x++ {
		for z := 7; z <= 9; z++ {
			c.SetSkyLight(x, 9, z, 15)
		}
	}
	c.SetSkyLight(8, 9, 8, 15)

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// Neither wall block is face-adjacent to the base block (each differs
	// from it in two axes), so no faces are culled and the base block,
	// being processed first in x,y,z order, keeps its faces at indices 0-5:
	// its top face is faceTop (2).
	//
	// AO is collected per corner *position* rather than per emitted index.
	// Which diagonal the quad splits along changes the order vertices come out
	// in, and that is a rendering decision covered by TestAO_... — indexing by
	// it here would make this geometric claim fail whenever that rule changes,
	// for a reason having nothing to do with inside corners.
	ao := make(map[[3]float32]uint8, 4)
	posOff := faceTop * 6 * 3
	for i := range 6 {
		pos := [3]float32{
			data.Vertices[posOff+i*3],
			data.Vertices[posOff+i*3+1],
			data.Vertices[posOff+i*3+2],
		}
		_, _, b, _ := vertexColorAt(data, faceTop, i)
		ao[pos] = b
	}
	if len(ao) != 4 {
		t.Fatalf("expected 4 distinct corners on the face, got %d: %v", len(ao), ao)
	}

	// The wall at (9,9,8) and the wall at (8,9,7) both border the +X/-Z corner
	// of this top face, which sits at world (9,9,8). That is the inside corner.
	// Read from the ramp rather than hard-coding a byte: the contrast is
	// documented as tunable, and this test is about which corner is darkest,
	// not about what that darkness happens to be worth today.
	fullyOccluded := mesh.AORampForTest()[0]
	inside := [3]float32{9, 9, 8}

	got, ok := ao[inside]
	if !ok {
		t.Fatalf("no vertex at the expected inside corner %v; corners were %v", inside, ao)
	}
	if got != fullyOccluded {
		t.Errorf("inside corner %v has B = %d, want %d (fully occluded)", inside, got, fullyOccluded)
	}
	for pos, b := range ao {
		if pos != inside && b <= got {
			t.Errorf("corner %v has B = %d, which is not brighter than the inside corner's %d", pos, b, got)
		}
	}
}

// TestSmoothLighting_AOIndependentOfLight checks validation criterion 4:
// the same geometry produces identical B (AO) values regardless of the
// skylight level.
func TestSmoothLighting_AOIndependentOfLight(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	buildWithLight := func(level uint8) *mesh.ChunkMeshData {
		w := world.NewWorld()
		c := world.NewChunk(0, 0)
		w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

		c.SetBlock(8, 8, 8, stone)
		c.SetBlock(9, 9, 8, stone)
		c.SetBlock(8, 9, 7, stone)

		for x := 7; x <= 9; x++ {
			for z := 7; z <= 9; z++ {
				c.SetSkyLight(x, 9, z, level)
			}
		}
		c.SetSkyLight(8, 9, 8, level)

		return mesh.GenerateChunkMeshData(c, w, nil)
	}

	bright := buildWithLight(15)
	dim := buildWithLight(4)
	if bright == nil || dim == nil {
		t.Fatal("expected mesh data, got nil")
	}

	for v := 0; v < 6; v++ {
		_, _, bBright, _ := vertexColorAt(bright, faceTop, v)
		_, _, bDim, _ := vertexColorAt(dim, faceTop, v)
		if bBright != bDim {
			t.Errorf("vertex %d: AO at skylight 15 = %d, at skylight 4 = %d; AO must not depend on light", v, bBright, bDim)
		}
	}
}

// TestSmoothLighting_OpaqueNeighborExcludedFromAverage checks validation
// criterion 6: a face flush against a wall is not darker than the same face
// in open air at the same light level, because opaque cells are excluded
// from the smoothing average rather than counted as unlit (zero).
//
// The base block's right face patch is lit fully bright (15) except for one
// cell, (9,9,8), which instead holds a solid "wall" block. If that opaque
// cell were wrongly averaged in as 0, two of the face's four corners (V3 and
// V4, whose B offset lands on that cell) would read below full bright.
func TestSmoothLighting_OpaqueNeighborExcludedFromAverage(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	c.SetBlock(8, 8, 8, stone)
	c.SetBlock(9, 9, 8, stone) // wall, sits inside the right face's own patch

	for y := 7; y <= 9; y++ {
		for z := 7; z <= 9; z++ {
			if y == 9 && z == 8 {
				continue // occupied by the wall block above
			}
			c.SetSkyLight(9, y, z, 15)
		}
	}

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// The base block is processed first (x=8 before the wall's x=9), so its
	// right face is still faceRight (0).
	for v := 0; v < 6; v++ {
		r, _, _, _ := vertexColorAt(data, faceRight, v)
		if r != 255 {
			t.Errorf("vertex %d: R = %d, want 255 (opaque neighbor must be excluded, not counted as unlit)", v, r)
		}
	}
}

// TestSmoothLighting_TriangulationFlipKeepsCornersTogether checks validation
// criterion 5, the failure mode the M7 brief calls out specifically: a quad
// whose AO is asymmetric across the default diagonal must flip its
// triangulation, and every one of the six emitted vertices must carry the
// position, texture coordinate AND colour of the SAME corner. Flipping only
// the position array (while leaving UVs or colours in the old order) would
// pass a naive "is the position order different" check but silently mismatch
// a vertex's look from its shape.
func TestSmoothLighting_TriangulationFlipKeepsCornersTogether(t *testing.T) {
	blocks.Reset()
	stone := blocks.Stone
	blocks.Register(stone)

	w := world.NewWorld()
	c := world.NewChunk(0, 0)
	w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

	// Base block at (8,8,8); two walls placed only at x>=8 so the base
	// block -- being first in x,y,z scan order -- keeps its own faces at
	// indices 0-5 (top = faceTop = 2).
	c.SetBlock(8, 8, 8, stone)
	c.SetBlock(9, 9, 8, stone) // occludes B for V2 and V3
	c.SetBlock(8, 9, 7, stone) // occludes C for V3 and V4

	data := mesh.GenerateChunkMeshData(c, w, nil)
	if data == nil {
		t.Fatal("expected mesh data, got nil")
	}

	// No light was set (everything defaults to 0), so every corner's R and G
	// are 0; corner identity is carried entirely by position, UV and AO, which
	// this test cross-checks to prove they travel together.
	//
	// Per-corner AO: V1=3 (255, unoccluded), V2=2 (204, B wall only),
	// V3=0 (102, both B and C walls), V4=2 (204, C wall only).
	//
	// The expected emission order is deliberately NOT written down here. Which
	// diagonal a quad splits along is a rendering decision that TestAO_...
	// covers on its own; pinning it here as well would mean any change to the
	// flip rule fails this test for the wrong reason, and the fix would be to
	// edit the expectation — exactly how a test gets bent to fit. What this
	// test asserts is the property that must hold under *either* split: every
	// emitted vertex's position, UV and colour belong to the same corner.
	type corner struct {
		u, v       float32
		r, g, b, a uint8
	}
	// Keyed by world-space position: block origin (8,8,8) + local offset.
	// AO bytes come from the ramp by level, not hard-coded: the contrast is
	// documented as tunable and this test is about corners travelling together,
	// not about the ramp's current values.
	ramp := mesh.AORampForTest()
	corners := map[[3]float32]corner{
		{8, 9, 9}: {0, 1, 0, 0, ramp[3], 255},
		{9, 9, 9}: {1, 1, 0, 0, ramp[2], 255},
		{9, 9, 8}: {1, 0, 0, 0, ramp[0], 255},
		{8, 9, 8}: {0, 0, 0, 0, ramp[2], 255},
	}

	const bytesPerFace = 6 * 4
	posOff := faceTop * 6 * 3
	uvOff := faceTop * 6 * 2
	colOff := faceTop * bytesPerFace

	seen := make(map[[3]float32]int, 4)

	for i := range 6 {
		pos := [3]float32{
			data.Vertices[posOff+i*3],
			data.Vertices[posOff+i*3+1],
			data.Vertices[posOff+i*3+2],
		}

		want, ok := corners[pos]
		if !ok {
			t.Errorf("vertex %d sits at %v, which is not a corner of this face", i, pos)
			continue
		}
		seen[pos]++

		u, v := data.Texcoords[uvOff+i*2], data.Texcoords[uvOff+i*2+1]
		if u != want.u || v != want.v {
			t.Errorf("vertex %d at %v: uv (%v,%v), want that corner's (%v,%v)", i, pos, u, v, want.u, want.v)
		}

		r, g, b, a := data.Colors[colOff+i*4], data.Colors[colOff+i*4+1], data.Colors[colOff+i*4+2], data.Colors[colOff+i*4+3]
		if r != want.r || g != want.g || b != want.b || a != want.a {
			t.Errorf("vertex %d at %v: colour (%d,%d,%d,%d), want that corner's (%d,%d,%d,%d)",
				i, pos, r, g, b, a, want.r, want.g, want.b, want.a)
		}
	}

	// Two triangles sharing a diagonal must use all four corners, with exactly
	// the two on the shared diagonal appearing twice. Anything else means the
	// quad was not triangulated as a quad.
	if len(seen) != 4 {
		t.Errorf("expected all 4 corners to be emitted, got %d distinct: %v", len(seen), seen)
	}
	twice := 0
	for _, n := range seen {
		if n == 2 {
			twice++
		}
	}
	if twice != 2 {
		t.Errorf("expected exactly 2 corners emitted twice (the shared diagonal), got %d: %v", twice, seen)
	}
}

// The triangulation flip must depend on raw occlusion levels, not on the
// aoRamp bytes those levels map to.
//
// aoRamp is documented as free to tune, including non-linearly. It happens to
// be evenly spaced today, which makes comparing the ramped bytes accidentally
// equivalent to comparing the levels — so a byte-based comparison would pass
// every test right up until someone adjusts the contrast, and then quietly
// change which diagonal some quads split along, reintroducing the AO crease.
//
// Same geometry, two very different ramps, identical triangulation.
func TestSmoothLighting_FlipIsIndependentOfTheAORamp(t *testing.T) {
	geometry := func() []float32 {
		blocks.Reset()
		stone := blocks.Stone
		blocks.Register(stone)

		w := world.NewWorld()
		c := world.NewChunk(0, 0)
		w.Chunks[world.ChunkCoord{X: 0, Z: 0}] = c

		// The asymmetric-AO arrangement from the flip test above.
		c.SetBlock(8, 8, 8, stone)
		c.SetBlock(9, 9, 8, stone)
		c.SetBlock(8, 9, 7, stone)

		data := mesh.GenerateChunkMeshData(c, w, nil)
		if data == nil {
			t.Fatal("expected mesh data, got nil")
		}
		return data.Vertices
	}

	linear := geometry()

	// Deliberately uneven. This geometry has corner AO levels (3,2,0,2), so
	// the level comparison flips (3+0 < 2+2). Under a byte comparison with
	// this ramp it would not (255+0 is not < 10+10), so the two disagree and
	// the mesh would differ.
	restore := mesh.SetAORampForTest([4]uint8{0, 5, 10, 255})
	t.Cleanup(restore)

	uneven := geometry()

	if len(linear) != len(uneven) {
		t.Fatalf("vertex count changed with the ramp: %d vs %d", len(linear), len(uneven))
	}
	for i := range linear {
		if linear[i] != uneven[i] {
			t.Fatalf("triangulation changed when aoRamp was retuned, at vertex float %d: %v vs %v.\n"+
				"The flip must compare occlusion levels, not ramped bytes.",
				i, linear[i], uneven[i])
		}
	}
}
