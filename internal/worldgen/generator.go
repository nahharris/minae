package worldgen

import (
	"math"

	"github.com/nahharris/minae/internal/noise"
)

// SeaLevel is the reference height the splines below are shaped around.
//
// It places no water: there is no water block registered (see
// internal/blocks/vanilla.go) and oceans are explicitly out of scope for
// this milestone. It exists purely so that when oceans do arrive, the land
// does not have to be re-tuned around a different zero -- see the
// milestone's "Sea level is a reference height, not water" design decision.
// Nothing in this package special-cases a column being above or below it.
const SeaLevel = 64

// dirtDepth is how many blocks of dirt sit under the grass. It mirrors
// chunks.FlatGenerator's layering (stone below, two dirt layers, grass on
// top) exactly, so the two generators read as the same visual language
// where tests compare them.
const dirtDepth = 2

// minSurfaceHeight and maxSurfaceHeight bound every column this package
// produces (criterion 5 in the milestone doc): comfortably above y=0 --
// there is no bedrock block for a column to rest directly on top of, so a
// column must never bottom out at the world floor -- and comfortably below
// config.ChunkHeight, so a column always has room for air above its surface.
//
// The noise formula in SurfaceHeight is tuned to stay inside this range on
// its own (see the frequency and amplitude constants below); the clamp here
// is a defensive backstop, not the mechanism the range guarantee actually
// rests on.
const (
	minSurfaceHeight = 32
	maxSurfaceHeight = 128
)

// Frequencies are expressed as a fraction of one block: multiplying a global
// coordinate by, say, continentalFrequency before sampling gives that field
// a wavelength of 1/continentalFrequency blocks. Continentalness is the
// broadest field (large-scale structure -- "how far inland"), erosion a
// little tighter, and peaks-and-valleys the tightest: it is local ridging,
// deliberately mostly suppressed here by peakAmplitude and the erosion
// spline (see NewGenerator), present so the machinery exists for M19 rather
// than because plains need it visible.
const (
	continentalFrequency = 1.0 / 400.0
	erosionFrequency     = 1.0 / 300.0
	peaksFrequency       = 1.0 / 80.0

	// peakAmplitude is the largest possible contribution, in blocks, of the
	// peaks-and-valleys field before the erosion spline suppresses it
	// further. It is deliberately small -- plains read as plains because
	// this stays small, not because the field is absent.
	peakAmplitude = 4.0

	// climateFrequency is shared by temperature, humidity and mysticness
	// (M19): deliberately much lower than any terrain field above, so biome
	// regions read as large, contiguous places rather than a fine-grained
	// speckle. A biome that flips every few columns fails the milestone's
	// criterion 5 outright; see biome_test.go's region-coherence checks for
	// the measured consequence of this constant.
	climateFrequency = 1.0 / 1000.0
)

// fieldConfig is shared by all three noise fields: four octaves at the
// conventional doubling frequency and halving amplitude (see
// noise.FBmConfig's doc comment).
var fieldConfig = noise.FBmConfig{Octaves: 4, Lacunarity: 2, Gain: 0.5}

