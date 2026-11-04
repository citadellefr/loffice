package xlsx

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// LET, LAMBDA and the functions that take one are stored with the
// prefixes Excel gives them, kept through a save, and calculated again
// once the workbook is reopened.
func TestLambdaRoundTrip(t *testing.T) {
	data, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	sheet := firstSheet(tree)
	id := sheet.ID
	names := `[{"name":"Scale","ref":"_xlfn.LAMBDA(_xlpm.x,_xlpm.x*'` + str(sheet, "name") + `'!$A$1)"},` +
		`{"name":"Fact2","ref":"_xlfn.LAMBDA(_xlpm.n,IF(_xlpm.n<=1,1,_xlpm.n*Fact2(_xlpm.n-1)))"}]`
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"names": json.RawMessage(names)}}})
	c := NewCalc(tree, formula.Options{})
	e := ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{
		cell(1, 1, `{"v":3}`), cell(2, 1, `{"v":4}`),
		cell(1, 2, `{"f":"Scale(2)"}`),
		cell(2, 2, `{"f":"LET(x,A1,x*10)"}`),
		cell(3, 2, `{"f":"Fact2(5)"}`),
		cell(1, 3, `{"f":"MAP(A1:A2,LAMBDA(v,v+1))"}`),
		cell(1, 4, `{"f":"LAMBDA(x,x+A2)(1)"}`),
	}}}
	apply(t, d, tree, e)
	c.Follow(e, nil)
	want := map[[2]int]string{
		{1, 2}: `{"f":"Scale(2)","v":6}`,
		{2, 2}: `{"f":"_xlfn.LET(_xlpm.x,A1,_xlpm.x*10)","v":30}`,
		{3, 2}: `{"f":"Fact2(5)","v":120}`,
		{1, 3}: `{"cm":1,"f":"_xlfn.MAP(A1:A2,_xlfn.LAMBDA(_xlpm.v,_xlpm.v+1))","fa":"C1:C2","v":4}`,
		{2, 3}: `{"v":5}`,
		{1, 4}: `{"f":"_xlfn.LAMBDA(_xlpm.x,_xlpm.x+A2)(1)","v":5}`,
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
	files := unzip(t, saved)
	for _, s := range []string{`<definedName name="Scale">_xlfn.LAMBDA(_xlpm.x,_xlpm.x*`, `_xlfn.LAMBDA(_xlpm.n,IF(_xlpm.n&lt;=1,1,_xlpm.n*Fact2(_xlpm.n-1)))</definedName>`} {
		if !strings.Contains(files["xl/workbook.xml"], s) {
			t.Errorf("workbook lacks %s: %s", s, files["xl/workbook.xml"])
		}
	}

	d2, again, err := Open(saved, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var before, after []Name
	_ = json.Unmarshal([]byte(names), &before)
	_ = json.Unmarshal(again.Node("book").Attrs["names"], &after)
	if !slices.Equal(after, before) {
		t.Errorf("names read back as %v", after)
	}
	id = firstSheet(again).ID
	for p, w := range want {
		if got := string(again.Node(id).Grid.Cell(p[0], p[1])); got != w {
			t.Errorf("cell %v read back: %s, want %s", p, got, w)
		}
	}
	twice, err := d2.Save(again)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for name, x := range unzip(t, twice) {
		if strings.HasPrefix(name, "xl/worksheets/sheet") && strings.Contains(x, "LET") {
			found = true
			for _, s := range []string{`<f>_xlfn.LET(_xlpm.x,A1,_xlpm.x*10)</f>`, `<f t="array" ref="C1:C2">_xlfn.MAP(A1:A2,_xlfn.LAMBDA(_xlpm.v,_xlpm.v+1))</f>`,
				`<f>_xlfn.LAMBDA(_xlpm.x,_xlpm.x+A2)(1)</f>`, `<f>Fact2(5)</f>`} {
				if !strings.Contains(x, s) {
					t.Errorf("sheet saved twice lacks %s: %s", s, x)
				}
			}
		}
	}
	if !found {
		t.Error("no sheet saved twice holds the formulas")
	}

	reread := NewCalc(again, formula.Options{})
	more := ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{cell(1, 1, `{"v":5}`), cell(2, 1, `{"v":7}`)}}}
	apply(t, d2, again, more)
	reread.Follow(more, nil)
	for p, w := range map[[2]int]string{{1, 2}: `"v":10`, {2, 2}: `"v":50`, {1, 3}: `"v":6`, {2, 3}: `{"v":8}`, {1, 4}: `"v":8`} {
		if got := string(again.Node(id).Grid.Cell(p[0], p[1])); !strings.Contains(got, w) {
			t.Errorf("cell %v once A1 and A2 changed: %s, want %s", p, got, w)
		}
	}
}
