package noise

import "math"

// This file adds a 3D field, Eval3D, alongside Eval2D's simplex noise --
// needed by M20 (docs/milestones/M20-density-terrain.md) for the density
// field's genuinely-3D `detail` term. It is built the same way Eval2D is: our
// own simplex-lattice implementation, tuned to satisfy this package's own
// property tests rather than to reproduce any reference bit-for-bit (see
// opensimplex2.go's doc comment on that point -- it applies here unchanged).
//
// The lattice geometry generalizes directly: a 3D simplex cell splits into 6
// tetrahedra instead of 2D's 2 triangles, contributing 4 corners instead of 3.
// Skew/unskew, per-corner falloff and gradient hashing all follow the same
// shape as opensimplex2.go; see that file for the reasoning this one leans on
// without repeating.

// f3 and g3 are the 3D simplex skew/unskew constants: f3 = 1/3, g3 = 1/6.
// Both are exact rational compile-time constants -- no runtime floating-point
// operation, so no FMA risk.
const (
	f3   = 1.0 / 3.0
	g3   = 1.0 / 6.0
	g3x2 = 2 * g3
	g3x3 = 3 * g3
)

// gradientCount3D is the size of gradients3D. A power of two, like
// gradientCount, so masking substitutes for a modulo when turning a
// permutation-table byte into a gradient index.
const gradientCount3D = 16

// gradients3D is the classic 3D "improved noise" gradient set (Ken Perlin,
// 2002): the 12 edge-midpoint directions of a cube, each with exactly two
// nonzero components in {-1, 1}, with 4 of them repeated to round the table
// out to a power of two for cheap masking. Every entry has the same magnitude
// (sqrt(2)), which is what lets gradientMaxContribution3D below use one
// closed-form bound for all of them.
//
// Written as literal integer components rather than computed unit vectors:
// exact values, no division, no possible cross-platform rounding
// disagreement -- the same reasoning gradients2D documents for its own
// literals.
var gradients3D = [gradientCount3D][3]float64{
	{1, 1, 0}, {-1, 1, 0}, {1, -1, 0}, {-1, -1, 0},
	{1, 0, 1}, {-1, 0, 1}, {1, 0, -1}, {-1, 0, -1},
	{0, 1, 1}, {0, -1, 1}, {0, 1, -1}, {0, -1, -1},
	{1, 1, 0}, {-1, 1, 0}, {0, -1, 1}, {0, -1, -1},
}

// gradientMaxContribution3D bounds a single 3D corner's contribution the same
// way gradientMaxContribution bounds a 2D one: t = 0.5 - |d|^2, contribution
// <= t^4 * |g| * |d| by Cauchy-Schwarz, maximised at the same r* = sqrt(1/18)
// (the extra factor |g| does not move the argmax, only scales the peak). Every
// gradients3D entry has |g| = sqrt(2), so that factor is exactly
// math.Sqrt2 * gradientMaxContribution.
//
// It is the proved single-corner bound. It is NOT what the scale is built on
// -- see rawSumSupremum3D -- but it cross-checks the numerical search that is.
const gradientMaxContribution3D = math.Sqrt2 * gradientMaxContribution

// rawSumSupremum3D is the least upper bound on the magnitude of the raw
// four-corner sum, found numerically exactly as opensimplex2.go's
// rawSumSupremum is: search sample positions, give every corner its best
// gradient independently (the hash picks them independently, so this bounds
// every seed), take the maximum. Rounded up.
//
// This replaced the triangle-inequality bound, 4 * gradientMaxContribution3D,
// which M20 first shipped with. That bound is safe but wastes a factor of
// exactly four: at the maximum only one corner is anywhere near its own peak,
// so counting all four at their peak overstates the sum fourfold, and Eval3D
// only ever reached 0.2475 of its documented [-1, 1].
//
// The implementation accepted that on the grounds that "nothing downstream
// needs Eval3D's own normalisation to be tight". Both halves of that were
// wrong. worldgen compensated with a detail amplitude four times too large,
// and then derived its SurfaceHeight scan band from that amplitude times the
// *documented* range: a band four times wider than any detail could reach,
// which became the dominant cost of chunk generation. And M21 will threshold
// this noise to carve caves, where "carve where noise exceeds 0.6" would never
// fire on a field that tops out at 0.25. This is the second time the loose
// bound has shipped in this package; see M16 and constraint 4 of
// docs/design/worldgen-constraints.md.
//
// TestRawSumSupremum3DIsSound re-runs the search so this cannot drift from the
// geometry, and TestNumericalSupremum3DRespectsTheProvedBound checks it
// against the loose bound, which a sound search can never exceed.
const rawSumSupremum3D = 0.01300716

// rawSumBound3DProved is the triangle-inequality bound, kept only as the
// cross-check described on rawSumSupremum3D.
const rawSumBound3DProved = 4 * gradientMaxContribution3D

// noiseScale3D brings the raw four-corner sum into [-1, 1], with the same
// rangeSafetyMargin headroom opensimplex2.go's noiseScale uses.
const noiseScale3D = 1.0 / (rawSumSupremum3D * rangeSafetyMargin)

