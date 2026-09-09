package blocks

import "github.com/nahharris/minae/internal/blocks/model"

// NumID uniquely identifies a block within runtime registries and chunk storage.
// A value of 0 represents air/empty space.
type NumID uint16

const (
	// InvalidNumericID is reserved for "air"/empty storage.
	InvalidNumericID NumID = 0

	airBlockID = "minae/air"
)

// Block represents a voxel type definition.
type Block struct {
	ID    string `yaml:"-"`     // Inferred from file path (e.g., "minae/stone")
	Name  string `yaml:"name"`  // Human readable name
	Color uint32 `yaml:"color"` // Hex color 0xRRGGBBAA

	// ModelSpec is the YAML-friendly description of how this block should be rendered.
	ModelSpec model.ModelSpec `yaml:"model"`

	// Model is the compiled runtime block model.
	Model model.BlockModel `yaml:"-"`

	// LightLevel is how much light this block emits, 0..15. Zero means the block
	// emits nothing, which is the common case and the useful zero value.
	LightLevel uint8 `yaml:"light_level"`

	// LightTransparent reports whether light passes through this block, as
	// opposed to being blocked by it. Before M18 every non-air block was
	// opaque, so a single `block == nil` check answered this question
	// correctly by accident; leaves are the first registered block for which
	// it must actually be asked. See OpaqueToLight, which is the single place
	// both the light engine and Chunk.highestSolidY derive this from, so the
	// two cannot silently disagree about what "opaque" means.
	LightTransparent bool `yaml:"light_transparent"`

	// SelfCulling reports whether two adjacent instances of this exact block
	// hide the face between them, even though the block does not otherwise
	// occlude a neighbour's face (see Block.HidesFaceOf). An ordinary opaque
	// block never needs this — it already hides every neighbour's face
	// unconditionally. It exists for a block like leaves that is otherwise
	// seen through (LightTransparent) but would render as a mass of pointless
	// interior geometry if every face between two instances of it were drawn.
	//
	// This is declared independently of LightTransparent rather than derived
	// from it: a future transparent block (stained glass, say) need not make
	// the same self-culling choice leaves do, and conflating the two is
	// exactly the mistake this milestone's "three properties, not one
	// predicate" design decision warns against repeating.
	SelfCulling bool `yaml:"self_culling"`

	numericID NumID
}

func (b *Block) ensureModel() {
	if b == nil || b.ID == airBlockID {
		return
	}
	if b.Model == nil {
		b.Model = model.CompileModel(b.ID, b.ModelSpec)
	}
}

// OpaqueToLight reports whether a block blocks light, i.e. whether the light
// engine must treat it as opaque. Nil (air) is never opaque. A non-nil block
// is opaque unless it declares itself LightTransparent.
//
// This is the single predicate world/lighting's isTransparent negates and
// world.Chunk.highestSolidY is defined in terms of ("the highest block the
// light engine considers opaque" — see that field's doc comment and
// docs/milestones/M18-vegetation-features.md's "light predicate and the sky
// ceiling must be the same predicate" design decision). Both packages import
// blocks already, so routing both through this one function — rather than
// each re-deriving "opaque" from block == nil, or from LightTransparent
// directly — is what makes it structurally impossible for the two to drift
// apart when a future block type arrives.
func OpaqueToLight(b *Block) bool {
	return b != nil && !b.LightTransparent
}

// HidesFaceOf reports whether b, sitting as the neighbour across a face,
// hides the face belonging to other.
//
// Nil (air) hides nothing. An opaque block (OpaqueToLight) always hides a
// neighbour's face, exactly as every block behaved before this milestone. A
// LightTransparent block (leaves) hides a neighbour's face only when that
// neighbour is the exact same block and declares SelfCulling — otherwise a
// solid block sitting behind translucent leaves would have its own face
// culled away along with theirs, leaving a hole where the leaves don't fully
// cover it.
//
// The caller (internal/gfx/mesh's face culling) still separately asks the
// neighbour's Model whether it geometrically covers the region in question —
// HidesFaceOf only decides whether that geometric question is worth asking at
// all for this pair of blocks.
func (b *Block) HidesFaceOf(other *Block) bool {
	if b == nil {
		return false
	}
	if OpaqueToLight(b) {
		return true
	}
	return b.SelfCulling && other == b
}
