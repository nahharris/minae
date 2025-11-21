package lighting

import (
	"github.com/nahharris/minae/pkg/config"
	"github.com/nahharris/minae/pkg/world"
)

// LightNode represents a position in the BFS queue
type LightNode struct {
	X, Y, Z int
	Level   uint8
}

// CalculateChunkLighting calculates the static skylight propagation for a single chunk.
// Note: For a full infinite world, this needs to handle cross-chunk propagation more robustly (e.g., lighting updates).
// But for generation, this works if we generate neighbors or handle boundaries.
func CalculateChunkLighting(chunk *world.Chunk, w *world.World) {
	// Queue for BFS
	queue := make([]LightNode, 0)

	// 1. Initialize Skylight
	// Go top-down. Sunlight hits the first solid block.
	// Anything above the first solid block gets 15.
	// The first solid block and below get 0 (initially).
	for x := 0; x < config.ChunkWidth; x++ {
		for z := 0; z < config.ChunkWidth; z++ {
			// Start from top
			lightLevel := uint8(15)
			for y := config.ChunkHeight - 1; y >= 0; y-- {
				block := chunk.GetBlock(x, y, z)

				// Determine if block is transparent (can pass light)
				// For now, nil or "Air" is transparent. Leaves/Glass could be too.
				isTransparent := block == nil || block.ID == "minae/air" || block.Name == "Air"

				if isTransparent {
					// If we are still in direct sunlight
					if lightLevel == 15 {
						chunk.SetLight(x, y, z, 15)
						// Add to queue to propagate sideways
						// Convert to Global coordinates for the queue to handle world boundaries if we expand
						// But here we are working locally on the chunk mostly, but we need world context for neighbors.
						// Let's use Global Coordinates in the queue to be safe and consistent.
						globalX := chunk.X*config.ChunkWidth + x
						globalZ := chunk.Z*config.ChunkWidth + z
						queue = append(queue, LightNode{globalX, y, globalZ, 15})
					} else {
						// Not direct sunlight (under overhang), set to 0 initially
						chunk.SetLight(x, y, z, 0)
					}
				} else {
					// Solid block stops direct sunlight
					lightLevel = 0
					chunk.SetLight(x, y, z, 0)
				}
			}
		}
	}

	// 2. Propagate Light (BFS)
	directions := [][3]int{
		{1, 0, 0}, {-1, 0, 0},
		{0, 1, 0}, {0, -1, 0},
		{0, 0, 1}, {0, 0, -1},
	}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]

		// If light level is 1 or 0, it cannot propagate further (0 would become -1)
		if node.Level <= 1 {
			continue
		}

		for _, dir := range directions {
			nx, ny, nz := node.X+dir[0], node.Y+dir[1], node.Z+dir[2]

			// Check Bounds (Y axis only, X/Z are infinite/world-based)
			if ny < 0 || ny >= config.ChunkHeight {
				continue
			}

			// Get Neighbor Block
			// Use World.GetBlock to handle chunk boundaries seamlessly
			neighborBlock := w.GetBlock(nx, ny, nz)

			// If neighbor is solid, it blocks light.
			isTransparent := neighborBlock == nil || neighborBlock.ID == "minae/air" || neighborBlock.Name == "Air"
			if !isTransparent {
				continue
			}

			// Get current light level of neighbor
			currentLevel := w.GetLight(nx, ny, nz)

			// Propagate if new level is brighter
			// Decrease by 1
			newLevel := node.Level - 1

			// Special Case: Downwards propagation for skylight?
			// In Minecraft, skylight propagates at full strength downwards through air.
			// But we handled the "Direct Sunlight" in Step 1.
			// Here we are propagating *dispersed* light (e.g. into caves).
			// Wait, if we have a hole in the ceiling, Step 1 fills the column with 15.
			// Then BFS propagates from that column into the cave.
			// So standard -1 decay is correct for "dispersed" light.

			if newLevel > currentLevel {
				w.SetLight(nx, ny, nz, newLevel)
				queue = append(queue, LightNode{nx, ny, nz, newLevel})
			}
		}
	}
}

