package ports

import (
	"context"

	"github.com/cwygoda/ansel/internal/camera/domain"
)

// CameraBackend is a secondary port for camera detection and file transfer.
type CameraBackend interface {
	Detect(ctx context.Context) ([]domain.Camera, error)
	ListFiles(ctx context.Context, camera domain.Camera) ([]domain.RemoteFile, error)
	Download(ctx context.Context, camera domain.Camera, file domain.RemoteFile, destination string) error
}

// StateStore is a secondary port for local import bookmarks.
type StateStore interface {
	IsImported(fileKey string) bool
	Record(record domain.ImportRecord) error
	Save() error
}
