package xlsx

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

const prices = `[{"id":1,"name":"Ventes","ref":"A1:C4","filter":true,"style":"TableStyleMedium2","rows":true,"columns":[` +
	`{"id":1,"name":"Article"},{"id":2,"name":"Prix"},{"id":3,"name":"Double","f":"Ventes[[#This Row],[Prix]]*2"}]}]`

// tableTree is a sheet with a table of three rows, a calculated column, and
// formulas reading it on another sheet.
func tableTree(t *testing.T) (*ot.Tree, *Calc) {
	t.Helper()
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{"name": json.RawMessage(`"Feuil1"`),
			tablesKey: json.RawMessage(prices)}, Cells: []ot.Cell{
			cell(1, 1, `{"v":"Article"}`), cell(1, 2, `{"v":"Prix"}`), cell(1, 3, `{"v":"Double"}`),
			cell(2, 1, `{"v":"a"}`), cell(2, 2, `{"v":1}`), cell(2, 3, `{"f":"Ventes[[#This Row],[Prix]]*2","v":2}`),
			cell(3, 1, `{"v":"b"}`), cell(3, 2, `{"v":2}`), cell(3, 3, `{"f":"Ventes[[#This Row],[Prix]]*2","v":4}`),
			cell(4, 1, `{"v":"c"}`), cell(4, 2, `{"v":3}`), cell(4, 3, `{"f":"Ventes[[#This Row],[Prix]]*2","v":6}`),
		}},
		{Op: ot.OpNew, ID: "S2", Type: "sheet", Parent: "book", Key: "L", Attrs: ot.Values{"name": json.RawMessage(`"Bilan"`)}, Cells: []ot.Cell{
			cell(1, 1, `{"f":"SUM(Ventes[Prix])","v":6}`),
			cell(2, 1, `{"f":"SUM(Ventes[Double])","v":12}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree, NewCalc(tree, formula.Options{Now: func() time.Time { return time.Date(2024, 12, 25, 12, 0, 0, 0, time.UTC) }})
}

func followed(t *testing.T, tree *ot.Tree, id string) []Table {
	t.Helper()
	var tables []Table
	if err := json.Unmarshal(tree.Node(id).Attrs[tablesKey], &tables); err != nil && tree.Node(id).Attrs[tablesKey] != nil {
		t.Fatal(err)
	}
	return tables
}

func retable(tables []Table) ot.Edit {
	return ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{tablesKey: mustJSON(tables)}}}
}

func TestTableRowsInserted(t *testing.T) {
	tree, c := tableTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 3, N: 2}})
	tables := followed(t, tree, "S1")
	if tables[0].Ref != "A1:C6" {
		t.Fatalf("ref %s", tables[0].Ref)
	}
	for _, r := range []int{3, 4} {
		if got := fieldsAt(tree, "S1", r, 3); !strings.Contains(got, `"f":"Ventes[[#This Row],[Prix]]*2"`) || !strings.Contains(got, `"v":0`) {
			t.Errorf("row %d: %s", r, got)
		}
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(3, 2, `{"v":10}`)}}})
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"SUM(Ventes[Prix])","v":16}` {
		t.Errorf("sum %s", got)
	}
	if got := fieldsAt(tree, "S2", 2, 1); got != `{"f":"SUM(Ventes[Double])","v":32}` {
		t.Errorf("double %s", got)
	}
}

func TestTableRowsRemoved(t *testing.T) {
	tree, c := tableTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 2, N: 1}})
	if got := followed(t, tree, "S1")[0].Ref; got != "A1:C3" {
		t.Fatalf("ref %s", got)
	}
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"SUM(Ventes[Prix])","v":5}` {
		t.Errorf("sum %s", got)
	}
	d := &Document{}
	for _, e := range []ot.Edit{
		{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 1, N: 1}},
		{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 2, N: 2}},
	} {
		if err := d.Check(tree, e, ""); err != ErrTable {
			t.Errorf("%v: %v", e, err)
		}
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 1, N: 3}})
	if tree.Node("S1").Attrs[tablesKey] != nil {
		t.Errorf("tables %s", tree.Node("S1").Attrs[tablesKey])
	}
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"e":"#REF!","f":"SUM(#REF!)"}` {
		t.Errorf("sum %s", got)
	}
}

