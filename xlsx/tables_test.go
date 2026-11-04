package xlsx

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func tablesOf(tree *ot.Tree, sheet string) []Table {
	var tables []Table
	_ = json.Unmarshal(tree.Node(sheet).Attrs[tablesKey], &tables)
	return tables
}

func setTables(sheet string, tables any) ot.Edit {
	raw, _ := json.Marshal(tables)
	return ot.Edit{{Op: ot.OpSet, ID: sheet, Attrs: ot.Values{tablesKey: raw}}}
}

func blankBook(t *testing.T) (*Document, *ot.Tree) {
	t.Helper()
	data, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree
}

// saveAndOpen saves the workbook and reads it again.
func saveAndOpen(t *testing.T, d *Document, tree *ot.Tree) (*Document, *ot.Tree, *opc.Package) {
	t.Helper()
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	again, treeAgain, err := Open(saved, Limits{})
	if err != nil {
		t.Fatalf("saved workbook does not open: %v", err)
	}
	pkg, err := opc.Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return again, treeAgain, pkg
}

func part(t *testing.T, pkg *opc.Package, name string) string {
	t.Helper()
	data, err := pkg.Read(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return string(data)
}

func TestTablesRead(t *testing.T) {
	_, tree := openCorpus(t, "libreoffice/tdf162963_TableWithTotalsEnabled.xlsx")
	want := `[{"id":1,"name":"myData","ref":"A1:B6","totals":true,"filter":true,"style":"TableStyleLight9","rows":true,` +
		`"columns":[{"id":1,"name":"Name","label":"All"},{"id":2,"name":"Sales","total":"custom","tf":"SUM(myData[Sales])"}]}]`
	if got := string(firstSheet(tree).Attrs[tablesKey]); got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestTablesChecked(t *testing.T) {
	d, tree := blankBook(t)
	table := func(name, ref string, columns ...string) Table {
		t := Table{Name: name, Ref: ref, Style: "TableStyleMedium2"}
		for _, c := range columns {
			t.Columns = append(t.Columns, TableColumn{Name: c})
		}
		return t
	}
	good := table("Sales", "A1:B4", "Name", "Amount")
	two := table("Other", "D1:D2", "X")
	long := strings.Repeat("a", 256)
	for name, c := range map[string]struct {
		tables any
		ok     bool
	}{
		"one table":        {[]Table{good}, true},
		"two tables":       {[]Table{good, two}, true},
		"none":             {nil, true},
		"no header":        {[]Table{{Name: "T", Ref: "A1", NoHeader: true, Columns: []TableColumn{{Name: "X"}}}}, true},
		"function":         {[]Table{{Name: "T", Ref: "A1:A3", Totals: true, Columns: []TableColumn{{Name: "X", Total: "sum", Label: "Σ", F: "[@Y]*2", TotalF: "SUM(T[X])"}}}}, true},
		"same names":       {[]Table{good, table("SALES", "D1:D2", "X")}, false},
		"overlap":          {[]Table{good, table("Other", "B3:C6", "X", "Y")}, false},
		"header only":      {[]Table{table("T", "A1:B1", "X", "Y")}, false},
		"totals only":      {[]Table{{Name: "T", Ref: "A1:A2", Totals: true, Columns: []TableColumn{{Name: "X"}}}}, false},
		"columns":          {[]Table{table("T", "A1:B4", "X")}, false},
		"same columns":     {[]Table{table("T", "A1:B4", "X", "x")}, false},
		"empty column":     {[]Table{table("T", "A1:B4", "X", "")}, false},
		"long column":      {[]Table{table("T", "A1:B4", "X", long)}, false},
		"entire columns":   {[]Table{table("T", "A:B", "X", "Y")}, false},
		"cell name":        {[]Table{table("A1", "A1:B4", "X", "Y")}, false},
		"R name":           {[]Table{table("r", "A1:B4", "X", "Y")}, false},
		"space name":       {[]Table{table("My Table", "A1:B4", "X", "Y")}, false},
		"digit name":       {[]Table{table("1T", "A1:B4", "X", "Y")}, false},
		"no name":          {[]Table{table("", "A1:B4", "X", "Y")}, false},
		"negative id":      {[]Table{{ID: -1, Name: "T", Ref: "A1:B4", Columns: good.Columns}}, false},
		"same ids":         {[]Table{{ID: 7, Name: "T", Ref: "A1:B4", Columns: good.Columns}, {ID: 7, Name: "U", Ref: "D1:E4", Columns: good.Columns}}, false},
		"builtin style":    {[]Table{{Name: "T", Ref: "A1:B4", Style: "TableStyleDark11", Columns: good.Columns}}, true},
		"unknown style":    {[]Table{{Name: "T", Ref: "A1:B4", Style: "TableStyleMedium29", Columns: good.Columns}}, false},
		"padded style":     {[]Table{{Name: "T", Ref: "A1:B4", Style: "TableStyleMedium02", Columns: good.Columns}}, false},
		"own style":        {[]Table{{Name: "T", Ref: "A1:B4", Style: "Mine", Columns: good.Columns}}, false},
		"total":            {[]Table{{Name: "T", Ref: "A1:A3", Totals: true, Columns: []TableColumn{{Name: "X", Total: "median"}}}}, false},
		"formula":          {[]Table{{Name: "T", Ref: "A1:A3", Columns: []TableColumn{{Name: "X", F: `1+"abc`}}}}, false},
		"long label":       {[]Table{{Name: "T", Ref: "A1:A3", Totals: true, Columns: []TableColumn{{Name: "X", Label: strings.Repeat("a", maxText+1)}}}}, false},
		"not tables":       {"nope", false},
		"not a table":      {[]Table{{Name: "T", Ref: "nope", Columns: good.Columns}}, false},
		"no columns":       {[]Table{{Name: "T", Ref: "A1:B4"}}, false},
		"too many columns": {[]Table{table("T", "A1:A4", "X", "Y")}, false},
	} {
		err := d.Check(tree, setTables("S1", c.tables), "")
		if (err == nil) != c.ok {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Tables are named once in the workbook, whichever sheet holds them, and
// no defined name is theirs.
func TestTablesChecked_workbook(t *testing.T) {
	d, tree := blankBook(t)
	t1 := Table{ID: 3, Name: "Sales", Ref: "A1:A3", Columns: []TableColumn{{Name: "X"}}}
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpNew, ID: "S2", Type: "sheet", Parent: "book", Key: ot.KeyBetween(tree.Children("book")[0].Key, ""),
			Attrs: ot.Values{"name": json.RawMessage(`"Deux"`), tablesKey: mustJSON([]Table{t1})}, Cells: []ot.Cell{}},
	})
	for name, c := range map[string]struct {
		edit ot.Edit
		ok   bool
	}{
		"same name":   {setTables("S1", []Table{{Name: "sales", Ref: "A1:A3", Columns: t1.Columns}}), false},
		"same id":     {setTables("S1", []Table{{ID: 3, Name: "Other", Ref: "A1:A3", Columns: t1.Columns}}), false},
		"other name":  {setTables("S1", []Table{{ID: 4, Name: "Other", Ref: "A1:A3", Columns: t1.Columns}}), true},
		"moved name":  {append(setTables("S2", nil), setTables("S1", []Table{{Name: "Sales", Ref: "A1:A3", Columns: t1.Columns}})...), true},
		"deleted":     {ot.Edit{{Op: ot.OpDel, ID: "S2"}, setTables("S1", []Table{{Name: "Sales", Ref: "A1:A3", Columns: t1.Columns}})[0]}, true},
		"defined":     {ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"names": json.RawMessage(`[{"name":"Sales","ref":"S1!A1"}]`)}}}, true},
		"sheet names": {append(ot.Edit{{Op: ot.OpSet, ID: "book", Attrs: ot.Values{"names": json.RawMessage(`[{"name":"Other","ref":"S1!A1"}]`)}}}, setTables("S1", []Table{{Name: "Other", Ref: "A1:A3", Columns: t1.Columns}})...), false},
	} {
		if err := d.Check(tree, c.edit, ""); (err == nil) != c.ok {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The ids of the tables of a file belong to their sheets.
func TestTablesCheckedRead(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/tdf162963_TableWithTotalsEnabled.xlsx")
	sheet := firstSheet(tree).ID
	t1 := tablesOf(tree, sheet)[0]
	apply(t, d, tree, setTables(sheet, []Table{t1}))
	apply(t, d, tree, setTables(sheet, []Table{{Name: "Other", Ref: "D1:D3", Columns: []TableColumn{{Name: "X"}}}, t1}))
	new := ot.Edit{{Op: ot.OpNew, ID: "S9", Type: "sheet", Parent: "book", Key: ot.KeyBetween(firstSheet(tree).Key, ""),
		Attrs: ot.Values{"name": json.RawMessage(`"Neuve"`), tablesKey: mustJSON([]Table{t1})}, Cells: []ot.Cell{}}}
	if err := d.Check(tree, new, ""); err == nil {
		t.Error("the table of a sheet taken for a new sheet")
	}
	t1.Style = "TableStyleLight9"
	t1.Ref = "A1:B7"
	apply(t, d, tree, setTables(sheet, []Table{t1}))
	t1.Style = "Mine"
	if err := d.Check(tree, setTables(sheet, []Table{t1}), ""); err == nil {
		t.Error("a style no table shows")
	}
}

func TestTablesCreated(t *testing.T) {
	d, tree := blankBook(t)
	apply(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{
		cell(1, 1, `{"v":"Name"}`), cell(1, 2, `{"v":"Amount"}`), cell(2, 1, `{"v":"a"}`), cell(2, 2, `{"v":1}`), cell(3, 2, `{"v":2}`),
	}}})
	made := Table{Name: "Sales", Ref: "A1:B3", Filter: true, Style: "TableStyleMedium9", Rows: true, Last: true,
		Columns: []TableColumn{{Name: "Name"}, {Name: "Amount", F: "[@Name]&\"x\"", Label: "a < b"}}}
	apply(t, d, tree, setTables("S1", []Table{made}))
	_, again, pkg := saveAndOpen(t, d, tree)
	made.ID = 1
	made.Columns[0].ID, made.Columns[1].ID = 1, 2
	got := tablesOf(again, "S1")
	if g, w := string(mustJSON(got)), string(mustJSON([]Table{made})); g != w {
		t.Errorf("got %s, want %s", g, w)
	}
	if rels := part(t, pkg, "xl/worksheets/_rels/sheet1.xml.rels"); !strings.Contains(rels, `Target="../tables/table1.xml"`) {
		t.Errorf("rels: %s", rels)
	}
	if sheet := part(t, pkg, "xl/worksheets/sheet1.xml"); !strings.Contains(sheet, `<tableParts count="1"><tablePart r:id="rId1"/></tableParts>`) {
		t.Errorf("sheet: %s", sheet)
	}
	if pkg.ContentType("xl/tables/table1.xml") != typeTable {
		t.Errorf("content type %q", pkg.ContentType("xl/tables/table1.xml"))
	}
	x := part(t, pkg, "xl/tables/table1.xml")
	for _, want := range []string{`id="1"`, `totalsRowShown="0"`, `<autoFilter ref="A1:B3"/>`, `<tableColumns count="2">`,
		`<calculatedColumnFormula>[@Name]&amp;"x"</calculatedColumnFormula>`, `showLastColumn="1"`} {
		if !strings.Contains(x, want) {
			t.Errorf("no %s in %s", want, x)
		}
	}
	first, _ := d.Save(tree)
	second, _ := d.Save(tree)
	if string(first) != string(second) {
		t.Error("saving twice differs")
	}

	// a table with no id takes the one after the greatest
	again2, tree2, _ := saveAndOpen(t, d, tree)
	two := Table{Name: "Other", Ref: "D1:D2", Columns: []TableColumn{{Name: "X"}}}
	apply(t, again2, tree2, setTables("S1", []Table{tablesOf(tree2, "S1")[0], two}))
	_, tree3, _ := saveAndOpen(t, again2, tree2)
	if got := tablesOf(tree3, "S1"); len(got) != 2 || got[0].ID != 1 || got[1].ID != 2 || got[1].Name != "Other" {
		t.Errorf("got %+v", got)
	}
}

