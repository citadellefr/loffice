package chart

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// CXNS is the namespace of the charts Excel 2016 added.
const CXNS = "http://schemas.microsoft.com/office/drawing/2014/chartex"

// Extended are the kinds of plot of those charts: "waterfall" "histogram"
// "pareto" "boxWhisker" "funnel" "treemap" "sunburst", and "regionMap"
// which a client cannot draw. They are read, not edited.
var Extended = map[string]bool{"waterfall": true, "histogram": true, "pareto": true, "boxWhisker": true, "funnel": true, "treemap": true, "sunburst": true, "regionMap": true}

// Options are what a series of a chart of Excel 2016 draws by.
type Options struct {
	// Subtotals are the points of a waterfall that are totals.
	Subtotals []int `json:"subtotals,omitempty"`
	// NoConnectors tells that a waterfall has no lines between its bars.
	NoConnectors bool `json:"noConnectors,omitempty"`
	// Bins are those of a histogram, Aggregate tells that it counts the
	// values of each category instead.
	Bins      *Bins `json:"bins,omitempty"`
	Aggregate bool  `json:"aggregate,omitempty"`
	// Line is the line of a pareto, drawn from the cumulated values of
	// the bars.
	Line bool `json:"line,omitempty"`
	// Box is how a box and whisker draws its series.
	Box *Box `json:"box,omitempty"`
	// Parents is how a treemap labels its parents: "banner"
	// "overlapping" "none".
	Parents string `json:"parents,omitempty"`
}

// Bins are the intervals of a histogram, automatic where they are absent.
type Bins struct {
	Size  *float64 `json:"size,omitempty"`
	Count *int     `json:"count,omitempty"`
	// Closed is the end of an interval that holds its value: "r" or "l".
	Closed string `json:"closed,omitempty"`
	// Under and Over are the bounds of the first and last interval, which
	// hold everything beyond.
	Under *float64 `json:"under,omitempty"`
	Over  *float64 `json:"over,omitempty"`
}

// Box is the look of a box and whisker.
type Box struct {
	MeanLine   bool `json:"meanLine,omitempty"`
	MeanMarker bool `json:"meanMarker,omitempty"`
	// Inner and Outliers tell which points are drawn: those inside the
	// whiskers, and those beyond them.
	Inner    bool `json:"inner,omitempty"`
	Outliers bool `json:"outliers,omitempty"`
	// Quartile is how quartiles are found: "exclusive" or "inclusive".
	Quartile string `json:"quartile,omitempty"`
}

// exData are the values a chartData holds for one series.
type exData struct {
	cat, val *Data
}

func readExtended(space *xmldom.Element) (*Chart, error) {
	c := child(space, "chart")
	if c == nil {
		return nil, ErrNotChart
	}
	out := &Chart{Space: shape(space), Text: textProps(child(space, "txPr")), Plots: []Plot{}}
	if plot := child(c, "plotArea"); plot != nil {
		out.Area = shape(plot)
		var axes []*xmldom.Element
		for _, e := range plot.Elements() {
			if e.Space == CXNS && e.Local == "axis" {
				axes = append(axes, e)
				out.Axes = append(out.Axes, readExAxis(e))
			}
		}
		out.Plots = readExPlots(child(plot, "plotAreaRegion"), readExData(child(space, "chartData")), out.Axes, catGap(axes))
	}
	if t := child(c, "title"); t != nil {
		out.Title = readTitle(t)
	}
	if l := child(c, "legend"); l != nil {
		out.Legend = readLegend(l)
	}
	return out, nil
}

// readExData reads the data of a chartData by id.
func readExData(e *xmldom.Element) map[string]*exData {
	out := map[string]*exData{}
	if e == nil {
		return out
	}
	for _, d := range e.Elements() {
		if d.Space != CXNS || d.Local != "data" {
			continue
		}
		set := &exData{}
		for _, dim := range d.Elements() {
			if dim.Space != CXNS {
				continue
			}
			switch kind := dim.Get("type"); {
			case dim.Local == "strDim" && kind == "cat":
				set.cat = readDim(dim)
			case dim.Local == "numDim" && (kind == "val" || kind == "size") && set.val == nil:
				set.val = readDim(dim)
			}
		}
		out[d.Get("id")] = set
	}
	return out
}

