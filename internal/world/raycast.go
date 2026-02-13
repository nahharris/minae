package world

import (
	"math"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/blocks"
)

// Raycast performs a 3D DDA raycast to find the first block hit.
// start: The starting position of the ray.
// dir: The direction vector of the ray (should be normalized).
// maxDist: The maximum distance to check.
// Returns:
// - hit: true if a block was hit
// - pos: The global coordinates of the hit block [x, y, z]
// - face: The normal of the face that was hit [x, y, z] (e.g., [0, 1, 0] for top)
// - block: The block that was hit
func (w *World) Raycast(start, dir rl.Vector3, maxDist float32) (bool, [3]int, [3]int, *blocks.Block) {
	// Initial voxel coordinates
	x := int(math.Floor(float64(start.X)))
	y := int(math.Floor(float64(start.Y)))
	z := int(math.Floor(float64(start.Z)))

	// Step direction
	stepX := 1
	stepY := 1
	stepZ := 1

	// Ray length (t) required to traverse one unit in each dimension
	var tDeltaX, tDeltaY, tDeltaZ float64

	// Ray length (t) to reach the next voxel boundary
	var tMaxX, tMaxY, tMaxZ float64

	// Calculate steps and deltas
	if dir.X < 0 {
		stepX = -1
		tDeltaX = -1.0 / float64(dir.X)
		tMaxX = (float64(x) - float64(start.X)) / float64(dir.X)
	} else {
		stepX = 1
		tDeltaX = 1.0 / float64(dir.X)
		tMaxX = (float64(x) + 1.0 - float64(start.X)) / float64(dir.X)
	}

	if dir.Y < 0 {
		stepY = -1
		tDeltaY = -1.0 / float64(dir.Y)
		tMaxY = (float64(y) - float64(start.Y)) / float64(dir.Y)
	} else {
		stepY = 1
		tDeltaY = 1.0 / float64(dir.Y)
		tMaxY = (float64(y) + 1.0 - float64(start.Y)) / float64(dir.Y)
	}

	if dir.Z < 0 {
		stepZ = -1
		tDeltaZ = -1.0 / float64(dir.Z)
		tMaxZ = (float64(z) - float64(start.Z)) / float64(dir.Z)
	} else {
		stepZ = 1
		tDeltaZ = 1.0 / float64(dir.Z)
		tMaxZ = (float64(z) + 1.0 - float64(start.Z)) / float64(dir.Z)
	}

	// Face normal of the last step
	faceX, faceY, faceZ := 0, 0, 0

	// Traverse
	dist := 0.0
	for dist < float64(maxDist) {
		// Check current block
		b := w.GetBlock(x, y, z)
		if b != nil && b != blocks.Air { // Assuming blocks.Air is the singleton or nil
			return true, [3]int{x, y, z}, [3]int{faceX, faceY, faceZ}, b
		}

		// Move to next voxel
		if tMaxX < tMaxY {
			if tMaxX < tMaxZ {
				x += stepX
				dist = tMaxX
				tMaxX += tDeltaX
				faceX, faceY, faceZ = -stepX, 0, 0
			} else {
				z += stepZ
				dist = tMaxZ
				tMaxZ += tDeltaZ
				faceX, faceY, faceZ = 0, 0, -stepZ
			}
		} else {
			if tMaxY < tMaxZ {
				y += stepY
				dist = tMaxY
				tMaxY += tDeltaY
				faceX, faceY, faceZ = 0, -stepY, 0
			} else {
				z += stepZ
				dist = tMaxZ
				tMaxZ += tDeltaZ
				faceX, faceY, faceZ = 0, 0, -stepZ
			}
		}
	}

	return false, [3]int{}, [3]int{}, nil
}
