# Design note — parallel lighting

**Status:** 🔬 Researched, not scheduled
**Blocked on:** the constant-factor fix in
[M15's performance follow-up](../milestones/M15-chunk-streaming.md#follow-up-streaming-was-too-slow-to-play)

M14 deferred concurrent lighting as "a research problem, not a milestone". This
note is that research. It concludes that the problem is smaller than the
framing suggested — and that it is probably not the thing to do first.

## What the prior art says

[Starlight](https://github.com/PaperMC/Starlight) is the fastest known light
engine for this exact workload: roughly 35× vanilla Minecraft and 25× Phosphor,
turning ~220 seconds of light generation into ~7. It is **entirely
single-threaded**. Its
[technical notes](https://github.com/PaperMC/Starlight/blob/fabric/TECHNICAL_DETAILS.md)
credit three algorithmic changes:

- **Push, don't pull.** Vanilla recalculates each queued position by reading all
  six neighbours. Starlight propagates outward and checks only the block it came
  from — "this actually reduces our block get count by 6 times".
- **Carry the light value in the queue entry**, so a position already at least
  as bright can be discarded before any block read happens.
- **A plain FIFO** rather than a hashtable-backed queue.

Measured effect: 4,089 block reads against Paper's 49,062 for one block
placement; 20,447 light reads against 152,079 on removal.

### We already do all three

Worth checking rather than assuming, because the obvious reading of the above
is "adopt these ideas" and that would be wasted work. `propagateAdd` already
pushes outward from a node rather than recalculating from six neighbours;
`lightNode` already carries `Level`, and the light comparison already runs
before the block read; and `addQueue` is already a plain slice walked by index,
not a hashtable.

That is the genuinely useful finding here. The algorithm is the right shape, so
being 140× off Starlight is *entirely* constant factor — which is what the
profile says independently, with `mapaccess2` at 55%.

The one structural thing left is narrower: each neighbour visit resolves its
chunk four separate times (`HasChunkAt`, the light read, the block read, then
the write), so 24 resolutions per node. A per-operation pointer view turns each
of those from a hash lookup into index arithmetic, which is most of the win;
collapsing them into a single resolution per neighbour is a further refinement,
not the main event.

[ScalableLux](https://github.com/RelativityMC/ScalableLux/blob/ver/1.21.4/TECHNICAL_DETAILS.md),
despite the name, documents the same single-threaded approach.

The number that matters for sequencing: post-1.20, a chunk costs Starlight
**0.75 ms**. Ours costs **103 ms**. That is ~140× — and threading offers 8×.

## The result that makes threading tractable

Both light kinds cap at 15 and lose at least one level per horizontal step.
Chunks are 16 wide. So light crossing into a neighbour arrives at 14 and dies
two blocks short of that neighbour's far border.

**Light originating in a chunk cannot escape that chunk's 3×3 neighbourhood.**

Diagonals do not break it: light may go east into one neighbour and then north
into another, but that lands on a chunk still Chebyshev-adjacent to the origin.
Two steps in the *same* direction would need 16 blocks of travel from a value of
14, which is impossible.

## Consequence: a 9-colour schedule

If a chunk's write footprint is its 3×3 neighbourhood, two chunks conflict only
when their footprints overlap — that is, when they are within Chebyshev distance
2 of each other. Colouring by `(x mod 3, z mod 3)` gives nine classes in which
every pair is at least 3 apart, so **no two chunks in a class can touch the same
cell**.

Process one colour class at a time, every chunk in it in parallel:

- No locking. Disjoint footprints, so there is nothing to contend for.
- Deterministic. Order within a class cannot matter, because writes do not
  overlap; order across classes is fixed.
- The existing `SeedChunk` is reused unchanged. This schedules the algorithm
  rather than rewriting it, which matters for an algorithm M3 spent a milestone
  getting right.

Nine sequential rounds over ~32 chunks each, at view distance 8.

## The actual work, which is not the concurrency

`Engine` owns mutable scratch reused across calls — `addQueue`, `removeQueue`,
`reAdd`, `dirty`. Two goroutines calling into it corrupt each other silently.
Making lighting parallel therefore means giving each worker its own scratch and
merging the dirty sets afterwards.

That is mechanical, and it is the bulk of the change. The scheduling is the easy
part; the shared mutable state is the part that bites — the same lesson M13's
reverted `World` scratch buffer taught.

## Recommendation

Do the constant-factor work first and re-measure. Concretely, in order of
expected value:

1. Resolve chunk pointers once per operation instead of per cell (the profile
   says 55% of time is `mapaccess2`).
2. Skip enqueueing sky cells that cannot propagate anywhere.
3. Nothing further on address arithmetic, by decision. A `locate` that returns
   `(chunk, localX, localZ)` already collapses the four per-neighbour
   resolutions into one as a consequence of (1) — the expensive half, two
   `ChunkAndLocal` calls with their sign-correction branches plus the lookup,
   happens once, and `Chunk`'s existing local-coordinate accessors take it from
   there. That part is free and is the requirement on (1)'s design.

   Sharing the computed *index* as well was considered and rejected. It saves
   three `getBlockIndex` calls per neighbour — about a dozen shift-and-add
   instructions — against a visit count that (2) cuts by an order of magnitude,
   and it costs a raw byte index held in the BFS hot loop. That index couples
   lighting to `Chunk`'s memory layout, which blocks the nibble-packing of the
   light arrays deferred in M6 and flagged again in M15 (289 chunks is ~55 MB,
   half of it light). It also puts `x + z*16 + y*256` in a second place, which
   is precisely the bug class that yields subtly wrong light — an axis swap, or
   a block index used against a light array once the two layouts diverge. This
   project exists because the lighting was subtly wrong.

   If a re-profile ever points here, do it as index accessors on `Chunk`, tested
   there, so layout knowledge stays where it belongs.
4. Only then consider the 9-colour schedule, and with it the per-worker scratch
   below.

The scratch split is deliberately **not** part of the constant-factor work. It
buys nothing single-threaded, and rewriting shared state that has exactly one
caller is churn until the second caller exists.

If lighting lands near 1–3 ms per chunk, a per-frame time budget makes it
invisible and threading becomes optional polish. If it does not, this design is
ready and is a substantially smaller change than M14 assumed.

The order matters for one plain reason: parallelising code that is about to be
rewritten throws the work away, and an 8× win on top of a 100× win is worth far
less than the 100× itself.
