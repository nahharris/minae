package world

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
)

// MeshBuffer is a reusable buffer for mesh generation to avoid allocations.
type MeshBuffer struct {
	Vertices  []float32
	Texcoords []float32
	Normals   []float32
	Colors    []uint8
}

// Reset clears the buffer for reuse while keeping capacity.
func (m *MeshBuffer) Reset() {
	m.Vertices = m.Vertices[:0]
	m.Texcoords = m.Texcoords[:0]
	m.Normals = m.Normals[:0]
	m.Colors = m.Colors[:0]
}

// ChunkMeshData holds the raw data for a chunk mesh.
type ChunkMeshData struct {
	Vertices  []float32
	Texcoords []float32
	Normals   []float32
	Colors    []uint8
}

// CalculateChunkMesh generates the mesh data for the given chunk.
// It performs face culling to remove invisible faces.
// world: used to check neighbors across chunk boundaries.
// buffer: optional buffer to reuse. If nil, new slices are allocated.
func CalculateChunkMesh(chunk *Chunk, world *World, buffer *MeshBuffer) *ChunkMeshData {
	var vertices []float32
	var texcoords []float32
	var normals []float32
	var colors []uint8

	if buffer != nil {
		buffer.Reset()
		vertices = buffer.Vertices
		texcoords = buffer.Texcoords
		normals = buffer.Normals
		colors = buffer.Colors
	}

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
		vertices = append(vertices, fx+v1.X, fy+v1.Y, fz+v1.Z)
		vertices = append(vertices, fx+v2.X, fy+v2.Y, fz+v2.Z)
		vertices = append(vertices, fx+v3.X, fy+v3.Y, fz+v3.Z)

		// T2
		vertices = append(vertices, fx+v1.X, fy+v1.Y, fz+v1.Z)
		vertices = append(vertices, fx+v3.X, fy+v3.Y, fz+v3.Z)
		vertices = append(vertices, fx+v4.X, fy+v4.Y, fz+v4.Z)

		// Normals
		for i := 0; i < 6; i++ {
			normals = append(normals, normal.X, normal.Y, normal.Z)
		}

		// Colors
		for i := 0; i < 6; i++ {
			colors = append(colors, color.R, color.G, color.B, color.A)
		}

		// Texcoords (dummy for now)
		texcoords = append(texcoords, 0, 0, 1, 0, 1, 1, 0, 0, 1, 1, 0, 1)
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

	if len(vertices) == 0 {
		return nil
	}

	if buffer != nil {
		buffer.Vertices = vertices
		buffer.Texcoords = texcoords
		buffer.Normals = normals
		buffer.Colors = colors
	}

	return &ChunkMeshData{
		Vertices:  vertices,
		Texcoords: texcoords,
		Normals:   normals,
		Colors:    colors,
	}
}

// isAir checks if a block is essentially air
func isAir(b *blocks.Block) bool {
	return b == nil || b.ID == "minae/air" || b.Name == "Air"
}

// GenerateChunkMesh generates and uploads a Raylib mesh.
func GenerateChunkMesh(chunk *Chunk, world *World, buffer *MeshBuffer) *rl.Mesh {
	data := CalculateChunkMesh(chunk, world, buffer)
	if data == nil {
		return nil
	}

	mesh := rl.Mesh{}
	mesh.VertexCount = int32(len(data.Vertices) / 3)
	mesh.TriangleCount = mesh.VertexCount / 3
	mesh.Vertices = &data.Vertices[0]
	mesh.Normals = &data.Normals[0]
	mesh.Texcoords = &data.Texcoords[0]
	mesh.Colors = &data.Colors[0]

	rl.UploadMesh(&mesh, false)
	return &mesh
}
