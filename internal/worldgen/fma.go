package worldgen

import "math"

// addProduct returns a*b + c as a single correctly-rounded operation, via
// math.FMA -- this package's equivalent of internal/noise/fma.go's madd,
// and for the same reason: Go permits fusing a plain a*b + c into one
// instruction on arm64 and not on amd64, skipping the intermediate IEEE-754
// rounding, and float64(a*b) + c does NOT prevent it either (the conversion
// is a no-op the compiler drops for float64 -- see noise/fma.go's comment
// for the full argument and the bug that taught it). That divergence would
// make the same seed generate a different terrain height, or select a
// different biome, on different players' machines.
//
// M17 gave this package a single call site (rawSurfaceHeight's addition of
// the peaks-and-valleys contribution). M19 gives it a second, unrelated one:
// weightedDistanceSquared's accumulation of a biome's weighted per-axis
// distance is exactly the same shape -- a sum of products -- and fuses the
// same way. Every product that feeds a sum anywhere in this package must be
// routed through here; TestNoUnintendedFusedOperations in
// fma_codegen_test.go checks the compiled arm64 code for exactly this,
// mirroring internal/noise's own guard.
func addProduct(a, b, c float64) float64 {
	return math.FMA(a, b, c)
}
