package exiftool

import (
	"context"
	"fmt"
	"strconv"

	"github.com/cwygoda/ansel/internal/cull/domain"
	"github.com/cwygoda/ansel/internal/exiftool"
)

// Writer records ratings, labels and tags in XMP sidecars beside the
// photographs they describe. The photographs themselves are never modified.
//
// XMP is an interoperability projection, never the source of truth: the store
// keeps every observation, while a sidecar carries only what another
// application can act on.
//
// The sidecar is updated through exiftool rather than rendered from a
// template, because the file is very often not ours alone. `ansel geolocate`
// records coordinates into the same sidecar, and Lightroom or Capture One may
// have written a great deal more. Rendering the file would discard all of it;
// exiftool updates it in place and leaves untouched what it was not asked
// about.
type Writer struct {
	session *exiftool.Session
}

// NewWriter returns a Writer driving the given session.
func NewWriter(session *exiftool.Session) *Writer {
	return &Writer{session: session}
}

// Write records one plan, creating the sidecar if it is not there yet.
func (w *Writer) Write(ctx context.Context, plan domain.SidecarPlan) error {
	args := append(sidecarArguments(plan), plan.SidecarPath)

	response, err := w.session.Execute(ctx, args...)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", plan.SidecarPath, err)
	}
	if !response.Ok() {
		return fmt.Errorf("failed to write %s: %s", plan.SidecarPath, response.Stderr)
	}
	return nil
}

// sidecarArguments builds the tag assignments for one plan.
//
// Every tag is qualified with its XMP group for the same reason the geolocate
// writer qualifies its own: left unqualified, exiftool resolves some of these
// to whichever group it prefers, which for a sidecar is not always the one
// another application will read.
func sidecarArguments(plan domain.SidecarPlan) []string {
	args := []string{
		"-overwrite_original",
		assign("XMP:Rating", strconv.Itoa(plan.Rating)),
		// Written even when empty, so a label this run did not award does not
		// survive from the last one.
		assign("XMP:Label", plan.Label),
	}

	// Withdraw the whole vocabulary, then add back what applies. See
	// domain.PolicyTags for why the withdrawal has to name every keyword
	// rather than clear the list.
	for _, name := range domain.PolicyTags() {
		args = append(args, "-XMP:Subject-="+name)
	}
	for _, name := range plan.Tags {
		args = append(args, "-XMP:Subject+="+name)
	}
	return args
}

func assign(tag, value string) string { return "-" + tag + "=" + value }
