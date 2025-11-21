package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/pkg/player"
)

// UIManager handles drawing of overlays and menus.
type UIManager struct {
	ShowAllUI bool
	ShowDebug bool
}

// NewUIManager creates a new UIManager.
func NewUIManager() *UIManager {
	return &UIManager{
		ShowAllUI: true,
		ShowDebug: false,
	}
}

// DrawHUD draws the in-game HUD (crosshair, hotbar, etc).
func (u *UIManager) DrawHUD(screenWidth, screenHeight int, p *player.Player) {
	if !u.ShowAllUI {
		return
	}

	// Crosshair
	centerX := int32(screenWidth / 2)
	centerY := int32(screenHeight / 2)
	rl.DrawLine(centerX-10, centerY, centerX+10, centerY, rl.White)
	rl.DrawLine(centerX, centerY-10, centerX, centerY+10, rl.White)

	// Hotbar (Right side list)
	// We list all available blocks and highlight the selected one.
	// Just a simple vertical list for now as requested.
	const padding = 10
	const itemHeight = 30
	const itemWidth = 150

	startX := int32(screenWidth) - itemWidth - padding
	startY := int32(screenHeight)/2 - int32(len(p.Inventory)*itemHeight)/2

	for i, block := range p.Inventory {
		y := startY + int32(i*itemHeight)

		var color rl.Color
		if i == p.SelectedBlockIndex {
			color = rl.Yellow
			// Draw selection box
			rl.DrawRectangleLines(startX-5, y, itemWidth, itemHeight, rl.Yellow)
		} else {
			color = rl.LightGray
		}

		// Draw Block Name
		rl.DrawText(block.Name, startX, y+5, 20, color)

		// Optional: Draw a small colored rect representing the block color
		c := rl.GetColor(uint(block.Color))
		rl.DrawRectangle(startX+100, y+5, 20, 20, c)
	}
}
