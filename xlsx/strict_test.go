package xlsx

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/trame/ot"
)

const (
	strictMain       = `"http://purl.oclc.org/ooxml/spreadsheetml/main"`
	transitionalMain = `"http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
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
	data, err := os.ReadFile("../testdata/xlsx/" + name)
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
	d, tree, data := openFile(t, "strict.xlsx")
	_, want, _ := openFile(t, "transitional.xlsx")
	if err := sameTree(want, tree); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, data) {
		t.Fatal("unedited workbook saved differently")
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(2, 2, `{"s":"x2","v":45366.75}`), cell(2, 3, `{"v":7}`), cell(3, 1, `{"s":"x1","v":60}`),
	}}})
	saved, err = d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	before, after := rawParts(t, data), rawParts(t, saved)
	for _, name := range []string{"xl/workbook.xml", "xl/worksheets/sheet1.xml", "xl/sharedStrings.xml"} {
		if part := string(after[name]); !strings.Contains(part, strictMain) || strings.Contains(part, transitionalMain) {
			t.Fatalf("%s not written as Strict:\n%s", name, part)
		}
	}
	sheet := string(after["xl/worksheets/sheet1.xml"])
	for _, c := range []string{`<c r="A1" s="1" t="d"><v>2024-03-15</v></c>`, `<c r="B2" s="2" t="d"><v>2024-03-15T18:00:00</v></c>`, `<c r="C2"><v>7</v></c>`, `<c r="A3" s="1"><v>60</v></c>`} {
		if !strings.Contains(sheet, c) {
			t.Errorf("no %s in\n%s", c, sheet)
		}
	}
	for _, name := range []string{"xl/styles.xml", "_rels/.rels"} {
		if !bytes.Equal(before[name], after[name]) {
			t.Errorf("%s changed", name)
		}
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := sameTree(tree, again); err != nil {
		t.Fatal(err)
	}
}

func TestISODates(t *testing.T) {
	for _, c := range []struct {
		iso      string
		date1904 bool
		serial   float64
	}{
		{"1900-01-01", false, 1},
		{"1900-02-28", false, 59},
		{"1900-03-01", false, 61},
		{"2024-03-15", false, 45366},
		{"2024-03-15T06:00:00", false, 45366.25},
		{"2024-03-15T06:00:00.5", false, 45366.25 + 0.5/86400},
		{"1904-01-02", true, 1},
	} {
		n, ok := isoSerial(c.iso, c.date1904)
		if !ok || n != c.serial {
			t.Errorf("isoSerial(%s) = %v, %v", c.iso, n, ok)
		}
		if s, ok := isoDate(c.serial, c.date1904); !ok || s != c.iso {
			t.Errorf("isoDate(%v) = %s, %v", c.serial, s, ok)
		}
	}
	for _, s := range []string{"2024-03-15T06:00:00Z", "2024-03-15T06:00:00+02:00"} {
		if n, _ := isoSerial(s, false); n != 45366.25 {
			t.Errorf("isoSerial(%s) = %v", s, n)
		}
	}
	if n, _ := isoSerial("18:00:00", false); n != 0.75 {
		t.Errorf("time alone read as %v", n)
	}
	for _, n := range []float64{0.75, 60, -1, 1e7} {
		if s, ok := isoDate(n, false); ok {
			t.Errorf("isoDate(%v) = %s", n, s)
		}
	}
}

func TestNeitherStrictNorTransitional(t *testing.T) {
	data, err := os.ReadFile("../testdata/xlsx/transitional.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ part, old string }{
		{"xl/workbook.xml", transitionalMain},
		{"_rels/.rels", `"http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"`},
	} {
		parts := rawParts(t, data)
		parts[c.part] = bytes.ReplaceAll(parts[c.part], []byte(c.old), []byte(`"urn:other"`))
		if _, _, err := Open(zipOf(t, parts)); !errors.Is(err, ErrNotWorkbook) || err.Error() != "xlsx: not a workbook" {
			t.Errorf("%s changed: %v", c.part, err)
		}
	}
}
