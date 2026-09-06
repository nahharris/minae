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
