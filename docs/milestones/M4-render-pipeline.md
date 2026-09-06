# M4 — Get light onto the screen

**Status:** 📋 Planned
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

Make the light that M3 computes actually reach a pixel, and make the day cycle
read as realistic colour.

## What is wrong today

The mesh builder encodes skylight into the vertex **alpha** channel and leaves
RGB at 255. The fragment shader then computes `texelColor * fragColor`.
Multiplying alpha by alpha never touches RGB, so the entire skylight
calculation is discarded before it reaches a pixel.

The only surviving term is a directional diffuse, and it is also wrong.
`TimeOfDay.GetLightingState` builds `lightDir` as `(cos(h·2π), sin(h·2π), 0.2)`,
which at noon (`h = 0.5`) is `(-1, 0, 0.2)` — horizontal. Top faces therefore
receive `diff ≈ 0` at every hour of the day; the sun never shines downward. The
constant `Z = 0.2` tilt is what leaves the −Z faces responding only to ambient.

`getStateFromTime` also has a dead zone for `hour ∈ [0, 0.2)`: no day state
matches, it falls through to `nightState`, whose `NextState` is `nil`, so
colour snaps instead of interpolating.

## Target shape

**Vertex payload:** `R = skylight × 17` (0–15 mapped onto 0–255),
`G = blocklight × 17` (always 0 until torches exist), `B = 255` (ambient
occlusion channel, reserved but unimplemented), `A = 255` (alpha freed for real
transparency later).

**Fragment shader:**

```glsl
vec3 sky   = skyTint   * fragColor.r;
vec3 block = blockTint * fragColor.g;
vec3 light = max(sky, block) * faceBias(normal) * fragColor.b;
finalColor = vec4(texel.rgb * max(light, minAmbient), texel.a * fragColor.a);
```

Face bias lives in the shader rather than baked into vertices, so it stays
tunable without re-meshing: top 1.00, ±X 0.80, ±Z 0.60, bottom 0.50. This
constant per-face step supplies the sense of depth and directly replaces the
broken N·L term.

## Steps

- [ ] Change the mesh builder to pack light into `R`, with `G = 0`, `B = 255`,
      `A = 255`.
- [ ] Rewrite `fragment.glsl` per the shape above.
- [ ] Add `faceBias` derived from the fragment normal.
- [ ] Drop the per-vertex `transpose(inverse(matModel))` from `vertex.glsl`.
      Chunk transforms are pure translation, so it is wasted work every frame.
- [ ] Replace the `lightDir` / `lightColor` / `ambientColor` uniforms with
      `skyTint`, `blockTint` and `minAmbient`.
- [ ] Make the day-state ring circular (`night → dawn`) to eliminate the dead
      zone at `hour ∈ [0, 0.2)`.
- [ ] Interpolate in linear float space rather than sRGB `uint8`, which is why
      the current ramps read muddy.
- [ ] Delete `lightDir` and its plumbing through `SceneRenderer`.
- [ ] Raise `.coverage-floor`.

## Day cycle keyframes

Tunable live against the running game; these are a starting point.

| hour | phase    | skyTint                        |
|------|----------|--------------------------------|
| 0.00 | midnight | deep blue `(0.16, 0.18, 0.32)` |
| 0.27 | sunrise  | orange `(1.00, 0.60, 0.35)`    |
| 0.50 | noon     | warm white `(1.00, 0.98, 0.92)`|
| 0.76 | sunset   | orange-red `(1.00, 0.55, 0.30)`|
| 0.82 | dusk     | violet `(0.40, 0.30, 0.40)`    |

## Tests

- [ ] Vertex colour packing: a known light level produces the expected `R`, and
      `G`/`B`/`A` hold their reserved values.
- [ ] Face bias is *not* baked into vertex colour — two faces of the same block
      at the same light level get identical vertex colours.
- [ ] Day cycle is continuous: sampling `skyTint` across the whole cycle,
      including the wrap at 1.0 → 0.0, shows no discontinuity above a small
      epsilon.
- [ ] `getStateFromTime` returns a state with a non-nil successor for every
      hour in `[0, 1)`.

## Manual verification

Shaders cannot be unit tested, so this checklist runs before the milestone is
marked done:

- [ ] Stand in an enclosed cave with no opening — it is genuinely dark, and it
      stays dark as the day cycle advances.
- [ ] Dig a one-block shaft to the surface — the column below is bright, and
      light falls off with distance sideways from the shaft.
- [ ] Break a ceiling block — the space below brightens immediately.
- [ ] Place a block over a lit area — it darkens immediately.
- [ ] Do the same on a chunk boundary — the neighbouring chunk updates too.
- [ ] Watch a full day cycle — deep blue at night, orange through sunrise,
      warm white at noon, orange-red at sunset, with no snap at any point.
- [ ] All six faces of a block are distinguishable: the top is brightest, the
      bottom darkest, and no face is unlit.
