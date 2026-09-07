package noise

import (
	"math"
	"math/rand"
	"testing"
)

// Criterion 5 is the entire reason this milestone chose OpenSimplex2 over
// Perlin: classic Perlin's square grid produces faint but real north-south
// and east-west structure in terrain (see "The Perlin Problem", linked from
// the milestone doc). axisDiagonalBias is the statistic that is supposed to
// detect that structure, and TestNoAxisAlignedBias is the test that actually
// relies on it.
//
// A statistic that always passes is worthless -- see "The axis-alignment
// test needs a negative control" in the milestone doc -- so this file also
// implements a deliberately axis-aligned square-grid value noise, in test
// code only, and TestAxisDiagonalBiasStatisticCanReject proves the same
// statistic rejects it. Without that control, a passing
// TestNoAxisAlignedBias would be just as consistent with "the statistic
// cannot detect this defect at all" as with "this field does not have the
// defect".

// axisDiagonalBias samples every field in fields at anchorsPerField random
// points each (all fields sharing the same anchor coordinates, drawn from one
// RNG stream so the two statistics below are computed from directly
// comparable samples) and, from each anchor, takes a finite difference to a
// second point a fixed distance lag away in four directions: along +x, along
// +y, and along the two diagonals (+x+y and +x-y). It returns the relative
// difference between the variance of the axis-direction differences, pooled
// across every field and every anchor, and the variance of the
// diagonal-direction differences.
//
// Two things about this construction are load-bearing:
//
//   - The four probe points are all exactly lag away from their anchor,
//     Euclidean distance. An earlier version of this statistic instead
//     compared a grid's edge differences (distance = step) against its
//     corner-to-corner diagonal differences (distance = step*sqrt(2)), and
//     any smooth field -- biased or not -- varies more over a longer
//     distance, which showed up as a bias that had nothing to do with axis
//     alignment. Holding the distance fixed and only rotating the probe
//     direction isolates the thing criterion 5 actually cares about.
//   - Differences are pooled across many independently seeded fields rather
//     than measured from one. A single seed's bias value is noisy enough
//     that it is not unusual for one OpenSimplex2 seed to score higher than
//     one square-grid seed picked at random -- see the milestone's git
//     history for the exploration that found this the hard way. Pooling
//     tens of seeds' worth of differences before computing variance is what
//     turns a coin-flip-noisy statistic into one that reliably separates the
//     two constructions by 5x or more.
func axisDiagonalBias(fields []func(x, y float64) float64, anchorsPerField int, lag float64) float64 {
	rng := rand.New(rand.NewSource(0xA915))

	diag := lag / math.Sqrt2

	var axisDiffs, diagDiffs []float64
	for _, field := range fields {
		for i := 0; i < anchorsPerField; i++ {
			x := rng.Float64()*2000 - 1000
			y := rng.Float64()*2000 - 1000
			base := field(x, y)

			axisDiffs = append(axisDiffs,
				field(x+lag, y)-base,
				field(x, y+lag)-base,
			)
			diagDiffs = append(diagDiffs,
				field(x+diag, y+diag)-base,
				field(x+diag, y-diag)-base,
			)
		}
	}

	axisVar := variance(axisDiffs)
	diagVar := variance(diagDiffs)

	mean := (axisVar + diagVar) / 2
	if mean == 0 {
		return 0
	}
	return absf(axisVar-diagVar) / mean
}

func variance(values []float64) float64 {
	var sum, sumSq float64
	for _, v := range values {
		sum += v
		sumSq += v * v
	}
	n := float64(len(values))
	mean := sum / n
	return sumSq/n - mean*mean
}

// biasTestSeeds is shared by TestNoAxisAlignedBias and
// TestAxisDiagonalBiasStatisticCanReject so both draw from the same pool of
// seeds -- an arbitrary arithmetic sequence, not hand-picked to flatter
// either result.
func biasTestSeeds() []int64 {
	seeds := make([]int64, 48)
	for i := range seeds {
		seeds[i] = int64(i+1)*97 - 13
	}
	return seeds
}

const (
	biasLag            = 1.0 // matches the base lattice period, where the square grid's own structure is most exposed
	biasAnchorsPerSeed = 8000
)

