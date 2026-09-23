package worldgen

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/nahharris/minae/internal/blocks"
)

// validPlains, validForest and validDunes are minimal, individually-valid
// biome YAML documents used as building blocks across the validation tests
// below: each test starts from one of these and mutates exactly the field
// under test, so a failure clearly indicts that one change rather than
// something incidental about the fixture.
const validPlains = `
id: minae/plains
surface: minae/grass
filler: minae/dirt
point:
  continentalness: 0.0
  erosion: 0.0
  peaks_and_valleys: 0.0
  temperature: 0.0
  humidity: 0.0
  mysticness: -1.0
weights:
  continentalness: 0.02
  erosion: 0.02
  peaks_and_valleys: 0.02
  temperature: 1.0
  humidity: 1.0
  mysticness: 0.05
features:
  - kind: bush
    numerator: 1
    denominator: 4
`

const validForest = `
id: minae/forest
surface: minae/grass
filler: minae/dirt
point:
  continentalness: 0.0
  erosion: 0.0
  peaks_and_valleys: 0.0
  temperature: 0.0
  humidity: 0.6
  mysticness: -1.0
weights:
  continentalness: 0.02
  erosion: 0.02
  peaks_and_valleys: 0.02
  temperature: 1.0
  humidity: 1.0
  mysticness: 0.05
features:
  - kind: tree
    numerator: 3
    denominator: 5
`

// singleFS builds a one-file fstest.MapFS at data/biomes/<name>, the shape
// LoadBiomesFS expects.
func singleFS(name, content string) fstest.MapFS {
	return fstest.MapFS{
		"data/biomes/" + name: &fstest.MapFile{Data: []byte(content)},
	}
}

func twoFileFS(nameA, contentA, nameB, contentB string) fstest.MapFS {
	return fstest.MapFS{
		"data/biomes/" + nameA: &fstest.MapFile{Data: []byte(contentA)},
		"data/biomes/" + nameB: &fstest.MapFile{Data: []byte(contentB)},
	}
}

// TestDefaultBiomesLoadsTheVanillaSet exercises the real embedded data: the
// three biomes the milestone specifies exist, are individually valid, and
// the surface/filler blocks they reference resolve against the real vanilla
// block registry (criterion 8's positive case -- valid data loads cleanly).
func TestDefaultBiomesLoadsTheVanillaSet(t *testing.T) {
	blocks.ResetToVanilla()

	set := DefaultBiomes()
	got := map[string]*Biome{}
	for _, b := range set.Biomes() {
		got[b.ID] = b
	}

	wantIDs := []string{"minae/plains", "minae/forest", "minae/dunes"}
	if len(got) != len(wantIDs) {
		t.Fatalf("DefaultBiomes() loaded %d biomes, want exactly %d: got %v", len(got), len(wantIDs), got)
	}
	for _, id := range wantIDs {
		b, ok := got[id]
		if !ok {
			t.Fatalf("DefaultBiomes() missing biome %q", id)
		}
		if b.Surface == nil || b.Filler == nil {
			t.Fatalf("biome %q has a nil surface or filler block", id)
		}
	}

	dunes := got["minae/dunes"]
	if dunes.Surface != blocks.Sand || dunes.Filler != blocks.Sandstone {
		t.Fatalf("dunes surface/filler = %v/%v, want sand/sandstone", dunes.Surface, dunes.Filler)
	}
	if len(dunes.Features) != 0 {
		t.Fatalf("dunes declares %d features, want 0 (no features is the point of this biome)", len(dunes.Features))
	}

	plains := got["minae/plains"]
	forest := got["minae/forest"]
	if plains.Surface != blocks.Grass || forest.Surface != blocks.Grass {
		t.Fatalf("plains/forest surface = %v/%v, want grass/grass", plains.Surface, forest.Surface)
	}
	if len(plains.Features) == 0 || len(forest.Features) == 0 {
		t.Fatal("plains and forest must each declare at least one feature")
	}
}

