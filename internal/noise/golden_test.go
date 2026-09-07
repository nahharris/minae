package noise

import (
	"bufio"
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// updateGolden regenerates testdata/golden.csv from the current
// implementation. Run it deliberately, and only deliberately:
//
//	go test ./internal/noise/ -run TestGoldenVectors -update-golden
//
// This is the "documented test helper" the milestone doc asks for in place
// of a separate go:generate program: the generation logic and the check that
// consumes its output are the same table (goldenCases, below), so they
// cannot drift apart the way a hand-copied case list and a generator script
// could.
var updateGolden = flag.Bool("update-golden", false, "regenerate testdata/golden.csv from the current implementation")

const goldenPath = "testdata/golden.csv"

// goldenCase is one row of testdata/golden.csv: a seed and coordinate, and
// the value this package's implementation produced for it the last time
// testdata/golden.csv was deliberately regenerated.
type goldenCase struct {
	seed int64
	x, y float64
}

// goldenCases is the fixed table of (seed, x, y) inputs golden vectors are
// frozen for. It deliberately mixes small and large coordinates, positive and
// negative seeds, and values that land near lattice cell boundaries (integer
// and half-integer coordinates) as well as arbitrary fractional ones, since a
// regression is more likely to show up at a boundary than in a cell's
// interior.
func goldenCases() []goldenCase {
	cases := []goldenCase{
		{seed: 0, x: 0, y: 0},
		{seed: 0, x: 1, y: 0},
		{seed: 0, x: 0, y: 1},
		{seed: 1, x: 0, y: 0},
		{seed: -1, x: 0, y: 0},
		{seed: 42, x: 12.5, y: -7.25},
		{seed: 42, x: -1000.125, y: 1000.125},
		{seed: 2026, x: 3.14159265, y: 2.71828182},
		{seed: 2026, x: 16, y: 16}, // a chunk-boundary coordinate, see criterion 7's test
		{seed: 2026, x: -16, y: -16},
		{seed: 999999999, x: 0.5, y: 0.5},
		{seed: -999999999, x: 0.5, y: -0.5},
		{seed: 7, x: 100000, y: -100000},
	}

	// A block of pseudo-random cases from a fixed RNG stream, large enough
	// that a subtle refactor touching only some code paths (say, only the
	// "middle corner is (1,0)" branch) has good odds of hitting one.
	rng := rand.New(rand.NewSource(20260907))
	for i := 0; i < 40; i++ {
		cases = append(cases, goldenCase{
			seed: rng.Int63()%2_000_000 - 1_000_000,
			x:    rng.Float64()*4000 - 2000,
			y:    rng.Float64()*4000 - 2000,
		})
	}
	return cases
}

// Criteria 8 and 9: a committed table of (seed, coordinate) -> value
// reproduces exactly, on every architecture CI runs this on (amd64 and,
// separately, arm64 -- see .github/workflows/ci.yml). This is what turns
// "deterministic" from a property of one build into a promise across every
// future one: a later refactor that changes a single output value fails this
// test loudly instead of silently reshaping every world already generated
// with this package.
//
// There is no external reference to check these values against -- see
// "Golden vectors are what forever actually means" in the milestone doc --
// so this table is frozen from this package's own implementation, the first
// time it satisfied every other test in this package. Regenerating it is a
// deliberate act (see updateGolden above), not something that happens as a
// side effect of running the suite.
func TestGoldenVectors(t *testing.T) {
	cases := goldenCases()

	if *updateGolden {
		writeGolden(t, cases)
	}

	want := readGolden(t)
	if len(want) != len(cases) {
		t.Fatalf("%s has %d rows, but goldenCases() defines %d cases; regenerate with -update-golden after changing the case table",
			goldenPath, len(want), len(cases))
	}

	for i, c := range cases {
		got := NewSeed(c.seed).Eval2D(c.x, c.y)
		if got != want[i] {
			t.Errorf("case %d: Seed(%d).Eval2D(%v, %v) = %v, want %v (from %s)\n\n"+
				"If this change is deliberate, regenerate the golden file with:\n"+
				"  go test ./internal/noise/ -run TestGoldenVectors -update-golden",
				i, c.seed, c.x, c.y, got, want[i], goldenPath)
		}
	}
}

func writeGolden(t *testing.T, cases []goldenCase) {
	t.Helper()

	f, err := os.Create(goldenPath)
	if err != nil {
		t.Fatalf("creating %s: %v", goldenPath, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"seed", "x", "y", "value"}); err != nil {
		t.Fatalf("writing %s header: %v", goldenPath, err)
	}
	for _, c := range cases {
		v := NewSeed(c.seed).Eval2D(c.x, c.y)
		row := []string{
			strconv.FormatInt(c.seed, 10),
			formatFloat(c.x),
			formatFloat(c.y),
			formatFloat(v),
		}
		if err := w.Write(row); err != nil {
			t.Fatalf("writing %s row: %v", goldenPath, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flushing %s: %v", goldenPath, err)
	}
}

// readGolden reads the frozen values back, in row order, skipping the
// header. It fails the test (rather than the whole package) if the file is
// missing, since a missing golden file is a setup mistake, not a build
// break.
func readGolden(t *testing.T) []float64 {
	t.Helper()

	f, err := os.Open(goldenPath)
	if err != nil {
		t.Fatalf("opening %s: %v (run with -update-golden to create it, then commit it)", goldenPath, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(bufio.NewReader(f))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("reading %s: %v", goldenPath, err)
	}
	if len(rows) == 0 {
		t.Fatalf("%s is empty", goldenPath)
	}

	values := make([]float64, 0, len(rows)-1)
	for _, row := range rows[1:] {
		v, err := strconv.ParseFloat(row[3], 64)
		if err != nil {
			t.Fatalf("parsing value column in %s row %v: %v", goldenPath, row, err)
		}
		values = append(values, v)
	}
	return values
}

// formatFloat writes v with enough digits (17 significant decimal digits) to
// round-trip a float64 exactly, per strconv's documented guarantee for
// format 'g' with precision -1... except we pin the precision explicitly
// here rather than relying on -1, so the file's column width is stable and
// the exact byte representation is not tied to strconv's shortest-round-trip
// heuristic changing between Go versions.
func formatFloat(v float64) string {
	return fmt.Sprintf("%.17g", v)
}
