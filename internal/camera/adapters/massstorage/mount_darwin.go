//go:build darwin

package massstorage

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

var diskutilPartitionRE = regexp.MustCompile(`^\s+\d+:\s+\S+\s+(.+?)\s+\d+(?:\.\d+)?\s+\S+\s+(disk\S+)\s*$`)

func mountKnownCardVolumes(ctx context.Context) error {
	out, err := exec.CommandContext(ctx, "diskutil", "list", "external", "physical").CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to list external disks: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	for _, volume := range parseExternalVolumes(string(out)) {
		camera := domain.Camera{Model: volume.Name}
		if !camera.IsKnown() {
			continue
		}
		if err := mountVolumeReadOnly(ctx, volume.Identifier); err != nil {
			return fmt.Errorf("failed to mount camera card %s (%s): %w", volume.Name, volume.Identifier, err)
		}
	}
	return nil
}

func parseExternalVolumes(out string) []externalVolume {
	volumes := []externalVolume{}
	for _, line := range strings.Split(out, "\n") {
		match := diskutilPartitionRE.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		volumes = append(volumes, externalVolume{
			Name:       strings.TrimSpace(match[1]),
			Identifier: match[2],
		})
	}
	return volumes
}

func mountVolumeReadOnly(ctx context.Context, identifier string) error {
	out, err := exec.CommandContext(ctx, "diskutil", "mount", "readOnly", identifier).CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already mounted") {
		return fmt.Errorf("diskutil mount readOnly %s failed: %w\n%s", identifier, err, strings.TrimSpace(string(out)))
	}
	return nil
}

type externalVolume struct {
	Name       string
	Identifier string
}
