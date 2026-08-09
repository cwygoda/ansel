package analysis

import (
	"context"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Clipped pixels are those pinned at the very ends of the range, where detail
// is unrecoverable. This is a stricter notion than shadow/highlight occupancy,
// which merely reports that a region is dark or bright.
const (
	shadowClipLevel    = 1.0 / 255.0
	highlightClipLevel = 254.0 / 255.0
)

// Clipping reports the proportion of unrecoverable shadow and highlight
// pixels, kept separate because losing highlight detail and losing shadow
// detail are different problems with different remedies.
type Clipping struct{}

func (Clipping) Name() string    { return "clipping" }
func (Clipping) Version() string { return "1" }

func (c Clipping) Analyze(_ context.Context, in Input) ([]domain.Observation, error) {
	if in.Preview.Empty() {
		return nil, ErrNoPreview
	}

	var shadow, highlight int
	for _, value := range in.Preview.Pix {
		if value <= shadowClipLevel {
			shadow++
		} else if value >= highlightClipLevel {
			highlight++
		}
	}

	total := float64(len(in.Preview.Pix))
	return []domain.Observation{
		observe(c, domain.KeyShadowClipping, float64(shadow)/total),
		observe(c, domain.KeyHighlightClipping, float64(highlight)/total),
	}, nil
}
