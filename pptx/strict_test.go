package pptx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

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

// sameTree tells how two trees differ, nil if they do not.
func sameTree(a, b *ot.Tree) error {
	x, _ := json.Marshal(a.Edit())
	y, _ := json.Marshal(b.Edit())
	at := 0
	for at < min(len(x), len(y)) && x[at] == y[at] {
		at++
	}
	if at < max(len(x), len(y)) {
		from := max(at-200, 0)
		return errors.New("trees differ:\n" + string(x[from:min(at+200, len(x))]) + "\n" + string(y[from:min(at+200, len(y))]))
	}
	return nil
}

func openFile(t *testing.T, name string) (*Document, *ot.Tree, []byte) {
	t.Helper()
	data, err := os.ReadFile("../testdata/pptx/" + name)
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	return d, tree, data
}

func TestStrict(t *testing.T) {
	d, tree, data := openFile(t, "strict.pptx")
	_, want, _ := openFile(t, "transitional.pptx")
	if err := sameTree(want, tree); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, data) {
		t.Fatal("unedited presentation saved differently")
	}

	slide := slidesOf(tree)[0].ID
	edit := func(d *Document, tree *ot.Tree) []byte {
		shape := firstText(tree, slide).ID
		apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: shape, Text: ot.Delta{{Insert: "Écrit en "}}}})
		apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: shape, Attrs: ot.Values{"fill": json.RawMessage(`{"solid":{"scheme":"accent2","mods":[["lumMod",50000],["lumOff",12500]]}}`)}}})
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	saved = edit(d, tree)
	before, after := rawParts(t, data), rawParts(t, saved)
	xml := string(after["ppt/slides/slide1.xml"])
	if !strings.Contains(xml, `"http://purl.oclc.org/ooxml/presentationml/main"`) || strings.Contains(xml, "schemas.openxmlformats.org") || !strings.Contains(xml, "Écrit en ") ||
		!strings.Contains(xml, `<a:lumMod val="50%"/><a:lumOff val="12.5%"/>`) || !strings.Contains(xml, `<a:spcPct val="120%"/>`) {
		t.Fatalf("slide not written as Strict:\n%s", xml)
	}
	for name := range before {
		if name != "ppt/slides/slide1.xml" && !bytes.Equal(before[name], after[name]) {
			t.Errorf("%s changed", name)
		}
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	td, transitional, _ := openFile(t, "transitional.pptx")
	_, want, err = Open(edit(td, transitional))
	if err != nil {
		t.Fatal(err)
	}
	if err := sameTree(want, again); err != nil {
		t.Fatal(err)
	}
}

func TestNeitherStrictNorTransitional(t *testing.T) {
	data, err := os.ReadFile("../testdata/pptx/transitional.pptx")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ part, old string }{
		{"ppt/presentation.xml", `"http://schemas.openxmlformats.org/presentationml/2006/main"`},
		{"_rels/.rels", `"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"`},
	} {
		parts := rawParts(t, data)
		parts[c.part] = bytes.ReplaceAll(parts[c.part], []byte(c.old), []byte(`"urn:other"`))
		if _, _, err := Open(zipOf(t, parts)); !errors.Is(err, ErrNotPresentation) || err.Error() != "pptx: not a presentation" {
			t.Errorf("%s changed: %v", c.part, err)
		}
	}
}
