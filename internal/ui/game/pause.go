package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
	minui "github.com/nahharris/minae/internal/ui/core"
)

func (u *UIManager) initPauseMenu() {
	panel := minui.NewPanel()
	// Border is not supported in Panel directly yet (it just draws background).
	// We could add a Border component or just draw it manually.
	// Or simply use a colored panel.
	panel.BackgroundColor = rl.LightGray
	panel.Padding = 20
	panel.Spacing = 10
	panel.Direction = minui.DirectionVertical
	panel.Alignment = minui.AlignCenter // Center buttons

	// Title
	title := minui.NewLabel("PAUSED")
	title.Color = rl.Black
	panel.AddChild(title)

	// Resume Button
	resume := minui.NewButton("Resume", func() {
		u.ResumeClicked = true
	})
	panel.AddChild(resume)

	// Quit Button
	quit := minui.NewButton("Quit", func() {
		u.QuitClicked = true
	})
	quit.NormalColor = rl.Maroon
	quit.HoverColor = rl.Red
	panel.AddChild(quit)

	u.pauseRoot = panel
}

// DrawPauseMenu draws the pause menu and handles input.
// Returns resume (true if Resume clicked) and quit (true if Quit clicked).
func (u *UIManager) DrawPauseMenu(screenWidth, screenHeight int) (bool, bool) {
	if !u.ShowAllUI {
		return false, false
	}

	// Reset flags
	u.ResumeClicked = false
	u.QuitClicked = false

	// Dim background
	rl.DrawRectangle(0, 0, int32(screenWidth), int32(screenHeight), rl.Fade(rl.Black, 0.4))

	// Center Panel
	// Use Fixed Size for menu?
	menuWidth := float32(200)
	menuHeight := float32(160) // approx

	menuX := (float32(screenWidth) - menuWidth) / 2
	menuY := (float32(screenHeight) - menuHeight) / 2

	// Set bounds for root
	u.pauseRoot.SetBounds(minui.Rect{X: menuX, Y: menuY, Width: menuWidth, Height: menuHeight})

	// Draw outline manually since Panel doesn't support it
	rl.DrawRectangleLines(int32(menuX), int32(menuY), int32(menuWidth), int32(menuHeight), rl.Black)

	// Update and Draw
	u.pauseRoot.Update()
	u.pauseRoot.Draw()

	return u.ResumeClicked, u.QuitClicked
}
