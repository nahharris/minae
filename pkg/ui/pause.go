package ui

import (
	rl "github.com/gen2brain/raylib-go/raylib"
)

// DrawPauseMenu draws the pause menu and handles input.
// Returns resume (true if Resume clicked) and quit (true if Quit clicked).
func (u *UIManager) DrawPauseMenu(screenWidth, screenHeight int) (bool, bool) {
	if !u.ShowAllUI {
		// Even if UI is hidden, we might be paused. But if UI is hidden globally (F1),
		// strictly speaking we shouldn't see the menu.
		// However, if the game IS paused, the user needs to see how to unpause or quit.
		// The requirement "F1 key should hide all UIs" implies overlays.
		// If the pause menu is essential for interaction, maybe it should override?
		// For now, let's assume F1 hides EVERYTHING including the pause menu,
		// but ESC can still toggle pause state in Game.Update.
		// If UI is hidden, we return false, false (no interaction from UI).
		return false, false
	}

	// Dim background
	rl.DrawRectangle(0, 0, int32(screenWidth), int32(screenHeight), rl.Fade(rl.Black, 0.4))

	// Menu Box
	menuWidth := int32(200)
	menuHeight := int32(160)
	menuX := (int32(screenWidth) - menuWidth) / 2
	menuY := (int32(screenHeight) - menuHeight) / 2

	rl.DrawRectangle(menuX, menuY, menuWidth, menuHeight, rl.LightGray)
	rl.DrawRectangleLines(menuX, menuY, menuWidth, menuHeight, rl.Black)

	// Title
	title := "PAUSED"
	titleWidth := rl.MeasureText(title, 20)
	rl.DrawText(title, menuX+(menuWidth-titleWidth)/2, menuY+10, 20, rl.Black)

	// Buttons
	buttonWidth := int32(160)
	buttonHeight := int32(40)
	buttonX := menuX + (menuWidth-buttonWidth)/2
	
	// Resume Button
	resumeY := menuY + 50
	resumeBtn := rl.NewRectangle(float32(buttonX), float32(resumeY), float32(buttonWidth), float32(buttonHeight))
	resumeHover := rl.CheckCollisionPointRec(rl.GetMousePosition(), resumeBtn)
	
	if resumeHover {
		rl.DrawRectangleRec(resumeBtn, rl.Gray)
	} else {
		rl.DrawRectangleRec(resumeBtn, rl.DarkGray)
	}
	rl.DrawText("Resume", buttonX+40, resumeY+10, 20, rl.White)

	// Quit Button
	quitY := resumeY + 50
	quitBtn := rl.NewRectangle(float32(buttonX), float32(quitY), float32(buttonWidth), float32(buttonHeight))
	quitHover := rl.CheckCollisionPointRec(rl.GetMousePosition(), quitBtn)

	if quitHover {
		rl.DrawRectangleRec(quitBtn, rl.Red)
	} else {
		rl.DrawRectangleRec(quitBtn, rl.Maroon)
	}
	rl.DrawText("Quit", buttonX+55, quitY+10, 20, rl.White)

	// Input Handling
	if rl.IsMouseButtonReleased(rl.MouseLeftButton) {
		if resumeHover {
			return true, false
		}
		if quitHover {
			return false, true
		}
	}

	return false, false
}

