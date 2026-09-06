# M8 — Gentle directional sun tint

**Status:** 📋 Planned — optional polish, and previously declined

## Objective

Let the sun's position be felt: faces pointing toward it warm slightly as the
day advances, without any face ever going dark because of where the sun is.

## Read this before starting

This was **option C** in the original design discussion, and it was explicitly
declined in favour of pure baked lighting. It is in the backlog because the
roadmap listed it, not because anything is wrong without it.

It is also the item most likely to undo M4's work if implemented carelessly. The
bug M4 fixed was a directional sun term that left top faces unlit at every hour.
Reintroducing a directional term is reintroducing that class of bug, so the
constraint below is not a nicety — it is the whole point.

**Consider skipping this.** Face bias already conveys shape, and a voxel world
with baked lighting does not obviously want a moving sun. If it is attempted and
looks worse, deleting it is the correct outcome, not a failure.

## Design

A sun direction derived from the time of day — correctly this time: at noon the
sun is overhead and light travels **downward**, so a top face receives the
maximum. The previous implementation had it horizontal at noon and pointing
straight up at sunrise.

The term is a narrow multiplier, not a light source:

```glsl
float sun = mix(sunFloor, 1.0, max(dot(n, -sunDir), 0.0));   // sunFloor ~0.85
```

applied **only** to the skylight component. A torch does not care where the sun
is.

## Validation criteria

1. **No face is ever darkened by more than `1 - sunFloor`.** For every hour of
   the day and all six face orientations, the sun term stays within
   `[sunFloor, 1.0]`. This is the criterion that prevents recreating M4's bug,
   and it should be tested across the whole cycle, not at a few sampled hours.
2. **Top faces peak at noon.** The sun contribution to a top face is maximal
   near hour 0.5 and minimal near midnight.
3. **Block light is unaffected.** A surface lit only by a torch renders
   identically at every hour.
4. **Continuity.** The sun direction is continuous across the day, including the
   wrap at 1.0 → 0.0 — the same property M4's keyframe ring had to satisfy.
5. **Below the horizon contributes nothing extra**, and never a negative value.
6. **Disabling it is one line.** Setting `sunFloor` to 1.0 must reproduce M4's
   output exactly, so the feature can be switched off without unpicking it.

Criterion 6 exists because criterion "it looks worse" is a real possible
outcome, and the cheapest response should be a one-line revert.