// readDim reads a strDim or numDim: the cells it comes from, and its
// levels, the innermost first.
func readDim(e *xmldom.Element) *Data {
	numbers := e.Local == "numDim"
	d := &Data{Ref: strings.TrimSpace(text(child(e, "f")))}
	var levels [][]string
	for _, lvl := range e.Elements() {
		if lvl.Space != CXNS || lvl.Local != "lvl" {
			continue
		}
		count := min(max(atoi(lvl.Get("ptCount")), 0), maxPoints)
		if len(levels) == 0 && d.Count == 0 {
			d.Count, d.Format = count, lvl.Get("formatCode")
		}
		var nums []*float64
		strs := make([]string, count)
		if numbers {
			nums = make([]*float64, count)
		}
		for _, pt := range lvl.Elements() {
			i, err := strconv.Atoi(pt.Get("idx"))
			if pt.Space != CXNS || pt.Local != "pt" || err != nil || i < 0 || i >= count {
				continue
			}
			if numbers {
				nums[i] = number(pt.Text())
			} else {
				strs[i] = pt.Text()
			}
		}
		if numbers {
			d.Num = nums
			break
		}
		levels = append(levels, strs)
	}
	if len(levels) > 0 {
		d.Str = levels[0]
	}
	if len(levels) > 1 {
		d.Levels = levels
	}
	return d
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

func readExPlots(region *xmldom.Element, data map[string]*exData, axes []Axis, gap *int) []Plot {
	plots := []Plot{}
	if region == nil {
		return plots
	}
	var ids []int64
	for _, a := range axes {
		ids = append(ids, a.ID)
	}
	n := 0
	for _, e := range region.Elements() {
		if e.Space != CXNS || e.Local != "series" {
			continue
		}
		s, kind := readExSeries(e, data[val(child(e, "dataId"))])
		i := slices.IndexFunc(plots, func(p Plot) bool { return p.Kind == kind })
		if i < 0 {
			plots = append(plots, Plot{Kind: kind, Axes: ids, Series: []Series{}})
			if kind != "treemap" && kind != "sunburst" && kind != "regionMap" && kind != "boxWhisker" {
				plots[len(plots)-1].Gap = gap
			}
			i = len(plots) - 1
		}
		s.Index, s.Order = n, len(plots[i].Series)
		n++
		plots[i].Series = append(plots[i].Series, s)
	}
	for i := range plots {
		if slices.ContainsFunc(plots[i].Series, func(s Series) bool { return s.Options != nil && s.Options.Line }) {
			plots[i].Kind = "pareto"
		}
	}
	return plots
}

// readExSeries reads a series, and the kind of plot it belongs to.
func readExSeries(e *xmldom.Element, d *exData) (Series, string) {
	layout := e.Get("layoutId")
	s := Series{Shape: shape(e)}
	if d != nil {
		s.Cat, s.Val = d.cat, d.val
	}
	if data := child(child(e, "tx"), "txData"); data != nil {
		s.Name = &Data{Ref: strings.TrimSpace(text(child(data, "f"))), Count: 1, Str: []string{text(child(data, "v"))}}
	}
	for _, c := range e.Elements() {
		if c.Space != CXNS {
			continue
		}
		switch c.Local {
		case "dataPt":
			s.Points = append(s.Points, Point{Idx: atoi(c.Get("idx")), Shape: shape(c)})
		case "dataLabels":
			s.Labels = readExLabels(c)
		}
	}
	s.Options = readExOptions(child(e, "layoutPr"), layout)
	kind := layout
	switch layout {
	case "clusteredColumn":
		kind = "histogram"
	case "paretoLine":
		kind, s.Options.Line = "histogram", true
	}
	return s, kind
}

func readExOptions(layoutPr *xmldom.Element, layout string) *Options {
	o := &Options{}
	if layout == "boxWhisker" {
		o.Box = &Box{MeanMarker: true, Outliers: true}
	}
	if layoutPr == nil {
		return o
	}
	for _, c := range layoutPr.Elements() {
		if c.Space != CXNS {
			continue
		}
		switch c.Local {
		case "subtotals":
			for _, i := range c.Elements() {
				o.Subtotals = append(o.Subtotals, atoi(i.Get("val")))
			}
		case "visibility":
			if o.Box == nil {
				o.NoConnectors = !truthy(c.Get("connectorLines"), true)
				continue
			}
			for name, flag := range map[string]*bool{"meanLine": &o.Box.MeanLine, "meanMarker": &o.Box.MeanMarker, "nonoutliers": &o.Box.Inner, "outliers": &o.Box.Outliers} {
				if v, ok := c.Attr(name); ok {
					*flag = truthy(v, false)
				}
			}
		case "binning":
			o.Bins = &Bins{Closed: c.Get("intervalClosed"), Size: number(val(child(c, "binSize"))), Under: number(val(child(c, "underflow"))), Over: number(val(child(c, "overflow")))}
			if n, err := strconv.Atoi(val(child(c, "binCount"))); err == nil {
				o.Bins.Count = &n
			}
		case "aggregation":
			o.Aggregate = true
		case "statistics":
			if o.Box != nil {
				o.Box.Quartile = c.Get("quartileMethod")
			}
		case "parentLabelLayout":
			o.Parents = c.Get("val")
		}
	}
	return o
}

func readExLabels(e *xmldom.Element) *Labels {
	l := &Labels{Pos: e.Get("pos"), Text: textProps(child(e, "txPr")), Shape: shape(e)}
	if f := child(e, "numFmt"); f != nil {
		l.Format = f.Get("formatCode")
	}
	if s := child(e, "separator"); s != nil {
		l.Sep = text(s)
	}
	if v := child(e, "visibility"); v != nil {
		l.Val, l.Cat, l.Ser = truthy(v.Get("value"), false), truthy(v.Get("categoryName"), false), truthy(v.Get("seriesName"), false)
	}
	if e.Local == "dataLabel" {
		i := atoi(e.Get("idx"))
		l.Idx = &i
		return l
	}
	for _, c := range e.Elements() {
		if c.Space != CXNS {
			continue
		}
		switch c.Local {
		case "dataLabel":
			l.Points = append(l.Points, *readExLabels(c))
		case "dataLabelHidden":
			i := atoi(c.Get("idx"))
			l.Points = append(l.Points, Labels{Idx: &i, Delete: true})
		}
	}
	return l
}

// readExAxis reads an axis: a category axis when it scales categories, a
// value axis otherwise.
func readExAxis(e *xmldom.Element) Axis {
	a := Axis{ID: int64(atoi(e.Get("id"))), Kind: "val", Pos: "l", Delete: truthy(e.Get("hidden"), false), Labels: "none",
		Shape: shape(e), Text: textProps(child(e, "txPr")), Rot: rotation(child(e, "txPr"))}
	for _, c := range e.Elements() {
		if c.Space != CXNS {
			continue
		}
		switch c.Local {
		case "catScaling":
			a.Kind, a.Pos, a.Between = "cat", "b", "between"
		case "valScaling":
			a.Min, a.Max = number(c.Get("min")), number(c.Get("max"))
			if v := number(c.Get("majorUnit")); v != nil {
				a.Major = *v
			}
			if v := number(c.Get("minorUnit")); v != nil {
				a.Minor = *v
			}
		case "title":
			a.Title = readTitle(c)
		case "majorGridlines":
			a.Grid = gridlines(c)
		case "minorGridlines":
			a.MinorGrid = gridlines(c)
		case "tickLabels":
			a.Labels = "nextTo"
		case "numFmt":
			a.Format, a.Linked = c.Get("formatCode"), truthy(c.Get("sourceLinked"), false)
		case "majorTickMarks":
			a.Tick = c.Get("type")
		case "minorTickMarks":
			a.MinorTick = c.Get("type")
		}
	}
	return a
}

// catGap is the gap between the bars of the category axis, in percent.
func catGap(axes []*xmldom.Element) *int {
	for _, a := range axes {
		if s := child(a, "catScaling"); s != nil {
			if v := number(s.Get("gapWidth")); v != nil {
				n := int(math.Round(*v * 100))
				return &n
			}
		}
	}
	return nil
}

// Extended tells whether c is a chart of Excel 2016, which a client
// reads and draws but does not edit.
func (c *Chart) Extended() bool {
	return slices.ContainsFunc(c.Plots, func(p Plot) bool { return Extended[p.Kind] })
}

// FindRef is the c:chart or cx:chart under e that a graphic frame points
// to, nil when there is none.
func FindRef(e *xmldom.Element) *xmldom.Element {
	for _, c := range e.Elements() {
		if c.Local == "chart" && (c.Space == NS || c.Space == CXNS) {
			return c
		}
		if f := FindRef(c); f != nil {
			return f
		}
	}
	return nil
}
