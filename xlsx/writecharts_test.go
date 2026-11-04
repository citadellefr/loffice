package xlsx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

// chartBook is a blank workbook whose first sheet holds sales by quarter,
// north and south.
func chartBook(t *testing.T) (*Document, *ot.Tree, *Calc, string) {
	t.Helper()
	data, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	id := firstSheet(tree).ID
	c := NewCalc(tree, formula.Options{})
	chartFollowed(t, d, tree, c, ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{
		cell(1, 2, `{"v":"Nord"}`), cell(1, 3, `{"v":"Sud"}`),
		cell(2, 1, `{"v":"T1"}`), cell(2, 2, `{"v":1}`), cell(2, 3, `{"v":3}`),
		cell(3, 1, `{"v":"T2"}`), cell(3, 2, `{"v":2.5}`), cell(3, 3, `{"v":4}`),
		cell(4, 1, `{"v":"T3"}`), cell(4, 2, `{"v":-4}`), cell(4, 3, `{"f":"C3+1","v":5}`),
	}}})
	return d, tree, c, id
}

// chartFollowed checks an edit, applies it and what follows it.
func chartFollowed(t *testing.T, d *Document, tree *ot.Tree, c *Calc, e ot.Edit) {
	t.Helper()
	apply(t, d, tree, e)
	c.Follow(e, nil)
}

func setCharts(t *testing.T, d *Document, tree *ot.Tree, c *Calc, id string, list []Placed) {
	t.Helper()
	chartFollowed(t, d, tree, c, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: mustJSON(list)}}})
}

func chartsOf(tree *ot.Tree, id string) []Placed {
	return placedCharts(tree.Node(id))
}

