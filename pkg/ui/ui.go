package ui

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
