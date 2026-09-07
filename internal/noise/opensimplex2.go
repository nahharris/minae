package noise

import "math"

// f2 skews (x, y) into the stretched grid whose unit cells line up with the
// simplex lattice's rhombi; g2 is its inverse (unskew), used to map a lattice
// corner back into the original coordinate system. These are the standard 2D
// simplex-lattice constants: f2 = (sqrt(3)-1)/2, g2 = (3-sqrt(3))/6. Both are
// compile-time constants, computed once by the Go compiler at arbitrary
// precision and rounded exactly once — they carry no runtime floating-point
// operation and so no FMA risk at all.
const (
	f2   = 0.36602540378443864676372317075294
	g2   = 0.21132486540518711774542560974902
	g2x2 = 2 * g2
)

// noiseScale brings the raw three-corner sum into the documented [-1, 1]
// range, and brings it close enough to the edges that the range means
// something.
//
// It is 1 divided by rawSumSupremum, plus a safety margin. The obvious
// alternative — three times gradientMaxContribution, by the triangle
// inequality — is safe but wastes a factor of 2.74, because the three corners
// cannot all sit at their individual optimum radius at once. A field scaled
// that way only ever reaches ±0.365.
//
// That is not a cosmetic complaint. Splines are how a designer says "this
// range of continentalness is coastline", and a curve authored across [-1, 1]
// that is only ever fed a third of its domain silently wastes two thirds of
// the control the interface exists to give. Every downstream consumer would
// end up applying its own correction factor, and they would not all pick the
// same one.
//
// See TestEval2DUsesMostOfItsRange for the property that would have caught
// the loose scale — criterion 3 alone cannot, since it only asks that values
// stay inside the range, and a field pinned at zero satisfies that perfectly.
const noiseScale = 1.0 / (rawSumSupremum * rangeSafetyMargin)

// rawSumSupremum is the least upper bound on the magnitude of the raw
// three-corner sum, before scaling.
//
// Unlike gradientMaxContribution this is a numerical result, not a closed
// form, and that difference is worth being plain about. It comes from a
// search over sample positions within the lattice, taking at each position
// the sum of the three corners' best achievable contributions — best over the
// sixteen discrete gradients, chosen independently per corner because the
// hash picks them independently. That makes it an upper bound whether or not
// any single seed attains it.
//
// It is attained in practice: sampling 32 million points across 40 seeds
// reaches 0.99 of it, so the bound is tight rather than merely valid. The
// maximum sits at (sqrt(3)/2, sqrt(3)/2), the deep interior of a cell.
//
// TestRawSumSupremumIsSound re-runs the search, so this constant cannot drift
// away from the geometry it describes, and cross-checks it against the
// closed-form triangle-inequality bound — a numerical search that landed
// somewhere absurd would show up as violating a bound that was proved by
// hand.
const rawSumSupremum = 0.01008021

// rangeSafetyMargin keeps the scaled output just inside [-1, 1] rather than
// exactly at it.
//
// rawSumSupremum is found by a search on a grid, so the true supremum could
// sit slightly between sample points. One percent is far more headroom than
// a smooth function needs at the resolution the search refines to, and it
// costs a field that peaks at 0.99 instead of 1.00 — which no consumer can
// tell apart from the real thing.
//
// The alternative, clamping, was rejected: a clamp that never fires is dead
// code, and one that does fire puts a flat spot in the terrain at exactly the
// most dramatic peaks.
const rangeSafetyMargin = 1.01

// gradientMaxContribution is the supremum of |t^4 * dot(g, d)| over a single
// simplex corner, where d = (dx, dy) is the offset from that corner, t =
// 0.5 - |d|^2 is the corner's falloff radius, and g is any unit gradient.
//
// By Cauchy-Schwarz, |dot(g, d)| <= |d| for a unit gradient, so the
// contribution is bounded by f(r) = (0.5 - r^2)^4 * r for r = |d| in
// [0, sqrt(0.5)]. f'(r) = (0.5-r^2)^3 * (0.5 - 9*r^2), zero at r =
// sqrt(1/18), where f(r) = (4/9)^4 * sqrt(1/18) = 256*sqrt(2) / 39366. That
// closed form is what is written below; the derivative's other root
// (r = sqrt(0.5)) is where t hits zero and contributes nothing.
const gradientMaxContribution = 256.0 * math.Sqrt2 / 39366.0

