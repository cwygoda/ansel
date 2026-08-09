package application

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/cwygoda/ansel/internal/config"
	"github.com/cwygoda/ansel/internal/cull/grouping"
	"github.com/cwygoda/ansel/internal/cull/policy"
	"github.com/cwygoda/ansel/internal/cull/ranking"
	"github.com/pelletier/go-toml/v2"
)

// Config is the application-level cull configuration, read from the [cull]
// section of ~/.ansel/config.toml. Every threshold lives here rather than in
// code, so tuning never requires a rebuild.
type Config struct {
	DBPath            string   `toml:"db_path"`
	IncludeExtensions []string `toml:"include_extensions"`
	MaxPreviewEdge    int      `toml:"max_preview_edge"`
	Workers           int      `toml:"workers"`
	ExiftoolBinary    string   `toml:"exiftool"`

	Similarity SimilarityConfig `toml:"similarity"`
	Ranking    RankingConfig    `toml:"ranking"`
	Policy     PolicyConfig     `toml:"policy"`
	Sidecar    SidecarConfig    `toml:"sidecar"`
}

type SimilarityConfig struct {
	WindowSeconds   float64 `toml:"window_seconds"`
	MaxDistance     int     `toml:"max_distance"`
	MaxDiameter     int     `toml:"max_diameter"`
	BurstGapSeconds float64 `toml:"burst_gap_seconds"`
}

type RankingConfig struct {
	Weights    map[string]float64 `toml:"weights"`
	Penalties  map[string]float64 `toml:"penalties"`
	Thresholds map[string]float64 `toml:"thresholds"`
}

type PolicyConfig struct {
	SharpAbove           float64 `toml:"sharp_above"`
	SoftBelow            float64 `toml:"soft_below"`
	SoftRelativeBelow    float64 `toml:"soft_relative_below"`
	HighlightClipWarning float64 `toml:"highlight_clip_warning"`
	ShadowClipWarning    float64 `toml:"shadow_clip_warning"`
	UnderexposedMedian   float64 `toml:"underexposed_median"`
	OverexposedMedian    float64 `toml:"overexposed_median"`
}

type SidecarConfig struct {
	RatingBest      int    `toml:"rating_best"`
	RatingAlternate int    `toml:"rating_alternate"`
	RatingUsable    int    `toml:"rating_usable"`
	LabelBest       string `toml:"label_best"`
	LabelWarning    string `toml:"label_warning"`
}

type rootConfig struct {
	Cull Config `toml:"cull"`
}

// DefaultConfig returns settings that work without any config file.
func DefaultConfig() Config {
	return Config{
		DBPath:            "~/.ansel/cull.db",
		IncludeExtensions: []string{".nef", ".jpg", ".jpeg"},
		MaxPreviewEdge:    2048,
		Workers:           runtime.NumCPU(),
		ExiftoolBinary:    "exiftool",
		Similarity: SimilarityConfig{
			WindowSeconds:   8,
			MaxDistance:     10,
			MaxDiameter:     14,
			BurstGapSeconds: 1.5,
		},
		Ranking: RankingConfig{
			// Absolute weights from the architecture's example policy. They
			// are renormalized over the terms that exist, so these keep their
			// relative meaning when face and aesthetic terms are added later.
			Weights:    map[string]float64{"sharpness": 0.30, "exposure": 0.15},
			Penalties:  map[string]float64{"severe_blur": 0.30, "severe_highlight_clipping": 0.10, "severe_shadow_clipping": 0.10},
			Thresholds: map[string]float64{"blur_below": 0.15, "highlight_clip_above": 0.03, "shadow_clip_above": 0.08},
		},
		Policy: PolicyConfig{
			SharpAbove:           0.65,
			SoftBelow:            0.25,
			SoftRelativeBelow:    0.5,
			HighlightClipWarning: 0.03,
			ShadowClipWarning:    0.08,
			UnderexposedMedian:   0.15,
			OverexposedMedian:    0.85,
		},
		Sidecar: SidecarConfig{
			RatingBest:      5,
			RatingAlternate: 4,
			RatingUsable:    3,
			LabelBest:       "green",
			LabelWarning:    "red",
		},
	}
}

// LoadConfig reads the [cull] section, treating a missing file as "use
// defaults". It deliberately does not go through config.Load, whose Validate
// requires Instagram credentials that have nothing to do with culling.
func LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	path, err := config.ConfigPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return expandConfig(cfg)
		}
		return cfg, fmt.Errorf("failed to read cull config: %w", err)
	}
	var root rootConfig
	if err := toml.Unmarshal(data, &root); err != nil {
		return cfg, fmt.Errorf("failed to parse cull config: %w", err)
	}
	overlayConfig(&cfg, root.Cull)
	return expandConfig(cfg)
}

func expandConfig(cfg Config) (Config, error) {
	expanded, err := config.ExpandPath(cfg.DBPath)
	if err != nil {
		return cfg, err
	}
	cfg.DBPath = expanded
	return cfg, nil
}

// GroupingOptions projects the config onto the grouping policy.
func (c Config) GroupingOptions() grouping.Options {
	return grouping.Options{
		Window:      time.Duration(c.Similarity.WindowSeconds * float64(time.Second)),
		MaxDistance: c.Similarity.MaxDistance,
		MaxDiameter: c.Similarity.MaxDiameter,
		BurstGap:    time.Duration(c.Similarity.BurstGapSeconds * float64(time.Second)),
	}
}

// RankingOptions projects the config onto the ranking policy.
func (c Config) RankingOptions() ranking.Options {
	return ranking.Options{
		Weights: ranking.Weights{
			Sharpness: c.Ranking.Weights["sharpness"],
			Exposure:  c.Ranking.Weights["exposure"],
		},
		Penalties: ranking.Penalties{
			SevereBlur:              c.Ranking.Penalties["severe_blur"],
			SevereHighlightClipping: c.Ranking.Penalties["severe_highlight_clipping"],
			SevereShadowClipping:    c.Ranking.Penalties["severe_shadow_clipping"],
		},
		Thresholds: ranking.Thresholds{
			BlurBelow:          c.Ranking.Thresholds["blur_below"],
			HighlightClipAbove: c.Ranking.Thresholds["highlight_clip_above"],
			ShadowClipAbove:    c.Ranking.Thresholds["shadow_clip_above"],
		},
	}
}

// PolicyOptions projects the config onto the tagging policy.
func (c Config) PolicyOptions() policy.Options {
	return policy.Options{
		SharpAbove:           c.Policy.SharpAbove,
		SoftBelow:            c.Policy.SoftBelow,
		SoftRelativeBelow:    c.Policy.SoftRelativeBelow,
		HighlightClipWarning: c.Policy.HighlightClipWarning,
		ShadowClipWarning:    c.Policy.ShadowClipWarning,
		UnderexposedMedian:   c.Policy.UnderexposedMedian,
		OverexposedMedian:    c.Policy.OverexposedMedian,
	}
}
