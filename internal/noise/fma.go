package noise

// madd and dot2 are the only two shapes of "product feeding a sum" this
// package needs, so every accumulation site routes through one of them
// instead of writing a*b+c inline. That gives the fusion hazard exactly one
// audited home per shape: a reviewer who trusts these two functions can trust
// every call site, and a future edit that "simplifies" a call site back to a
// raw expression has to delete an explicit float64(...) conversion to do it,
// which is a visible, reviewable diff rather than a silent one.
//
// Do not rewrite a call site as `a*b + c`. The Go spec allows the compiler to
// fuse that into a single fused-multiply-add instruction, skipping the
// intermediate rounding to float64 — and it is free to do so "across
// statements", so splitting the multiply and the add across two lines of
// caller code is not a workaround. Go emits FMA on arm64, ppc64, s390x and
// riscv64, and does not on amd64, so a fused call site produces a different
// bit pattern on those architectures than this one does — silently reshaping
// every world generated with this package on that platform. See
// docs/milestones/M16-noise-foundation.md.

// madd returns a*b + c with the product's rounding to float64 forced before
// the addition, so the result is bit-identical on every architecture
// regardless of whether that architecture fuses multiply-add.
func madd(a, b, c float64) float64 {
	return float64(a*b) + c
}

// dot2 returns the 2D dot product ax*bx + ay*by with each product's rounding
// forced independently. This needs two forced conversions, not one: an
// expression of the shape p*q + r*s gives the compiler two candidate
// fusions — fold the left product into the addition, or the right — so both
// must be rounded explicitly to rule out either.
func dot2(ax, ay, bx, by float64) float64 {
	return float64(ax*bx) + float64(ay*by)
}
