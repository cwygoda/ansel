# AGENTS.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this
repository.

**Read [CONTRIBUTING.md](CONTRIBUTING.md) first.** Setup, the task runner, the golden
rules this codebase holds itself to, code conventions and testing style all live there and
are not repeated here. This file covers what is left: where things are, and why the
non-obvious parts are the way they are.

Quick reference: `task build`, `task test`, `task lint`, `task check`. Narrow a test run
with `task test -- -run TestName ./internal/image/`.

## Architecture

Ansel is a CLI for photographers: it imports from USB cameras, assists culling, places
photographs on GPS tracks, resizes and frames images, and publishes to a CDN. It uses
Cobra for CLI handling.

### Package Structure

- **`cmd/`** — Cobra commands, and the composition root where concrete adapters are
  constructed and injected
  - `root.go` — Root command setup
  - `process.go` — Resize/frame with size presets, frame options, fit modes
  - `camera.go` — USB camera detection and import, plus the launchd agent
  - `cull.go`, `cull_report.go` — Culling pipeline and its output rendering
  - `geolocate.go`, `geolocate_report.go` — GPS track matching and its output rendering
  - `ig.go` — Instagram posting
  - `publish.go` — CloudFormation-backed static site publishing

- **`internal/image/`** — libvips-backed image library (`github.com/davidbyttow/govips/v2`)
  - `vips.go` — `VipsImage` wrapper: `InitVips`/`ShutdownVips`, `LoadVips`,
    `ResizeToFit`, `AddFrame`, `AddLabel`, `SaveJPEG`/`Save`. Magic Kernel Sharp 2021 is
    libvips kernel `7` (`KernelMKS2021`), not a Go implementation.
  - `common.go` — `Filter` type and parsing, `ParseColor`
  - `metadata.go` — `ReadIPTCHeadline`, DXO `.dop` sidecar reading
  - `analysis.go` — `LoadVipsBuffer`, `Grayscale` for the cull pipeline

- **`internal/camera/`**, **`internal/cull/`**, **`internal/geolocate/`** — hexagonal;
  the layout and its rules are golden rules 4-6 in CONTRIBUTING.md
- **`internal/exiftool/`** — the shared long-lived `-stay_open` session used by cull and
  geolocate
- **`internal/config/`** — `~/.ansel/config.toml` access, `ExpandPath`
- **`internal/publish/`**, **`internal/instagram/`**, **`internal/tunnel/`**, **`internal/nanoid/`**

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

Writing skips a sidecar that already carries a rating, not one that merely exists — a
sidecar holding only coordinates from `ansel geolocate` must still receive its rating, and
a rating of zero counts as absent.

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

Geolocate writes through exiftool for sidecars as well as in-place, rather than rendering
XMP like cull does, because it targets the same `<basename>.xmp` cull writes and must not
destroy the ratings in it. The two targets need genuinely different tags: EXIF splits
magnitude from hemisphere and date from time and has `OffsetTimeOriginal`; XMP folds the
hemisphere into the value, keeps one `GPSDateTime`, and carries the zone inside the
timestamp. Unqualified tag names let exiftool pick a group, which for a JPEG silently
means writing XMP instead of EXIF — so every tag is written group-qualified.
