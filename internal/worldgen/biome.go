package worldgen

import (
	"sort"

	"github.com/nahharris/minae/internal/blocks"
)

// This file is the data model and selection logic for M19 biomes -- see
// docs/milestones/M19-biome-selection.md. biome_load.go turns YAML into the
// types declared here; generator.go and chunk.go are the only callers that
// use them to affect a generated world.
//
// The central architectural constraint this file exists under: nothing here
// may ever be consulted by SurfaceHeight. Biomes describe what a column
// looks like, never how tall it is. See generator.go's SurfaceHeight doc
// comment and the milestone's "the finding that reshapes this milestone".

// Axis identifies one of the six climate parameters biome selection ranges
// over. The first three already drive terrain shape (M17); the last three
// exist for biome selection only, in this milestone.
type Axis int

const (
	AxisContinentalness Axis = iota
	AxisErosion
	AxisPeaksValleys
	AxisTemperature
	AxisHumidity
	AxisMysticness

	numAxes
)

// axisNames maps each Axis to the YAML field name biome_load.go reads it
// from, and back again in validation error messages. Keeping this the one
// place the string form is spelled out means a renamed axis cannot silently
// desync between the loader and its error messages.
var axisNames = [numAxes]string{
	AxisContinentalness: "continentalness",
	AxisErosion:         "erosion",
	AxisPeaksValleys:    "peaks_and_valleys",
	AxisTemperature:     "temperature",
	AxisHumidity:        "humidity",
	AxisMysticness:      "mysticness",
}

// climateMin and climateMax bound every axis value: noise.Seed.FBm2D
// guarantees its output stays in [-1, 1] (see internal/noise/fbm.go), and
// every axis here -- including the three new ones -- is sampled from FBm2D,
// so a declared point outside this range can never be reached by any column
// and is almost certainly a typo.
const (
	climateMin = -1.0
	climateMax = 1.0
)

// ClimatePoint is a point in the six-axis parameter space: either a column's
// sampled climate (Generator.climateAt) or a biome's declared point.
type ClimatePoint [numAxes]float64

// Weights scales each axis' contribution to a biome's own distance
// computation. Weights belong to the biome, not to the selector: two biomes
// may legitimately disagree about which axes matter to them (the milestone's
// "weighting per axis is allowed... part of the data" design decision).
type Weights [numAxes]float64

// FeatureDef is one (kind, density) pair a biome declares. A biome with no
// FeatureDefs (dunes) places no features at all.
//
// Numerator/Denominator express density as an exact integer fraction, the
// same reason features.go's original treeChanceNumerator/Denominator did:
// the accept roll it feeds (paintRoot) stays an exact integer comparison
// against a raw splitmix64 draw, with no float conversion in the hot path.
type FeatureDef struct {
	Kind        featureKind
	Numerator   int
	Denominator int
}

// densityFor reports whether roll (a raw splitmix64 draw) accepts, for a
// feature at this density: exactly the same shape of check
// candidateRoots.tree-vs-bush coin flip already used.
func (f FeatureDef) accepts(roll uint64) bool {
	if f.Denominator <= 0 {
		return false
	}
	return roll%uint64(f.Denominator) < uint64(f.Numerator)
}

// Biome is one fully-validated biome definition: a point and per-axis
// weights in climate space, its surface and filler blocks, and the features
// it allows at their root.
type Biome struct {
	ID       string
	Point    ClimatePoint
	Weights  Weights
	Surface  *blocks.Block
	Filler   *blocks.Block
	Features []FeatureDef
}

// featureDensity returns the FeatureDef this biome declares for kind, and
// whether it declares one at all. A biome that never mentions a kind never
// places it -- this is the mechanism behind "a feature is placed if the
// biome at its root column allows it" (the milestone's design decision).
func (b *Biome) featureDensity(kind featureKind) (FeatureDef, bool) {
	for _, f := range b.Features {
		if f.Kind == kind {
			return f, true
		}
	}
	return FeatureDef{}, false
}

