package xlsx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/ot"
)

func calcTree(t *testing.T) (*ot.Tree, *Calc) {
	t.Helper()
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V", Attrs: ot.Values{"names": json.RawMessage(`[{"name":"Taux","ref":"Feuil1!$B$1"}]`)}},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`)}, Cells: []ot.Cell{
			cell(1, 1, `{"v":1}`),
			cell(1, 2, `{"v":0.2}`),
			cell(2, 1, `{"v":2}`),
			cell(2, 2, `{"f":"A3*Taux","v":0.6}`),
			cell(3, 1, `{"f":"SUM(A1:A2)","v":3}`),
			cell(5, 1, `{"m":[2,2],"v":"fusion"}`),
		}},
		{Op: ot.OpNew, ID: "S2", Type: "sheet", Parent: "book", Key: "L", Attrs: ot.Values{"name": json.RawMessage(`"Data"`)}, Cells: []ot.Cell{
			cell(1, 1, `{"f":"Feuil1!A3*10","v":30}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{Now: func() time.Time { return time.Date(2024, 12, 25, 12, 0, 0, 0, time.UTC) }})
	return tree, c
}

// edit applies an edit as the hub does, then its follow-up.
func edit(t *testing.T, tree *ot.Tree, c *Calc, e ot.Edit, since ...ot.Edit) string {
	t.Helper()
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(c.Follow(e, since))
	return string(out)
}

func fieldsAt(tree *ot.Tree, id string, row, col int) string {
	return string(tree.Node(id).Grid.Cell(row, col))
}

func TestCalcValues(t *testing.T) {
	tree, c := calcTree(t)
	if c.Formulas() != 3 {
		t.Fatalf("%d formulas", c.Formulas())
	}
	got := edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":5}`)}}})
	want := `[{"o":"cel","id":"S1","c":[[2,2,{"v":1.4000000000000001}],[3,1,{"v":7}]]},` +
		`{"o":"cel","id":"S2","c":[[1,1,{"v":70}]]}]`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"Feuil1!A3*10","v":70}` {
		t.Fatalf("Data!A1 = %s", got)
	}
	// a formula typed in, and an error
	got = edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(4, 3, `{"f":"1/0"}`)}}})
	if want := `[{"o":"cel","id":"S1","c":[[4,3,{"e":"#DIV/0!"}]]}]`; got != want {
		t.Fatalf("got %s", got)
	}
}

func TestCalcRows(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 1}})
	for _, x := range []struct {
		id       string
		row, col int
		want     string
	}{
		{"S1", 4, 1, `{"f":"SUM(A1:A3)","v":3}`},
		{"S1", 3, 2, `{"f":"A4*Taux","v":0.6}`},
		{"S2", 1, 1, `{"f":"Feuil1!A4*10","v":30}`},
		{"S1", 6, 1, `{"m":[2,2],"v":"fusion"}`},
	} {
		if got := fieldsAt(tree, x.id, x.row, x.col); got != x.want {
			t.Errorf("%s %d,%d = %s, want %s", x.id, x.row, x.col, got, x.want)
		}
	}
	// a row inserted inside merged cells widens them
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 7, N: 2}})
	if got := fieldsAt(tree, "S1", 6, 1); got != `{"m":[4,2],"v":"fusion"}` {
		t.Errorf("merged cells: %s", got)
	}
	// removing the rows a formula reads leaves it a #REF!
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 1, N: 3}})
	if got := fieldsAt(tree, "S1", 1, 1); got != `{"e":"#REF!","f":"SUM(#REF!)"}` {
		t.Errorf("formula reading removed rows: %s", got)
	}
}

// A formula typed before rows were inserted, and sent after, moves with
// them.
func TestCalcRebased(t *testing.T) {
	tree, c := calcTree(t)
	ins := ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 1}}
	edit(t, tree, c, ins)
	late := ot.TransformEdit(ins, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 3, `{"f":"A3+1"}`)}}}, true)
	edit(t, tree, c, late, ins)
	if got := fieldsAt(tree, "S1", 1, 3); got != `{"f":"A4+1","v":4}` {
		t.Fatalf("got %s", got)
	}
}

// Cells typed below those a range read last still count.
func TestCalcBounds(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(4, 4, `{"f":"SUM(A:A)"}`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(9, 1, `{"v":0}`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(12, 1, `{"v":100}`)}}})
	if got := fieldsAt(tree, "S1", 4, 4); got != `{"f":"SUM(A:A)","v":106}` {
		t.Fatalf("got %s", got)
	}
}

// Subtotals leave out the rows hidden by a filter, and 109 those hidden
// by hand too.
func TestCalcSubtotal(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(1, 3, `{"f":"SUBTOTAL(9,A1:A2)"}`), cell(2, 3, `{"f":"SUBTOTAL(109,A1:A2)"}`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(2, 0, `{"hide":true}`)}}})
	if got := fieldsAt(tree, "S1", 1, 3) + fieldsAt(tree, "S1", 2, 3); got != `{"f":"SUBTOTAL(9,A1:A2)","v":3}{"f":"SUBTOTAL(109,A1:A2)","v":1}` {
		t.Fatalf("hidden by hand: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{filterKey: json.RawMessage(`{"ref":"A1:A3","cols":[{"col":0,"vals":["1"]}]}`)}}})
	if got := fieldsAt(tree, "S1", 1, 3); got != `{"f":"SUBTOTAL(9,A1:A2)","v":1}` {
		t.Fatalf("filtered: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(2, 0, `{"hide":null}`)}}})
	if got := fieldsAt(tree, "S1", 1, 3) + fieldsAt(tree, "S1", 2, 3); got != `{"f":"SUBTOTAL(9,A1:A2)","v":3}{"f":"SUBTOTAL(109,A1:A2)","v":3}` {
		t.Fatalf("shown: %s", got)
	}
}

func TestCalcSheets(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{"name": json.RawMessage(`"Ventes 2024"`)}}})
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"'Ventes 2024'!A3*10","v":30}` {
		t.Fatalf("renamed: %s", got)
	}
	if got := string(tree.Node("book").Attrs["names"]); got != `[{"name":"Taux","ref":"'Ventes 2024'!$B$1"}]` {
		t.Fatalf("names: %s", got)
	}
	// a name Excel refuses is made one it takes
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{"name": json.RawMessage(`"ventes 2024"`)}}})
	if got := str(tree.Node("S2"), "name"); got != "ventes 2024 (2)" {
		t.Fatalf("duplicate name: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpDel, ID: "S1"}})
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"e":"#REF!","f":"#REF!*10"}` {
		t.Fatalf("deleted: %s", got)
	}
}

// 10 000 formulas read the cell changed: the server calculates them again
// and sets their values in the tree.
func BenchmarkFollow(b *testing.B) {
	cells := []ot.Cell{{Row: 1, Col: 1, Fields: json.RawMessage(`{"v":1}`)}}
	for i := 2; i <= 10000; i++ {
		cells = append(cells, ot.Cell{Row: i, Col: 1, Fields: json.RawMessage(`{"v":` + strconv.Itoa(i) + `}`)})
	}
	for i := 1; i <= 10000; i++ {
		f := fmt.Sprintf(`{"f":"$A$1*%d+SUM(A1:A%d)","v":0}`, i, min(i, 50))
		cells = append(cells, ot.Cell{Row: i, Col: 2, Fields: json.RawMessage(f)})
	}
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`)}, Cells: cells},
	})
	if err != nil {
		b.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	i := 0
	for b.Loop() {
		i++
		e := ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{{Row: 1, Col: 1, Fields: json.RawMessage(`{"v":` + strconv.Itoa(i+1) + `}`)}}}}
		if err := tree.Apply(e); err != nil {
			b.Fatal(err)
		}
		if out := c.Follow(e, nil); len(out) != 1 || len(out[0].Cells) != 10000 {
			b.Fatal("not all calculated")
		}
	}
}
