package blocks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

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

// GetAll returns all registered blocks, sorted by ID.
func GetAll() []*Block {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	blocks := make([]*Block, 0, len(globalRegistry.blocks))
	for _, b := range globalRegistry.blocks {
		blocks = append(blocks, b)
	}

	sort.Slice(blocks, func(i, j int) bool {
		return blocks[i].ID < blocks[j].ID
	})

	return blocks
}

// Register adds a block to the registry.
func Register(b *Block) *Block {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.blocks[b.ID] = b
	return b
}

// Reset clears the registry (useful for tests/reloading).
func Reset() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()
	globalRegistry.blocks = make(map[string]*Block)
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