func TestTablesDeleted(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/simpleTable.xlsx")
	sheet := firstSheet(tree).ID
	apply(t, d, tree, setTables(sheet, nil))
	_, again, pkg := saveAndOpen(t, d, tree)
	if got := tablesOf(again, sheet); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
	for _, name := range pkg.Names() {
		if strings.Contains(name, "tables/") {
			t.Errorf("%s kept", name)
		}
	}
	if x := part(t, pkg, "xl/worksheets/sheet1.xml"); strings.Contains(x, "tablePart") {
		t.Errorf("sheet: %s", x)
	}
	if pkg.Has("xl/worksheets/_rels/sheet1.xml.rels") {
		t.Error("relationships of the sheet kept")
	}
	if ct := part(t, pkg, "[Content_Types].xml"); strings.Contains(ct, "table+xml") {
		t.Errorf("content types: %s", ct)
	}
}

// A totals row comes with a larger area, and keeps what the part holds.
func TestTablesTotals(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/simpleTable.xlsx")
	sheet := firstSheet(tree).ID
	table := tablesOf(tree, sheet)[0]
	table.Totals, table.Ref = true, "A1:C7"
	table.Columns[0].Label = "Total"
	table.Columns[2].Total = "sum"
	apply(t, d, tree, setTables(sheet, []Table{table}))
	_, again, pkg := saveAndOpen(t, d, tree)
	if g, w := string(mustJSON(tablesOf(again, sheet))), string(mustJSON([]Table{table})); g != w {
		t.Errorf("got %s, want %s", g, w)
	}
	x := part(t, pkg, "xl/tables/table1.xml")
	for _, want := range []string{`ref="A1:C7"`, `totalsRowCount="1"`, `<autoFilter ref="A1:C6" xr:uid=`, `xr3:uid="{29356043-88A6-499B-BF6A-AAB979707917}"`,
		`name="Column3" totalsRowFunction="sum"`, `totalsRowLabel="Total"`} {
		if !strings.Contains(x, want) {
			t.Errorf("no %s in %s", want, x)
		}
	}

	table = tablesOf(again, sheet)[0]
	table.Totals, table.Ref = false, "A1:C6"
	apply(t, d, tree, setTables(sheet, []Table{table}))
	d2, tree2, _ := saveAndOpen(t, d, tree)
	apply(t, d2, tree2, setTables(sheet, []Table{table}))
	_, _, pkg = saveAndOpen(t, d2, tree2)
	if x := part(t, pkg, "xl/tables/table1.xml"); strings.Contains(x, "totalsRowCount") || !strings.Contains(x, `<autoFilter ref="A1:C6"`) {
		t.Errorf("totals row kept: %s", x)
	}
}