// Criterion 5. See axisDiagonalBias's doc comment for what the statistic
// measures, and TestAxisDiagonalBiasStatisticCanReject below for the proof
// that it is capable of failing.
func TestNoAxisAlignedBias(t *testing.T) {
	var fields []func(x, y float64) float64
	for _, seed := range biasTestSeeds() {
		fields = append(fields, NewSeed(seed).Eval2D)
	}

	bias := axisDiagonalBias(fields, biasAnchorsPerSeed, biasLag)

	// Calibrated against TestAxisDiagonalBiasStatisticCanReject below, where
	// the square-grid control's bias is at least 5x larger than this.
	const threshold = 0.01
	if bias > threshold {
		t.Fatalf("axis/diagonal variance bias = %.4f, want <= %.3f; OpenSimplex2 should show no axis-aligned structure (criterion 5)", bias, threshold)
	}
	t.Logf("OpenSimplex2 pooled axis/diagonal bias: %.4f (threshold %.3f)", bias, threshold)
}

// TestAxisDiagonalBiasStatisticCanReject is the negative control the
// milestone doc requires: it proves axisDiagonalBias is actually capable of
// failing, by running it against a noise implementation known to have the
// defect criterion 5 exists to catch.
//
// squareGridValueNoise below is deliberately the kind of noise OpenSimplex2
// was chosen to avoid -- bilinear interpolation of hashed values on an
// axis-aligned unit grid, the textbook "value noise" construction and the
// direct square-grid cousin of classic Perlin. If this test does not show a
// clearly larger bias than TestNoAxisAlignedBias's OpenSimplex2 field, the
// statistic cannot actually tell the two apart, and TestNoAxisAlignedBias
// passing would mean nothing.
func TestAxisDiagonalBiasStatisticCanReject(t *testing.T) {
	seeds := biasTestSeeds()

	var osFields, controlFields []func(x, y float64) float64
	for _, seed := range seeds {
		osFields = append(osFields, NewSeed(seed).Eval2D)
		controlFields = append(controlFields, squareGridValueNoise(seed))
	}

	osBias := axisDiagonalBias(osFields, biasAnchorsPerSeed, biasLag)
	controlBias := axisDiagonalBias(controlFields, biasAnchorsPerSeed, biasLag)

	// This must be well above TestNoAxisAlignedBias's threshold of 0.01.
	const mustExceed = 0.01
	if controlBias <= mustExceed {
		t.Fatalf("square-grid value noise control scored %.4f, want > %.3f; the axis/diagonal statistic cannot detect the defect it exists to catch, so TestNoAxisAlignedBias passing proves nothing",
			controlBias, mustExceed)
	}

	// A hard multiple, not just "greater than": this is the actual proof
	// that the statistic separates the two constructions by a wide margin,
	// rather than merely landing on the right side of a single threshold by
	// chance.
	const minSeparation = 5
	if controlBias <= osBias*minSeparation {
		t.Fatalf("square-grid value noise control (%.4f) is not at least %dx more axis-biased than OpenSimplex2 (%.4f); the statistic needs a wider separation to be trustworthy",
			controlBias, minSeparation, osBias)
	}

	t.Logf("square-grid value noise (control) pooled bias: %.4f; OpenSimplex2 pooled bias: %.4f (%.1fx separation)",
		controlBias, osBias, controlBias/osBias)
}

// squareGridValueNoise returns a deliberately axis-aligned 2D value-noise
// field: a hashed random value at every integer lattice point, bilinearly
// interpolated with a smoothstep fade curve. This is test code only -- see
// this file's package comment -- built solely to give
// TestAxisDiagonalBiasStatisticCanReject a known-bad field to reject. It must
// never be promoted to production code: reproducing the exact defect
// criterion 5 exists to catch is the entire point of it.
func squareGridValueNoise(seed int64) func(x, y float64) float64 {
	s := NewSeed(seed)

	latticeValue := func(ix, iy int32) float64 {
		// Reuses Seed's own hash for convenience; the point under test is
		// the square lattice and bilinear interpolation below, not this
		// hash, so borrowing a hash this package already has is fine for a
		// test fixture.
		h := s.gradientIndex(ix, iy)
		return float64(h)/float64(gradientCount-1)*2 - 1
	}

	smoothstep := func(t float64) float64 {
		return t * t * (3 - 2*t)
	}

	return func(x, y float64) float64 {
		x0 := math.Floor(x)
		y0 := math.Floor(y)
		tx := smoothstep(x - x0)
		ty := smoothstep(y - y0)

		ix0, iy0 := int32(x0), int32(y0)
		v00 := latticeValue(ix0, iy0)
		v10 := latticeValue(ix0+1, iy0)
		v01 := latticeValue(ix0, iy0+1)
		v11 := latticeValue(ix0+1, iy0+1)

		top := v00 + tx*(v10-v00)
		bottom := v01 + tx*(v11-v01)
		return top + ty*(bottom-top)
	}
}
