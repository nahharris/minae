package blocks

import "sort"

// Face is one of the 6 axis-aligned faces of a voxel cell.
type Face uint8

const (
	FaceRight  Face = iota // +X
	FaceLeft               // -X
	FaceTop                // +Y
	FaceBottom             // -Y
	FaceFront              // +Z
	FaceBack               // -Z
)

// Opposite returns the opposite face direction.
func (f Face) Opposite() Face {
	switch f {
	case FaceRight:
		return FaceLeft
	case FaceLeft:
		return FaceRight
	case FaceTop:
		return FaceBottom
	case FaceBottom:
		return FaceTop
	case FaceFront:
		return FaceBack
	default:
		return FaceFront
	}
}

// Vec3 is a lightweight 3D vector in local block space (0..1).
type Vec3 struct {
	X, Y, Z float32
}

// Rect represents an axis-aligned rectangle in face-local coordinates (0..1).
// The interpretation of U/V is up to the caller; for culling we only need a
// consistent axis choice per face.
type Rect struct {
	MinU, MinV float32
	MaxU, MaxV float32
}

func (r Rect) contains(o Rect) bool {
	return r.MinU <= o.MinU && r.MinV <= o.MinV && r.MaxU >= o.MaxU && r.MaxV >= o.MaxV
}

// Quad is a single axis-aligned quad in local block space.
//
// Vertex order is expected to be v1,v2,v3,v4 where triangles are:
// (v1,v2,v3) and (v1,v3,v4). This matches the existing cube mesher layout.
type Quad struct {
	V1, V2, V3, V4 Vec3
	Face           Face
	Texture        string

	// Cull indicates the quad lies on a voxel boundary and can be culled by a
	// neighbor block on that side.
	Cull bool
}

// BlockModel defines how a block instance is rendered and how it occludes
// neighboring geometry for face culling.
type BlockModel interface {
	// AppendQuads appends all renderable quads for this block instance.
	AppendQuads(dst []Quad, meta uint8) []Quad

	// Occludes reports whether this block instance fully covers the given region
	// on the specified voxel boundary face.
	Occludes(meta uint8, face Face, region Rect) bool

	// Textures returns the set of texture keys referenced by this model.
	Textures() []string
}

// ModelSpec is the YAML-friendly description of a block model.
// It is compiled into a BlockModel at load/register time.
type ModelSpec struct {
	// Type selects the concrete model implementation.
	// Supported values: "full", "sided", "slab".
	Type string `yaml:"type"`

	// Textures maps logical names to texture keys (e.g. "minae/stone").
	// Supported keys (depending on model type):
	// - all, side
	// - top, bottom, left, right, front, back
	// - for sided: top, bottom, side (or all)
	Textures map[string]string `yaml:"textures"`

	// Orientable wraps the compiled model in an Orientable wrapper (4-way Y rotation).
	Orientable bool `yaml:"orientable"`
}

func (s ModelSpec) normalized() ModelSpec {
	// Keep it simple: treat empty as default "full".
	if s.Type == "" {
		s.Type = "full"
	}
	if s.Textures == nil {
		s.Textures = make(map[string]string)
	}
	return s
}

func resolveFaceTextures(blockID string, textures map[string]string) [6]string {
	// Default to block ID for all faces.
	all := blockID
	if v, ok := textures["all"]; ok && v != "" {
		all = v
	}

	side := all
	if v, ok := textures["side"]; ok && v != "" {
		side = v
	}

	t := [6]string{
		FaceRight:  side,
		FaceLeft:   side,
		FaceTop:    all,
		FaceBottom: all,
		FaceFront:  side,
		FaceBack:   side,
	}

	if v, ok := textures["right"]; ok && v != "" {
		t[FaceRight] = v
	}
	if v, ok := textures["left"]; ok && v != "" {
		t[FaceLeft] = v
	}
	if v, ok := textures["top"]; ok && v != "" {
		t[FaceTop] = v
	}
	if v, ok := textures["bottom"]; ok && v != "" {
		t[FaceBottom] = v
	}
	if v, ok := textures["front"]; ok && v != "" {
		t[FaceFront] = v
	}
	if v, ok := textures["back"]; ok && v != "" {
		t[FaceBack] = v
	}

	return t
}

