# Ansel

A command-line image processing tool for resizing and framing images for social media and print.

Named after [Ansel Adams](https://en.wikipedia.org/wiki/Ansel_Adams), the legendary photographer known for his meticulous attention to image quality.

## Features

- **Linear light resizing** using [Magic Kernel Sharp 2021](https://johncostella.com/magic/) — the gold-standard algorithm used by Facebook and Instagram
- **Automatic framing** with configurable colors and widths
- **Size presets** for Instagram, Facebook, Twitter/X, YouTube, LinkedIn, and print
- **High-quality JPEG output** with configurable quality
- **USB camera import** for Nikon Z6 III and Ricoh GR IIIx with local bookmarks and day folders
- **Culling assistance** that scores sharpness and exposure, groups bursts, and picks the strongest frame — without touching your RAW files
- **Geolocation** from GPS tracks, interpolating each frame's position along the route and correcting a drifted camera clock

## Installation

```bash
go install github.com/cwygoda/ansel@latest
```

Or build from source. [mise](https://mise.jdx.dev/) pins the toolchain and
[Task](https://taskfile.dev/) runs the jobs:

```bash
git clone https://github.com/cwygoda/ansel.git
cd ansel
mise install      # Go, golangci-lint and task, as pinned in mise.toml
task deps:check   # Reports any missing Homebrew dependency
task build
```

`task install` builds and drops the binary into `~/.local/bin`; set `INSTALL_DIR` to
put it somewhere else.

Some tools cannot come from mise and are installed with Homebrew:

```bash
brew install vips exiftool gphoto2
```

`vips` is required to build — `govips` is cgo and links against libvips. `exiftool` is
needed by `cull` and `geolocate`, `gphoto2` by `camera import`.

Without mise, a `go build -o ansel .` against Go 1.25 or newer works the same.

## Usage

```bash
ansel process --size SIZE [flags] <input>...
```

Output files are created next to the input with a version suffix:
- `photo.jpg` → `photo_v0.jpg`
- `photo_v0.jpg` → `photo_v1.jpg`

### Examples

```bash
# Create an Instagram post with default 5% white frame
ansel process --size ig-post photo.jpg

# Process multiple images
ansel process --size ig-story --color black *.jpg

# Custom size with 3% gray frame
ansel process --size 1920x1080 --color gray --frame 3 photo.jpg

# Wrap mode (frame wraps around image, output size varies)
ansel process --size 800x600 --fit wrap photo.jpg

# Use a different resize filter
ansel process --size ig-post --filter lanczos photo.jpg

# Output to a specific directory
ansel process --size ig-post -o processed/ *.jpg
```

### Flags

| Flag           | Default   | Description                                                    |
|----------------|-----------|----------------------------------------------------------------|
| `--size`       | required  | Output size: `WxH`, `W,H`, or preset name                      |
| `-o, --outdir` |           | Output directory (created if needed)                           |
| `--filter`     | `mks2021` | Resize filter: `mks2021`, `lanczos`, `catmull-rom`, `bilinear` |
| `--fit`        | `expand`  | Fit mode: `expand` or `wrap`                                   |
| `--frame`      | `5`       | Frame width as percentage of shorter side                      |
| `--color`      | `#fff`    | Frame color (hex or named color)                               |
| `--quality`    | `92`      | JPEG output quality (1-100)                                    |

### Size Presets

| Preset         | Dimensions | Platform                 |
|----------------|------------|--------------------------|
| `ig-post`      | 1080×1080  | Instagram square post    |
| `ig-portrait`  | 1080×1350  | Instagram portrait post  |
| `ig-landscape` | 1080×566   | Instagram landscape post |
| `ig-story`     | 1080×1920  | Instagram story/reel     |
| `ig-reel`      | 1080×1920  | Instagram reel           |
| `fb-post`      | 1200×630   | Facebook post            |
| `fb-cover`     | 820×312    | Facebook cover           |
| `x-post`       | 1200×675   | Twitter/X post           |
| `x-header`     | 1500×500   | Twitter/X header         |
| `yt-thumb`     | 1280×720   | YouTube thumbnail        |
| `li-post`      | 1200×627   | LinkedIn post            |
| `li-cover`     | 1584×396   | LinkedIn cover           |
| `4x6`          | 1800×1200  | 4×6 print (300 DPI)      |
| `5x7`          | 2100×1500  | 5×7 print (300 DPI)      |
| `8x10`         | 3000×2400  | 8×10 print (300 DPI)     |

### Fit Modes

- **`expand`** (default): Output is exactly the specified size. The image is resized to fit within the frame area and centered. The frame fills the remaining space.

- **`wrap`**: The frame wraps tightly around the resized image. The output size equals the image size plus the frame on all sides.

### Resize Filters

| Filter        | Description                                                      |
|---------------|------------------------------------------------------------------|
| `mks2021`     | Magic Kernel Sharp 2021 — highest quality, used by Facebook/Instagram |
| `lanczos`     | Lanczos3 — classic high-quality filter                           |
| `catmull-rom` | Catmull-Rom cubic — good balance of sharpness and smoothness     |
| `bilinear`    | Bilinear — fast but lower quality                                |

### Colors

Supports hex colors and named colors:

- Hex: `#fff`, `#ffffff`, `#ff0000`, `#rgba`
- Named: `white`, `black`, `gray`, `red`, `green`, `blue`, `yellow`, `orange`, `purple`, `pink`, `cyan`, `magenta`, `navy`, `teal`, `olive`, `maroon`, `silver`, `lime`

## Camera Import Command

Import new photos from supported USB cameras into day-by-day folders.

```bash
ansel camera detect
ansel camera import --dry-run
ansel camera import
```

Supported cameras:

- Nikon Z6 III
- Ricoh GR IIIx

By default, imports go to `~/Pictures/Ansel/Imports/YYYY-MM-DD/` and local bookmark state is stored in `~/.ansel/camera-import-state.json`.

Optional configuration in `~/.ansel/config.toml`:

```toml
[camera_import]
base_dir = "~/Pictures/Ansel/Imports"
state_path = "~/.ansel/camera-import-state.json"
backend = "gphoto2"
folder_layout = "2006-01-02"
include_extensions = [".jpg", ".jpeg", ".nef", ".dng", ".mov", ".mp4"]
```

On macOS, install a user-space LaunchAgent to run imports when a supported USB camera is attached:

```bash
ansel camera install-agent
ansel camera uninstall-agent
```

The initial backend shells out to `gphoto2`; install it with `brew install gphoto2`.

## Cull Command

Analyze a shoot, group near-identical frames, and rank each group so the strongest
frame is easy to find.

```bash
ansel cull [flags] [directory]
```

Requires `exiftool`; install it with `brew install exiftool`.

**Nothing is written unless you ask.** By default `cull` reports exactly what a real
run would do and changes nothing on disk:

```bash
ansel cull ~/Pictures/Ansel/Imports/2026-08-09
```

```text
2026-08-09: 412 images, 47 groups
  Analyzed: 412 (reused 0)

  g4a7b71-0007  6 frames  burst
    ★ DSC_1234.NEF             0.91  sharp, similar_group, best_in_group
      DSC_1235.NEF             0.74  sharp, similar_group
      DSC_1236.NEF             0.31  soft, similar_group, technical_warning

  Flagged: 12
    DSC_1301.NEF             soft, technical_warning

  Would write: 47 sidecars
    10 already exist and would be kept (use --force to replace)
```

Add `--write` to emit XMP sidecars beside the originals:

```bash
ansel cull --write ~/Pictures/Ansel/Imports/2026-08-09
```

### How It Works

Each photograph is measured through its **embedded preview**, so RAW files are read
but never demosaiced and never modified. Sharpness (variance of Laplacian and
Tenengrad), luminance distribution, and highlight/shadow clipping are recorded, along
with a perceptual hash.

Frames close together in capture time *and* visually alike are grouped. Candidates are
compared against the group's medoid rather than their nearest neighbour, so a run of
gradually changing frames is not chained into one oversized group.

Ranking is **group-relative**: the question answered is "which of these six frames is
strongest", not "is this objectively a good photograph". Every placement carries its
reasoning, visible with `--json`.

Two details worth knowing:

- **Sharpness is ranked against comparable frames only.** A camera's embedded RAW
  preview carries far less sharpening than a delivered JPEG — the same photograph can
  differ more than twentyfold — so RAW and rendered files are scored in separate
  populations.
- **Ranking low is not by itself a defect.** A quarter of any shoot sits in the bottom
  quarter. A frame is only called `soft` when it also has measurably less detail than a
  typical frame, so a uniformly sharp shoot reports nothing soft.

### Safety

- RAW and JPEG originals are never modified.
- Existing `.xmp` sidecars are **preserved**, since they may hold your own ratings.
  `--force` replaces them.
- Sidecars are written atomically, so an interrupted run leaves no partial file.
- Results are cached in SQLite; re-running skips unchanged files.

### Flags

| Flag             | Default | Description                                          |
| ---------------- | ------- | ---------------------------------------------------- |
| `--dry-run`      | `true`  | Report what would be written without writing it      |
| `--write`        | `false` | Write XMP sidecars (disables the default dry run)    |
| `--force`        | `false` | Replace XMP sidecars that already exist              |
| `--reanalyze`    | `false` | Ignore cached results and measure everything again   |
| `--json`         | `false` | Emit results as JSON, with rank scores and reasons   |
| `--db`           | config  | Analysis database path                               |
| `--exiftool`     | config  | Path to the exiftool binary                          |
| `--workers`      | CPUs    | Concurrent analysis workers                          |
| `--max-edge`     | `2048`  | Longest edge of the analysis preview, in pixels      |
| `--group-window` | `8`     | Seconds between frames to consider them related      |

`--write` and `--dry-run=true` together are rejected rather than silently resolved.

### Configuration

Every threshold lives in `~/.ansel/config.toml` and can be tuned without rebuilding.
Changing a threshold recomputes tags without re-measuring pixels.

```toml
[cull]
db_path = "~/.ansel/cull.db"
include_extensions = [".nef", ".jpg", ".jpeg"]
max_preview_edge = 2048

[cull.similarity]
window_seconds = 8
max_distance = 10
max_diameter = 14
burst_gap_seconds = 1.5

[cull.ranking.weights]
sharpness = 0.30
exposure = 0.15

[cull.ranking.penalties]
severe_blur = 0.30
severe_highlight_clipping = 0.10

[cull.policy]
sharp_above = 0.65
soft_below = 0.25
soft_relative_below = 0.5

[cull.sidecar]
rating_best = 5
label_best = "green"
label_warning = "red"
```

Analysis is stored in SQLite and is queryable directly, so any tag can be traced back
to the numbers behind it:

```bash
sqlite3 ~/.ansel/cull.db \
  "SELECT i.path, o.key, o.value FROM observations o JOIN images i ON i.id = o.image_id;"
```

### Scope

This is the technical stage: sharpness, exposure, clipping, near-duplicate grouping and
rule-based ranking. Face detection, eye-state analysis and learned image embeddings are
deliberately not included yet. Grouping therefore handles bursts and near-duplicates
well, and alternate compositions of the same subject less well.

## Geolocate Command

Place photographs on a recorded GPS track.

```bash
# Report only, writes nothing
ansel geolocate ~/Pictures/shoot --track ride.fit.xz

# Search a configured directory for recordings covering the shoot
ansel geolocate ~/Pictures/shoot

# Write XMP sidecars
ansel geolocate ~/Pictures/shoot --track ride.fit.xz --write
```

Only xz-compressed Garmin FIT activity files are read today. Other formats are added as
adapters without touching the matching logic.

### How It Works

Each photograph's capture time is resolved against the track. A frame taken exactly on a
recorded point takes that position; one taken between points is interpolated along the
**great circle** joining them, rather than by averaging coordinates. Devices drop to
smart recording on straight sections — five-second gaps and longer are routine — and over
those gaps straight-line coordinates visibly leave the road, increasingly so away from
the equator. Elevation is interpolated linearly and only when both neighbouring points
carry it.

Two limits keep inference honest:

- `--max-gap` (default 2m) is the widest spacing that may be interpolated across. A
  device paused for an hour has not described where you went, and the frame is reported
  rather than guessed at.
- `--buffer` (default 5m) is how far outside a recording a frame may still be placed, at
  its first or last point — for the shots taken while unpacking at the trailhead.

Tracks are matched one at a time and never merged, so a photograph taken between the
morning ride and the evening one is never placed halfway along the line joining them.

### Timezones

Cameras usually write the local wall clock with no zone; tracks record UTC. The zone is
resolved in this order, and **never guessed from the machine you are sitting at**:

1. the photograph's own `OffsetTimeOriginal`, when the camera recorded one
2. the offset stated by the track itself — FIT files carry a local timestamp alongside
   the UTC one, which is what makes the common case need no flags at all
3. `--tz Europe/Berlin` or `--utc-offset +02:00`

If none of those answer, the frame is reported as unlocated rather than placed somewhere
plausible but wrong. Prefer `--tz` over `--utc-offset`: a named zone knows when summer
time started.

### Drift

`--drift` is how far the camera clock runs **ahead** of true time. A camera reading
20:30:54 when it was really 20:29:24 has drifted 90 seconds:

```bash
ansel geolocate ~/Pictures/shoot --track ride.fit.xz --drift 90s --write
```

A camera running slow takes a negative value (`--drift -90s`). The correction shifts the
position lookup and the photograph's own `DateTimeOriginal` and `CreateDate` by the same
amount, so place and time can never disagree. The corrected timestamp stays in the zone
the camera was set to. Without `--drift`, timestamps are left exactly as written.

### Safety

Nothing is written unless you ask for it; the default run reports exactly what a real one
would do. Positions go to XMP sidecars beside the originals, so **the photographs
themselves are not modified** — the same stance `cull` takes. Sidecars are updated rather
than replaced, so ratings and labels written by `ansel cull` survive.

`--in-place` opts out of that and embeds EXIF GPS into the photographs themselves. It
requires `--write`.

Coordinates already present are treated as user data and kept unless you pass `--force`.

### Flags

| Flag           | Description                                                      | Default |
| -------------- | ---------------------------------------------------------------- | ------- |
| `--track`      | Track file, glob or directory (repeatable)                       |         |
| `--drift`      | How far the camera clock runs ahead of true time                 | `0`     |
| `--max-gap`    | Largest track gap to interpolate across                          | `2m`    |
| `--buffer`     | How far outside a track photographs may still be placed          | `5m`    |
| `--tz`         | Camera timezone, e.g. `Europe/Berlin`                            |         |
| `--utc-offset` | Camera clock UTC offset, e.g. `+02:00`                           |         |
| `--tracks-dir` | Directory to search when no `--track` is given                   |         |
| `--dry-run`    | Report without writing                                           | `true`  |
| `--write`      | Write coordinates                                                | `false` |
| `--in-place`   | Embed into the photographs instead of sidecars                   | `false` |
| `--force`      | Replace coordinates already present                              | `false` |
| `--json`       | Emit results as JSON, including how each position was derived    | `false` |

### Configuration

```toml
[geolocate]
# Searched when no --track is given. Unset by default, which disables the search.
tracks_dir = "~/Documents/Activities"

max_gap_seconds = 120
buffer_seconds = 300

# Last resort only; the photograph and the track are both consulted first.
timezone = "Europe/Berlin"
```

Track files are matched by the date in their filename before any are opened, so pointing
`tracks_dir` at years of recordings stays fast.

Requires `exiftool` (`brew install exiftool`).

## Publish Command

Publish processed images to a CDN-backed subdomain on AWS.

```bash
ansel publish [flags]
```

### What It Creates

On first run, the publish command creates a CloudFormation stack with:

- **S3 bucket** for content storage
- **CloudFront distribution** with Origin Access Control (OAC)
- **ACM certificate** with automatic DNS validation
- **Route53 subdomain record** pointing to CloudFront

Subsequent runs update the existing site and invalidate the CloudFront cache.

### Examples

```bash
# Publish ./build directory (default)
ansel publish

# Publish a specific directory
ansel publish --build-dir ./dist

# Use a specific subdomain
ansel publish --subdomain gallery

# Use a specific AWS profile
ansel publish --profile myprofile
```

### Flags

| Flag           | Default   | Description                                      |
|----------------|-----------|--------------------------------------------------|
| `--build-dir`  | `./build` | Directory containing files to upload             |
| `--subdomain`  |           | Subdomain name (randomly generated if not set)   |
| `--profile`    |           | AWS profile name                                 |
| `--region`     |           | AWS region (uses default from AWS config)        |

### Configuration

Settings are saved to `.ansel.toml` in the current directory:

```toml
[publish]
subdomain = "abc123"
hosted_zone_id = "Z1234567890ABC"
domain_name = "example.com"
```

On first run, if no subdomain is specified, a random one is generated and saved.

### AWS Credentials

Requires AWS credentials with permissions for CloudFormation, S3, CloudFront, ACM, and Route53. See [`publish-iam-policy.json`](publish-iam-policy.json) for the required IAM policy.

Configure credentials via:

- AWS CLI profile (`~/.aws/credentials`)
- Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)
- IAM role (when running on EC2/ECS)

## Environment Variables

| Variable          | Default | Description                                      |
|-------------------|---------|--------------------------------------------------|
| `ANSEL_LOG_LEVEL` | `error` | Log level: `error`, `warning`, `info`, `debug`   |

Set to `info` or `debug` for verbose libvips output (useful for debugging).

## Why Linear Light Resizing?

Standard image resizing operates in sRGB color space, which is non-linear. This causes colors to shift during resizing — dark areas become too dark, and bright areas lose detail.

Linear light resizing converts the image to linear color space before resizing, then converts back to sRGB. This produces more accurate colors and better detail preservation, especially in high-contrast areas.

The Magic Kernel Sharp 2021 algorithm combines this with optimized sharpening to produce results that are visibly superior to traditional methods.

## Testing

```bash
task test    # go test ./...
task lint    # golangci-lint run
task check   # formatting, lint and tests together
```

### Test Data

The test image (`testdata/input.jpg`) is ["Close-up of a Jumping Spider on a Leaf"](https://www.pexels.com/photo/close-up-of-a-jumping-spider-on-a-leaf-35243201/) by Silvio Fotografias, sourced from Pexels.

## License

MIT
