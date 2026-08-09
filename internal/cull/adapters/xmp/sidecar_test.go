package xmp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cwygoda/ansel/internal/cull/domain"
)

func TestRenderProducesExpectedPacket(t *testing.T) {
	plan := domain.SidecarPlan{
		Rating: 5,
		Label:  "green",
		Tags:   []string{"sharp", "best_in_group"},
	}

	got, err := Render(plan)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := "<?xpacket begin=\"\ufeff\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n" +
		`<x:xmpmeta xmlns:x="adobe:ns:meta/" x:xmptk="ansel cull">
 <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">
  <rdf:Description rdf:about=""
    xmlns:xmp="http://ns.adobe.com/xap/1.0/"
    xmlns:dc="http://purl.org/dc/elements/1.1/"
    xmp:Rating="5"
    xmp:Label="green">
   <dc:subject>
    <rdf:Bag>
     <rdf:li>sharp</rdf:li>
     <rdf:li>best_in_group</rdf:li>
    </rdf:Bag>
   </dc:subject>
  </rdf:Description>
 </rdf:RDF>
</x:xmpmeta>
<?xpacket end="w"?>
`

	if string(got) != want {
		t.Errorf("Render produced:\n%s\nexpected:\n%s", got, want)
	}
}

func TestRenderOmitsEmptySections(t *testing.T) {
	got, err := Render(domain.SidecarPlan{Rating: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rendered := string(got)
	if strings.Contains(rendered, "xmp:Label") {
		t.Error("an empty label was still written")
	}
	if strings.Contains(rendered, "dc:subject") {
		t.Error("an empty keyword bag was still written")
	}
	if !strings.Contains(rendered, `xmp:Rating="0"`) {
		t.Error("rating is missing")
	}
}

// Tag names come from constants today, but a sidecar must never be able to
// emit malformed XML if that changes.
func TestRenderEscapesMarkup(t *testing.T) {
	got, err := Render(domain.SidecarPlan{Tags: []string{`a<b>&"c`}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(string(got), "<b>") {
		t.Errorf("unescaped markup leaked into the packet:\n%s", got)
	}
}

func TestWriteIsAtomicAndLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "DSC_1234.xmp")

	err := New().Write(domain.SidecarPlan{
		ImagePath:   filepath.Join(dir, "DSC_1234.NEF"),
		SidecarPath: sidecar,
		Rating:      5,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := os.Stat(sidecar); err != nil {
		t.Fatalf("sidecar was not created: %v", err)
	}
	if _, err := os.Stat(sidecar + ".tmp"); !os.IsNotExist(err) {
		t.Error("a temporary file was left behind")
	}
}
