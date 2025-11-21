package ui

import (
	"github.com/nahharris/minae/minui"
	"github.com/nahharris/minae/pkg/player"
	"github.com/nahharris/minae/pkg/world"
	"github.com/nahharris/minae/pkg/world/lighting"
)

// UIManager handles drawing of overlays and menus.
type UIManager struct {
	ShowAllUI bool
	ShowDebug bool

	// Dependencies
	Player   *player.Player
	World    *world.World
	Lighting *lighting.Manager

	// UI Components
	hudRoot       minui.Component
	debugRoot     minui.Component
	debugInfoRoot minui.Component
	pauseRoot     minui.Component

	// Pause menu interaction state
	ResumeClicked bool
	QuitClicked   bool
}

// NewUIManager creates a new UIManager.
func NewUIManager(p *player.Player, w *world.World, l *lighting.Manager) *UIManager {
	u := &UIManager{
		ShowAllUI: true,
		ShowDebug: false,
		Player:    p,
		World:     w,
		Lighting:  l,
	}
	u.initHUD()
	u.initDebug()
	u.initPauseMenu()
	return u
}
