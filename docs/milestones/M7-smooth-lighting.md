# M7 — Smooth lighting and ambient occlusion

**Status:** ✅ Done — pending visual confirmation

## Objective

Light varies smoothly across a face instead of being flat, and inside corners
darken. This is the change that makes voxel lighting stop looking like flat
coloured cubes.

## Why these are one milestone, not two

Both require the same surgery: the mesh builder currently computes **one** light
value per quad and hands it to `addQuad`, which writes it to all six vertices.
Smooth lighting needs **four** values, one per corner. Ambient occlusion also
needs four, computed from a different neighbourhood.

Doing them separately means performing that restructuring twice and reviewing
the vertex-emission path twice. They share the neighbour-sampling loop, so they
land together.

## Design

**Smooth lighting.** For each of a quad's four corners, average the light of the
four voxels touching that corner on the outward side of the face. Opaque
neighbours are excluded from the average rather than counted as zero, so a face
against a wall does not darken spuriously. If every sample is opaque, fall back
to the face's own outward cell.

The average is per channel — skylight and block light are averaged
independently, then packed into `R` and `G` as now.

**Ambient occlusion.** For each corner, examine the three outward neighbours
that meet there: two edge-adjacent and one diagonal. The classic rule:

```
if side1 and side2 are both solid -> 0        (fully occluded inside corner)
otherwise                        -> 3 - (side1 + side2 + corner)
```

giving four levels. Map them onto a brightness ramp in the `B` channel; `255`
stays "unoccluded", matching the constant M4 reserved.

**The diagonal artifact.** When AO values on a quad's two diagonally opposite
corners are asymmetric, the fixed triangulation produces a visible seam running
the wrong way. The standard fix is to flip which diagonal splits the quad when
`ao[0] + ao[2] < ao[1] + ao[3]`. Without it, corners look creased. This is the
detail most implementations miss on the first pass.

## Validation criteria

1. **A flat unoccluded surface stays flat.** All four corners of a quad in the
   middle of an open plain get identical vertex colours. Smooth lighting must
   not invent gradients where the light is uniform.
2. **A light gradient appears across a quad.** Near the mouth of a shaft, the
   four corners of a single face carry different `R` values, ordered so the
   corner nearest the light is brightest.
3. **Inside corners darken.** Where two solid blocks meet, the shared corner has
   the lowest AO value of the four. This must hold **at full light** — AO is
   geometric and independent of light level.
4. **AO is independent of light.** The same geometry at skylight 15 and at
   skylight 4 produces the same `B` values.
5. **Triangulation flips on asymmetric AO.** Construct a quad whose AO is
   asymmetric across one diagonal and assert the emitted triangle split uses the
   other diagonal. This is testable on the vertex order without rendering.
6. **Averaging excludes opaque neighbours.** A face flush against a wall is not
   darker than the same face in open air at the same light level.
7. **Channel invariants hold.** `A` is still a constant 255, and `G` still
   carries block light — smooth lighting must average it, not drop it.
8. **No regression** in the existing mesh tests beyond the expected change from
   per-face to per-vertex values, and each such change explained in review.

## Risks

Vertex count is unchanged, but per-vertex sampling multiplies the neighbour
lookups per quad by roughly four. If meshing time becomes noticeable at this
world size, that is a signal to cache the 3×3×3 neighbourhood per voxel rather
than to abandon the approach.

## Result

All eight criteria hold. `internal/gfx/mesh` is at 89.2% coverage; total 41.2%.

### How the flip is kept safe

`addQuad` computes positions, UVs and colours into per-corner arrays first,
then emits through a **single** `order [6]int` decided once. Position, normal,
UV and colour for one corner are appended in the same loop iteration, so there
is no way to flip one output without flipping the others. The failure mode this
guards against — every vertex silently carrying a neighbouring corner's UV and
light — renders as plausible-looking garbage rather than as an obvious break.

### A latent coupling found in review

The flip originally compared the **ramped** `B`-channel bytes rather than raw
occlusion levels. That is correct only while `aoRamp` is evenly spaced, which it
is today (`102, 153, 204, 255` is `102 + 51·level`), so every test passed. But
`aoRamp` is explicitly documented as tunable, and its comment claimed nothing
else depended on its spacing — which was false. Retuning it unevenly would have
silently changed which diagonal some quads split along, reintroducing the crease
the flip exists to remove.

The flip now compares raw levels, and a test pins the property by meshing the
same geometry under two very different ramps and asserting identical output.

That test needed mutation-testing of its own: the first ramp chosen for it,
`{0, 200, 210, 255}`, happened to agree with the level comparison for this
geometry, so the test passed against the byte-based implementation and proved
nothing. `{0, 5, 10, 255}` separates them. Picking an "obviously different"
value is not the same as picking a *discriminating* one.