// gradients2D is a fixed set of 16 unit vectors at evenly spaced angles
// (k * 22.5 degrees), written as literal constants rather than computed with
// math.Sin/math.Cos at init time.
//
// This is deliberate, not just a style choice: math.Sin and math.Cos have
// separate architecture-specific assembly implementations for at least
// amd64 and arm64, and nothing guarantees they agree in the last bit. Using
// them here would reopen exactly the cross-architecture divergence this
// package exists to close (see the package doc). Literal constants are
// parsed once by the Go compiler using the language's constant-arithmetic
// rules and produce the identical float64 bit pattern on every platform.
var gradients2D = [gradientCount][2]float64{
	{1, 0},
	{0.9238795325112867, 0.3826834323650898},
	{0.7071067811865476, 0.7071067811865476},
	{0.3826834323650898, 0.9238795325112867},
	{0, 1},
	{-0.3826834323650898, 0.9238795325112867},
	{-0.7071067811865476, 0.7071067811865476},
	{-0.9238795325112867, 0.3826834323650898},
	{-1, 0},
	{-0.9238795325112867, -0.3826834323650898},
	{-0.7071067811865476, -0.7071067811865476},
	{-0.3826834323650898, -0.9238795325112867},
	{0, -1},
	{0.3826834323650898, -0.9238795325112867},
	{0.7071067811865476, -0.7071067811865476},
	{0.9238795325112867, -0.3826834323650898},
}

// Eval2D returns this seed's noise field sampled at (x, y). The result is
// always in [-1, 1] (see noiseScale) and is a deterministic, pure function of
// (s, x, y): the same seed and coordinate always produce the same value,
// forever, independent of evaluation order (criterion 1) and identical on
// every architecture (criterion 9, enforced by CI's arm64 job).
//
// Unlike classic Perlin noise, the gradients here live on a simplex
// (triangular) lattice rather than a square one — see gradients2D and the
// skew/unskew constants above — which is what avoids the faint axis-aligned
// structure a square grid produces (see TestNoAxisAlignedBias and "The
// Perlin Problem" linked from the milestone doc). This is our own
// implementation, built to satisfy the property tests in this package rather
// than to reproduce any reference implementation bit-for-bit — see "Golden
// vectors are what forever actually means" in the milestone doc for why that
// is the honest bar here, not a lesser one.
func (s Seed) Eval2D(x, y float64) float64 {
	// Skew (x, y) into the stretched grid whose corners line up with the
	// simplex lattice. skew feeds the additions below, so its rounding is
	// forced explicitly before it is combined with anything else — see the
	// FMA design decision in the package doc.
	skew := float64((x + y) * f2)
	xs := x + skew
	ys := y + skew
	i := math.Floor(xs)
	j := math.Floor(ys)

	// Unskew the lattice corner (i, j) back into the original coordinate
	// system to find this simplex cell's origin, then (x, y)'s offset from
	// it. unskew feeds the subtractions below, so it is forced the same way.
	unskew := float64((i + j) * g2)
	x0 := x - (i - unskew)
	y0 := y - (j - unskew)

	// A skewed unit square splits into two simplex triangles; which one
	// (x0, y0) falls in decides the middle corner.
	var i1, j1 float64
	if x0 > y0 {
		i1, j1 = 1, 0
	} else {
		i1, j1 = 0, 1
	}

	// No multiplication appears in any of these four expressions (i1, j1 are
	// 0 or 1 added directly; g2 and g2x2 are compile-time constants), so none
	// of them carries any FMA risk regardless of how they are grouped.
	x1 := x0 - i1 + g2
	y1 := y0 - j1 + g2
	x2 := x0 - 1 + g2x2
	y2 := y0 - 1 + g2x2

	ii := int32(i)
	jj := int32(j)

	n0 := s.corner(ii, jj, x0, y0)
	n1 := s.corner(ii+int32(i1), jj+int32(j1), x1, y1)
	n2 := s.corner(ii+1, jj+1, x2, y2)

	// n0, n1 and n2 are each already a forced-rounding result (see corner),
	// so their sum carries no live multiply for anything to fuse with; only
	// the final scale multiply needs its own forcing, since it is what
	// crosses this function's return boundary.
	return float64((n0 + n1 + n2) * noiseScale)
}

// corner returns one simplex corner's contribution to Eval2D's result, for a
// sample offset (dx, dy) from that corner. (ix, iy) is the corner's lattice
// coordinate, used only to pick its gradient.
func (s Seed) corner(ix, iy int32, dx, dy float64) float64 {
	// t is an accumulation (0.5 minus two squared terms), so each squared
	// term's rounding is forced before it is subtracted — see the FMA
	// design decision.
	t := 0.5
	t -= float64(dx * dx)
	t -= float64(dy * dy)
	if t <= 0 {
		// Outside this corner's kernel radius: no contribution. This is also
		// what makes Eval2D continuous across the boundary where the set of
		// active corners changes — the corner being dropped already
		// contributes exactly zero there, not some non-zero value that then
		// vanishes discontinuously.
		return 0
	}

	g := gradients2D[s.gradientIndex(ix, iy)]

	// t*t*t*t is a pure product chain with no addition in it anywhere, so it
	// carries no fusion risk on its own regardless of grouping.
	t2 := t * t
	t4 := t2 * t2

	d := dot2(g[0], g[1], dx, dy)

	// This product feeds Eval2D's corner sum, so its rounding is forced here
	// rather than left for the caller.
	return float64(t4 * d)
}
