package xlsx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

// move applies rows or columns inserted or removed as the hub does, the
// book told how many moves the file saw.
func moveRows(t *testing.T, d *Document, tree *ot.Tree, e ot.Edit) error {
	t.Helper()
	if err := d.Check(tree, e); err != nil {
		return err
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	n := d.Moved(e)
	return tree.Apply(ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{movesKey: json.RawMessage(strconv.Itoa(n))}}})
}

func TestMovedFormats(t *testing.T) {
	data, err := os.ReadFile("../corpus/files/poi/xssf-enum.xltx.xlsx")
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	if err := moveRows(t, d, tree, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 3}}); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _ := opc.Open(saved, Limits)
	sheet, _ := pkg.Read("xl/worksheets/sheet1.xml")
	for _, want := range []string{`<conditionalFormatting sqref="C7:C25">`, `<formula>LEN(B7)&gt;0</formula>`, `sqref="E7:E25"`} {
		if !strings.Contains(string(sheet), want) {
			t.Errorf("no %s in the sheet", want)
		}
	}
}

// Every workbook of the corpus takes rows inserted and a column removed.
// When LOFFICE_MOVED is set, the packages are written there for the
// validator.
func TestCorpusMoves(t *testing.T) {
	out := os.Getenv("LOFFICE_MOVED")
	moved := 0
	for _, f := range workbooks(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data)
		if err != nil {
			continue
		}
		var sheet string
		for _, n := range tree.Children("book") {
			if n.Type == "sheet" {
				sheet = n.ID
				break
			}
		}
		if sheet == "" {
			continue
		}
		for _, e := range []ot.Edit{
			{{Op: ot.OpIns, ID: sheet, Dim: ot.DimRows, At: 3, N: 2}},
			{{Op: ot.OpRem, ID: sheet, Dim: ot.DimCols, At: 2, N: 1}},
		} {
			if err := moveRows(t, d, tree, e); err != nil && !errors.Is(err, ErrTable) {
				t.Fatalf("%s: %v", f, err)
			}
		}
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		moved++
		if _, _, err := Open(saved); err != nil {
			t.Fatalf("%s: moved workbook does not open: %v", f, err)
		}
		if out != "" {
			rel, _ := filepath.Rel(filepath.Join("..", "corpus", "files"), f)
			dst := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, saved, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("%d workbooks moved", moved)
}