### Existing tests that changed, and why

Five existing tests needed new setup. In every case the assertions are
unchanged; only the world being asserted about changed.

Before smooth lighting, one neighbouring cell determined a whole face, so a test
could light exactly one cell and expect a flat value. Now each corner averages a
2×2 patch, so lighting one cell of four leaves a genuine gradient. The tests now
light the full 3×3 tangent patch, which restores a uniform field and makes the
original flat expectation correct again.

`TestVertexColor_AllChannelsAcrossEveryVertex` additionally had to split into six
subtests with a fresh world each: a block's six face patches overlap at the
diagonals — the top face's patch and the right face's patch share the cell above
and to the right — so six distinct simultaneous values are no longer achievable.
Its per-vertex assertions, including `B == 255` and `A == 255`, are preserved
exactly.

### Performance

`internal/gfx/mesh` tests run in roughly the same time as before. Per-corner
sampling does about four times the neighbour lookups per quad, but the test suite
meshes single blocks rather than full chunks, so it does not exercise that. Worth
re-measuring at real chunk-meshing volume before drawing conclusions.

## Manual verification

Automated tests cover the vertex data, not the pixels.

- [ ] Faces near a shaft or cave mouth show a smooth gradient rather than flat
      bands.
- [ ] Inside corners where two blocks meet are visibly darker.
- [ ] No diagonal crease running the wrong way across a face — this is what the
      triangulation flip prevents.
- [ ] Textures are not skewed or misaligned on any face, which is what a
      desynchronised flip would produce.
- [ ] A glowstone-lit cave still looks correct: block light is averaged too, not
      dropped.

## Follow-up fix: the triangulation flip was inverted

Found by eye after M7 merged: with one block the corner shadow looked right, but
adding a second made a corner "vanish" — a hard bright wedge cutting through
where the shadow should have been darkest.

**The flip condition was backwards.** A quad must split along the diagonal
through its *darker* corners. Light is interpolated across each triangle
independently, so splitting along the brighter diagonal can leave one triangle
with every corner unoccluded. That triangle then renders at full brightness
right up to the edge it shares with the shaded half. Splitting through the
occluded corner instead puts it in both triangles, and the shadow radiates from
it.

The code flipped when the default diagonal was *darker* — precisely when it
should have kept it, and vice versa.

### Why the M7 criteria missed it

Criterion 5 asked that the flip keep each vertex's position, UV and colour
together. It never asked that the flip go the *right way*. Both properties
matter and only one was specified, so an implementation could satisfy every
stated criterion and still produce the artifact the feature exists to prevent.

Writing "the flip happens" as a criterion is not the same as writing "the flip
is correct".

### The invariant now pinned

`TestAO_NoFullyUnoccludedTriangleBesideAnOccludedCorner`: if any corner of a
quad is occluded, no emitted triangle may consist entirely of unoccluded
corners. That is a direct statement of the visual artifact, and it fails against
the inverted rule.

### Two tests had to stop depending on emission order

`TestSmoothLighting_TriangulationFlipKeepsCornersTogether` and
`TestSmoothLighting_InsideCornerDarkens` both indexed vertices by their position
in the emitted stream, which encodes the flip direction. Changing the rule broke
them for reasons unrelated to what they test, and the tempting fix — editing the
expected order — is exactly how a test gets bent to fit a bug.

Both now key off vertex *positions*: the first checks that each emitted vertex's
position, UV and colour belong to the same corner under either split, and the
second checks that the geometrically-identified inside corner is the darkest.
Which diagonal the quad splits along is now asserted in exactly one place.

## Known cosmetic limitation: banding

Light is interpolated between four per-corner values, each quantised to one of
16 levels, with AO quantised to four. On large untextured surfaces in a dark
palette this reads as visible steps rather than a smooth ramp. It is inherent to
per-vertex lighting at one-block resolution and is not a defect in the
implementation; textured blocks hide most of it.

Options, none scheduled: dither in the fragment shader (cheap, hides 8-bit
quantisation), or sample light at sub-block resolution (expensive, and a much
larger change).

## Follow-up: AO tuning against light sources and tight spaces

Two artifacts reported from screenshots after the flip fix. Both came from the
same place: ambient occlusion was being applied indiscriminately.

**A glowstone cast an occlusion shadow.** AO's notion of "solid" was simply "not
air", so an emitting block occluded like any other. The result was a dark halo
on precisely the surfaces the glowstone was illuminating — the lamp casting its
own shadow.

Fixed by separating two questions that had been conflated in one boolean:

