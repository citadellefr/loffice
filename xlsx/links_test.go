package xlsx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func openBlank(t *testing.T) (*Document, *ot.Tree, string) {
	t.Helper()
	data, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree, firstSheet(tree).ID
}

func reopen(t *testing.T, d *Document, tree *ot.Tree) (*opc.Package, *ot.Tree) {
	t.Helper()
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := Open(saved, Limits{})
	if err != nil {
		t.Fatalf("edited workbook does not open: %v", err)
	}
	pkg, err := opc.Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return pkg, again
}

func sheetXML(t *testing.T, pkg *opc.Package) (xml, rels string) {
	t.Helper()
	d, _ := pkg.Read("xl/worksheets/sheet1.xml")
	r, _ := pkg.Read("xl/worksheets/_rels/sheet1.xml.rels")
	return string(d), string(r)
}

func TestMergeEdited(t *testing.T) {
	d, tree, sheet := openBlank(t)
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{
		cell(2, 2, `{"v":"titre","m":[1,3]}`), cell(4, 1, `{"v":1,"m":[2,2]}`),
	}}})
	pkg, again := reopen(t, d, tree)
	if got := string(firstSheet(again).Grid.Cell(2, 2)); got != `{"m":[1,3],"v":"titre"}` {
		t.Fatalf("merged cell read back as %s", got)
	}
	xml, _ := sheetXML(t, pkg)
	if !strings.Contains(xml, `<mergeCells count="2"><mergeCell ref="B2:D2"/><mergeCell ref="A4:B5"/></mergeCells>`) {
		t.Fatalf("merged cells written as %s", xml)
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{cell(2, 2, `{"m":null}`)}}})
	pkg, again = reopen(t, d, tree)
	if xml, _ = sheetXML(t, pkg); !strings.Contains(xml, `<mergeCells count="1"><mergeCell ref="A4:B5"/></mergeCells>`) {
		t.Fatalf("merged cells after splitting one: %s", xml)
	}
}

func TestMergeOverlap(t *testing.T) {
	d, tree, sheet := openBlank(t)
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{cell(2, 2, `{"m":[2,2]}`)}}})
	for name, cells := range map[string][]ot.Cell{
		"inside":       {cell(3, 3, `{"m":[1,2]}`)},
		"around":       {cell(1, 1, `{"m":[4,4]}`)},
		"corner":       {cell(3, 3, `{"m":[2,2]}`), cell(1, 1, `{"m":[2,2]}`)},
		"together":     {cell(8, 8, `{"m":[2,2]}`), cell(9, 9, `{"m":[2,2]}`)},
		"out of sheet": {cell(1048576, 1, `{"m":[2,1]}`)},
	} {
		if err := d.Check(tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: cells}}, ""); err != ErrReadOnly {
			t.Errorf("%s: got %v", name, err)
		}
	}
	for name, cells := range map[string][]ot.Cell{
		"beside": {cell(2, 4, `{"m":[2,2]}`)},
		"below":  {cell(4, 2, `{"m":[1,2]}`)},
		"again":  {cell(2, 2, `{"m":[3,3]}`)},
		"moved":  {cell(2, 2, `{"m":null}`), cell(3, 3, `{"m":[2,2]}`)},
	} {
		if err := d.Check(tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: cells}}, ""); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	added := ot.Edit{{Op: ot.OpNew, ID: "n", Type: "sheet", Parent: "book", Key: "z", Cells: []ot.Cell{
		cell(1, 1, `{"m":[2,2]}`), cell(2, 2, `{"m":[2,2]}`)}}}
	if err := d.Check(tree, added, ""); err != ErrReadOnly {
		t.Errorf("overlap in a new sheet: got %v", err)
	}
}

func TestLinkEdited(t *testing.T) {
	d, tree, sheet := openBlank(t)
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{
		cell(1, 1, `{"v":"Site","link":{"url":"https://example.com/a?b=1","tip":"Ouvrir"}}`),
		cell(2, 1, `{"v":"Écrire","link":{"url":"mailto:a@example.com"}}`),
		cell(3, 1, `{"link":{"loc":"Feuil1!B2"}}`),
		cell(4, 1, `{"link":{"url":"https://example.com/a?b=1"}}`),
	}}})
	pkg, again := reopen(t, d, tree)
	xml, rels := sheetXML(t, pkg)
	for _, want := range []string{
		`<hyperlinks><hyperlink ref="A1" r:id="rId1" tooltip="Ouvrir"/><hyperlink ref="A2" r:id="rId2"/>` +
			`<hyperlink ref="A3" location="Feuil1!B2"/><hyperlink ref="A4" r:id="rId1"/></hyperlinks>`,
		`xmlns:r="`,
	} {
		if !strings.Contains(xml, want) {
			t.Errorf("worksheet lacks %s: %s", want, xml)
		}
	}
	for _, want := range []string{
		`Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/a?b=1" TargetMode="External"`,
		`Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="mailto:a@example.com" TargetMode="External"`,
	} {
		if !strings.Contains(rels, want) {
			t.Errorf("relationships lack %s: %s", want, rels)
		}
	}
	if got := string(firstSheet(again).Grid.Cell(1, 1)); got != `{"link":{"url":"https://example.com/a?b=1","tip":"Ouvrir"},"v":"Site"}` {
		t.Errorf("link read back as %s", got)
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{
		cell(1, 1, `{"link":{"url":"https://example.org/"}}`), cell(2, 1, `{"link":null}`), cell(4, 1, `{"link":null}`),
	}}})
	pkg, again = reopen(t, d, tree)
	xml, rels = sheetXML(t, pkg)
	if !strings.Contains(xml, `<hyperlinks><hyperlink ref="A1" r:id="rId1"/><hyperlink ref="A3" location="Feuil1!B2"/></hyperlinks>`) {
		t.Errorf("links after the edit: %s", xml)
	}
	if strings.Contains(rels, "mailto") || strings.Contains(rels, "example.com") || !strings.Contains(rels, "https://example.org/") {
		t.Errorf("relationships after the edit: %s", rels)
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{cell(1, 1, `{"link":null}`), cell(3, 1, `{"link":null}`)}}})
	pkg, again = reopen(t, d, tree)
	xml, rels = sheetXML(t, pkg)
	if strings.Contains(xml, "hyperlink") || rels != "" {
		t.Errorf("links left after all were removed: %s %s", xml, rels)
	}
}