// savedChart saves the workbook and opens it again; with LOFFICE_EDITED, it is
// also written there for the validator.
func savedChart(t *testing.T, d *Document, tree *ot.Tree, name string) ([]byte, *Document, *ot.Tree) {
	t.Helper()
	data, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("LOFFICE_EDITED"); out != "" {
		path := filepath.Join(out, "charts", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, tree, err = Open(data, Limits{})
	if err != nil {
		t.Fatalf("saved workbook does not open: %v", err)
	}
	return data, d, tree
}

func chartPart(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	pkg, err := opc.Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !pkg.Has(name) {
		return nil
	}
	b, err := pkg.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func inserted(kind, dir string) Placed {
	return Placed{From: &Corner{Col: 4, Row: 1}, To: &Corner{Col: 11, Row: 16, DX: 1000}, Source: "A1:C4",
		Chart: &chart.Chart{Title: &chart.Title{Text: "Ventes"}, Legend: &chart.Legend{Pos: "r"}, Plots: []chart.Plot{{Kind: kind, Dir: dir}}}}
}

func TestChartInsert(t *testing.T) {
	for _, k := range []struct{ kind, dir string }{{"bar", "col"}, {"bar", "bar"}, {"line", ""}, {"pie", ""}, {"scatter", ""}, {"area", ""}} {
		d, tree, c, id := chartBook(t)
		p := inserted(k.kind, k.dir)
		if k.kind == "pie" {
			p.Chart.Title = &chart.Title{}
			p.Source = "'" + str(tree.Node(id), "name") + "'!$A$1:$C$4"
		}
		setCharts(t, d, tree, c, id, []Placed{p})
		if p := chartsOf(tree, id)[0]; p.Source != "" || len(p.Chart.Plots[0].Series) != 2 {
			t.Fatalf("%s: not expanded: %+v", k.kind, p)
		}
		data, _, again := savedChart(t, d, tree, k.kind+k.dir+".xlsx")
		list := chartsOf(again, id)
		if len(list) != 1 {
			t.Fatalf("%s: charts %+v", k.kind, list)
		}
		p = list[0]
		if *p.From != (Corner{Col: 4, Row: 1}) || *p.To != (Corner{Col: 11, Row: 16, DX: 1000}) || p.ID == "" || chartPart(t, data, p.ID) == nil {
			t.Errorf("%s: placed %+v %+v %s", k.kind, p.From, p.To, p.ID)
		}
		pl := p.Chart.Plots[0]
		own := str(again.Node(id), "name")
		if pl.Kind != k.kind || k.dir != "" && pl.Dir != k.dir || len(pl.Series) != 2 || p.Chart.Title == nil || k.kind != "pie" && p.Chart.Title.Text != "Ventes" {
			t.Fatalf("%s: %+v", k.kind, p.Chart)
		}
		s := pl.Series[1]
		if s.Name.Str[0] != "Sud" || s.Val.Ref != own+"!$C$2:$C$4" || *s.Val.Num[2] != 5 || s.Cat.Ref != own+"!$A$2:$A$4" || s.Cat.Str[1] != "T2" {
			t.Errorf("%s: series %+v %+v %+v", k.kind, s.Name, s.Val, s.Cat)
		}
	}
}

// chartSaved is the chart book with a column chart inserted, saved and
// opened again: a chart the workbook has.
func chartSaved(t *testing.T) ([]byte, *Document, *ot.Tree, *Calc, string) {
	t.Helper()
	d, tree, c, id := chartBook(t)
	setCharts(t, d, tree, c, id, []Placed{inserted("bar", "col")})
	data, d, tree := savedChart(t, d, tree, "inserted.xlsx")
	return data, d, tree, NewCalc(tree, formula.Options{}), id
}

func TestChartUntouched(t *testing.T) {
	data, d, tree, _, _ := chartSaved(t)
	again, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _ := opc.Open(data, opc.Limits{})
	for _, name := range pkg.Names() {
		if !bytes.Equal(chartPart(t, data, name), chartPart(t, again, name)) {
			t.Errorf("%s rewritten", name)
		}
	}
}

func TestChartPlace(t *testing.T) {
	data, d, tree, c, id := chartSaved(t)
	list := chartsOf(tree, id)
	name := list[0].ID
	list[0].From = &Corner{Col: 1, Row: 6, DX: 500}
	list[0].To = &Corner{Col: 9, Row: 30}
	setCharts(t, d, tree, c, id, list)
	moved, _, again := savedChart(t, d, tree, "moved.xlsx")
	p := chartsOf(again, id)[0]
	if *p.From != (Corner{Col: 1, Row: 6, DX: 500}) || *p.To != (Corner{Col: 9, Row: 30}) {
		t.Errorf("moved: %+v %+v", p.From, p.To)
	}
	if !bytes.Equal(chartPart(t, data, name), chartPart(t, moved, name)) {
		t.Error("chart rewritten when moved")
	}
	drawing := "xl/drawings/drawing1.xml"
	frame := regexp.MustCompile(`(?s)<xdr:graphicFrame.*</xdr:graphicFrame>`)
	if a, b := frame.Find(chartPart(t, data, drawing)), frame.Find(chartPart(t, moved, drawing)); a == nil || !bytes.Equal(a, b) {
		t.Errorf("frame rewritten:\n%s\n%s", a, b)
	}

	setCharts(t, d, tree, c, id, nil)
	removed, _, again := savedChart(t, d, tree, "removed.xlsx")
	if len(chartsOf(again, id)) != 0 || chartPart(t, removed, name) != nil || chartPart(t, removed, opc.RelsName(drawing)) != nil {
		t.Errorf("chart kept: %s", chartPart(t, removed, drawing))
	}
	if bytes.Contains(chartPart(t, removed, "[Content_Types].xml"), []byte(name)) {
		t.Error("content type kept")
	}
}

func TestChartPatch(t *testing.T) {
	data, d, tree, c, id := chartSaved(t)
	list := chartsOf(tree, id)
	name := list[0].ID
	ch := list[0].Chart
	ch.Title.Text = "Chiffre"
	ch.Legend.Pos = "b"
	ch.Axes[0].Title = &chart.Title{Text: "Trimestre"}
	ch.Axes[1].Title = &chart.Title{Text: "Euros"}
	setCharts(t, d, tree, c, id, list)
	out, _, again := savedChart(t, d, tree, "titled.xlsx")
	got := chartsOf(again, id)[0].Chart
	if got.Title.Text != "Chiffre" || got.Legend.Pos != "b" || got.Axes[0].Title.Text != "Trimestre" || got.Axes[1].Title.Text != "Euros" {
		t.Errorf("edited: %+v %+v %+v", got.Title, got.Legend, got.Axes)
	}
	plot := regexp.MustCompile(`(?s)<c:barChart>.*</c:barChart>`)
	if a, b := plot.Find(chartPart(t, data, name)), plot.Find(chartPart(t, out, name)); a == nil || !bytes.Equal(a, b) {
		t.Errorf("plot rewritten:\n%s\n%s", a, b)
	}

	list = chartsOf(tree, id)
	ch = list[0].Chart
	ch.Plots[0].Kind, ch.Plots[0].Dir = "line", ""
	ch.Plots[0].Series = ch.Plots[0].Series[:1]
	ch.Plots[0].Series[0].Val.Ref = "C2:C4"
	setCharts(t, d, tree, c, id, list)
	out, _, again = savedChart(t, d, tree, "retyped.xlsx")
	got = chartsOf(again, id)[0].Chart
	s := got.Plots[0].Series
	if got.Plots[0].Kind != "line" || len(s) != 1 || s[0].Val.Ref != str(again.Node(id), "name")+"!C2:C4" || *s[0].Val.Num[0] != 3 {
		t.Errorf("retyped: %+v", got.Plots[0])
	}
	if strings.Contains(string(chartPart(t, out, name)), "barChart") {
		t.Error("bar kept")
	}
}

func TestChartCache(t *testing.T) {
	data, d, tree, c, id := chartSaved(t)
	name := chartsOf(tree, id)[0].ID
	chartFollowed(t, d, tree, c, ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{cell(3, 2, `{"v":42}`), cell(3, 3, `{"v":7}`)}}})
	out, _, again := savedChart(t, d, tree, "recalculated.xlsx")
	s := chartsOf(again, id)[0].Chart.Plots[0].Series
	if *s[0].Val.Num[1] != 42 || *s[1].Val.Num[1] != 7 || *s[1].Val.Num[2] != 8 {
		t.Errorf("cache: %v %v %v", *s[0].Val.Num[1], *s[1].Val.Num[1], *s[1].Val.Num[2])
	}
	cat := regexp.MustCompile(`(?s)<c:cat>.*?</c:cat>`)
	if a, b := cat.Find(chartPart(t, data, name)), cat.Find(chartPart(t, out, name)); a == nil || !bytes.Equal(a, b) {
		t.Errorf("categories rewritten:\n%s\n%s", a, b)
	}
}

func TestChartCheck(t *testing.T) {
	_, d, tree, _, id := chartSaved(t)
	list := chartsOf(tree, id)
	bad := map[string][]Placed{}
	clone := func() []Placed {
		var out []Placed
		_ = json.Unmarshal(mustJSON(list), &out)
		return out
	}
	l := clone()
	l[0].ID = "xl/charts/chart9.xml"
	bad["unknown id"] = l
	bad["twice"] = append(clone(), clone()...)
	l = clone()
	l[0].Chart.Plots[0].Kind = "radar"
	bad["kind"] = l
	l = clone()
	l[0].Chart.Plots[0].Series[0].Val.Ref = "[1]Feuil1!A1:A3"
	bad["book"] = l
	l = clone()
	l[0].ID, l[0].From = "", nil
	bad["no anchor"] = l
	l = clone()
	l[0].Chart.Plots[0].Series[0].Index = -1 << 62
	bad["index"] = l
	l = clone()
	l[0].Chart.Axes[0].Kind = `a b="1"`
	bad["axis kind"] = l
	l = clone()
	l[0].Chart.Plots[0].Grouping = "x"
	bad["grouping"] = l
	p := inserted("bar", "")
	p.Source = "A1:C4,E1"
	bad["source"] = []Placed{p}
	p = inserted("bar", "")
	p.Source = "A1:XFD60000"
	bad["source too large"] = []Placed{p}
	p = inserted("bar", "")
	p.Source = "A1:IV20000"
	bad["source of many cells"] = []Placed{p}
	var many []Placed
	for range 17 {
		p = inserted("bar", "")
		p.Source = "A1:A65536"
		many = append(many, p)
	}
	bad["sources of many cells"] = many
	l = clone()
	l[0].Chart.Plots[0].Series[0].Name = &chart.Data{Count: 1, Str: []string{"a\x01b"}}
	bad["control character"] = l
	l = clone()
	l[0].Chart.Plots[0].Series[0].Val.Format = "\x02"
	bad["control character in a format"] = l
	for why, l := range bad {
		if d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: mustJSON(l)}}}, "") == nil {
			t.Errorf("%s taken", why)
		}
	}
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: mustJSON(append(clone(), inserted("pie", "")))}}}, ""); err != nil {
		t.Error(err)
	}
}

