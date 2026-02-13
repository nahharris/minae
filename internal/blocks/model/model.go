package model

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

func (r Rect) Contains(o Rect) bool {
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

const (
	// MetaFacingMask uses the lowest 2 bits to represent 4-way horizontal facing.
	MetaFacingMask uint8 = 0b00000011
	// MetaSlabTopBit indicates a slab occupies the top half of the voxel (y=0.5..1).
	MetaSlabTopBit uint8 = 0b00000100
)
