package game

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	render "github.com/nahharris/minae/internal/gfx"
	"github.com/nahharris/minae/internal/platform/logging"
	resources "github.com/nahharris/minae/internal/platform/resources"
	"github.com/nahharris/minae/internal/player"
	ui "github.com/nahharris/minae/internal/ui/game"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
	"github.com/sirupsen/logrus"
)

// Game manages the global game state and systems.
type Game struct {
	State   GameState
	Running bool

	World    *world.World
	Player   *player.Player
	Renderer *render.SceneRenderer
	UI       *ui.UIManager
	Lighting *lighting.Engine

	Log *logrus.Entry
}

// NewGame initializes the game systems.
func NewGame(res *resources.Resources, dataFolder string) *Game {
	log := logging.ForPackage("game")
	log.Info("Initializing game...")

	// Initialize World (Chunks + PlayerState + TimeOfDay)
	w := world.NewWorld()
	w.GenerateFixedGrid()

	// Light the whole world once. RecomputeAll is order-independent, unlike
	// seeding chunk by chunk, so the result does not depend on map iteration
	// order.
	log.Info("Calculating initial lighting...")
	lightEngine := lighting.NewEngine(w)
	lightEngine.RecomputeAll()
	lightEngine.DirtyChunks() // Drain: every chunk is meshed for the first time below.

	// Initialize Player (Runtime wrapper around World.PlayerState)
	p := player.NewPlayer(w.PlayerState)

	// Initialize Renderer
	renderer := render.NewSceneRenderer(res)

	// Initial mesh generation
	log.Info("Generating initial meshes...")
	for _, chunk := range w.Chunks {
		renderer.UpdateMesh(chunk, w)
	}

	// Initialize UI
	u := ui.NewUIManager(p, w, w.TimeOfDay)

	g := &Game{
		State:    StatePlaying,
		Running:  true,
		World:    w,
		Player:   p,
		Renderer: renderer,
		UI:       u,
		Lighting: lightEngine,
		Log:      log,
	}

	return g
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
			g.Log.Info("Game Paused")
		} else {
			g.State = StatePlaying
			g.Log.Info("Game Resumed")
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

		dt := rl.GetFrameTime()

		// Update Time
		g.World.TimeOfDay.Update(dt)

		// Update Player
		g.Player.Update(dt)

		// Handle Block Interaction
		// We moved interaction logic to world/interaction.go, but we need to call it.
		// Player has runtime state (HasTarget, TargetBlock) but interaction logic needs to run.
		// Let's call ProcessBlockInteraction.

		action := world.ActionNone
		if rl.IsMouseButtonPressed(rl.MouseLeftButton) {
			action = world.ActionBreak
		} else if rl.IsMouseButtonPressed(rl.MouseRightButton) {
			action = world.ActionPlace
		}

		// Get selected block from inventory

		inventoryBlock := g.Player.State.Inventory[g.Player.SelectedBlockIndex]

		// Determine meta (orientation/slab) - logic was in interaction.go but we need to pass placeMeta?
		// interaction.go handles `MetaSlabTopBit` and `MetaFacingMask`.
		// So we pass 0 as initial meta unless we have specific state.

		result := world.ProcessBlockInteraction(
			g.World,
			render.FromVector3(g.Player.Camera.Position),
			render.FromVector3(rl.Vector3Subtract(g.Player.Camera.Target, g.Player.Camera.Position)), // Direction
			action,
			inventoryBlock,
			0, // Initial meta
		)

		// Update Player runtime state with interaction result
		g.Player.HasTarget = result.HasTarget
		if result.HasTarget {
			g.Player.TargetBlock = rl.NewVector3(float32(result.TargetBlock[0]), float32(result.TargetBlock[1]), float32(result.TargetBlock[2]))
		}

		if result.Changed {
			g.remeshAfterBlockChange(result)
		}
	}
}

// Draw renders the game scene and UI.
func (g *Game) Draw() {
	rl.BeginDrawing()

	// Update Lighting Uniforms
	skyColor, lightColor, ambientColor, lightDir := g.World.TimeOfDay.GetLightingState()

	rl.ClearBackground(render.ToColor(skyColor))

	g.Renderer.SetLighting(lightColor, ambientColor, lightDir)

	// Draw 3D Scene (meshes)
	g.Renderer.Draw(g.Player.Camera)

	// Draw game-specific 3D elements
	rl.BeginMode3D(g.Player.Camera)

	// Draw Selection Highlight
	if g.Player.HasTarget {
		targetPos := rl.Vector3Add(g.Player.TargetBlock, rl.NewVector3(0.5, 0.5, 0.5))
		rl.DrawCubeWires(targetPos, 1.01, 1.01, 1.01, rl.Black)
	}

	// Debug: Draw Chunk Bounds
	if g.UI.ShowDebug && g.UI.ShowAllUI {
		g.Renderer.DrawDebugChunkBounds()
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
			g.Log.Info("Game Resumed")
		}
		if quit {
			g.Running = false
			g.Log.Info("Quitting game...")
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
	g.Renderer.Unload()
}

// remeshAfterBlockChange updates the lighting for a block change and rebuilds
// every chunk mesh the change invalidated.
//
// Two independent sets of chunks need rebuilding, and missing either one leaves
// stale geometry on screen:
//
//   - result.AffectedChunks — the chunk holding the block, plus its neighbours
//     when the block sits on a border. Their face culling changed.
//   - the engine's dirty set — every chunk whose skylight the change altered.
//     Light crosses chunk seams, so this routinely includes chunks the block
//     itself did not touch.
func (g *Game) remeshAfterBlockChange(result world.InteractionResult) {
	pos := result.ChangedBlock
	g.Lighting.OnBlockChanged(pos[0], pos[1], pos[2])

	stale := make(map[world.ChunkCoord]struct{}, len(result.AffectedChunks)+4)
	for _, coord := range result.AffectedChunks {
		stale[coord] = struct{}{}
	}
	for _, coord := range g.Lighting.DirtyChunks() {
		stale[coord] = struct{}{}
	}

	for coord := range stale {
		chunk, exists := g.World.Chunks[coord]
		if !exists {
			g.Renderer.RemoveMesh(coord)
			continue
		}
		g.Renderer.UpdateMesh(chunk, g.World)
	}
}
