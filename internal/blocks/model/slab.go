package model

// SlabBlock is a half-height block (top or bottom half) controlled by meta.
type SlabBlock struct {
	tex [6]string
}

// NewSlabBlock creates a slab block model with per-face texture keys.
func NewSlabBlock(textures [6]string) *SlabBlock {
	return &SlabBlock{tex: textures}
}

func (m *SlabBlock) AppendQuads(dst []Quad, meta uint8) []Quad {
	isTop := meta&MetaSlabTopBit != 0
	var y0, y1 float32
	if isTop {
		y0, y1 = 0.5, 1.0
	} else {
		y0, y1 = 0.0, 0.5
	}

	// Side faces are boundary faces and can be culled.
	dst = append(dst,
		Quad{V1: Vec3{1, y0, 1}, V2: Vec3{1, y0, 0}, V3: Vec3{1, y1, 0}, V4: Vec3{1, y1, 1}, Face: FaceRight, Texture: m.tex[FaceRight], Cull: true},
		Quad{V1: Vec3{0, y0, 0}, V2: Vec3{0, y0, 1}, V3: Vec3{0, y1, 1}, V4: Vec3{0, y1, 0}, Face: FaceLeft, Texture: m.tex[FaceLeft], Cull: true},
		Quad{V1: Vec3{0, y0, 1}, V2: Vec3{1, y0, 1}, V3: Vec3{1, y1, 1}, V4: Vec3{0, y1, 1}, Face: FaceFront, Texture: m.tex[FaceFront], Cull: true},
		Quad{V1: Vec3{1, y0, 0}, V2: Vec3{0, y0, 0}, V3: Vec3{0, y1, 0}, V4: Vec3{1, y1, 0}, Face: FaceBack, Texture: m.tex[FaceBack], Cull: true},
	)

	// Horizontal faces:
	// - The boundary face exists only on the occupied half edge (y=0 or y=1).
	// - The internal face at y=0.5 is always rendered and is not culled by neighbors.
	if isTop {
		// Top boundary (y=1): cullable
		dst = append(dst, Quad{
			V1: Vec3{0, 1, 1}, V2: Vec3{1, 1, 1}, V3: Vec3{1, 1, 0}, V4: Vec3{0, 1, 0},
			Face: FaceTop, Texture: m.tex[FaceTop], Cull: true,
		})
		// Bottom internal (y=0.5): not cullable
		dst = append(dst, Quad{
			V1: Vec3{0, 0.5, 0}, V2: Vec3{1, 0.5, 0}, V3: Vec3{1, 0.5, 1}, V4: Vec3{0, 0.5, 1},
			Face: FaceBottom, Texture: m.tex[FaceBottom], Cull: false,
		})
		return dst
	}

	// Bottom slab
	// Bottom boundary (y=0): cullable
	dst = append(dst, Quad{
		V1: Vec3{0, 0, 0}, V2: Vec3{1, 0, 0}, V3: Vec3{1, 0, 1}, V4: Vec3{0, 0, 1},
		Face: FaceBottom, Texture: m.tex[FaceBottom], Cull: true,
	})
	// Top internal (y=0.5): not cullable
	dst = append(dst, Quad{
		V1: Vec3{0, 0.5, 1}, V2: Vec3{1, 0.5, 1}, V3: Vec3{1, 0.5, 0}, V4: Vec3{0, 0.5, 0},
		Face: FaceTop, Texture: m.tex[FaceTop], Cull: false,
	})
	return dst
}

func (m *SlabBlock) Occludes(meta uint8, face Face, region Rect) bool {
	isTop := meta&MetaSlabTopBit != 0

	switch face {
	case FaceTop:
		return isTop
	case FaceBottom:
		return !isTop
	}

	// Vertical faces: slab occlusion depends on Y span (we use V=y for all vertical faces).
	if isTop {
		return region.MinV >= 0.5
	}
	return region.MaxV <= 0.5
}

func (m *SlabBlock) Textures() []string {
	return uniqueStrings(m.tex[:])
}

// CollisionBoxes appends the half-height box the slab occupies: the bottom
// half (y=0..0.5), or the top half (y=0.5..1) when MetaSlabTopBit is set,
// mirroring the same bit test AppendQuads and Occludes already use.
func (m *SlabBlock) CollisionBoxes(dst []Box, meta uint8) []Box {
	if meta&MetaSlabTopBit != 0 {
		return append(dst, Box{Min: Vec3{0, 0.5, 0}, Max: Vec3{1, 1, 1}})
	}
	return append(dst, Box{Min: Vec3{0, 0, 0}, Max: Vec3{1, 0.5, 1}})
}
