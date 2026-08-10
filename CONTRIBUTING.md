# Contributing to Ansel

## Getting set up

[mise](https://mise.jdx.dev/) pins the toolchain and [Task](https://taskfile.dev/) runs
the jobs. On a fresh clone:

```bash
git clone https://github.com/cwygoda/ansel.git
cd ansel
mise trust        # mise refuses to read an untrusted config
mise install      # Go, golangci-lint and task, as pinned in mise.toml
task deps:check   # Reports any missing Homebrew dependency
task build
```

### Native dependencies

Three programs have no mise registry entry and come from Homebrew:

```bash
brew install vips exiftool gphoto2
```

- **libvips** is a build-time dependency, not just a runtime one: `govips` is cgo and
  links against it, so the project will not compile without the headers.
- **exiftool** is invoked by `cull` and `geolocate`, through a single long-lived
  `-stay_open` session (`internal/exiftool`) rather than one process per file.
- **gphoto2** backs `camera import`.

`task deps:check` reports which are missing and the formula that provides each.

### Tasks

| Task              | Does                                                       |
| ----------------- | ---------------------------------------------------------- |
| `task`            | List all tasks                                             |
| `task build`      | `go build -o ansel .`                                      |
| `task install`    | Build and install into `~/.local/bin` (set `INSTALL_DIR` to change) |
| `task test`       | `go test ./...`                                            |
| `task lint`       | `golangci-lint run`                                        |
| `task fmt`        | gofumpt + goimports, via `golangci-lint fmt`               |
| `task fmt:check`  | Fail if formatting has not been applied                    |
| `task tidy`       | `go mod tidy`                                              |
| `task check`      | `fmt:check`, then `lint`, then `test`                      |
| `task deps:check` | Verify the native tools mise cannot install                |

Anything after `--` replaces the default `go test` arguments:

```bash
task test -- ./cmd/...
task test -- -run TestParseColor ./internal/image/
```

Run `task check` before opening a pull request.

## Golden rules

These are the commitments the codebase makes to the photographer using it. Each one has
cost something to get right; none is a style preference.

**1. Photographs are never modified.** Ratings, labels and coordinates go to XMP sidecars
beside the original. `ansel geolocate --in-place` is the single exception in the entire
repository — it is opt-in, it additionally requires `--write`, and that exception does not
widen. If a feature seems to need to touch an original, it almost certainly needs a
sidecar instead.

**2. Nothing is written unless asked.** Commands that write default to a dry run that
reports *exactly* what a real run would do — the same counts, the same list. `--write` and
`--dry-run=true` together are rejected rather than silently resolved, because guessing
which one the user meant is how files get written by accident.

**3. Sidecars are updated, not replaced.** `cull` and `geolocate` target the same
`<basename>.xmp`. A cull must not destroy coordinates and a geolocate must not destroy
ratings, so writes merge. Only genuine user judgement is protected behind `--force`: an
existing rating, an existing coordinate. Everything else survives a write regardless.

**4. New commands follow the hexagonal layout.** `internal/camera/`, `internal/cull/` and
`internal/geolocate/` all use ports and adapters, and new features must too:

```text
internal/<feature>/
├── domain/      pure types, stdlib imports only
├── ports/       secondary port interfaces, imports domain only
├── application/ orchestrator holding ports as interface-typed fields; own TOML config
└── adapters/    one package per external dependency
```

Dependencies point inward — `adapters → ports → domain`, `application → ports → domain`.
`application` never imports an adapter. `cmd/` is the only place concrete adapters are
constructed.

**5. No third-party type reaches the core.** A vendor client type never appears in
`domain` or `ports`. Concretely: no `govips` outside `internal/image`, no
`modernc.org/sqlite` outside `internal/cull/adapters/sqlite`. This is what makes the
domain testable without a database or an image library.

**6. Each feature loads its own config section.** `[camera_import]`, `[cull]`,
`[geolocate]` — defaults first, then overlay, treating a missing file as "use defaults".
Do **not** route through `config.Load()`: its `Validate()` requires Instagram credentials
(`internal/config/config.go:83`) that have nothing to do with culling a shoot.

**7. Measurement and interpretation stay apart.** Cull's analyzers emit raw observations;
policy code normalizes and interprets them. That split is what lets a threshold change
recompute tags without re-measuring a single pixel, and it is why the SQLite database is
authoritative while the XMP sidecar is only a projection of it.

**8. Inference states its reasoning and admits when it cannot answer.** A geolocate fix
carries the method and the gap that produced it; a cull ranking carries why. When the data
does not support an answer — no timezone could be resolved, the track gap is too wide —
the frame is reported as unresolved rather than given a plausible-looking guess.

## Conventions

- **Errors** wrapped with `fmt.Errorf("...: %w", err)`. Lowercase, actionable, with
  context: `failed to fetch user id=42: ...`.
- **Output** via `fmt.Fprintf(os.Stdout, ...)` / `os.Stderr`, with two-space indent for
  nesting. There is no logger. `ANSEL_LOG_LEVEL` gates vips and metadata debug output.
- **Commands** use `RunE`, a package-level `var xxxCmd`, and register in `init()`.
- **Batch operations are best-effort**: report the failure, continue with the rest, return
  nil. One unreadable file does not abandon a 400-frame shoot.
- **Size**: files under ~500 LOC, split at natural seams.
- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/).

## Testing

Tests are stdlib table-driven with `t.Run` subtests. **No testify.** CLI tests target the
pure helper functions rather than executing Cobra commands, which keeps them fast and
keeps the interesting logic out of the command layer in the first place.

```bash
task test
task test -- -run TestParseColor ./internal/image/
```

Bug fixes should come with a regression test when one is meaningful.

### Test data

The test image (`testdata/input.jpg`) is
["Close-up of a Jumping Spider on a Leaf"](https://www.pexels.com/photo/close-up-of-a-jumping-spider-on-a-leaf-35243201/)
by Silvio Fotografias, sourced from Pexels.

`testdata/output/` is gitignored — tests write there freely.

## Known lint debt

`task lint` currently reports twelve pre-existing findings that predate the linter being
introduced. Fixing them is welcome as its own commit; fixing them inside an unrelated
change is not, and neither is adding more.

One of them is a real bug rather than a style nit and is worth taking first:
`internal/publish/s3.go:146` returns `objects, nil` on *any* pagination error. The comment
claims the bucket might not exist yet, but a throttle or a permissions failure is reported
as an empty bucket — which makes the next sync upload everything.
