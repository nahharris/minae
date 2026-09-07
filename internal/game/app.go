package game

import (
	"runtime"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/chunks"
	render "github.com/nahharris/minae/internal/gfx"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/platform/logging"
	resources "github.com/nahharris/minae/internal/platform/resources"
	"github.com/nahharris/minae/internal/player"
	ui "github.com/nahharris/minae/internal/ui/game"
	"github.com/nahharris/minae/internal/world"
	"github.com/nahharris/minae/internal/world/lighting"
	"github.com/nahharris/minae/internal/worldgen"
	"github.com/sirupsen/logrus"
)

// pipelineBudget caps the chunk pipeline's wall-clock work in a single
// Update call, replacing the fixed "9 chunks" count M14/M15 chose against
// FlatGenerator's uniform terrain. That count stopped meaning anything once
// M17 gave chunks real relief: SeedChunk's cost against real terrain measured
// at 103ms/chunk in the running pipeline, one order of magnitude past a
// frame budget, driven by map lookups the M15 follow-up's three changes then
// removed from the hot path (see docs/milestones/M15-chunk-streaming.md's
// "Follow-up" section). BenchmarkSeedChunk, isolated from the rest of the
// pipeline, measured roughly 3ms/chunk after that fix — comfortably inside
// 4ms with margin for a chunk whose neighbours are not yet lit, which does
// more cross-seam enqueueing than the benchmark's steady-state case. Mesh
// draining is per-item channel receives, not real CPU work — 2ms is
// generous headroom for a burst of finished results without letting either
// budget dominate a 16ms frame at 60 FPS.
//
// At the default view distance the streamer's desired set is up to 289
// chunks, so this budget is a steady trickle rather than something startup
// waits on in one burst, exactly as before — only the unit changed from a
// chunk count to a duration, so a slower machine or a more expensive chunk
// degrades to a slower fill instead of a stalled frame.
var pipelineBudget = chunks.Budget{Light: 4 * time.Millisecond, Mesh: 2 * time.Millisecond}

// unloadMargin is added to the load radius (config.GameConfig.ViewDistance)
// to get the unload radius the Streamer uses. The M15 design decisions fix
// this at 2: one chunk of margin still leaves a player oscillating across a
// single boundary sitting exactly on the load edge, where floating point
// rounding decides the outcome every frame; two gives the frontier somewhere
// to sit instead of thrashing.
const unloadMargin = 2

// meshRemover is the subset of *render.SceneRenderer the unload path needs:
// freeing the GPU mesh of a chunk that has left the streamer's desired set.
// Factoring it out as an interface, rather than depending on SceneRenderer's
// concrete type, is what makes releaseUnloaded testable without a live
// raylib window -- a test double satisfies this trivially.
type meshRemover interface {
	RemoveMesh(coord world.ChunkCoord)
}

// releaseUnloaded frees the GPU mesh of every coord the streamer released
// this call.
//
// Unloading is the direction M15 calls out as the one nobody tests: a mesh
// leak here is silent and only shows up as memory climbing over a long
// session. Pulling this one call out of Update's body is what lets a test
// drive it directly against a fake renderer and count what it holds,
// instead of trusting that RemoveMesh was called.
func releaseUnloaded(renderer meshRemover, released []world.ChunkCoord) {
	for _, coord := range released {
		renderer.RemoveMesh(coord)
	}
}

// Game manages the global game state and systems.
type Game struct {
	State   GameState
	Running bool

	World    *world.World
	Player   *player.Player
	Renderer *render.SceneRenderer
	UI       *ui.UIManager
	Lighting *lighting.Engine
	Pipeline *chunks.Pipeline
	Streamer *chunks.Streamer

	Log *logrus.Entry
}

