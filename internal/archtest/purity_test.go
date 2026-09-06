// Package archtest holds architectural constraints that the compiler cannot
// express. It contains tests only.
package archtest

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// raylibPkg is the dependency the simulation layer must stay free of. It is
// cgo, so anything that reaches it needs OpenGL, X11 and Wayland headers just
// to compile — which is what made the lighting engine untestable.
const raylibPkg = "github.com/gen2brain/raylib-go/raylib"

// purePackages must never depend on raylib, directly or transitively.
//
// Adding a package here is a promise. Removing one should be a deliberate
// decision recorded in a milestone document, not a quick fix for a red test.
var purePackages = []string{
	"github.com/nahharris/minae/internal/core",
	"github.com/nahharris/minae/internal/blocks",
	"github.com/nahharris/minae/internal/blocks/model",
	"github.com/nahharris/minae/internal/world",
	"github.com/nahharris/minae/internal/world/lighting",
	"github.com/nahharris/minae/internal/physics",
	"github.com/nahharris/minae/internal/platform/config",
	"github.com/nahharris/minae/internal/platform/logging",
	"github.com/nahharris/minae/internal/testutil",
}

func TestPurePackagesDoNotDependOnRaylib(t *testing.T) {
	for _, pkg := range purePackages {
		t.Run(pkg, func(t *testing.T) {
			for _, dep := range depsOf(t, pkg) {
				if dep == raylibPkg {
					t.Errorf(
						"%s depends on %s.\n\n"+
							"The simulation layer must stay pure so it can be tested without a GPU.\n"+
							"Convert at the boundary in internal/gfx or internal/player instead, or\n"+
							"use the types in internal/core.\n\n"+
							"To find the path that introduced it:\n"+
							"  go list -deps -json %s | grep -B5 raylib",
						pkg, raylibPkg, pkg,
					)
				}
			}
		})
	}
}

// TestGuardCoversRenderingPackages is a control. If go list silently stopped
// reporting transitive dependencies, the guard above would pass for the wrong
// reason, so assert that a package which genuinely does use raylib is still
// detected.
func TestGuardCoversRenderingPackages(t *testing.T) {
	const impure = "github.com/nahharris/minae/internal/gfx"

	for _, dep := range depsOf(t, impure) {
		if dep == raylibPkg {
			return
		}
	}
	t.Fatalf("%s no longer reports a raylib dependency; the purity guard is not actually checking anything", impure)
}

// depsOf returns the full transitive dependency list of a package.
func depsOf(t *testing.T, pkg string) []string {
	t.Helper()

	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		var stderr string
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr = string(exitErr.Stderr)
		}
		t.Fatalf("go list -deps %s: %v\n%s", pkg, err, stderr)
	}

	return strings.Fields(string(out))
}
