package world

import (
	"math"

	"github.com/nahharris/minae/internal/blocks"
	"github.com/nahharris/minae/internal/core"
)

// Raycast performs a 3D DDA raycast to find the first block hit.
// start: The starting position of the ray.
// dir: The direction vector of the ray. It need not be unit length: Raycast
// normalizes it internally, so maxDist is always measured in world units
// regardless of dir's magnitude. A zero-length dir has no direction to
// travel in, so the ray reports no hit.
// maxDist: The maximum distance to check, in world units.
// Returns:
// - hit: true if a block was hit
// - pos: The global coordinates of the hit block [x, y, z]
// - face: The normal of the face that was hit [x, y, z] (e.g., [0, 1, 0] for top)
// - block: The block that was hit
func (w *World) Raycast(start, dir core.Vec3, maxDist float32) (bool, [3]int, [3]int, *blocks.Block) {
	dir = dir.Normalize()
	if dir == (core.Vec3{}) {
		// No direction to travel in. A ray with no direction cannot reach any
		// block, including the one start happens to sit inside, so this is
		// not treated as an immediate hit.
		return false, [3]int{}, [3]int{}, nil
	}

	// Initial voxel coordinates
	x := int(math.Floor(float64(start.X)))
	y := int(math.Floor(float64(start.Y)))
	z := int(math.Floor(float64(start.Z)))

	// Step direction. Assigned in every branch of the delta calculation below.
	var stepX, stepY, stepZ int

	// Ray length (t) required to traverse one unit in each dimension
	var tDeltaX, tDeltaY, tDeltaZ float64

	// Ray length (t) to reach the next voxel boundary
	var tMaxX, tMaxY, tMaxZ float64

	// Calculate steps and deltas. A component of exactly zero is handled
	// before the sign check: +0.0 and -0.0 both compare equal to 0, but
	// -0.0 fails "< 0" (it is not less than zero), so without this explicit
	// branch a -0.0 component would fall into the positive branch below and
	// divide by a negative-signed zero, producing -Inf for tDelta/tMax. A
	// -Inf tMax is smaller than every other axis on every iteration, so that
	// axis would be chosen forever and the loop would never terminate.
	// Routing exact zero to +Inf here, regardless of its sign, means that
	// axis is simply never chosen - which is correct for a ray that does not
	// move along it.
	if dir.X == 0 {
		stepX = 0
		tDeltaX = math.Inf(1)
		tMaxX = math.Inf(1)
	} else if dir.X < 0 {
		stepX = -1
		tDeltaX = -1.0 / float64(dir.X)
		tMaxX = (float64(x) - float64(start.X)) / float64(dir.X)
	} else {
		stepX = 1
		tDeltaX = 1.0 / float64(dir.X)
		tMaxX = (float64(x) + 1.0 - float64(start.X)) / float64(dir.X)
	}

	if dir.Y == 0 {
		stepY = 0
		tDeltaY = math.Inf(1)
		tMaxY = math.Inf(1)
	} else if dir.Y < 0 {
		stepY = -1
		tDeltaY = -1.0 / float64(dir.Y)
		tMaxY = (float64(y) - float64(start.Y)) / float64(dir.Y)
	} else {
		stepY = 1
		tDeltaY = 1.0 / float64(dir.Y)
		tMaxY = (float64(y) + 1.0 - float64(start.Y)) / float64(dir.Y)
	}

	if dir.Z == 0 {
		stepZ = 0
		tDeltaZ = math.Inf(1)
		tMaxZ = math.Inf(1)
	} else if dir.Z < 0 {
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
