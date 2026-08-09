package domain

// Luma is a normalized grayscale analysis preview: row-major luminance in the
// range 0..1. It is decoded once per photograph and shared by every analyzer,
// because decoding dominates the cost of a run.
type Luma struct {
	Width  int
	Height int
	Pix    []float64
}

// NewLuma allocates a zeroed buffer of the given size.
func NewLuma(width, height int) *Luma {
	return &Luma{Width: width, Height: height, Pix: make([]float64, width*height)}
}

// At returns the luminance at x,y. Coordinates are clamped to the edges so
// convolution kernels do not need their own bounds handling.
func (l *Luma) At(x, y int) float64 {
	if x < 0 {
		x = 0
	} else if x >= l.Width {
		x = l.Width - 1
	}
	if y < 0 {
		y = 0
	} else if y >= l.Height {
		y = l.Height - 1
	}
	return l.Pix[y*l.Width+x]
}

// Set writes the luminance at x,y. Out-of-bounds writes are ignored.
func (l *Luma) Set(x, y int, value float64) {
	if x < 0 || y < 0 || x >= l.Width || y >= l.Height {
		return
	}
	l.Pix[y*l.Width+x] = value
}

// Empty reports whether there are no pixels to analyze.
func (l *Luma) Empty() bool {
	return l == nil || l.Width <= 0 || l.Height <= 0 || len(l.Pix) == 0
}
