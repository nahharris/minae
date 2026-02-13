package model

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

func normalizeModelSpec(s ModelSpec) ModelSpec {
	// Keep it simple: treat empty as default "full".
	if s.Type == "" {
		s.Type = "full"
	}
	if s.Textures == nil {
		s.Textures = make(map[string]string)
	}
	return s
}

// CompileModel compiles a YAML model spec into a runtime BlockModel.
// It always returns a non-nil model for non-air blocks.
func CompileModel(blockID string, spec ModelSpec) BlockModel {
	spec = normalizeModelSpec(spec)

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
