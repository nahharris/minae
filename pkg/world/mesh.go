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

// meshDataPool reuses ChunkMeshData structs to reduce allocations.
var meshDataPool = sync.Pool{
	New: func() interface{} {
		return &ChunkMeshData{
			Vertices:  make([]float32, 0, 65536), // Max ~65k vertices for full chunk (16*16*256*6 faces worst case)
			Texcoords: make([]float32, 0, 65536),
			Normals:   make([]float32, 0, 65536),
			Colors:    make([]uint8, 0, 87381), // RGBA per vertex
		}
	},
}

// Pre-computed face vertex offsets to avoid allocations
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

	// Pre-computed normal vectors to avoid allocations
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
	// Get mesh data from pool and reset slices
	data := meshDataPool.Get().(*ChunkMeshData)
	data.Vertices = data.Vertices[:0]
	data.Texcoords = data.Texcoords[:0]
	data.Normals = data.Normals[:0]
	data.Colors = data.Colors[:0]

	vertices := data.Vertices
	texcoords := data.Texcoords
	normals := data.Normals
	colors := data.Colors

	// Helper to add a face
	addFace := func(x, y, z int, normal rl.Vector3, color rl.Color) {
		// Base coordinates
		fx, fy, fz := float32(x), float32(y), float32(z)

		var v1, v2, v3, v4 rl.Vector3

		if normal.X == 1 { // Right
			v1 = faceRightV1
			v2 = faceRightV2
			v3 = faceRightV3
			v4 = faceRightV4
		} else if normal.X == -1 { // Left
			v1 = faceLeftV1
			v2 = faceLeftV2
			v3 = faceLeftV3
			v4 = faceLeftV4
		} else if normal.Y == 1 { // Top
			v1 = faceTopV1
			v2 = faceTopV2
			v3 = faceTopV3
			v4 = faceTopV4
		} else if normal.Y == -1 { // Bottom
			v1 = faceBottomV1
			v2 = faceBottomV2
			v3 = faceBottomV3
			v4 = faceBottomV4
		} else if normal.Z == 1 { // Front
			v1 = faceFrontV1
			v2 = faceFrontV2
			v3 = faceFrontV3
			v4 = faceFrontV4
		} else if normal.Z == -1 { // Back
			v1 = faceBackV1
			v2 = faceBackV2
			v3 = faceBackV3
			v4 = faceBackV4
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
				// Treat nil or air blocks as transparent/non-visible
				if block == nil || block == blocks.Air {
					continue
				}

				// Determine color from block definition
				color := rl.GetColor(uint(block.Color))

				// Check neighbors
				gx, gy, gz := chunk.X*config.ChunkWidth+x, y, chunk.Z*config.ChunkWidth+z

				checkNeighbor := func(dx, dy, dz int, normal rl.Vector3) {
					neighbor := world.GetBlock(gx+dx, gy+dy, gz+dz)
					// If neighbor is air (or nil), we draw the face
					if neighbor == nil || neighbor == blocks.Air {
						addFace(x, y, z, normal, color)
					}
				}

				checkNeighbor(0, 1, 0, normalTop)    // Top
				checkNeighbor(0, -1, 0, normalBottom) // Bottom
				checkNeighbor(-1, 0, 0, normalLeft)  // Left
				checkNeighbor(1, 0, 0, normalRight)  // Right
				checkNeighbor(0, 0, 1, normalFront)  // Front
				checkNeighbor(0, 0, -1, normalBack)  // Back
			}
		}
	}

	if len(vertices) == 0 {
		// Return to pool if empty
		meshDataPool.Put(data)
		return nil
	}

	// Update data struct with final slices
	data.Vertices = vertices
	data.Texcoords = texcoords
	data.Normals = normals
	data.Colors = colors

	return data
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

	// Free mesh data immediately after upload - GPU has the data now
	meshDataPool.Put(data)

	return &mesh
}
