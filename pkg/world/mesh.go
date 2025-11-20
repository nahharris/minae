package world

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

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
func CalculateChunkMesh(chunk *Chunk, world *World) *ChunkMeshData {
	var vertices []float32
	var texcoords []float32
	var normals []float32
	var colors []uint8

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
	for x := 0; x < ChunkWidth; x++ {
		for y := 0; y < ChunkHeight; y++ {
			for z := 0; z < ChunkWidth; z++ {
				block := chunk.GetBlock(x, y, z)
				if block == BlockAir {
					continue
				}

				// Determine color
				var color rl.Color
				if block == BlockStone {
					color = rl.Gray
				} else if block == BlockDirt {
					color = rl.Brown
				} else {
					color = rl.Pink
				}

				// Check neighbors
				gx, gy, gz := chunk.X*ChunkWidth+x, y, chunk.Z*ChunkWidth+z

				checkNeighbor := func(dx, dy, dz int, nx, ny, nz float32) {
					neighbor := world.GetBlock(gx+dx, gy+dy, gz+dz)
					if neighbor == BlockAir {
						addFace(x, y, z, rl.NewVector3(nx, ny, nz), color)
					}
				}

				checkNeighbor(0, 1, 0, 0, 1, 0)  // Top
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

	return &ChunkMeshData{
		Vertices:  vertices,
		Texcoords: texcoords,
		Normals:   normals,
		Colors:    colors,
	}
}

// GenerateChunkMesh generates and uploads a Raylib mesh.
func GenerateChunkMesh(chunk *Chunk, world *World) *rl.Mesh {
	data := CalculateChunkMesh(chunk, world)
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
