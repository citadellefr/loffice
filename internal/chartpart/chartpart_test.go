package chartpart_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/loffice/internal/chartpart"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/pptx"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

// file is a Word document or a presentation open for editing.
type file interface {
	Check(tree *ot.Tree, e ot.Edit, author string) error
	Follow(tree *ot.Tree, e ot.Edit) ot.Edit
	Save(tree *ot.Tree) ([]byte, error)
}

func open(name string, data []byte) (file, *ot.Tree, error) {
	if strings.HasSuffix(name, ".docx") {
		return docx.Open(data, opc.Limits{})
	}
	return pptx.Open(data, opc.Limits{})
}

func reopen(t *testing.T, name string, data []byte) *ot.Tree {
	t.Helper()
	_, tree, err := open(name, data)
	if err != nil {
		t.Fatalf("%s: saved file does not open: %v", name, err)
	}
	return tree
}

func read(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../corpus/files/" + name)
	if err != nil {
		t.Skip("no corpus: go run ./tools/corpus")
	}
	return data
}

// nodes are the nodes that hold an editable chart, in the order of the tree.
func nodes(tree *ot.Tree) []string {
	var out []string
	for _, c := range tree.Edit() {
		if c.Attrs[chartpart.ChartKey] != nil && onSlide(tree, c.ID) {
			out = append(out, c.ID)
		}
	}
	return out
}

func onSlide(tree *ot.Tree, id string) bool {
	n := tree.Node(id)
	if n.Type == "chart" {
		return true
	}
	for n != nil && n.Type != "slide" {
		n = tree.Node(n.Parent)
	}
	return n != nil
}

func chartOf(t *testing.T, tree *ot.Tree, id string) *chart.Chart {
	t.Helper()
	var c chart.Chart
	if err := json.Unmarshal(tree.Node(id).Attrs[chartpart.ChartKey], &c); err != nil {
		t.Fatal(err)
	}
	return &c
}

func tableOf(t *testing.T, tree *ot.Tree, id string) *xlsx.ChartTable {
	t.Helper()
	raw := tree.Node(id).Attrs[chartpart.DataKey]
	if raw == nil {
		return nil
	}
	var table xlsx.ChartTable
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	return &table
}

// edit checks an edit, applies it and what follows it, as a hub does.
func edit(f file, tree *ot.Tree, id string, attrs ot.Values) error {
	e := ot.Edit{{Op: ot.OpSet, ID: id, Attrs: attrs}}
	if err := f.Check(tree, e, "Test"); err != nil {
		return err
	}
	if err := tree.Apply(e); err != nil {
		return err
	}
	f.Follow(tree, e)
	return nil
}

func setChart(t *testing.T, f file, tree *ot.Tree, id string, c *chart.Chart) {
	t.Helper()
	if err := edit(f, tree, id, ot.Values{chartpart.ChartKey: c.JSON()}); err != nil {
		t.Fatalf("chart refused: %v", err)
	}
}

func setTable(f file, tree *ot.Tree, id string, table *xlsx.ChartTable) error {
	data, err := json.Marshal(table)
	if err != nil {
		return err
	}
	return edit(f, tree, id, ot.Values{chartpart.DataKey: data})
}

