package ui

import (
	"fmt"
	"math"
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/minui"
	"github.com/nahharris/minae/pkg/config"
)

func (u *UIManager) initDebug() {
	// Left Panel: FPS, Stats
	left := minui.NewPanel()
	left.BackgroundColor = rl.Fade(rl.Black, 0.5)
	left.Padding = 10
	left.Spacing = 5

	// FPS
	left.AddChild(minui.NewReactiveLabel(func() string {
		return fmt.Sprintf("FPS: %d", rl.GetFPS())
	}))

	// Time
	left.AddChild(minui.NewReactiveLabel(func() string {
		if u.Lighting == nil {
			return "Time: N/A"
		}
		return "Time: " + formatTime(u.Lighting.Time, u.Lighting.CycleDuration)
	}))

	// Memory
	left.AddChild(minui.NewReactiveLabel(func() string {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return fmt.Sprintf("Alloc: %v MB", m.Alloc/1024/1024)
	}))
	left.AddChild(minui.NewReactiveLabel(func() string {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return fmt.Sprintf("TotalAlloc: %v MB", m.TotalAlloc/1024/1024)
	}))
	left.AddChild(minui.NewReactiveLabel(func() string {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		return fmt.Sprintf("Sys: %v MB", m.Sys/1024/1024)
	}))

	// Player Position
	left.AddChild(minui.NewReactiveLabel(func() string {
		if u.Player == nil {
			return ""
		}
		pos := u.Player.Camera.Position
		return fmt.Sprintf("XYZ: %.2f / %.2f / %.2f", pos.X, pos.Y, pos.Z)
	}))

	// Chunk Coords
	left.AddChild(minui.NewReactiveLabel(func() string {
		if u.Player == nil {
			return ""
		}
		pos := u.Player.Camera.Position
		chunkX := int(math.Floor(float64(pos.X) / float64(config.ChunkWidth)))
		chunkZ := int(math.Floor(float64(pos.Z) / float64(config.ChunkWidth)))
		localX := int(pos.X) - chunkX*config.ChunkWidth
		localZ := int(pos.Z) - chunkZ*config.ChunkWidth
		return fmt.Sprintf("Chunk: %d, %d (Loc: %d, %d)", chunkX, chunkZ, localX, localZ)
	}))
	
	// Direction
	left.AddChild(minui.NewReactiveLabel(func() string {
		if u.Player == nil {
			return ""
		}
		dir := rl.Vector3Subtract(u.Player.Camera.Target, u.Player.Camera.Position)
		return fmt.Sprintf("Dir: %.2f, %.2f, %.2f", dir.X, dir.Y, dir.Z)
	}))

	u.debugRoot = left

	// Right Panel: System Info
	right := minui.NewPanel()
	right.BackgroundColor = rl.Fade(rl.Black, 0.5)
	right.Padding = 10
	right.Spacing = 5

	right.AddChild(minui.NewLabel(fmt.Sprintf("Go Runtime: %s", runtime.Version())))
	right.AddChild(minui.NewLabel(fmt.Sprintf("OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)))
	right.AddChild(minui.NewLabel(fmt.Sprintf("Game Name: %s", config.GameName)))
	right.AddChild(minui.NewLabel(fmt.Sprintf("Version: %s", config.GameVersion)))

	u.debugInfoRoot = right
}

// DrawDebug draws debug information.
func (u *UIManager) DrawDebug() {
	if !u.ShowAllUI || !u.ShowDebug {
		return
	}

	// Left Panel Positioning
	// Fixed position 5,5
	// We need to compute size to set bounds properly (mostly for background)
	available := minui.Size{Width: 300, Height: 1000} // Max height
	
	// Left
	leftSize := u.debugRoot.ComputeSize(available)
	// Force width to 300 as per original
	leftSize.Width = 300 
	u.debugRoot.SetBounds(minui.Rect{X: 5, Y: 5, Width: leftSize.Width, Height: leftSize.Height})
	u.debugRoot.Draw()

	// Right Panel Positioning
	screenWidth := float32(rl.GetScreenWidth())
	panelWidth := float32(300)
	
	rightSize := u.debugInfoRoot.ComputeSize(minui.Size{Width: panelWidth, Height: 1000})
	rightSize.Width = panelWidth
	
	panelX := screenWidth - panelWidth - 10
	u.debugInfoRoot.SetBounds(minui.Rect{X: panelX, Y: 5, Width: rightSize.Width, Height: rightSize.Height})
	u.debugInfoRoot.Draw()
}

func formatTime(time, duration float32) string {
	hour := int((time / duration) * 24.0)
	minute := int(((time/duration)*24.0 - float32(hour)) * 60.0)
	return fmt.Sprintf("%02d:%02d", hour, minute)
}
