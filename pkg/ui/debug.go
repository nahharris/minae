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

	// Initialize cached strings
	u.debugFPS = "FPS: 0"
	u.debugTime = "Time: N/A"
	u.debugAlloc = "Alloc: 0 MB"
	u.debugTotalAlloc = "TotalAlloc: 0 MB"
	u.debugSys = "Sys: 0 MB"
	u.debugXYZ = ""
	u.debugChunk = ""
	u.debugDir = ""

	// FPS
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugFPS
	}))

	// Time
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugTime
	}))

	// Memory
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugAlloc
	}))
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugTotalAlloc
	}))
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugSys
	}))

	// Player Position
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugXYZ
	}))

	// Chunk Coords
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugChunk
	}))

	// Direction
	left.AddChild(minui.NewReactiveLabel(func() string {
		return u.debugDir
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

	// Update cached debug strings only when values change
	u.updateDebugStrings()

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

// updateDebugStrings updates cached debug strings only when values change.
func (u *UIManager) updateDebugStrings() {
	// FPS
	fps := int(rl.GetFPS())
	if fps != u.cachedFPS {
		u.cachedFPS = fps
		u.debugFPS = fmt.Sprintf("FPS: %d", fps)
	}

	// Time
	if u.Lighting != nil {
		timeStr := formatTime(u.Lighting.Time, u.Lighting.CycleDuration)
		if timeStr != u.cachedTime {
			u.cachedTime = timeStr
			u.debugTime = "Time: " + timeStr
		}
	} else {
		if u.debugTime != "Time: N/A" {
			u.debugTime = "Time: N/A"
		}
	}

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.Alloc != u.cachedAlloc {
		u.cachedAlloc = m.Alloc
		u.debugAlloc = fmt.Sprintf("Alloc: %v MB", m.Alloc/1024/1024)
	}
	if m.TotalAlloc != u.cachedTotalAlloc {
		u.cachedTotalAlloc = m.TotalAlloc
		u.debugTotalAlloc = fmt.Sprintf("TotalAlloc: %v MB", m.TotalAlloc/1024/1024)
	}
	if m.Sys != u.cachedSys {
		u.cachedSys = m.Sys
		u.debugSys = fmt.Sprintf("Sys: %v MB", m.Sys/1024/1024)
	}

	// Player Position
	if u.Player != nil {
		pos := u.Player.Camera.Position
		xyz := [3]float32{pos.X, pos.Y, pos.Z}
		if xyz != u.cachedXYZ {
			u.cachedXYZ = xyz
			u.debugXYZ = fmt.Sprintf("XYZ: %.2f / %.2f / %.2f", pos.X, pos.Y, pos.Z)
		}
	} else {
		if u.debugXYZ != "" {
			u.debugXYZ = ""
		}
	}

	// Chunk Coords
	if u.Player != nil {
		pos := u.Player.Camera.Position
		chunkX := int(math.Floor(float64(pos.X) / float64(config.ChunkWidth)))
		chunkZ := int(math.Floor(float64(pos.Z) / float64(config.ChunkWidth)))
		localX := int(pos.X) - chunkX*config.ChunkWidth
		localZ := int(pos.Z) - chunkZ*config.ChunkWidth
		chunk := [4]int{chunkX, chunkZ, localX, localZ}
		if chunk != u.cachedChunk {
			u.cachedChunk = chunk
			u.debugChunk = fmt.Sprintf("Chunk: %d, %d (Loc: %d, %d)", chunkX, chunkZ, localX, localZ)
		}
	} else {
		if u.debugChunk != "" {
			u.debugChunk = ""
		}
	}

	// Direction
	if u.Player != nil {
		dir := rl.Vector3Subtract(u.Player.Camera.Target, u.Player.Camera.Position)
		dirArr := [3]float32{dir.X, dir.Y, dir.Z}
		if dirArr != u.cachedDir {
			u.cachedDir = dirArr
			u.debugDir = fmt.Sprintf("Dir: %.2f, %.2f, %.2f", dir.X, dir.Y, dir.Z)
		}
	} else {
		if u.debugDir != "" {
			u.debugDir = ""
		}
	}
}

func formatTime(time, duration float32) string {
	hour := int((time / duration) * 24.0)
	minute := int(((time/duration)*24.0 - float32(hour)) * 60.0)
	return fmt.Sprintf("%02d:%02d", hour, minute)
}