func TestChartCorpusUntouched(t *testing.T) {
	d, tree := openCorpus(t, "poi/WithTwoCharts.xlsx")
	var id string
	for _, n := range tree.Children("book") {
		if len(placedCharts(n)) > 0 {
			id = n.ID
		}
	}
	data, _ := os.ReadFile("../corpus/files/poi/WithTwoCharts.xlsx")
	list := placedCharts(tree.Node(id))
	name := list[0].ID
	list[0].Chart.Title = &chart.Title{Text: "Édité"}
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: mustJSON(list)}}}, ""); err != nil {
		t.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	setCharts(t, d, tree, c, id, list)
	out, _, again := savedChart(t, d, tree, "WithTwoCharts.xlsx")
	if got := placedCharts(again.Node(id))[0].Chart.Title.Text; got != "Édité" {
		t.Errorf("title: %q", got)
	}
	pkg, _ := opc.Open(data, opc.Limits{})
	for _, n := range pkg.Names() {
		if n != name && !bytes.Equal(chartPart(t, data, n), chartPart(t, out, n)) {
			t.Errorf("%s rewritten", n)
		}
	}
	plot := regexp.MustCompile(`(?s)<c:plotArea>.*</c:plotArea>`)
	if a, b := plot.Find(chartPart(t, data, name)), plot.Find(chartPart(t, out, name)); a == nil || !bytes.Equal(a, b) {
		t.Error("plot area rewritten")
	}
}