- `transparent` — does light pass through this cell? Governs which cells join
  the smooth-lighting average. An emitter is **not** transparent; it is solid.
- `occludes` — does this cell cast ambient occlusion? An emitter does **not**.
  AO approximates how much surrounding geometry blocks incoming light, and a
  glowstone is not blocking light, it is light.

**A 2×2 pocket drew a bright cross on the floor.** Not a bug in the AO rule but
the standard rule behaving badly in enclosed space. A 2×2 floor is four quads
meeting at one shared centre corner. That centre touches no wall and is fully
unoccluded; every outer corner touches two walls, which the
`side1 && side2 → 0` rule forces to the darkest level. Four quads bright only at
the room's centre reads as a cross.

Two changes, per Hannah's call:

- AO now applies **in full to skylight and at half strength to block light**
  (`blockAOStrength` in the fragment shader). AO models occlusion of the distant
  sky, which is what skylight is; a torch one block away is not blocked by the
  same geometry in the same way. Removing AO from block light entirely would
  have erased the artifact but flattened every torch-lit cave, so it is halved
  rather than dropped.
- The ramp softened from `0.4 → 1.0` to `0.6 → 1.0`. With flat untextured
  colours the original contrast read as heavy. Worth revisiting once real
  textures land, in either direction.

### Tests

`TestAO_EmittingBlocksDoNotOcclude` carries a control subtest asserting that a
*stone* diagonal neighbour still occludes exactly one corner. Without it, the
emitter case would pass just as happily if AO stopped working altogether.

Two tests hard-coded ramp bytes (`102`, `204`) and broke when the contrast
changed. They now read `AORampForTest()` by level, so tuning the ramp no longer
requires editing tests — which is the point, since a test that must be edited
every time a tunable value is tuned trains you to edit tests without reading
them.

## Follow-up: ambient occlusion ignored block shape

Reported from screenshots: occlusion above and below a slab looked wrong.

`occludes` was a per-block boolean — "is this cell occupied by a non-emitting
block". A slab fills half its cell, so a half-height block cast exactly as much
occlusion as a full cube, in whichever direction you looked at it.

Measured before the fix, a floor's top face with a slab beside it:

| neighbour | before | correct |
|---|---|---|
| bottom slab beside a floor | shaded | shaded — it fills the half against the floor |
| **top slab beside a floor** | **shaded** | **not shaded — half a cell away** |
| **bottom slab beside a ceiling** | **shaded** | **not shaded — half a cell away** |
| top slab beside a ceiling | shaded | shaded |

Occlusion now asks the block's *shape*, not the cell's occupancy: the shared
corner is probed a quarter cell into each neighbour, and the neighbour's
collision boxes are tested for that point. A quarter cell resolves half-block
shapes unambiguously — probing from a cell's lower face lands at 0.25, inside a
bottom slab and clear of a top one. Shapes with features finer than a half cell
would need a finer probe or genuine area sampling; `cornerProbe` is where that
would be revisited.

This reuses the collision boxes added for the player, so a block's shape is
described once and both physics and lighting read the same description. Stairs
will get correct occlusion for free.

### The other report: corners much darker than the edge between them

Investigated and **not changed**, because it is correct.

At an inside corner the level goes 3 → 2 → 0, skipping 1. That is the
`side1 && side2 → 0` rule, and it is right: where two neighbours meet along an
edge, the diagonal cell touches the vertex at a single *point* and so
contributes no solid angle. The corner genuinely is fully occluded, whether or
not the diagonal is filled — which the scenario matrix now asserts both ways.

What makes it conspicuous is that the jump is two ramp steps wide at exactly the
place the eye is drawn to, on untextured flat colour. That is a contrast
question, and `aoRamp` is the knob: widening the ramp softens every step
including this one. Left alone pending real textures.

### A scenario matrix

`ao_scenarios_test.go` covers twelve corner and edge configurations — open
faces, single edges, diagonals, inside corners with the diagonal both open and
filled, a corridor, all four slab-against-face orientations, a slab against a
wall, and an emitter.

It asserts AO **levels**, not colour bytes. Levels are what the geometry
determines; the bytes are a contrast setting documented as tunable, and
asserting those would make all twelve fail the next time the ramp moves, for a
reason having nothing to do with corners.

It also locates the face by matching the emitted normal and checking the
vertices lie inside the block under test, rather than by index arithmetic —
face indices shift whenever a neighbour sorts earlier in the chunk's scan
order, which several of these scenarios do.

Mutation-tested: reverting occlusion to the shape-blind boolean fails the three
slab scenarios and nothing else, which is exactly the blast radius it should
have.
