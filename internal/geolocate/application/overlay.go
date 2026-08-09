package application

// overlayConfig applies the file's settings over the defaults.
//
// A zero value means "not set" throughout, matching how the camera and cull
// features merge their own sections. The consequence worth knowing is that
// max_gap_seconds and buffer_seconds cannot be configured to 0 — a zero there
// keeps the default. The command line can still express it, since --max-gap=0
// and --buffer=0 are explicit flags rather than absent fields, and both mean
// something useful: no gap limit, and no clamping at all.
func overlayConfig(base *Config, override Config) {
	if override.ExiftoolBinary != "" {
		base.ExiftoolBinary = override.ExiftoolBinary
	}
	if len(override.IncludeExtensions) > 0 {
		base.IncludeExtensions = override.IncludeExtensions
	}
	if override.TracksDir != "" {
		base.TracksDir = override.TracksDir
	}
	if override.MaxGapSeconds > 0 {
		base.MaxGapSeconds = override.MaxGapSeconds
	}
	if override.BufferSeconds > 0 {
		base.BufferSeconds = override.BufferSeconds
	}
	if override.UTCOffset != "" {
		base.UTCOffset = override.UTCOffset
	}
	if override.Timezone != "" {
		base.Timezone = override.Timezone
	}
}
