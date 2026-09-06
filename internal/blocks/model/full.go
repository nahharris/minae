package model

// FullBlock is the classic Minecraft-like cube model.
type FullBlock struct {
	tex [6]string
}

// NewFullBlock creates a FullBlock with per-face texture keys.
func NewFullBlock(textures [6]string) *FullBlock {
	return &FullBlock{tex: textures}
}

func (m *FullBlock) AppendQuads(dst []Quad, _ uint8) []Quad {
	dst = append(dst,
		Quad{V1: Vec3{1, 0, 1}, V2: Vec3{1, 0, 0}, V3: Vec3{1, 1, 0}, V4: Vec3{1, 1, 1}, Face: FaceRight, Texture: m.tex[FaceRight], Cull: true},
		Quad{V1: Vec3{0, 0, 0}, V2: Vec3{0, 0, 1}, V3: Vec3{0, 1, 1}, V4: Vec3{0, 1, 0}, Face: FaceLeft, Texture: m.tex[FaceLeft], Cull: true},
		Quad{V1: Vec3{0, 1, 1}, V2: Vec3{1, 1, 1}, V3: Vec3{1, 1, 0}, V4: Vec3{0, 1, 0}, Face: FaceTop, Texture: m.tex[FaceTop], Cull: true},
		Quad{V1: Vec3{0, 0, 0}, V2: Vec3{1, 0, 0}, V3: Vec3{1, 0, 1}, V4: Vec3{0, 0, 1}, Face: FaceBottom, Texture: m.tex[FaceBottom], Cull: true},
		Quad{V1: Vec3{0, 0, 1}, V2: Vec3{1, 0, 1}, V3: Vec3{1, 1, 1}, V4: Vec3{0, 1, 1}, Face: FaceFront, Texture: m.tex[FaceFront], Cull: true},
		Quad{V1: Vec3{1, 0, 0}, V2: Vec3{0, 0, 0}, V3: Vec3{0, 1, 0}, V4: Vec3{1, 1, 0}, Face: FaceBack, Texture: m.tex[FaceBack], Cull: true},
	)
	return dst
}

func (m *FullBlock) Occludes(_ uint8, _ Face, _ Rect) bool {
	return true
}

func (m *FullBlock) Textures() []string {
	return uniqueStrings(m.tex[:])
}

// CollisionBoxes appends the single unit box that fills the voxel.
func (m *FullBlock) CollisionBoxes(dst []Box, _ uint8) []Box {
	return append(dst, Box{Min: Vec3{0, 0, 0}, Max: Vec3{1, 1, 1}})
}
