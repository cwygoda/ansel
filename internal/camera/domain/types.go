package domain

import (
	"fmt"
	"strings"
	"time"
)

// KnownCamera describes a camera Ansel knows how to target automatically.
type KnownCamera struct {
	Name      string
	VendorID  int
	ProductID int
	Aliases   []string
}

var KnownCameras = []KnownCamera{
	{
		Name:      "Nikon Z6 III",
		VendorID:  0x04b0,
		ProductID: 0x0454,
		Aliases:   []string{"nikon:z6 iii", "nikon z6 iii", "nikon z6_3", "nikon dsc z6_3", "z6 iii", "z6_3"},
	},
	{
		Name:      "Ricoh GR IIIx",
		VendorID:  0x25fb,
		ProductID: 0x2115,
		Aliases:   []string{"ricoh:gr iiix", "ricoh gr iiix", "gr iiix"},
	},
}

// Camera is a camera visible to an adapter.
type Camera struct {
	Model     string
	Port      string
	VendorID  int
	ProductID int
}

// Key returns a stable-enough key for bookmark state.
func (c Camera) Key() string {
	if c.VendorID != 0 || c.ProductID != 0 {
		return fmt.Sprintf("%04x:%04x:%s", c.VendorID, c.ProductID, c.Model)
	}
	return c.Model
}

func (c Camera) KnownName() string {
	model := strings.ToLower(c.Model)
	for _, known := range KnownCameras {
		if c.VendorID == known.VendorID && c.ProductID == known.ProductID {
			return known.Name
		}
		for _, alias := range known.Aliases {
			if strings.Contains(model, alias) {
				return known.Name
			}
		}
	}
	return ""
}

func (c Camera) IsKnown() bool { return c.KnownName() != "" }

// RemoteFile is an importable file on a camera.
type RemoteFile struct {
	Folder    string
	Name      string
	SizeBytes int64
	MIMEType  string
}

func (f RemoteFile) Key(cameraKey string) string {
	return cameraKey + "|" + f.Folder + "|" + f.Name + "|" + fmt.Sprint(f.SizeBytes)
}

// ImportRecord is persisted after a successful import.
type ImportRecord struct {
	CameraKey    string    `json:"camera_key"`
	CameraModel  string    `json:"camera_model"`
	Folder       string    `json:"folder"`
	Name         string    `json:"name"`
	SizeBytes    int64     `json:"size_bytes"`
	Destination  string    `json:"destination"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

func (r ImportRecord) Key() string {
	return RemoteFile{Folder: r.Folder, Name: r.Name, SizeBytes: r.SizeBytes}.Key(r.CameraKey)
}

// ImportResult summarizes one import run.
type ImportResult struct {
	Camera     Camera
	Seen       int
	Downloaded int
	Skipped    int
	Planned    []RemoteFile
	Records    []ImportRecord
}
