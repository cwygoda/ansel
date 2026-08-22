package massstorage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

// Client imports from mounted camera cards that expose a DCIM directory.
type Client struct {
	Roots []string
}

func New(roots []string) *Client {
	if len(roots) == 0 {
		roots = defaultRoots()
	}
	return &Client{Roots: roots}
}

func (c *Client) Detect(ctx context.Context) ([]domain.Camera, error) {
	if scansMacVolumes(c.Roots) {
		if err := mountKnownCardVolumes(ctx); err != nil {
			return nil, err
		}
	}

	var cameras []domain.Camera
	for _, root := range c.Roots {
		if err := ctx.Err(); err != nil {
			return cameras, err
		}
		if hasDCIM(root) {
			name := strings.TrimSpace(filepath.Base(root))
			cameras = append(cameras, domain.Camera{Model: name, Port: root})
			continue
		}
		volumes, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) || os.IsPermission(err) {
				continue
			}
			return cameras, fmt.Errorf("failed to read card root %s: %w", root, err)
		}
		for _, volume := range volumes {
			if err := ctx.Err(); err != nil {
				return cameras, err
			}
			if !volume.IsDir() {
				continue
			}
			name := strings.TrimSpace(volume.Name())
			if name == "" || strings.HasPrefix(name, ".") {
				continue
			}
			mountPath := filepath.Join(root, volume.Name())
			if !hasDCIM(mountPath) {
				continue
			}
			cameras = append(cameras, domain.Camera{Model: name, Port: mountPath})
		}
	}
	sort.Slice(cameras, func(i, j int) bool {
		return cameras[i].Port < cameras[j].Port
	})
	return cameras, nil
}

func (c *Client) ListFiles(ctx context.Context, camera domain.Camera) ([]domain.RemoteFile, error) {
	root := camera.Port
	if root == "" {
		return nil, fmt.Errorf("card mount path is empty")
	}
	dcim := filepath.Join(root, "DCIM")
	var files []domain.RemoteFile
	err := filepath.WalkDir(dcim, func(path string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		relDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		files = append(files, domain.RemoteFile{
			Folder:    "/" + filepath.ToSlash(relDir),
			Name:      entry.Name(),
			SizeBytes: info.Size(),
		})
		return nil
	})
	if err != nil {
		return files, fmt.Errorf("failed to list files on card %s: %w", root, err)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Folder == files[j].Folder {
			return files[i].Name < files[j].Name
		}
		return files[i].Folder < files[j].Folder
	})
	return files, nil
}

func (c *Client) Download(ctx context.Context, camera domain.Camera, file domain.RemoteFile, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	source := filepath.Join(camera.Port, filepath.FromSlash(strings.TrimPrefix(file.Folder, "/")), file.Name)
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", source)
	}
	if err := copyFile(source, destination, info.Mode().Perm()); err != nil {
		return err
	}
	if err := os.Chtimes(destination, info.ModTime(), info.ModTime()); err != nil {
		return fmt.Errorf("failed to preserve timestamp on %s: %w", destination, err)
	}
	return nil
}

func hasDCIM(path string) bool {
	info, err := os.Stat(filepath.Join(path, "DCIM"))
	return err == nil && info.IsDir()
}

func copyFile(source, destination string, mode os.FileMode) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", source, err)
	}
	defer in.Close()

	out, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", destination, err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("failed to copy %s to %s: %w", source, destination, copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close %s: %w", destination, closeErr)
	}
	return nil
}

func scansMacVolumes(roots []string) bool {
	if runtime.GOOS != "darwin" {
		return false
	}
	for _, root := range roots {
		if filepath.Clean(root) == "/Volumes" {
			return true
		}
	}
	return false
}

func defaultRoots() []string {
	if runtime.GOOS == "darwin" {
		return []string{"/Volumes"}
	}
	roots := []string{"/media", "/mnt"}
	if user := os.Getenv("USER"); user != "" {
		roots = append(roots, filepath.Join("/run/media", user))
	}
	return roots
}
