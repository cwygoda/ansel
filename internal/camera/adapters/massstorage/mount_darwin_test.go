//go:build darwin

package massstorage

import (
	"reflect"
	"testing"
)

func TestParseExternalVolumes(t *testing.T) {
	out := `/dev/disk8 (external, physical):
   #:                       TYPE NAME                    SIZE       IDENTIFIER
   0:     FDisk_partition_scheme                        *512.1 GB   disk8
   1:               Windows_NTFS NIKON Z6_3              512.1 GB   disk8s1
/dev/disk9 (external, physical):
   #:                       TYPE NAME                    SIZE       IDENTIFIER
   0:      GUID_partition_scheme                        *128.0 GB   disk9
   1:                        EFI EFI                     209.7 MB   disk9s1
   2:                  Apple_HFS Travel Photos           127.7 GB   disk9s2
`

	got := parseExternalVolumes(out)
	want := []externalVolume{
		{Name: "NIKON Z6_3", Identifier: "disk8s1"},
		{Name: "EFI", Identifier: "disk9s1"},
		{Name: "Travel Photos", Identifier: "disk9s2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseExternalVolumes() = %#v, want %#v", got, want)
	}
}
