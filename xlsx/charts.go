package xlsx

import (
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

const chartsKey = "charts"

// A Placed chart is a chart the drawing of a sheet holds, and where its
// anchor puts it. The file keeps it as XML: the sheet's "charts" show it
// and move with it.
type Placed struct {
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
	for _, a := range drawing.Elements() {
		if a.Space != drawingNS {
			continue
		}
		ref := find(a, chart.NS, "chart")
		if ref == nil {
			continue
		}
		data, err := d.pkg.Read(d.relTarget(name, relID(ref, drawing)))
		if err != nil {
			continue
		}
		c, err := chart.Read(data)
		if err != nil {
			continue
		}
		p := Placed{Chart: c, From: corner(a.Child(drawingNS, "from")), To: corner(a.Child(drawingNS, "to"))}
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