func TestChartRowsRemoved(t *testing.T) {
	_, d, tree, c, id := chartSaved(t)
	chartFollowed(t, d, tree, c, ot.Edit{{Op: ot.OpRem, ID: id, Dim: ot.DimRows, At: 2, N: 3}})
	list := chartsOf(tree, id)
	if ref := list[0].Chart.Plots[0].Series[0].Val.Ref; !strings.Contains(ref, "#REF!") {
		t.Fatalf("ref: %s", ref)
	}
	list[0].From.Col++
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: mustJSON(list)}}}, ""); err != nil {
		t.Error(err)
	}
}

func TestChartUnreadKept(t *testing.T) {
	data, _, _, _, _ := chartSaved(t)
	pkg, err := opc.Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	name := "xl/charts/chart1.xml"
	if err := pkg.Set(name, []byte(`<oops/>`)); err != nil {
		t.Fatal(err)
	}
	if data, err = pkg.Bytes(); err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(chartPart(t, out, name), []byte(`<oops/>`)) || !bytes.Contains(chartPart(t, out, "xl/drawings/drawing1.xml"), []byte("graphicFrame")) {
		t.Error("a chart not read is removed")
	}
}

func TestChartTitleKept(t *testing.T) {
	long := &chart.Title{Text: strings.Repeat("a", maxChartTitle+1)}
	if !checkChartTitle(long, &chart.Title{Text: long.Text}) || checkChartTitle(long, nil) {
		t.Error("a title too long is refused only when it changes")
	}
}

