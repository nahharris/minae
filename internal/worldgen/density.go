package worldgen

import (
	"math"

	"github.com/nahharris/minae/internal/platform/config"
	"github.com/nahharris/minae/internal/world"
)

// This file is M20's density field (docs/milestones/M20-density-terrain.md):
//
//	density(x, y, z) = (surfaceHeight(x, z) - y) + interpolate(detail)(x, y, z)
//
// solid where density > 0. The 2D base term, surfaceHeight(x,z) - y, is exact
// per column -- it is linear in y, so nothing about it needs interpolating --
// and only `detail`, the genuinely 3D term, is sampled on a coarse cell grid
// and trilinearly interpolated (see the milestone's "Only the 3D term is
// interpolated" refinement, which corrects the original spec's contradiction
// between exact-heightmap reproduction and interpolating the whole field).
//
// generator.go's rawSurfaceHeight/clampHeight untouched: they still compute
// the 2D base height exactly as M17/M19 did. This file adds the 3D term on
// top and redefines the public SurfaceHeight (below) to read the same
// interpolated field Generate fills blocks from, per the milestone's "one
// field, one path" refinement.

const (
	// cellWidth and cellHeight are the interpolation cell's horizontal and
	// vertical extents, in blocks -- the milestone's "cells 4 blocks wide and
	// 8 tall". config.ChunkWidth (16) and config.ChunkHeight (256) are both
	// exact multiples of these, so a chunk's corner grid lines up evenly with
	// no partial cell at its edges.
	cellWidth  = 4
	cellHeight = 8

	// detailFrequency is expressed the same way generator.go's terrain
	// frequencies are: a fraction of one block, so 1/detailFrequency is the
	// field's wavelength in blocks. Chosen tighter than peaksFrequency
	// (1/80) so relief varies within the span of a single chunk rather than
	// drifting uniformly across many -- an overhang needs the field to
	// change sign within a short horizontal run, not just tilt a whole
	// chunk's worth of terrain up or down.
	detailFrequency = 1.0 / 10.0

	// detailAmplitude is `detail`'s amplitude before erosion scaling: the most
	// any column's detail can move its surface, in blocks, before erosion scales
	// it down.
	//
	// Scaled by erosionFactor, whose range is 0.05 to 0.6, a plains column sees at
	// most 0.6 blocks of detail and the roughest terrain at most 7.5 -- which is
	// what "small and scaled by erosion" in the milestone asks for.
	//
	// This was 50 when M20 first landed, because noise.Eval3D then reached only
	// 0.2475 of its documented [-1, 1] and the amplitude was quadrupled to make up
	// for it. That cost far more than the constant's name: detailBand, and with it
	// SurfaceHeight's scan band and Generate's per-column density work, is derived
	// from this amplitude times the *documented* range, so it came out four times
	// wider than any real detail could reach -- 63 cells evaluated per column where
	// 19 suffice, and the dominant cost of generating a chunk. Once Eval3D was
	// given a tight scale (see noise.rawSumSupremum3D), 12.5 reproduces the same
	// terrain to within 0.008% and the band shrank to match.
	detailAmplitude = 12.5

	// maxErosionScale mirrors NewGenerator's erosionSpline: its highest
	// control point is Y=0.6 at X=-1, and the spline is monotonically
	// decreasing (0.6 -> 0.3 -> 0.05), so 0.6 is the largest value
	// erosionFactor can ever take -- and therefore the largest multiplier
	// `detail`'s amplitude is ever scaled by. If that spline is ever
	// retuned to a higher peak, this constant must move with it;
	// TestDetailBandBoundsRealDensity (density_test.go) cross-checks this
	// bound empirically against sampled columns rather than trusting it
	// blindly.
	maxErosionScale = 0.6

	// detailBand is a rigorous (if not tight) upper bound on |interpolated
	// detail| anywhere: detailAmplitude*maxErosionScale bounds the erosion-
	// scaled amplitude, Eval3D never exceeds magnitude 1 (its documented
	// range), and trilinear interpolation is a convex combination of eight
	// corner values, so it cannot exceed the bound any single corner does.
	// density can therefore only differ in sign from the 2D-only heightmap
	// answer within detailBand blocks of it -- see surfaceHeightBand, and
	// the milestone's "SurfaceHeight scans a bounded band" refinement.
	detailBand = detailAmplitude * maxErosionScale
)