// NewGame initializes the game systems.
func NewGame(res *resources.Resources, dataFolder string) *Game {
	log := logging.ForPackage("game")
	log.Info("Initializing game...")

	// Initialize World (Chunks + PlayerState + TimeOfDay). Chunks arrive
	// asynchronously below rather than being filled in here.
	w := world.NewWorld()

	lightEngine := lighting.NewEngine(w)

	// M17 replaces the flat plane with generated plains, seeded from config
	// so the same saved seed always reproduces the same world (see
	// worldgen.Generator's determinism guarantee). chunks.FlatGenerator
	// stays -- it is the fixture most of the existing chunk and lighting
	// tests are built on -- but the running game uses real terrain now.
	gen := worldgen.NewGenerator(config.Current.WorldSeed)

	// workers is left at NewPipeline's discretion beyond "more than zero";
	// NumCPU is a reasonable default for a pool that does both CPU-bound
	// generation and meshing.
	pipeline := chunks.NewPipeline(w, lightEngine, gen, res.Atlas, runtime.NumCPU())

	// Spawn on the surface at column (0, 0), derived from SurfaceHeight
	// rather than the old hardcoded (0, 40, 0): against generated terrain a
	// fixed y spawns inside a hill or far above one. SurfaceHeight needs no
	// chunk to be Generated first (see its doc comment), which sidesteps
	// M15's player-hold entirely instead of requiring the spawn chunk to
	// have already arrived by the time Update runs.
	const spawnX, spawnZ = 0, 0
	spawnY := gen.SurfaceHeight(spawnX, spawnZ)
	w.PlayerState.Position = [3]float32{spawnX, float32(spawnY), spawnZ}

	// Initialize Player (Runtime wrapper around World.PlayerState)
	p := player.NewPlayer(w.PlayerState)

	// The streamer takes over from the fixed 3x3 startup region M14 used: it
	// owns the desired set of loaded chunks around the player and drives the
	// pipeline through Request and Release every frame (see Update). Priming
	// it here, at the player's spawn position, means the region around spawn
	// is already requested before the first frame runs rather than waiting
	// for one Update to notice where the player is.
	loadRadius := config.Current.ViewDistance
	streamer := chunks.NewStreamer(pipeline, loadRadius, loadRadius+unloadMargin)
	spawnChunk := world.ChunkCoordAt(p.Body.Position.X, p.Body.Position.Z)
	log.WithField("chunk", spawnChunk).Info("Requesting startup chunks...")
	streamer.Update(spawnChunk)

	// Initialize Renderer
	renderer := render.NewSceneRenderer(res)

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
		Pipeline: pipeline,
		Streamer: streamer,
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

	// Recompute the desired set around the player and drive the pipeline to
	// match it, then advance the pipeline and upload whatever it finished.
	// This runs every frame regardless of pause state, so the world keeps
	// streaming in even while the pause menu is up.
	playerChunk := world.ChunkCoordAt(g.Player.Body.Position.X, g.Player.Body.Position.Z)
	released := g.Streamer.Update(playerChunk)
	releaseUnloaded(g.Renderer, released)

	for _, ready := range g.Pipeline.Update(pipelineBudget) {
		g.Renderer.UploadChunkMesh(ready.Coord, ready.Data)
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

		// The player is held, not dropped (M15): if the chunk underneath the
		// player's current position has not reached Generated yet -- an
		// absurd-speed teleport can outrun the loader even though ordinary
		// walking speed never will -- Update must not integrate position or
		// let gravity accumulate fall speed. See player.Player.Held's doc
		// comment; internal/physics itself stays entirely unaware of chunks
		// or pipelines.
		g.Player.Held = g.Pipeline.Stage(playerChunk) < chunks.Generated

		// Update Player
		g.Player.Update(dt, g.World)

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
			g.Player.Body.Box(),
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

	// The day cycle is a single tint multiplying baked skylight, so advancing
	// time costs no re-meshing and enclosed spaces correctly ignore it.
	skyColor, skyTint := g.World.TimeOfDay.GetLightingState()

	rl.ClearBackground(render.ToColor(skyColor))

	g.Renderer.SetLighting(skyTint)

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
	g.Pipeline.Close()
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
//
// Every stale chunk is also reported to the pipeline via Invalidate. Editing
// a block bypasses the pipeline entirely — this method rebuilds the mesh here
// and now, synchronously — but the pipeline does not know that happened. If
// it had a mesh job in flight for one of these chunks -- always possible now
// that chunks stream in continuously rather than only during a fixed startup
// window -- Invalidate stops that job's now-stale result from later
// overwriting the mesh built here with one that predates the edit.
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
		g.Pipeline.Invalidate(coord)

		chunk, exists := g.World.Chunks[coord]
		if !exists {
			g.Renderer.RemoveMesh(coord)
			continue
		}
		g.Renderer.UpdateMesh(chunk, g.World)
	}
}
