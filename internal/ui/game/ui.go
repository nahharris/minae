package ui

import (
	"runtime"

	"github.com/nahharris/minae/internal/player"
	minui "github.com/nahharris/minae/internal/ui/core"
	"github.com/nahharris/minae/internal/world"
)

// UIManager handles drawing of overlays and menus.
type UIManager struct {
	ShowAllUI bool
	ShowDebug bool

	// Dependencies
	Player *player.Player
	World  *world.World
	Time   *world.TimeOfDay

	// UI Components
	hudRoot       minui.Component
	debugRoot     minui.Component
	debugInfoRoot minui.Component
	pauseRoot     minui.Component

	// Pause menu interaction state
	ResumeClicked bool
	QuitClicked   bool

	// Cached debug strings to avoid per-frame allocations
	debugFPS        string
	debugTime       string
	debugAlloc      string
	debugTotalAlloc string
	debugSys        string
	debugXYZ        string
	debugChunk      string
	debugDir        string
	// Cached values to detect changes
	cachedFPS        int
	cachedTime       string
	cachedAlloc      uint64
	cachedTotalAlloc uint64
	cachedSys        uint64
	cachedXYZ        [3]float32
	cachedChunk      [4]int
	cachedDir        [3]float32

	memStats runtime.MemStats
}

// NewUIManager creates a new UIManager.
func NewUIManager(p *player.Player, w *world.World, t *world.TimeOfDay) *UIManager {
	u := &UIManager{
		ShowAllUI: true,
		ShowDebug: false,
		Player:    p,
		World:     w,
		Time:      t,
	}
	u.initHUD()
	u.initDebug()
	u.initPauseMenu()
	return u
}

func (u *UIManager) captureMemStats() {
	runtime.ReadMemStats(&u.memStats)
}
