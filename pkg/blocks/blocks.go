package blocks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	Air = "minae/air"
	Stone = "minae/stone"
	Dirt = "minae/dirt"
)

// Block represents a voxel type definition.
type Block struct {
	ID    string `yaml:"-"`     // Inferred from file path (e.g., "minae/stone")
	Name  string `yaml:"name"`  // Human readable name
	Color uint32 `yaml:"color"` // Hex color 0xRRGGBBAA
}

// Registry manages all loaded block definitions.
type Registry struct {
	blocks map[string]*Block
	mu     sync.RWMutex
}

var (
	// Global registry instance
	globalRegistry = &Registry{
		blocks: make(map[string]*Block),
	}
)

// Get returns a block definition by ID.
// Returns nil if not found.
func Get(id string) *Block {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	return globalRegistry.blocks[id]
}

// Register adds a block to the registry.
func Register(b *Block) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.blocks[b.ID] = b
}

// Reset clears the registry (useful for tests/reloading).
func Reset() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.blocks = make(map[string]*Block)
}

// RegisterVanilla adds the hardcoded vanilla blocks to the registry.
func RegisterVanilla() {
	Register(&Block{ID: Air, Name: "Air", Color: 0x00000000})
	Register(&Block{ID: Stone, Name: "Stone", Color: 0x828282FF})
	Register(&Block{ID: Dirt, Name: "Dirt", Color: 0x7F513DFF})
}

// Load recursively walks the blocks directory and loads all YAML definitions.
// It overrides existing definitions if IDs match.
func Load(dataFolder string) error {
	blocksPath := filepath.Join(dataFolder, "blocks")

	// If blocks directory doesn't exist, just skip loading custom blocks
	if _, err := os.Stat(blocksPath); os.IsNotExist(err) {
		return nil
	}

	err := filepath.Walk(blocksPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if filepath.Ext(path) != ".yaml" && filepath.Ext(path) != ".yml" {
			return nil
		}

		// Infer ID from path relative to blocks directory
		// e.g., .../data/blocks/minae/stone.yaml -> minae/stone
		relPath, err := filepath.Rel(blocksPath, path)
		if err != nil {
			return err
		}

		// Remove extension
		id := strings.TrimSuffix(relPath, filepath.Ext(relPath))
		// Ensure forward slashes for consistency across OS
		id = strings.ReplaceAll(id, string(os.PathSeparator), "/")

		// Parse YAML
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read block file %s: %w", path, err)
		}

		var block Block
		if err := yaml.Unmarshal(data, &block); err != nil {
			return fmt.Errorf("failed to parse block file %s: %w", path, err)
		}

		block.ID = id
		Register(&block) // This will overwrite if exists
		return nil
	})

	return err
}
