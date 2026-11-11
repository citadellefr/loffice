package xlsx

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
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

func spillTree(t *testing.T) (*ot.Tree, *Calc) {
	t.Helper()
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`)}, Cells: []ot.Cell{cell(9, 9, `{"v":0}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree, NewCalc(tree, formula.Options{})
}

func cells(t *testing.T, tree *ot.Tree, col int, rows ...int) string {
	t.Helper()
	var out []string
	for _, r := range rows {
		out = append(out, fieldsAt(tree, "S1", r, col))
	}
	return strings.Join(out, "|")
}

// A dynamic array spills over the cells below, is blocked by a value
// there, takes the ground back when it is cleared, and shrinks with the
// array.
func TestCalcSpill(t *testing.T) {
	tree, c := spillTree(t)
	got := edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(1, 1, `{"f":"SEQUENCE(3)"}`), cell(1, 2, `{"f":"SUM(A1#)"}`)}}})
	if want := `[{"o":"cel","id":"S1","c":[[1,1,{"cm":1,"f":"_xlfn.SEQUENCE(3)","fa":"A1:A3","v":1}],[1,2,{"f":"SUM(_xlfn.ANCHORARRAY(A1))","v":6}],[2,1,{"v":2}],[3,1,{"v":3}]]}]`; got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
	if got, want := cells(t, tree, 1, 1, 2, 3), `{"cm":1,"f":"_xlfn.SEQUENCE(3)","fa":"A1:A3","v":1}|{"v":2}|{"v":3}`; got != want {
		t.Fatalf("spilled: %s", got)
	}
	if got := fieldsAt(tree, "S1", 1, 2); got != `{"f":"SUM(_xlfn.ANCHORARRAY(A1))","v":6}` {
		t.Fatalf("B1 = %s", got)
	}

	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(3, 1, `{"v":"x"}`)}}})
	if got, want := cells(t, tree, 1, 1, 2, 3), `{"cm":1,"e":"#SPILL!","f":"_xlfn.SEQUENCE(3)","fa":"A1"}||{"v":"x"}`; got != want {
		t.Fatalf("blocked: %s", got)
	}

	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(3, 1, `{"v":null}`)}}})
	if got, want := cells(t, tree, 1, 1, 2, 3), `{"cm":1,"f":"_xlfn.SEQUENCE(3)","fa":"A1:A3","v":1}|{"v":2}|{"v":3}`; got != want {
		t.Fatalf("cleared: %s", got)
	}
	if got := fieldsAt(tree, "S1", 1, 2); got != `{"f":"SUM(_xlfn.ANCHORARRAY(A1))","v":6}` {
		t.Fatalf("B1 after the spill came back = %s", got)
	}

	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"f":"SEQUENCE(2)"}`)}}})
	if got, want := cells(t, tree, 1, 1, 2, 3), `{"cm":1,"f":"_xlfn.SEQUENCE(2)","fa":"A1:A2","v":1}|{"v":2}|`; got != want {
		t.Fatalf("shrunk: %s", got)
	}
	if got := fieldsAt(tree, "S1", 1, 2); got != `{"f":"SUM(_xlfn.ANCHORARRAY(A1))","v":3}` {
		t.Fatalf("B1 after the spill shrank = %s", got)
	}

	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(5, 5, `{"f":"1+1"}`)}}})
	if got := fieldsAt(tree, "S1", 5, 5); got != `{"f":"1+1","v":2}` {
		t.Fatalf("scalar: %s", got)
	}

	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(3, 1, `{"f":"1+1"}`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"f":"SEQUENCE(3)"}`)}}})
	if got, want := cells(t, tree, 1, 1, 2, 3), `{"cm":1,"e":"#SPILL!","f":"_xlfn.SEQUENCE(3)","fa":"A1"}||{"f":"1+1","v":2}`; got != want {
		t.Fatalf("blocked by a scalar: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(3, 1, `{"f":null,"v":null}`)}}})
	if got, want := cells(t, tree, 1, 1, 2, 3), `{"cm":1,"f":"_xlfn.SEQUENCE(3)","fa":"A1:A3","v":1}|{"v":2}|{"v":3}`; got != want {
		t.Fatalf("freed by the scalar: %s", got)
	}
}

// 10 000 formulas read the cell changed: the server calculates them again
// and sets their values in the tree.
func BenchmarkFollow(b *testing.B) {
	benchmarkFollow(b, false)
}

// The same formulas typed by the client, dynamic as in Excel 365.
func BenchmarkFollowTyped(b *testing.B) {
	benchmarkFollow(b, true)
}

func benchmarkFollow(b *testing.B, typed bool) {
	cells := []ot.Cell{{Row: 1, Col: 1, Fields: json.RawMessage(`{"v":1}`)}}
	for i := 2; i <= 10000; i++ {
		cells = append(cells, ot.Cell{Row: i, Col: 1, Fields: json.RawMessage(`{"v":` + strconv.Itoa(i) + `}`)})
	}
	var formulas []ot.Cell
	for i := 1; i <= 10000; i++ {
		f := fmt.Sprintf(`{"f":"$A$1*%d+SUM(A1:A%d)","v":0}`, i, min(i, 50))
		formulas = append(formulas, ot.Cell{Row: i, Col: 2, Fields: json.RawMessage(f)})
	}
	if !typed {
		cells = append(cells, formulas...)
	}
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`)}, Cells: cells},
	})
	if err != nil {
		b.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	if typed {
		e := ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: formulas}}
		if err := tree.Apply(e); err != nil {
			b.Fatal(err)
		}
		c.Follow(e, nil)
	}
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

func TestRefreshCalculatesEverythingAgainFromAnEngineBroken(t *testing.T) {
	tree, c := calcTree(t)
	if err := tree.Apply(ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":10}`)}}}); err != nil {
		t.Fatal(err)
	}
	c.engine = nil
	out := c.Refresh()
	if got := fieldsAt(tree, "S1", 3, 1); got != `{"f":"SUM(A1:A2)","v":12}` {
		t.Fatalf("A3: %s", got)
	}
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"Feuil1!A3*10","v":120}` {
		t.Fatalf("Data!A1: %s", got)
	}
	raw, _ := json.Marshal(out)
	if want := `{"o":"cel","id":"S1","c":[[2,2,{"f":"A3*Taux","v":2.4000000000000004}],[3,1,{"f":"SUM(A1:A2)","v":12}]]}`; !strings.Contains(string(raw), want) {
		t.Fatalf("the formula cells are not all sent: %s", raw)
	}
}

// The formulas of a circle hold 0 and the book tells their cells; with
// iteration set they settle, and once the circle ends they read their
// values again.
func TestCalcCircles(t *testing.T) {
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`)}, Cells: []ot.Cell{
			cell(1, 1, `{"v":1000}`),
			cell(1, 2, `{"f":"A1+C1","v":1052.63157894737}`),
			cell(1, 3, `{"f":"B1*5%","v":52.6315789473684}`),
			cell(1, 4, `{"f":"C1*2","v":105.263157894737}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	if got := string(tree.Node("book").Attrs["circular"]); got != `{"S1":["B1","C1"]}` {
		t.Fatalf("circular %s", got)
	}
	got := edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":2000}`)}}})
	if want := `[{"o":"cel","id":"S1","c":[[1,2,{"v":0}],[1,3,{"v":0}],[1,4,{"v":0}]]}]`; got != want {
		t.Fatalf("got %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"iterate": json.RawMessage(`{"count":100,"delta":0.000001}`)}}})
	if got := fieldsAt(tree, "S1", 1, 3); got[:21] != `{"f":"B1*5%","v":105.` {
		t.Fatalf("C1 = %s", got)
	}
	got = edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 3, `{"f":null,"v":10}`)}}})
	want := `[{"o":"cel","id":"S1","c":[[1,2,{"v":2010}],[1,4,{"v":20}]]},{"o":"set","id":"book","a":{"circular":null}}]`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

