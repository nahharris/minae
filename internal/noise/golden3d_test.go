package noise

import (
	"bufio"
	"encoding/csv"
	"math/rand"
	"os"
	"strconv"
	"testing"
)

// This mirrors golden_test.go exactly, for Eval3D: M20's "if there is no 3D
// evaluation, you must add one to internal/noise, and it then falls under
// that package's own codegen guard and golden-vector discipline: add golden
// vectors for any new noise function." See golden_test.go's own comments for
// the full argument (frozen inputs AND outputs, regenerated only
// deliberately, checked on every architecture CI runs); nothing about it
// changes for a third input axis.

const golden3DPath = "testdata/golden3d.csv"

type goldenCase3D struct {
	seed    int64
	x, y, z float64
}

// goldenCases3D deliberately mixes small/large coordinates, positive/negative
// seeds, integer and half-integer coordinates (lattice-cell boundaries), and
// a block of pseudo-random cases -- the same mix goldenCases() uses for
// Eval2D, generalized to three axes.
func goldenCases3D() []goldenCase3D {
	cases := []goldenCase3D{
		{seed: 0, x: 0, y: 0, z: 0},
		{seed: 0, x: 1, y: 0, z: 0},
		{seed: 0, x: 0, y: 1, z: 0},
		{seed: 0, x: 0, y: 0, z: 1},
		{seed: 1, x: 0, y: 0, z: 0},
		{seed: -1, x: 0, y: 0, z: 0},
		{seed: 42, x: 12.5, y: -7.25, z: 3.5},
		{seed: 42, x: -1000.125, y: 1000.125, z: -500.5},
		{seed: 2026, x: 3.14159265, y: 2.71828182, z: 1.41421356},
		{seed: 2026, x: 16, y: 16, z: 8}, // a cell-boundary coordinate, per M20's 4x8 cell grid
		{seed: 2026, x: -16, y: -16, z: -8},
		{seed: 999999999, x: 0.5, y: 0.5, z: 0.5},
		{seed: -999999999, x: 0.5, y: -0.5, z: 0.5},
		{seed: 7, x: 100000, y: -100000, z: 50000},
	}

	rng := rand.New(rand.NewSource(20260921))
	for i := 0; i < 40; i++ {
		cases = append(cases, goldenCase3D{
			seed: rng.Int63()%2_000_000 - 1_000_000,
			// madd, not rng.Float64()*4000-2000: that shape fuses on arm64
			// and not amd64, exactly as golden_test.go's goldenCases notes.
			x: madd(rng.Float64(), 4000, -2000),
			y: madd(rng.Float64(), 4000, -2000),
			z: madd(rng.Float64(), 4000, -2000),
		})
	}
	return cases
}

// TestGoldenVectors3D is golden_test.go's TestGoldenVectors, for Eval3D.
// Regenerate deliberately with:
//
//	go test ./internal/noise/ -run TestGoldenVectors3D -update-golden
func TestGoldenVectors3D(t *testing.T) {
	cases := goldenCases3D()

	if *updateGolden {
		writeGolden3D(t, cases)
	}

	rows := readGolden3D(t)
	if len(rows) != len(cases) {
		t.Fatalf("%s has %d rows, but goldenCases3D() defines %d cases; regenerate with -update-golden after changing the case table",
			golden3DPath, len(rows), len(cases))
	}

	for i, row := range rows {
		got := NewSeed(row.seed).Eval3D(row.x, row.y, row.z)
		if got != row.value {
			t.Errorf("case %d: Seed(%d).Eval3D(%s, %s, %s) = %s, want %s (from %s)\n\n"+
				"If this change is deliberate, regenerate the golden file with:\n"+
				"  go test ./internal/noise/ -run TestGoldenVectors3D -update-golden",
				i, row.seed, formatFloat(row.x), formatFloat(row.y), formatFloat(row.z),
				formatFloat(got), formatFloat(row.value), golden3DPath)
		}
	}
}

func writeGolden3D(t *testing.T, cases []goldenCase3D) {
	t.Helper()

	f, err := os.Create(golden3DPath)
	if err != nil {
		t.Fatalf("creating %s: %v", golden3DPath, err)
	}
	defer func() { _ = f.Close() }()

	w := csv.NewWriter(f)
	if err := w.Write([]string{"seed", "x", "y", "z", "value"}); err != nil {
		t.Fatalf("writing %s header: %v", golden3DPath, err)
	}
	for _, c := range cases {
		v := NewSeed(c.seed).Eval3D(c.x, c.y, c.z)
		row := []string{
			strconv.FormatInt(c.seed, 10),
			formatFloat(c.x),
			formatFloat(c.y),
			formatFloat(c.z),
			formatFloat(v),
		}
		if err := w.Write(row); err != nil {
			t.Fatalf("writing %s row: %v", golden3DPath, err)
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		t.Fatalf("flushing %s: %v", golden3DPath, err)
	}
}

type goldenRow3D struct {
	seed    int64
	x, y, z float64
	value   float64
}

func readGolden3D(t *testing.T) []goldenRow3D {
	t.Helper()

	f, err := os.Open(golden3DPath)
	if err != nil {
		t.Fatalf("opening %s: %v (run with -update-golden to create it, then commit it)", golden3DPath, err)
	}
	defer func() { _ = f.Close() }()

	r := csv.NewReader(bufio.NewReader(f))
	rows, err := r.ReadAll()
	if err != nil {
		t.Fatalf("reading %s: %v", golden3DPath, err)
	}
	if len(rows) == 0 {
		t.Fatalf("%s is empty", golden3DPath)
	}

	out := make([]goldenRow3D, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(row) != 5 {
			t.Fatalf("%s row %v has %d columns, want 5", golden3DPath, row, len(row))
		}
		seed, err := strconv.ParseInt(row[0], 10, 64)
		if err != nil {
			t.Fatalf("parsing seed column in %s row %v: %v", golden3DPath, row, err)
		}
		var parsed [4]float64
		for i, col := range row[1:] {
			v, err := strconv.ParseFloat(col, 64)
			if err != nil {
				t.Fatalf("parsing column %d in %s row %v: %v", i+1, golden3DPath, row, err)
			}
			parsed[i] = v
		}
		out = append(out, goldenRow3D{seed: seed, x: parsed[0], y: parsed[1], z: parsed[2], value: parsed[3]})
	}
	return out
}