// surfaceHeightBand is detailBand rounded up to an integer, with one extra
// block of margin: SurfaceHeight's scan and Generate's fast-path skip both
// use it as A, the milestone's own name for this bound. Outside
// [h-surfaceHeightBand, h+surfaceHeightBand], density's sign is exactly
// sign(h-y): the base term alone dominates any possible detail contribution,
// so evaluating the interpolated term there is wasted work, not a source of
// disagreement -- both solidAt and cornerCache.solidAt take that fast path
// for the exact same reason.
var surfaceHeightBand = int(math.Ceil(detailBand)) + 1

// enable3DDetail is the single switch the milestone's criterion 1 asks for:
// with it false, `detail` is zero everywhere and density collapses to
// exactly the pre-M20, heightmap-only generator -- see detailCornerValue,
// the only place that reads it. TestGenerateReproducesHeightmapExactly (in
// density_test.go) flips it off for the duration of one test via
// withDetailDisabled, then restores it; production code never touches it.
var enable3DDetail = true

// erosionFactorAt returns the same erosionFactor rawHeightFromFields computes
// for column (x, z): the erosion spline evaluated at this column's own raw
// erosion noise sample. `detail`'s amplitude is scaled by this same value
// (see detailCornerValue) precisely so relief tracks the same "roughness"
// signal that already scales peaksValue -- a column erosionFactor already
// flattens is a column `detail` flattens too, and vice versa.
func (g *Generator) erosionFactorAt(x, z int) float64 {
	fx, fz := float64(x), float64(z)
	e := g.erosion.FBm2D(fx*erosionFrequency, fz*erosionFrequency, fieldConfig)
	return g.erosionSpline.Eval(e)
}

// detailCornerValue is `detail`'s raw sample at lattice corner (x, y, z):
// zero if enable3DDetail is false (criterion 1's switch), otherwise a single
// octave of 3D noise scaled by this column's own erosion factor and
// detailAmplitude.
//
// x, y and z MUST be global coordinates -- the same requirement every other
// sampler in this package documents, and for the same reason: corner values
// are shared exactly between adjacent chunks (criterion 2) only if both
// compute them from the same global lattice.
//
// erosionFactorAt(x,z)*detailAmplitude and that product's further multiply by
// the noise sample are each a lone product feeding no addition in this
// expression -- no fusion risk regardless of grouping, the same reasoning
// rawHeightFromFields gives for scaledPeak.
func (g *Generator) detailCornerValue(x, y, z int) float64 {
	return g.detailFromScale(g.detailScale(x, z), x, y, z)
}

// detailScale is the part of a corner's value that depends only on its
// column: the erosion-driven amplitude. detailFromScale is the part that
// depends on the full position. detailCornerValue is exactly the two composed,
// so a caller that computes the scale once per column and reuses it gets
// bit-identical corners.
//
// The split exists because the scale is a four-octave fBm plus a spline, and
// it was being recomputed for every corner of every cell. A chunk has 825
// corners but only 25 distinct corner columns, and a SurfaceHeight scan could
// ask for the same column's scale several hundred times. Those duplicates were
// most of what made M20's generation and the worldgen test suite slow.
func (g *Generator) detailScale(x, z int) float64 {
	if !enable3DDetail {
		return 0
	}
	return g.erosionFactorAt(x, z) * detailAmplitude
}

// detailFromScale returns the detail sample at (x, y, z) given that column's
// detailScale. See detailScale.
func (g *Generator) detailFromScale(scale float64, x, y, z int) float64 {
	if !enable3DDetail {
		return 0
	}
	n := g.detail.Eval3D(float64(x)*detailFrequency, float64(y)*detailFrequency, float64(z)*detailFrequency)
	return scale * n
}

// lerp is a + t*(b-a), routed through addProduct so every interpolation step
// fuses identically on every architecture -- the milestone's "Interpolation
// is FMA-shaped" refinement, made literal. TestNoUnintendedFusedOperations
// (fma_codegen_test.go) fails on a bare `a + t*(b-a)` here.
func lerp(a, b, t float64) float64 {
	return addProduct(t, b-a, a)
}

