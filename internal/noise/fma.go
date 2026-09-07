package noise

import "math"

// madd and dot2 are the only two shapes of "product feeding a sum" this
// package needs, and both are written with math.FMA so that the fusion
// happens *everywhere*, identically, instead of being left to the compiler.
//
// The obvious approach is the opposite one, and it does not work. Go's spec
// says an explicit floating-point conversion "rounds to the precision of the
// target type, preventing fusion that would discard that rounding", which
// reads like a licence to write float64(a*b) + c and be safe. It is not: for
// float64 to float64 the conversion changes no value, so the compiler is
// entitled to drop it before the fused-multiply-add rewrite runs — and it
// does. This package shipped that spelling, and the arm64 CI job caught
// madd compiling to a single FMADDD anyway. The spec sentence is really
// about float32, where the conversion genuinely narrows.
//
// Eval2D happened to survive that bug, and it is worth knowing why, because
// it was luck rather than design: its one multiply feeds two separate
// additions, so the compiler had to materialise the product regardless.
// Relying on that is relying on a register allocator.
//
// math.FMA gives the guarantee directly. It is specified as the exactly
// rounded fused result — one rounding, not two — so every architecture
// computes the same bits, whether it has an FMA instruction or reaches for a
// software fallback. Determinism stops depending on defeating an optimiser
// and starts depending on an operation that is defined.
//
// Do not rewrite a call site as a*b + c or as float64(a*b) + c. The first
// fuses on arm64 and not amd64; the second does the same, only while looking
// as though it does not. TestNoUnintendedFusedOperations checks the compiled
// arm64 code for exactly this.

// madd returns the fused a*b + c: the product and the sum are computed with a
// single rounding, identically on every architecture.
func madd(a, b, c float64) float64 {
	return math.FMA(a, b, c)
}

// dot2 returns the 2D dot product (ax, ay) . (bx, by) = ax*bx + ay*by.
//
// The left product is folded into the addition and the right is not, which is
// an arbitrary choice — but it is a fixed one, made here rather than left to
// whichever architecture the code is compiled for. That is the whole point:
// the result need not be the most accurate possible, it needs to be the same
// everywhere, forever.
func dot2(ax, ay, bx, by float64) float64 {
	return math.FMA(ax, bx, ay*by)
}
