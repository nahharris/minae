# M22 — Mysticness

**Status:** 📋 Planned
**Depends on:** [M21](M21-caves.md)
**Inherits:** [world-generation constraints](../design/worldgen-constraints.md)

## Objective

The first magi-tech content: a biome that only exists at high mysticness, and
floating islands above it. This is where the world stops being generic, and
the milestone where restraint matters most.

## On restraint with the fantasy

Carried over from the original sketch, because it is the design principle this
milestone exists to honour.

The world should read as a *place* first. Impossible things land harder when
they are rare: a single floating island on the horizon is remarkable, a sky full
of them is wallpaper. Mysticness is the mechanism for exactly that — most of the
world sits low on it and looks like a world, and the strange regions are strange
because they are the exception.

"Not too absurdly unrealistic, with poetic licence for floating islands" is the
brief. The licence is spent on a few striking things rather than spread thin.

## Design decisions

### Mysticness gates, it does not decorate

The mysticness axis has existed since M19, with every biome placed low on it.
This milestone adds a biome placed high, and nearest-neighbour selection does
the rest — no conditionals, no special case.

The mystic biome is the only one where fantastical features may appear, and
floating islands appear only where mysticness is high. Ordinary biomes gain
nothing strange. That is the restraint principle turned into a rule the code
can enforce and a test can check.

### Restraint is a test, not a hope

The fraction of the world above the mystic threshold is bounded — a declared
small share of a large sample — and floating islands occur only inside it. Taste
is not testable; *proportion* is, and proportion is most of what the brief
asks for.

### Floating islands are positive density with nothing beneath

With a density field (M20), an island is a region of positive density high
above the ground, shaped by noise so it tapers underneath rather than ending in
a flat slab. No separate structure placement: islands are density, so they
inherit purity, seams and determinism the same way caves did.

### A magi-tech block or two, with light

A mystic biome needs something that says so. The obvious candidate is a
crystal that emits block light — Glowstone already proves the light engine
handles emitters — placed sparingly in the mystic biome. Every new block must
answer `LightTransparent` deliberately, per constraint 7.

## The risk this milestone carries: the sky ceiling

Recorded in advance, because it is foreseeable and it is the one most likely to
undo the streaming performance work.

The per-frame lighting cost was cut from 103 ms to under 4 ms per chunk, partly
by skipping sky cells above the highest opaque block in each chunk's 3×3
neighbourhood — everything above that is open sky at full brightness, so it
cannot propagate anywhere new. With terrain topping out around y=76, that skips
most of every column.

A floating island at y=180 raises that ceiling to 180 for its own chunk and all
eight neighbours. The seeding walk then enqueues nearly every sky cell below
180 across nine chunks — several times the current work, and a large step back
toward the cost that froze the game.

The optimisation stays *correct* under islands; it only stops helping. The
likely fix is to make the ceiling per column rather than per chunk: a cell only
needs enqueueing if one of its four horizontal neighbours has opaque geometry
somewhere above it, which is a per-column height map maintained exactly the way
`highestSolidY` is today. Under that rule an island only costs anything in the
columns beneath and beside it.

**Measure first.** Benchmark `SeedChunkColdNeighbours` with an island
overhead before touching the ceiling. If the regression is small, leave it; if
not, the per-column ceiling is the planned answer, and M18's lesson applies to
its test — the terrain must put the island *above* every other opaque block in
the neighbourhood, or the test does not strain the thing it claims to.

## Validation criteria

1. **The mystic biome appears only at high mysticness**, and nearest-neighbour
   selection is unchanged for every existing biome — adding a biome must not
   move any M19 boundary except where mysticness is high.
2. **Rarity is bounded**: the share of the world above the mystic threshold
   stays within a declared band, and every floating island lies inside it.
3. **Islands are well-formed**: no island smaller than a threshold, none
   touching the ground, tapered underneath rather than slab-bottomed.
4. **Deterministic, load-order independent, seamless**, per the constraints.
5. **Lighting equivalence holds under an island** that straddles a chunk seam,
   with the island computed to sit above every other opaque block in the
   neighbourhood.
6. **Costs reported**, with the island case benchmarked explicitly and the sky
   ceiling decision justified by the number.

## Manual verification

- [ ] A floating island seen from a distance is remarkable.
- [ ] The mystic region reads as strange *because* its surroundings are not.
- [ ] Travelling in, the world gets stranger by degrees rather than switching.

## Explicitly out of scope

Whether mysticness affects gameplay — spawns, resources, magical effects. That
decision reaches far beyond world generation and deserves its own discussion
before any code. Also structures, and more than one mystic biome.
