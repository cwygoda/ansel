package exiftool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireExiftool(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("exiftool"); err != nil {
		t.Skip("exiftool not installed")
	}
}

// sampleImage copies the checked-in fixture somewhere writable, so tests that
// write cannot damage it.
func sampleImage(t *testing.T) string {
	t.Helper()

	original, err := os.ReadFile(filepath.Join("..", "..", "testdata", "input.jpg"))
	if err != nil {
		t.Skipf("no sample image available: %v", err)
	}

	target := filepath.Join(t.TempDir(), "DSC_0001.jpg")
	if err := os.WriteFile(target, original, 0644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return target
}

func openSession(t *testing.T) *Session {
	t.Helper()
	requireExiftool(t)

	session := New("")
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestExecuteReportsSuccess(t *testing.T) {
	session := openSession(t)

	response, err := session.Execute(context.Background(), "-json", "-n", "-FileName", sampleImage(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !response.Ok() {
		t.Errorf("Status = %d with stderr %q, expected success", response.Status, response.Stderr)
	}
	if !strings.Contains(string(response.Stdout), "DSC_0001.jpg") {
		t.Errorf("stdout %q does not name the file that was read", response.Stdout)
	}
}

// A failure inside a shared process has no process exit code to report it.
// This is the whole reason the session frames stderr with -echo4 ${status}.
func TestExecuteAttributesFailureToTheCommand(t *testing.T) {
	session := openSession(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.jpg")

	response, err := session.Execute(context.Background(), "-json", "-n", "-FileName", missing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.Ok() {
		t.Error("a read of a missing file reported success")
	}
	if !strings.Contains(response.Stderr, "does-not-exist.jpg") {
		t.Errorf("stderr %q does not name the file that failed", response.Stderr)
	}
}

// The session must survive a failed command: a shoot with one unreadable frame
// still has hundreds of good ones behind it.
func TestExecuteStaysUsableAfterAFailure(t *testing.T) {
	session := openSession(t)
	image := sampleImage(t)

	if _, err := session.Execute(context.Background(), "-json", "-FileName", "/nope.jpg"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	response, err := session.Execute(context.Background(), "-json", "-n", "-FileName", image)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !response.Ok() {
		t.Errorf("Status = %d, expected the session to recover", response.Status)
	}
}

// Each command carries its own sequence number in both markers, so a response
// can never be mistaken for its neighbour's.
func TestExecuteKeepsResponsesInStep(t *testing.T) {
	session := openSession(t)
	image := sampleImage(t)

	for i := range 5 {
		response, err := session.Execute(context.Background(),
			"-json", "-n", "-FileName", image)
		if err != nil {
			t.Fatalf("command %d: unexpected error: %v", i, err)
		}

		var entries []map[string]any
		if err := json.Unmarshal(response.Stdout, &entries); err != nil {
			t.Fatalf("command %d: response was not its own json: %v", i, err)
		}
		if len(entries) != 1 {
			t.Fatalf("command %d: got %d entries, expected exactly one", i, len(entries))
		}
	}
}

func TestExecuteWritesAndReportsStatus(t *testing.T) {
	session := openSession(t)
	image := sampleImage(t)

	written, err := session.Execute(context.Background(),
		"-overwrite_original", "-XMP:Rating=5", image)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !written.Ok() {
		t.Fatalf("Status = %d with stderr %q, expected the write to succeed", written.Status, written.Stderr)
	}

	entries, err := session.Query(context.Background(), []string{"-XMP:Rating"}, []string{image})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || AsInt(entries[0]["Rating"]) != 5 {
		t.Errorf("read back %v, expected a rating of 5", entries)
	}
}

func TestQueryBatchesBeyondTheBatchSize(t *testing.T) {
	session := openSession(t)

	// The same file repeated is enough: what is under test is that every path
	// handed in comes back, across more than one underlying command.
	image := sampleImage(t)
	paths := make([]string, batchSize+7)
	for i := range paths {
		paths[i] = image
	}

	entries, err := session.Query(context.Background(), []string{"-FileName"}, paths)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != len(paths) {
		t.Errorf("got %d entries for %d paths", len(entries), len(paths))
	}
}

func TestQueryToleratesAnUnreadableFrame(t *testing.T) {
	session := openSession(t)
	image := sampleImage(t)
	missing := filepath.Join(t.TempDir(), "gone.jpg")

	entries, err := session.Query(context.Background(),
		[]string{"-FileName"}, []string{image, missing})
	if err != nil {
		t.Fatalf("one bad path failed the whole batch: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("got %d entries, expected the readable one to survive alone", len(entries))
	}
}

func TestQueryOnNoPathsAsksNothing(t *testing.T) {
	session := openSession(t)

	entries, err := session.Query(context.Background(), []string{"-FileName"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, expected none", len(entries))
	}
}

func TestCloseIsSafeOnAnUnstartedSession(t *testing.T) {
	if err := New("exiftool").Close(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
