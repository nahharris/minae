package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/core"
	"github.com/nahharris/minae/internal/gfx/atlas"
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/platform/logging"
	resource "github.com/nahharris/minae/internal/platform/resources"
	"github.com/nahharris/minae/internal/world"
	"github.com/sirupsen/logrus"
)

// SceneRenderer handles the 3D rendering of the world.
type SceneRenderer struct {
	Shader   rl.Shader
	Material rl.Material
	Atlas    *atlas.Atlas

	ChunkMeshes map[world.ChunkCoord]*rl.Mesh

	// Shader Uniform Locations
	LocSkyTint    int32
	LocBlockTint  int32
	LocMinAmbient int32

	// Reusable shader value buffers, to avoid allocating every frame.
	shaderSkyTint    []float32
	shaderBlockTint  []float32
	shaderMinAmbient []float32

	Log *logrus.Entry
}

// Shader uniform names. These are looked up by string at runtime: a mismatch
// between these and the GLSL source makes GetShaderLocation return -1 and
// SetShaderValue silently do nothing, leaving the world lit by whatever the
// uniform defaults to. shader_uniforms_test.go pins them to the actual source.
const (
	uniformSkyTint    = "skyTint"
	uniformBlockTint  = "blockTint"
	uniformMinAmbient = "minAmbient"
	uniformTexture0   = "texture0"
)

// blockTint is the colour of light emitted by blocks. Nothing writes the block
// light channel yet, so this has no visible effect; it is here so the shader
// has a defined value and torches can light warm when they arrive.
var blockTint = core.RGB{R: 1.0, G: 0.85, B: 0.6}

// minAmbient keeps a sealed cave dim rather than pure black, so the player can
// still make out geometry. A playability floor, not a physical term.
const minAmbient = 0.035

// NewSceneRenderer creates a new SceneRenderer using loaded resources.
func NewSceneRenderer(res *resource.Resources) *SceneRenderer {
	r := &SceneRenderer{
		Shader:      res.Shader,
		Material:    res.Material,
		Atlas:       res.Atlas,
		ChunkMeshes: make(map[world.ChunkCoord]*rl.Mesh),
		Log:         logging.ForPackage("render"),
	}

	// Get Shader Locations
	r.LocSkyTint = rl.GetShaderLocation(r.Shader, uniformSkyTint)
	r.LocBlockTint = rl.GetShaderLocation(r.Shader, uniformBlockTint)
	r.LocMinAmbient = rl.GetShaderLocation(r.Shader, uniformMinAmbient)

	// Initialize buffers
	r.shaderSkyTint = make([]float32, 3)
	r.shaderBlockTint = make([]float32, 3)
	r.shaderMinAmbient = make([]float32, 1)

	// blockTint and minAmbient never change, so set them once rather than
	// every frame.
	r.shaderBlockTint[0] = blockTint.R
	r.shaderBlockTint[1] = blockTint.G
	r.shaderBlockTint[2] = blockTint.B
	rl.SetShaderValue(r.Shader, r.LocBlockTint, r.shaderBlockTint, rl.ShaderUniformVec3)

	r.shaderMinAmbient[0] = minAmbient
	rl.SetShaderValue(r.Shader, r.LocMinAmbient, r.shaderMinAmbient, rl.ShaderUniformFloat)

	return r
}

// UpdateMesh generates and updates the mesh for a specific chunk.
func (r *SceneRenderer) UpdateMesh(chunk mesh.ChunkReader, w mesh.WorldReader) {
	// Generate mesh data (CPU side)
	data := mesh.GenerateChunkMeshData(chunk, w, r.Atlas)

	coord := world.ChunkCoord{X: chunk.ChunkX(), Z: chunk.ChunkZ()}

	// Unload old mesh if it exists
	if oldMesh, ok := r.ChunkMeshes[coord]; ok {
		rl.UnloadMesh(oldMesh)
		delete(r.ChunkMeshes, coord)
	}

	if data != nil {
		// Upload to GPU
		newMesh := data.Upload()
		if newMesh != nil {
			r.ChunkMeshes[coord] = newMesh
			r.Log.WithFields(logrus.Fields{
				"chunk_x":  chunk.ChunkX(),
				"chunk_z":  chunk.ChunkZ(),
				"vertices": len(data.Vertices) / 3,
			}).Debug("Updated chunk mesh")
		}
	}
}

// RemoveMesh removes a chunk mesh from memory/GPU.
func (r *SceneRenderer) RemoveMesh(coord world.ChunkCoord) {
	if oldMesh, ok := r.ChunkMeshes[coord]; ok {
		rl.UnloadMesh(oldMesh)
		delete(r.ChunkMeshes, coord)
	}
}

// SetLighting updates the daylight tint for the current time of day.
//
// This is the only per-frame lighting uniform. Per-voxel skylight is baked into
// the chunk meshes, so the day cycle costs no re-meshing, and enclosed spaces
// correctly ignore it.
func (r *SceneRenderer) SetLighting(skyTint core.RGB) {
	r.shaderSkyTint[0] = skyTint.R
	r.shaderSkyTint[1] = skyTint.G
	r.shaderSkyTint[2] = skyTint.B
	rl.SetShaderValue(r.Shader, r.LocSkyTint, r.shaderSkyTint, rl.ShaderUniformVec3)
}

// Draw renders the scene.
func (r *SceneRenderer) Draw(camera rl.Camera3D) {
	rl.BeginMode3D(camera)

	for coord, mesh := range r.ChunkMeshes {
		pos := rl.NewVector3(float32(coord.X*config.ChunkWidth), 0, float32(coord.Z*config.ChunkWidth))
		rl.DrawMesh(*mesh, r.Material, rl.MatrixTranslate(pos.X, pos.Y, pos.Z))
	}

	rl.EndMode3D()
}

// DrawDebugChunkBounds renders debug wireframes for chunk boundaries.
// This must be called within a 3D rendering context (between BeginMode3D/EndMode3D).
func (r *SceneRenderer) DrawDebugChunkBounds() {
	for coord := range r.ChunkMeshes {
		pos := rl.NewVector3(float32(coord.X*config.ChunkWidth), 0, float32(coord.Z*config.ChunkWidth))
		// Center the wireframe at chunk center, size matches chunk dimensions
		rl.DrawCubeWires(rl.Vector3Add(pos, rl.NewVector3(8, 128, 8)), 16, 256, 16, rl.Red)
	}
}

// Unload cleans up all GPU resources managed by the renderer.
// Note: Shader and Texture are owned by Resources/Loader and should be unloaded there.
// We only unload the meshes we created.
func (r *SceneRenderer) Unload() {
	r.Log.Info("Unloading scene renderer...")
	for _, mesh := range r.ChunkMeshes {
		rl.UnloadMesh(mesh)
	}
	r.ChunkMeshes = make(map[world.ChunkCoord]*rl.Mesh)
}
