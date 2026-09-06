package main

import (
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/game"
	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/platform/logging"
	"github.com/nahharris/minae/internal/platform/logging/raylog"
	resource "github.com/nahharris/minae/internal/platform/resources"
	"github.com/sirupsen/logrus"
)

func init() {
	// Lock the main thread for OpenGL (Raylib requirement)
	runtime.LockOSThread()
}

func main() {
	// 1. Initialize Logging
	logging.Init(logrus.DebugLevel)
	raylog.Init()
	logger := logging.ForPackage("main")

	// 2. Bootstrap Data Folder (Create if missing)
	dataFolder, err := config.BootstrapDataFolder()
	if err != nil {
		logger.Fatalf("Failed to bootstrap data folder: %v", err)
	}

	// 3. Initialize Resource Loader
	loader := resource.NewLoader(dataFolder)

	// 4. Load Config
	if err := loader.LoadConfig(); err != nil {
		logger.Warnf("Failed to load config: %v. Using defaults.", err)
	}

	// 5. Load Blocks
	if err := loader.LoadBlocks(); err != nil {
		logger.Warnf("Failed to load custom blocks: %v. Proceeding with vanilla only.", err)
	}

	// 6. Initialize Window
	rl.InitWindow(
		int32(config.Current.ScreenWidth),
		int32(config.Current.ScreenHeight),
		config.GameName,
	)
	defer rl.CloseWindow()

	rl.SetExitKey(0)
	rl.SetTargetFPS(int32(config.Current.TargetFPS))

	// 7. Load Render Resources (Must be after InitWindow)
	resources, err := loader.LoadRenderResources()
	if err != nil {
		logger.Fatalf("Failed to load render resources: %v", err)
	}
	defer loader.Unload(resources)

	// 8. Initialize Game
	g := game.NewGame(resources, dataFolder)
	defer g.Unload()

	// 9. Main Game Loop
	for !rl.WindowShouldClose() && g.Running {
		g.Update()
		g.Draw()
	}

	logger.Info("Exiting...")
}
