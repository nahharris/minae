package game

import (
	"fmt"

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
}

// NewGame initializes the game systems.
func NewGame() *Game {
	// Initialize Player
	// Start at 0, 40, 0 to be above the ground (generated up to y=32)
	p := player.NewPlayer(rl.NewVector3(0, 40, 0))

	// Initialize World
	w := world.NewWorld()
	w.GenerateFixedGrid()

	// Initialize UI
	u := ui.NewUIManager()

	// Initialize Lighting
	l := lighting.NewManager()
	shader := rl.LoadShaderFromMemory(lighting.VsCode, lighting.FsCode)

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

	// Set Shader Values
	// Raylib Go wrappers for SetShaderValue are a bit specific.
	// We need to pass slice of float32.
	rl.SetShaderValue(g.Shader, g.LocLightDir, []float32{lightDir.X, lightDir.Y, lightDir.Z}, rl.ShaderUniformVec3)

	lc := rl.ColorNormalize(lightColor)
	rl.SetShaderValue(g.Shader, g.LocLightColor, []float32{lc.X, lc.Y, lc.Z, lc.W}, rl.ShaderUniformVec4)

	ac := rl.ColorNormalize(ambientColor)
	rl.SetShaderValue(g.Shader, g.LocAmbient, []float32{ac.X, ac.Y, ac.Z, ac.W}, rl.ShaderUniformVec4)

	camPos := g.Player.Camera.Position
	rl.SetShaderValue(g.Shader, g.LocViewPos, []float32{camPos.X, camPos.Y, camPos.Z}, rl.ShaderUniformVec3)

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
		rl.DrawCubeWires(targetPos, 1.05, 1.05, 1.05, rl.Black)
	}

	rl.EndMode3D()

	// UI
	screenWidth := rl.GetScreenWidth()
	screenHeight := rl.GetScreenHeight()

	if g.State == StatePlaying {
		g.UI.DrawHUD(screenWidth, screenHeight, g.Player)
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
		timeStr := timeToString(g.Lighting.Time, g.Lighting.CycleDuration)
		g.UI.DrawDebug(g.Player, g.World, timeStr)
	}

	rl.EndDrawing()
}

// Helper to format time
func timeToString(time, duration float32) string {
	hour := int((time / duration) * 24.0)
	minute := int(((time/duration)*24.0 - float32(hour)) * 60.0)
	return fmt.Sprintf("%02d:%02d", hour, minute)
}

// Unload cleans up resources.
func (g *Game) Unload() {
	rl.UnloadShader(g.Shader)
	for _, mesh := range g.ChunkMeshes {
		rl.UnloadMesh(mesh)
	}
}
