//go:build gpu

// This file needs a real GPU and a windowing system, so it is excluded from
// the default build and from CI, whose runners have neither. Run it by hand
// after touching a shader:
//
//	mise exec -- go test -tags gpu -run TestShaderCompiles ./internal/gfx/
//
// It opens a hidden 64x64 window, which is enough to get an OpenGL context.

package render

import (
	"runtime"
	"testing"

	rl "github.com/gen2brain/raylib-go/raylib"
	"github.com/nahharris/minae/internal/world/lighting"
)

// A GLSL compile error is the one rendering failure nothing else here can see.
// raylib logs it and then hands back its own default shader, so the program
// runs, draws, and looks subtly wrong with no error anywhere. Every uniform
// lookup against that fallback returns -1, and SetShaderValue on -1 is a
// silent no-op — so checking that our uniforms resolve is also a check that
// our shader, and not the fallback, is what got loaded.
func TestShaderCompilesAndBindsUniforms(t *testing.T) {
	// OpenGL contexts are bound to the OS thread that created them, and the Go
	// runtime is free to move a goroutine between threads. Without this the
	// context is gone by the time the shader loads, and cgo segfaults.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	rl.SetTraceLogLevel(rl.LogWarning)
	rl.SetConfigFlags(rl.FlagWindowHidden)
	rl.InitWindow(64, 64, "minae shader probe")
	defer rl.CloseWindow()

	shader := rl.LoadShaderFromMemory(lighting.VsCode, lighting.FsCode)
	defer rl.UnloadShader(shader)

	if shader.ID == 0 {
		t.Fatal("shader ID is 0; the shader did not load at all")
	}

	for _, name := range []string{uniformSkyTint, uniformBlockTint, uniformMinAmbient} {
		if loc := rl.GetShaderLocation(shader, name); loc < 0 {
			t.Errorf("uniform %q resolved to location %d.\n"+
				"Either the shader failed to compile and raylib substituted its default, "+
				"or the uniform was optimised out because nothing reads it.", name, loc)
		}
	}
}
