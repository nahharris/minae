package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/player"
	"github.com/nahharris/minae/pkg/ui"
	"github.com/nahharris/minae/pkg/world"
	"github.com/nahharris/minae/pkg/world/lighting"
)

type GameState int

const (
	StatePlaying GameState = iota
	StatePaused
)

// Game manages the global game state and systems.
type Game struct {
	Player      *player.Player
	World       *world.World
	UI          *ui.UIManager
	State       GameState
	ChunkMeshes map[world.ChunkCoord]*rl.Mesh
	Running     bool

	// Lighting
	Lighting      *lighting.Manager
	Shader        rl.Shader
	ChunkMaterial rl.Material

	// Shader Locations
	LocLightDir   int32
	LocLightColor int32
	LocAmbient    int32
	LocViewPos    int32

	// Reusable shader value slices to avoid per-frame allocations
	shaderLightDir   []float32 // Vec3
	shaderLightColor []float32 // Vec4
	shaderAmbient    []float32 // Vec4
	shaderViewPos    []float32 // Vec3
}

// NewGame initializes the game systems.
func NewGame() *Game {
	// Initialize Player
	// Start at 0, 40, 0 to be above the ground (generated up to y=32)
	p := player.NewPlayer(rl.NewVector3(0, 40, 0))

	// Initialize World
	w := world.NewWorld()
	w.GenerateFixedGrid()

	// Initialize Lighting
	l := lighting.NewManager()
	shader := rl.LoadShaderFromMemory(lighting.VsCode, lighting.FsCode)

	// Initialize UI
	u := ui.NewUIManager(p, w, l)

	// Get Shader Locations
	// Standard locations are set automatically by LoadShader if names match standard Raylib names.
	// Custom locations need to be retrieved.
	locLightDir := rl.GetShaderLocation(shader, "lightDir")
	locLightColor := rl.GetShaderLocation(shader, "lightColor")
	locAmbient := rl.GetShaderLocation(shader, "ambientColor")
	locViewPos := rl.GetShaderLocation(shader, "viewPos")

	// Create shared material
	mat := rl.LoadMaterialDefault()
	mat.Shader = shader

	g := &Game{
		Player:        p,
		World:         w,
		UI:            u,
		State:         StatePlaying,
		ChunkMeshes:   make(map[world.ChunkCoord]*rl.Mesh),
		Running:       true,
		Lighting:      l,
		Shader:        shader,
		ChunkMaterial: mat,
		LocLightDir:   locLightDir,
		LocLightColor: locLightColor,
		LocAmbient:    locAmbient,
		LocViewPos:    locViewPos,
		// Pre-allocate shader value slices
		shaderLightDir:   make([]float32, 3),
		shaderLightColor: make([]float32, 4),
		shaderAmbient:    make([]float32, 4),
		shaderViewPos:    make([]float32, 3),
	}

	g.generateMeshes()

	return g
}

// generateMeshes generates meshes for all chunks in the world.
func (g *Game) generateMeshes() {
	for coord, chunk := range g.World.Chunks {
		mesh := world.GenerateChunkMesh(chunk, g.World)
		if mesh != nil {
			g.ChunkMeshes[coord] = mesh
		}
	}
}

// Update handles the main game logic updates.
func (g *Game) Update() {
	// Global Input
	if rl.IsKeyPressed(rl.KeyF1) {
		g.UI.ShowAllUI = !g.UI.ShowAllUI
	}
	if rl.IsKeyPressed(rl.KeyF2) {
		g.UI.ShowDebug = !g.UI.ShowDebug
	}
	if rl.IsKeyPressed(rl.KeyEscape) {
		if g.State == StatePlaying {
			g.State = StatePaused
		} else {
			g.State = StatePlaying
		}
	}

	// State Management
	if g.State == StatePaused {
		if rl.IsCursorHidden() {
			rl.EnableCursor()
		}
	} else {
		if !rl.IsCursorHidden() {
			rl.DisableCursor()
		}

		// Update Lighting
		dt := rl.GetFrameTime()
		g.Lighting.Update(dt)

		// Only update player if playing
		g.Player.Update(dt)

		// Handle Block Interaction
		affectedChunks := g.Player.HandleBlockInteraction(g.World)
		for _, coord := range affectedChunks {
			// Regenerate mesh for affected chunk
			if chunk, exists := g.World.Chunks[coord]; exists {
				// Unload old mesh
				if oldMesh, ok := g.ChunkMeshes[coord]; ok {
					rl.UnloadMesh(oldMesh)
				}

				newMesh := world.GenerateChunkMesh(chunk, g.World)
				if newMesh != nil {
					g.ChunkMeshes[coord] = newMesh
				} else {
					// If mesh is nil (e.g. empty chunk), remove from map
					delete(g.ChunkMeshes, coord)
				}
			}
		}
	}
}

