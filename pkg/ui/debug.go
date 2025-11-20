package ui

import (
	"fmt"
	"math"
	"runtime"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/player"
	"github.com/nahharris/minae/pkg/world"
)

// DrawDebug draws debug information like FPS, player coordinates, etc.
func (u *UIManager) DrawDebug(p *player.Player, w *world.World) {
	if !u.ShowAllUI || !u.ShowDebug {
		return
	}

	// Background for debug text
	rl.DrawRectangle(5, 5, 300, 150, rl.Fade(rl.Black, 0.5))

	y := int32(10)
	fontSize := int32(20)
	spacing := int32(20)

	// FPS
	rl.DrawFPS(10, y)
	y += spacing

	// Memory Usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	rl.DrawText(fmt.Sprintf("Alloc: %v MB", m.Alloc/1024/1024), 10, y, fontSize, rl.White)
	y += spacing
	rl.DrawText(fmt.Sprintf("TotalAlloc: %v MB", m.TotalAlloc/1024/1024), 10, y, fontSize, rl.White)
	y += spacing
	rl.DrawText(fmt.Sprintf("Sys: %v MB", m.Sys/1024/1024), 10, y, fontSize, rl.White)
	y += spacing

	// Player Position
	pos := p.Camera.Position
	rl.DrawText(fmt.Sprintf("XYZ: %.2f / %.2f / %.2f", pos.X, pos.Y, pos.Z), 10, y, fontSize, rl.White)
	y += spacing

	// Chunk Coordinates
	chunkX := int(math.Floor(float64(pos.X) / float64(config.ChunkWidth)))
	chunkZ := int(math.Floor(float64(pos.Z) / float64(config.ChunkWidth)))

	// Local Coordinates within chunk (handle negative correctly)
	localX := int(pos.X) - chunkX*config.ChunkWidth
	localZ := int(pos.Z) - chunkZ*config.ChunkWidth

	rl.DrawText(fmt.Sprintf("Chunk: %d, %d (Loc: %d, %d)", chunkX, chunkZ, localX, localZ), 10, y, fontSize, rl.White)
	y += spacing

	// Direction
	dir := rl.Vector3Subtract(p.Camera.Target, p.Camera.Position)
	rl.DrawText(fmt.Sprintf("Dir: %.2f, %.2f, %.2f", dir.X, dir.Y, dir.Z), 10, y, fontSize, rl.White)
	y += spacing

	// Platform Info Panel (Right Side)
	screenWidth := int32(rl.GetScreenWidth())
	panelWidth := int32(300)
	panelX := screenWidth - panelWidth - 10

	rl.DrawRectangle(panelX, 5, panelWidth, 100, rl.Fade(rl.Black, 0.5))

	rightY := int32(10)
	rl.DrawText(fmt.Sprintf("Go Runtime: %s", runtime.Version()), panelX+5, rightY, fontSize, rl.White)
	rightY += spacing
	rl.DrawText(fmt.Sprintf("OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH), panelX+5, rightY, fontSize, rl.White)
	rightY += spacing
	rl.DrawText(fmt.Sprintf("Game Name: %s", config.GameName), panelX+5, rightY, fontSize, rl.White)
	rightY += spacing
	rl.DrawText(fmt.Sprintf("Version: %s", config.GameVersion), panelX+5, rightY, fontSize, rl.White)
}
