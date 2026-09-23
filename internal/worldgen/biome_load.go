package worldgen

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/nahharris/minae/internal/blocks"
	"gopkg.in/yaml.v3"
)

// This file follows the precedent internal/blocks/registry.go's Load already
// sets: definitions are plain YAML, parsed into a strict intermediate form,
// then validated field by field with an error that names the file and the
// field -- never a silently wrong world (the milestone's own phrasing).
//
// The one deliberate difference from blocks.Load: the vanilla biome set is
// embedded into the binary with go:embed rather than read from the player's
// data folder, per the milestone's "biomes are data, embedded in the binary"
// design decision. Tests and a fresh install see exactly the same three
// biomes without depending on anything on disk.

//go:embed data/biomes/*.yaml
var embeddedBiomesFS embed.FS

// biomeYAML is the on-disk shape of one biome file, before validation turns
// it into a Biome. Point and Weights are maps keyed by axis name (see
// axisNames) rather than a fixed-size array, so a file that misspells or
// omits an axis fails validation instead of silently unmarshalling into a
// zero-valued slot.
type biomeYAML struct {
	ID       string             `yaml:"id"`
	Surface  string             `yaml:"surface"`
	Filler   string             `yaml:"filler"`
	Point    map[string]float64 `yaml:"point"`
	Weights  map[string]float64 `yaml:"weights"`
	Features []featureYAML      `yaml:"features"`
}

type featureYAML struct {
	Kind        string `yaml:"kind"`
	Numerator   int    `yaml:"numerator"`
	Denominator int    `yaml:"denominator"`
}

// DefaultBiomes returns the vanilla biome set embedded in the binary. It
// panics if that embedded data fails to load or validate: exactly like
// NewGenerator panicking on a malformed spline, the embedded data is not
// something a running program can recover from -- it can only ever be wrong
// because of a bug shipped in this package, never because of something a
// player did.
func DefaultBiomes() *BiomeSet {
	set, err := LoadBiomesFS(embeddedBiomesFS, "data/biomes")
	if err != nil {
		panic("worldgen: embedded vanilla biomes: " + err.Error())
	}
	return set
}

// LoadBiomesFS reads every *.yaml/*.yml file directly inside dir (no
// recursion -- biomes, unlike blocks, have no need for a namespacing
// directory tree) within fsys, validates them together, and returns the
// resulting BiomeSet.
//
// Every error names the file it came from and, wherever it identifies one,
// the offending field -- criterion 8's own requirement, restated as this
// function's contract.
func LoadBiomesFS(fsys fs.FS, dir string) (*BiomeSet, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("reading biome directory %s: %w", dir, err)
	}

	var biomes []*Biome
	seenID := make(map[string]string)          // biome ID -> file that declared it
	seenPoint := make(map[ClimatePoint]string) // exact point -> biome ID that sits there

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		names = append(names, name)
	}
	// Sort file names before parsing so validation errors (in particular,
	// which file a duplicate ID or point is reported "already declared in")
	// are deterministic regardless of the filesystem's own directory order.
	sort.Strings(names)

	for _, name := range names {
		path := dir + "/" + name
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("reading biome file %s: %w", path, err)
		}

		var raw biomeYAML
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parsing biome file %s: %w", path, err)
		}

		b, err := validateBiome(path, raw)
		if err != nil {
			return nil, err
		}

		if existingFile, ok := seenID[b.ID]; ok {
			return nil, fmt.Errorf("biome file %s: duplicate biome id %q (already declared in %s)", path, b.ID, existingFile)
		}
		seenID[b.ID] = path

		if existingID, ok := seenPoint[b.Point]; ok {
			return nil, fmt.Errorf("biome file %s: biome %q sits at the identical climate point as %q -- "+
				"nearest-neighbour selection cannot distinguish them", path, b.ID, existingID)
		}
		seenPoint[b.Point] = b.ID

		biomes = append(biomes, b)
	}

	if len(biomes) == 0 {
		return nil, fmt.Errorf("no biome definitions found in %s", dir)
	}

	sortByID(biomes)
	return &BiomeSet{biomes: biomes}, nil
}

