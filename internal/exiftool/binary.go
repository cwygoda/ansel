package exiftool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// RunBinary asks exiftool for a binary payload in a process of its own.
//
// This deliberately bypasses the session. Binary data on a shared stdout
// stream cannot be framed against a textual ready marker: the payload may
// contain the marker's own bytes. Binary extraction is rare enough — embedded
// previews, and only for RAW files — that a process per call is affordable.
func (s *Session) RunBinary(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, s.binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s failed: %w\n%s",
			s.binary, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
