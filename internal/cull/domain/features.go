package domain

// Features is a typed projection of the observation store: the normalized
// inputs that ranking and tagging actually consume.
//
// It exists so ranking code does not have to know observation keys, and so
// adding an analyzer does not force every consumer to change. Raw values stay
// in Observations; these are derived and may be recomputed at any time without
// re-analyzing pixels.
type Features struct {
	ImageID string

	// Sharpness is a percentile rank among comparable frames, not an absolute
	// measure. A shoot supplies its own scale, so a set of landscapes and a
	// set of portraits are each judged on their own terms.
	Sharpness float64

	// SharpnessRelative is raw sharpness as a multiple of the median for
	// comparable frames: 1.0 is typical, 0.5 is half the detail of a typical
	// frame.
	//
	// A percentile alone cannot say whether a frame is actually soft, only
	// that something ranked below it. Some frame is always in the bottom
	// quartile, even in a shoot where every one is tack sharp. This is what
	// lets a verdict of "soft" require real evidence rather than mere rank.
	SharpnessRelative float64

	// ExposureQuality is 0..1, derived from clipping rather than mean
	// brightness so that intentional low-key and high-key frames are not
	// punished for being dark or bright.
	ExposureQuality float64

	HighlightClipping float64
	ShadowClipping    float64
	LuminanceMedian   float64

	// Analyzed is false when analysis failed for this photograph. Such frames
	// still appear in output, but must never outrank a measured one.
	Analyzed bool
}
