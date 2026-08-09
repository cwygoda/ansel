package application

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// planSidecars decides what would be written for every photograph. It runs
// whether or not writing is enabled, so a dry run reports exactly the work a
// real run would perform.
func (c *Culler) planSidecars(images []domain.Image, ranks map[string]domain.RankResult, tags map[string][]domain.Tag) []domain.SidecarPlan {
	plans := make([]domain.SidecarPlan, 0, len(images))

	for _, img := range images {
		names := tagNames(tags[img.ID])
		sidecarPath := SidecarPathFor(img.Path)
		rating, label := c.ratingFor(ranks[img.ID], names)

		plans = append(plans, domain.SidecarPlan{
			ImagePath:   img.Path,
			SidecarPath: sidecarPath,
			Rating:      rating,
			Label:       label,
			Tags:        names,
			Exists:      exists(sidecarPath),
		})
	}
	return plans
}

// SidecarPathFor returns the XMP path beside a photograph: DSC_1234.NEF
// becomes DSC_1234.xmp. The photograph itself is never touched.
func SidecarPathFor(imagePath string) string {
	ext := filepath.Ext(imagePath)
	return strings.TrimSuffix(imagePath, ext) + ".xmp"
}

// ratingFor maps a placement onto stars and a colour label.
//
// A technically flawed frame is left unrated rather than given a low rating:
// zero stars means "not yet judged" in most applications, which is honest,
// whereas one star reads as a deliberate rejection the photographer did not make.
func (c *Culler) ratingFor(rank domain.RankResult, names []string) (int, string) {
	cfg := c.Config.Sidecar

	if contains(names, domain.TagTechnicalWarning) {
		return 0, cfg.LabelWarning
	}
	if rank.BestInGroup() {
		return cfg.RatingBest, cfg.LabelBest
	}
	if rank.OutOf > 1 && rank.Position == 2 {
		return cfg.RatingAlternate, ""
	}
	return cfg.RatingUsable, ""
}

func tagNames(tags []domain.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return names
}

func contains(names []string, target string) bool {
	for _, name := range names {
		if name == target {
			return true
		}
	}
	return false
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