func TestChartCategoriesTurnText(t *testing.T) {
	_, d, tree, c, id := chartSaved(t)
	chartFollowed(t, d, tree, c, ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{cell(2, 1, `{"v":2001}`), cell(3, 1, `{"v":2002}`), cell(4, 1, `{"v":2003}`)}}})
	list := chartsOf(tree, id)
	s := list[0].Chart.Plots[0].Series
	for i := range s {
		ref := s[i].Cat.Ref
		s[i].Cat = &chart.Data{Ref: ref, Count: 3, Num: []*float64{ptr(2001), ptr(2002), ptr(2003)}}
	}
	setCharts(t, d, tree, c, id, list)
	_, d, tree = savedChart(t, d, tree, "numeric-categories.xlsx")
	if s := chartsOf(tree, id)[0].Chart.Plots[0].Series[0]; s.Cat.Num == nil {
		t.Fatalf("categories: %+v", s.Cat)
	}
	c = NewCalc(tree, formula.Options{})
	chartFollowed(t, d, tree, c, ot.Edit{{Op: ot.OpCel, ID: id, Cells: []ot.Cell{cell(3, 1, `{"v":"deux"}`)}}})
	_, _, tree = savedChart(t, d, tree, "text-category.xlsx")
	cat := chartsOf(tree, id)[0].Chart.Plots[0].Series[0].Cat
	if cat.Num != nil || strings.Join(cat.Str, " ") != "2001 deux 2003" {
		t.Errorf("categories: %+v", cat)
	}
}

func ptr(v float64) *float64 { return &v }

// redrawn is the chart book saved with its drawing changed by change.
func redrawn(t *testing.T, change func(drawing string) string) (*Document, *ot.Tree, string) {
	t.Helper()
	data, _, _, _, id := chartSaved(t)
	pkg, err := opc.Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	const name = "xl/drawings/drawing1.xml"
	if err := pkg.Set(name, []byte(change(string(chartPart(t, data, name))))); err != nil {
		t.Fatal(err)
	}
	if data, err = pkg.Bytes(); err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return d, tree, id
}

func TestChartGroupedRemoved(t *testing.T) {
	d, tree, id := redrawn(t, func(s string) string {
		frame := regexp.MustCompile(`(?s)<xdr:graphicFrame.*</xdr:graphicFrame>`).FindString(s)
		group := `<xdr:grpSp><xdr:nvGrpSpPr><xdr:cNvPr id="9" name="Groupe"/><xdr:cNvGrpSpPr/></xdr:nvGrpSpPr><xdr:grpSpPr/>` + frame + `</xdr:grpSp>`
		return strings.Replace(s, frame, group, 1)
	})
	if len(chartsOf(tree, id)) != 1 {
		t.Fatal("grouped chart not read")
	}
	setCharts(t, d, tree, NewCalc(tree, formula.Options{}), id, []Placed{})
	out, _, _ := savedChart(t, d, tree, "grouped.xlsx")
	drawing := chartPart(t, out, "xl/drawings/drawing1.xml")
	if !bytes.Contains(drawing, []byte("grpSp")) || bytes.Contains(drawing, []byte("graphicFrame")) || chartPart(t, out, "xl/charts/chart1.xml") != nil {
		t.Errorf("drawing:\n%s", drawing)
	}
}

func TestChartAnchoredTwice(t *testing.T) {
	d, tree, id := redrawn(t, func(s string) string {
		anchor := regexp.MustCompile(`(?s)<xdr:twoCellAnchor.*</xdr:twoCellAnchor>`).FindString(s)
		return strings.Replace(s, anchor, anchor+anchor, 1)
	})
	list := chartsOf(tree, id)
	if len(list) != 1 {
		t.Fatalf("charts: %d", len(list))
	}
	list[0].From.Col++
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: mustJSON(list)}}}, ""); err != nil {
		t.Error(err)
	}
	setCharts(t, d, tree, NewCalc(tree, formula.Options{}), id, []Placed{})
	out, _, _ := savedChart(t, d, tree, "anchored-twice.xlsx")
	if bytes.Contains(chartPart(t, out, "xl/drawings/drawing1.xml"), []byte("graphicFrame")) || chartPart(t, out, "xl/charts/chart1.xml") != nil {
		t.Error("an anchor of a chart removed stays")
	}
}
