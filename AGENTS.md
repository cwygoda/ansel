# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build -o ansel .      # Build binary
go test ./...            # Run all tests
go test ./cmd/...        # Run CLI tests only
go test ./internal/...   # Run library tests only
go test -run TestName    # Run specific test
```

External tools some commands shell out to: `vips`/`vipsheader` (libvips), `gphoto2`
(camera import), `exiftool` (cull, geolocate).

## Architecture

Ansel is a CLI for photographers: it resizes and frames images, imports from USB
cameras, assists culling, and publishes to a CDN. It uses Cobra for CLI handling.

### Package Structure

- **`cmd/`** — Cobra commands, and the composition root where concrete adapters are
  constructed and injected
  - `root.go` — Root command setup
  - `process.go` — Resize/frame with size presets, frame options, fit modes
  - `camera.go` — USB camera detection and import, plus the launchd agent
  - `cull.go`, `cull_report.go` — Culling pipeline and its output rendering
  - `geolocate.go`, `geolocate_report.go` — GPS track matching and its output rendering
  - `ig.go` — Instagram posting (undocumented in README)
  - `publish.go` — CloudFormation-backed static site publishing

- **`internal/image/`** — libvips-backed image library (`github.com/davidbyttow/govips/v2`)
  - `vips.go` — `VipsImage` wrapper: `InitVips`/`ShutdownVips`, `LoadVips`,
    `ResizeToFit`, `AddFrame`, `AddLabel`, `SaveJPEG`/`Save`. Magic Kernel Sharp 2021 is
    libvips kernel `7` (`KernelMKS2021`), not a Go implementation.
  - `common.go` — `Filter` type and parsing, `ParseColor`
  - `metadata.go` — `ReadIPTCHeadline`, DXO `.dop` sidecar reading
  - `analysis.go` — `LoadVipsBuffer`, `Grayscale` for the cull pipeline

- **`internal/camera/`**, **`internal/cull/`**, **`internal/geolocate/`** — hexagonal,
  see below
- **`internal/config/`** — `~/.ansel/config.toml` access, `ExpandPath`
- **`internal/publish/`**, **`internal/instagram/`**, **`internal/tunnel/`**, **`internal/nanoid/`**

### Hexagonal Structure (required for new commands)

`internal/camera/`, `internal/cull/` and `internal/geolocate/` all use ports and adapters.
**New commands must follow the same layout:**

```text
internal/<feature>/
├── domain/      pure types, stdlib imports only
├── ports/       secondary port interfaces, imports domain only
├── application/ orchestrator holding ports as interface-typed fields; own TOML config
└── adapters/    one package per external dependency
```

Rules:

- Dependencies point inward: `adapters → ports → domain`, `application → ports → domain`.
- `application` never imports an adapter.
- `cmd/` is the only place concrete adapters are constructed.
- A third-party client type never appears in `domain` or `ports`. No `govips` outside
  `internal/image`, no `modernc.org/sqlite` outside `internal/cull/adapters/sqlite`.

Each feature loads its own config section (`[camera_import]`, `[cull]`, `[geolocate]`)
with a defaults-then-overlay merge, treating a missing file as "use defaults". Do **not**
route through `config.Load()` — its `Validate()` requires Instagram credentials.

## Key Concepts

**Linear Light Resizing**: Images are converted to linear color space before resizing,
then back. This prevents color shifts during interpolation. Pipeline:
`Load → ToLinear → Resize → ToSRGB → Save`, delegated to libvips.

**Fit Modes**:
- `expand` — Output is exact target size, image centered with frame filling gaps
- `wrap` — Frame wraps around resized image, output size varies

**Output Naming**: Processed files get `_v0`, `_v1`, etc. via `generateOutputPath()`.

**Culling**: Analyzers emit raw observations; policy code normalizes and interprets them.
That split is deliberate — changing a threshold recomputes tags without re-measuring
pixels. Ranking is group-relative and carries its reasoning. Sharpness is percentile-
ranked only against comparable frames (embedded RAW previews are scored separately from
rendered JPEGs, since they differ by more than an order of magnitude). Originals are
never modified; SQLite is authoritative and XMP sidecars are a projection.

Design notes live in `notes/culling/architecture.md` (gitignored).

**Geolocation**: photographs are matched to GPS tracks by time. Position between recorded
points is spherical (slerp), not linear — devices drop to smart recording on straight
sections and linear coordinates drift off-route over those gaps. Tracks are searched
individually and never concatenated, so a frame between two recordings is never placed on
the line joining them. `--max-gap` and `--buffer` bound how far inference may reach, and
every fix carries the method and gap that produced it.

The timezone ladder is the subtle part. Cameras write unzoned wall clocks; tracks record
UTC. The zone comes from EXIF `OffsetTimeOriginal`, else the offset the FIT file states
via its `local_timestamp`, else `--tz`/`--utc-offset` — and otherwise the frame is left
unlocated. It is **never** resolved against the machine's local zone.

Note `internal/cull/adapters/exiftool` *does* parse capture times as `time.Local`. That is
tolerable for grouping bursts by their spacing and wrong for geolocation, which is why
`internal/geolocate` has its own reader returning `domain.CaptureClock` with the reading
and its offset kept apart. The two readers are deliberately not shared.

`ansel geolocate --in-place` is the **only** code path in this repository that modifies a
photograph. It is opt-in, requires `--write`, and everything else — including geolocate's
own default — still writes sidecars only. Do not widen that exception casually.

Geolocate writes through exiftool for sidecars as well as in-place, rather than rendering
XMP like cull does, because it targets the same `<basename>.xmp` cull writes and must not
destroy the ratings in it. The two targets need genuinely different tags: EXIF splits
magnitude from hemisphere and date from time and has `OffsetTimeOriginal`; XMP folds the
hemisphere into the value, keeps one `GPSDateTime`, and carries the zone inside the
timestamp. Unqualified tag names let exiftool pick a group, which for a JPEG silently
means writing XMP instead of EXIF — so every tag is written group-qualified.

## Conventions

- Errors wrapped with `fmt.Errorf("...: %w", err)`; lowercase, actionable messages.
- Output via `fmt.Fprintf(os.Stdout, ...)` / `os.Stderr` with two-space indent nesting.
  No logger. `ANSEL_LOG_LEVEL` gates vips and metadata debug output.
- Commands use `RunE`, package-level `var xxxCmd`, and register in `init()`.
- Batch operations are best-effort: report the failure, continue, return nil.
- Tests are stdlib table-driven with `t.Run` subtests. No testify. CLI tests target pure
  helper functions rather than executing Cobra commands.
