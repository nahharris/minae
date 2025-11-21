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

	// Helper to add a face
	addFace := func(x, y, z int, normal rl.Vector3, color rl.Color) {
		// Base coordinates
		fx, fy, fz := float32(x), float32(y), float32(z)

		var v1, v2, v3, v4 rl.Vector3

		if normal.X == 1 { // Right
			v1 = rl.NewVector3(1, 0, 1)
			v2 = rl.NewVector3(1, 0, 0)
			v3 = rl.NewVector3(1, 1, 0)
			v4 = rl.NewVector3(1, 1, 1)
		} else if normal.X == -1 { // Left
			v1 = rl.NewVector3(0, 0, 0)
			v2 = rl.NewVector3(0, 0, 1)
			v3 = rl.NewVector3(0, 1, 1)
			v4 = rl.NewVector3(0, 1, 0)
		} else if normal.Y == 1 { // Top
			v1 = rl.NewVector3(0, 1, 1)
			v2 = rl.NewVector3(1, 1, 1)
			v3 = rl.NewVector3(1, 1, 0)
			v4 = rl.NewVector3(0, 1, 0)
		} else if normal.Y == -1 { // Bottom
			v1 = rl.NewVector3(0, 0, 0)
			v2 = rl.NewVector3(1, 0, 0)
			v3 = rl.NewVector3(1, 0, 1)
			v4 = rl.NewVector3(0, 0, 1)
		} else if normal.Z == 1 { // Front
			v1 = rl.NewVector3(0, 0, 1)
			v2 = rl.NewVector3(1, 0, 1)
			v3 = rl.NewVector3(1, 1, 1)
			v4 = rl.NewVector3(0, 1, 1)
		} else if normal.Z == -1 { // Back
			v1 = rl.NewVector3(1, 0, 0)
			v2 = rl.NewVector3(0, 0, 0)
			v3 = rl.NewVector3(0, 1, 0)
			v4 = rl.NewVector3(1, 1, 0)
		}

		// T1
		builder.vertices = append(builder.vertices, fx+v1.X, fy+v1.Y, fz+v1.Z)
		builder.vertices = append(builder.vertices, fx+v2.X, fy+v2.Y, fz+v2.Z)
		builder.vertices = append(builder.vertices, fx+v3.X, fy+v3.Y, fz+v3.Z)

		// T2
		builder.vertices = append(builder.vertices, fx+v1.X, fy+v1.Y, fz+v1.Z)
		builder.vertices = append(builder.vertices, fx+v3.X, fy+v3.Y, fz+v3.Z)
		builder.vertices = append(builder.vertices, fx+v4.X, fy+v4.Y, fz+v4.Z)

		// Normals
		for i := 0; i < 6; i++ {
			builder.normals = append(builder.normals, normal.X, normal.Y, normal.Z)
		}

		// Colors
		for i := 0; i < 6; i++ {
			builder.colors = append(builder.colors, color.R, color.G, color.B, color.A)
		}

		// Texcoords (dummy for now)
		builder.texcoords = append(builder.texcoords, 0, 0, 1, 0, 1, 1, 0, 0, 1, 1, 0, 1)
	}

	// Loop through all blocks
	for x := range config.ChunkWidth {
		for y := range config.ChunkHeight {
			for z := range config.ChunkWidth {
				block := chunk.GetBlock(x, y, z)
				// Treat nil or blocks with "air" ID as transparent/non-visible
				if block == nil || isAir(block) {
					continue
				}

				// Determine color from block definition
				color := rl.GetColor(uint(block.Color))

				// Check neighbors
				gx, gy, gz := chunk.X*config.ChunkWidth+x, y, chunk.Z*config.ChunkWidth+z

				checkNeighbor := func(dx, dy, dz int, nx, ny, nz float32) {
					neighbor := world.GetBlock(gx+dx, gy+dy, gz+dz)
					// If neighbor is air (or nil), we draw the face
					if neighbor == nil || isAir(neighbor) {
						addFace(x, y, z, rl.NewVector3(nx, ny, nz), color)
					}
				}

				checkNeighbor(0, 1, 0, 0, 1, 0)   // Top
				checkNeighbor(0, -1, 0, 0, -1, 0) // Bottom
				checkNeighbor(-1, 0, 0, -1, 0, 0) // Left
				checkNeighbor(1, 0, 0, 1, 0, 0)   // Right
				checkNeighbor(0, 0, 1, 0, 0, 1)   // Front
				checkNeighbor(0, 0, -1, 0, 0, -1) // Back
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

// isAir checks if a block is essentially air
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
