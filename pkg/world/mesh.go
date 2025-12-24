package world

import (
	"math"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/render/atlas"
)

// ChunkMeshData holds the raw data for a chunk mesh.
type ChunkMeshData struct {
	Vertices  []float32
	Texcoords []float32
	Normals   []float32
	Colors    []uint8
}

type meshBuilder struct {
	vertices  []float32
	texcoords []float32
	normals   []float32
	colors    []uint8
}

func (b *meshBuilder) reset() {
	b.vertices = b.vertices[:0]
	b.texcoords = b.texcoords[:0]
	b.normals = b.normals[:0]
	b.colors = b.colors[:0]
}

func (b *meshBuilder) ensureCapacity(h chunkMeshHint) {
	if h.vertices > 0 && cap(b.vertices) < h.vertices {
		b.vertices = make([]float32, 0, h.vertices)
	}
	if h.texcoords > 0 && cap(b.texcoords) < h.texcoords {
		b.texcoords = make([]float32, 0, h.texcoords)
	}
	if h.normals > 0 && cap(b.normals) < h.normals {
		b.normals = make([]float32, 0, h.normals)
	}
	if h.colors > 0 && cap(b.colors) < h.colors {
		b.colors = make([]uint8, 0, h.colors)
	}
}

func (b *meshBuilder) release() {
	b.reset()
	meshBuilderPool.Put(b)
}

var meshBuilderPool = sync.Pool{
	New: func() any {
		return &meshBuilder{}
	},
}

// UVLookup provides atlas UV coordinates for a texture key.
type UVLookup interface {
	UV(key string) (atlas.UV, bool)
}

// Pre-computed face normals to avoid allocations.
var (
	normalTop    = rl.Vector3{X: 0, Y: 1, Z: 0}
	normalBottom = rl.Vector3{X: 0, Y: -1, Z: 0}
	normalLeft   = rl.Vector3{X: -1, Y: 0, Z: 0}
	normalRight  = rl.Vector3{X: 1, Y: 0, Z: 0}
	normalFront  = rl.Vector3{X: 0, Y: 0, Z: 1}
	normalBack   = rl.Vector3{X: 0, Y: 0, Z: -1}
)

// CalculateChunkMesh generates the mesh data for the given chunk.
// It performs face culling to remove invisible faces.
// world: used to check neighbors across chunk boundaries.
func CalculateChunkMesh(chunk *Chunk, world *World, uvLookup UVLookup) *ChunkMeshData {
	builder := buildChunkMesh(chunk, world, uvLookup)
	if builder == nil {
		return nil
	}

	data := &ChunkMeshData{
		Vertices:  append([]float32(nil), builder.vertices...),
		Texcoords: append([]float32(nil), builder.texcoords...),
		Normals:   append([]float32(nil), builder.normals...),
		Colors:    append([]uint8(nil), builder.colors...),
	}

	builder.release()
	return data
}

func buildChunkMesh(chunk *Chunk, world *World, uvLookup UVLookup) *meshBuilder {
	builder := meshBuilderPool.Get().(*meshBuilder)
	builder.reset()
	builder.ensureCapacity(chunk.meshHint)

	addQuad := func(x, y, z int, q blocks.Quad, alpha uint8, uv atlas.UV) {
		fx, fy, fz := float32(x), float32(y), float32(z)

		n := normalForFace(q.Face)

		p1, p2, p3, p4 := q.V1, q.V2, q.V3, q.V4
		builder.vertices = append(builder.vertices,
			fx+p1.X, fy+p1.Y, fz+p1.Z,
			fx+p2.X, fy+p2.Y, fz+p2.Z,
			fx+p3.X, fy+p3.Y, fz+p3.Z,
			fx+p1.X, fy+p1.Y, fz+p1.Z,
			fx+p3.X, fy+p3.Y, fz+p3.Z,
			fx+p4.X, fy+p4.Y, fz+p4.Z,
		)

		for range 6 {
			builder.normals = append(builder.normals, n.X, n.Y, n.Z)
			// RGB is white so textures show true colors; alpha encodes skylight (0..255).
			builder.colors = append(builder.colors, 255, 255, 255, alpha)
		}

		u1, v1 := uvForVertex(q.Face, p1, uv)
		u2, v2 := uvForVertex(q.Face, p2, uv)
		u3, v3 := uvForVertex(q.Face, p3, uv)
		u4, v4 := uvForVertex(q.Face, p4, uv)

		builder.texcoords = append(builder.texcoords,
			u1, v1, u2, v2, u3, v3,
			u1, v1, u3, v3, u4, v4,
		)
	}

	quads := make([]blocks.Quad, 0, 16)

	for x := range config.ChunkWidth {
		for y := range config.ChunkHeight {
			for z := range config.ChunkWidth {
				block, meta := chunk.GetBlockState(x, y, z)
				if block == nil {
					continue
				}

				model := block.Model
				if model == nil {
					model = blocks.CompileModel(block.ID, block.ModelSpec)
				}

				gx, gy, gz := chunk.X*config.ChunkWidth+x, y, chunk.Z*config.ChunkWidth+z

				quads = model.AppendQuads(quads[:0], meta)
				for _, q := range quads {
					dx, dy, dz := offsetForFace(q.Face)
					light := world.GetLight(gx+dx, gy+dy, gz+dz)
					alpha := uint8((uint16(light) * 255) / 15)

					if q.Cull {
						neighbor, nmeta := world.GetBlockState(gx+dx, gy+dy, gz+dz)
						if neighbor != nil {
							nmodel := neighbor.Model
							if nmodel == nil {
								nmodel = blocks.CompileModel(neighbor.ID, neighbor.ModelSpec)
							}

							region := quadRegion(q)
							if nmodel.Occludes(nmeta, q.Face.Opposite(), region) {
								continue
							}
						}
					}

					uv := atlas.UV{U0: 0, V0: 0, U1: 1, V1: 1}
					if uvLookup != nil && q.Texture != "" {
						if r, ok := uvLookup.UV(q.Texture); ok {
							uv = r
						}
					}

					addQuad(x, y, z, q, alpha, uv)
				}
			}
		}
	}

	if len(builder.vertices) == 0 {
		chunk.meshHint = chunkMeshHint{}
		builder.release()
		return nil
	}

	chunk.meshHint = chunkMeshHint{
		vertices:  len(builder.vertices),
		texcoords: len(builder.texcoords),
		normals:   len(builder.normals),
		colors:    len(builder.colors),
	}

	return builder
}

