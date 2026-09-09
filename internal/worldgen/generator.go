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

	continentalSpline *noise.Spline
	erosionSpline     *noise.Spline
	peaksSpline       *noise.Spline

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

	return &Generator{
		continentalness:   noise.NewSeed(seed),
		erosion:           noise.NewSeed(seed + 1),
		peaksValleys:      noise.NewSeed(seed + 2),
		continentalSpline: continentalSpline,
		erosionSpline:     erosionSpline,
		peaksSpline:       peaksSpline,
		worldSeed:         seed,
	}
}

// addProduct returns a*b + c as a single correctly-rounded operation, via
// math.FMA.
//
// This mirrors internal/noise/fma.go's madd, and for the same reason: Go
// permits fusing a plain a*b + c into one instruction on arm64 and not on
// amd64, skipping the intermediate IEEE-754 rounding -- which would make the
// same seed generate a different terrain height on different players'
// machines. math.FMA fuses deliberately and identically on every
// architecture instead of leaving it to the compiler. See
// docs/milestones/M16-noise-foundation.md for the full argument and
// docs/milestones/M17-plains-terrain.md's note that the same hazard applies
// to any floating-point accumulation in this package.
//
// SurfaceHeight's addition of the peaks-and-valleys contribution to the base
// height is the only place in this package that combines a product with a
// sum; it is routed through here for that reason. Everything else is either
// a pure product chain -- no fusion risk regardless of grouping, the same
// reasoning internal/noise/opensimplex2.go gives for t*t*t*t -- or delegated
// to noise.Seed.FBm2D and Spline.Eval, which already carry this discipline
// internally.
func addProduct(a, b, c float64) float64 {
	return math.FMA(a, b, c)
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
	fx, fz := float64(x), float64(z)

	c := g.continentalness.FBm2D(fx*continentalFrequency, fz*continentalFrequency, fieldConfig)
	baseHeight := g.continentalSpline.Eval(c)

	e := g.erosion.FBm2D(fx*erosionFrequency, fz*erosionFrequency, fieldConfig)
	erosionFactor := g.erosionSpline.Eval(e)

	pv := g.peaksValleys.FBm2D(fx*peaksFrequency, fz*peaksFrequency, fieldConfig)
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
