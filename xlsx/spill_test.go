package xlsx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name] = string(b)
	}
	return files
}

func dynamicBook(t *testing.T) (*Document, *ot.Tree, string) {
	t.Helper()
	data, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	id := firstSheet(tree).ID
	c := NewCalc(tree, formula.Options{})
	e := ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{
		cell(1, 1, `{"f":"SEQUENCE(3)"}`),
		cell(1, 3, `{"v":3}`), cell(2, 3, `{"v":1}`), cell(3, 3, `{"v":2}`),
		cell(1, 5, `{"f":"_xlfn._xlws.SORT(C1:C3)"}`),
		cell(1, 7, `{"f":"_xlfn._xlws.FILTER(C1:C3,C1:C3-1)"}`),
		cell(1, 8, `{"f":"SUM(A1#)"}`),
	}}}
	apply(t, d, tree, e)
	c.Follow(e, nil)
	return d, tree, id
}

// Dynamic formulas keep their spilled area, their metadata and the values
// of the cells they fill through a save, and the package describes them
// the way Excel reads them.
func TestSpillRoundTrip(t *testing.T) {
	d, tree, id := dynamicBook(t)
	want := map[[2]int]string{
		{1, 1}: `{"cm":1,"f":"_xlfn.SEQUENCE(3)","fa":"A1:A3","v":1}`,
		{2, 1}: `{"v":2}`,
		{3, 1}: `{"v":3}`,
		{1, 5}: `{"cm":1,"f":"_xlfn._xlws.SORT(C1:C3)","fa":"E1:E3","v":1}`,
		{2, 5}: `{"v":2}`,
		{3, 5}: `{"v":3}`,
		{1, 7}: `{"cm":1,"f":"_xlfn._xlws.FILTER(C1:C3,C1:C3-1)","fa":"G1:G2","v":3}`,
		{2, 7}: `{"v":2}`,
		{1, 8}: `{"f":"SUM(_xlfn.ANCHORARRAY(A1))","v":6}`,
	}
	for p, w := range want {
		if got := string(tree.Node(id).Grid.Cell(p[0], p[1])); got != w {
			t.Fatalf("cell %v typed: %s, want %s", p, got, w)
		}
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	d2, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	for p, w := range want {
		if got := string(again.Node(firstSheet(again).ID).Grid.Cell(p[0], p[1])); got != w {
			t.Errorf("cell %v read back: %s, want %s", p, got, w)
		}
	}
	reread := NewCalc(again, formula.Options{})
	more := ot.Edit{{Op: ot.OpCel, ID: firstSheet(again).ID, Cells: []ot.Cell{cell(1, 3, `{"v":6}`)}}}
	apply(t, d2, again, more)
	reread.Follow(more, nil)
	if got := string(again.Node(firstSheet(again).ID).Grid.Cell(3, 5)); got != `{"v":6}` {
		t.Errorf("E3 after C1 changed in the reopened book: %s", got)
	}

	files := unzip(t, saved)
	meta := files["xl/metadata.xml"]
	if !strings.Contains(meta, `name="XLDAPR"`) {
		t.Errorf("metadata.xml: %s", meta)
	}
	if rels := files["xl/_rels/workbook.xml.rels"]; strings.Count(rels, relSheetMetadata) != 1 {
		t.Errorf("workbook relations: %s", rels)
	}
	if types := files["[Content_Types].xml"]; strings.Count(types, typeSheetMetadata) != 1 {
		t.Errorf("content types: %s", types)
	}
	var sheet string
	for name, x := range files {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.Contains(x, "SEQUENCE") {
			sheet = x
		}
	}
	for _, s := range []string{`cm="1"`, `<f t="array" ref="A1:A3">_xlfn.SEQUENCE(3)</f>`, `<f t="array" ref="E1:E3">_xlfn._xlws.SORT(C1:C3)</f>`,
		`<f t="array" ref="G1:G2">_xlfn._xlws.FILTER(C1:C3,C1:C3-1)</f>`, `_xlfn.ANCHORARRAY(A1)`} {
		if !strings.Contains(sheet, s) {
			t.Errorf("sheet lacks %s: %s", s, sheet)
		}
	}

	twice, err := d2.Save(again)
	if err != nil {
		t.Fatal(err)
	}
	files = unzip(t, twice)
	n := 0
	for name := range files {
		if strings.HasPrefix(name, "xl/metadata") {
			n++
		}
	}
	if n != 1 || strings.Count(files["xl/_rels/workbook.xml.rels"], relSheetMetadata) != 1 || strings.Count(files["[Content_Types].xml"], typeSheetMetadata) != 1 {
		t.Errorf("saved twice: %d metadata parts, relations %s", n, files["xl/_rels/workbook.xml.rels"])
	}
	if strings.Count(files["xl/metadata.xml"], "<metadataType ") != 1 {
		t.Errorf("metadata.xml saved twice: %s", files["xl/metadata.xml"])
	}
	_, third, err := Open(twice)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(json.RawMessage(third.Node(firstSheet(third).ID).Grid.Cell(1, 1))); got != want[[2]int{1, 1}] {
		t.Errorf("A1 after two saves: %s", got)
	}
}
