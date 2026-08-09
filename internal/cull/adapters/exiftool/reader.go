// Package exiftool adapts the exiftool executable to the cull ports. It is an
// adapter, not part of the domain: nothing outside this package knows that
// metadata comes from a subprocess.
package exiftool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// batchSize bounds how many paths go into a single exiftool request, keeping
// the response buffer predictable on very large shoots.
const batchSize = 500

// readyMarker is what exiftool prints after each -execute in stay_open mode.
const readyMarker = "{ready}"

// Reader reads capture metadata and embedded previews.
//
// Metadata flows through one long-lived `-stay_open` process, because a shoot
// means thousands of lookups and process startup would otherwise dominate.
// Preview extraction deliberately does not: preview data is binary, and
// binary on a shared stdout stream cannot be framed reliably against the
// ready marker. Previews are only needed for RAW files, and each one is
// immediately followed by far more expensive decoding and analysis.
type Reader struct {
	binary string

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
}

// New returns a Reader driving the given exiftool binary.
func New(binary string) *Reader {
	if binary == "" {
		binary = "exiftool"
	}
	return &Reader{binary: binary}
}

// Read returns metadata keyed by the path it was requested for.
func (r *Reader) Read(ctx context.Context, paths []string) (map[string]domain.Metadata, error) {
	metadata := make(map[string]domain.Metadata, len(paths))

	for start := 0; start < len(paths); start += batchSize {
		end := min(start+batchSize, len(paths))
		entries, err := r.readBatch(ctx, paths[start:end])
		if err != nil {
			return nil, err
		}
		for path, entry := range entries {
			metadata[path] = entry
		}
	}
	return metadata, nil
}

func (r *Reader) readBatch(ctx context.Context, paths []string) (map[string]domain.Metadata, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.ensureStarted(); err != nil {
		return nil, err
	}
	if err := r.request(paths); err != nil {
		return nil, err
	}

	payload, err := r.readUntilReady(ctx)
	if err != nil {
		return nil, err
	}
	return parseMetadata(payload)
}

// request writes one exiftool command. Arguments go one per line, terminated
// by -execute, which is the protocol stay_open mode expects.
func (r *Reader) request(paths []string) error {
	args := append([]string{"-json", "-n"}, tagArguments()...)
	args = append(args, paths...)
	args = append(args, "-execute")

	if _, err := io.WriteString(r.stdin, strings.Join(args, "\n")+"\n"); err != nil {
		return fmt.Errorf("failed to send request to exiftool: %w", err)
	}
	return nil
}

// tagArguments lists exactly the tags the pipeline needs. Asking for
// everything would pull in large MakerNote payloads for no benefit.
func tagArguments() []string {
	return []string{
		"-SourceFile",
		"-DateTimeOriginal",
		"-CreateDate",
		"-Make",
		"-Model",
		"-LensModel",
		"-FocalLength",
		"-FNumber",
		"-ExposureTime",
		"-ISO",
		"-Orientation",
		"-ImageWidth",
		"-ImageHeight",
	}
}

func (r *Reader) readUntilReady(ctx context.Context) ([]byte, error) {
	var payload strings.Builder
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line, err := r.stdout.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("failed to read exiftool response: %w", err)
		}
		if strings.TrimSpace(line) == readyMarker {
			return []byte(payload.String()), nil
		}
		payload.WriteString(line)
	}
}

// Close shuts the long-lived process down cleanly.
func (r *Reader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cmd == nil {
		return nil
	}
	_, _ = io.WriteString(r.stdin, "-stay_open\nFalse\n")
	_ = r.stdin.Close()
	err := r.cmd.Wait()
	r.cmd, r.stdin, r.stdout = nil, nil, nil
	return err
}

func (r *Reader) ensureStarted() error {
	if r.cmd != nil {
		return nil
	}

	cmd := exec.Command(r.binary, "-stay_open", "True", "-@", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to open exiftool stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to open exiftool stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w (install it with: brew install exiftool)", r.binary, err)
	}

	r.cmd, r.stdin, r.stdout = cmd, stdin, bufio.NewReaderSize(stdout, 1<<20)
	return nil
}

func parseMetadata(payload []byte) (map[string]domain.Metadata, error) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return map[string]domain.Metadata{}, nil
	}

	var entries []map[string]any
	if err := json.Unmarshal(payload, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse exiftool json: %w", err)
	}

	metadata := make(map[string]domain.Metadata, len(entries))
	for _, entry := range entries {
		source := asString(entry["SourceFile"])
		if source == "" {
			continue
		}
		metadata[source] = metadataFrom(entry)
	}
	return metadata, nil
}
