package application

import (
	"context"

	"github.com/cwygoda/ansel/internal/cull/analysis"
	"github.com/cwygoda/ansel/internal/cull/domain"
	"golang.org/x/sync/errgroup"
)

// outcome is one photograph's analysis, written by exactly one goroutine to
// its own slot so the parallel phase needs no locking.
type outcome struct {
	observations   domain.Observations
	perceptualHash uint64
	reused         bool
	analyzed       bool
	failures       []domain.Failure
}

// analyzeAll measures every photograph, reusing stored results where nothing
// relevant has changed.
//
// Decoding dominates the cost of a run, so work is bounded rather than
// unlimited: one goroutine per photograph would hold thousands of decoded
// previews in memory at once.
func (c *Culler) analyzeAll(ctx context.Context, images []domain.Image, result *domain.CullResult) map[string]domain.Observations {
	outcomes := make([]outcome, len(images))
	version := analysis.SetVersion(c.analyzers())

	group := &errgroup.Group{}
	group.SetLimit(c.workerLimit())
	for i := range images {
		index := i
		group.Go(func() error {
			outcomes[index] = c.analyzeOne(ctx, images[index], version)
			return nil
		})
	}
	// The per-image func never returns an error: analysis is best-effort, and
	// one unreadable frame must not abandon the rest of the shoot.
	_ = group.Wait()

	return collect(images, outcomes, result)
}

// analyzeOne resolves a single photograph, from cache when possible.
func (c *Culler) analyzeOne(ctx context.Context, img domain.Image, version string) outcome {
	if ctx.Err() != nil {
		return outcome{failures: []domain.Failure{failure(img, "analysis", "", ctx.Err())}}
	}
	if cached, ok := c.cached(img, version); ok {
		return cached
	}

	luma, err := c.preview(ctx, img)
	if err != nil {
		return outcome{failures: []domain.Failure{failure(img, "preview", "", err)}}
	}

	return c.runAnalyzers(ctx, img, luma)
}

func (c *Culler) cached(img domain.Image, version string) (outcome, bool) {
	if c.Reanalyze {
		return outcome{}, false
	}
	observations, hash, ok, err := c.Store.CachedAnalysis(img.ID, img.Fingerprint, version)
	if err != nil || !ok {
		return outcome{}, false
	}
	return outcome{observations: observations, perceptualHash: hash, reused: true, analyzed: true}, true
}

func (c *Culler) preview(ctx context.Context, img domain.Image) (*domain.Luma, error) {
	data, err := c.Previews.Preview(ctx, img.Path)
	if err != nil {
		return nil, err
	}
	return c.Decoder.DecodeLuma(data, c.Config.MaxPreviewEdge)
}

// runAnalyzers executes every analyzer against the shared preview. One
// analyzer failing does not stop the others, and does not remove the
// photograph from the run.
func (c *Culler) runAnalyzers(ctx context.Context, img domain.Image, luma *domain.Luma) outcome {
	result := outcome{
		perceptualHash: analysis.PerceptualHash(luma),
		analyzed:       true,
	}
	input := analysis.Input{ImageID: img.ID, Metadata: img.Metadata, Preview: luma}

	for _, analyzer := range c.analyzers() {
		observations, err := analyzer.Analyze(ctx, input)
		if err != nil {
			result.failures = append(result.failures, failure(img, "analysis", analyzer.Name(), err))
			continue
		}
		result.observations = append(result.observations, observations...)
	}
	return result
}

// collect merges the parallel results back into the run, in image order.
func collect(images []domain.Image, outcomes []outcome, result *domain.CullResult) map[string]domain.Observations {
	observations := make(map[string]domain.Observations, len(images))

	for i := range images {
		current := outcomes[i]
		images[i].PerceptualHash = current.perceptualHash
		result.Failures = append(result.Failures, current.failures...)

		if !current.analyzed {
			continue
		}
		observations[images[i].ID] = current.observations
		if current.reused {
			result.Reused++
		} else {
			result.Analyzed++
		}
	}
	return observations
}

func (c *Culler) workerLimit() int {
	if c.Config.Workers > 0 {
		return c.Config.Workers
	}
	return 1
}

func failure(img domain.Image, stage, analyzer string, err error) domain.Failure {
	return domain.Failure{
		ImageID:  img.ID,
		Path:     img.Path,
		Stage:    stage,
		Analyzer: analyzer,
		Err:      err.Error(),
	}
}