// trilerp trilinearly interpolates the eight corners of one cell. Corner
// naming is (x,y,z) bit order: c101 is the corner at (x1, y0, z1), etc.
// Both the cached path (cornerCache.detailAt) and the uncached path
// (Generator.interpolatedDetailAt) call this same function on the same eight
// input values for the same (tx, ty, tz) -- which is what makes them produce
// bit-identical results (criterion 7: "SurfaceHeight reads the SAME
// interpolated field Generate fills from") rather than merely similar ones.
func trilerp(c000, c100, c010, c110, c001, c101, c011, c111, tx, ty, tz float64) float64 {
	c00 := lerp(c000, c100, tx)
	c10 := lerp(c010, c110, tx)
	c01 := lerp(c001, c101, tx)
	c11 := lerp(c011, c111, tx)
	c0 := lerp(c00, c10, ty)
	c1 := lerp(c01, c11, ty)
	return lerp(c0, c1, tz)
}

// floorDivMod splits v into a cell index and an in-cell offset, size blocks
// per cell, correct for negative v -- the same floor-division requirement
// world.ChunkAndLocal documents, needed here because global coordinates west
// or south of the origin are negative and Go's native / and % truncate
// toward zero rather than flooring.
func floorDivMod(v, size int) (idx, rem int) {
	idx = v / size
	rem = v % size
	if rem < 0 {
		idx--
		rem += size
	}
	return
}

// interpolatedDetailAt is `interpolate(detail)(x, y, z)` computed directly
// from eight fresh corner samples, with no chunk cache -- the path
// SurfaceHeight uses, since a column can be asked about (spawn placement, a
// neighbouring chunk's feature root) with no chunk ever generated.
//
// x, y and z MUST be global coordinates. The cell (x, y, z) falls in is found
// by floor division, so two callers asking about the same global point --
// whether or not either one belongs to a chunk currently being generated --
// land on exactly the same cell and the same eight corners.
func (g *Generator) interpolatedDetailAt(x, y, z int) float64 {
	ax, tx := cellFrac(x, cellWidth)
	by, ty := cellFrac(y, cellHeight)
	az, tz := cellFrac(z, cellWidth)

	x0, x1 := ax*cellWidth, (ax+1)*cellWidth
	y0, y1 := by*cellHeight, (by+1)*cellHeight
	z0, z1 := az*cellWidth, (az+1)*cellWidth

	c000 := g.detailCornerValue(x0, y0, z0)
	c100 := g.detailCornerValue(x1, y0, z0)
	c010 := g.detailCornerValue(x0, y1, z0)
	c110 := g.detailCornerValue(x1, y1, z0)
	c001 := g.detailCornerValue(x0, y0, z1)
	c101 := g.detailCornerValue(x1, y0, z1)
	c011 := g.detailCornerValue(x0, y1, z1)
	c111 := g.detailCornerValue(x1, y1, z1)

	return trilerp(c000, c100, c010, c110, c001, c101, c011, c111, tx, ty, tz)
}

// cellFrac returns v's cell index and its fractional offset within that cell
// (in [0, 1)), size blocks per cell.
func cellFrac(v, size int) (idx int, frac float64) {
	i, r := floorDivMod(v, size)
	return i, float64(r) / float64(size)
}

// solidAt reports whether global cell (x, y, z) is solid, given h =
// SurfaceHeight's own 2D base height for column (x, z) (the same value
// rawSurfaceHeight/clampHeight already compute -- callers pass it in so a
// column's per-cell scan does not recompute the three fBm fields for every
// y). This is the uncached path; cornerCache.solidAt below is its cached
// twin and must agree with it exactly.
//
// The fast paths are not a heuristic: they are exactly the bound
// surfaceHeightBand's own comment proves. Outside it, density's sign is
// sign(h-y) unconditionally, so returning early does not need it to be
// evaluated at all.
func (g *Generator) solidAt(x, y, z, h int) bool {
	diff := h - y
	switch {
	case diff > surfaceHeightBand:
		return true
	case diff < -surfaceHeightBand:
		return false
	default:
		return float64(diff)+g.interpolatedDetailAt(x, y, z) > 0
	}
}

// cornersPerChunkXZ and cornersPerChunkY are the corner-grid dimensions a
// single chunk's cornerCache covers: config.ChunkWidth/cellWidth+1 (5) cell
// boundaries across each horizontal axis, config.ChunkHeight/cellHeight+1
// (33) up the vertical one -- "roughly 5x33x5 corner samples per chunk", per
// the milestone's own estimate.
const (
	cornersPerChunkXZ = config.ChunkWidth/cellWidth + 1
	cornersPerChunkY  = config.ChunkHeight/cellHeight + 1
)

