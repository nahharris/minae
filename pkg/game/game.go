package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/player"
	"github.com/nahharris/minae/pkg/ui"
	"github.com/nahharris/minae/pkg/world"
)

type GameState int

const (
	StatePlaying GameState = iota
	StatePaused
)

// Game manages the global game state and systems.
type Game struct {
	Player         *player.Player
	World          *world.World
	UI             *ui.UIManager
	State          GameState
	ChunkMeshes    map[world.ChunkCoord]*rl.Mesh
	ChunkMaterials map[world.ChunkCoord]rl.Material
	Running        bool
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

	g := &Game{
		Player:         p,
		World:          w,
		UI:             u,
		State:          StatePlaying,
		ChunkMeshes:    make(map[world.ChunkCoord]*rl.Mesh),
		ChunkMaterials: make(map[world.ChunkCoord]rl.Material),
		Running:        true,
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
			// We need a material to draw the mesh
			mat := rl.LoadMaterialDefault()
			g.ChunkMaterials[coord] = mat
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
		
		// Only update player if playing
		dt := rl.GetFrameTime()
		g.Player.Update(dt)
	}
}

// Draw renders the game scene and UI.
func (g *Game) Draw() {
	rl.BeginDrawing()
	rl.ClearBackground(rl.SkyBlue)

	// 3D World
	rl.BeginMode3D(g.Player.Camera)

	for coord, mesh := range g.ChunkMeshes {
		pos := rl.NewVector3(float32(coord.X*world.ChunkWidth), 0, float32(coord.Z*world.ChunkWidth))
		rl.DrawMesh(*mesh, g.ChunkMaterials[coord], rl.MatrixTranslate(pos.X, pos.Y, pos.Z))
		
		// Debug: Draw Chunk Bounds
		if g.UI.ShowDebug && g.UI.ShowAllUI {
			rl.DrawCubeWires(rl.Vector3Add(pos, rl.NewVector3(8, 128, 8)), 16, 256, 16, rl.Red)
		}
	}
	
	// Draw Grid for reference
	rl.DrawGrid(100, 1.0)

	rl.EndMode3D()

	// UI
	if g.State == StatePaused {
		screenWidth := rl.GetScreenWidth()
		screenHeight := rl.GetScreenHeight()
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
		g.UI.DrawDebug(g.Player, g.World)
	}

	rl.EndDrawing()
}

// Unload cleans up resources.
func (g *Game) Unload() {
	for _, mesh := range g.ChunkMeshes {
		rl.UnloadMesh(mesh)
	}
}