// Columns come and go, and the filters of the columns left follow them.
func TestTablesColumns(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/tdf162963_TableWithTotalsEnabled.xlsx")
	sheet := firstSheet(tree).ID
	table := tablesOf(tree, sheet)[0]
	table.Ref = "A1:C6"
	table.Columns = append([]TableColumn{{Name: "Added"}}, table.Columns...)
	apply(t, d, tree, setTables(sheet, []Table{table}))
	_, again, pkg := saveAndOpen(t, d, tree)
	got := tablesOf(again, sheet)[0]
	if len(got.Columns) != 3 || got.Columns[0].Name != "Added" || got.Columns[0].ID != 3 || got.Columns[2].ID != 2 || got.Columns[2].TotalF != "SUM(myData[Sales])" {
		t.Errorf("got %+v", got)
	}
	x := part(t, pkg, "xl/tables/table1.xml")
	for _, want := range []string{`<filterColumn colId="1" hiddenButton="1"/><filterColumn colId="2" hiddenButton="1"/>`, `<tableColumns count="3">`} {
		if !strings.Contains(x, want) {
			t.Errorf("no %s in %s", want, x)
		}
	}

	table = tablesOf(tree, sheet)[0]
	table.Ref = "B1:B6"
	table.Columns = table.Columns[2:]
	apply(t, d, tree, setTables(sheet, []Table{table}))
	_, _, pkg = saveAndOpen(t, d, tree)
	x = part(t, pkg, "xl/tables/table1.xml")
	if !strings.Contains(x, `<autoFilter ref="B1:B5" xr:uid="{41042251-8F4C-48D1-B590-4B9784287E5F}"><filterColumn colId="0" hiddenButton="1"/></autoFilter>`) {
		t.Errorf("filter: %s", x)
	}
}

