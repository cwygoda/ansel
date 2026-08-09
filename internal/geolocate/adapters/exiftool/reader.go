// Package exiftool adapts the exiftool executable to the geolocate ports.
// Nothing outside this package knows that metadata comes from a subprocess.
//
// The cull feature drives exiftool too, but its reader cannot be reused here.
// It resolves a camera's unzoned timestamp against the machine's own local
// zone, which is harmless when grouping bursts by their spacing and quietly
// wrong when the answer is a place on the Earth: culling a Tokyo shoot at a
// desk in Berlin would shift every frame eight hours along the track. This
// reader keeps the reading and its zone apart and lets the application decide.
package exiftool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/cwygoda/ansel/internal/geolocate/domain"
)

// batchSize bounds how many paths go into a single exiftool request, keeping
// the response buffer predictable on very large shoots.
const batchSize = 500

// readyMarker is what exiftool prints after each -execute in stay_open mode.
const readyMarker = "{ready}"

// Reader reads capture metadata through one long-lived `-stay_open` process,
// because a shoot means thousands of lookups and process startup would
// otherwise dominate.
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

// Read returns capture clocks keyed by the path they were requested for.
func (r *Reader) Read(ctx context.Context, paths []string) (map[string]domain.Photo, error) {
	photos := make(map[string]domain.Photo, len(paths))

	err := r.eachBatch(ctx, paths, func(entries []map[string]any) {
		for _, entry := range entries {
			source := asString(entry["SourceFile"])
			if source == "" {
				continue
			}
			photos[source] = domain.Photo{Path: source, Clock: clockFrom(entry)}
		}
	})
	if err != nil {
		return nil, err
	}
	return photos, nil
}

// HasCoordinates reports which targets already carry a position. Targets that
// do not exist are skipped rather than queried, since a sidecar that has yet
// to be created plainly holds no coordinates and exiftool would only complain.
func (r *Reader) HasCoordinates(ctx context.Context, paths []string) (map[string]bool, error) {
	existing := make([]string, 0, len(paths))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, path)
		}
	}

	located := make(map[string]bool, len(existing))
	err := r.eachBatch(ctx, existing, func(entries []map[string]any) {
		for _, entry := range entries {
			source := asString(entry["SourceFile"])
			if source == "" {
				continue
			}
			located[source] = entry["GPSLatitude"] != nil && entry["GPSLongitude"] != nil
		}
	})
	if err != nil {
		return nil, err
	}
	return located, nil
}

// eachBatch runs the query in chunks, handing each decoded response to visit.
func (r *Reader) eachBatch(ctx context.Context, paths []string, visit func([]map[string]any)) error {
	for start := 0; start < len(paths); start += batchSize {
		end := min(start+batchSize, len(paths))
		entries, err := r.readBatch(ctx, paths[start:end])
		if err != nil {
			return err
		}
		visit(entries)
	}
	return nil
}

func (r *Reader) readBatch(ctx context.Context, paths []string) ([]map[string]any, error) {
	if len(paths) == 0 {
		return nil, nil
	}

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
	return parseEntries(payload)
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

// tagArguments lists exactly the tags geolocation needs. Asking for everything
// would pull in large MakerNote payloads for no benefit.
func tagArguments() []string {
	return []string{
		"-SourceFile",
		"-DateTimeOriginal",
		"-CreateDate",
		// The zone the camera was set to, when it bothered to record one.
		// Newer bodies write it; most do not.
		"-OffsetTimeOriginal",
		"-OffsetTime",
		"-GPSLatitude",
		"-GPSLongitude",
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

func parseEntries(payload []byte) ([]map[string]any, error) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return nil, nil
	}

	var entries []map[string]any
	if err := json.Unmarshal(payload, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse exiftool json: %w", err)
	}
	return entries, nil
}
