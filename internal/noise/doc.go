// Package noise is the deterministic noise library every terrain generator
// downstream of it is built from. See docs/milestones/M16-noise-foundation.md
// for why it exists and the design decisions binding this implementation.
//
// It imports nothing outside the standard library — internal/archtest enforces
// this — so it stays property-testable with no world, no GPU and no other
// package's assumptions baked in.
//
// # Determinism across architectures
//
// A seed must produce the same field on every machine, forever. Go's spec
// permits fusing `a*b + c` into a single fused-multiply-add on some
// architectures (arm64, ppc64, s390x, riscv64) but not others (amd64),
// skipping the intermediate IEEE-754 rounding step and producing a different
// bit pattern from the same source and the same inputs. Every place in this
// package where a product feeds into a sum forces that rounding with an
// explicit float64(...) conversion — see madd and dot2 in fma.go — because the
// Go spec guarantees an explicit conversion rounds to the target precision
// before the value can be fused with anything else, and it does so regardless
// of inlining or statement boundaries.
//
// # float64 only
//
// The rest of the engine is float32; this package is float64 throughout,
// input and output. Octave summation and domain warping accumulate error
// over many terms, and narrowing to float32 before that accumulation is done
// would throw away precision exactly where the determinism guarantee is most
// delicate. Callers convert once, at the point of use.
package noise
