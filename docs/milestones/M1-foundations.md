# M1 — Verifiable foundations

**Status:** ✅ Done
**Design:** [2026-09-05 lighting and foundations](../superpowers/specs/2026-09-05-lighting-and-foundations-design.md)

## Goal

No gameplay change. Everything here exists so that M2, M3 and M4 can be proven
rather than asserted. Before this milestone the project had no CI, no lint
configuration, and 12 test functions across ~4,100 lines, with the entire
lighting package at 0% coverage.

## Steps

- [x] Add `.golangci.yml` (schema v2) enabling the standard linter set plus
      `errorlint`, with `gofmt` and `goimports` as formatters.
- [x] Pin `golangci-lint` in `mise.toml`. It was `latest`, which resolved to
      2.1.6 — built with Go 1.24 and therefore unable to lint a Go 1.25.4
      module at all. Now pinned to 2.13.2.
- [x] Fix the 14 pre-existing lint failures (see *Issues found* below).
- [x] Add `internal/testutil`: a world builder composing scenarios from `Fill`
      and `Clear`, plus `Flat` for terrain, with its own tests.
- [x] Export `world.ChunkAndLocal` so test helpers use the same coordinate
      conversion as production code, and cover it — including negative
      coordinates and a round-trip property test.
- [x] Add `blocks.ResetToVanilla`.
- [x] Add `.coverage-floor` and `scripts/check-coverage.sh`.
- [x] Add `.github/workflows/ci.yml` with `build-test` and `lint` jobs.
- [x] Add `vet`, `cover` and `ci` mise tasks so the gates run locally.
- [x] Add `docs/ROADMAP.md` and `docs/milestones/`.
- [x] Rewrite `docs/PROJECT_STATUS.md` against the real `internal/` layout;
      delete `docs/REORGANIZATION.md`.

## Design decisions worth recording

**The test-world builder is fluent, not a set of named scenarios.** The design
document listed `FlatWorld`, `WithOverhang`, `WithShaft` and `WithCave`. Four
bespoke constructors turned out to be both longer and less flexible than one
builder with two primitives, so scenarios are composed at the call site
instead. `Clear` carves the cave, the shaft and the space under an overhang.

**The coverage floor is a hand-turned ratchet, not an automatic one.** An
automatically-updating floor ratifies whatever just happened. Raising it is a
deliberate edit, and `check-coverage.sh` prints a nudge once there are two
points of slack.

**CI resolves its toolchain through mise rather than `setup-go`.** `mise.toml`
is the project's stated single source of truth for tool versions; pinning
versions a second time in the workflow would let the two drift apart.

## Issues found and fixed

Found by turning the linter on for the first time:

| Issue | Where | Fix |
|---|---|---|
| Dead `chunkMeshHint` optimisation, declared in two packages and wired to nothing | `gfx/mesh/builder.go`, `world/chunk.go` | Deleted, along with `ensureCapacity` and the commented-out call site |
| Ineffectual initialisers for `stepX/Y/Z`, overwritten in every branch | `world/raycast.go` | Replaced with a plain `var` declaration |
| Redundant explicit type on `action` | `game/app.go` | Inferred from the right-hand side |
| Three unchecked `os.Setenv`/`os.Unsetenv` returns | `platform/config/config_test.go` | Replaced with `t.Setenv`, which also restores state automatically |
| Three unformatted files | `gfx/mesh/chunk.go`, `gfx/scene.go`, `platform/resources/loader.go` | `golangci-lint fmt` |

Found while writing the harness:

| Issue | Where | Fix |
|---|---|---|
| `TestBootstrapDataFolder_Default` created `~/.minae` in the developer's real home directory, and would do the same on a CI runner | `platform/config/config_test.go` | Added a `redirectHome` helper pointing `HOME` and `USERPROFILE` at a temp dir, and asserted the redirect took effect |
| `blocks.Reset()` leaves the package-level block variables holding `InvalidNumericID`, so anything stored through them silently becomes air | `blocks/registry.go` | Added `blocks.ResetToVanilla()`, which `testutil` calls |
| A stray `log.Println("Initializing blocks")` in `init()`, bypassing the project's logrus setup and printing on every test run | `blocks/vanilla.go` | Removed |
| `worldToLocal` handles negative coordinates via floor-division logic and had no test at all | `world/world.go` | Exported as `ChunkAndLocal` and covered |

## Verification

```bash
mise run ci
```

Runs build, vet, race-enabled tests with the coverage floor, and lint — the
same four gates CI enforces.

Result at the close of M1:

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -race ./...` — all packages pass
- `golangci-lint run` — 0 issues
- Total coverage — 21.3%, floor set to 21.0

**Not verified locally:** `.github/workflows/ci.yml` has never executed. GitHub
Actions cannot be run from here, so the workflow is unproven until the first
push. The commands it runs are exactly the ones verified above; what remains
untested is the YAML itself, the apt dependency list, and `mise-action` on a
clean runner.
