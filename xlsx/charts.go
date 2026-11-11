package xlsx

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const chartsKey = "charts"

// A Placed chart is a chart the drawing of a sheet holds, and where its
// anchor puts it. The file keeps it as XML: the sheet's "charts" show it
// and move with it.
type Placed struct {
	// ID is the part of the chart, "" for one inserted.
	ID string `json:"id,omitempty"`
	// Source are the cells a chart inserted is drawn from: the server
	// gives it its series and clears it.
	Source string `json:"source,omitempty"`
	// From is the cell of its top-left corner, To that of its
	// bottom-right one when it stretches with the cells.
	From *Corner `json:"from,omitempty"`
	To   *Corner `json:"to,omitempty"`
	// X and Y place a chart anchored to the sheet rather than a cell, W
	// and H size one not anchored by its bottom-right corner, in EMU.
	X     int64        `json:"x,omitempty"`
	Y     int64        `json:"y,omitempty"`
	W     int64        `json:"w,omitempty"`
	H     int64        `json:"h,omitempty"`
	Chart *chart.Chart `json:"chart"`
}

// Corner is a cell, counted from 0, and an offset into it in EMU.
type Corner struct {
	Col int   `json:"col"`
	Row int   `json:"row"`
	DX  int64 `json:"dx,omitempty"`
	DY  int64 `json:"dy,omitempty"`
}

// drawing is the drawing part a worksheet or chart sheet points to.
func (d *Document) drawing(part string, root *xmldom.Element) (string, *xmldom.Element) {
	ref := root.Child(mainNS, "drawing")
	if ref == nil {
		return "", nil
	}
	name := d.relTarget(part, relID(ref, root))
	data, err := d.pkg.Read(name)
	if err != nil {
		return "", nil
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return "", nil
	}
	return name, doc.Root
}

// relTarget is the part a relationship of source points to, "" when it
// points outside the package or nowhere.
func (d *Document) relTarget(source, id string) string {
	rels, err := d.pkg.Relationships(source)
	if err != nil || id == "" {
		return ""
	}
	for _, r := range rels {
		if r.ID == id && !r.External {
			if name, err := opc.Resolve(source, r.Target); err == nil {
				return name
			}
		}
	}
	return ""
}

// sheetCharts reads the charts the drawing of a sheet holds.
func (d *Document) sheetCharts(part string, root *xmldom.Element) []Placed {
	name, drawing := d.drawing(part, root)
	if drawing == nil {
		return nil
	}
	var out []Placed
	seen := map[string]bool{}
	for _, a := range drawingAnchors(drawing, false) {
		ref := chart.FindRef(a)
		if ref == nil {
			continue
		}
		part := d.relTarget(name, relID(ref, drawing))
		if seen[part] {
			continue
		}
		seen[part] = true
		data, err := d.pkg.Read(part)
		if err != nil {
			continue
		}
		c, err := chart.Read(data)
		if err != nil {
			continue
		}
		p := Placed{ID: part, Chart: c, From: corner(a.Child(drawingNS, "from")), To: corner(a.Child(drawingNS, "to"))}
		if pos := a.Child(drawingNS, "pos"); pos != nil {
			p.X, p.Y = emu(pos.Get("x")), emu(pos.Get("y"))
		}
		if ext := a.Child(drawingNS, "ext"); ext != nil {
			p.W, p.H = emu(ext.Get("cx")), emu(ext.Get("cy"))
		}
		out = append(out, p)
	}
	return out
}

const mcNS = "http://schemas.openxmlformats.org/markup-compatibility/2006"

// drawingAnchors are the anchors of a drawing, with those of the alternate
// content it offers: Excel wraps in it the anchors of its newest charts,
// before a fallback. Only the choices are taken unless all is set.
func drawingAnchors(root *xmldom.Element, all bool) []*xmldom.Element {
	var out []*xmldom.Element
	for _, e := range root.Elements() {
		switch {
		case e.Space == drawingNS:
			out = append(out, e)
		case e.Space == mcNS && e.Local == "AlternateContent":
			for _, branch := range e.Elements() {
				if branch.Space == mcNS && (branch.Local == "Choice" || all && branch.Local == "Fallback") {
					for _, a := range branch.Elements() {
						if a.Space == drawingNS {
							out = append(out, a)
						}
					}
				}
			}
		}
	}
	return out
}

// alternateOf is the alternate content of a drawing whose choice holds
// the anchor a, nil when a stands alone.
func alternateOf(root, a *xmldom.Element) *xmldom.Element {
	for _, e := range root.Elements() {
		if e.Space != mcNS || e.Local != "AlternateContent" {
			continue
		}
		if choice := e.Child(mcNS, "Choice"); choice != nil && slices.Contains(choice.Elements(), a) {
			return e
		}
	}
	return nil
}

