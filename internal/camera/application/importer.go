package application

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cwygoda/ansel/internal/camera/domain"
	"github.com/cwygoda/ansel/internal/camera/ports"
)

type Importer struct {
	Backend  ports.CameraBackend
	Backends []ports.CameraBackend
	State    ports.StateStore
	Config   Config
	DryRun   bool
}

func (i *Importer) Detect(ctx context.Context) ([]domain.Camera, error) {
	backends := i.importBackends()
	if len(backends) == 0 {
		return nil, errors.New("no camera import backend configured")
	}

	var cameras []domain.Camera
	for _, backend := range backends {
		detected, err := backend.Detect(ctx)
		if err != nil {
			return cameras, err
		}
		cameras = append(cameras, detected...)
	}
	return cameras, nil
}

func (i *Importer) Import(ctx context.Context) ([]domain.ImportResult, error) {
	backends := i.importBackends()
	if len(backends) == 0 {
		return nil, errors.New("no camera import backend configured")
	}

	var results []domain.ImportResult
	for _, backend := range backends {
		cameras, err := backend.Detect(ctx)
		if err != nil {
			return results, err
		}
		for _, camera := range cameras {
			if !camera.IsKnown() && !i.Config.IncludeUnknown {
				continue
			}
			result, err := i.importCamera(ctx, backend, camera)
			if err != nil {
				return results, err
			}
			results = append(results, result)
		}
	}

	if !i.DryRun {
		if err := i.State.Save(); err != nil {
			return results, err
		}
	}
	return results, nil
}

func (i *Importer) importCamera(ctx context.Context, backend ports.CameraBackend, camera domain.Camera) (domain.ImportResult, error) {
	result := domain.ImportResult{Camera: camera}
	files, err := backend.ListFiles(ctx, camera)
	if err != nil {
		return result, fmt.Errorf("failed to list files on %s: %w", camera.Model, err)
	}
	result.Seen = len(files)

	cameraKey := camera.Key()
	for _, file := range files {
		if !i.shouldInclude(file.Name) {
			result.Skipped++
			continue
		}
		if i.State.IsImported(file.Key(cameraKey)) {
			result.Skipped++
			continue
		}
		result.Planned = append(result.Planned, file)
		if i.DryRun {
			continue
		}

		record, err := i.downloadFile(ctx, backend, camera, file)
		if err != nil {
			return result, err
		}
		if err := i.State.Record(record); err != nil {
			return result, err
		}
		result.Downloaded++
		result.Records = append(result.Records, record)
	}

	return result, nil
}

func (i *Importer) downloadFile(ctx context.Context, backend ports.CameraBackend, camera domain.Camera, file domain.RemoteFile) (domain.ImportRecord, error) {
	if err := os.MkdirAll(i.Config.BaseDir, 0o755); err != nil {
		return domain.ImportRecord{}, fmt.Errorf("failed to create base dir: %w", err)
	}
	staging, err := os.MkdirTemp(i.Config.BaseDir, ".ansel-import-*")
	if err != nil {
		return domain.ImportRecord{}, fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer os.RemoveAll(staging)

	tmp := filepath.Join(staging, file.Name)
	if err := backend.Download(ctx, camera, file, tmp); err != nil {
		return domain.ImportRecord{}, fmt.Errorf("failed to download %s/%s: %w", file.Folder, file.Name, err)
	}

	day := dateForFile(tmp).Format(i.Config.FolderLayout)
	dayDir := filepath.Join(i.Config.BaseDir, day)
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		return domain.ImportRecord{}, fmt.Errorf("failed to create day dir: %w", err)
	}
	dest := uniquePath(filepath.Join(dayDir, file.Name))
	if err := os.Rename(tmp, dest); err != nil {
		return domain.ImportRecord{}, fmt.Errorf("failed to move %s to %s: %w", tmp, dest, err)
	}

	return domain.ImportRecord{
		CameraKey:    camera.Key(),
		CameraModel:  camera.Model,
		Folder:       file.Folder,
		Name:         file.Name,
		SizeBytes:    file.SizeBytes,
		Destination:  dest,
		DownloadedAt: time.Now(),
	}, nil
}

func (i *Importer) importBackends() []ports.CameraBackend {
	if len(i.Backends) > 0 {
		return i.Backends
	}
	if i.Backend != nil {
		return []ports.CameraBackend{i.Backend}
	}
	return nil
}

func (i *Importer) shouldInclude(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	for _, allowed := range i.Config.IncludeExtensions {
		if ext == strings.ToLower(allowed) {
			return true
		}
	}
	return false
}

func dateForFile(path string) time.Time {
	info, err := os.Stat(path)
	if err == nil && !info.ModTime().IsZero() {
		return info.ModTime()
	}
	return time.Now()
}

func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for n := 1; ; n++ {
		candidate := fmt.Sprintf("%s_%d%s", base, n, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}
