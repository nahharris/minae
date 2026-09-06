package render

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/nahharris/minae/internal/world/lighting"
)

// Shader uniforms are bound by string at runtime. If a name in the GLSL source
// and a name in this package ever drift apart, rl.GetShaderLocation returns -1
// and rl.SetShaderValue silently does nothing — no error, no log, just a world
// lit by whatever the uniform happens to default to. That failure is invisible
// in every automated check we have, so it gets pinned here.
func TestFragmentShaderDeclaresEveryUniformWeSet(t *testing.T) {
	t.Parallel()

	declared := declaredUniforms(lighting.FsCode)

	for _, name := range []string{
		uniformSkyTint,
		uniformBlockTint,
		uniformMinAmbient,
		uniformTexture0,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if !declared[name] {
				t.Errorf("this package binds the uniform %q, but the fragment shader does not declare it.\n"+
					"Uniforms actually declared: %v\n"+
					"A missing uniform is bound to location -1 and set silently, so nothing would report this at runtime.",
					name, sortedKeys(declared))
			}
		})
	}
}

// The reverse direction: a uniform the shader declares but nothing ever sets
// reads as zero, which for a tint means a black world. Catching it here beats
// discovering it by looking at the screen.
func TestEveryFragmentUniformIsSetByThisPackage(t *testing.T) {
	t.Parallel()

	bound := map[string]bool{
		uniformSkyTint:    true,
		uniformBlockTint:  true,
		uniformMinAmbient: true,
		uniformTexture0:   true, // Bound by raylib itself, from the material's map.
	}

	for name := range declaredUniforms(lighting.FsCode) {
		if !bound[name] {
			t.Errorf("the fragment shader declares uniform %q but nothing in this package sets it; "+
				"it will read as zero", name)
		}
	}
}

// The vertex shader must not reintroduce matModel. Chunk transforms are pure
// translation, so the normal needs no correction, and the usual
// transpose(inverse(matModel)) costs a matrix inverse per vertex per frame to
// compute the identity.
func TestVertexShaderDoesNotInvertTheModelMatrix(t *testing.T) {
	t.Parallel()

	// Comments are stripped first: the shader carries a comment explaining why
	// transpose(inverse(matModel)) is not used, and matching that would be a
	// false positive on the very documentation of the decision.
	if strings.Contains(stripComments(lighting.VsCode), "inverse(") {
		t.Error("vertex shader calls inverse(); chunk transforms are pure translation, so the normal needs no correction")
	}
}

// Light must not travel in the alpha channel. Encoding it there is what made
// dark blocks render see-through instead of dark — shadows looked brighter than
// lit faces, and caves got brighter the deeper they went.
func TestFragmentShaderDoesNotDeriveLightFromAlpha(t *testing.T) {
	t.Parallel()

	// The one legitimate use of vertex alpha is as opacity, multiplied into the
	// output alpha. It must never reach the RGB term.
	for _, line := range strings.Split(stripComments(lighting.FsCode), "\n") {
		code := strings.TrimSpace(line)
		if !strings.Contains(code, "fragColor.a") {
			continue
		}
		if !strings.Contains(code, "finalColor") {
			t.Errorf("fragColor.a is used outside the final alpha term, in: %q\n"+
				"Vertex alpha is opacity, not light.", code)
		}
	}
}

// stripComments removes // line comments from GLSL source, so that assertions
// about what the shader *does* are not confused by prose about what it
// deliberately does not do.
func stripComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "//"); idx >= 0 {
			lines[i] = line[:idx]
		}
	}
	return strings.Join(lines, "\n")
}

var uniformPattern = regexp.MustCompile(`(?m)^\s*uniform\s+\w+\s+(\w+)\s*;`)

func declaredUniforms(src string) map[string]bool {
	out := make(map[string]bool)
	for _, m := range uniformPattern.FindAllStringSubmatch(src, -1) {
		out[m[1]] = true
	}
	return out
}

func sortedKeys(m map[string]bool) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	return fmt.Sprint(names)
}
