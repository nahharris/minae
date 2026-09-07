package noise

import (
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// fusedInstruction matches arm64's fused multiply-add family in the
// compiler's assembly output.
var fusedInstruction = regexp.MustCompile(`\b(FMADDD|FMSUBD|FNMADDD|FNMSUBD|FMADDS|FMSUBS|FNMADDS|FNMSUBS)\b`)

// sourceLine pulls the "(path/file.go:123)" attribution the compiler prints
// on each instruction. Inlined code keeps its original file and line, which
// is what makes this test able to say *which* source line fused.
var sourceLine = regexp.MustCompile(`\(([^()]*\.go):(\d+)\)`)

// TestNoUnintendedFusedOperations compiles this package for arm64 and checks
// that every fused multiply-add in the generated code comes from an explicit
// math.FMA call in fma.go.
//
// This exists because the property it guards is invisible on the machine most
// of us develop on. Go emits FMA on arm64, ppc64, s390x and riscv64 and not
// on amd64, so a fused accumulation produces different bits — and therefore a
// different world from the same seed — on some players' machines and not
// others. An amd64 test suite cannot see that, however thorough.
//
// It caught a real bug in the form it replaced. This package originally
// prevented fusion by writing float64(a*b) + c, on the strength of the spec
// sentence about explicit conversions forcing a rounding. For float64 that
// conversion changes no value, so the compiler drops it before the FMA
// rewrite runs, and madd compiled to a single FMADDD regardless. Nothing on
// amd64 could have noticed.
//
// Reading the compiled instructions is the only check that actually answers
// the question. It runs anywhere, needs no arm64 hardware, and names the
// offending source line rather than reporting that some number changed. The
// arm64 CI job is still worth keeping — it exercises the real toolchain end
// to end — but this is what makes the property testable during development.
func TestNoUnintendedFusedOperations(t *testing.T) {
	if runtime.GOOS == "js" || runtime.GOOS == "wasip1" {
		t.Skip("no host toolchain to cross-compile with")
	}

	cmd := exec.Command("go", "build", "-gcflags=-S", "./")
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "GOARCH=arm64", "CGO_ENABLED=0")

	// The compiler writes its assembly listing to stderr, and CombinedOutput
	// keeps it whether or not the build itself succeeds.
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
		// that came from madd or dot2 still points at fma.go however deeply
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
			"which means the same seed generates a different world on different machines.\n\n"+
			"Rewrite the offending expression to call madd or dot2. Note that\n"+
			"float64(a*b) + c does NOT prevent fusion -- see fma.go's comment.\n\n"+
			"Offending instructions:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestCodegenGuardCanActuallyFail is the control for the test above, in the
// same spirit as archtest's TestGuardCoversRenderingPackages: a guard that
// has never been seen to fire is not known to be a guard at all.
//
// It compiles a throwaway package containing a bare a*b + c and asserts the
// detector finds a fused instruction in it. If arm64 codegen ever stops
// fusing that shape, or the instruction names change, this fails and tells us
// the guard above has quietly become vacuous.
func TestCodegenGuardCanActuallyFail(t *testing.T) {
	dir := t.TempDir()

	const src = `package fuseprobe

func Probe(a, b, c float64) float64 { return a*b + c }
`
	write := func(name, content string) {
		t.Helper()
		if err := writeFile(dir+"/"+name, content); err != nil {
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

// writeFile is a tiny helper so the control test above reads as one thought
// rather than four error checks.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
