package gphoto2

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

type Client struct {
	Binary string
}

func New(binary string) *Client {
	if binary == "" {
		binary = "gphoto2"
	}
	return &Client{Binary: binary}
}

func (c *Client) Detect(ctx context.Context) ([]domain.Camera, error) {
	out, err := c.run(ctx, "--auto-detect")
	if err != nil {
		return nil, err
	}
	return parseAutoDetect(out), nil
}

func (c *Client) ListFiles(ctx context.Context, camera domain.Camera) ([]domain.RemoteFile, error) {
	args := c.cameraArgs(camera, "--list-files", "--recurse")
	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}
	return parseListFiles(out), nil
}

func (c *Client) Download(ctx context.Context, camera domain.Camera, file domain.RemoteFile, destination string) error {
	args := c.cameraArgs(camera, "--folder", file.Folder, "--filename", destination, "--force-overwrite", "--get-file", file.Name)
	_, err := c.run(ctx, args...)
	return err
}

func (c *Client) cameraArgs(camera domain.Camera, args ...string) []string {
	prefix := []string{}
	if camera.Port != "" {
		prefix = append(prefix, "--port", camera.Port)
	}
	return append(prefix, args...)
}

func (c *Client) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, c.Binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s %s failed: %w\n%s", c.Binary, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func parseAutoDetect(out string) []domain.Camera {
	var cameras []domain.Camera
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "Model") || strings.HasPrefix(line, "---") {
			continue
		}
		idx := strings.LastIndex(line, " usb:")
		if idx < 0 {
			idx = strings.LastIndex(line, " ptpip:")
		}
		if idx < 0 {
			continue
		}
		model := strings.TrimSpace(line[:idx])
		port := strings.TrimSpace(line[idx+1:])
		cameras = append(cameras, withKnownIDs(domain.Camera{Model: model, Port: port}))
	}
	return cameras
}

func withKnownIDs(camera domain.Camera) domain.Camera {
	model := strings.ToLower(camera.Model)
	for _, known := range domain.KnownCameras {
		for _, alias := range known.Aliases {
			if strings.Contains(model, alias) {
				camera.VendorID = known.VendorID
				camera.ProductID = known.ProductID
				return camera
			}
		}
	}
	return camera
}

var folderRE = regexp.MustCompile(`^There (?:is|are) \d+ files? in folder '([^']+)'\.`)
var fileRE = regexp.MustCompile(`^#\d+\s+(.+?)\s+\w+\s+(\d+)\s+([A-Za-z]+)(?:\s+(.+))?$`)

func parseListFiles(out string) []domain.RemoteFile {
	var files []domain.RemoteFile
	folder := "/"
	s := bufio.NewScanner(strings.NewReader(out))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if m := folderRE.FindStringSubmatch(line); len(m) == 2 {
			folder = m[1]
			continue
		}
		m := fileRE.FindStringSubmatch(line)
		if len(m) == 0 {
			continue
		}
		files = append(files, domain.RemoteFile{
			Folder:    folder,
			Name:      filepath.Base(strings.TrimSpace(m[1])),
			SizeBytes: sizeBytes(m[2], m[3]),
			MIMEType:  strings.TrimSpace(m[4]),
		})
	}
	return files
}

func sizeBytes(value, unit string) int64 {
	n, _ := strconv.ParseInt(value, 10, 64)
	switch strings.ToLower(unit) {
	case "b", "byte", "bytes":
		return n
	case "kb", "kib":
		return n * 1024
	case "mb", "mib":
		return n * 1024 * 1024
	case "gb", "gib":
		return n * 1024 * 1024 * 1024
	default:
		return n
	}
}