func TestTableColumnsMoved(t *testing.T) {
	tree, c := tableTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimCols, At: 2, N: 1}})
	tables := followed(t, tree, "S1")
	if tables[0].Ref != "A1:D4" || len(tables[0].Columns) != 4 || tables[0].Columns[1].Name != "Colonne1" || tables[0].Columns[1].ID != 4 {
		t.Fatalf("tables %+v", tables)
	}
	if got := fieldsAt(tree, "S1", 1, 2); got != `{"v":"Colonne1"}` {
		t.Errorf("header %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimCols, At: 3, N: 1}})
	tables = followed(t, tree, "S1")
	if tables[0].Ref != "A1:C4" || tables[0].Columns[1].Name != "Colonne1" {
		t.Fatalf("tables %+v", tables)
	}
	if got := fieldsAt(tree, "S2", 1, 1); !strings.Contains(got, `"e":"#REF!"`) {
		t.Errorf("sum of a removed column %s", got)
	}
}

func TestTableColumnRenamed(t *testing.T) {
	tree, c := tableTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 2, `{"v":"Tarif"}`)}}})
	tables := followed(t, tree, "S1")
	if tables[0].Columns[1].Name != "Tarif" || tables[0].Columns[2].F != "Ventes[[#This Row],[Tarif]]*2" {
		t.Fatalf("columns %+v", tables[0].Columns)
	}
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"SUM(Ventes[Tarif])","v":6}` {
		t.Errorf("sum %s", got)
	}
	if got := fieldsAt(tree, "S1", 2, 3); got != `{"f":"Ventes[[#This Row],[Tarif]]*2","v":2}` {
		t.Errorf("calculated %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":"tarif"}`), cell(1, 3, `{"v":null}`)}}})
	tables = followed(t, tree, "S1")
	if tables[0].Columns[0].Name != "tarif2" || tables[0].Columns[2].Name != "Colonne1" {
		t.Fatalf("columns %+v", tables[0].Columns)
	}
	if got := fieldsAt(tree, "S1", 1, 3); got != `{"v":"Colonne1"}` {
		t.Errorf("header %s", got)
	}
	tables[0].Name = "Achats"
	edit(t, tree, c, retable(tables))
	if got := fieldsAt(tree, "S2", 2, 1); got != `{"f":"SUM(Achats[Colonne1])","v":12}` {
		t.Errorf("renamed table %s", got)
	}
}

func TestTableTotals(t *testing.T) {
	tree, c := tableTree(t)
	tables := followed(t, tree, "S1")
	tables[0].Totals = true
	edit(t, tree, c, retable(tables))
	tables = followed(t, tree, "S1")
	if tables[0].Ref != "A1:C5" || tables[0].Columns[0].Label != "Total" || tables[0].Columns[2].Total != "sum" {
		t.Fatalf("tables %+v", tables)
	}
	if got := fieldsAt(tree, "S1", 5, 1); got != `{"v":"Total"}` {
		t.Errorf("label %s", got)
	}
	if got := fieldsAt(tree, "S1", 5, 3); got != `{"f":"SUBTOTAL(109,Ventes[Double])","v":12}` {
		t.Errorf("total %s", got)
	}
	if got := fieldsAt(tree, "S2", 2, 1); got != `{"f":"SUM(Ventes[Double])","v":12}` {
		t.Errorf("sum %s", got)
	}
	tables[0].Totals = false
	edit(t, tree, c, retable(tables))
	if got := followed(t, tree, "S1")[0].Ref; got != "A1:C4" {
		t.Fatalf("ref %s", got)
	}
	if got := fieldsAt(tree, "S1", 5, 3); got != "" && got != "{}" {
		t.Errorf("total left %s", got)
	}
}

func TestTableCreated(t *testing.T) {
	tree, c := tableTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S2", Cells: []ot.Cell{
		cell(5, 1, `{"v":"Nom"}`), cell(5, 2, `{"v":2024}`), cell(6, 1, `{"v":"x"}`), cell(6, 2, `{"v":7}`)}}})
	created := `[{"name":"Tableau1","ref":"A5:C6","filter":true,"style":"TableStyleMedium2","rows":true,` +
		`"columns":[{"name":"Nom"},{"name":"2024"},{"name":"Colonne1"}]}]`
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{tablesKey: json.RawMessage(created)}}})
	tables := followed(t, tree, "S2")
	if tables[0].ID != 2 || tables[0].Columns[2].ID != 3 {
		t.Fatalf("tables %+v", tables)
	}
	if got := fieldsAt(tree, "S2", 5, 2); got != `{"v":"2024"}` {
		t.Errorf("header %s", got)
	}
	if got := fieldsAt(tree, "S2", 5, 3); got != `{"v":"Colonne1"}` {
		t.Errorf("header %s", got)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S2", Cells: []ot.Cell{cell(6, 3, `{"f":"[@2024]*10"}`)}}})
	if got := fieldsAt(tree, "S2", 6, 3); got != `{"f":"Tableau1[[#This Row],[2024]]*10","v":70}` {
		t.Errorf("qualified %s", got)
	}
	if got := followed(t, tree, "S2")[0].Columns[2].F; got != "Tableau1[[#This Row],[2024]]*10" {
		t.Errorf("calculated column %q", got)
	}
}

func TestTableExtended(t *testing.T) {
	tree, c := tableTree(t)
	tables := followed(t, tree, "S1")
	tables[0].Ref = "A1:C5"
	e := append(ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(5, 2, `{"v":5}`)}}}, retable(tables)...)
	edit(t, tree, c, e)
	if got := fieldsAt(tree, "S1", 5, 3); got != `{"f":"Ventes[[#This Row],[Prix]]*2","v":10}` {
		t.Errorf("filled %s", got)
	}
	if got := fieldsAt(tree, "S2", 2, 1); got != `{"f":"SUM(Ventes[Double])","v":22}` {
		t.Errorf("sum %s", got)
	}
}

func TestTableConverted(t *testing.T) {
	tree, c := tableTree(t)
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{tablesKey: json.RawMessage("null")}}})
	if got := fieldsAt(tree, "S2", 1, 1); got != `{"f":"SUM(Feuil1!$B$2:$B$4)","v":6}` {
		t.Errorf("sum %s", got)
	}
	if got := fieldsAt(tree, "S1", 3, 3); got != `{"f":"$B$3*2","v":4}` {
		t.Errorf("this row %s", got)
	}
}