// GenerateChunkMesh generates and uploads a Raylib mesh.
func GenerateChunkMesh(chunk *Chunk, world *World, uvLookup UVLookup) *rl.Mesh {
	builder := buildChunkMesh(chunk, world, uvLookup)
	if builder == nil {
		return nil
	}

	mesh := rl.Mesh{}
	mesh.VertexCount = int32(len(builder.vertices) / 3)
	mesh.TriangleCount = mesh.VertexCount / 3
	mesh.Vertices = &builder.vertices[0]
	mesh.Normals = &builder.normals[0]
	mesh.Texcoords = &builder.texcoords[0]
	mesh.Colors = &builder.colors[0]

	rl.UploadMesh(&mesh, false)

	// Allow GC/pool reuse without keeping references from the mesh struct.
	mesh.Vertices = nil
	mesh.Normals = nil
	mesh.Texcoords = nil
	mesh.Colors = nil

	builder.release()
	return &mesh
}

func normalForFace(face blocks.Face) rl.Vector3 {
	switch face {
	case blocks.FaceRight:
		return normalRight
	case blocks.FaceLeft:
		return normalLeft
	case blocks.FaceTop:
		return normalTop
	case blocks.FaceBottom:
		return normalBottom
	case blocks.FaceFront:
		return normalFront
	default:
		return normalBack
	}
}

func offsetForFace(face blocks.Face) (dx, dy, dz int) {
	switch face {
	case blocks.FaceRight:
		return 1, 0, 0
	case blocks.FaceLeft:
		return -1, 0, 0
	case blocks.FaceTop:
		return 0, 1, 0
	case blocks.FaceBottom:
		return 0, -1, 0
	case blocks.FaceFront:
		return 0, 0, 1
	default:
		return 0, 0, -1
	}
}

func quadRegion(q blocks.Quad) blocks.Rect {
	minU, minV := float32(math.Inf(1)), float32(math.Inf(1))
	maxU, maxV := float32(math.Inf(-1)), float32(math.Inf(-1))

	vs := [4]blocks.Vec3{q.V1, q.V2, q.V3, q.V4}
	for _, v := range vs {
		var u, w float32
		switch q.Face {
		case blocks.FaceRight, blocks.FaceLeft:
			u, w = v.Z, v.Y
		case blocks.FaceFront, blocks.FaceBack:
			u, w = v.X, v.Y
		default: // Top/Bottom
			u, w = v.X, v.Z
		}

		if u < minU {
			minU = u
		}
		if w < minV {
			minV = w
		}
		if u > maxU {
			maxU = u
		}
		if w > maxV {
			maxV = w
		}
	}

	if math.IsInf(float64(minU), 1) || math.IsInf(float64(minV), 1) {
		return blocks.Rect{}
	}

	return blocks.Rect{MinU: minU, MinV: minV, MaxU: maxU, MaxV: maxV}
}

func uvForVertex(face blocks.Face, v blocks.Vec3, r atlas.UV) (u, vt float32) {
	localU, localV := float32(0), float32(0)

	// Convention:
	// - Texture origin is top-left (V increases downward).
	// - On vertical faces, V=0 maps to world-up (Y=1), V=1 maps to world-down (Y=0).
	switch face {
	case blocks.FaceFront: // +Z
		localU = 1 - v.X
		localV = 1 - v.Y
	case blocks.FaceBack: // -Z
		localU = v.X
		localV = 1 - v.Y
	case blocks.FaceRight: // +X
		localU = v.Z
		localV = 1 - v.Y
	case blocks.FaceLeft: // -X
		localU = 1 - v.Z
		localV = 1 - v.Y
	case blocks.FaceTop: // +Y
		localU = v.X
		localV = v.Z
	default: // blocks.FaceBottom (-Y)
		localU = v.X
		localV = 1 - v.Z
	}

	// Clamp to avoid tiny floating errors from rotations.
	if localU < 0 {
		localU = 0
	} else if localU > 1 {
		localU = 1
	}
	if localV < 0 {
		localV = 0
	} else if localV > 1 {
		localV = 1
	}

	u = r.U0 + (r.U1-r.U0)*localU
	vt = r.V0 + (r.V1-r.V0)*localV
	return u, vt
}
