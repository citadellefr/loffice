package xlsx

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/trame/ot"
)

// bookChart is a chart of the sales of chartBook, by column.
func bookChart() *chart.Chart {
	series := func(index int, name, val string) chart.Series {
		return chart.Series{Index: index, Order: index, Name: &chart.Data{Ref: name, Count: 1}, Cat: &chart.Data{Ref: "Feuille1!$A$2:$A$4"}, Val: &chart.Data{Ref: val}}
	}
	return &chart.Chart{Plots: []chart.Plot{{Kind: "bar", Dir: "col", Grouping: "clustered", Series: []chart.Series{
		series(0, "Feuille1!$B$1", "Feuille1!$B$2:$B$4"),
		series(1, "Feuille1!$C$1", "Feuille1!$C$2:$C$4"),
	}}}}
}

func openBook(t *testing.T, c *chart.Chart, e ...ot.Change) *ChartBook {
	t.Helper()
	d, tree, calc, id := chartBook(t)
	for i := range e {
		e[i].ID = id
	}
	chartFollowed(t, d, tree, calc, e)
	data, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	b, err := OpenChartBook(data, c, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func clone(t *ChartTable) *ChartTable {
	var out ChartTable
	data, _ := json.Marshal(t)
	_ = json.Unmarshal(data, &out)
	return &out
}

func TestChartBookTable(t *testing.T) {
	b := openBook(t, bookChart())
	want := &ChartTable{Categories: []any{"T1", "T2", "T3"}, Series: []ChartColumn{
		{Index: 0, Name: "Nord", Values: []any{1.0, 2.5, -4.0}},
		{Index: 1, Name: "Sud", Values: []any{3.0, 4.0, 5.0}},
	}}
	if !reflect.DeepEqual(b.Table(), want) {
		t.Fatalf("table %+v", b.Table())
	}
	if !b.Same(clone(want)) {
		t.Fatal("a table read back differs")
	}
}

func TestChartBookGrows(t *testing.T) {
	b := openBook(t, bookChart())
	table := clone(b.Table())
	table.Categories = append(table.Categories, "T4")
	table.Series[0].Values = append(table.Series[0].Values, 7.0)
	table.Series[1].Values = append(table.Series[1].Values, nil)
	table.Series = append(table.Series, ChartColumn{Index: 5, Name: "Est", Values: []any{1.0, 2.0, 3.0, 4.0}})
	if err := b.Check(table); err != nil {
		t.Fatal(err)
	}
	c := bookChart()
	got, err := b.Chart(table, c)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, table) {
		t.Fatalf("table %+v", got)
	}
	s := c.Plots[0].Series
	if len(s) != 3 || s[0].Val.Ref != "Feuille1!$B$2:$B$5" || s[0].Cat.Ref != "Feuille1!$A$2:$A$5" || s[2].Val.Ref != "Feuille1!$D$2:$D$5" || s[2].Name.Ref != "Feuille1!$D$1" {
		t.Fatalf("series %+v", s)
	}
	if s[2].Name.Str[0] != "Est" || *s[2].Val.Num[3] != 4 || s[1].Val.Num[3] != nil || s[0].Cat.Str[3] != "T4" {
		t.Fatalf("cached %+v", s[2])
	}
	data, err := b.Save(table)
	if err != nil {
		t.Fatal(err)
	}
	again, err := OpenChartBook(data, c, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !again.Same(table) {
		t.Fatalf("saved table %+v", again.Table())
	}
}

func TestChartBookShrinks(t *testing.T) {
	b := openBook(t, bookChart())
	table := clone(b.Table())
	table.Categories = table.Categories[:2]
	table.Series = table.Series[:1]
	table.Series[0].Values = table.Series[0].Values[:2]
	c := bookChart()
	if _, err := b.Chart(table, c); err != nil {
		t.Fatal(err)
	}
	if s := c.Plots[0].Series; len(s) != 1 || s[0].Val.Ref != "Feuille1!$B$2:$B$3" {
		t.Fatalf("series %+v", s)
	}
	tree, _, _, err := b.edit(table)
	if err != nil {
		t.Fatal(err)
	}
	sheet := firstSheet(tree)
	for _, at := range [][2]int{{4, 1}, {4, 2}, {1, 3}, {2, 3}, {4, 3}} {
		if !blankCell(sheet.Grid.Cell(at[0], at[1])) {
			t.Errorf("cell %v stays: %s", at, sheet.Grid.Cell(at[0], at[1]))
		}
	}
}

func TestChartBookKeepsOtherCells(t *testing.T) {
	b := openBook(t, bookChart(), ot.Change{Op: ot.OpCel, Cells: []ot.Cell{cell(5, 2, `{"v":"Total"}`)}})
	table := clone(b.Table())
	table.Categories = append(table.Categories, "T4")
	table.Series[0].Values = append(table.Series[0].Values, 7.0)
	table.Series[1].Values = append(table.Series[1].Values, 8.0)
	if err := b.Check(table); !errors.Is(err, ErrChartCells) {
		t.Fatalf("got %v", err)
	}
}

func TestChartBookPlacesSeriesPastCells(t *testing.T) {
	b := openBook(t, bookChart(), ot.Change{Op: ot.OpCel, Cells: []ot.Cell{cell(3, 4, `{"v":"note"}`)}})
	table := clone(b.Table())
	table.Series = append(table.Series, ChartColumn{Index: 2, Name: "Est", Values: []any{1.0, 2.0, 3.0}})
	c := bookChart()
	if _, err := b.Chart(table, c); err != nil {
		t.Fatal(err)
	}
	if got := c.Plots[0].Series[2].Val.Ref; got != "Feuille1!$E$2:$E$4" {
		t.Fatalf("new series reads %s", got)
	}
}

func TestChartBookKeepsFormulas(t *testing.T) {
	b := openBook(t, bookChart())
	table := clone(b.Table())
	table.Series[1].Values[1] = 10.0
	c := bookChart()
	got, err := b.Chart(table, c)
	if err != nil {
		t.Fatal(err)
	}
	if got.Series[1].Values[2] != 11.0 || *c.Plots[0].Series[1].Val.Num[2] != 11 {
		t.Fatalf("formula not calculated: %+v", got)
	}
	tree, _, _, err := b.edit(got)
	if err != nil {
		t.Fatal(err)
	}
	if !holdsFormula(firstSheet(tree).Grid.Cell(4, 3)) {
		t.Fatal("a calculated table wrote over the formula")
	}
	got.Series[1].Values[2] = 20.0
	if tree, _, _, err = b.edit(got); err != nil {
		t.Fatal(err)
	}
	if holdsFormula(firstSheet(tree).Grid.Cell(4, 3)) {
		t.Fatal("a value typed left the formula")
	}
}

func TestChartBookRefusesTables(t *testing.T) {
	b := openBook(t, bookChart())
	for name, change := range map[string]func(*ChartTable){
		"no series":      func(t *ChartTable) { t.Series = nil },
		"no rows":        func(t *ChartTable) { t.Categories, t.Series[0].Values, t.Series[1].Values = []any{}, nil, nil },
		"lengths":        func(t *ChartTable) { t.Series[1].Values = t.Series[1].Values[:2] },
		"categories":     func(t *ChartTable) { t.Categories = nil },
		"same index":     func(t *ChartTable) { t.Series[1].Index = 0 },
		"negative index": func(t *ChartTable) { t.Series[1].Index = -1 },
		"value":          func(t *ChartTable) { t.Series[0].Values[0] = true },
		"control":        func(t *ChartTable) { t.Series[0].Values[0] = "a\x01" },
		"name":           func(t *ChartTable) { t.Series[0].Name = "a\x01" },
	} {
		table := clone(b.Table())
		change(table)
		if err := b.Check(table); !errors.Is(err, ErrChartTable) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestChartBookRefusesLayouts(t *testing.T) {
	d, tree, _, _ := chartBook(t)
	data, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*chart.Chart){
		"pivot":      func(c *chart.Chart) { c.Pivot = true },
		"other book": func(c *chart.Chart) { c.Plots[0].Series[0].Val.Ref = "[2]Feuille1!$B$2:$B$4" },
		"no sheet":   func(c *chart.Chart) { c.Plots[0].Series[0].Val.Ref = "Other!$B$2:$B$4" },
		"area":       func(c *chart.Chart) { c.Plots[0].Series[0].Val.Ref = "Feuille1!$B$2:$C$4" },
		"lengths":    func(c *chart.Chart) { c.Plots[0].Series[0].Val.Ref = "Feuille1!$B$2:$B$3" },
		"sizes":      func(c *chart.Chart) { c.Plots[0].Series[0].Size = &chart.Data{Ref: "Feuille1!$C$2:$C$4"} },
		"categories": func(c *chart.Chart) { c.Plots[0].Series[1].Cat.Ref = "Feuille1!$D$2:$D$4" },
		"name":       func(c *chart.Chart) { c.Plots[0].Series[0].Name.Ref = "Feuille1!$A$1:$B$1" },
		"same index": func(c *chart.Chart) { c.Plots[0].Series[1].Index = 0 },
	} {
		c := bookChart()
		change(c)
		if _, err := OpenChartBook(data, c, Limits{}); !errors.Is(err, ErrChartLayout) {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

func TestChartBookRenamesTableColumn(t *testing.T) {
	b := openBook(t, bookChart(), ot.Change{Op: ot.OpSet, Attrs: ot.Values{tablesKey: json.RawMessage(
		`[{"id":1,"name":"Ventes","ref":"A1:C4","columns":[{"id":1,"name":"Column1"},{"id":2,"name":"Nord"},{"id":3,"name":"Sud"}]}]`)}})
	table := clone(b.Table())
	table.Series[0].Name = "Nord-Est"
	tree, _, _, err := b.edit(table)
	if err != nil {
		t.Fatal(err)
	}
	tables := tablesOf(tree, firstSheet(tree).ID)
	if len(tables) != 1 || tables[0].Columns[1].Name != "Nord-Est" {
		t.Fatalf("tables %+v", tables)
	}
}