// Generator produces plains terrain from a single seed.
//
// It implements chunks.Generator (internal/chunks/pipeline.go) structurally,
// without importing that package: internal/chunks already imports
// internal/world, so the reverse import would cycle, exactly as that
// package's own doc comment notes for FlatGenerator.
//
// A *Generator holds no mutable state once NewGenerator returns: its noise
// seeds and splines are fixed at construction, which is what makes
// SurfaceHeight and Generate pure functions of their arguments alone --
// safe to call from many goroutines at once, as Generator requires.
type Generator struct {
	continentalness noise.Seed
	erosion         noise.Seed
	peaksValleys    noise.Seed

	// temperature, humidity and mysticness are M19's three new climate
	// fields (docs/milestones/M19-biome-selection.md). They drive biome
	// selection only -- see climateAt and biome.go's Select -- and are
	// derived from the world seed exactly the way the first three fields
	// are: seed+3, seed+4, seed+5. M17 measured that seed+k derivation
	// produces no cross-seed correlation for k up to 2; nothing about that
	// measurement depends on which k values are used, so it carries over
	// unchanged to k=3,4,5.
	temperature noise.Seed
	humidity    noise.Seed
	mysticness  noise.Seed

	continentalSpline *noise.Spline
	erosionSpline     *noise.Spline
	peaksSpline       *noise.Spline

	// biomes is the validated set Generate and fillColumn select from (see
	// selectBiome). NewGenerator always populates this from DefaultBiomes;
	// newGeneratorWithBiomes exists so tests can substitute a different --
	// including a deliberately absurd -- set without duplicating every
	// other field NewGenerator builds. SurfaceHeight never reads this field,
	// by construction: that is criterion 1, and TestSurfaceHeightIgnoresBiomes
	// checks it by injecting exactly such an absurd set.
	biomes *BiomeSet

	// maxFeatureRadius, featureChunkRadius and poolRadius are derived from
	// biomes once, at construction (see deriveFeatureRadii in features.go),
	// rather than assumed as package constants: the milestone's "the global
	// feature radius becomes the maximum over every feature in every biome"
	// design decision, made literal.
	maxFeatureRadius   int
	featureChunkRadius int
	poolRadius         int

	// worldSeed is the raw seed NewGenerator was built from, kept alongside
	// the derived noise.Seed fields so features.go's per-chunk RNG (see
	// newChunkRNG) can be seeded from it directly. It is never mutated after
	// construction, same as everything else on Generator.
	worldSeed int64
}

// NewGenerator builds a Generator for seed.
//
// Three independent noise fields are derived from seed, seed+1 and seed+2.
// NewSeed's own guarantee (criterion 2 in
// docs/milestones/M16-noise-foundation.md: adjacent seeds already produce
// uncorrelated fields) is exactly what is needed here, so there is no call
// for a more elaborate per-field derivation.
//
// The three splines are declared as control-point data here, once, rather
// than as arithmetic evaluated per column -- see the milestone's "Parameters
// and splines now, even for one biome" design decision. Reshaping terrain
// means moving these points, not touching SurfaceHeight or Generate.
func NewGenerator(seed int64) *Generator {
	return newGeneratorWithBiomes(seed, DefaultBiomes())
}

// newGeneratorWithBiomes is NewGenerator parameterized on the biome set to
// use, so tests can substitute a different -- including a deliberately
// absurd -- set without duplicating the noise/spline construction below.
// Production code has exactly one caller of this, NewGenerator, which always
// passes DefaultBiomes(); everything else is test-only.
func newGeneratorWithBiomes(seed int64, biomes *BiomeSet) *Generator {
	continentalSpline, err := noise.NewSpline([]noise.SplinePoint{
		{X: -1.0, Y: 56},
		{X: -0.4, Y: 60},
		{X: 0.0, Y: SeaLevel},
		{X: 0.4, Y: 70},
		{X: 1.0, Y: 78},
	})
	if err != nil {
		panic("worldgen: continental spline: " + err.Error())
	}

	// "High erosion flattens" (the milestone's own phrasing): this spline's
	// output multiplies the peaks-and-valleys contribution in SurfaceHeight,
	// so a high erosion value shrinks that contribution toward zero instead
	// of leaving local ridging everywhere.
	erosionSpline, err := noise.NewSpline([]noise.SplinePoint{
		{X: -1.0, Y: 0.6},
		{X: 0.0, Y: 0.3},
		{X: 1.0, Y: 0.05},
	})
	if err != nil {
		panic("worldgen: erosion spline: " + err.Error())
	}

	peaksSpline, err := noise.NewSpline([]noise.SplinePoint{
		{X: -1.0, Y: -1.0},
		{X: -0.5, Y: -0.3},
		{X: 0.0, Y: 0.0},
		{X: 0.5, Y: 0.3},
		{X: 1.0, Y: 1.0},
	})
	if err != nil {
		panic("worldgen: peaks-and-valleys spline: " + err.Error())
	}

	maxFeatureRadius, featureChunkRadius, poolRadius := deriveFeatureRadii(biomes)

	return &Generator{
		continentalness:    noise.NewSeed(seed),
		erosion:            noise.NewSeed(seed + 1),
		peaksValleys:       noise.NewSeed(seed + 2),
		temperature:        noise.NewSeed(seed + 3),
		humidity:           noise.NewSeed(seed + 4),
		mysticness:         noise.NewSeed(seed + 5),
		continentalSpline:  continentalSpline,
		erosionSpline:      erosionSpline,
		peaksSpline:        peaksSpline,
		biomes:             biomes,
		maxFeatureRadius:   maxFeatureRadius,
		featureChunkRadius: featureChunkRadius,
		poolRadius:         poolRadius,
		worldSeed:          seed,
	}
}