// validateBiome turns one parsed biomeYAML into a Biome, or returns an error
// naming path and the offending field. It is the single place every
// criterion-8 error class is checked:
//
//   - unknown block names (surface/filler that blocks.Get cannot resolve)
//   - missing axes (a point or weights map missing an entry axisNames lists)
//   - out-of-range values (a point outside [climateMin, climateMax], or a
//     negative weight)
//   - unknown feature kinds (not one of the classes the milestone lists by
//     name, but the same "reject bad data at load" principle, and the only
//     way an invalid `kind:` string could otherwise reach featureKind
//     silently as zero, i.e. a tree)
//
// Duplicate IDs and identical points are cross-biome properties and are
// checked by the caller, once every file has been parsed.
func validateBiome(path string, raw biomeYAML) (*Biome, error) {
	if raw.ID == "" {
		return nil, fmt.Errorf("biome file %s: field %q is required", path, "id")
	}

	pointArr, err := parseAxisMap(path, "point", raw.Point, true)
	if err != nil {
		return nil, err
	}
	point := ClimatePoint(pointArr)

	weightsArr, err := parseAxisMap(path, "weights", raw.Weights, false)
	if err != nil {
		return nil, err
	}
	weights := Weights(weightsArr)
	if sumOf(weights) <= 0 {
		return nil, fmt.Errorf("biome file %s: field %q: at least one axis must have a positive weight", path, "weights")
	}

	surface := blocks.Get(raw.Surface)
	if surface == nil {
		return nil, fmt.Errorf("biome file %s: field %q: unknown block %q", path, "surface", raw.Surface)
	}
	filler := blocks.Get(raw.Filler)
	if filler == nil {
		return nil, fmt.Errorf("biome file %s: field %q: unknown block %q", path, "filler", raw.Filler)
	}

	features := make([]FeatureDef, 0, len(raw.Features))
	seenKind := make(map[featureKind]bool, len(raw.Features))
	for i, f := range raw.Features {
		kind, ok := parseFeatureKind(f.Kind)
		if !ok {
			return nil, fmt.Errorf("biome file %s: field %q: unknown feature kind %q", path, fmt.Sprintf("features[%d].kind", i), f.Kind)
		}
		if seenKind[kind] {
			return nil, fmt.Errorf("biome file %s: field %q: feature kind %q declared more than once", path, fmt.Sprintf("features[%d].kind", i), f.Kind)
		}
		seenKind[kind] = true
		if f.Denominator <= 0 {
			return nil, fmt.Errorf("biome file %s: field %q: denominator must be positive, got %d", path, fmt.Sprintf("features[%d].denominator", i), f.Denominator)
		}
		if f.Numerator < 0 || f.Numerator > f.Denominator {
			return nil, fmt.Errorf("biome file %s: field %q: numerator must be between 0 and denominator (%d), got %d", path, fmt.Sprintf("features[%d].numerator", i), f.Denominator, f.Numerator)
		}
		features = append(features, FeatureDef{Kind: kind, Numerator: f.Numerator, Denominator: f.Denominator})
	}

	return &Biome{
		ID:       raw.ID,
		Point:    point,
		Weights:  weights,
		Surface:  surface,
		Filler:   filler,
		Features: features,
	}, nil
}

// parseAxisMap reads every axis in axisNames out of m, keyed by field
// (either "point" or "weights", used only for error messages), enforcing
// that every axis is present and, when rangeChecked, inside
// [climateMin, climateMax]. Weights are never range-checked against
// [climateMin, climateMax] -- they are not climate values -- but are
// rejected if negative, by the caller.
//
// It returns the plain [numAxes]float64 array rather than either named type
// (ClimatePoint, Weights) so both call sites in validateBiome can convert to
// whichever one they need; the two are structurally identical but kept as
// distinct types so a point can never be passed where weights are expected,
// or vice versa, anywhere else in this package.
func parseAxisMap(path, field string, m map[string]float64, rangeChecked bool) ([numAxes]float64, error) {
	var p [numAxes]float64
	for axis := Axis(0); axis < numAxes; axis++ {
		name := axisNames[axis]
		v, ok := m[name]
		if !ok {
			return p, fmt.Errorf("biome file %s: field %q: missing axis %q", path, field, name)
		}
		if rangeChecked && (v < climateMin || v > climateMax) {
			return p, fmt.Errorf("biome file %s: field %q: axis %q value %v is outside [%v, %v]", path, field, name, v, climateMin, climateMax)
		}
		if !rangeChecked && v < 0 {
			return p, fmt.Errorf("biome file %s: field %q: axis %q weight %v must not be negative", path, field, name, v)
		}
		p[axis] = v
	}
	if len(m) > int(numAxes) {
		for name := range m {
			if !isKnownAxis(name) {
				return p, fmt.Errorf("biome file %s: field %q: unknown axis %q", path, field, name)
			}
		}
	}
	return p, nil
}

func isKnownAxis(name string) bool {
	for _, n := range axisNames {
		if n == name {
			return true
		}
	}
	return false
}

func sumOf(w Weights) float64 {
	var total float64
	for _, v := range w {
		total += v
	}
	return total
}

// parseFeatureKind maps a YAML "kind:" string to a featureKind, the one
// place that string form is interpreted.
func parseFeatureKind(s string) (featureKind, bool) {
	switch s {
	case "tree":
		return featureTree, true
	case "bush":
		return featureBush, true
	default:
		return 0, false
	}
}