// Eval3D returns this seed's noise field sampled at (x, y, z). The result is
// always in [-1, 1] (see noiseScale3D) and is a deterministic, pure function
// of (s, x, y, z): same seed and coordinate always produce the same value,
// independent of evaluation order and identical on every architecture --
// TestEval3DDeterministic and this package's arm64 codegen guard
// (fma_codegen_test.go, which builds this whole package) check exactly that.
func (s Seed) Eval3D(x, y, z float64) float64 {
	// Skew into the stretched grid whose corners line up with the simplex
	// lattice, then unskew the chosen lattice corner back -- same shape as
	// Eval2D, generalized to three axes.
	skew := float64((x + y + z) * f3)
	xs := x + skew
	ys := y + skew
	zs := z + skew
	i := math.Floor(xs)
	j := math.Floor(ys)
	k := math.Floor(zs)

	unskew := float64((i + j + k) * g3)
	x0 := x - (i - unskew)
	y0 := y - (j - unskew)
	z0 := z - (k - unskew)

	// A skewed unit cube splits into six simplex tetrahedra; which one
	// (x0, y0, z0) falls in decides the two middle corners. This is the
	// standard ordering test (Gustavson, "Simplex noise demystified"):
	// ranking x0, y0, z0 against each other picks which of the six axis
	// orderings the point falls into, and the middle corners walk the
	// lattice one step at a time along that ordering.
	var i1, j1, k1, i2, j2, k2 float64
	switch {
	case x0 >= y0 && y0 >= z0: // X, Y, Z
		i1, j1, k1 = 1, 0, 0
		i2, j2, k2 = 1, 1, 0
	case x0 >= y0 && x0 >= z0: // X, Z, Y
		i1, j1, k1 = 1, 0, 0
		i2, j2, k2 = 1, 0, 1
	case x0 >= y0: // Z, X, Y
		i1, j1, k1 = 0, 0, 1
		i2, j2, k2 = 1, 0, 1
	case y0 < z0: // Z, Y, X
		i1, j1, k1 = 0, 0, 1
		i2, j2, k2 = 0, 1, 1
	case x0 < z0: // Y, Z, X
		i1, j1, k1 = 0, 1, 0
		i2, j2, k2 = 0, 1, 1
	default: // Y, X, Z
		i1, j1, k1 = 0, 1, 0
		i2, j2, k2 = 1, 1, 0
	}

	// No multiplication appears in any of these nine expressions (i1, i2 etc
	// are 0 or 1 added directly; g3, g3x2, g3x3 are compile-time constants),
	// so none carries any FMA risk regardless of grouping -- same reasoning
	// as Eval2D's x1/y1/x2/y2.
	x1 := x0 - i1 + g3
	y1 := y0 - j1 + g3
	z1 := z0 - k1 + g3
	x2 := x0 - i2 + g3x2
	y2 := y0 - j2 + g3x2
	z2 := z0 - k2 + g3x2
	x3 := x0 - 1 + g3x3
	y3 := y0 - 1 + g3x3
	z3 := z0 - 1 + g3x3

	ii := int32(i)
	jj := int32(j)
	kk := int32(k)

	n0 := s.corner3(ii, jj, kk, x0, y0, z0)
	n1 := s.corner3(ii+int32(i1), jj+int32(j1), kk+int32(k1), x1, y1, z1)
	n2 := s.corner3(ii+int32(i2), jj+int32(j2), kk+int32(k2), x2, y2, z2)
	n3 := s.corner3(ii+1, jj+1, kk+1, x3, y3, z3)

	// n0..n3 are each already a forced-rounding result (see corner3), so
	// their sum carries no live multiply for anything to fuse with; only the
	// final scale multiply crosses this function's return boundary, and it
	// is a lone trailing multiply with nothing after it to fuse into.
	return float64((n0 + n1 + n2 + n3) * noiseScale3D)
}

// corner3 returns one simplex corner's contribution to Eval3D's result, for a
// sample offset (dx, dy, dz) from that corner. (ix, iy, iz) is the corner's
// lattice coordinate, used only to pick its gradient.
func (s Seed) corner3(ix, iy, iz int32, dx, dy, dz float64) float64 {
	// t is an accumulation (0.5 minus three squared terms), so each squared
	// term's rounding is forced before it is subtracted -- see the FMA design
	// decision in fma.go.
	t := 0.5
	t -= float64(dx * dx)
	t -= float64(dy * dy)
	t -= float64(dz * dz)
	if t <= 0 {
		// Outside this corner's kernel radius: no contribution, and the
		// dropped corner already contributes exactly zero at the boundary --
		// same continuity argument as Eval2D's corner.
		return 0
	}

	g := gradients3D[s.gradientIndex3(ix, iy, iz)]

	// t*t*t*t is a pure product chain with no addition in it anywhere, so it
	// carries no fusion risk on its own regardless of grouping.
	t2 := t * t
	t4 := t2 * t2

	d := dot3(g[0], g[1], g[2], dx, dy, dz)

	// This product feeds Eval3D's corner sum, so its rounding is forced here
	// rather than left for the caller.
	return float64(t4 * d)
}

// gradientIndex3 hashes a lattice coordinate to an index into gradients3D, by
// triple-hashing through perm exactly the way gradientIndex double-hashes for
// 2D: look up ix, fold in iy, look up again, fold in iz, look up a third time.
// This is the same technique for avoiding directional correlation between the
// three hash streams, extended by one more fold.
func (s Seed) gradientIndex3(ix, iy, iz int32) uint8 {
	a := s.perm[uint8(ix)]
	b := s.perm[uint8(int32(a)+iy)]
	c := s.perm[uint8(int32(b)+iz)]
	return c & (gradientCount3D - 1)
}