// Draw renders the game scene and UI.
func (g *Game) Draw() {
	rl.BeginDrawing()

	// Update Lighting Uniforms
	skyColor, lightColor, ambientColor, lightDir := g.Lighting.GetState()

	rl.ClearBackground(skyColor)

	// Set Shader Values using pre-allocated slices
	g.shaderLightDir[0] = lightDir.X
	g.shaderLightDir[1] = lightDir.Y
	g.shaderLightDir[2] = lightDir.Z
	rl.SetShaderValue(g.Shader, g.LocLightDir, g.shaderLightDir, rl.ShaderUniformVec3)

	lc := rl.ColorNormalize(lightColor)
	g.shaderLightColor[0] = lc.X
	g.shaderLightColor[1] = lc.Y
	g.shaderLightColor[2] = lc.Z
	g.shaderLightColor[3] = lc.W
	rl.SetShaderValue(g.Shader, g.LocLightColor, g.shaderLightColor, rl.ShaderUniformVec4)

	ac := rl.ColorNormalize(ambientColor)
	g.shaderAmbient[0] = ac.X
	g.shaderAmbient[1] = ac.Y
	g.shaderAmbient[2] = ac.Z
	g.shaderAmbient[3] = ac.W
	rl.SetShaderValue(g.Shader, g.LocAmbient, g.shaderAmbient, rl.ShaderUniformVec4)

	camPos := g.Player.Camera.Position
	g.shaderViewPos[0] = camPos.X
	g.shaderViewPos[1] = camPos.Y
	g.shaderViewPos[2] = camPos.Z
	rl.SetShaderValue(g.Shader, g.LocViewPos, g.shaderViewPos, rl.ShaderUniformVec3)

	// 3D World
	rl.BeginMode3D(g.Player.Camera)

	for coord, mesh := range g.ChunkMeshes {
		pos := rl.NewVector3(float32(coord.X*config.ChunkWidth), 0, float32(coord.Z*config.ChunkWidth))
		rl.DrawMesh(*mesh, g.ChunkMaterial, rl.MatrixTranslate(pos.X, pos.Y, pos.Z))

		// Debug: Draw Chunk Bounds
		if g.UI.ShowDebug && g.UI.ShowAllUI {
			rl.DrawCubeWires(rl.Vector3Add(pos, rl.NewVector3(8, 128, 8)), 16, 256, 16, rl.Red)
		}
	}

	// Draw Grid for reference (optional, might look weird with lighting now)
	// rl.DrawGrid(100, 1.0)

	// Draw Selection Highlight
	if g.Player.HasTarget {
		// Draw a slightly larger wireframe cube around the target block
		// TargetBlock is integer coordinates, so we can use it directly.
		// We need to add 0.5 to center it, and size 1.005 to be slightly outside.
		targetPos := rl.Vector3Add(g.Player.TargetBlock, rl.NewVector3(0.5, 0.5, 0.5))
		rl.DrawCubeWires(targetPos, 1.01, 1.01, 1.01, rl.Black)
	}

	rl.EndMode3D()

	// UI
	screenWidth := rl.GetScreenWidth()
	screenHeight := rl.GetScreenHeight()

	if g.State == StatePlaying {
		g.UI.DrawHUD(screenWidth, screenHeight)
	}

	if g.State == StatePaused {
		resume, quit := g.UI.DrawPauseMenu(screenWidth, screenHeight)

		if resume {
			g.State = StatePlaying
		}
		if quit {
			g.Running = false
		}
	}

	// Debug Overlay
	if g.UI.ShowDebug {
		g.UI.DrawDebug()
	}

	rl.EndDrawing()
}

// Unload cleans up resources.
func (g *Game) Unload() {
	rl.UnloadShader(g.Shader)
	for _, mesh := range g.ChunkMeshes {
		rl.UnloadMesh(mesh)
	}
}
