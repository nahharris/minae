# M21 — Caves

**Status:** 📋 Planned
**Depends on:** [M20](M20-density-terrain.md)
**Inherits:** [world-generation constraints](../design/worldgen-constraints.md)

## Objective

Underground space: caverns, tunnels and the occasional entrance from the
surface, carved as subtractions from M20's density field.

## Design decisions

### Noise caves, not a carving pass

Minecraft's post-1.18 caves are noise fields combined into the density function
rather than tunnels dug afterwards — "cheese" caves (large open pockets),
"spaghetti" caves (long winding tunnels) and "noodle" caves (thin, squiggly
passages) ([Cave](https://minecraft.wiki/w/Cave)).

Carving as density subtraction is what makes caves cheap to get right here.
Because the result is still one field, caves inherit M20's purity, seam
continuity and determinism for free. A separate carving pass — the classic
"worm" approach that walks a tunnel through the world — is exactly the kind of
generation that crosses chunk boundaries and depends on order, which
constraint 1 forbids.

Two types to start: cheese for caverns and spaghetti for the tunnels between
them. Noodle caves are a later refinement if the underground feels sparse.

### Entrances are allowed and rare

A cave that never reaches the surface is invisible to a player who does not
dig. A world riddled with holes reads as damaged. Caves are attenuated near the
surface so that breaching it is possible but uncommon, and that rate is
measured, not assumed.

### Caves ignore surface biomes, for now

The sketch asked whether caves get their own biome axis. Not yet: caves are
carved from density everywhere, and every biome's underground is the same.
Underground biomes are a real design question, and answering it well needs
caves that already exist and have been walked through.

## Risks worth naming now

**Features over holes.** M18's trees root on `SurfaceHeight`. A tree rooted on a
thin roof over a cavern, or across a cave entrance, is a floating tree. Feature
placement must check that its root has solid support.

**Spawning into a cave.** Spawn uses `SurfaceHeight`, which is the highest
solid cell and cannot be inside a cave — but the spawn *column* must also have
headroom, and a player spawned on a thin crust above a cavern will fall through
the first hole they dig. Worth one test.

**Lighting.** Caves are mostly unlit, which is cheap. Cave *entrances* are not:
skylight pours in and propagates down and sideways into a large connected
volume. That is the case to benchmark.

## Validation criteria

1. **Caves are density subtractions** — deterministic, load-order independent,
   continuous across seams, distinct across chunks.
2. **Caves exist, at a bounded volume fraction.** The range lesson: the fraction
   of underground cells that are air falls inside a declared band.
3. **No isolated one-block voids.** Tiny disconnected air pockets are the cave
   equivalent of M20's floating debris. Assert a minimum connected size.
4. **Surface breaches are rare** — measured against a declared bound.
5. **Features never root over air.**
6. **Lighting equivalence holds with a cave entrance** that crosses a chunk seam,
   so skylight has to flow down the entrance and sideways into two chunks.
7. **Costs reported**, with the cave-entrance case benchmarked explicitly.

## Explicitly out of scope

Underground biomes, aquifers and lava, ores, dungeons and other structures, and
noodle caves.