// CompileModel compiles a YAML model spec into a runtime BlockModel.
// It always returns a non-nil model for non-air blocks.
func CompileModel(blockID string, spec ModelSpec) BlockModel {
	spec = spec.normalized()

	var base BlockModel
	switch spec.Type {
	case "sided":
		all := ""
		if v, ok := spec.Textures["all"]; ok {
			all = v
		}
		top := spec.Textures["top"]
		bottom := spec.Textures["bottom"]
		side := spec.Textures["side"]
		base = NewSidedBlock(blockID, all, top, bottom, side)
	case "slab":
		base = NewSlabBlock(resolveFaceTextures(blockID, spec.Textures))
	default: // "full" and unknown values
		base = NewFullBlock(resolveFaceTextures(blockID, spec.Textures))
	}

	if spec.Orientable {
		return NewOrientable(base)
	}
	return base
}

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

const (
	// MetaFacingMask uses the lowest 2 bits to represent 4-way horizontal facing.
	MetaFacingMask uint8 = 0b00000011
	// MetaSlabTopBit indicates a slab occupies the top half of the voxel (y=0.5..1).
	MetaSlabTopBit uint8 = 0b00000100
)

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

// Orientable rotates a wrapped model around the Y axis based on the 2-bit facing in meta.
// It uses 4-way rotation (0, 90, 180, 270 degrees).
type Orientable struct {
	base BlockModel
}

// NewOrientable wraps a base model.
func NewOrientable(base BlockModel) *Orientable {
	return &Orientable{base: base}
}

func (m *Orientable) AppendQuads(dst []Quad, meta uint8) []Quad {
	tmp := m.base.AppendQuads(nil, meta)
	steps := int(meta & MetaFacingMask) // 0..3
	for _, q := range tmp {
		dst = append(dst, rotateQuadY(q, steps))
	}
	return dst
}

func (m *Orientable) Occludes(meta uint8, face Face, region Rect) bool {
	steps := int(meta & MetaFacingMask)
	baseFace := rotateFaceY(face, (4-steps)%4)
	// NOTE: For now we keep region unchanged. This is sufficient for the current
	// shipped models (full blocks and slabs), whose occlusion on vertical faces
	// does not depend on U and is invariant under horizontal mirroring.
	return m.base.Occludes(meta, baseFace, region)
}

func (m *Orientable) Textures() []string {
	return m.base.Textures()
}

func rotateFaceY(face Face, steps int) Face {
	steps %= 4
	if steps == 0 {
		return face
	}

	switch face {
	case FaceTop, FaceBottom:
		return face
	}

	for i := 0; i < steps; i++ {
		switch face {
		case FaceFront:
			face = FaceRight
		case FaceRight:
			face = FaceBack
		case FaceBack:
			face = FaceLeft
		default: // FaceLeft
			face = FaceFront
		}
	}
	return face
}

func rotateQuadY(q Quad, steps int) Quad {
	steps %= 4
	if steps == 0 {
		return q
	}

	q.Face = rotateFaceY(q.Face, steps)

	q.V1 = rotateVecY(q.V1, steps)
	q.V2 = rotateVecY(q.V2, steps)
	q.V3 = rotateVecY(q.V3, steps)
	q.V4 = rotateVecY(q.V4, steps)

	return q
}

func rotateVecY(v Vec3, steps int) Vec3 {
	steps %= 4
	if steps == 0 {
		return v
	}

	// Rotate around center (0.5, 0.5, 0.5).
	x, z := v.X-0.5, v.Z-0.5

	switch steps {
	case 1:
		// +90: (x,z) -> (z, -x)
		x, z = z, -x
	case 2:
		// 180: (x,z) -> (-x, -z)
		x, z = -x, -z
	case 3:
		// 270: (x,z) -> (-z, x)
		x, z = -z, x
	}

	return Vec3{X: x + 0.5, Y: v.Y, Z: z + 0.5}
}

func uniqueStrings(in []string) []string {
	set := make(map[string]struct{}, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