// The tables of a sheet deleted go with it.
func TestTablesSheetDeleted(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/tableref-column-linebreak.xlsx")
	sheet := firstSheet(tree).ID
	tables := tablesOf(tree, sheet)
	if len(tables) != 2 || tables[1].Columns[0].Name != "Also LONG\nBUT TWO LINES" {
		t.Fatalf("got %+v", tables)
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpNew, ID: "S9", Type: "sheet", Parent: "book", Key: ot.KeyBetween(firstSheet(tree).Key, ""),
		Attrs: ot.Values{"name": json.RawMessage(`"Neuve"`)}, Cells: []ot.Cell{}}, {Op: ot.OpDel, ID: sheet}})
	_, _, pkg := saveAndOpen(t, d, tree)
	for _, name := range pkg.Names() {
		if strings.Contains(name, "tables/") {
			t.Errorf("%s kept", name)
		}
	}
}

// The tables of rows inserted are those the tree holds, not those of the part.
func TestTablesMoved(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/simpleTable.xlsx")
	sheet := firstSheet(tree).ID
	ins := ot.Edit{{Op: ot.OpIns, ID: sheet, Dim: ot.DimRows, At: 1, N: 2}}
	if err := moveRows(t, d, tree, ins); err != nil {
		t.Fatal(err)
	}
	table := tablesOf(tree, sheet)[0]
	shifted, _ := table.shifted(str(tree.Node(sheet), "name"), true, 1, 2)
	apply(t, d, tree, setTables(sheet, []Table{shifted}))
	_, again, pkg := saveAndOpen(t, d, tree)
	if got := tablesOf(again, sheet)[0].Ref; got != "A3:C8" {
		t.Errorf("got %s", got)
	}
	if x := part(t, pkg, "xl/tables/table1.xml"); !strings.Contains(x, `ref="A3:C8"`) || !strings.Contains(x, `<autoFilter ref="A3:C8"`) {
		t.Errorf("part: %s", x)
	}
}

