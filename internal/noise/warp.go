package noise

// warpDecorrelation offsets the second sample Warp2D takes from warp's field,
// so the x and y displacement come from two different points on the same
// field rather than the same value pushing both axes along one diagonal. The
// exact value is arbitrary; what matters is that it is large enough to decorrelate
// the two samples (the field's structure repeats over roughly unit-scale
// cells) and fixed, so the same seed always warps the same way.
const warpDecorrelation = 5.2

// Warp2D returns (x, y) displaced by warp's own field evaluated near that
// point, scaled by amplitude. It implements the domain-warping technique
// described in the milestone doc: sampling a second noise field at the
// caller's intended coordinate and using it to perturb where the caller
// actually samples turns visibly noise-shaped output into something that
// reads as eroded, without the generator needing to know anything about how
// the displacement was produced.
//
// Warp2D only computes the displaced coordinate; it is the caller's job to
// then sample whatever field they actually want at (wx, wy). This keeps
// warping composable: the same warp field can precede a plain Eval2D call or
// an FBm2D call, and a warped FBm2D call can itself be warped again by a
// different seed.
func Warp2D(warp Seed, x, y, amplitude float64) (wx, wy float64) {
	dx := warp.Eval2D(x, y)
	dy := warp.Eval2D(x+warpDecorrelation, y+warpDecorrelation)

	// Each offset is an accumulation (amplitude*d either side, added to the
	// original coordinate), so each product's rounding is forced before the
	// addition -- see the FMA design decision.
	wx = madd(amplitude, dx, x)
	wy = madd(amplitude, dy, y)
	return wx, wy
}