func TestLinkChecked(t *testing.T) {
	for raw, ok := range map[string]bool{
		`{"url":"https://example.com"}`:               true,
		`{"url":"mailto:a@example.com","tip":"Mail"}`: true,
		`{"url":"urn:isbn:123"}`:                      true,
		`{"loc":"Feuil2!A1"}`:                         true,
		`{"url":"https://example.com","loc":"A1"}`:    true,
		`{}`:                                         false,
		`{"url":"javascript:alert(1)"}`:              false,
		`{"url":"https://"}`:                         false,
		`{"url":"file:///etc/passwd"}`:               false,
		`{"loc":"A\u0000"}`:                          false,
		`{"tip":"seul"}`:                             false,
		`{"url":"https://example.com","x":1}`:        false,
		`"https://example.com"`:                      false,
		`{"loc":"` + strings.Repeat("a", 256) + `"}`: false,
	} {
		_, got := parseLink(json.RawMessage(raw))
		if got != ok {
			t.Errorf("%s: got %v", raw, got)
		}
	}
}

// A workbook as Excel writes links is read as cells with links, and written
// back with them, those it does not model kept.
func TestLinkRead(t *testing.T) {
	d, tree, sheet := openBlank(t)
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{
		cell(1, 1, `{"link":{"url":"https://example.com/"}}`), cell(2, 1, `{"link":{"loc":"B2"}}`),
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _ := opc.Open(saved, opc.Limits{})
	data, _ := pkg.Read("xl/worksheets/sheet1.xml")
	file := strings.Replace(string(data), `</hyperlinks>`,
		`<hyperlink ref="C1:C2" location="A1"/><hyperlink ref="E1" r:id="rId9"/><hyperlink ref="F1:F5000" location="A1"/></hyperlinks>`, 1)
	rels, _ := pkg.Read("xl/worksheets/_rels/sheet1.xml.rels")
	rels = []byte(strings.Replace(string(rels), `</Relationships>`,
		`<Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="file:///c:/a.xlsx" TargetMode="External"/></Relationships>`, 1))
	_ = pkg.Set("xl/worksheets/sheet1.xml", []byte(file))
	_ = pkg.Set("xl/worksheets/_rels/sheet1.xml.rels", rels)
	data, _ = pkg.Bytes()

	d, tree, err = Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	g := firstSheet(tree).Grid
	for at, want := range map[[2]int]string{{1, 1}: "example.com", {2, 1}: `"loc":"B2"`, {1, 3}: `"loc":"A1"`, {2, 3}: `"loc":"A1"`} {
		if got := string(g.Cell(at[0], at[1])); !strings.Contains(got, want) {
			t.Errorf("cell %v read as %s", at, got)
		}
	}
	for _, at := range [][2]int{{1, 5}, {1, 6}} {
		if got := g.Cell(at[0], at[1]); got != nil {
			t.Errorf("cell %v holds %s, its link is kept apart", at, got)
		}
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: firstSheet(tree).ID, Cells: []ot.Cell{cell(1, 1, `{"link":null}`), cell(2, 3, `{"link":null}`)}}})
	pkg, _ = reopen(t, d, tree)
	xml, relXML := sheetXML(t, pkg)
	if !strings.Contains(xml, `<hyperlink ref="E1" r:id="rId9"/><hyperlink ref="F1:F5000" location="A1"/><hyperlink ref="C1" location="A1"/><hyperlink ref="A2" location="B2"/>`) {
		t.Errorf("links written as %s", xml)
	}
	if !strings.Contains(relXML, "file:///c:/a.xlsx") || strings.Contains(relXML, "example.com") {
		t.Errorf("relationships written as %s", relXML)
	}
}

// Links follow their cells when rows are inserted.
func TestLinkMoved(t *testing.T) {
	d, tree, sheet := openBlank(t)
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{cell(2, 1, `{"link":{"url":"https://example.com/"}}`)}}})
	if err := moveRows(t, d, tree, ot.Edit{{Op: ot.OpIns, ID: sheet, Dim: ot.DimRows, At: 1, N: 3}}); err != nil {
		t.Fatal(err)
	}
	pkg, _ := reopen(t, d, tree)
	if xml, _ := sheetXML(t, pkg); !strings.Contains(xml, `<hyperlink ref="A5" r:id="rId1"/>`) {
		t.Errorf("link after the rows moved: %s", xml)
	}
}