// changedParts are the parts two packages do not hold alike.
func changedParts(t *testing.T, a, b []byte) []string {
	t.Helper()
	x, err := opc.Open(a, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	y, err := opc.Open(b, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(x.Names(), y.Names()) {
		t.Fatalf("parts differ: %v and %v", x.Names(), y.Names())
	}
	var out []string
	for _, name := range x.Names() {
		p, _ := x.Read(name)
		q, _ := y.Read(name)
		if !bytes.Equal(p, q) {
			out = append(out, name)
		}
	}
	return out
}

func retype(c *chart.Chart) {
	if c.Plots[0].Kind == "line" {
		c.Plots[0].Kind, c.Plots[0].Dir, c.Plots[0].Grouping = "bar", "col", "clustered"
	} else {
		c.Plots[0].Kind, c.Plots[0].Dir, c.Plots[0].Grouping = "line", "", "standard"
	}
}

// look gives a chart another title, legend and kind.
func look(c *chart.Chart) {
	c.Title = &chart.Title{Text: "Nouveau titre"}
	c.Legend = &chart.Legend{Pos: "t"}
	if chart.Kinds[c.Plots[0].Kind] && len(c.Plots) == 1 && !c.Plots[0].ThreeD && c.Plots[0].Kind != "scatter" && c.Plots[0].Kind != "bubble" {
		retype(c)
	}
}

// grown is a table with a value changed, a row, and a series.
func grown(table *xlsx.ChartTable) *xlsx.ChartTable {
	out := xlsx.ChartTable{}
	data, _ := json.Marshal(table)
	_ = json.Unmarshal(data, &out)
	next := 0
	out.Series[0].Values[0] = 42.5
	for i := range out.Series {
		out.Series[i].Values = append(out.Series[i].Values, float64(i+1)/4)
		next = max(next, out.Series[i].Index+1)
	}
	if out.Categories != nil {
		out.Categories = append(out.Categories, "Nouvelle")
	}
	values := make([]any, len(out.Series[0].Values))
	for i := range values {
		values[i] = float64(i)
	}
	out.Series = append(out.Series, xlsx.ChartColumn{Index: next, Name: "Série neuve", Values: values})
	return &out
}

// asked tells whether a table keeps what was asked of it: the first value
// that is 42.5, whatever the formulas of the workbook make of the others,
// and the series and categories asked for.
func asked(table, requested *xlsx.ChartTable) error {
	if len(table.Series) != len(requested.Series) || len(table.Categories) != len(requested.Categories) {
		return fmt.Errorf("%d series and %d categories, not %d and %d", len(table.Series), len(table.Categories), len(requested.Series), len(requested.Categories))
	}
	if v := table.Series[0].Values[0]; v != 42.5 {
		return fmt.Errorf("the first value is %v, not 42.5", v)
	}
	return nil
}

// matches tells how the series of a chart differ from a table.
func matches(c *chart.Chart, table *xlsx.ChartTable) error {
	series := map[int]chart.Series{}
	for _, p := range c.Plots {
		for _, s := range p.Series {
			series[s.Index] = s
		}
	}
	if len(series) != len(table.Series) {
		return fmt.Errorf("%d series, %d in the table", len(series), len(table.Series))
	}
	for _, column := range table.Series {
		s, ok := series[column.Index]
		if !ok {
			return fmt.Errorf("series %d lost", column.Index)
		}
		if column.Name != "" && (s.Name == nil || len(s.Name.Str) != 1 || s.Name.Str[0] != column.Name) {
			return fmt.Errorf("series %d named %+v, not %q", column.Index, s.Name, column.Name)
		}
		if s.Val.Count != len(column.Values) || len(s.Val.Num) != len(column.Values) {
			return fmt.Errorf("series %d caches %d values, not %d", column.Index, s.Val.Count, len(column.Values))
		}
		for i, v := range column.Values {
			got := s.Val.Num[i]
			if n, number := v.(float64); number && (got == nil || *got != n) || v == nil && got != nil {
				return fmt.Errorf("series %d: value %d is %v, not %v", column.Index, i, got, v)
			}
		}
		if table.Categories != nil {
			if s.Cat == nil || s.Cat.Count != len(table.Categories) {
				return fmt.Errorf("series %d caches categories %+v, not %d", column.Index, s.Cat, len(table.Categories))
			}
		}
	}
	return nil
}

var (
	chartName = regexp.MustCompile(`/charts/[^/]*\.xml$`)
	bookName  = regexp.MustCompile(`/embeddings/[^/]*\.xlsx$`)
)

// refuseData checks that the data and the cached values of a chart are read-only.
func refuseData(t *testing.T, name string, f file, tree *ot.Tree, id string) {
	t.Helper()
	old := chartOf(t, tree, id)
	if err := setTable(f, tree, id, &xlsx.ChartTable{Series: []xlsx.ChartColumn{{Values: []any{1.0}}}}); !errors.Is(err, chartpart.ErrReadOnly) {
		t.Fatalf("%s: data refused with %v", name, err)
	}
	if s := firstSeries(old); s != nil && s.Val != nil && len(s.Val.Num) > 0 {
		v := 42.5
		s.Val.Num[0] = &v
		if err := edit(f, tree, id, ot.Values{chartpart.ChartKey: old.JSON()}); !errors.Is(err, chartpart.ErrReadOnly) {
			t.Fatalf("%s: cached values refused with %v", name, err)
		}
	}
}

// editCharts edits the first chart of a file: its look, then its data,
// or checks they are read-only. It returns the file saved and whether its
// data were edited; ok is false for a file without a chart.
func editCharts(t *testing.T, name string, data []byte) (saved []byte, withData, ok bool) {
	t.Helper()
	f, tree, err := open(name, data)
	if err != nil {
		return nil, false, false
	}
	ids := nodes(tree)
	if len(ids) == 0 {
		return nil, false, false
	}
	id := ids[0]
	c := chartOf(t, tree, id)
	if c.Extended() {
		look(c)
		if err := edit(f, tree, id, ot.Values{chartpart.ChartKey: c.JSON()}); !errors.Is(err, chartpart.ErrReadOnly) {
			t.Fatalf("%s: look refused with %v", name, err)
		}
		refuseData(t, name, f, tree, id)
		if saved, err = f.Save(tree); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return saved, false, true
	}
	look(c)
	if err := edit(f, tree, id, ot.Values{chartpart.ChartKey: c.JSON()}); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if saved, err = f.Save(tree); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if changed := changedParts(t, data, saved); len(changed) != 1 || !chartName.MatchString(changed[0]) {
		t.Fatalf("%s: %v changed", name, changed)
	}
	if got := chartOf(t, reopen(t, name, saved), id); got.Title == nil || got.Title.Text != "Nouveau titre" || got.Legend == nil || got.Legend.Pos != "t" || got.Plots[0].Kind != c.Plots[0].Kind {
		t.Fatalf("%s: the chart reads %s", name, got.JSON())
	}

	table := tableOf(t, tree, id)
	if table == nil {
		refuseData(t, name, f, tree, id)
		return saved, false, true
	}
	want := grown(table)
	if err := setTable(f, tree, id, want); err != nil {
		t.Logf("%s: %v", name, err)
		want = table
		want.Series[0].Values[0] = 42.5
		if err := setTable(f, tree, id, want); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if saved, err = f.Save(tree); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	requested := want
	want = tableOf(t, tree, id)
	if err := asked(want, requested); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	for _, part := range changedParts(t, data, saved) {
		if !chartName.MatchString(part) && !bookName.MatchString(part) {
			t.Fatalf("%s: %s changed", name, part)
		}
	}
	if err := matches(chartOf(t, tree, id), want); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	again := reopen(t, name, saved)
	got := tableOf(t, again, id)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: table %+v, not %+v", name, got, want)
	}
	if err := matches(chartOf(t, again, id), want); err != nil {
		t.Fatalf("%s: read again: %v", name, err)
	}
	if err := asked(got, requested); err != nil {
		t.Fatalf("%s: read again: %v", name, err)
	}
	if v := chartOf(t, again, id).Plots[0].Series[0].Val.Num[0]; v == nil || *v != 42.5 {
		t.Fatalf("%s: the first value of the chart reads %v, not 42.5", name, v)
	}
	return saved, true, true
}

func TestChartEdits(t *testing.T) {
	for name, data := range map[string]bool{
		"python-pptx/cht-chart-type.pptx":          true,
		"libreoffice/chart.docx":                   true,
		"python-docx/shp-inline-shape-access.docx": true,
		"libreoffice/tdf115557.docx":               false,
	} {
		_, withData, ok := editCharts(t, name, read(t, name))
		if !ok || withData != data {
			t.Errorf("%s: chart %v, data edited %v", name, ok, withData)
		}
	}
}

func TestReadOnlyError(t *testing.T) {
	name := "libreoffice/tdf115557.docx"
	f, tree, err := open(name, read(t, name))
	if err != nil {
		t.Fatal(err)
	}
	err = setTable(f, tree, "chart-1", &xlsx.ChartTable{Series: []xlsx.ChartColumn{{Values: []any{1.0}}}})
	if err == nil || !strings.Contains(err.Error(), "no embedded workbook") || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("got %v", err)
	}
}

func TestCorpusCharts(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	out := os.Getenv("LOFFICE_EDITED")
	paths, _ := filepath.Glob("../../corpus/files/*/*")
	var looks, datas int
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".pptx", ".docx":
		default:
			continue
		}
		data, _ := os.ReadFile(path)
		name, _ := filepath.Rel("../../corpus/files", path)
		saved, withData, ok := editCharts(t, name, data)
		if !ok {
			continue
		}
		looks++
		if withData {
			datas++
		}
		if out != "" {
			dst := filepath.Join(out, name)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, saved, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("%d charts edited, %d with data, %d read-only", looks, datas, looks-datas)
}

func firstSeries(c *chart.Chart) *chart.Series {
	for i := range c.Plots {
		if len(c.Plots[i].Series) > 0 {
			return &c.Plots[i].Series[0]
		}
	}
	return nil
}
