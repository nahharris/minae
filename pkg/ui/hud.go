package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/minui"
)

func (u *UIManager) initHUD() {
	// Hotbar Panel
	hotbar := minui.NewPanel()
	hotbar.Direction = minui.DirectionVertical
	hotbar.Alignment = minui.AlignEnd
	hotbar.Padding = 10
	hotbar.Spacing = 5

	u.hudRoot = hotbar
}

// DrawHUD draws the in-game HUD.
func (u *UIManager) DrawHUD(screenWidth, screenHeight int) {
	if !u.ShowAllUI {
		return
	}

	// Crosshair
	centerX := int32(screenWidth / 2)
	centerY := int32(screenHeight / 2)
	rl.DrawLine(centerX-10, centerY, centerX+10, centerY, rl.White)
	rl.DrawLine(centerX, centerY-10, centerX, centerY+10, rl.White)

	// Rebuild Hotbar components based on inventory state
	hotbar := u.hudRoot.(*minui.Panel)
	hotbar.Children = nil // Clear

	for i, block := range u.Player.Inventory {
		// Create a container for the slot to handle background highlighting
		slot := minui.NewPanel()
		slot.Direction = minui.DirectionHorizontal
		slot.Padding = 5
		slot.Spacing = 10

		// Highlight selected
		if i == u.Player.SelectedBlockIndex {
			slot.BackgroundColor = rl.Fade(rl.Yellow, 0.3)
		} else {
			slot.BackgroundColor = rl.Fade(rl.Black, 0.5)
		}

		// Color Box
		c := rl.GetColor(uint(block.Color))
		colorBox := minui.NewBox(20, 20, c)
		slot.AddChild(colorBox)

		// Name Label
		nameLabel := minui.NewLabel(block.Name)
		nameLabel.FontSize = 20
		// Highlight text color?
		if i == u.Player.SelectedBlockIndex {
			nameLabel.Color = rl.Yellow
		} else {
			nameLabel.Color = rl.White
		}
		slot.AddChild(nameLabel)

		hotbar.AddChild(slot)
	}

	// Layout and Draw Hotbar
	available := minui.Size{
		Width:  float32(screenWidth),
		Height: float32(screenHeight),
	}

	size := hotbar.ComputeSize(available)

	x := float32(screenWidth) - size.Width - 10
	y := (float32(screenHeight) - size.Height) / 2

	hotbar.SetBounds(minui.Rect{
		X:      x,
		Y:      y,
		Width:  size.Width,
		Height: size.Height,
	})

	hotbar.Draw()
}
