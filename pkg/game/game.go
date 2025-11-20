package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/config"
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
					// Ensure material exists (it should)
					if _, ok := g.ChunkMaterials[coord]; !ok {
						g.ChunkMaterials[coord] = rl.LoadMaterialDefault()
					}
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
	rl.ClearBackground(rl.SkyBlue)

	// 3D World
	rl.BeginMode3D(g.Player.Camera)

	for coord, mesh := range g.ChunkMeshes {
		pos := rl.NewVector3(float32(coord.X*config.ChunkWidth), 0, float32(coord.Z*config.ChunkWidth))
		rl.DrawMesh(*mesh, g.ChunkMaterials[coord], rl.MatrixTranslate(pos.X, pos.Y, pos.Z))

		// Debug: Draw Chunk Bounds
		if g.UI.ShowDebug && g.UI.ShowAllUI {
			rl.DrawCubeWires(rl.Vector3Add(pos, rl.NewVector3(8, 128, 8)), 16, 256, 16, rl.Red)
		}
	}

	// Draw Grid for reference
	rl.DrawGrid(100, 1.0)

	// Draw Selection Highlight
	if g.Player.HasTarget {
		// Draw a slightly larger wireframe cube around the target block
		// TargetBlock is integer coordinates, so we can use it directly.
		// We need to add 0.5 to center it, and size 1.005 to be slightly outside.
		targetPos := rl.Vector3Add(g.Player.TargetBlock, rl.NewVector3(0.5, 0.5, 0.5))
		rl.DrawCubeWires(targetPos, 1.05, 1.05, 1.05, rl.Black) // Inverted color logic is hard in Raylib simply, Black/White is good enough
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
