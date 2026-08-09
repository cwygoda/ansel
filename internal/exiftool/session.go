// Package exiftool drives the exiftool executable.
//
// This is shared transport, not an adapter: nothing here knows what a
// photograph is, and no domain type may appear in this package. Each feature
// keeps its own adapter holding the tags it asks for and the mapping onto its
// own domain, and leans on a Session for the subprocess protocol they would
// otherwise each reimplement.
package exiftool

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// Session is one long-lived `-stay_open` exiftool process.
//
// A shoot means thousands of lookups, and process startup would dominate if
// every one of them paid for it. Commands are serialized through a mutex:
// exiftool consumes one argument block at a time, and interleaving two would
// corrupt both.
type Session struct {
	binary string

	mu       sync.Mutex
	sequence int
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   *bufio.Reader
	stderr   *bufio.Reader
}

// Response is the result of one command.
type Response struct {
	// Stdout is everything the command printed before its ready marker.
	Stdout []byte

	// Stderr is everything it printed to the diagnostic stream: warnings for
	// tags that could not be read, errors naming files that could not be
	// written.
	Stderr string

	// Status is exiftool's own exit status for this command alone: 0 on
	// success, 1 if an error occurred, 2 if every file failed an -if
	// condition. Inside a shared process this is the only exact attribution
	// available, since the process itself has not exited.
	Status int
}

// Ok reports whether the command succeeded.
func (r Response) Ok() bool { return r.Status == 0 }

// New returns a Session driving the given exiftool binary.
func New(binary string) *Session {
	if binary == "" {
		binary = "exiftool"
	}
	return &Session{binary: binary}
}

// Execute runs one command and waits for it to finish.
//
// Both streams are drained concurrently. Reading stdout to completion first
// would deadlock as soon as a command produced more diagnostics than the
// stderr pipe buffer holds — five hundred unreadable frames in one batch is
// enough — because exiftool would block writing stderr while still owing us
// the stdout we were waiting on.
func (s *Session) Execute(ctx context.Context, args ...string) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureStarted(); err != nil {
		return Response{}, err
	}

	s.sequence++
	sequence := s.sequence
	if err := s.send(args, sequence); err != nil {
		return Response{}, err
	}

	type payloadResult struct {
		payload string
		err     error
	}
	collected := make(chan payloadResult, 1)
	go func() {
		payload, err := readUntilLine(s.stdout, readyMarker(sequence))
		collected <- payloadResult{payload, err}
	}()

	diagnostics, status, statusErr := readUntilStatus(s.stderr, statusPrefix(sequence))
	result := <-collected

	if result.err != nil {
		return Response{}, result.err
	}
	if statusErr != nil {
		return Response{}, statusErr
	}
	return Response{Stdout: []byte(result.payload), Stderr: diagnostics, Status: status}, nil
}

// Close shuts the process down cleanly.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd == nil {
		return nil
	}
	_, _ = io.WriteString(s.stdin, "-stay_open\nFalse\n")
	_ = s.stdin.Close()
	err := s.cmd.Wait()
	s.cmd, s.stdin, s.stdout, s.stderr = nil, nil, nil, nil
	return err
}

// send writes one command. Arguments go one per line, terminated by -execute,
// which is the protocol stay_open mode expects.
//
// The -echo4 line is what makes a shared process usable for writing. It is
// emitted to stderr after processing completes and interpolates ${status},
// exiftool's exit status for this command alone, so a failure can be pinned to
// the file that caused it. It also guarantees stderr terminates on every
// command, so the drain above never waits for output that will not come.
func (s *Session) send(args []string, sequence int) error {
	block := make([]string, 0, len(args)+3)
	block = append(block, args...)
	block = append(block,
		"-echo4", statusPrefix(sequence)+"${status}",
		fmt.Sprintf("-execute%d", sequence))

	if _, err := io.WriteString(s.stdin, strings.Join(block, "\n")+"\n"); err != nil {
		return fmt.Errorf("failed to send request to exiftool: %w", err)
	}
	return nil
}

// readyMarker is what exiftool prints on stdout after each numbered -execute.
func readyMarker(sequence int) string { return fmt.Sprintf("{ready%d}", sequence) }

// statusPrefix leads the stderr line carrying the command's exit status. The
// sequence number is part of both markers so a desynchronized stream is
// detected rather than silently mistaken for the next command's output.
func statusPrefix(sequence int) string { return fmt.Sprintf("{status%d}", sequence) }

// readUntilLine accumulates output up to, but not including, a line that is
// exactly the marker.
func readUntilLine(reader *bufio.Reader, marker string) (string, error) {
	var payload strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("failed to read exiftool response: %w", err)
		}
		if strings.TrimSpace(line) == marker {
			return payload.String(), nil
		}
		payload.WriteString(line)
	}
}

// readUntilStatus accumulates diagnostics up to the status line, and reports
// the status it carries.
func readUntilStatus(reader *bufio.Reader, prefix string) (string, int, error) {
	var diagnostics strings.Builder
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", 0, fmt.Errorf("failed to read exiftool diagnostics: %w", err)
		}

		reported, found := strings.CutPrefix(strings.TrimSpace(line), prefix)
		if !found {
			diagnostics.WriteString(line)
			continue
		}

		status, err := strconv.Atoi(strings.TrimSpace(reported))
		if err != nil {
			return "", 0, fmt.Errorf("exiftool reported an unreadable status %q", reported)
		}
		return strings.TrimSpace(diagnostics.String()), status, nil
	}
}

func (s *Session) ensureStarted() error {
	if s.cmd != nil {
		return nil
	}

	cmd := exec.Command(s.binary, "-stay_open", "True", "-@", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to open exiftool stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to open exiftool stdout: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to open exiftool stderr: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w (install it with: brew install exiftool)", s.binary, err)
	}

	s.cmd, s.stdin = cmd, stdin
	s.stdout = bufio.NewReaderSize(stdout, 1<<20)
	s.stderr = bufio.NewReader(stderr)
	return nil
}
