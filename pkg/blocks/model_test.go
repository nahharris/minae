package blocks

import "testing"

func TestSlabBlock_AppendQuads_Heights(t *testing.T) {
	slab := NewSlabBlock(resolveFaceTextures("test/slab", nil))

	t.Run("bottom", func(t *testing.T) {
		quads := slab.AppendQuads(nil, 0)
		minY, maxY := float32(1e9), float32(-1e9)
		for _, q := range quads {
			for _, v := range [...]Vec3{q.V1, q.V2, q.V3, q.V4} {
				if v.Y < minY {
					minY = v.Y
				}
				if v.Y > maxY {
					maxY = v.Y
				}
			}
		}
		if minY != 0 || maxY != 0.5 {
			t.Fatalf("expected bottom slab y range [0,0.5], got [%.3f, %.3f]", minY, maxY)
		}
	})

	t.Run("top", func(t *testing.T) {
		quads := slab.AppendQuads(nil, MetaSlabTopBit)
		minY, maxY := float32(1e9), float32(-1e9)
		for _, q := range quads {
			for _, v := range [...]Vec3{q.V1, q.V2, q.V3, q.V4} {
				if v.Y < minY {
					minY = v.Y
				}
				if v.Y > maxY {
					maxY = v.Y
				}
			}
		}
		if minY != 0.5 || maxY != 1 {
			t.Fatalf("expected top slab y range [0.5,1], got [%.3f, %.3f]", minY, maxY)
		}
	})
}

func TestSlabBlock_Occludes_VerticalFaces(t *testing.T) {
	slab := NewSlabBlock(resolveFaceTextures("test/slab", nil))

	bottomHalf := Rect{MinU: 0, MinV: 0, MaxU: 1, MaxV: 0.5}
	topHalf := Rect{MinU: 0, MinV: 0.5, MaxU: 1, MaxV: 1}

	if !slab.Occludes(0, FaceRight, bottomHalf) {
		t.Fatal("expected bottom slab to occlude bottom half on vertical faces")
	}
	if slab.Occludes(0, FaceRight, topHalf) {
		t.Fatal("expected bottom slab NOT to occlude top half on vertical faces")
	}

	if !slab.Occludes(MetaSlabTopBit, FaceRight, topHalf) {
		t.Fatal("expected top slab to occlude top half on vertical faces")
	}
	if slab.Occludes(MetaSlabTopBit, FaceRight, bottomHalf) {
		t.Fatal("expected top slab NOT to occlude bottom half on vertical faces")
	}
}

func TestOrientable_RotatesFaceTextures(t *testing.T) {
	tex := resolveFaceTextures("test/block", map[string]string{
		"front":  "front",
		"back":   "back",
		"left":   "left",
		"right":  "right",
		"top":    "top",
		"bottom": "bottom",
	})

	base := NewFullBlock(tex)
	o := NewOrientable(base)

	// steps=0: front stays front
	qs := o.AppendQuads(nil, 0)
	if got := textureForFace(qs, FaceFront); got != "front" {
		t.Fatalf("steps=0: expected front texture 'front', got %q", got)
	}

	// steps=1: base left rotates to front
	qs = o.AppendQuads(nil, 1)
	if got := textureForFace(qs, FaceFront); got != "left" {
		t.Fatalf("steps=1: expected front texture 'left', got %q", got)
	}

	// steps=2: base back rotates to front
	qs = o.AppendQuads(nil, 2)
	if got := textureForFace(qs, FaceFront); got != "back" {
		t.Fatalf("steps=2: expected front texture 'back', got %q", got)
	}

	// steps=3: base right rotates to front
	qs = o.AppendQuads(nil, 3)
	if got := textureForFace(qs, FaceFront); got != "right" {
		t.Fatalf("steps=3: expected front texture 'right', got %q", got)
	}
}

func textureForFace(quads []Quad, face Face) string {
	for _, q := range quads {
		if q.Face == face {
			return q.Texture
		}
	}
	return ""
}
