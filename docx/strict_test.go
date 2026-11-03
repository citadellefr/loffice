package docx

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

// rawParts are the parts of a package as stored.
func rawParts(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string][]byte{}
	for _, f := range z.File {
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		parts[f.Name], _ = io.ReadAll(r)
		r.Close()
	}
	return parts
}

func zipOf(t *testing.T, parts map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for name, data := range parts {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write(data)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func openFile(t *testing.T, name string) (*Document, *ot.Tree, []byte) {
	t.Helper()
	data, err := os.ReadFile("../testdata/docx/" + name)
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree, data
}

func TestStrict(t *testing.T) {
	d, tree, data := openFile(t, "strict.docx")
	_, want, _ := openFile(t, "transitional.docx")
	if err := sameTree(want, tree); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, data) {
		t.Fatal("unedited document saved differently")
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: firstText(t, tree).ID, Text: ot.Delta{{Insert: "Écrit en "}}}})
	saved, err = d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	before, after := rawParts(t, data), rawParts(t, saved)
	doc := string(after["word/document.xml"])
	if !strings.Contains(doc, `"http://purl.oclc.org/ooxml/wordprocessingml/main"`) || strings.Contains(doc, "schemas.openxmlformats.org") || !strings.Contains(doc, `w:conformance="strict"`) || !strings.Contains(doc, "Écrit en ") {
		t.Fatalf("document not written as Strict:\n%s", doc)
	}
	for name := range before {
		if name != "word/document.xml" && !bytes.Equal(before[name], after[name]) {
			t.Errorf("%s changed", name)
		}
	}
	_, again, err := Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sameTree(tree, again); err != nil {
		t.Fatal(err)
	}
}

func TestNeitherStrictNorTransitional(t *testing.T) {
	data, err := os.ReadFile("../testdata/docx/transitional.docx")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ part, old string }{
		{"word/document.xml", `"http://schemas.openxmlformats.org/wordprocessingml/2006/main"`},
		{"_rels/.rels", `"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"`},
	} {
		parts := rawParts(t, data)
		parts[c.part] = bytes.ReplaceAll(parts[c.part], []byte(c.old), []byte(`"urn:other"`))
		if _, _, err := Open(zipOf(t, parts), opc.Limits{}); !errors.Is(err, ErrNotDocument) || err.Error() != "docx: not a Word document" {
			t.Errorf("%s changed: %v", c.part, err)
		}
	}
}
