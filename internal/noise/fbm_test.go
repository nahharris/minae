package noise

import (
	"math/rand"
	"testing"
)

func defaultFBmConfig(octaves int) FBmConfig {
	return FBmConfig{Octaves: octaves, Lacunarity: 2, Gain: 0.5}
}

// Criterion 1, for FBm2D specifically: same seed and coordinate, same value.
func TestFBm2DDeterministic(t *testing.T) {
	seed := NewSeed(11)
	cfg := defaultFBmConfig(6)

	a := seed.FBm2D(12.34, -56.78, cfg)
	b := seed.FBm2D(12.34, -56.78, cfg)
	if a != b {
		t.Fatalf("FBm2D(12.34, -56.78) = %v then %v; same seed and coordinate must agree", a, b)
	}

	again := NewSeed(11).FBm2D(12.34, -56.78, cfg)
	if again != a {
		t.Fatalf("a fresh Seed built from the same value gave %v, want %v", again, a)
	}
}

// Criterion 3: naive octave summing overflows [-1, 1] unless normalised --
// this is the case the milestone doc calls out by name. This test uses many
// octaves specifically because the overflow a missing normalisation would
// produce grows with octave count; a 1- or 2-octave test could pass by
// accident even with normalisation deleted.
func TestFBm2DRangeIsBounded(t *testing.T) {
	seed := NewSeed(21)
	cfg := defaultFBmConfig(10)
	rng := rand.New(rand.NewSource(6))

	const samples = 50_000
	for i := 0; i < samples; i++ {
		x := rng.Float64()*20000 - 10000
		y := rng.Float64()*20000 - 10000
		v := seed.FBm2D(x, y, cfg)
		if v < -1 || v > 1 {
			t.Fatalf("FBm2D(%v, %v) with %d octaves = %v, outside the documented [-1, 1] range", x, y, cfg.Octaves, v)
		}
	}
}

// Octaves below 1 must not panic (division by a zero total weight, or a loop
// that never runs) and must behave as exactly one octave.
func TestFBm2DZeroOctavesIsOneOctave(t *testing.T) {
	seed := NewSeed(31)
	one := seed.FBm2D(3, 4, FBmConfig{Octaves: 1, Lacunarity: 2, Gain: 0.5})
	zero := seed.FBm2D(3, 4, FBmConfig{Octaves: 0, Lacunarity: 2, Gain: 0.5})
	if one != zero {
		t.Fatalf("FBm2D with Octaves: 0 gave %v, want %v (same as Octaves: 1)", zero, one)
	}
}

// More octaves should generally change the result relative to fewer octaves
// sampled at the same point -- a sanity check that later octaves are actually
// contributing rather than being silently dropped (e.g. by an off-by-one in
// the loop bound).
func TestFBm2DOctavesAccumulate(t *testing.T) {
	seed := NewSeed(41)
	rng := rand.New(rand.NewSource(7))

	differed := 0
	const trials = 50
	for i := 0; i < trials; i++ {
		x := rng.Float64()*2000 - 1000
		y := rng.Float64()*2000 - 1000

		v1 := seed.FBm2D(x, y, defaultFBmConfig(1))
		v4 := seed.FBm2D(x, y, defaultFBmConfig(4))
		if v1 != v4 {
			differed++
		}
	}

	if differed == 0 {
		t.Fatal("1-octave and 4-octave FBm2D agreed at every sample point; later octaves do not appear to be contributing anything")
	}
}
