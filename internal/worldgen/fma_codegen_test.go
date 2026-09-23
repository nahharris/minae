package worldgen

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// This mirrors internal/noise/fma_codegen_test.go exactly, extended to this
// package per the milestone's "the FMA guard reaches worldgen here" design
// decision: biome distance (biome.go's weightedDistanceSquared) is a
// weighted sum of squares, the same shape that fused on arm64 in
// internal/noise, and a biome choice that differs across architectures means
// a different world on a different machine.

// fusedInstruction matches arm64's fused multiply-add family in the
// compiler's assembly output.
var fusedInstruction = regexp.MustCompile(`\b(FMADDD|FMSUBD|FNMADDD|FNMSUBD|FMADDS|FMSUBS|FNMADDS|FNMSUBS)\b`)

// sourceLine pulls the "(path/file.go:123)" attribution the compiler prints
// on each instruction.
var sourceLine = regexp.MustCompile(`\(([^()]*\.go):(\d+)\)`)

// TestNoUnintendedFusedOperations compiles this package for arm64 and checks
// that every fused multiply-add in the generated code comes from an explicit
// math.FMA call in fma.go. See internal/noise/fma_codegen_test.go for the
// full argument; nothing about it changes by moving packages.
func TestNoUnintendedFusedOperations(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("no host toolchain to cross-compile with")
	}

	cmd := exec.Command("go", "build", "-gcflags=-S", "./")
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("cross-compiling for arm64 failed: %v\n%s", err, out)
	}

	var offenders []string
	for _, line := range strings.Split(string(out), "\n") {
		if !fusedInstruction.MatchString(line) {
			continue
		}

		m := sourceLine.FindStringSubmatch(line)
		if m == nil {
			offenders = append(offenders, strings.TrimSpace(line))
			continue
		}

		// Inlining preserves the original attribution, so a fused operation
		// that came from addProduct still points at fma.go however deeply
		// it was inlined.
		file := m[1]
		if strings.HasSuffix(file, "/fma.go") || strings.HasSuffix(file, `\fma.go`) {
			continue
		}
		offenders = append(offenders, strings.TrimSpace(line))
	}

	if len(offenders) > 0 {
		t.Errorf("found %d fused multiply-add instruction(s) outside fma.go in the arm64 build.\n\n"+
			"Every fused operation in this package must come from an explicit math.FMA\n"+
			"call in fma.go, so that it fuses identically on every architecture. A fused\n"+
			"operation anywhere else computes different bits on arm64 than on amd64,\n"+
			"which means the same seed generates a different world -- or selects a\n"+
			"different biome -- on different machines.\n\n"+
			"Rewrite the offending expression to call addProduct. Note that\n"+
			"float64(a*b) + c does NOT prevent fusion -- see fma.go's comment.\n\n"+
			"Offending instructions:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestCodegenGuardCanActuallyFail is the control for the test above, in the
// same spirit as internal/noise's copy: a guard that has never been seen to
// fire is not known to be a guard at all.
func TestCodegenGuardCanActuallyFail(t *testing.T) {
	dir := t.TempDir()

	const src = `package fuseprobe

func Probe(a, b, c float64) float64 { return a*b + c }
`
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(dir+"/"+name, []byte(content), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	write("go.mod", "module fuseprobe\n\ngo 1.25\n")
	write("probe.go", src)

	cmd := exec.Command("go", "build", "-gcflags=-S", "./")
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compiling the probe package failed: %v\n%s", err, out)
	}

	if !fusedInstruction.MatchString(string(out)) {
		t.Fatal("a bare `a*b + c` did not compile to a fused instruction on arm64; " +
			"TestNoUnintendedFusedOperations is no longer detecting anything")
	}
}
