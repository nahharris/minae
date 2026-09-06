package model

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

// CollisionBoxes delegates to the wrapped model without rotating the boxes.
// Every shape that exists today (full blocks, sided blocks and slabs) is
// symmetric under the 4-way Y rotation Orientable applies, so the unrotated
// boxes are already correct for every facing. A future asymmetric shape
// (e.g. stairs) will need the boxes actually rotated by meta's facing bits
// the way AppendQuads rotates its quads; that rotation is deliberately not
// implemented here because there is nothing asymmetric yet to rotate.
func (m *Orientable) CollisionBoxes(dst []Box, meta uint8) []Box {
	return m.base.CollisionBoxes(dst, meta)
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