// Renaming a sheet keeps the circles found, and those an edit makes or ends
// after it are found as well.
func TestCalcCirclesAfterRename(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(4, 1, `{"f":"A5+1"}`), cell(5, 1, `{"f":"A4+1"}`)}}})
	if got := string(tree.Node("book").Attrs["circular"]); got != `{"S1":["A4","A5"]}` {
		t.Fatalf("circular %s", got)
	}
	got := edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{"name": json.RawMessage(`"Autre"`)}}})
	if strings.Contains(got, "circular") || string(tree.Node("book").Attrs["circular"]) != `{"S1":["A4","A5"]}` {
		t.Fatalf("renamed: %s %s", got, tree.Node("book").Attrs["circular"])
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S2", Cells: []ot.Cell{cell(2, 1, `{"f":"B2"}`), cell(2, 2, `{"f":"Feuil1!A3+Autre!A2"}`)}}})
	if got := string(tree.Node("book").Attrs["circular"]); got != `{"S1":["A4","A5"],"S2":["A2","B2"]}` {
		t.Fatalf("made after a rename: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(5, 1, `{"f":null,"v":1}`)}}})
	if got := string(tree.Node("book").Attrs["circular"]); got != `{"S2":["A2","B2"]}` {
		t.Fatalf("ended after a rename: %s", got)
	}
}

// A renaming that makes valid a reference to a sheet that was not there
// closes the circle the reference makes at once.
func TestCalcCircleRenamedIntoReference(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{
		{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(4, 1, `{"f":"Feuil9!A1"}`)}},
		{Op: ot.OpCel, ID: "S2", Cells: []ot.Cell{cell(1, 1, `{"f":"Feuil1!A4+1"}`)}},
	})
	if got := tree.Node("book").Attrs["circular"]; got != nil {
		t.Fatalf("circular %s before the renaming", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{"name": json.RawMessage(`"Feuil9"`)}}})
	if got := string(tree.Node("book").Attrs["circular"]); got != `{"S1":["A4"],"S2":["A1"]}` {
		t.Fatalf("circular %s after the renaming", got)
	}
}

