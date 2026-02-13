package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapDataFolder_Default(t *testing.T) {
	// Unset env var to test default
	os.Unsetenv("MINAE_DATA_FOLDER")

	path, err := BootstrapDataFolder()
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".minae")
	if path != expected {
		t.Errorf("Expected default path %s, got %s", expected, path)
	}

	// Clean up
	// os.RemoveAll(path) // Be careful deleting home dir stuff, maybe just leave it or mock
}

func TestBootstrapDataFolder_Env(t *testing.T) {
	tmpDir := t.TempDir()
	targetDir := filepath.Join(tmpDir, "data")
	os.Setenv("MINAE_DATA_FOLDER", targetDir)
	defer os.Unsetenv("MINAE_DATA_FOLDER")

	path, err := BootstrapDataFolder()
	if err != nil {
		t.Fatalf("Bootstrap failed: %v", err)
	}

	if path != targetDir {
		t.Errorf("Expected path %s, got %s", targetDir, path)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("Data folder was not created")
	}
}

func TestLoad_DumpDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	// Ensure config doesn't exist
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatal("Config file should not exist yet")
	}

	// Load should create it
	if err := Load(tmpDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}

	// Check content was read back into Current
	if Current.ScreenWidth != 1280 { // Default
		t.Errorf("Expected default ScreenWidth 1280, got %d", Current.ScreenWidth)
	}
}

func TestLoad_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")
	content := []byte("screen_width: 800\ntarget_fps: 30\n")
	if err := os.WriteFile(configFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	if err := Load(tmpDir); err != nil {
		t.Fatal(err)
	}

	if Current.ScreenWidth != 800 {
		t.Errorf("Expected ScreenWidth 800, got %d", Current.ScreenWidth)
	}
	if Current.TargetFPS != 30 {
		t.Errorf("Expected TargetFPS 30, got %d", Current.TargetFPS)
	}
	// Check default preserved
	if Current.ScreenHeight != 720 {
		t.Errorf("Expected default ScreenHeight 720, got %d", Current.ScreenHeight)
	}
}
