package blocks

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

	numericID NumID
}
