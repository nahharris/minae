# M4 — Get light onto the screen

**Status:** ✅ Done
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

Make the light that M3 computes actually reach a pixel, and make the day cycle
read as realistic colour.

## What was wrong

**Light travelled in the alpha channel.** The mesh builder packed skylight into
vertex alpha and left RGB white; the fragment shader ended with
`finalColor = texel * fragColor * lighting`, so that alpha landed in
`finalColor.a`. raylib blends by default, so **a dark block rendered transparent
rather than dark** — the sky showed through it, which reads as brighter than a
lit block, and a cave got visibly brighter the deeper it went.

This bug predates M3, but M3 is what made it obvious. Before M3 the engine could
not darken anything and light did not cross chunk seams, so nearly every cell
sat at 15 and alpha was 255 almost everywhere. M3 started computing correct
darkness, and correct darkness poured into a broken channel.

**The sun never shone downward.** `lightDir` was `(cos(h·2π), sin(h·2π), 0.2)`.
At hour 0.25 that is `(0, 1, 0.2)`, so `dot(normal, -lightDir)` on a top face is
about −0.98, clamped to zero. At noon it is horizontal. There was no hour at
which top faces received any diffuse light, so the world sat at bare ambient —
about 31% brightness — and the constant `Z = 0.2` tilt is what left the −Z faces
responding to nothing but ambient.

**The day cycle snapped.** `getStateFromTime` had no state matching
`hour ∈ [0, 0.2)` and fell through to a terminal night state with a nil
successor, so colour jumped instead of easing.

## Steps

- [x] Pack light into vertex colour: `R = skylight × 17`, `G = 0` (block light,
      reserved), `B = 255` (ambient occlusion, reserved), `A = 255` (opacity,
      constant). Alpha is freed for real transparency later.
- [x] Rewrite `fragment.glsl` to read the packed payload from RGB.
- [x] Add `faceBias` in the shader — top 1.00, ±X 0.80, ±Z 0.60, bottom 0.50.
- [x] Drop the per-vertex `transpose(inverse(matModel))` from `vertex.glsl`.
- [x] Replace the `lightDir` / `lightColor` / `ambientColor` uniforms with
      `skyTint`, `blockTint` and `minAmbient`.
- [x] Make the day-state ring circular; interpolate in linear float space;
      delete `lightDir` and the dead `SunIntensity`.
- [x] Raise `.coverage-floor` from 29.0 to 31.0.
- [x] **Manual visual verification** — signed off by Hannah on 2026-09-05.

## The model

Baked per-voxel skylight is the source of truth. Time of day is a single
`skyTint` uniform multiplying it, so the day cycle costs **zero re-meshing** and
enclosed spaces correctly ignore it.

```glsl
vec3 light = max(skyTint * sky, blockTint * block);
light *= faceBias(normalize(fragNormal)) * ao;
light = max(light, vec3(minAmbient));
finalColor = vec4(texelColor.rgb * light, texelColor.a * fragColor.a);
```

Face bias lives in the shader rather than baked into vertices, so it stays
tunable without regenerating every chunk mesh. On axis-aligned cubes that
constant per-orientation step is what supplies the sense of depth, and it is the
direct replacement for the broken N·L term.

`minAmbient` (0.035) keeps a sealed cave dim rather than pure black. It is a
playability floor, not a physical term.

## Testing a thing that cannot be tested

Shaders have no unit tests, so three narrower guards were added instead.

**`shader_uniforms_test.go`** — uniforms bind by string. If a name drifts
between Go and GLSL, `GetShaderLocation` returns −1 and `SetShaderValue` is a
silent no-op, leaving the world lit by whatever the uniform defaults to. Nothing
in build, vet, lint or tests would notice. The names are now constants, checked
in both directions against the real GLSL source. The same file asserts that
`fragColor.a` never reaches the RGB term — pinning the alpha bug shut — and that
`inverse(` stays out of the vertex shader.

**`shader_compile_gpu_test.go`** — behind a `gpu` build tag, because it needs a
display and CI runners have none. It opens a hidden 64×64 window, compiles the
real shaders, and asserts every uniform resolves. A GLSL error otherwise makes
raylib log and quietly substitute its own default shader, so the program runs
and merely looks wrong.

```bash
mise run test:shaders
```

**Mutation testing** confirmed each guard fails when it should:

| Mutation | Caught by |
|---|---|
| Day-state ring not circular | continuity test — `skyTint.R jumped by 0.25 at hour 0.820` |
| Missing semicolon in `fragment.glsl` | GPU probe — uniforms fail to resolve |
| Comment mentioning `inverse(` | the `inverse(` guard, which is why it strips comments first |

## Manual verification checklist

The final verdict on this milestone was visual, and Hannah confirmed on
2026-09-05 that the world renders correctly -- specifically that the inverted
brightness is gone.

The list below is deliberately left unticked. It is not a record of that
sign-off; it is the checklist to re-run whenever a shader, the vertex packing or
the day cycle changes, since none of it can be covered by an automated test.

- [ ] An enclosed cave with no opening is genuinely dark, and stays dark as the
      day cycle advances.
- [ ] A one-block shaft to the surface leaves the column below bright, with
      light falling off sideways from the shaft.
- [ ] Breaking a ceiling block brightens the space below immediately.
- [ ] Placing a block over a lit area darkens it immediately.
- [ ] The same, done on a chunk boundary — the neighbouring chunk updates too.
- [ ] Nothing is see-through. Shadowed faces are *dark*, not transparent.
- [ ] A full day cycle: deep blue at night, orange through sunrise, warm white
      at noon, orange-red at sunset, with no snap at any point.
- [ ] All six faces of a block are distinguishable, top brightest and bottom
      darkest, with no face unlit.

## Verification

```bash
mise run ci            # build, vet, race tests, coverage floor, lint
mise run test:shaders  # GLSL compiles on real hardware (needs a display)
```

Total coverage 31.6%, floor raised to 31.0.