// A renaming that makes no reference valid does not look for circles again:
// one the engine was not told of stays unseen until an edit that does.
func TestCalcRenameDoesNotScan(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(6, 1, `{"v":1}`)}}})
	if err := tree.Apply(ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(4, 1, `{"f":"A5+1"}`), cell(5, 1, `{"f":"A4+1"}`)}}}); err != nil {
		t.Fatal(err)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{"name": json.RawMessage(`"Autre"`)}}})
	if got := tree.Node("book").Attrs["circular"]; got != nil {
		t.Fatalf("circles looked for after a renaming: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpNew, ID: "S3", Type: "sheet", Parent: "book", Key: "M", Attrs: ot.Values{"name": json.RawMessage(`"Trois"`)}}})
	if got := string(tree.Node("book").Attrs["circular"]); got != `{"S1":["A4","A5"]}` {
		t.Fatalf("circular %s after a new sheet", got)
	}
}

// Iteration is read from calcPr and written back there.
func TestCalcPr(t *testing.T) {
	blank, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(blank, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, iterate := range []string{`{"count":50,"delta":0.0001}`, `null`} {
		e := ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"iterate": json.RawMessage(iterate)}}}
		apply(t, d, tree, e)
		data, err := d.Save(tree)
		if err != nil {
			t.Fatal(err)
		}
		if d, tree, err = Open(data, Limits{}); err != nil {
			t.Fatal(err)
		}
		if got := string(tree.Node("book").Attrs["iterate"]); got != strings.TrimSuffix(iterate, "null") {
			t.Fatalf("iterate %s, want %s", got, iterate)
		}
	}
	bad := ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"iterate": json.RawMessage(`{"count":0,"delta":0.001}`)}}}
	if err := d.Check(tree, bad, ""); err == nil {
		t.Fatal("no iteration taken")
	}
}

func largeTree(b *testing.B, n int) *ot.Tree {
	return largeTreeWith(b, n, "")
}

// Every fourth formula of column B is extra, when it is not empty.
func largeTreeWith(b *testing.B, n int, extra string) *ot.Tree {
	cells := make([]ot.Cell, 0, 2*n)
	for i := 1; i <= n; i++ {
		f := fmt.Sprintf(`A%d*2+SUM(A%d:A%d)`, i, max(i-4, 1), i)
		if extra != "" && i%4 == 0 {
			f = extra
		}
		cells = append(cells,
			ot.Cell{Row: i, Col: 1, Fields: json.RawMessage(`{"v":` + strconv.Itoa(i) + `}`)},
			ot.Cell{Row: i, Col: 2, Fields: json.RawMessage(`{"f":"` + strings.ReplaceAll(f, `"`, `\"`) + `","v":0}`)})
	}
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`)}, Cells: cells},
	})
	if err != nil {
		b.Fatal(err)
	}
	return tree
}

// 100 000 formulas without a circle: opening the book looks for circles
// once.
func BenchmarkOpenFormulas(b *testing.B) {
	tree := largeTree(b, 100000)
	for b.Loop() {
		NewCalc(tree, formula.Options{})
	}
}

// Renaming a sheet of the same book does not look for circles again.
func BenchmarkRenameSheet(b *testing.B) {
	tree := largeTree(b, 100000)
	c := NewCalc(tree, formula.Options{})
	i := 0
	for b.Loop() {
		i++
		e := ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{"name": json.RawMessage(`"Feuil` + strconv.Itoa(i+1) + `"`)}}}
		if err := tree.Apply(e); err != nil {
			b.Fatal(err)
		}
		c.Follow(e, nil)
	}
}

// One cell edited in 100 000 formulas, a quarter of them CELL, SHEETS or
// INFO, against the same book without them (a quarter of ROW).
func BenchmarkEditPlain(b *testing.B)  { benchmarkEdit(b, `ROW(A1)`) }
func BenchmarkEditCell(b *testing.B)   { benchmarkEdit(b, `CELL("filename",A1)`) }
func BenchmarkEditSheets(b *testing.B) { benchmarkEdit(b, `SHEETS()`) }
func BenchmarkEditInfo(b *testing.B)   { benchmarkEdit(b, `INFO("numfile")`) }

func benchmarkEdit(b *testing.B, extra string) {
	tree := largeTreeWith(b, 100000, extra)
	c := NewCalc(tree, formula.Options{})
	i := 0
	for b.Loop() {
		i++
		e := ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":`+strconv.Itoa(i)+`}`)}}}
		if err := tree.Apply(e); err != nil {
			b.Fatal(err)
		}
		c.Follow(e, nil)
	}
}

