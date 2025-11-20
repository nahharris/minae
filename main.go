package main

import (
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/game"
)

func init() {
	// Lock the main thread for OpenGL (Raylib requirement)
	runtime.LockOSThread()
}

func main() {
	// Initialize Window
	rl.InitWindow(1280, 720, "Minae")
	defer rl.CloseWindow()
	
	// Disable ESC key closing the window (we handle it manually for Pause)
	rl.SetExitKey(0)

	rl.SetTargetFPS(60)
	// Initial cursor state handled by Game

	// Initialize Game
	g := game.NewGame()
	defer g.Unload()

	// Main Game Loop
	for !rl.WindowShouldClose() && g.Running {
		g.Update()
		g.Draw()
	}
}
