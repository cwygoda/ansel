# Ansel

A command-line image processing tool for resizing and framing images for social media and print.

Named after [Ansel Adams](https://en.wikipedia.org/wiki/Ansel_Adams), the legendary photographer known for his meticulous attention to image quality.

## Features

- **Linear light resizing** using [Magic Kernel Sharp 2021](https://johncostella.com/magic/) — the gold-standard algorithm used by Facebook and Instagram
- **Automatic framing** with configurable colors and widths
- **Size presets** for Instagram, Facebook, Twitter/X, YouTube, LinkedIn, and print
- **High-quality JPEG output** with configurable quality

## Installation

```bash
go install github.com/cwygoda/ansel@latest
```

Or build from source:

```bash
git clone https://github.com/cwygoda/ansel.git
cd ansel
go build -o ansel .
```

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
go test ./...
```

### Test Data

The test image (`testdata/input.jpg`) is ["Close-up of a Jumping Spider on a Leaf"](https://www.pexels.com/photo/close-up-of-a-jumping-spider-on-a-leaf-35243201/) by Silvio Fotografias, sourced from Pexels.

## License

MIT