// cornerCache is Generate's performance path: the corner `detail` values a
// chunk's blocks interpolate between, each computed at most once and only if
// some block actually reads it.
//
// Corners are filled lazily. solidAt answers every cell more than
// surfaceHeightBand from the surface without consulting detail at all, so most
// of a column's 33 corner levels are never read; computing them up front was
// the bulk of M20's generation cost. Each corner column's erosion scale is
// computed once at construction, since it is shared by every level of that
// column.
//
// It is a pure memoization of detailCornerValue, nothing more -- every value
// it stores is exactly what a fresh call to detailCornerValue at that same
// global corner would return, and detailAt below feeds those cached values
// through the exact same trilerp function interpolatedDetailAt uses. That
// equality is what makes the cache a performance optimisation rather than a
// second, independently-fallible implementation: Generate and SurfaceHeight
// cannot disagree about a column's shape, because both ultimately evaluate
// the same corners through the same interpolation (criterion 7).
type cornerCache struct {
	g      *Generator
	x0, z0 int

	// scale holds detailScale for each of the chunk's corner columns, indexed
	// a*cornersPerChunkXZ + c.
	scale [cornersPerChunkXZ * cornersPerChunkXZ]float64

	// detail holds each corner once computed; known says which have been.
	// Read corners through corner(), never through detail directly.
	detail []float64
	known  []bool
}

// newCornerCache builds coord's corner cache: one detailCornerValue sample
// per lattice corner overlapping coord, at global coordinates so a shared
// chunk edge's corners are computed identically by both neighbours
// (criterion 2). Only the values are kept -- detailAt below indexes back
// into them using LOCAL coordinates, since the cache's grid layout is
// already anchored to this one chunk's origin by construction.
func newCornerCache(g *Generator, coord world.ChunkCoord) *cornerCache {
	n := cornersPerChunkXZ * cornersPerChunkXZ * cornersPerChunkY
	cc := &cornerCache{
		g:      g,
		x0:     coord.X * config.ChunkWidth,
		z0:     coord.Z * config.ChunkWidth,
		detail: make([]float64, n),
		known:  make([]bool, n),
	}
	for a := 0; a < cornersPerChunkXZ; a++ {
		for c := 0; c < cornersPerChunkXZ; c++ {
			cc.scale[a*cornersPerChunkXZ+c] = g.detailScale(cc.x0+a*cellWidth, cc.z0+c*cellWidth)
		}
	}
	return cc
}

// corner returns the detail value at corner (a, b, c), computing it on first
// use. The value is detailFromScale on this column's cached scale, which is
// exactly detailCornerValue at the same global corner.
func (cc *cornerCache) corner(a, b, c int) float64 {
	i := cc.index(a, b, c)
	if !cc.known[i] {
		cc.detail[i] = cc.g.detailFromScale(cc.scale[a*cornersPerChunkXZ+c],
			cc.x0+a*cellWidth, b*cellHeight, cc.z0+c*cellWidth)
		cc.known[i] = true
	}
	return cc.detail[i]
}

// index maps a corner's (a, b, c) grid position -- a and c across
// cornersPerChunkXZ, b across cornersPerChunkY -- to its slot in detail.
func (cc *cornerCache) index(a, b, c int) int {
	return (a*cornersPerChunkXZ+c)*cornersPerChunkY + b
}

// detailAt interpolates `detail` at LOCAL column (lx, lz) and global height
// y, using this chunk's cached corners. lx and lz MUST be in
// [0, config.ChunkWidth), i.e. local to the chunk this cache was built for;
// y is global (there is only one vertical corner grid, shared by every
// chunk regardless of its X/Z).
func (cc *cornerCache) detailAt(lx, y, lz int) float64 {
	ax, tx := lx/cellWidth, float64(lx%cellWidth)/float64(cellWidth)
	az, tz := lz/cellWidth, float64(lz%cellWidth)/float64(cellWidth)
	by, remY := floorDivMod(y, cellHeight)
	ty := float64(remY) / float64(cellHeight)

	c000 := cc.corner(ax, by, az)
	c100 := cc.corner(ax+1, by, az)
	c010 := cc.corner(ax, by+1, az)
	c110 := cc.corner(ax+1, by+1, az)
	c001 := cc.corner(ax, by, az+1)
	c101 := cc.corner(ax+1, by, az+1)
	c011 := cc.corner(ax, by+1, az+1)
	c111 := cc.corner(ax+1, by+1, az+1)

	return trilerp(c000, c100, c010, c110, c001, c101, c011, c111, tx, ty, tz)
}

// solidAt is cache's twin of Generator.solidAt: identical fast-path bound,
// identical final comparison, differing only in getting `detail` from the
// cache instead of eight fresh samples. lx, lz are local; y and h are global.
func (cc *cornerCache) solidAt(lx, y, lz, h int) bool {
	diff := h - y
	switch {
	case diff > surfaceHeightBand:
		return true
	case diff < -surfaceHeightBand:
		return false
	default:
		return float64(diff)+cc.detailAt(lx, y, lz) > 0
	}
}

// columnDetail evaluates the interpolated detail field down one column, for
// SurfaceHeight's band scan.
//
// A column's cells all share the same four corner columns, and consecutive
// cells share a whole layer of corners, so evaluating each cell independently
// (as solidAt does) recomputes the same corners over and over: a 63-cell band
// scan cost roughly 500 corner evaluations, each with its own four-octave
// erosion fBm, for what is at most about forty distinct corners and four
// distinct scales. Feature placement calls SurfaceHeight for every candidate
// root, so that redundancy landed straight in chunk generation.
//
// Every corner here is detailFromScale on the corner column's own
// detailScale, fed through the same trilerp in the same argument order as
// interpolatedDetailAt and cornerCache.detailAt, so the three agree bit for
// bit. TestSolidAtAgreesCachedAndUncached and the layering tests hold that.
type columnDetail struct {
	g          *Generator
	x0, x1     int
	z0, z1     int
	tx, tz     float64
	scale      [4]float64 // corner columns (x0,z0), (x1,z0), (x0,z1), (x1,z1)
	firstLevel int
	levels     [][4]float64
	known      []bool
}

// newColumnDetail prepares a scan of column (x, z) over y in [lo, hi].
func (g *Generator) newColumnDetail(x, z, lo, hi int) *columnDetail {
	ax, tx := cellFrac(x, cellWidth)
	az, tz := cellFrac(z, cellWidth)
	loLevel, _ := floorDivMod(lo, cellHeight)
	hiLevel, _ := floorDivMod(hi, cellHeight)
	n := hiLevel - loLevel + 2 // a cell reads its own level and the one above

	c := &columnDetail{
		g:  g,
		x0: ax * cellWidth, x1: (ax + 1) * cellWidth,
		z0: az * cellWidth, z1: (az + 1) * cellWidth,
		tx: tx, tz: tz,
		firstLevel: loLevel,
		levels:     make([][4]float64, n),
		known:      make([]bool, n),
	}
	c.scale = [4]float64{
		g.detailScale(c.x0, c.z0),
		g.detailScale(c.x1, c.z0),
		g.detailScale(c.x0, c.z1),
		g.detailScale(c.x1, c.z1),
	}
	return c
}

// level returns the four corner values of corner level b, computing them on
// first use.
func (c *columnDetail) level(b int) [4]float64 {
	i := b - c.firstLevel
	if !c.known[i] {
		y := b * cellHeight
		c.levels[i] = [4]float64{
			c.g.detailFromScale(c.scale[0], c.x0, y, c.z0),
			c.g.detailFromScale(c.scale[1], c.x1, y, c.z0),
			c.g.detailFromScale(c.scale[2], c.x0, y, c.z1),
			c.g.detailFromScale(c.scale[3], c.x1, y, c.z1),
		}
		c.known[i] = true
	}
	return c.levels[i]
}

// solidAt is Generator.solidAt for this column, reusing corners across cells.
func (c *columnDetail) solidAt(y, h int) bool {
	diff := h - y
	switch {
	case diff > surfaceHeightBand:
		return true
	case diff < -surfaceHeightBand:
		return false
	}

	by, ty := cellFrac(y, cellHeight)
	lower, upper := c.level(by), c.level(by+1)

	// Same argument order as interpolatedDetailAt: c000, c100, c010, c110,
	// c001, c101, c011, c111, where the digits are (x, y, z) offsets.
	d := trilerp(
		lower[0], lower[1], upper[0], upper[1],
		lower[2], lower[3], upper[2], upper[3],
		c.tx, ty, c.tz,
	)
	return float64(diff)+d > 0
}