// sheetChart is the chart a chart sheet shows, nil when it has none.
func (d *Document) sheetChart(part string) *chart.Chart {
	data, err := d.pkg.Read(part)
	if err != nil {
		return nil
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil
	}
	for _, p := range d.sheetCharts(part, doc.Root) {
		return p.Chart
	}
	return nil
}

func corner(e *xmldom.Element) *Corner {
	if e == nil {
		return nil
	}
	value := func(local string) string {
		if c := e.Child(drawingNS, local); c != nil {
			return strings.TrimSpace(c.Text())
		}
		return ""
	}
	return &Corner{Col: max(atoi(value("col")), 0), Row: max(atoi(value("row")), 0), DX: emu(value("colOff")), DY: emu(value("rowOff"))}
}

func emu(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}

// find is the first descendant of e with that name.
func find(e *xmldom.Element, space, local string) *xmldom.Element {
	for _, c := range e.Elements() {
		if c.Space == space && c.Local == local {
			return c
		}
		if f := find(c, space, local); f != nil {
			return f
		}
	}
	return nil
}

// shiftIndex is a row or column counted from 0, as drawings count them,
// once n are inserted at at, counted from 1, or -n removed there: into
// the first left when its own is removed.
func shiftIndex(i, at, n int) int {
	switch {
	case i+1 < at:
	case n < 0 && i+1 < at-n:
		i = at - 1
	default:
		i += n
	}
	return max(i, 0)
}

// shiftCharts moves the anchors of charts when rows or columns are
// inserted or removed.
func shiftCharts(list []Placed, rows bool, at, n int) {
	for _, p := range list {
		for _, c := range []*Corner{p.From, p.To} {
			if c == nil {
				continue
			}
			if rows {
				c.Row = shiftIndex(c.Row, at, n)
			} else {
				c.Col = shiftIndex(c.Col, at, n)
			}
		}
	}
}

// chartRefs calls rewrite with each reference of a chart to cells, and
// keeps what it gives.
func chartRefs(c *chart.Chart, rewrite func(string) string) bool {
	changed := false
	for i := range c.Plots {
		for j := range c.Plots[i].Series {
			s := &c.Plots[i].Series[j]
			for _, d := range []*chart.Data{s.Name, s.Cat, s.Val, s.Size} {
				if d == nil || d.Ref == "" {
					continue
				}
				if g := rewrite(d.Ref); g != d.Ref {
					d.Ref = g
					changed = true
				}
			}
		}
	}
	return changed
}

// The bounds of what an edited chart holds.
const (
	maxCharts      = 256
	maxChartSeries = 255
	maxChartTitle  = 255
	// maxChartRead bounds the cells an edit makes the charts of a sheet
	// read anew.
	maxChartRead = 1 << 20
)

// checkCharts tells whether charts are those the sheet id had, moved,
// resized, edited or removed, and charts inserted from cells.
func (d *Document) checkCharts(tree *ot.Tree, id string, raw json.RawMessage) bool {
	var list []Placed
	if json.Unmarshal(raw, &list) != nil || len(list) > maxCharts {
		return false
	}
	had := map[string]bool{}
	for _, p := range placedCharts(d.loaded.Node(id)) {
		had[p.ID] = true
	}
	current := map[string]*chart.Chart{}
	for _, p := range placedCharts(tree.Node(id)) {
		current[p.ID] = p.Chart
	}
	seen := map[string]bool{}
	read := maxChartRead
	for _, p := range list {
		if p.ID != "" && (!had[p.ID] || seen[p.ID]) || p.ID == "" && (p.From == nil || p.To == nil) || p.Chart == nil ||
			p.Source != "" && p.ID != "" || !checkAnchor(p) || !checkChart(p.Chart, current[p.ID], &read) {
			return false
		}
		if p.Source != "" {
			n := chartRefCells(p.Source, false)
			read -= n
			if n == 0 || n > maxChartCells {
				return false
			}
		}
		seen[p.ID] = true
	}
	return read >= 0
}

func checkAnchor(p Placed) bool {
	for _, c := range []*Corner{p.From, p.To} {
		if c != nil && (c.Col < 0 || c.Col >= formula.MaxCols || c.Row < 0 || c.Row >= formula.MaxRows || c.DX < 0 || c.DY < 0 || c.DX > maxEMU || c.DY > maxEMU) {
			return false
		}
	}
	return p.X >= 0 && p.Y >= 0 && p.W >= 0 && p.H >= 0 && max(p.X, p.Y, p.W, p.H) <= maxEMU
}

// maxEMU bounds the offsets and sizes of a chart: a hundred meters.
const maxEMU = 36000000 * 100

var (
	chartDirs      = []string{"", "bar", "col"}
	chartGroupings = []string{"", "clustered", "stacked", "percentStacked", "standard"}
	axisKinds      = []string{"cat", "val", "date", "ser"}
	axisPositions  = []string{"", "l", "r", "t", "b"}
	axisCrosses    = []string{"", "autoZero", "min", "max"}
	axisBetweens   = []string{"", "between", "midCat"}
	legendPlaces   = []string{"", "r", "l", "t", "b", "tr"}
)

// checkChart tells whether c is a chart of the kinds charts are made in,
// or old, the chart as the sheet holds it, with only what can be edited
// changed: what c keeps of old is not bounded again. What it reads anew
// comes off read.
func checkChart(c, old *chart.Chart, read *int) bool {
	if c.Extended() || old != nil && old.Extended() {
		return old != nil && bytes.Equal(mustJSON(c), mustJSON(old))
	}
	if old == nil {
		old = &chart.Chart{}
	}
	if len(c.Plots) == 0 || len(c.Plots) > max(4, len(old.Plots)) || !checkChartTitle(c.Title, old.Title) ||
		c.Legend != nil && !slices.Contains(legendPlaces, c.Legend.Pos) {
		return false
	}
	axes := map[int64]chart.Axis{}
	for _, a := range old.Axes {
		axes[a.ID] = a
	}
	for _, a := range c.Axes {
		if !checkChartTitle(a.Title, axes[a.ID].Title) || !slices.Contains(axisKinds, a.Kind) || a.Kind == "ser" && axes[a.ID].Kind != "ser" || !slices.Contains(axisPositions, a.Pos) ||
			!slices.Contains(axisCrosses, a.Crosses) || !slices.Contains(axisBetweens, a.Between) {
			return false
		}
	}
	kept := map[string]bool{}
	for _, p := range old.Plots {
		for _, s := range p.Series {
			for _, d := range []*chart.Data{s.Name, s.Cat, s.Val, s.Size} {
				if d != nil {
					kept[string(mustJSON(d))] = true
				}
			}
		}
	}
	for i, p := range c.Plots {
		var was chart.Plot
		if i < len(old.Plots) {
			was = old.Plots[i]
		}
		if !chart.Kinds[p.Kind] && was.Kind != p.Kind || len(p.Series) > max(maxChartSeries, len(was.Series)) ||
			!slices.Contains(chartDirs, p.Dir) || !slices.Contains(chartGroupings, p.Grouping) {
			return false
		}
		for _, s := range p.Series {
			if s.Index < 0 {
				return false
			}
			for _, d := range []*chart.Data{s.Name, s.Cat, s.Val, s.Size} {
				if d != nil && !kept[string(mustJSON(d))] && !checkChartData(d, read) {
					return false
				}
			}
		}
	}
	return true
}

// CheckChart tells whether c is old with only what can be edited changed.
func CheckChart(c, old *chart.Chart) bool {
	read := maxChartRead
	return checkChart(c, old, &read) && read >= 0
}

func checkChartData(d *chart.Data, read *int) bool {
	if d.Ref != "" {
		n := chartRefCells(d.Ref, true)
		*read -= n
		if n == 0 {
			return false
		}
	}
	return d.Count >= 0 && d.Count <= maxChartCells && len(d.Num) <= maxChartCells && len(d.Str) <= maxChartCells &&
		(len(d.Levels) == 0 || d.Ref != "") && chartText(d.Format) && !slices.ContainsFunc(d.Str, func(s string) bool { return !chartText(s) })
}

func checkChartTitle(t, old *chart.Title) bool {
	return t == nil || old != nil && t.Text == old.Text || len(utf16.Encode([]rune(t.Text))) <= maxChartTitle && chartText(t.Text)
}

// chartText tells whether s holds no character XML cannot.
func chartText(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' && r != '\r' }) < 0
}

// chartRefCells is the cells of ref, an area a chart reads, a row or
// column when flat is set; 0 for anything else.
func chartRefCells(ref string, flat bool) int {
	r, ok := chartRef(ref, "")
	rows, cols := r.Area.R2-r.Area.R1+1, r.Area.C2-r.Area.C1+1
	if !ok || flat && rows > 1 && cols > 1 {
		return 0
	}
	return rows * cols
}
