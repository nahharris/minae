package model

// SidedBlock is a full cube with top/bottom and side textures (e.g. grass).
type SidedBlock struct {
	top, bottom, side string
}

// NewSidedBlock creates a SidedBlock. If any texture is empty, it falls back
// to 'all', and then to blockID.
func NewSidedBlock(blockID, all, top, bottom, side string) *SidedBlock {
	if all == "" {
		all = blockID
	}
	if top == "" {
		top = all
	}
	if bottom == "" {
		bottom = all
	}
	if side == "" {
		side = all
	}
	return &SidedBlock{top: top, bottom: bottom, side: side}
}

func (m *SidedBlock) AppendQuads(dst []Quad, _ uint8) []Quad {
	dst = append(dst,
		Quad{V1: Vec3{1, 0, 1}, V2: Vec3{1, 0, 0}, V3: Vec3{1, 1, 0}, V4: Vec3{1, 1, 1}, Face: FaceRight, Texture: m.side, Cull: true},
		Quad{V1: Vec3{0, 0, 0}, V2: Vec3{0, 0, 1}, V3: Vec3{0, 1, 1}, V4: Vec3{0, 1, 0}, Face: FaceLeft, Texture: m.side, Cull: true},
		Quad{V1: Vec3{0, 1, 1}, V2: Vec3{1, 1, 1}, V3: Vec3{1, 1, 0}, V4: Vec3{0, 1, 0}, Face: FaceTop, Texture: m.top, Cull: true},
		Quad{V1: Vec3{0, 0, 0}, V2: Vec3{1, 0, 0}, V3: Vec3{1, 0, 1}, V4: Vec3{0, 0, 1}, Face: FaceBottom, Texture: m.bottom, Cull: true},
		Quad{V1: Vec3{0, 0, 1}, V2: Vec3{1, 0, 1}, V3: Vec3{1, 1, 1}, V4: Vec3{0, 1, 1}, Face: FaceFront, Texture: m.side, Cull: true},
		Quad{V1: Vec3{1, 0, 0}, V2: Vec3{0, 0, 0}, V3: Vec3{0, 1, 0}, V4: Vec3{1, 1, 0}, Face: FaceBack, Texture: m.side, Cull: true},
	)
	return dst
}

func (m *SidedBlock) Occludes(_ uint8, _ Face, _ Rect) bool {
	return true
}

func (m *SidedBlock) Textures() []string {
	return uniqueStrings([]string{m.top, m.bottom, m.side})
}

// CollisionBoxes appends the single unit box that fills the voxel. A
// SidedBlock is a full cube with different textures per face, not a
// different shape, so its collision box is identical to FullBlock's.
func (m *SidedBlock) CollisionBoxes(dst []Box, _ uint8) []Box {
	return append(dst, Box{Min: Vec3{0, 0, 0}, Max: Vec3{1, 1, 1}})
}
