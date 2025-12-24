package main

import (
	"log"
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/blocks"
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/game"
)

func init() {
	// Lock the main thread for OpenGL (Raylib requirement)
	runtime.LockOSThread()
}

func main() {
	// 1. Bootstrap Data Folder (Create if missing)
	dataFolder, err := config.BootstrapDataFolder()
	if err != nil {
		log.Fatalf("Failed to bootstrap data folder: %v", err)
	}

	// 2. Load Config (Write defaults if missing)
	if err := config.Load(dataFolder); err != nil {
		log.Printf("Warning: Failed to load config: %v. Using defaults.", err)
	}

	// 4. Load Custom/Override Blocks
	if err := blocks.Load(dataFolder); err != nil {
		log.Printf("Warning: Failed to load custom blocks: %v. Proceeding with vanilla only.", err)
	}

	// Initialize Window
	rl.InitWindow(
		int32(config.Current.ScreenWidth),
		int32(config.Current.ScreenHeight),
		config.GameName,
	)
	defer rl.CloseWindow()

	// Disable ESC key closing the window (we handle it manually for Pause)
	rl.SetExitKey(0)

	rl.SetTargetFPS(int32(config.Current.TargetFPS))
	// Initial cursor state handled by Game

	// Initialize Game
	g := game.NewGame(dataFolder)
	defer g.Unload()

	// Main Game Loop
	for !rl.WindowShouldClose() && g.Running {
		g.Update()
		g.Draw()
	}
}
