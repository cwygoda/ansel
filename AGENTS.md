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
(camera import), `exiftool` (cull).

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
  - `ig.go` — Instagram posting (undocumented in README)
  - `publish.go` — CloudFormation-backed static site publishing

- **`internal/image/`** — libvips-backed image library (`github.com/davidbyttow/govips/v2`)
  - `vips.go` — `VipsImage` wrapper: `InitVips`/`ShutdownVips`, `LoadVips`,
    `ResizeToFit`, `AddFrame`, `AddLabel`, `SaveJPEG`/`Save`. Magic Kernel Sharp 2021 is
    libvips kernel `7` (`KernelMKS2021`), not a Go implementation.
  - `common.go` — `Filter` type and parsing, `ParseColor`
  - `metadata.go` — `ReadIPTCHeadline`, DXO `.dop` sidecar reading
  - `analysis.go` — `LoadVipsBuffer`, `Grayscale` for the cull pipeline

- **`internal/camera/`**, **`internal/cull/`** — hexagonal, see below
- **`internal/config/`** — `~/.ansel/config.toml` access, `ExpandPath`
- **`internal/publish/`**, **`internal/instagram/`**, **`internal/tunnel/`**, **`internal/nanoid/`**

### Hexagonal Structure (required for new commands)

`internal/camera/` and `internal/cull/` both use ports and adapters. **New commands must
follow the same layout:**

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

Each feature loads its own config section (`[camera_import]`, `[cull]`) with a
defaults-then-overlay merge, treating a missing file as "use defaults". Do **not** route
through `config.Load()` — its `Validate()` requires Instagram credentials.

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

## Conventions

- Errors wrapped with `fmt.Errorf("...: %w", err)`; lowercase, actionable messages.
- Output via `fmt.Fprintf(os.Stdout, ...)` / `os.Stderr` with two-space indent nesting.
  No logger. `ANSEL_LOG_LEVEL` gates vips and metadata debug output.
- Commands use `RunE`, package-level `var xxxCmd`, and register in `init()`.
- Batch operations are best-effort: report the failure, continue, return nil.
- Tests are stdlib table-driven with `t.Run` subtests. No testify. CLI tests target pure
  helper functions rather than executing Cobra commands.
