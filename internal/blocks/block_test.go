package blocks

import "testing"

// TestOpaqueToLight pins down the single predicate world/lighting's
// isTransparent and world.Chunk.highestSolidY both derive from (M18's
// "light predicate and the sky ceiling must be the same predicate" design
// decision): nil is never opaque, and a non-nil block is opaque unless it
// declares LightTransparent.
func TestOpaqueToLight(t *testing.T) {
	opaque := &Block{ID: "test/opaque"}
	transparent := &Block{ID: "test/transparent", LightTransparent: true}

	cases := []struct {
		name string
		b    *Block
		want bool
	}{
		{"nil (air)", nil, false},
		{"ordinary block", opaque, true},
		{"light-transparent block", transparent, false},
	}
	for _, tc := range cases {
		if got := OpaqueToLight(tc.b); got != tc.want {
			t.Errorf("%s: OpaqueToLight = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestBlock_HidesFaceOf checks every pairing HidesFaceOf must distinguish,
// including the case the milestone singles out as the reason SelfCulling is
// declared independently of LightTransparent: a solid block's face must not
// be hidden by a light-transparent neighbour that isn't its own kind, or a
// stone face behind translucent leaves would vanish along with them.
func TestBlock_HidesFaceOf(t *testing.T) {
	stone := &Block{ID: "test/stone"}
	otherOpaque := &Block{ID: "test/other_opaque"}
	leaves := &Block{ID: "test/leaves", LightTransparent: true, SelfCulling: true}
	glassLike := &Block{ID: "test/glass", LightTransparent: true} // transparent, but does not self-cull

	cases := []struct {
		name        string
		neighbour   *Block
		self        *Block
		wantHidesMe bool
	}{
		{"nil neighbour hides nothing", nil, stone, false},
		{"opaque neighbour always hides", stone, otherOpaque, true},
		{"opaque neighbour hides even a leaf's face", stone, leaves, true},
		{"leaf neighbour hides another leaf's face", leaves, leaves, true},
		{"leaf neighbour does NOT hide a stone face behind it", leaves, stone, false},
		{"leaf neighbour does not hide a different transparent block's face", leaves, glassLike, false},
		{"non-self-culling transparent neighbour hides nothing, even its own kind", glassLike, glassLike, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.neighbour.HidesFaceOf(tc.self); got != tc.wantHidesMe {
				t.Errorf("HidesFaceOf: neighbour=%v self=%v => %v, want %v", tc.neighbour, tc.self, got, tc.wantHidesMe)
			}
		})
	}
}
