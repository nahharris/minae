package render

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/logging"
	"github.com/nahharris/minae/pkg/render/atlas"
	"github.com/nahharris/minae/pkg/render/mesh"
	"github.com/nahharris/minae/pkg/resource"
	"github.com/nahharris/minae/pkg/world"
	"github.com/sirupsen/logrus"
)

// SceneRenderer handles the 3D rendering of the world.
type SceneRenderer struct {
	Shader   rl.Shader
	Material rl.Material
	Atlas    *atlas.Atlas

	ChunkMeshes map[world.ChunkCoord]*rl.Mesh

	// Shader Uniform Locations
	LocLightDir   int32
	LocLightColor int32
	LocAmbient    int32
	LocViewPos    int32

	// Reusable shader value buffers
	shaderLightDir   []float32
	shaderLightColor []float32
	shaderAmbient    []float32
	shaderViewPos    []float32

	Log *logrus.Entry
}

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
	r.LocLightDir = rl.GetShaderLocation(r.Shader, "lightDir")
	r.LocLightColor = rl.GetShaderLocation(r.Shader, "lightColor")
	r.LocAmbient = rl.GetShaderLocation(r.Shader, "ambientColor")
	r.LocViewPos = rl.GetShaderLocation(r.Shader, "viewPos")

	// Initialize buffers
	r.shaderLightDir = make([]float32, 3)
	r.shaderLightColor = make([]float32, 4)
	r.shaderAmbient = make([]float32, 4)
	r.shaderViewPos = make([]float32, 3)

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

// SetLighting updates the shader uniforms for lighting.
func (r *SceneRenderer) SetLighting(lightColor, ambientColor rl.Color, lightDir rl.Vector3) {
	r.shaderLightDir[0] = lightDir.X
	r.shaderLightDir[1] = lightDir.Y
	r.shaderLightDir[2] = lightDir.Z
	rl.SetShaderValue(r.Shader, r.LocLightDir, r.shaderLightDir, rl.ShaderUniformVec3)

	lc := rl.ColorNormalize(lightColor)
	r.shaderLightColor[0] = lc.X
	r.shaderLightColor[1] = lc.Y
	r.shaderLightColor[2] = lc.Z
	r.shaderLightColor[3] = lc.W
	rl.SetShaderValue(r.Shader, r.LocLightColor, r.shaderLightColor, rl.ShaderUniformVec4)

	ac := rl.ColorNormalize(ambientColor)
	r.shaderAmbient[0] = ac.X
	r.shaderAmbient[1] = ac.Y
	r.shaderAmbient[2] = ac.Z
	r.shaderAmbient[3] = ac.W
	rl.SetShaderValue(r.Shader, r.LocAmbient, r.shaderAmbient, rl.ShaderUniformVec4)
}

// SetViewPosition updates the view position shader uniform.
func (r *SceneRenderer) SetViewPosition(camera rl.Camera3D) {
	r.shaderViewPos[0] = camera.Position.X
	r.shaderViewPos[1] = camera.Position.Y
	r.shaderViewPos[2] = camera.Position.Z
	rl.SetShaderValue(r.Shader, r.LocViewPos, r.shaderViewPos, rl.ShaderUniformVec3)
}

// Draw renders the scene.
func (r *SceneRenderer) Draw(camera rl.Camera3D) {
	// Update View Position Uniform
	r.SetViewPosition(camera)

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