func TestCalcWorkbookInfo(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpNew, ID: "S3", Type: "sheet", Parent: "book", Key: "M", Attrs: ot.Values{"name": json.RawMessage(`"Trois"`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(6, 1, `{"f":"SHEETS()"}`), cell(6, 2, `{"f":"INFO(\"numfile\")"}`), cell(6, 3, `{"f":"CELL(\"filename\",A1)"}`)}}})
	for col, want := range map[int]string{1: `"v":3`, 2: `"v":3`, 3: `"v":""`} {
		if got := fieldsAt(tree, "S1", 6, col); !strings.HasSuffix(got, want+"}") {
			t.Fatalf("col %d: %s, want %s", col, got, want)
		}
	}
}

func TestCalcStructuralFollowsSheets(t *testing.T) {
	tree, c := calcTree(t)
	c.opt.FileName = "budget.xlsx"
	c.rebuild()
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(6, 1, `{"f":"SHEETS()"}`), cell(6, 2, `{"f":"INFO(\"numfile\")"}`), cell(6, 3, `{"f":"CELL(\"filename\",A1)"}`)}}})
	check := func(sheets, name string) {
		t.Helper()
		got := fieldsAt(tree, "S1", 6, 1) + "|" + fieldsAt(tree, "S1", 6, 2) + "|" + fieldsAt(tree, "S1", 6, 3)
		want := `{"f":"_xlfn.SHEETS()","v":` + sheets + `}|{"f":"INFO(\"numfile\")","v":` + sheets + `}|{"f":"CELL(\"filename\",A1)","v":"[budget.xlsx]` + name + `"}`
		if got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	}
	check("2", "Feuil1")
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{"name": json.RawMessage(`"Autre"`)}}})
	check("2", "Autre")
	edit(t, tree, c, ot.Edit{{Op: ot.OpNew, ID: "S3", Type: "sheet", Parent: "book", Key: "M", Attrs: ot.Values{"name": json.RawMessage(`"Trois"`)}}})
	check("3", "Autre")
	edit(t, tree, c, ot.Edit{{Op: ot.OpDel, ID: "S3"}})
	check("2", "Autre")
}

func TestCalcFileName(t *testing.T) {
	tree, c := calcTree(t)
	c.opt.FileName = "budget.xlsx"
	c.rebuild()
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(6, 3, `{"f":"CELL(\"filename\",A1)"}`)}}})
	if got := fieldsAt(tree, "S1", 6, 3); got != `{"f":"CELL(\"filename\",A1)","v":"[budget.xlsx]Feuil1"}` {
		t.Fatalf("got %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{"name": json.RawMessage(`"Autre"`)}}})
	if got := fieldsAt(tree, "S1", 6, 3); got != `{"f":"CELL(\"filename\",A1)","v":"[budget.xlsx]Autre"}` {
		t.Fatalf("after rename: %s", got)
	}
}

func TestCalcCellFollowsInsertion(t *testing.T) {
	tree, c := calcTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(8, 4, `{"f":"CELL(\"row\",A5)"}`), cell(8, 5, `{"f":"CELL(\"col\",C5)"}`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 2}})
	if got := fieldsAt(tree, "S1", 10, 4); got != `{"f":"CELL(\"row\",A7)","v":7}` {
		t.Errorf("after rows: %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimCols, At: 2, N: 3}})
	if got := fieldsAt(tree, "S1", 10, 8); got != `{"f":"CELL(\"col\",F7)","v":6}` {
		t.Errorf("after columns: %s", got)
	}
}
