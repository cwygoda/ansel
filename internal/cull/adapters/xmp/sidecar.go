// Package xmp writes analysis results as XMP sidecar files.
//
// XMP is an interoperability projection, never the source of truth: the store
// keeps every observation, while a sidecar carries only what another
// application can act on. Photographs themselves are never modified.
package xmp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

// Toolkit identifies the writer inside the packet, so it is obvious later
// which tool produced a sidecar.
const Toolkit = "ansel cull"

// xpacketBegin carries the byte order mark that the XMP specification requires
// at the head of a packet. It is written as an escape because Go rejects a
// literal BOM inside a source file.
const xpacketBegin = "<?xpacket begin=\"\ufeff\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n"

var sidecarTemplate = template.Must(template.New("xmp").Parse(
	xpacketBegin +
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="{{.Toolkit}}">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmp:Rating="{{.Rating}}"{{if .Label}}
    xmp:Label="{{.Label}}"{{end}}>
{{- if .Tags}}
   <dc:subject>
    <rdf:Bag>
{{- range .Tags}}
     <rdf:li>{{.}}</rdf:li>
{{- end}}
    </rdf:Bag>
   </dc:subject>
{{- end}}
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>
`))

type templateData struct {
	Toolkit string
	Rating  int
	Label   string
	Tags    []string
}

// Writer emits XMP sidecars beside the photographs they describe.
type Writer struct{}

// New returns a Writer.
func New() Writer { return Writer{} }

// Write renders one sidecar. The write is atomic — a temporary file followed
// by a rename — so an interrupted run can never leave a half-written sidecar
// where a photo application will try to read one.
func (Writer) Write(plan domain.SidecarPlan) error {
	content, err := Render(plan)
	if err != nil {
		return err
	}

	tmp := plan.SidecarPath + ".tmp"
	if err := os.WriteFile(tmp, content, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", filepath.Base(tmp), err)
	}
	if err := os.Rename(tmp, plan.SidecarPath); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("failed to replace %s: %w", filepath.Base(plan.SidecarPath), err)
	}
	return nil
}

// Render produces the sidecar content. It is exported so the exact bytes can
// be asserted in tests without touching the filesystem.
func Render(plan domain.SidecarPlan) ([]byte, error) {
	var out bytes.Buffer
	data := templateData{
		Toolkit: Toolkit,
		Rating:  plan.Rating,
		Label:   escape(plan.Label),
		Tags:    escapeAll(plan.Tags),
	}
	if err := sidecarTemplate.Execute(&out, data); err != nil {
		return nil, fmt.Errorf("failed to render sidecar: %w", err)
	}
	return out.Bytes(), nil
}

func escapeAll(values []string) []string {
	escaped := make([]string, 0, len(values))
	for _, value := range values {
		escaped = append(escaped, escape(value))
	}
	return escaped
}

// escape guards the packet against characters that would break the XML, even
// though present tag names are all generated from constants.
func escape(value string) string {
	var out bytes.Buffer
	if err := xml.EscapeText(&out, []byte(value)); err != nil {
		return ""
	}
	return out.String()
}
