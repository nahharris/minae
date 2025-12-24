package blocks

import "log"

var (
	Air   = Register(&Block{ID: "minae/air", Name: "Air", Color: 0x00000000})
	Stone = Register(&Block{ID: "minae/stone", Name: "Stone", Color: 0x828282FF})
	Dirt  = Register(&Block{ID: "minae/dirt", Name: "Dirt", Color: 0x7F513DFF})
	Grass = Register(&Block{
		ID:    "minae/grass",
		Name:  "Grass",
		Color: 0x32CD32FF,
		ModelSpec: ModelSpec{
			Type: "sided",
			Textures: map[string]string{
				"top":    "minae/grass_top",
				"bottom": "minae/dirt",
				"side":   "minae/grass_side",
			},
		},
	})
	Wood = Register(&Block{ID: "minae/wood", Name: "Wood", Color: 0x8B4513FF})
)

func init() {
	log.Println("Initializing blocks")
}
