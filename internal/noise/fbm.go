package noise

// FBmConfig controls how FBm2D layers Eval2D into fractal Brownian motion.
type FBmConfig struct {
	// Octaves is the number of layers summed. Fewer than 1 is treated as 1.
	Octaves int
	// Lacunarity is the frequency multiplier applied to the sample
	// coordinate between octaves. 2 (each octave doubling frequency) is the
	// conventional choice and what the milestone doc describes.
	Lacunarity float64
	// Gain is the amplitude multiplier applied between octaves. 0.5 (each
	// octave's amplitude halving) is the conventional choice.
	Gain float64
}

// FBm2D sums Octaves layers of s's field at doubling frequency and halving
// amplitude (or whatever Lacunarity/Gain specify), and normalises the result
// back into [-1, 1].
//
// Normalising is not optional. Summing N unnormalised octaves of a field
// already in [-1, 1] can reach a magnitude of N — naive octave summing
// overflows the documented range, which is exactly criterion 3's concern.
// FBm2D instead divides by the sum of the amplitudes actually used, which
// makes the result a weighted average of values each already in [-1, 1] and
// therefore itself in [-1, 1] by construction, for any positive Gain: no
// clamping, and no sample can push the result outside the range no matter
// how the octaves happen to align.
func (s Seed) FBm2D(x, y float64, cfg FBmConfig) float64 {
	octaves := cfg.Octaves
	if octaves < 1 {
		octaves = 1
	}

	var (
		sum          float64
		totalWeight  float64
		amplitude    = 1.0
		freqX, freqY = x, y
	)

	for range octaves {
		n := s.Eval2D(freqX, freqY)

		// sum accumulates amplitude*n; forcing the product's rounding here
		// is the whole reason FBm2D exists as more than "call Eval2D in a
		// loop" — see the FMA design decision.
		sum = madd(amplitude, n, sum)

		// Pure addition of an already-materialized amplitude: no multiply is
		// attached to this operation, so it carries no fusion risk on its
		// own.
		totalWeight += amplitude

		// Each reassignment below is a single multiply with no addition
		// touching its result in this expression, but the result is used on
		// the next iteration inside additions deep in Eval2D's skew
		// computation, so it is forced here rather than relying on a
		// downstream conversion the loop does not control.
		freqX = float64(freqX * cfg.Lacunarity)
		freqY = float64(freqY * cfg.Lacunarity)
		amplitude = float64(amplitude * cfg.Gain)
	}

	if totalWeight == 0 {
		// The [-1, 1] guarantee above is proved for positive Gain, where
		// every weight is positive and totalWeight can only be zero if
		// octaves is zero -- already excluded by the clamp above. Outside
		// that documented contract (Gain <= 0), weights can have mixed signs
		// and cancel exactly; this guards the division below rather than
		// dividing by zero in that case.
		return 0
	}

	return sum / totalWeight
}
