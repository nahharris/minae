package blocks

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/nahharris/minae/internal/platform/logging"
	"gopkg.in/yaml.v3"
)

// maxLightLevel is the highest light level a block may emit.
const maxLightLevel uint8 = 15

// log is the logger for the blocks package. internal/platform/logging is
// itself required to stay free of raylib (see internal/archtest), so
// depending on it here does not compromise the simulation layer's purity.
var log = logging.ForPackage("blocks")

// Registry manages all loaded block definitions.
type Registry struct {
	blocks map[string]*Block
	ids    map[string]NumID
	byID   []*Block
	nextID NumID
	mu     sync.RWMutex
}

var (
	// Global registry instance
	globalRegistry = newRegistry()
)

// opaqueByID is a lock-free, copy-on-write cache of OpaqueToLight indexed by
// NumID, rebuilt (via rebuildOpaqueCacheLocked) every time the registry's ID
// table changes.
//
// It exists for hot per-block-write paths -- world.Chunk.SetBlock and
// SetBlockState -- that need this one boolean about the block a write is
// replacing far more often than they need the *Block itself, and cannot
// afford globalRegistry.mu's lock on every call: chunk generation alone
// calls SetBlock on the order of 16,000 times per chunk. FromNumericID
// remains correct and is still what everything else uses; this cache is a
// narrower, faster answer to a narrower question, computed from the exact
// same OpaqueToLight function FromNumericID's callers would get, so the two
// can never disagree about what "opaque" means for a given ID -- only about
// how fast the question can be answered.
var opaqueByID atomic.Pointer[[]bool]

// rebuildOpaqueCacheLocked recomputes opaqueByID from globalRegistry.byID.
// Callers must already hold globalRegistry.mu (for writing) — it reads byID
// directly rather than through the locking accessors.
func rebuildOpaqueCacheLocked() {
	cache := make([]bool, len(globalRegistry.byID))
	for id, b := range globalRegistry.byID {
		cache[id] = OpaqueToLight(b)
	}
	opaqueByID.Store(&cache)
}

// OpaqueToLightID is OpaqueToLight looked up by numeric ID instead of by
// *Block, for callers that only have the compact storage form. It answers
// exactly the same question OpaqueToLight(FromNumericID(id)) would -- air
// (InvalidNumericID) is never opaque, any other ID reports whatever the
// registry last computed for it via OpaqueToLight -- but does so through the
// lock-free opaqueByID cache instead of FromNumericID's mutex, which matters
// on the hot SetBlock/SetBlockState path (see opaqueByID's doc comment).
func OpaqueToLightID(id NumID) bool {
	if id == InvalidNumericID {
		return false
	}
	cache := opaqueByID.Load()
	if cache == nil {
		return false
	}
	idx := int(id)
	if idx < 0 || idx >= len(*cache) {
		return false
	}
	return (*cache)[idx]
}

func newRegistry() *Registry {
	return &Registry{
		blocks: make(map[string]*Block),
		ids:    make(map[string]NumID),
		byID:   make([]*Block, 1), // Index 0 reserved for air/empty
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

	if b == nil {
		return nil
	}

	if b.LightLevel > maxLightLevel {
		log.Warnf("block %q declares light_level %d, clamping to %d", b.ID, b.LightLevel, maxLightLevel)
		b.LightLevel = maxLightLevel
	}

	b.ensureModel()

	if existing, ok := globalRegistry.blocks[b.ID]; ok {
		// Overwrite in place so existing *Block pointers stay valid, copying
		// the whole struct rather than field by field. Enumerating fields here
		// means every future one has to be remembered, and forgetting a field
		// does not fail loudly: the definition reloads and that property
		// silently keeps its old value. LightLevel was already missed once.
		//
		// numericID is the sole exception. It is assigned by the registry, not
		// by the definition, and the incoming block has not been allocated one.
		id := existing.numericID
		*existing = *b
		existing.numericID = id

		existing.ensureModel()
		rebuildOpaqueCacheLocked()
		return existing
	}

	id := globalRegistry.allocateID(b.ID)
	b.numericID = id
	b.ensureModel()

	globalRegistry.blocks[b.ID] = b
	globalRegistry.ids[b.ID] = id
	globalRegistry.ensureByIDCapacity(id)
	globalRegistry.byID[id] = b

	rebuildOpaqueCacheLocked()
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
	globalRegistry.ids = make(map[string]NumID)
	globalRegistry.byID = make([]*Block, 1)
	globalRegistry.nextID = 1

	rebuildOpaqueCacheLocked()
}

func (r *Registry) allocateID(blockID string) NumID {
	if blockID == airBlockID {
		return InvalidNumericID
	}
	id := r.nextID
	r.nextID++
	return id
}

func (r *Registry) ensureByIDCapacity(id NumID) {
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
		// Start from existing definition (if any) so that partial YAML overrides
		// don't unintentionally zero fields like model settings.
		if existing := Get(id); existing != nil {
			block = *existing
		}
		if err := yaml.Unmarshal(data, &block); err != nil {
			return fmt.Errorf("failed to parse block file %s: %w", path, err)
		}

		block.ID = id
		// Recompile the model after applying YAML overrides.
		block.Model = nil
		Register(&block) // This will overwrite if exists
		return nil
	})

	return err
}

// NumericIDOf returns the compact numeric ID for the given block (or InvalidNumericID if nil/air).
func NumericIDOf(b *Block) NumID {
	if b == nil || b.ID == airBlockID {
		return InvalidNumericID
	}
	if b.numericID != InvalidNumericID {
		return b.numericID
	}

	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	if id, ok := globalRegistry.ids[b.ID]; ok {
		b.numericID = id
		return id
	}
	return InvalidNumericID
}

// FromNumericID returns the registered block for the given numeric ID.
// InvalidNumericID returns nil (air).
func FromNumericID(id NumID) *Block {
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
