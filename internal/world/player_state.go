package world

import "github.com/nahharris/minae/internal/blocks"

// PlayerState holds the saveable data for a player.
type PlayerState struct {
	Position  [3]float32
	Inventory []*blocks.Block
}

// NewPlayerState creates a new PlayerState with default values.
func NewPlayerState() *PlayerState {
	return &PlayerState{
		Position:  [3]float32{0, 40, 0}, // Default spawn
		Inventory: blocks.GetAll(),
	}
}

