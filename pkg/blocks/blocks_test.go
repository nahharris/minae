package blocks

import (
	"os"
	"path/filepath"
	"testing"
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
