package chunks

import (
	"github.com/nahharris/minae/internal/gfx/mesh"
	"github.com/nahharris/minae/internal/world"
)

// The snapshot must be substitutable for the live world wherever the mesher
// reads, and its centre chunk for the chunk being meshed.
var (
	_ mesh.WorldReader = (*Snapshot)(nil)
	_ mesh.ChunkReader = (*world.Chunk)(nil)
)
