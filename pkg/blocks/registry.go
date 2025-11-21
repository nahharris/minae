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
	ids    map[string]NumericID
	byID   []*Block
	nextID NumericID
	mu     sync.RWMutex
}

var (
	// Global registry instance
	globalRegistry = newRegistry()
)

func newRegistry() *Registry {
	return &Registry{
		blocks: make(map[string]*Block),
		ids:    make(map[string]NumericID),
		byID:   make([]*Block, 1), // index 0 reserved for air/empty
		nextID: 1,
	}
}

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

	if existing, ok := globalRegistry.blocks[b.ID]; ok {
		existing.Name = b.Name
		existing.Color = b.Color
		return existing
	}

	id := globalRegistry.allocateID(b.ID)
	b.numericID = id

	globalRegistry.blocks[b.ID] = b
	globalRegistry.ids[b.ID] = id
	globalRegistry.ensureByIDCapacity(id)
	globalRegistry.byID[id] = b

	return b
}

// Reset clears the registry (useful for tests/reloading).
func Reset() {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	for _, b := range globalRegistry.blocks {
		b.numericID = InvalidNumericID
	}

	globalRegistry.blocks = make(map[string]*Block)
	globalRegistry.ids = make(map[string]NumericID)
	globalRegistry.byID = make([]*Block, 1)
	globalRegistry.nextID = 1
}

func (r *Registry) allocateID(blockID string) NumericID {
	if blockID == airBlockID {
		return InvalidNumericID
	}
	id := r.nextID
	r.nextID++
	return id
}

func (r *Registry) ensureByIDCapacity(id NumericID) {
	idx := int(id)
	if idx < len(r.byID) {
		return
	}
	newByID := make([]*Block, idx+1)
	copy(newByID, r.byID)
	r.byID = newByID
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

// NumericIDOf returns the compact numeric ID for the given block (or InvalidNumericID if nil/air).
func NumericIDOf(b *Block) NumericID {
	if b == nil || b.ID == airBlockID {
		return InvalidNumericID
	}
	if b.numericID != InvalidNumericID {
		return b.numericID
	}

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	id, ok := globalRegistry.ids[b.ID]
	if ok {
		b.numericID = id
		return id
	}
	return InvalidNumericID
}

// FromNumericID returns the registered block for the given numeric ID.
// InvalidNumericID returns nil (air).
func FromNumericID(id NumericID) *Block {
	if id == InvalidNumericID {
		return nil
	}

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	if idx := int(id); idx < len(globalRegistry.byID) {
		return globalRegistry.byID[idx]
	}
	return nil
}
