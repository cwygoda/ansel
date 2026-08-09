package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// printCullResult renders the run for a human.
//
// Groups of one are summarized rather than listed: a shoot is mostly unique
// frames, and printing every one of them would bury the burst sequences that
// actually need a decision. Flawed singles are still called out individually,
// because those are the rejects worth seeing.
func printCullResult(result domain.CullResult, write bool) {
	out := os.Stdout
	fmt.Fprintf(out, "%s: %d images, %d groups\n",
		filepath.Base(result.Root), result.Discovered, len(result.Groups))
	fmt.Fprintf(out, "  Analyzed: %d (reused %d)\n", result.Analyzed, result.Reused)

	if result.Discovered == 0 {
		fmt.Fprintln(out, "  Nothing to do.")
		return
	}

	printGroups(result)
	printFlaggedSingles(result)
	printSidecarSummary(result, write)
	printFailures(result)
}

func printGroups(result domain.CullResult) {
	images := result.ImageByID()
	printed := 0

	for _, group := range result.Groups {
		if len(group.Members) < 2 {
			continue
		}
		if printed == 0 {
			fmt.Fprintln(os.Stdout)
		}
		printGroup(result, group, images)
		printed++
	}
}

func printGroup(result domain.CullResult, group domain.SimilarityGroup, images map[string]domain.Image) {
	fmt.Fprintf(os.Stdout, "  %s  %d frames  %s\n", group.ID, len(group.Members), group.Kind)

	for _, imageID := range membersByRank(group, result.Ranks) {
		rank := result.Ranks[imageID]
		marker := " "
		if rank.BestInGroup() {
			marker = "★"
		}
		fmt.Fprintf(os.Stdout, "    %s %-24s %.2f  %s\n",
			marker, filepath.Base(images[imageID].Path), rank.Score, tagList(result.Tags[imageID]))
	}
	fmt.Fprintln(os.Stdout)
}

func membersByRank(group domain.SimilarityGroup, ranks map[string]domain.RankResult) []string {
	ordered := make([]string, len(group.Members))
	copy(ordered, group.Members)
	sort.SliceStable(ordered, func(a, b int) bool {
		return ranks[ordered[a]].Position < ranks[ordered[b]].Position
	})
	return ordered
}

// printFlaggedSingles surfaces ungrouped frames that have a technical problem.
func printFlaggedSingles(result domain.CullResult) {
	var flagged []domain.Image
	for _, img := range result.Images {
		if result.Ranks[img.ID].OutOf > 1 {
			continue
		}
		if hasTag(result.Tags[img.ID], domain.TagTechnicalWarning) {
			flagged = append(flagged, img)
		}
	}
	if len(flagged) == 0 {
		return
	}

	fmt.Fprintf(os.Stdout, "  Flagged: %d\n", len(flagged))
	for _, img := range flagged {
		fmt.Fprintf(os.Stdout, "    %-24s %s\n",
			filepath.Base(img.Path), tagList(result.Tags[img.ID]))
	}
	fmt.Fprintln(os.Stdout)
}

func printSidecarSummary(result domain.CullResult, write bool) {
	existing := 0
	for _, plan := range result.Sidecars {
		if plan.Exists {
			existing++
		}
	}

	if !write {
		fmt.Fprintf(os.Stdout, "  Would write: %d sidecars\n", len(result.Sidecars)-existing)
		if existing > 0 {
			fmt.Fprintf(os.Stdout, "    %d already exist and would be kept (use --force to replace)\n", existing)
		}
		return
	}

	fmt.Fprintf(os.Stdout, "  Wrote: %d sidecars\n", result.Written)
	if skipped := len(result.Sidecars) - result.Written; skipped > 0 {
		fmt.Fprintf(os.Stdout, "    %d kept as they already exist (use --force to replace)\n", skipped)
	}
}

func printFailures(result domain.CullResult) {
	if len(result.Failures) == 0 {
		return
	}
	fmt.Fprintf(os.Stderr, "\n  %d failures:\n", len(result.Failures))
	for _, failure := range result.Failures {
		fmt.Fprintf(os.Stderr, "    %s [%s] %s\n",
			filepath.Base(failure.Path), failure.Stage, failure.Err)
	}
}

func tagList(tags []domain.Tag) string {
	if len(tags) == 0 {
		return "-"
	}
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	return strings.Join(names, ", ")
}

func hasTag(tags []domain.Tag, name string) bool {
	for _, tag := range tags {
		if tag.Name == name {
			return true
		}
	}
	return false
}

// cullJSONImage is the machine-readable view. It carries the rank score and
// the reasons behind it, so a recommendation can be inspected rather than
// taken on faith.
type cullJSONImage struct {
	Path      string   `json:"path"`
	Group     string   `json:"group,omitempty"`
	RankScore float64  `json:"rank_score"`
	Position  int      `json:"position"`
	OutOf     int      `json:"out_of"`
	Reasons   []string `json:"reasons,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Rating    int      `json:"rating"`
	Label     string   `json:"label,omitempty"`
	Sidecar   string   `json:"sidecar"`
}

type cullJSONReport struct {
	Root       string          `json:"root"`
	Discovered int             `json:"discovered"`
	Analyzed   int             `json:"analyzed"`
	Reused     int             `json:"reused"`
	Groups     int             `json:"groups"`
	Written    int             `json:"written"`
	DryRun     bool            `json:"dry_run"`
	Images     []cullJSONImage `json:"images"`
	Failures   []string        `json:"failures,omitempty"`
}

func printCullJSON(result domain.CullResult, write bool) error {
	report := cullJSONReport{
		Root:       result.Root,
		Discovered: result.Discovered,
		Analyzed:   result.Analyzed,
		Reused:     result.Reused,
		Groups:     len(result.Groups),
		Written:    result.Written,
		DryRun:     !write,
		Images:     jsonImages(result),
	}
	for _, failure := range result.Failures {
		report.Failures = append(report.Failures,
			fmt.Sprintf("%s [%s] %s", failure.Path, failure.Stage, failure.Err))
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

func jsonImages(result domain.CullResult) []cullJSONImage {
	plans := make(map[string]domain.SidecarPlan, len(result.Sidecars))
	for _, plan := range result.Sidecars {
		plans[plan.ImagePath] = plan
	}

	images := make([]cullJSONImage, 0, len(result.Images))
	for _, img := range result.Images {
		rank := result.Ranks[img.ID]
		plan := plans[img.Path]
		images = append(images, cullJSONImage{
			Path:      img.Path,
			Group:     rank.GroupID,
			RankScore: rank.Score,
			Position:  rank.Position,
			OutOf:     rank.OutOf,
			Reasons:   rank.Reasons,
			Tags:      plan.Tags,
			Rating:    plan.Rating,
			Label:     plan.Label,
			Sidecar:   plan.SidecarPath,
		})
	}
	return images
}