// BiomeSet is a validated, immutable collection of biomes, ready for nearest-
// neighbour selection. The zero value is not usable; construct one via
// LoadBiomesFS or DefaultBiomes.
type BiomeSet struct {
	// biomes is kept sorted by ID. Nearest-neighbour selection does not
	// depend on this order -- ties are broken explicitly by ID, never by
	// position in this slice (see Select) -- but a fixed order makes the
	// set's own tests and error messages reproducible.
	biomes []*Biome
}

// Biomes returns every biome in the set, sorted by ID. Callers must not
// mutate the returned slice's elements.
func (bs *BiomeSet) Biomes() []*Biome {
	return bs.biomes
}

// maxFeatureRadiusOf returns the largest declared feature's radius across
// every biome in bs, via featureRadiusFor. This is the milestone's "the
// global feature radius becomes the maximum over every feature in every
// biome" made concrete: whatever the data declares, this is computed from
// it rather than assumed from the two kinds features.go happens to
// implement today.
//
// A set that declares no features anywhere returns 0.
func maxFeatureRadiusOf(bs *BiomeSet) int {
	max := 0
	for _, b := range bs.biomes {
		for _, f := range b.Features {
			if r := featureRadiusFor(f.Kind); r > max {
				max = r
			}
		}
	}
	return max
}

// weightedDistanceSquared computes biome b's own distance from point p: the
// weighted sum of squared per-axis differences, using b's own weights.
//
// This is exactly the shape the milestone singles out as needing the FMA
// guard (docs/milestones/M19-biome-selection.md's "the FMA guard reaches
// worldgen here"): a weighted sum of products, which Go fuses into a single
// FMADDD on arm64 and not on amd64. Every axis' weight*diff*diff term is
// folded into the running sum via addProduct (fma.go) instead of a bare
// `sum += w*diffSq`, so the sum is bit-identical everywhere.
func weightedDistanceSquared(p, q ClimatePoint, w Weights) float64 {
	var sum float64
	for i := 0; i < int(numAxes); i++ {
		diff := p[i] - q[i]
		// diff*diff is a pure product with no addition attached to it in
		// this expression -- no fusion risk regardless of grouping, the
		// same reasoning generator.go's rawSurfaceHeight gives for
		// peaksValue*peakAmplitude. Only folding w[i]*diffSq into sum needs
		// forcing.
		diffSq := diff * diff
		sum = addProduct(w[i], diffSq, sum)
	}
	return sum
}

// Select returns the biome nearest to p in the weighted six-axis space,
// where "nearest" is measured by each candidate's own weights (see
// weightedDistanceSquared).
//
// Ties -- including exact float64 ties -- are broken by biome ID,
// lexicographically, never by a biome's position in bs.biomes and never by
// iteration order. This is the milestone's "ties must be broken by biome ID"
// design decision: Go deliberately randomises map iteration, so any tie
// resolved by it would make the same seed produce different worlds on
// different runs. bs.biomes is a slice, not a map, precisely so this
// function does not have that hazard to begin with -- but the explicit
// ID comparison below is what actually guarantees the tie-break, not the
// slice's incidental order.
func (bs *BiomeSet) Select(p ClimatePoint) *Biome {
	var best *Biome
	bestDist := 0.0
	for _, b := range bs.biomes {
		d := weightedDistanceSquared(p, b.Point, b.Weights)
		switch {
		case best == nil:
			best, bestDist = b, d
		case d < bestDist:
			best, bestDist = b, d
		case d == bestDist && b.ID < best.ID:
			best, bestDist = b, d
		}
	}
	return best
}

// sortByID sorts biomes in place by ID. Exists as its own function so
// biome_load.go's construction path and any future test fixture build a
// BiomeSet the same way.
func sortByID(biomes []*Biome) {
	sort.Slice(biomes, func(i, j int) bool { return biomes[i].ID < biomes[j].ID })
}