// TestLoadBiomes_RejectsUnknownBlock is criterion 8's "unknown block names"
// class.
func TestLoadBiomes_RejectsUnknownBlock(t *testing.T) {
	blocks.ResetToVanilla()

	bad := strings.Replace(validPlains, "surface: minae/grass", "surface: minae/not_a_real_block", 1)
	_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
	if err == nil {
		t.Fatal("expected an error for an unknown surface block, got nil")
	}
	if !strings.Contains(err.Error(), "plains.yaml") || !strings.Contains(err.Error(), "surface") || !strings.Contains(err.Error(), "not_a_real_block") {
		t.Fatalf("error %q does not name the file and field", err.Error())
	}
}

// TestLoadBiomes_RejectsDuplicateID is criterion 8's "duplicate IDs" class.
func TestLoadBiomes_RejectsDuplicateID(t *testing.T) {
	blocks.ResetToVanilla()

	dup := strings.Replace(validForest, "id: minae/forest", "id: minae/plains", 1)
	_, err := LoadBiomesFS(twoFileFS("a_plains.yaml", validPlains, "b_dup.yaml", dup), "data/biomes")
	if err == nil {
		t.Fatal("expected an error for a duplicate biome id, got nil")
	}
	if !strings.Contains(err.Error(), "b_dup.yaml") || !strings.Contains(err.Error(), "minae/plains") {
		t.Fatalf("error %q does not name the offending file and the duplicated id", err.Error())
	}
}

// TestLoadBiomes_RejectsIdenticalPoint is criterion 8's "two biomes at the
// identical point" class: same six-axis point (even with different IDs and
// weights) is rejected, because nearest-neighbour selection could never
// distinguish them.
func TestLoadBiomes_RejectsIdenticalPoint(t *testing.T) {
	blocks.ResetToVanilla()

	// forest normally differs from plains only in humidity; pin it to
	// plains' exact point (leaving the id different) to construct the
	// identical-point case.
	sameSpot := strings.Replace(validForest, "humidity: 0.6", "humidity: 0.0", 1)
	_, err := LoadBiomesFS(twoFileFS("a_plains.yaml", validPlains, "b_forest.yaml", sameSpot), "data/biomes")
	if err == nil {
		t.Fatal("expected an error for two biomes at the identical climate point, got nil")
	}
	if !strings.Contains(err.Error(), "b_forest.yaml") || !strings.Contains(err.Error(), "identical") {
		t.Fatalf("error %q does not name the file and describe the identical-point conflict", err.Error())
	}
}

// TestLoadBiomes_RejectsMissingAxis is criterion 8's "missing axes" class,
// checked for both the point and the weights maps.
func TestLoadBiomes_RejectsMissingAxis(t *testing.T) {
	blocks.ResetToVanilla()

	t.Run("point", func(t *testing.T) {
		bad := strings.Replace(validPlains, "  mysticness: -1.0\n", "", 1)
		_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
		if err == nil {
			t.Fatal("expected an error for a missing point axis, got nil")
		}
		if !strings.Contains(err.Error(), "point") || !strings.Contains(err.Error(), "mysticness") {
			t.Fatalf("error %q does not name the field and the missing axis", err.Error())
		}
	})

	t.Run("weights", func(t *testing.T) {
		bad := strings.Replace(validPlains, "  mysticness: 0.05\n", "", 1)
		_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
		if err == nil {
			t.Fatal("expected an error for a missing weight axis, got nil")
		}
		if !strings.Contains(err.Error(), "weights") || !strings.Contains(err.Error(), "mysticness") {
			t.Fatalf("error %q does not name the field and the missing axis", err.Error())
		}
	})
}

