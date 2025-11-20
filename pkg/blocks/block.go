package blocks

// Block represents a voxel type definition.
type Block struct {
	ID    string `yaml:"-"`     // Inferred from file path (e.g., "minae/stone")
	Name  string `yaml:"name"`  // Human readable name
	Color uint32 `yaml:"color"` // Hex color 0xRRGGBBAA
}