// terrainFields samples the three noise fields SurfaceHeight and climateAt
// both need -- continentalness, erosion and peaks-and-valleys, in that
// order -- once, so a caller needing both (Generate's per-column loop, via
// heightAndClimate) does not pay for two independent fBm evaluations of the
// same three fields. climateAt and rawSurfaceHeight both call this rather
// than sampling directly, so there is exactly one place these three fields
// are ever read from.
func (g *Generator) terrainFields(x, z int) (continental, erosion, peaksValleys float64) {
	fx, fz := float64(x), float64(z)
	continental = g.continentalness.FBm2D(fx*continentalFrequency, fz*continentalFrequency, fieldConfig)
	erosion = g.erosion.FBm2D(fx*erosionFrequency, fz*erosionFrequency, fieldConfig)
	peaksValleys = g.peaksValleys.FBm2D(fx*peaksFrequency, fz*peaksFrequency, fieldConfig)
	return
}

// climateFields3 samples the three climate-only fields: temperature,
// humidity and mysticness, in that order.
func (g *Generator) climateFields3(x, z int) (temperature, humidity, mysticness float64) {
	fx, fz := float64(x), float64(z)
	temperature = g.temperature.FBm2D(fx*climateFrequency, fz*climateFrequency, fieldConfig)
	humidity = g.humidity.FBm2D(fx*climateFrequency, fz*climateFrequency, fieldConfig)
	mysticness = g.mysticness.FBm2D(fx*climateFrequency, fz*climateFrequency, fieldConfig)
	return
}

// climateAt samples every one of the six climate axes at global column
// (x, z): the first three are the same fields SurfaceHeight reads
// (continentalness, erosion, peaks-and-valleys, via terrainFields), the last
// three -- temperature, humidity, mysticness -- exist for biome selection
// only.
//
// x and z MUST be global coordinates, never chunk-local -- the same
// requirement SurfaceHeight documents, and for the same reason: sampling at
// local coordinates produces a climate that repeats identically in every
// chunk, which criterion 7's test checks directly.
func (g *Generator) climateAt(x, z int) ClimatePoint {
	var p ClimatePoint
	p[AxisContinentalness], p[AxisErosion], p[AxisPeaksValleys] = g.terrainFields(x, z)
	p[AxisTemperature], p[AxisHumidity], p[AxisMysticness] = g.climateFields3(x, z)
	return p
}

// selectBiome returns the biome nearest to global column (x, z) in the
// weighted climate space (see BiomeSet.Select). It never returns nil for a
// non-empty BiomeSet, which DefaultBiomes and LoadBiomesFS both guarantee
// (an empty set is rejected at load, per criterion 8).
func (g *Generator) selectBiome(x, z int) *Biome {
	return g.biomes.Select(g.climateAt(x, z))
}

