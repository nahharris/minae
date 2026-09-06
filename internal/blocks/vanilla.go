package blocks

import (
	"github.com/nahharris/minae/internal/blocks/model"
)

var (
	Air       = Register(&Block{ID: "minae/air", Name: "Air", Color: 0x00000000})
	Stone     = Register(&Block{ID: "minae/stone", Name: "Stone", Color: 0x828282FF})
	StoneSlab = Register(&Block{
		ID:    "minae/stone_slab",
		Name:  "Stone Slab",
		Color: 0x828282FF,
		ModelSpec: model.ModelSpec{
			Type: "slab",
			Textures: map[string]string{
				"top":    "minae/stone",
				"bottom": "minae/stone",
				"side":   "minae/stone",
			},
		},
	})
	Dirt  = Register(&Block{ID: "minae/dirt", Name: "Dirt", Color: 0x7F513DFF})
	Grass = Register(&Block{
		ID:    "minae/grass",
		Name:  "Grass",
		Color: 0x32CD32FF,
		ModelSpec: model.ModelSpec{
			Type: "sided",
			Textures: map[string]string{
				"top":    "minae/grass_top",
				"bottom": "minae/dirt",
				"side":   "minae/grass_side",
			},
		},
	})
	Wood      = Register(&Block{ID: "minae/wood", Name: "Wood", Color: 0x8B4513FF})
	Glowstone = Register(&Block{ID: "minae/glowstone", Name: "Glowstone", Color: 0xFFD98CFF, LightLevel: 15})
)

// vanillaBlocks returns every built-in block, in registration order.
func vanillaBlocks() []*Block {
	return []*Block{Air, Stone, StoneSlab, Dirt, Grass, Wood, Glowstone}
}

// ResetToVanilla clears the registry and re-registers the built-in blocks.
// Reset on its own leaves the package-level block variables holding
// InvalidNumericID, so anything stored through them silently becomes air.
// Tests that need a clean registry should call this instead.
func ResetToVanilla() {
	Reset()
	for _, b := range vanillaBlocks() {
		Register(b)
	}
}
