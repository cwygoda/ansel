package application

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cwygoda/ansel/internal/camera/domain"
	"github.com/cwygoda/ansel/internal/camera/ports"
)

func TestImporterImportUsesMultipleBackends(t *testing.T) {
	knownCard := domain.Camera{Model: "NIKON Z6_3", Port: "/Volumes/NIKON Z6_3"}
	file := domain.RemoteFile{Folder: "/DCIM/100NZ6_3", Name: "DSC_0001.NEF", SizeBytes: 42}
	state := &fakeState{imported: map[string]bool{}}
	importer := &Importer{
		Backends: []ports.CameraBackend{
			fakeBackend{cameras: []domain.Camera{{Model: "Unsupported", Port: "usb:001"}}},
			fakeBackend{cameras: []domain.Camera{knownCard}, files: []domain.RemoteFile{file}},
		},
		State:  state,
		Config: DefaultConfig(),
		DryRun: true,
	}

	got, err := importer.Import(context.Background())
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	want := []domain.ImportResult{{Camera: knownCard, Seen: 1, Planned: []domain.RemoteFile{file}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Import() = %#v, want %#v", got, want)
	}
	if state.saved {
		t.Fatal("dry run saved state")
	}
}

func TestImporterImportCopiesFromSelectedBackend(t *testing.T) {
	camera := domain.Camera{Model: "NIKON Z6_3", Port: "/Volumes/NIKON Z6_3"}
	file := domain.RemoteFile{Folder: "/DCIM/100NZ6_3", Name: "DSC_0001.NEF", SizeBytes: 42}
	state := &fakeState{imported: map[string]bool{}}
	backend := fakeBackend{cameras: []domain.Camera{camera}, files: []domain.RemoteFile{file}, content: []byte("raw")}
	importer := &Importer{
		Backends: []ports.CameraBackend{backend},
		State:    state,
		Config: Config{
			BaseDir:           t.TempDir(),
			FolderLayout:      "2006-01-02",
			IncludeExtensions: []string{".nef"},
		},
	}

	got, err := importer.Import(context.Background())
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	if len(got) != 1 || got[0].Downloaded != 1 {
		t.Fatalf("Downloaded = %#v, want one downloaded file", got)
	}
	if !state.saved {
		t.Fatal("state was not saved")
	}
	data, err := os.ReadFile(got[0].Records[0].Destination)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != "raw" {
		t.Fatalf("downloaded content = %q, want %q", string(data), "raw")
	}
}

type fakeBackend struct {
	cameras []domain.Camera
	files   []domain.RemoteFile
	content []byte
}

func (b fakeBackend) Detect(context.Context) ([]domain.Camera, error) {
	return b.cameras, nil
}

func (b fakeBackend) ListFiles(context.Context, domain.Camera) ([]domain.RemoteFile, error) {
	return b.files, nil
}

func (b fakeBackend) Download(_ context.Context, _ domain.Camera, file domain.RemoteFile, destination string) error {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destination, b.content, 0o644)
}

type fakeState struct {
	imported map[string]bool
	recorded []domain.ImportRecord
	saved    bool
}

func (s *fakeState) IsImported(fileKey string) bool {
	return s.imported[fileKey]
}

func (s *fakeState) Record(record domain.ImportRecord) error {
	s.recorded = append(s.recorded, record)
	s.imported[record.Key()] = true
	return nil
}

func (s *fakeState) Save() error {
	s.saved = true
	return nil
}
