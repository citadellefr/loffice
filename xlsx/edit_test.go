package xlsx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/citadellefr/trame/ot"
)

func apply(t *testing.T, d *Document, tree *ot.Tree, e ot.Edit) {
	t.Helper()
	if err := d.Check(tree, e); err != nil {
		t.Fatalf("%v refused: %v", e, err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatalf("%v: %v", e, err)
	}
}

func cell(r, c int, fields string) ot.Cell {
	return ot.Cell{Row: r, Col: c, Fields: json.RawMessage(fields)}
}

// Every workbook of the corpus takes the same edits: cells set, a format
// made, cells merged, rows frozen, a filter set, a sheet renamed, one added
// before the others and the last one deleted. When LOFFICE_EDITED is set, the packages
// are written there for the validator.
func TestCorpusEdits(t *testing.T) {
	out := os.Getenv("LOFFICE_EDITED")
	edited := 0
	for _, f := range workbooks(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data)
		if err != nil {
			continue
		}
		var sheets []*ot.Node
		for _, n := range tree.Children("book") {
			if n.Type == "sheet" {
				sheets = append(sheets, n)
			}
		}
		if len(sheets) == 0 {
			continue
		}
		first := sheets[0].ID
		all := tree.Children("book")
		style := `{"base":"x0","fmt":"0.00 \"€\"","font":{"name":"Calibri","sz":11,"b":true,"color":{"rgb":"C00000"}},` +
			`"fill":{"pattern":"solid","fg":{"theme":4,"tint":0.4}},"border":{"b":{"style":"double","color":{"auto":true}}},` +
			`"align":{"h":"center","wrap":true}}`
		if tree.Node("x0") == nil {
			style = `{"font":{"b":true}}`
		}
		apply(t, d, tree, ot.Edit{
			{Op: ot.OpNew, ID: "fmt", Type: "xf", Key: "V", Attrs: ot.Values{"style": json.RawMessage(style)}},
			{Op: ot.OpCel, ID: first, Cells: []ot.Cell{
				cell(0, 3, `{"w":20,"cw":true}`),
				cell(1, 1, `{"v":"Bref","f":null,"e":null}`),
				cell(2, 0, `{"h":30,"ch":true}`),
				cell(2, 2, `{"v":3.5,"s":"fmt","f":null,"e":null}`),
				cell(3, 3, `{"f":"B2*2","v":7,"e":null}`),
				cell(4, 4, `{"m":[2,2],"v":"fusion","f":null,"e":null,"s":null,"rich":null}`),
			}},
			{Op: ot.OpSet, ID: first, Attrs: ot.Values{"name": json.RawMessage(`"Édité"`), "frozen": json.RawMessage(`{"r":1}`),
				"tab": json.RawMessage(`{"rgb":"00B050"}`), "zoom": json.RawMessage(`150`),
				filterKey: json.RawMessage(`{"ref":"A1:E6","cols":[{"col":1,"vals":["3,5","x"],"blank":true}]}`)}},
			{Op: ot.OpNew, ID: "added", Type: "sheet", Parent: "book", Key: ot.KeyBetween("", all[0].Key),
				Attrs: ot.Values{"name": json.RawMessage(`"Nouvelle"`)}, Cells: []ot.Cell{
					cell(1, 1, `{"v":"a\r\nb <&> _x0041_"}`),
					cell(1, 2, `{"v":true}`),
					cell(2, 1, `{"e":"#N/A"}`),
					cell(2, 2, `{"f":"SUM(Édité!B2:C3)","v":10.5}`),
				}},
		})
		if last := all[len(all)-1]; len(all) > 1 && last.ID != first {
			apply(t, d, tree, ot.Edit{{Op: ot.OpDel, ID: last.ID}})
		}
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		edited++
		_, again, err := Open(saved)
		if err != nil {
			t.Fatalf("%s: edited workbook does not open: %v", f, err)
		}
		book := again.Children("book")
		if str(book[0], "name") != "Nouvelle" || book[0].Grid.Len() != 4 {
			t.Fatalf("%s: added sheet read back as %v", f, book[0])
		}
		var text cellFields
		_ = json.Unmarshal(book[0].Grid.Cell(1, 1), &text)
		if string(text.V) != string(mustJSON("a\r\nb <&> _x0041_")) {
			t.Fatalf("%s: text read back as %s", f, text.V)
		}
		var ed *ot.Node
		for _, n := range book {
			if str(n, "name") == "Édité" {
				ed = n
			}
		}
		if ed == nil {
			t.Fatalf("%s: renamed sheet lost", f)
		}
		var c cellFields
		_ = json.Unmarshal(ed.Grid.Cell(2, 2), &c)
		var st Style
		if again.Node(c.S) == nil {
			t.Fatalf("%s: format %q of the cell lost", f, c.S)
		}
		_ = json.Unmarshal(again.Node(c.S).Attrs["style"], &st)
		if string(c.V) != "3.5" || st.Font == nil || !st.Font.Bold {
			t.Fatalf("%s: formatted cell read back as %s, %+v", f, ed.Grid.Cell(2, 2), st)
		}
		if got := string(ed.Grid.Cell(4, 4)); got != `{"m":[2,2],"v":"fusion"}` {
			t.Fatalf("%s: merged cell read back as %s", f, got)
		}
		if got := string(ed.Attrs[filterKey]); got != `{"ref":"A1:E6","cols":[{"col":1,"vals":["3,5","x"],"blank":true}]}` {
			t.Fatalf("%s: filter read back as %s", f, got)
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
	t.Logf("%d workbooks edited", edited)
}
