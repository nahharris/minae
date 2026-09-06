package mesh

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/gfx/atlas"
)

// WorldReader provides world data access needed for mesh generation.
type WorldReader interface {
	GetBlock(x, y, z int) *blocks.Block
	GetBlockState(x, y, z int) (*blocks.Block, uint8)
	GetSkyLight(x, y, z int) uint8
	HasChunkAt(x, z int) bool
}

// ChunkReader provides access to a single chunk's data.
type ChunkReader interface {
	GetBlock(x, y, z int) *blocks.Block
	GetBlockState(x, y, z int) (*blocks.Block, uint8)
	ChunkX() int
	ChunkZ() int
}

// UVLookup provides atlas UV coordinates for a texture key.
type UVLookup interface {
	UV(key string) (atlas.UV, bool)
}

// ChunkMeshData holds the raw data for a chunk mesh.
type ChunkMeshData struct {
	Vertices  []float32
	Texcoords []float32
	Normals   []float32
	Colors    []uint8
}

// GenerateChunkMeshData generates the mesh data for the given chunk.
// It performs face culling to remove invisible faces.
// world: used to check neighbors across chunk boundaries.
func GenerateChunkMeshData(chunk ChunkReader, world WorldReader, uvLookup UVLookup) *ChunkMeshData {
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

// UploadMesh uploads the mesh data to the GPU and returns a Raylib mesh.
// This must be called from the main thread.
func (d *ChunkMeshData) Upload() *rl.Mesh {
	if d == nil || len(d.Vertices) == 0 {
		return nil
	}

	mesh := rl.Mesh{}
	mesh.VertexCount = int32(len(d.Vertices) / 3)
	mesh.TriangleCount = mesh.VertexCount / 3

	// We need to copy data because Raylib might not keep references or we might want to reuse buffers.
	// In Go raylib bindings, we usually pass slices.
	// Ideally we should use the specialized upload functions if available or just assign fields.
	// Since the fields in rl.Mesh are *float32, we need to be careful about pointer validity.
	// But d.Vertices is a slice, taking &d.Vertices[0] is safe as long as d exists.
	// However, UploadMesh copies data to GPU.

	mesh.Vertices = &d.Vertices[0]
	mesh.Normals = &d.Normals[0]
	mesh.Texcoords = &d.Texcoords[0]
	mesh.Colors = &d.Colors[0]

	rl.UploadMesh(&mesh, false)

	return &mesh
}