func TestTablesErrTable(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/simpleTable.xlsx")
	err := d.Check(tree, ot.Edit{{Op: ot.OpRem, ID: firstSheet(tree).ID, Dim: ot.DimRows, At: 1, N: 1}}, "")
	if !errors.Is(err, ErrTable) {
		t.Errorf("got %v", err)
	}
}

// sent checks an edit as the server does, then applies it and what follows.
func sent(t *testing.T, d *Document, tree *ot.Tree, c *Calc, e ot.Edit) {
	t.Helper()
	if err := moveRows(t, d, tree, e); err != nil {
		t.Fatalf("%v refused: %v", e, err)
	}
	c.Follow(e, nil)
}

// The edits the editor sends: a row inserted in an edit of its own, then
// the tables, and a totals row turned on under a single row of data.
func TestTablesSentEdits(t *testing.T) {
	d, tree := blankBook(t)
	c := NewCalc(tree, formula.Options{})
	sent(t, d, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":1}`), cell(1, 2, `{"v":2}`)}}})
	made := Table{Name: "Tableau1", Ref: "A1:B2", Filter: true, Style: "TableStyleMedium2", Rows: true,
		Columns: []TableColumn{{ID: 1, Name: "Colonne1"}, {ID: 2, Name: "Colonne2"}}}
	both := append(ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 1, N: 1}}, setTables("S1", []Table{made})...)
	if err := d.Check(tree, both, ""); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("rows inserted with other changes: %v", err)
	}
	sent(t, d, tree, c, both[:1])
	sent(t, d, tree, c, append(ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":"Colonne1"}`), cell(1, 2, `{"v":"Colonne2"}`)}}},
		setTables("S1", []Table{made})...))
	if got := fieldsAt(tree, "S1", 2, 2); got != `{"v":2}` {
		t.Fatalf("data %s", got)
	}

	table := tablesOf(tree, "S1")[0]
	table.Totals = true
	sent(t, d, tree, c, setTables("S1", []Table{table}))
	table = tablesOf(tree, "S1")[0]
	if table.Ref != "A1:B3" || table.Columns[1].Total != "sum" {
		t.Fatalf("totals %+v", table)
	}
	if got := fieldsAt(tree, "S1", 3, 2); !strings.Contains(got, `"f":"SUBTOTAL(109,Tableau1[Colonne2])"`) {
		t.Errorf("total %s", got)
	}

	table.Totals = false
	sent(t, d, tree, c, setTables("S1", []Table{table}))
	table = tablesOf(tree, "S1")[0]
	sent(t, d, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(3, 1, `{"v":"y"}`)}}})
	sent(t, d, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 3, N: 1}})
	table.Totals = true
	sent(t, d, tree, c, setTables("S1", []Table{table}))
	if got := tablesOf(tree, "S1")[0].Ref; got != "A1:B3" {
		t.Errorf("ref %s", got)
	}
	if got := fieldsAt(tree, "S1", 4, 1); got != `{"v":"y"}` {
		t.Errorf("below %s", got)
	}
}
