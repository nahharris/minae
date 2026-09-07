package noise

// permTableSize is the size of the permutation table. It is a power of two so
// that converting a lattice coordinate to uint8 (see gradientIndex) doubles as
// a fast, correct-for-negative-inputs modulo-256: Go's conversion from a
// signed integer to uint8 keeps only the low 8 bits (a defined truncation,
// not an arithmetic operation — see the Go spec's "Conversions" section), and
// for two's-complement values that is exactly value mod 256.
const permTableSize = 256

// gradientCount is the number of directions in the gradient table (see
// gradients2D). It must be a power of two so masking with gradientCount-1
// substitutes for a modulo when turning a permutation-table byte into a
// gradient index.
const gradientCount = 16

// Seed derives everything a 2D noise field needs from a single 64-bit value: a
// permutation table used to hash lattice coordinates into gradient indices.
//
// Two Seed values built from the same int64 always produce identical fields
// (criterion 1). Two Seeds built from adjacent int64s must not: see NewSeed.
type Seed struct {
	perm [permTableSize]uint8
}

// NewSeed builds the permutation table for value.
//
// The classic mistake here is folding the seed in by truncation or a
// low-quality shuffle — using value directly as an index, or XOR-ing it into
// a handful of table entries — which makes seeds 1000 and 1001 produce
// visibly related tables, and therefore visibly related worlds. NewSeed
// instead drives a full Fisher-Yates shuffle of 0..255 from splitmix64, a
// well-understood 64-bit mixer whose adjacent inputs produce unrelated
// 64-bit streams. See the "Seeding must mix" design decision in
// docs/milestones/M16-noise-foundation.md.
func NewSeed(value int64) Seed {
	state := uint64(value)

	var perm [permTableSize]uint8
	for i := range perm {
		perm[i] = uint8(i)
	}

	for i := permTableSize - 1; i > 0; i-- {
		r := splitmix64(&state)
		// r is uniform over the full uint64 range; reducing it mod (i+1)
		// (i+1 <= 256) with i+1 nowhere near a power of two dividing 2^64
		// introduces a bias far too small to matter for a hash table used
		// only to pick gradients, and avoiding it would cost a rejection
		// loop for no observable benefit here.
		j := int(r % uint64(i+1))
		perm[i], perm[j] = perm[j], perm[i]
	}

	return Seed{perm: perm}
}

// splitmix64 advances *state and returns the next value in its output stream.
// This is the reference splitmix64 mixer: four operations, well documented,
// and specifically chosen because it turns adjacent seeds into unrelated
// streams — the property NewSeed depends on.
func splitmix64(state *uint64) uint64 {
	*state += 0x9E3779B97F4A7C15
	z := *state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// gradientIndex hashes a lattice coordinate to an index into gradients2D.
//
// It double-hashes through perm (look up ix, then use that to look up a
// second time combined with iy) rather than combining ix and iy with a single
// arithmetic mix, which is the standard technique for avoiding directional
// correlation between the x and y hash streams — a single mix like
// perm[ix]^perm[iy] tends to leave visible structure along one axis.
func (s Seed) gradientIndex(ix, iy int32) uint8 {
	a := s.perm[uint8(ix)]
	b := s.perm[uint8(int32(a)+iy)]
	return b & (gradientCount - 1)
}
