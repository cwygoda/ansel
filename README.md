# Ansel

A command-line tool for photographers: import from the camera, cull a shoot, place it on
a GPS track, and resize and frame the keepers for print or social media.

Named after [Ansel Adams](https://en.wikipedia.org/wiki/Ansel_Adams), the legendary
photographer known for his meticulous attention to image quality.

**Your originals are never modified.** Ratings and coordinates go to XMP sidecars beside
them, and every command that writes anything defaults to reporting what it would do.

## Features

- **USB camera import** for Nikon Z6 III and Ricoh GR IIIx, with local bookmarks and day
  folders — and a LaunchAgent to run it the moment you plug the camera in
- **Culling assistance** that scores sharpness and exposure, groups bursts, and picks the
  strongest frame — reading RAW files without ever demosaicing or modifying them
- **Geolocation** from GPS tracks, interpolating each frame's position along the route and
  correcting a drifted camera clock
- **Linear light resizing** using [Magic Kernel Sharp 2021](https://johncostella.com/magic/)
  — the gold-standard algorithm used by Facebook and Instagram
- **Automatic framing** with configurable colors and widths, and **size presets** for
  Instagram, Facebook, Twitter/X, YouTube, LinkedIn and print
- **Instagram publishing** and **CDN-backed static hosting** on AWS

## Installation

Ansel shells out to a few programs that Go cannot bundle:

```bash
brew install vips exiftool gphoto2
```

`vips` is required always, `exiftool` by `cull` and `geolocate`, `gphoto2` by
`camera import`. Then:

```bash
go install github.com/cwygoda/ansel@latest
```

Building from source is covered in [CONTRIBUTING.md](CONTRIBUTING.md).

## Quickstart

A shoot from camera to finished post:

```bash
ansel camera import                      # Pull today's frames into a day folder
ansel cull ~/Pictures/Ansel/Imports/2026-08-10       # See what it thinks; writes nothing
ansel cull --write ~/Pictures/Ansel/Imports/2026-08-10
ansel geolocate ~/Pictures/Ansel/Imports/2026-08-10 --track ride.fit.xz --write
ansel process --size ig-post DSC_1234.jpg
```

Every command that can write defaults to a dry run, so the middle steps are safe to
explore before committing to them.

---

## Camera Import

Import new photos from supported USB cameras or mounted camera cards into day-by-day
folders. Each camera/card import is bookmarked locally, so re-running picks up only what
is new.

```bash
ansel camera detect          # What is attached?
ansel camera import --dry-run
ansel camera import
```

Supported cameras: **Nikon Z6 III** and **Ricoh GR IIIx**. Mounted cards are detected by
a `DCIM` directory under a card root such as `/Volumes`, which covers a CFExpress card in
a USB-C reader. On macOS, known camera cards attached but not mounted yet are mounted
read-only before scanning. Anything else is skipped unless you pass `--include-unknown`.

Imports go to `~/Pictures/Ansel/Imports/YYYY-MM-DD/` and bookmark state lives in
`~/.ansel/camera-import-state.json`.

### Flags

| Flag                | Default    | Description                                          |
| ------------------- | ---------- | ---------------------------------------------------- |
| `--dry-run`         | `false`    | Plan imports without downloading                       |
| `--include-unknown` | `false`    | Import from cameras/cards not recognized as Z6 III / GR IIIx |
| `--base-dir`        | config     | Base import directory                                  |
| `--state`           | config     | Bookmark state path                                    |
| `--backend`         | config     | Import backend: `auto`, `gphoto2`, or `card`            |
| `--card-root`       | config     | Root directory to scan for mounted cards; repeatable    |
| `--gphoto2`         | `gphoto2`  | Path to the gphoto2 binary                             |

`ansel camera detect` takes `--backend`, `--card-root`, and `--gphoto2`.

### Import on plug-in

On macOS, install a user-space LaunchAgent that runs an import whenever a supported
camera is attached or a filesystem is mounted:

```bash
ansel camera install-agent
ansel camera uninstall-agent
```

The agent invokes the binary at its current path; pass `--ansel-path` to point it
somewhere else.

### Configuration

```toml
[camera_import]
base_dir = "~/Pictures/Ansel/Imports"
state_path = "~/.ansel/camera-import-state.json"
backend = "auto"
card_roots = ["/Volumes"]
folder_layout = "2006-01-02"
include_extensions = [".jpg", ".jpeg", ".nef", ".dng", ".mov", ".mp4", ".tif", ".tiff"]
include_unknown = false
```

`folder_layout` is a [Go time layout](https://pkg.go.dev/time#pkg-constants), so
`2006/01/02` gives you nested year/month/day folders instead.

---

## Cull

Analyze a shoot, group near-identical frames, and rank each group so the strongest frame
is easy to find.

```bash
ansel cull [flags] [directory]
```

**Nothing is written unless you ask.** By default `cull` reports exactly what a real run
would do and changes nothing on disk:

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
    10 already carry a rating and would be kept (use --force to replace)
```

Add `--write` to emit XMP sidecars beside the originals:

```bash
ansel cull --write ~/Pictures/Ansel/Imports/2026-08-09
```

### How It Works

Each photograph is measured through its **embedded preview**, so RAW files are read but
never demosaiced and never modified. Sharpness (variance of Laplacian and Tenengrad),
luminance distribution, and highlight/shadow clipping are recorded, along with a
perceptual hash.

Frames close together in capture time *and* visually alike are grouped. Candidates are
compared against the group's medoid rather than their nearest neighbour, so a run of
gradually changing frames is not chained into one oversized group.

Ranking is **group-relative**: the question answered is "which of these six frames is
strongest", not "is this objectively a good photograph". Every placement carries its
reasoning, visible with `--json`.

Two details worth knowing:

- **Sharpness is ranked against comparable frames only.** A camera's embedded RAW preview
  carries far less sharpening than a delivered JPEG — the same photograph can differ more
  than twentyfold — so RAW and rendered files are scored in separate populations.
- **Ranking low is not by itself a defect.** A quarter of any shoot sits in the bottom
  quarter. A frame is only called `soft` when it also has measurably less detail than a
  typical frame, so a uniformly sharp shoot reports nothing soft.

### Safety

- RAW and JPEG originals are never modified.
- Sidecars are **updated, not replaced**, so coordinates written by `ansel geolocate`
  survive a cull, and vice versa.
- A **rating already in a sidecar is yours** and is left alone; those frames are skipped
  and reported. `--force` replaces them. A rating of zero counts as no rating, which is
  what a sidecar holding only coordinates looks like.
- Results are cached in SQLite; re-running skips unchanged files.

### Flags

| Flag             | Default | Description                                          |
| ---------------- | ------- | ---------------------------------------------------- |
| `--dry-run`      | `true`  | Report what would be written without writing it      |
| `--write`        | `false` | Write XMP sidecars (disables the default dry run)    |
| `--force`        | `false` | Replace ratings that are already present             |
| `--reanalyze`    | `false` | Ignore cached results and measure everything again   |
| `--json`         | `false` | Emit results as JSON, with rank scores and reasons   |
| `--db`           | config  | Analysis database path                               |
| `--exiftool`     | config  | Path to the exiftool binary                          |
| `--workers`      | CPUs    | Concurrent analysis workers                          |
| `--max-edge`     | `2048`  | Longest edge of the analysis preview, in pixels      |
| `--group-window` | `8`     | Seconds between frames to consider them related      |

`--write` and `--dry-run=true` together are rejected rather than silently resolved.

### Configuration

Every threshold can be tuned without rebuilding. Changing a threshold recomputes tags
without re-measuring pixels.

```toml
[cull]
db_path = "~/.ansel/cull.db"
include_extensions = [".nef", ".jpg", ".jpeg"]
max_preview_edge = 2048
exiftool = "exiftool"

[cull.similarity]
window_seconds = 8      # Frames further apart are never grouped
max_distance = 10       # Perceptual hash distance to the group medoid
max_diameter = 14       # Widest a group may spread before it splits
burst_gap_seconds = 1.5 # Below this, a group is labelled a burst

[cull.ranking.weights]
sharpness = 0.30
exposure = 0.15

[cull.ranking.penalties]
severe_blur = 0.30
severe_highlight_clipping = 0.10
severe_shadow_clipping = 0.10

[cull.policy]
sharp_above = 0.65
soft_below = 0.25
soft_relative_below = 0.5
highlight_clip_warning = 0.03
shadow_clip_warning = 0.08

[cull.sidecar]
rating_best = 5
rating_alternate = 4
rating_usable = 3
label_best = "green"
label_warning = "red"
```

Analysis is stored in SQLite and is queryable directly, so any tag can be traced back to
the numbers behind it:

```bash
sqlite3 ~/.ansel/cull.db \
  "SELECT i.path, o.key, o.value FROM observations o JOIN images i ON i.id = o.image_id;"
```

### Scope

This is the technical stage: sharpness, exposure, clipping, near-duplicate grouping and
rule-based ranking. Face detection, eye-state analysis and learned image embeddings are
deliberately not included yet. Grouping therefore handles bursts and near-duplicates
well, and alternate compositions of the same subject less well.

---

## Geolocate

Place photographs on a recorded GPS track.

```bash
# Report only, writes nothing
ansel geolocate ~/Pictures/shoot --track ride.fit.xz

# Search a configured directory for recordings covering the shoot
ansel geolocate ~/Pictures/shoot

# Write XMP sidecars
ansel geolocate ~/Pictures/shoot --track ride.fit.xz --write

# Write geolocate-preview.kmz with thumbnail placemarks into the shoot directory
ansel geolocate ~/Pictures/shoot --track ride.fit.xz --preview-map
```

Only xz-compressed Garmin FIT activity files are read today. Other formats are added as
adapters without touching the matching logic.

### How It Works

Each photograph's capture time is resolved against the track. A frame taken exactly on a
recorded point takes that position; one taken between points is interpolated along the
**great circle** joining them, rather than by averaging coordinates. Devices drop to smart
recording on straight sections — five-second gaps and longer are routine — and over those
gaps straight-line coordinates visibly leave the road, increasingly so away from the
equator. Elevation is interpolated linearly and only when both neighbouring points carry
it.

Two limits keep inference honest:

- `--max-gap` (default 2m) is the widest spacing that may be interpolated across. A device
  paused for an hour has not described where you went, and the frame is reported rather
  than guessed at.
- `--buffer` (default 5m) is how far outside a recording a frame may still be placed, at
  its first or last point — for the shots taken while unpacking at the trailhead.

Tracks are matched one at a time and never merged, so a photograph taken between the
morning ride and the evening one is never placed halfway along the line joining them.

### Timezones

Cameras usually write the local wall clock with no zone; tracks record UTC. The zone is
resolved in this order, and **never guessed from the machine you are sitting at**:

1. the photograph's own `OffsetTimeOriginal`, when the camera recorded one
2. the offset stated by the track itself — FIT files carry a local timestamp alongside the
   UTC one, which is what makes the common case need no flags at all
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

### Preview map

`--preview-map` writes `geolocate-preview.kmz` into the shoot directory. The KMZ contains
one placemark per located photograph and embeds 320px JPEG thumbnails for quick checking
in Google Earth or another KML/KMZ viewer. This is independent of `--write`: a dry run can
still write the preview map because the map is a separate artifact you explicitly asked
for, not metadata written into photographs or sidecars.

Use `--preview-map-format kml` to write `geolocate-preview.kml` plus a sibling
`geolocate-preview-thumbnails/` directory instead of a single KMZ archive.

### Flags

| Flag                   | Default | Description                                                        |
| ---------------------- | ------- | ------------------------------------------------------------------ |
| `--track`              |         | Track file, glob or directory (repeatable)                         |
| `--drift`              | `0`     | How far the camera clock runs ahead of true time                   |
| `--max-gap`            | `2m`    | Largest track gap to interpolate across                            |
| `--buffer`             | `5m`    | How far outside a track photographs may still be placed            |
| `--tz`                 |         | Camera timezone, e.g. `Europe/Berlin`                              |
| `--utc-offset`         |         | Camera clock UTC offset, e.g. `+02:00`                             |
| `--tracks-dir`         | config  | Directory to search when no `--track` is given                     |
| `--dry-run`            | `true`  | Report without writing                                             |
| `--write`              | `false` | Write coordinates                                                  |
| `--in-place`           | `false` | Embed into the photographs instead of sidecars                     |
| `--force`              | `false` | Replace coordinates already present                                |
| `--json`               | `false` | Emit results as JSON, including how each position was derived      |
| `--preview-map`        | `false` | Write a KML/KMZ map with thumbnail placemarks into the shoot directory |
| `--preview-map-format` | `kmz`   | Preview map format: `kmz` or `kml`                                 |
| `--exiftool`           | config  | Path to the exiftool binary                                        |

### Configuration

```toml
[geolocate]
# Searched when no --track is given. Unset by default, which disables the search.
tracks_dir = "~/Documents/Activities"

max_gap_seconds = 120
buffer_seconds = 300
include_extensions = [".nef", ".jpg", ".jpeg"]
exiftool = "exiftool"

# Last resort only; the photograph and the track are both consulted first.
timezone = "Europe/Berlin"
```

Track files are matched by the date in their filename before any are opened, so pointing
`tracks_dir` at years of recordings stays fast.

---

## Process

Resize and frame images.

```bash
ansel process --size SIZE [flags] <input>...
```

Output files are created next to the input with a version suffix, so nothing is
overwritten:

- `photo.jpg` → `photo_v0.jpg`
- `photo_v0.jpg` → `photo_v1.jpg`

### Examples

```bash
# Instagram post with the default 5% white frame
ansel process --size ig-post photo.jpg

# Many files at once, black frame
ansel process --size ig-story --color black *.jpg

# Custom size with a 3% gray frame
ansel process --size 1920x1080 --color gray --frame 3 photo.jpg

# Wrap mode: the frame wraps the image, so output size varies
ansel process --size 800x600 --fit wrap photo.jpg

# Burn the IPTC headline in as a caption
ansel process --size ig-post --label photo.jpg

# Output to a specific directory
ansel process --size ig-post -o processed/ *.jpg
```

### Flags

| Flag              | Default   | Description                                                    |
| ----------------- | --------- | -------------------------------------------------------------- |
| `--size`          | required  | Output size: `WxH`, `W,H`, or preset name                      |
| `-o, --outdir`    |           | Output directory (created if needed)                           |
| `--filter`        | `mks2021` | Resize filter: `mks2021`, `lanczos`, `catmull-rom`, `bilinear` |
| `--fit`           | `expand`  | Fit mode: `expand` or `wrap`                                   |
| `--frame`         | `5`       | Frame width as percentage of shorter side                      |
| `--color`         | `#fff`    | Frame color (hex or named)                                     |
| `--quality`       | `92`      | JPEG output quality (1-100)                                    |
| `--label`         | `false`   | Render the image's IPTC headline as a text label               |
| `--label-font`    | `sans`    | Font family for the label                                      |
| `--label-size`    | `1.5`     | Label font size as percentage of shorter side                  |
| `--label-padding` | `1`       | Gap between image and label, as percentage of shorter side     |

### Size Presets

| Preset         | Dimensions | Platform                 |
| -------------- | ---------- | ------------------------ |
| `ig-post`      | 1080×1080  | Instagram square post    |
| `ig-portrait`  | 1080×1350  | Instagram portrait post  |
| `ig-landscape` | 1080×566   | Instagram landscape post |
| `ig-story`     | 1080×1920  | Instagram story          |
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

- **`expand`** (default): output is exactly the specified size. The image is resized to
  fit within the frame area and centered; the frame fills the remaining space.
- **`wrap`**: the frame wraps tightly around the resized image, so output size equals the
  image size plus the frame on all sides.

### Resize Filters

| Filter        | Description                                                           |
| ------------- | --------------------------------------------------------------------- |
| `mks2021`     | Magic Kernel Sharp 2021 — highest quality, used by Facebook/Instagram |
| `lanczos`     | Lanczos3 — classic high-quality filter                               |
| `catmull-rom` | Catmull-Rom cubic — good balance of sharpness and smoothness         |
| `bilinear`    | Bilinear — fast but lower quality                                    |

### Colors

Hex (`#fff`, `#ffffff`, `#ff0000`, `#rgba`) or named: `white`, `black`, `gray`, `red`,
`green`, `blue`, `yellow`, `orange`, `purple`, `pink`, `cyan`, `magenta`, `navy`, `teal`,
`olive`, `maroon`, `silver`, `lime`.

### Why Linear Light Resizing?

Standard image resizing operates in sRGB color space, which is non-linear. This causes
colors to shift during resizing — dark areas become too dark, and bright areas lose
detail.

Linear light resizing converts the image to linear color space before resizing, then
converts back to sRGB. This produces more accurate colors and better detail preservation,
especially in high-contrast areas. The Magic Kernel Sharp 2021 algorithm combines this
with optimized sharpening to produce results that are visibly superior to traditional
methods.

---

## Instagram

Publish images to Instagram through the Graph API.

```bash
# Single image; prompts for a caption
ansel ig photo.jpg

# Carousel of 2-10 images
ansel ig --carousel photo1.jpg photo2.jpg photo3.jpg

# Non-interactive
ansel ig --caption "Hello world!" photo.jpg

# Prepare the images but stop before publishing
ansel ig --dry-run photo.jpg
```

Images wider than 1080px are resized with Magic Kernel Sharp 2021 before upload, which
looks better than letting Instagram downscale them.

> **The Graph API fetches your images over the public internet.** `ansel ig` starts an
> [ngrok](https://ngrok.com/) tunnel and serves the prepared JPEGs from it so Instagram
> can reach them. The URLs are unguessable and the tunnel closes when the command exits,
> but the files are genuinely public while it runs. `--dry-run` never opens a tunnel.

Without `--caption`, ansel prompts on the terminal; in a non-interactive shell it fails
rather than posting an empty caption.

### Flags

| Flag         | Default | Description                                     |
| ------------ | ------- | ----------------------------------------------- |
| `--carousel` | `false` | Publish the images as one carousel (2-10)       |
| `--caption`  |         | Caption for the post (prompts if not set)       |
| `--dry-run`  | `false` | Prepare images but do not publish               |

### Configuration

```toml
[instagram]
access_token = "your-access-token"
user_id = "your-instagram-user-id"
ngrok_token = "your-ngrok-auth-token"  # Optional; falls back to $NGROK_AUTHTOKEN
```

`access_token` and `user_id` are required — this is the only command that needs them.

---

## Publish

Publish processed images to a CDN-backed subdomain on AWS.

```bash
ansel publish [flags]
```

On first run this creates a CloudFormation stack with an **S3 bucket**, a **CloudFront
distribution** with Origin Access Control, an **ACM certificate** with automatic DNS
validation, and a **Route53 subdomain record**. Later runs update the site and invalidate
the CloudFront cache.

```bash
ansel publish                          # Publish ./build
ansel publish --build-dir ./dist
ansel publish --subdomain gallery
ansel publish --profile myprofile
```

### Flags

| Flag          | Default   | Description                                    |
| ------------- | --------- | ---------------------------------------------- |
| `--build-dir` | `./build` | Directory containing files to upload           |
| `--subdomain` |           | Subdomain name (randomly generated if not set) |
| `--profile`   |           | AWS profile name                               |
| `--region`    |           | AWS region (uses default from AWS config)      |

### Configuration

Publish is the one command configured **per directory**, in `.ansel.toml` in the current
working directory rather than in `~/.ansel/config.toml`:

```toml
[publish]
subdomain = "abc123"
hosted_zone_id = "Z1234567890ABC"
domain_name = "example.com"
```

On first run, if no subdomain is specified, a random one is generated and saved.

### AWS Credentials

Requires credentials with permissions for CloudFormation, S3, CloudFront, ACM and
Route53 — see [`publish-iam-policy.json`](publish-iam-policy.json) for the exact policy.
Credentials are read from an AWS CLI profile, the standard environment variables, or an
IAM role.

---

## Configuration

Every command except `publish` reads one file, `~/.ansel/config.toml`, each taking its own
section. A missing file means "use the defaults", and no command is affected by another's
section:

```toml
[camera_import]  # See "Camera Import"
[cull]           # See "Cull"
[geolocate]      # See "Geolocate"
[instagram]      # See "Instagram"
```

Paths may start with `~`, which is expanded against your home directory. `publish` is the
exception: it reads `.ansel.toml` from the working directory, so each site keeps its own
settings next to its build.

## Environment Variables

| Variable           | Default | Description                                                  |
| ------------------ | ------- | ------------------------------------------------------------ |
| `ANSEL_LOG_LEVEL`  | `error` | Log level: `error`, `warning`, `info`, `debug`               |
| `NGROK_AUTHTOKEN`  |         | Used by `ansel ig` when `instagram.ngrok_token` is unset     |

Set `ANSEL_LOG_LEVEL` to `info` or `debug` for verbose libvips output.

## Contributing

Setup, project layout and the rules this codebase holds itself to are in
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT
