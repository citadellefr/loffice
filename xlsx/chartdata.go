package xlsx

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// maxChartCells bounds the cells a series of a chart reads.
const maxChartCells = 1 << 16

// chartRef reads a reference of a chart: a single area, of the sheet
// named own when it names none.
func chartRef(ref, own string) (formula.Ref, bool) {
	tokens, err := formula.Tokens(strings.TrimPrefix(strings.TrimSpace(ref), "="))
	if err != nil || len(tokens) != 1 || tokens[0].Kind != formula.Reference {
		return formula.Ref{}, false
	}
	r := *tokens[0].Ref
	if r.Book != "" || r.Invalid || r.LastSheet != "" || r.Area.Rows || r.Area.Cols {
		return formula.Ref{}, false
	}
	if r.Sheet == "" {
		r.Sheet = own
	}
	return r, true
}

// absolute is a reference with its sheet, every row and column fixed.
func absolute(r formula.Ref) string {
	r.Area.AbsR1, r.Area.AbsC1, r.Area.AbsR2, r.Area.AbsC2 = true, true, true, true
	r.Area.Cell = r.Area.R1 == r.Area.R2 && r.Area.C1 == r.Area.C2
	return r.String()
}

// sheetNamed is the sheet of tree with that name.
func sheetNamed(tree *ot.Tree, name string) *ot.Node {
	for _, n := range tree.Children("book") {
		if n.Type == "sheet" && strings.EqualFold(str(n, "name"), name) {
			return n
		}
	}
	return nil
}

// chartCells reads the cells of charts in a tree.
type chartCells struct {
	tree     *ot.Tree
	date1904 bool
	formats  map[string]*formula.Format
}

func newChartCells(tree *ot.Tree) *chartCells {
	c := &chartCells{tree: tree, formats: map[string]*formula.Format{}}
	if book := tree.Node("book"); book != nil {
		_ = json.Unmarshal(book.Attrs["date1904"], &c.date1904)
	}
	return c
}

// format is the number format of a cell style.
func (c *chartCells) format(xf string) (string, *formula.Format) {
	var st Style
	if n := c.tree.Node(xf); n != nil {
		_ = json.Unmarshal(n.Attrs["style"], &st)
	}
	code := either(st.Format, "General")
	f := c.formats[code]
	if f == nil {
		f = formula.ParseFormat(code)
		c.formats[code] = f
	}
	return code, f
}

