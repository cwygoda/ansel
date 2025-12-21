# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
go build -o ansel .      # Build binary
go test ./...            # Run all tests
go test ./cmd/...        # Run CLI tests only
go test ./internal/...   # Run image processing tests only
go test -run TestName    # Run specific test
```

## Architecture

Ansel is a CLI image processor that resizes images in linear light color space and adds frames. It uses Cobra for CLI handling.

### Package Structure

- **`cmd/`** - Cobra CLI commands
  - `root.go` - Root command setup
  - `process.go` - Main processing command with size presets, frame options, fit modes

- **`internal/image/`** - Image processing library
  - `colorspace.go` - sRGB ↔ Linear conversion (`LinearImage` type, `ToLinear()`, `ToSRGB()`)
  - `resize.go` - Resampling filters (Bilinear, Catmull-Rom, Lanczos, Magic Kernel Sharp 2021)
  - `frame.go` - Frame addition (inner/outer), color parsing
  - `loader.go` - Multi-format loading (JPEG, PNG, TIFF)
  - `saver.go` - JPEG output with quality settings

### Key Concepts

**Linear Light Resizing**: Images are converted from sRGB to linear color space before resizing, then back to sRGB. This prevents color shifts during interpolation. The pipeline is: `Load → ToLinear → Resize → ToSRGB → Save`.

**Magic Kernel Sharp 2021**: The default resize filter, combining a quadratic B-spline base kernel with sharpening coefficients `{-1, 6, -35, 204, -35, 6, -1}/144`. Implementation in `mks2021Kernel()` and `magicKernel()`.

**Fit Modes**:
- `expand` - Output is exact target size, image centered with frame filling gaps
- `wrap` - Frame wraps around resized image, output size varies

**Output Naming**: Files get `_v0`, `_v1`, etc. suffix via `generateOutputPath()`.