// heightAndClimate computes SurfaceHeight and climateAt's ClimatePoint for
// the same column in one call, sharing terrainFields' three samples between
// them instead of evaluating each twice.
//
// Generate's per-column loop calls this instead of SurfaceHeight and
// selectBiome separately: BenchmarkGenerateChunk measured that as the
// difference between doubling and not-quite-doubling the milestone's cost,
// since those three fBm fields (four octaves each) dominate both
// computations. The result is bit-for-bit identical to calling
// SurfaceHeight(x, z) and climateAt(x, z) independently -- same values,
// same order of operations, see rawSurfaceHeight -- so this is purely a
// performance path and touches nothing about criterion 1: it still computes
// height from terrainFields alone, never from a selected biome.
func (g *Generator) heightAndClimate(x, z int) (height int, climate ClimatePoint) {
	c, e, pv := g.terrainFields(x, z)

	var p ClimatePoint
	p[AxisContinentalness], p[AxisErosion], p[AxisPeaksValleys] = c, e, pv
	p[AxisTemperature], p[AxisHumidity], p[AxisMysticness] = g.climateFields3(x, z)

	return g.clampHeight(g.rawHeightFromFields(c, e, pv)), p
}

// SurfaceHeight returns the y coordinate of the first air block above the
// ground at global column (x, z): the column is solid from y=0 up to (not
// including) this value, and air from it upward.
//
// It is a pure function of g and (x, z): no access to World, no dependence
// on which chunks exist or what order they were generated in. That is what
// makes it testable independently of Generate (the milestone's criteria 3
// and 6 are both properties of this function alone) and safe to call before
// any chunk exists -- used for spawn placement in internal/game/app.go,
// which needs no chunk to be Generated first.
//
// x and z MUST be global block coordinates, not chunk-local ones. Generate
// is the one call site that matters: sampling at (localX, localZ) instead
// produces terrain that looks entirely plausible in isolation and repeats
// identically in every chunk, with a wall at every seam -- see the
// milestone's design decision on why this is the single most likely bug in
// this package.
func (g *Generator) SurfaceHeight(x, z int) int {
	return g.clampHeight(g.rawSurfaceHeight(x, z))
}

// rawSurfaceHeight is SurfaceHeight without the clamp: the height the noise
// and splines actually produce.
//
// It exists so the range guarantee can be tested where it is really made.
// Asserting that SurfaceHeight returns values inside [minSurfaceHeight,
// maxSurfaceHeight] proves only that clampHeight works, which is not in
// doubt -- a formula that drifted far out of range would pass that test
// while producing a world pinned flat against a ceiling. Measured across
// 540,000 samples and six seeds, the clamp currently never fires at all;
// TestRawSurfaceHeightNeedsNoClamp is what keeps that true.
func (g *Generator) rawSurfaceHeight(x, z int) float64 {
	c, e, pv := g.terrainFields(x, z)
	return g.rawHeightFromFields(c, e, pv)
}

// rawHeightFromFields is rawSurfaceHeight's formula, factored out so
// heightAndClimate can reuse terrainFields' three samples instead of
// resampling them. c, e and pv MUST be exactly what terrainFields(x, z)
// returns for the column being computed -- this function does no sampling
// of its own.
func (g *Generator) rawHeightFromFields(c, e, pv float64) float64 {
	baseHeight := g.continentalSpline.Eval(c)
	erosionFactor := g.erosionSpline.Eval(e)
	peaksValue := g.peaksSpline.Eval(pv)

	// peaksValue*peakAmplitude is a pure product with no addition attached in
	// this expression, so it carries no fusion risk on its own regardless of
	// grouping; it is only the addition of erosionFactor's product into
	// baseHeight that needs forcing, via addProduct.
	scaledPeak := peaksValue * peakAmplitude
	return addProduct(scaledPeak, erosionFactor, baseHeight)
}

// clampHeight is the defensive backstop described on minSurfaceHeight: it
// guarantees criterion 5 unconditionally, for any future retuning of the
// constants above, but it is not the mechanism the guarantee currently rests
// on.
func (g *Generator) clampHeight(height float64) int {
	rounded := int(math.Round(height))
	switch {
	case rounded < minSurfaceHeight:
		return minSurfaceHeight
	case rounded > maxSurfaceHeight:
		return maxSurfaceHeight
	default:
		return rounded
	}
}