// TestLoadBiomes_RejectsOutOfRangeValue is criterion 8's "out-of-range
// values" class: a point axis outside [-1, 1] (FBm2D's own documented
// range, which no column's climate can ever leave) and a negative weight.
func TestLoadBiomes_RejectsOutOfRangeValue(t *testing.T) {
	blocks.ResetToVanilla()

	t.Run("point above range", func(t *testing.T) {
		bad := strings.Replace(validPlains, "temperature: 0.0", "temperature: 1.5", 1)
		_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
		if err == nil {
			t.Fatal("expected an error for a point axis outside [-1,1], got nil")
		}
		if !strings.Contains(err.Error(), "point") || !strings.Contains(err.Error(), "temperature") {
			t.Fatalf("error %q does not name the field and the out-of-range axis", err.Error())
		}
	})

	t.Run("negative weight", func(t *testing.T) {
		bad := strings.Replace(validPlains, "temperature: 1.0", "temperature: -0.5", 1)
		_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
		if err == nil {
			t.Fatal("expected an error for a negative weight, got nil")
		}
		if !strings.Contains(err.Error(), "weights") {
			t.Fatalf("error %q does not name the weights field", err.Error())
		}
	})
}

// TestLoadBiomes_RejectsAllZeroWeights guards against a degenerate biome
// that would be equidistant from every point in the space: not one of the
// milestone's five named classes, but the same "reject bad data at load"
// principle -- see validateBiome's doc comment.
func TestLoadBiomes_RejectsAllZeroWeights(t *testing.T) {
	blocks.ResetToVanilla()

	bad := validPlains
	for _, from := range []string{"continentalness: 0.02", "erosion: 0.02", "peaks_and_valleys: 0.02", "temperature: 1.0", "humidity: 1.0", "mysticness: 0.05"} {
		axis := strings.SplitN(from, ":", 2)[0]
		bad = strings.Replace(bad, from, axis+": 0.0", 1)
	}
	_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
	if err == nil {
		t.Fatal("expected an error for all-zero weights, got nil")
	}
}

// TestLoadBiomes_RejectsUnknownFeatureKind guards against a typo'd `kind:`
// silently becoming featureKind's zero value (a tree) rather than being
// rejected -- also not one of the five named classes, but the same
// principle.
func TestLoadBiomes_RejectsUnknownFeatureKind(t *testing.T) {
	blocks.ResetToVanilla()

	bad := strings.Replace(validPlains, "kind: bush", "kind: mushroom", 1)
	_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
	if err == nil {
		t.Fatal("expected an error for an unknown feature kind, got nil")
	}
	if !strings.Contains(err.Error(), "mushroom") {
		t.Fatalf("error %q does not name the unknown kind", err.Error())
	}
}

// TestLoadBiomes_RejectsUnknownAxis guards against a typo'd axis name in
// the point or weights map (e.g. "temperture") being silently ignored --
// which would otherwise leave that axis at its zero value with no error at
// all, exactly the "silently wrong world" the milestone's own framing warns
// against.
func TestLoadBiomes_RejectsUnknownAxis(t *testing.T) {
	blocks.ResetToVanilla()

	bad := strings.Replace(validPlains, "  mysticness: -1.0\n", "  mysticness: -1.0\n  altitude: 0.0\n", 1)
	_, err := LoadBiomesFS(singleFS("plains.yaml", bad), "data/biomes")
	if err == nil {
		t.Fatal("expected an error for an unknown axis, got nil")
	}
	if !strings.Contains(err.Error(), "altitude") {
		t.Fatalf("error %q does not name the unknown axis", err.Error())
	}
}

// TestLoadBiomes_RejectsEmptySet guards the load-time invariant every
// selection function relies on: BiomeSet.Select assumes at least one biome
// exists.
func TestLoadBiomes_RejectsEmptySet(t *testing.T) {
	// A directory that exists but contains no *.yaml/*.yml files -- as
	// opposed to a directory that does not exist at all, which fails for an
	// entirely different (also correct) reason inside fs.ReadDir.
	fsys := fstest.MapFS{"data/biomes/readme.txt": &fstest.MapFile{Data: []byte("not a biome")}}
	_, err := LoadBiomesFS(fsys, "data/biomes")
	if err == nil {
		t.Fatal("expected an error for a directory with no biome files, got nil")
	}
}
