package world

import (
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
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

// Pre-computed face vertex offsets and normals to avoid allocations.
var (
	faceRightV1 = rl.Vector3{X: 1, Y: 0, Z: 1}
	faceRightV2 = rl.Vector3{X: 1, Y: 0, Z: 0}
	faceRightV3 = rl.Vector3{X: 1, Y: 1, Z: 0}
	faceRightV4 = rl.Vector3{X: 1, Y: 1, Z: 1}

	faceLeftV1 = rl.Vector3{X: 0, Y: 0, Z: 0}
	faceLeftV2 = rl.Vector3{X: 0, Y: 0, Z: 1}
	faceLeftV3 = rl.Vector3{X: 0, Y: 1, Z: 1}
	faceLeftV4 = rl.Vector3{X: 0, Y: 1, Z: 0}

	faceTopV1 = rl.Vector3{X: 0, Y: 1, Z: 1}
	faceTopV2 = rl.Vector3{X: 1, Y: 1, Z: 1}
	faceTopV3 = rl.Vector3{X: 1, Y: 1, Z: 0}
	faceTopV4 = rl.Vector3{X: 0, Y: 1, Z: 0}

	faceBottomV1 = rl.Vector3{X: 0, Y: 0, Z: 0}
	faceBottomV2 = rl.Vector3{X: 1, Y: 0, Z: 0}
	faceBottomV3 = rl.Vector3{X: 1, Y: 0, Z: 1}
	faceBottomV4 = rl.Vector3{X: 0, Y: 0, Z: 1}

	faceFrontV1 = rl.Vector3{X: 0, Y: 0, Z: 1}
	faceFrontV2 = rl.Vector3{X: 1, Y: 0, Z: 1}
	faceFrontV3 = rl.Vector3{X: 1, Y: 1, Z: 1}
	faceFrontV4 = rl.Vector3{X: 0, Y: 1, Z: 1}

	faceBackV1 = rl.Vector3{X: 1, Y: 0, Z: 0}
	faceBackV2 = rl.Vector3{X: 0, Y: 0, Z: 0}
	faceBackV3 = rl.Vector3{X: 0, Y: 1, Z: 0}
	faceBackV4 = rl.Vector3{X: 1, Y: 1, Z: 0}

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
func CalculateChunkMesh(chunk *Chunk, world *World) *ChunkMeshData {
	builder := buildChunkMesh(chunk, world)
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

func buildChunkMesh(chunk *Chunk, world *World) *meshBuilder {
	builder := meshBuilderPool.Get().(*meshBuilder)
	builder.reset()
	builder.ensureCapacity(chunk.meshHint)

	addFace := func(x, y, z int, normal rl.Vector3, color rl.Color) {
		fx, fy, fz := float32(x), float32(y), float32(z)

		var v1, v2, v3, v4 rl.Vector3

		switch normal {
		case normalRight:
			v1, v2, v3, v4 = faceRightV1, faceRightV2, faceRightV3, faceRightV4
		case normalLeft:
			v1, v2, v3, v4 = faceLeftV1, faceLeftV2, faceLeftV3, faceLeftV4
		case normalTop:
			v1, v2, v3, v4 = faceTopV1, faceTopV2, faceTopV3, faceTopV4
		case normalBottom:
			v1, v2, v3, v4 = faceBottomV1, faceBottomV2, faceBottomV3, faceBottomV4
		case normalFront:
			v1, v2, v3, v4 = faceFrontV1, faceFrontV2, faceFrontV3, faceFrontV4
		default: // normalBack
			v1, v2, v3, v4 = faceBackV1, faceBackV2, faceBackV3, faceBackV4
		}

		builder.vertices = append(builder.vertices,
			fx+v1.X, fy+v1.Y, fz+v1.Z,
			fx+v2.X, fy+v2.Y, fz+v2.Z,
			fx+v3.X, fy+v3.Y, fz+v3.Z,
			fx+v1.X, fy+v1.Y, fz+v1.Z,
			fx+v3.X, fy+v3.Y, fz+v3.Z,
			fx+v4.X, fy+v4.Y, fz+v4.Z,
		)

		for range 6 {
			builder.normals = append(builder.normals, normal.X, normal.Y, normal.Z)
			builder.colors = append(builder.colors, color.R, color.G, color.B, color.A)
		}

		builder.texcoords = append(builder.texcoords,
			0, 0, 1, 0, 1, 1,
			0, 0, 1, 1, 0, 1,
		)
	}

	for x := range config.ChunkWidth {
		for y := range config.ChunkHeight {
			for z := range config.ChunkWidth {
				block := chunk.GetBlock(x, y, z)
				if block == nil || isAir(block) {
					continue
				}

				color := rl.GetColor(uint(block.Color))
				gx, gy, gz := chunk.X*config.ChunkWidth+x, y, chunk.Z*config.ChunkWidth+z

				checkNeighbor := func(dx, dy, dz int, normal rl.Vector3) {
					neighbor := world.GetBlock(gx+dx, gy+dy, gz+dz)
					if neighbor == nil || isAir(neighbor) {
						// Get light level of the air block we are facing
						light := world.GetLight(gx+dx, gy+dy, gz+dz)
						// Map 0-15 to 0-255 using integer arithmetic to avoid precision issues
						alpha := uint8((uint16(light) * 255) / 15)
						faceColor := color
						faceColor.A = alpha
						addFace(x, y, z, normal, faceColor)
					}
				}

				checkNeighbor(0, 1, 0, normalTop)
				checkNeighbor(0, -1, 0, normalBottom)
				checkNeighbor(-1, 0, 0, normalLeft)
				checkNeighbor(1, 0, 0, normalRight)
				checkNeighbor(0, 0, 1, normalFront)
				checkNeighbor(0, 0, -1, normalBack)
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

// isAir checks if a block is essentially air.
func isAir(b *blocks.Block) bool {
	return b == nil || b.ID == "minae/air" || b.Name == "Air"
}

// GenerateChunkMesh generates and uploads a Raylib mesh.
func GenerateChunkMesh(chunk *Chunk, world *World) *rl.Mesh {
	builder := buildChunkMesh(chunk, world)
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