func either(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

// data is what a chart caches of the cells of ref: their numbers, or
// their texts as shown when text is set. It tells whether ref points to a
// row or column of cells of the tree.
func (c *chartCells) data(ref, own string, text bool) (*chart.Data, bool) {
	r, ok := chartRef(ref, own)
	if !ok {
		return nil, false
	}
	n := sheetNamed(c.tree, r.Sheet)
	a := r.Area
	if n == nil || a.R1 != a.R2 && a.C1 != a.C2 || (a.R2-a.R1+1)*(a.C2-a.C1+1) > maxChartCells {
		return nil, false
	}
	d := &chart.Data{Ref: ref}
	for row := a.R1; row <= a.R2; row++ {
		for col := a.C1; col <= a.C2; col++ {
			var raw json.RawMessage
			if n.Grid != nil {
				raw = n.Grid.Cell(row, col)
			}
			v := valueOf(raw)
			var fields struct {
				S string `json:"s"`
			}
			_ = json.Unmarshal(raw, &fields)
			code, f := c.format(fields.S)
			if d.Count == 0 && !text {
				d.Format = code
			}
			d.Count++
			if text {
				d.Str = append(d.Str, c.text(v, f))
				continue
			}
			if v.Type == formula.TypeNumber {
				d.Num = append(d.Num, &v.Num)
			} else {
				d.Num = append(d.Num, nil)
			}
		}
	}
	return d, true
}

// holdsText tells whether a cell of ref holds what is no number.
func (c *chartCells) holdsText(ref, own string) bool {
	numbers, ok := c.data(ref, own, false)
	if !ok {
		return false
	}
	texts, _ := c.data(ref, own, true)
	for i, n := range numbers.Num {
		if n == nil && texts.Str[i] != "" {
			return true
		}
	}
	return false
}

func (c *chartCells) text(v formula.Value, f *formula.Format) string {
	switch v.Type {
	case formula.TypeNumber:
		return formula.English.Format(v.Num, f, c.date1904)
	case formula.TypeBool:
		if v.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	}
	return v.Str
}

func sameData(a, b *chart.Data) bool {
	if a.Count != b.Count || len(a.Num) != len(b.Num) || strings.Join(a.Str, "\x00") != strings.Join(b.Str, "\x00") || len(a.Str) != len(b.Str) {
		return false
	}
	for i := range a.Num {
		if (a.Num[i] == nil) != (b.Num[i] == nil) || a.Num[i] != nil && *a.Num[i] != *b.Num[i] {
			return false
		}
	}
	return true
}

// refresh gives the series of want, a chart of the sheet own, the values
// of the cells they point to, where those differ from what the cells of
// the workbook as read gave to had, the chart as it was: a cache Excel
// wrote is kept while the cells it reads are the same.
func refresh(want, had *chart.Chart, own string, now, then *chartCells) {
	if want.Pivot {
		return
	}
	for i := range want.Plots {
		for j := range want.Plots[i].Series {
			s := &want.Plots[i].Series[j]
			var h *chart.Series
			if had != nil && i < len(had.Plots) {
				if k := slices.IndexFunc(had.Plots[i].Series, func(o chart.Series) bool { return o.Index == s.Index }); k >= 0 {
					h = &had.Plots[i].Series[k]
				}
			}
			field := func(get func(*chart.Series) *chart.Data, numbers bool) {
				d := get(s)
				if d == nil || d.Ref == "" || d.Levels != nil {
					return
				}
				text := !numbers && (d.Num == nil || now.holdsText(d.Ref, own))
				fresh, ok := now.data(d.Ref, own, text)
				if !ok {
					return
				}
				if h != nil {
					if old := get(h); old != nil && old.Ref != "" && old.Levels == nil {
						if was, ok := then.data(old.Ref, own, text); ok && sameData(was, fresh) {
							return
						}
					}
				}
				if d.Format != "" && fresh.Format == "General" {
					fresh.Format = d.Format
				}
				fresh.Ref = d.Ref
				*d = *fresh
			}
			field(func(s *chart.Series) *chart.Data { return s.Name }, false)
			field(func(s *chart.Series) *chart.Data { return s.Cat }, false)
			field(func(s *chart.Series) *chart.Data { return s.Val }, true)
			field(func(s *chart.Series) *chart.Data { return s.Size }, true)
		}
	}
}

// expand gives a chart inserted from source, an area of the sheet own,
// its series as Excel makes them: a series by column when the area has
// more rows than columns, by row otherwise, named after the first row or
// column when it holds texts, with the categories of the first column or
// row when it does.
func expand(c *chart.Chart, source, own string, cells *chartCells) bool {
	r, ok := chartRef(source, own)
	if !ok || len(c.Plots) == 0 {
		return false
	}
	n := sheetNamed(cells.tree, r.Sheet)
	if n == nil {
		return false
	}
	a := r.Area
	value := func(row, col int) formula.Value {
		if n.Grid == nil {
			return formula.Value{}
		}
		return valueOf(n.Grid.Cell(row, col))
	}
	header := func(rows bool, first int, from, to int) bool {
		texts := 0
		for i := from; i <= to; i++ {
			v := value(first, i)
			if !rows {
				v = value(i, first)
			}
			switch v.Type {
			case formula.TypeText:
				texts++
			case formula.TypeBlank:
			default:
				return false
			}
		}
		return texts > 0
	}
	blankCorner := value(a.R1, a.C1).Type == formula.TypeBlank && a.R2 > a.R1 && a.C2 > a.C1
	names := a.R2 > a.R1 && (blankCorner || header(true, a.R1, a.C1+1, a.C2) || a.C1 == a.C2 && value(a.R1, a.C1).Type == formula.TypeText)
	cats := a.C2 > a.C1 && (blankCorner || header(false, a.C1, a.R1+1, a.R2) || a.R1 == a.R2 && value(a.R1, a.C1).Type == formula.TypeText)
	top, left := a.R1, a.C1
	if names {
		top++
	}
	if cats {
		left++
	}
	if top > a.R2 || left > a.C2 {
		return false
	}
	byCol := a.R2-top >= a.C2-left
	area := func(r1, c1, r2, c2 int) string {
		return absolute(formula.Ref{Sheet: r.Sheet, Area: formula.Area{R1: r1, C1: c1, R2: r2, C2: c2}})
	}
	var series []chart.Series
	var cat string
	count := a.C2 - left + 1
	if !byCol {
		count = a.R2 - top + 1
	}
	if count > maxChartSeries {
		return false
	}
	for i := range count {
		var s chart.Series
		var name, val string
		if byCol {
			val = area(top, left+i, a.R2, left+i)
			if names {
				name = area(a.R1, left+i, a.R1, left+i)
			}
			if cats {
				cat = area(top, a.C1, a.R2, a.C1)
			}
		} else {
			val = area(top+i, left, top+i, a.C2)
			if names {
				name = area(top+i, a.C1, top+i, a.C1)
			}
			if cats {
				cat = area(a.R1, left, a.R1, a.C2)
			}
		}
		if c.Plots[0].Kind == "scatter" && !cats && count > 1 {
			if i == 0 {
				cat = val
				continue
			}
		}
		s.Index, s.Order = len(series), len(series)
		if name != "" {
			s.Name, _ = cells.data(name, own, true)
		}
		if cat != "" {
			s.Cat = categories(cat, own, cells)
		}
		s.Val, _ = cells.data(val, own, false)
		series = append(series, s)
	}
	c.Plots = c.Plots[:1]
	c.Plots[0].Series = series
	return true
}

// categories are those of ref: numbers when its cells all hold numbers,
// texts otherwise.
func categories(ref, own string, cells *chartCells) *chart.Data {
	if d, ok := cells.data(ref, own, false); ok {
		numbers := 0
		for _, v := range d.Num {
			if v != nil {
				numbers++
			}
		}
		if numbers == d.Count {
			return d
		}
	}
	d, _ := cells.data(ref, own, true)
	return d
}

// qualify gives the references of a chart of the sheet own its name, and
// tells whether one changed.
func qualify(c *chart.Chart, own string) bool {
	return chartRefs(c, func(ref string) string {
		if r, ok := chartRef(ref, own); ok && !strings.Contains(ref, "!") {
			return r.String()
		}
		return ref
	})
}

// placeCharts gives the charts inserted into a sheet their series, from
// the cells they are drawn from, and every chart of the sheet the shape
// it is written in.
func (f *follow) placeCharts(id string) {
	n := f.c.tree.Node(id)
	var list []Placed
	if f.attr(id, chartsKey, &list); n == nil || len(list) == 0 {
		return
	}
	before := mustJSON(list)
	own := str(n, "name")
	cells := newChartCells(f.c.tree)
	for i := range list {
		p := &list[i]
		if p.Source != "" {
			expand(p.Chart, p.Source, own, cells)
			p.Source = ""
		}
		qualify(p.Chart, own)
		chart.Normalize(p.Chart)
		if p.ID == "" {
			if c, err := chart.Read(chart.New(p.Chart)); err == nil {
				p.Chart = c
			}
		}
	}
	if !bytes.Equal(before, mustJSON(list)) {
		setList(f, id, chartsKey, list)
	}
}
