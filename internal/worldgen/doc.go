// Package worldgen generates the plains terrain described in
// docs/milestones/M17-plains-terrain.md: a pure function of a world seed and
// global block coordinates, built on internal/noise's fBm and splines.
//
// SurfaceHeight is the load-bearing export -- a pure function of (seed,
// global x, global z) with no access to World and no dependence on which
// chunks exist yet or the order they were generated in. That is what makes
// seam and slope properties testable without generating a single chunk (see
// the milestone's "SurfaceHeight is exported" design decision), and what
// lets internal/game/app.go place the player's spawn without needing the
// spawn chunk to have arrived (see the M15 interaction noted there).
//
// Generate fills a chunk's blocks by calling SurfaceHeight once per column,
// at that column's GLOBAL coordinates. Both are methods on *Generator, built
// once by NewGenerator so a single construction's noise seeds and splines
// are reused across every chunk and column it is asked for.
//
// Even for the single biome this milestone produces, three independent noise
// fields -- continentalness, erosion, peaks-and-valleys -- are each shaped by
// their own declared spline (control-point data, not arithmetic) and then
// combined into a height. This is more machinery than one flat biome needs on
// its own; it is what lets M19's biome framework add axes to an existing
// structure instead of replacing a single height noise wholesale.
//
// Water, caves, ores, trees and every biome but plains are explicitly out of
// scope here. Sea level exists only as the datum the splines are shaped
// around -- nothing in this package places a water block or special-cases a
// column being above or below it.
package worldgen
