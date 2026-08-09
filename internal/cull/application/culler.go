package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/cwygoda/ansel/internal/cull/analysis"
	"github.com/cwygoda/ansel/internal/cull/domain"
	"github.com/cwygoda/ansel/internal/cull/grouping"
	"github.com/cwygoda/ansel/internal/cull/policy"
	"github.com/cwygoda/ansel/internal/cull/ports"
	"github.com/cwygoda/ansel/internal/cull/ranking"
)

// Culler runs the analysis pipeline over a shoot. It holds only ports, so the
// pipeline is testable without exiftool, libvips or a database.
type Culler struct {
	Metadata  ports.MetadataReader
	Previews  ports.PreviewSource
	Decoder   ports.PreviewDecoder
	Store     ports.Store
	Sidecars  ports.SidecarWriter
	Config    Config
	Analyzers []analysis.Analyzer

	// Write enables the only mutating step in the pipeline. When it is false
	// the full plan is still computed and reported, so a dry run and a real
	// run differ in exactly one place.
	Write bool

	// Force allows replacing sidecars that already exist on disk.
	Force bool

	// Reanalyze ignores cached results and recomputes every measurement.
	Reanalyze bool
}

// Cull analyzes, groups, ranks and tags a directory of photographs.
func (c *Culler) Cull(ctx context.Context, root string) (domain.CullResult, error) {
	images, err := Discover(root, c.Config.IncludeExtensions)
	if err != nil {
		return domain.CullResult{Root: root}, err
	}

	result := domain.CullResult{Root: root, Discovered: len(images)}
	if len(images) == 0 {
		return result, nil
	}

	if err := c.attachMetadata(ctx, images, &result); err != nil {
		return result, err
	}

	observations := c.analyzeAll(ctx, images, &result)
	c.interpret(images, observations, &result)

	if err := c.persist(images, observations, result); err != nil {
		return result, err
	}
	if err := c.markRatedSidecars(ctx, result.Sidecars); err != nil {
		return result, fmt.Errorf("failed to inspect existing sidecars: %w", err)
	}
	if c.Write {
		c.writeSidecars(ctx, &result)
	}
	return result, nil
}

// interpret turns raw observations into features, groups, ranks and tags.
// None of it touches pixels, which is why changing a threshold is cheap.
func (c *Culler) interpret(images []domain.Image, observations map[string]domain.Observations, result *domain.CullResult) {
	groupingOptions := c.Config.GroupingOptions()
	groupingOptions.IDPrefix = groupPrefix(result.Root)

	features := policy.Normalize(images, observations, c.Config.PolicyOptions())
	groups := grouping.Build(images, groupingOptions)
	ranks := ranking.Rank(groups, features, c.Config.RankingOptions())
	tags := policy.Tags(features, groups, ranks, c.Config.PolicyOptions())

	result.Images = images
	result.Groups = groups
	result.Ranks = ranks
	result.Tags = tags
	result.Sidecars = c.planSidecars(images, ranks, tags)
}

// attachMetadata fills in capture information before any visual analysis, so
// grouping has timestamps even for photographs whose analysis later fails.
func (c *Culler) attachMetadata(ctx context.Context, images []domain.Image, result *domain.CullResult) error {
	paths := make([]string, 0, len(images))
	for _, img := range images {
		paths = append(paths, img.Path)
	}

	metadata, err := c.Metadata.Read(ctx, paths)
	if err != nil {
		return fmt.Errorf("failed to read metadata: %w", err)
	}

	for i := range images {
		found, ok := metadata[images[i].Path]
		if !ok {
			result.Failures = append(result.Failures, domain.Failure{
				ImageID: images[i].ID, Path: images[i].Path,
				Stage: "metadata", Err: "no metadata returned",
			})
			continue
		}
		images[i].Metadata = found
	}
	return nil
}

// persist records everything the run learned. Analysis is written before
// grouping so that a later failure cannot cost the expensive measurements.
func (c *Culler) persist(images []domain.Image, observations map[string]domain.Observations, result domain.CullResult) error {
	version := analysis.SetVersion(c.analyzers())

	for _, img := range images {
		obs, ok := observations[img.ID]
		if !ok {
			continue
		}
		if err := c.Store.SaveAnalysis(img, version, obs); err != nil {
			return fmt.Errorf("failed to save analysis for %s: %w", img.Path, err)
		}
	}
	if err := c.Store.SaveGrouping(result.Groups, result.Ranks); err != nil {
		return fmt.Errorf("failed to save grouping: %w", err)
	}
	if err := c.Store.SaveTags(policy.Source, result.Tags); err != nil {
		return fmt.Errorf("failed to save tags: %w", err)
	}
	return nil
}

// writeSidecars performs the run's only mutation. A rating already in a
// sidecar is a judgement its photographer made, so it is preserved unless
// overwriting was explicitly requested. Everything else in the file survives
// regardless: the writer updates in place rather than replacing.
func (c *Culler) writeSidecars(ctx context.Context, result *domain.CullResult) {
	for i := range result.Sidecars {
		plan := &result.Sidecars[i]
		if plan.HasUserRating && !c.Force {
			plan.Skipped = "sidecar already carries a rating"
			continue
		}
		if err := c.Sidecars.Write(ctx, *plan); err != nil {
			plan.Skipped = err.Error()
			result.Failures = append(result.Failures, domain.Failure{
				Path: plan.ImagePath, Stage: "sidecar", Err: err.Error(),
			})
			continue
		}
		plan.Written = true
		result.Written++
	}
}

// groupPrefix scopes group identifiers to a shoot. One database serves every
// directory that has been culled, so sequential numbering alone would let two
// shoots claim the same group.
func groupPrefix(root string) string {
	sum := sha256.Sum256([]byte(root))
	return "g" + hex.EncodeToString(sum[:3])
}

func (c *Culler) analyzers() []analysis.Analyzer {
	if len(c.Analyzers) > 0 {
		return c.Analyzers
	}
	return analysis.Default()
}
