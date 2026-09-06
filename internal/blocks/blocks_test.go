package blocks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nahharris/minae/internal/blocks/model"
)

func TestLoad(t *testing.T) {
	Reset()
	tmpDir := t.TempDir()
	blocksDir := filepath.Join(tmpDir, "blocks")
	if err := os.MkdirAll(blocksDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test block file
	blockFile := filepath.Join(blocksDir, "test.yaml")
	content := []byte("name: Test Block\ncolor: 0xFF0000FF\n")
	if err := os.WriteFile(blockFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Create nested block file
	nestedDir := filepath.Join(blocksDir, "nested")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nestedDir, "deep.yaml")
	nestedContent := []byte("name: Deep Block\ncolor: 0x00FF00FF\n")
	if err := os.WriteFile(nestedFile, nestedContent, 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Verify root block
	b := Get("test")
	if b == nil {
		t.Fatal("Block 'test' not found")
	}
	if b.Name != "Test Block" {
		t.Errorf("Expected name 'Test Block', got '%s'", b.Name)
	}
	if b.Color != 0xFF0000FF {
		t.Errorf("Expected color 0xFF0000FF, got 0x%X", b.Color)
	}

	// Verify nested block
	// On Windows, path separator is \, but we normalize to /
	b2 := Get("nested/deep")
	if b2 == nil {
		t.Fatal("Block 'nested/deep' not found")
	}
	if b2.Name != "Deep Block" {
		t.Errorf("Expected name 'Deep Block', got '%s'", b2.Name)
	}
}

func TestLightLevel(t *testing.T) {
	t.Run("round-trips through a YAML definition", func(t *testing.T) {
		Reset()
		tmpDir := t.TempDir()
		blocksDir := filepath.Join(tmpDir, "blocks")
		if err := os.MkdirAll(blocksDir, 0755); err != nil {
			t.Fatal(err)
		}

		blockFile := filepath.Join(blocksDir, "lantern.yaml")
		content := []byte("name: Lantern\ncolor: 0xFFFFAAFF\nlight_level: 12\n")
		if err := os.WriteFile(blockFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		if err := Load(tmpDir); err != nil {
			t.Fatal(err)
		}

		b := Get("lantern")
		if b == nil {
			t.Fatal("Block 'lantern' not found")
		}
		if got, want := b.LightLevel, uint8(12); got != want {
			t.Errorf("LightLevel = %d, want %d", got, want)
		}
	})

	t.Run("defaults to zero when omitted", func(t *testing.T) {
		Reset()
		tmpDir := t.TempDir()
		blocksDir := filepath.Join(tmpDir, "blocks")
		if err := os.MkdirAll(blocksDir, 0755); err != nil {
			t.Fatal(err)
		}

		blockFile := filepath.Join(blocksDir, "plain.yaml")
		content := []byte("name: Plain Block\ncolor: 0x123456FF\n")
		if err := os.WriteFile(blockFile, content, 0644); err != nil {
			t.Fatal(err)
		}

		if err := Load(tmpDir); err != nil {
			t.Fatal(err)
		}

		b := Get("plain")
		if b == nil {
			t.Fatal("Block 'plain' not found")
		}
		if got, want := b.LightLevel, uint8(0); got != want {
			t.Errorf("LightLevel = %d, want %d", got, want)
		}
	})

	t.Run("clamps out-of-range levels on registration", func(t *testing.T) {
		tests := []struct {
			name  string
			level uint8
			want  uint8
		}{
			{"just over max", 16, 15},
			{"far over max", 200, 15},
			{"at max", 15, 15},
			{"zero", 0, 0},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				Reset()
				b := Register(&Block{ID: "minae/test_light", Name: "Test Light", LightLevel: tt.level})
				if got := b.LightLevel; got != tt.want {
					t.Errorf("LightLevel = %d, want %d", got, tt.want)
				}
			})
		}
	})
}

func TestGlowstone(t *testing.T) {
	ResetToVanilla()

	if Glowstone == nil {
		t.Fatal("Glowstone is not registered")
	}
	if got, want := Glowstone.LightLevel, uint8(15); got != want {
		t.Errorf("Glowstone.LightLevel = %d, want %d", got, want)
	}

	// LightLevel must survive ResetToVanilla, since Reset alone would leave
	// the package-level Glowstone variable holding InvalidNumericID.
	ResetToVanilla()
	b := Get("minae/glowstone")
	if b == nil {
		t.Fatal("Block 'minae/glowstone' not found after ResetToVanilla")
	}
	if got, want := b.LightLevel, uint8(15); got != want {
		t.Errorf("after ResetToVanilla, LightLevel = %d, want %d", got, want)
	}
}

// Re-registering an existing block ID must carry every field across. This is
// asserted structurally rather than field by field: a hand-written merge only
// updates the fields someone remembered, and forgetting one fails silently —
// the definition reloads and that property quietly keeps its old value.
// LightLevel was missed exactly this way. A new field on Block is covered by
// this test automatically.
func TestRegister_ReRegistrationCopiesEveryField(t *testing.T) {
	Reset()

	Register(&Block{ID: "minae/test_merge", Name: "Old", Color: 0x111111FF})

	updated := &Block{
		ID:         "minae/test_merge",
		Name:       "New",
		Color:      0x22334455,
		LightLevel: 9,
		ModelSpec: model.ModelSpec{
			Type:     "slab",
			Textures: map[string]string{"top": "minae/stone"},
		},
	}
	got := Register(updated)

	want := *updated
	// numericID is owned by the registry, not by the definition, and Model is
	// derived from ModelSpec by ensureModel. Everything else must match.
	want.numericID = got.numericID
	want.Model = got.Model

	if !reflect.DeepEqual(*got, want) {
		t.Errorf("re-registration dropped at least one field.\n got: %+v\nwant: %+v", *got, want)
	}
}
