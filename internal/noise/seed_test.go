package noise

import (
	"math"
	"math/rand"
	"testing"
)

// samplePoint is a coordinate the seed-independence tests sample both fields
// at. It exists as a named type so it can be shared between the tests below
// and correlation without every caller needing an identical anonymous struct
// literal type.
type samplePoint struct{ x, y float64 }

// Criterion 2: different seeds produce uncorrelated fields, and nearby seeds
// specifically must not produce nearly-identical worlds -- the classic
// symptom of folding the seed into the permutation table by truncation or a
// low-quality shuffle. Testing only distant seeds (0 vs 1_000_000, say) would
// not catch that: a bad mix can easily separate distant inputs while still
// leaving int64(n) and int64(n+1) nearly indistinguishable.
func TestSeedsAreIndependentAdjacentSeeds(t *testing.T) {
	points := randomSamplePoints(rand.New(rand.NewSource(4)), 2000)

	for base := int64(0); base < 20; base++ {
		a := NewSeed(base)
		b := NewSeed(base + 1)

		if corr := correlation(a, b, points); corr > 0.2 {
			t.Errorf("seed %d and seed %d correlate at %.4f over %d sample points; adjacent seeds should produce statistically unrelated fields (see NewSeed's doc comment on why splitmix64 is used)",
				base, base+1, corr, len(points))
		}
	}
}

// TestSeedsAreIndependentDistantSeeds is the complement of the adjacent-seed
// test: it is not enough for a seeding scheme to merely separate seeds that
// are close together while leaving some other pair correlated.
func TestSeedsAreIndependentDistantSeeds(t *testing.T) {
	points := randomSamplePoints(rand.New(rand.NewSource(5)), 2000)

	pairs := [][2]int64{
		{0, 1_000_000},
		{1, -1},
		{42, 8675309},
		{-999999, 999999},
	}
	for _, pair := range pairs {
		a := NewSeed(pair[0])
		b := NewSeed(pair[1])
		if corr := correlation(a, b, points); corr > 0.2 {
			t.Errorf("seed %d and seed %d correlate at %.4f over %d sample points",
				pair[0], pair[1], corr, len(points))
		}
	}
}

func randomSamplePoints(rng *rand.Rand, n int) []samplePoint {
	points := make([]samplePoint, n)
	for i := range points {
		points[i] = samplePoint{x: rng.Float64()*2000 - 1000, y: rng.Float64()*2000 - 1000}
	}
	return points
}

// correlation returns the Pearson correlation coefficient between two seeds'
// fields sampled at the same points, as a measure of how related the two
// worlds are. 0 means unrelated; 1 means identical; -1 means perfectly
// inverted. NewSeed(n) and NewSeed(n) trivially correlate at 1 (see the
// determinism tests elsewhere), so this function is only meaningful for
// distinct seeds.
func correlation(a, b Seed, points []samplePoint) float64 {
	n := float64(len(points))
	var sumA, sumB, sumAB, sumA2, sumB2 float64
	for _, p := range points {
		va := a.Eval2D(p.x, p.y)
		vb := b.Eval2D(p.x, p.y)
		sumA += va
		sumB += vb
		sumAB += va * vb
		sumA2 += va * va
		sumB2 += vb * vb
	}

	meanA := sumA / n
	meanB := sumB / n
	covariance := sumAB/n - meanA*meanB
	varA := sumA2/n - meanA*meanA
	varB := sumB2/n - meanB*meanB

	denom := math.Sqrt(varA * varB)
	if denom == 0 {
		return 0
	}
	return absf(covariance / denom)
}
