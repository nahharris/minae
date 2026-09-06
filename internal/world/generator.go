package world

import (
	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/platform/config"
)

// GenerateTerrain fills the chunk with simple debug terrain.
func GenerateTerrain(c *Chunk) {
	for x := range config.ChunkWidth {
		for z := range config.ChunkWidth {
			height := 32
			for y := 0; y < height; y++ {
				if y < height-3 {
					c.SetBlock(x, y, z, blocks.Stone)
				} else if y < height-1 {
					c.SetBlock(x, y, z, blocks.Dirt)
				} else {
					c.SetBlock(x, y, z, blocks.Grass)
				}
			}
		}
	}
}
