package lighting

import _ "embed"

//go:embed shaders/vertex.glsl
var VsCode string

//go:embed shaders/fragment.glsl
var FsCode string
