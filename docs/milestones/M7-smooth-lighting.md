# M7 — Smooth lighting and ambient occlusion

**Status:** 📋 Planned

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
