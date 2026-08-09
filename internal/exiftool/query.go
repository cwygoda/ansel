package exiftool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

// batchSize bounds how many paths go into a single request, keeping the
// response buffer predictable on very large shoots.
const batchSize = 500

// Query reads the given tags for the given paths, returning one decoded JSON
// entry per file exiftool could read. Entries are keyed by their SourceFile,
// which callers map onto their own domain.
//
// Tags are always named explicitly by the caller. Asking for everything would
// pull in large MakerNote payloads for no benefit.
func (s *Session) Query(ctx context.Context, tags, paths []string) ([]map[string]any, error) {
	entries := make([]map[string]any, 0, len(paths))

	for start := 0; start < len(paths); start += batchSize {
		end := min(start+batchSize, len(paths))
		batch, err := s.queryBatch(ctx, tags, paths[start:end])
		if err != nil {
			return nil, err
		}
		entries = append(entries, batch...)
	}
	return entries, nil
}

func (s *Session) queryBatch(ctx context.Context, tags, paths []string) ([]map[string]any, error) {
	if len(paths) == 0 {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	args := make([]string, 0, len(tags)+len(paths)+2)
	args = append(args, "-json", "-n")
	args = append(args, tags...)
	args = append(args, paths...)

	response, err := s.Execute(ctx, args...)
	if err != nil {
		return nil, err
	}

	// A non-zero status is deliberately not fatal for a read. One unreadable
	// frame in a batch of five hundred names itself on stderr and is simply
	// absent from the JSON; failing the batch over it would cost the operator
	// the other four hundred and ninety nine.
	return decodeEntries(response)
}

func decodeEntries(response Response) ([]map[string]any, error) {
	payload := bytes.TrimSpace(response.Stdout)
	if len(payload) == 0 {
		return nil, nil
	}

	var entries []map[string]any
	if err := json.Unmarshal(payload, &entries); err != nil {
		if response.Stderr != "" {
			return nil, fmt.Errorf("failed to parse exiftool json: %w (%s)", err, response.Stderr)
		}
		return nil, fmt.Errorf("failed to parse exiftool json: %w", err)
	}
	return entries, nil
}
