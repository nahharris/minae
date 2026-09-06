# M6 — Block light sources

**Status:** ✅ Done — pending visual confirmation

## Objective

Blocks can emit light. A glowstone in a sealed cave lights its surroundings, and
that light is unaffected by the time of day — the vertex `G` channel and the
`blockTint` uniform, reserved since M4, finally carry something.

## Why it is not just "run the skylight engine again"

Block light and skylight differ in one rule that matters:

| | Skylight | Block light |
|---|---|---|
| Decay downward | none — falls at full strength | 1 per block, like every other direction |
| Source | the open sky above a column | individual emitting blocks |
| Time of day | multiplied by `skyTint` | multiplied by `blockTint`, which is constant |

The lossless downward rule models the sun, and applying it to a torch would make
every torch cast a bright shaft to bedrock.

`expected(from, d)` currently hard-codes the skylight rule. Rather than write a
second BFS — which is precisely how the removal bug that M3 fixed would come
back in a copy nobody is looking at — the existing add and remove walks are
parameterized over a small `lightKind` describing how to read, write and decay
one channel. One BFS, two configurations.

This is not speculative abstraction: there are two concrete light types today.

## Design decisions

**Storage.** `Chunk.BlockLight` alongside `Chunk.SkyLight`, both
`[16*16*256]uint8`. That is 128 KB of light per chunk. Nibble-packing the two
into one array would halve it and is what Minecraft does, but it obscures every
access for a saving that does not matter at a 3×3 chunk world. Deferred.

**Emission is a block property.** `Block.LightLevel uint8`, 0–15, loadable from
the YAML block definitions like every other property. Zero means non-emitting,
so the zero value is correct and existing definitions need no change.

**One vanilla emitter: `minae/glowstone`, level 15, a full cube.** A torch is
the more obvious choice but needs a thin cross-shaped model, which is model
work, not lighting work. Glowstone tests the whole pipeline as a plain cube.
Player inventory is built from the registry, so it appears automatically.

**Emitters are opaque.** A glowstone blocks light like any solid block; it emits
from its own cell. Transparent emitters are a later concern.

## Validation criteria

Set before implementation.

1. **Radial decay, all six directions.** A glowstone at level 15 in a sealed
   dark cave lights its six neighbours at 14, their neighbours at 13, and so on.
   **Specifically including straight down** — block light must *not* inherit
   skylight's lossless downward rule. A test asserting the cell below an emitter
   is 14, not 15, is the one that pins this apart from skylight.
2. **Independence from skylight.** Block light and skylight are stored and
   propagated separately. A cell can hold sky 0 and block 12 at once, and
   changing one must not perturb the other.
3. **Removal works.** Breaking an emitter darkens everything it was feeding,
   back to exactly the state a full recompute produces.
4. **Property test extended.** After any random sequence of edits that includes
   placing and breaking emitters, the incremental result must be
   byte-identical to a full recompute **for both channels**. This is the
   load-bearing criterion — it is what proves the parameterized BFS did not
   break skylight while gaining block light.
5. **Skylight is unchanged.** Every existing lighting test still passes without
   modification. If a skylight test needs editing to accommodate block light,
   that is a signal the refactor changed behaviour it should not have.
6. **Vertex packing.** `G = blockLight * 17`. `R`, `B` and `A` keep their M4
   meanings and values exactly.
7. **Time of day does not touch block light.** The shader multiplies block light
   by `blockTint`, which is constant. A cave lit only by glowstone must render
   identically at midnight and at noon.
8. **No leaking into unloaded chunks**, same as skylight.

## Steps

- [x] `Block.LightLevel` property, YAML loading, `blocks.Glowstone`.
- [x] `Chunk.BlockLight` storage and the world-level accessors.
- [x] Mesh builder writes `G = blockLight * 17`.
- [x] Parameterize the add and remove walks over a light kind.
- [x] Block light propagation, emitter seeding, emitter removal.
- [x] Extend the property test to cover both channels.
- [x] Raise `.coverage-floor` from 35.0 to 38.0.

## Verification

```bash
mise run ci
```

Plus manual: place a glowstone in a sealed cave, confirm it lights the space and
that the space does not change brightness as the day advances.

## Result

All eight criteria hold. `internal/world/lighting` is at 95.8% coverage, total
38.9%.

One BFS serves both channels. `lightKind` bundles how to read a channel, how to
write it, and how it decays; `skyExpected` keeps the lossless-downward rule and
`blockExpected` decays by one in every direction. The removal walk's comparison
against `expected` — rather than the textbook `nl < level` — was left structurally
untouched, so its reasoning carries over: for block light it simply degenerates
to the textbook case, since block light has no lossless direction.

Every existing skylight test passed **unmodified**, which was criterion 5.

### Two tests, two different jobs

Mutation testing showed the property test and the hand-written cases cover
genuinely different failures, and that neither is sufficient alone:

| Mutation | Property test | Radial-decay test |
|---|---|---|
| Incremental block-light removal disabled | **fails** — `block light ... incremental=2 full-recompute=0` | passes |
| Block light given skylight's lossless-downward rule | **passes** | **fails** — `GetBlockLight(8,19,8) = 15, want 14` |

The property test compares the incremental engine against a full recompute, so
it catches *inconsistency* between the two paths. It cannot catch a rule that is
uniformly wrong, because both paths then apply the same wrong rule and agree.
That is what the hand-written "directly below is 14, not 15" case is for.

Worth remembering when adding light types later: a property test of this shape
proves the incremental path matches the batch path, not that either is correct.

### A sharp edge in writing lighting tests

Shaft-based tests need a deep surface. With a shallow one, neighbouring columns
are open sky above the plug, so sealing a single cell mid-shaft seals nothing —
light arrives sideways from the adjacent open columns. Existing tests use
`Flat(60)` for this reason.

## Verification

```bash
mise run ci
```

**Manual, still outstanding:** place a glowstone in a sealed cave and confirm it
lights the space, and that the space does not change brightness as the day
advances. Criterion 7 is only checkable by eye — nothing automated observes the
fragment shader.
