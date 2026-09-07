package archtest

import (
	"strings"
	"testing"
)

// noisePkg is the arithmetic every terrain generator downstream of it is
// built from -- see docs/milestones/M16-noise-foundation.md. It must import
// nothing outside the standard library, which is a stronger requirement than
// TestPurePackagesDoNotDependOnRaylib above: that guard only rules out one
// specific dependency (raylib), because the packages it covers legitimately
// need other things, like internal/core's shared value types. internal/noise
// needs nothing but arithmetic, so it gets the stricter guard here instead.
const noisePkg = "github.com/nahharris/minae/internal/noise"

// TestNoiseDependsOnlyOnStandardLibrary asserts that internal/noise imports
// nothing but the Go standard library, directly or transitively.
//
// This is what keeps it property-testable with no world, no GPU, and no
// other package's assumptions baked in -- see the package doc in
// internal/noise/doc.go. Unlike the raylib guard, this one has no exceptions
// to enumerate: any non-standard-library dependency at all is a violation,
// so isStandardLibraryPackage below only needs to answer "is this package
// part of Go itself".
func TestNoiseDependsOnlyOnStandardLibrary(t *testing.T) {
	for _, dep := range depsOf(t, noisePkg) {
		if dep == noisePkg {
			continue
		}
		if !isStandardLibraryPackage(dep) {
			t.Errorf("%s depends on %s, which is not part of the Go standard library.\n\n"+
				"internal/noise must depend on nothing else, so it stays a pure, "+
				"property-testable library with no world and no GPU -- see "+
				"docs/milestones/M16-noise-foundation.md.\n\n"+
				"To find the path that introduced it:\n"+
				"  go list -deps -json %s | grep -B5 %q",
				noisePkg, dep, noisePkg, dep)
		}
	}
}

// TestNoiseGuardCanDetectANonStandardDependency is a control, mirroring
// TestGuardCoversRenderingPackages above: it proves
// isStandardLibraryPackage's classification is not simply returning true for
// everything, by checking it correctly flags a package known to depend on
// something outside the standard library.
func TestNoiseGuardCanDetectANonStandardDependency(t *testing.T) {
	const impure = "github.com/nahharris/minae/internal/gfx"

	for _, dep := range depsOf(t, impure) {
		if !isStandardLibraryPackage(dep) {
			return
		}
	}
	t.Fatalf("every dependency of %s was classified as standard library; the noise purity guard is not actually checking anything", impure)
}

// isStandardLibraryPackage reports whether pkg is part of the Go standard
// library, using the same heuristic goimports and go/build's tooling rely
// on: a standard library import path never contains a dot in its first
// path segment, while every module-qualified path does (a domain name, by
// Go module convention). "internal/reflectlite" (no dot, standard library)
// and "github.com/nahharris/minae/internal/noise" (dot in "github.com", not
// standard library) are both classified correctly by this rule.
func isStandardLibraryPackage(pkg string) bool {
	first, _, _ := strings.Cut(pkg, "/")
	return !strings.Contains(first, ".")
}
