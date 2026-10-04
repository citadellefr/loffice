// Package chart reads the charts of Office documents, the parts a
// DrawingML graphic frame points to, into what a client needs to draw
// them: plots and their series with the values Office cached, axes,
// title, legend and the fills and lines of each. Charts are drawn, not
// edited: the part stays as it is in the package.
package chart

import (
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

const NS = "http://schemas.openxmlformats.org/drawingml/2006/chart"

// maxPoints bounds the points of a series, whatever count a part claims.
const maxPoints = 1 << 16

var ErrNotChart = errors.New("chart: not a chart part")

// Chart is a chart as a client draws it.
type Chart struct {
	Title  *Title  `json:"title,omitempty"`
	Legend *Legend `json:"legend,omitempty"`
	Plots  []Plot  `json:"plots"`
	Axes   []Axis  `json:"axes,omitempty"`
	Layout *Layout `json:"layout,omitempty"`
	// Space is the fill and line of the whole chart, Area those of the
	// plot area, as drawingml.ShapeProps reads them.
	Space ot.Values       `json:"space,omitempty"`
	Area  ot.Values       `json:"area,omitempty"`
	Text  drawingml.Props `json:"text,omitempty"`
	// Style is the chart style, 1 to 48, which gives series without a
	// fill their color.
	Style   int  `json:"style,omitempty"`
	Rounded bool `json:"rounded,omitempty"`
	// Blanks is how missing values show: "gap", "zero" or "span".
	Blanks string `json:"blanks,omitempty"`
	// Pivot tells a chart of a pivot table, whose values are those of the
	// table: not all the cells its series point to.
	Pivot bool `json:"pivot,omitempty"`
}

// Plot is a group of series drawn the same way on the same axes.
type Plot struct {
	// Kind is "bar" "line" "pie" "doughnut" "area" "scatter" "radar"
	// "bubble" "stock" "surface".
	Kind string `json:"kind"`
	// Dir is "bar" for horizontal bars, "col" for vertical ones.
	Dir string `json:"dir,omitempty"`
	// Grouping is "clustered" "stacked" "percentStacked" "standard".
	Grouping string `json:"grouping,omitempty"`
	// Vary gives each point of a single series its own color.
	Vary bool `json:"vary,omitempty"`
	// Gap and Overlap are in percent of a bar's width.
	Gap     *int `json:"gap,omitempty"`
	Overlap int  `json:"overlap,omitempty"`
	// Hole is the size of a doughnut's hole in percent, Angle where the
	// first slice starts in degrees.
	Hole  int `json:"hole,omitempty"`
	Angle int `json:"angle,omitempty"`
	// Style is a scatter's or radar's: "lineMarker" "smoothMarker"
	// "marker" "line" "filled"…
	Style   string   `json:"style,omitempty"`
	Markers bool     `json:"markers,omitempty"`
	ThreeD  bool     `json:"3d,omitempty"`
	Axes    []int64  `json:"axes,omitempty"`
	Labels  *Labels  `json:"labels,omitempty"`
	Series  []Series `json:"series"`
}

// Series is a row of values, drawn in one color.
type Series struct {
	Index int   `json:"index"`
	Order int   `json:"order"`
	Name  *Data `json:"name,omitempty"`
	// Cat holds the categories, or the x values of a scatter; Val the
	// values, or the y values; Size the sizes of bubbles.
	Cat       *Data     `json:"cat,omitempty"`
	Val       *Data     `json:"val,omitempty"`
	Size      *Data     `json:"size,omitempty"`
	Shape     ot.Values `json:"shape,omitempty"`
	Marker    *Marker   `json:"marker,omitempty"`
	Points    []Point   `json:"points,omitempty"`
	Labels    *Labels   `json:"labels,omitempty"`
	Smooth    bool      `json:"smooth,omitempty"`
	Explosion int       `json:"explosion,omitempty"`
	Invert    bool      `json:"invert,omitempty"`
}

// Data are the values of a series as Office cached them, and the cells
// they come from.
type Data struct {
	Ref    string `json:"ref,omitempty"`
	Format string `json:"format,omitempty"`
	Count  int    `json:"count"`
	// Num holds numbers, null where there is none; Str texts. Levels are
	// the texts of categories on several levels, the innermost first.
	Num    []*float64 `json:"num,omitempty"`
	Str    []string   `json:"str,omitempty"`
	Levels [][]string `json:"levels,omitempty"`
}

// Point is a point of a series drawn otherwise.
type Point struct {
	Idx       int       `json:"idx"`
	Shape     ot.Values `json:"shape,omitempty"`
	Marker    *Marker   `json:"marker,omitempty"`
	Explosion int       `json:"explosion,omitempty"`
	Invert    *bool     `json:"invert,omitempty"`
}

// Marker is the mark at the points of a line or scatter.
type Marker struct {
	// Symbol is "circle" "square" "diamond" "triangle" "x" "star" "dot"
	// "dash" "plus" "picture" "none", or "auto".
	Symbol string    `json:"symbol,omitempty"`
	Size   int       `json:"size,omitempty"`
	Shape  ot.Values `json:"shape,omitempty"`
}

// Labels tell what is written by the points.
type Labels struct {
	Idx     *int `json:"idx,omitempty"`
	Delete  bool `json:"delete,omitempty"`
	Val     bool `json:"val,omitempty"`
	Percent bool `json:"percent,omitempty"`
	Cat     bool `json:"cat,omitempty"`
	Ser     bool `json:"ser,omitempty"`
	Key     bool `json:"key,omitempty"`
	// Pos is "ctr" "inEnd" "inBase" "outEnd" "bestFit" "t" "b" "l" "r".
	Pos    string          `json:"pos,omitempty"`
	Format string          `json:"format,omitempty"`
	Sep    string          `json:"sep,omitempty"`
	Text   drawingml.Props `json:"text,omitempty"`
	Shape  ot.Values       `json:"shape,omitempty"`
	Points []Labels        `json:"points,omitempty"`
}

// Axis is an axis of a plot.
type Axis struct {
	ID int64 `json:"id"`
	// Kind is "cat" "val" "date" "ser".
	Kind   string `json:"kind"`
	Pos    string `json:"pos,omitempty"`
	Delete bool   `json:"delete,omitempty"`
	Cross  int64  `json:"cross,omitempty"`
	// Crosses is where the axis it crosses meets it: "autoZero" "min"
	// "max", or CrossesAt.
	Crosses   string   `json:"crosses,omitempty"`
	CrossesAt *float64 `json:"crossesAt,omitempty"`
	// Between is "between" when categories sit between ticks, "midCat"
	// when on them.
	Between string   `json:"between,omitempty"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Reverse bool     `json:"reverse,omitempty"`
	Log     float64  `json:"log,omitempty"`
	Major   float64  `json:"major,omitempty"`
	Minor   float64  `json:"minor,omitempty"`
	// Grid and MinorGrid are the lines across the plot, absent when
	// there are none.
	Grid      *ot.Values `json:"grid,omitempty"`
	MinorGrid *ot.Values `json:"minorGrid,omitempty"`
	Format    string     `json:"format,omitempty"`
	Linked    bool       `json:"linked,omitempty"`
	// Labels is where tick labels go: "nextTo" "high" "low" "none".
	Labels    string          `json:"labels,omitempty"`
	Tick      string          `json:"tick,omitempty"`
	MinorTick string          `json:"minorTick,omitempty"`
	Skip      int             `json:"skip,omitempty"`
	Title     *Title          `json:"title,omitempty"`
	Shape     ot.Values       `json:"shape,omitempty"`
	Text      drawingml.Props `json:"text,omitempty"`
	Rot       *int            `json:"rot,omitempty"`
}

// Title is the title of a chart or an axis.
type Title struct {
	// Text is empty when Office makes it up: the name of the only series,
	// or "Chart Title".
	Text    string          `json:"text,omitempty"`
	Props   drawingml.Props `json:"props,omitempty"`
	Overlay bool            `json:"overlay,omitempty"`
	Layout  *Layout         `json:"layout,omitempty"`
	Shape   ot.Values       `json:"shape,omitempty"`
	Rot     *int            `json:"rot,omitempty"`
}

// Legend is the legend of a chart.
type Legend struct {
	// Pos is "r" "l" "t" "b" "tr".
	Pos     string          `json:"pos,omitempty"`
	Overlay bool            `json:"overlay,omitempty"`
	Hidden  []int           `json:"hidden,omitempty"`
	Text    drawingml.Props `json:"text,omitempty"`
	Shape   ot.Values       `json:"shape,omitempty"`
	Layout  *Layout         `json:"layout,omitempty"`
}

// Layout places an element by hand, in fractions of the chart.
type Layout struct {
	// Inner tells that the plot area's box leaves out the tick labels.
	Inner bool `json:"inner,omitempty"`
	// XMode and YMode are "edge" when X and Y are from the chart's
	// corner, "factor" when they move the element from where it would
	// be.
	XMode string  `json:"xMode,omitempty"`
	YMode string  `json:"yMode,omitempty"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	W     float64 `json:"w,omitempty"`
	H     float64 `json:"h,omitempty"`
}

// Read reads a chart part.
func Read(data []byte) (*Chart, error) {
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil, err
	}
	return ReadElement(doc.Root)
}

// ReadElement reads the chartSpace element of a chart part.
func ReadElement(space *xmldom.Element) (*Chart, error) {
	if space == nil || space.Space != NS || space.Local != "chartSpace" {
		return nil, ErrNotChart
	}
	c := child(space, "chart")
	if c == nil {
		return nil, ErrNotChart
	}
	out := &Chart{
		Space:   shape(space),
		Text:    textProps(child(space, "txPr")),
		Style:   intVal(child(space, "style")),
		Rounded: boolVal(child(space, "roundedCorners")),
		Blanks:  val(child(c, "dispBlanksAs")),
		Pivot:   child(space, "pivotSource") != nil,
	}
	if e := mcStyle(space); e != nil && out.Style == 0 {
		out.Style = intVal(e)
	}
	plot := child(c, "plotArea")
	if plot != nil {
		out.Area = shape(plot)
		out.Layout = layout(child(plot, "layout"))
		for _, e := range plot.Elements() {
			if e.Space != NS {
				continue
			}
			switch {
			case strings.HasSuffix(e.Local, "Chart"):
				out.Plots = append(out.Plots, readPlot(e))
			case strings.HasSuffix(e.Local, "Ax"):
				out.Axes = append(out.Axes, readAxis(e))
			}
		}
	}
	if out.Plots == nil {
		out.Plots = []Plot{}
	}
	if t := child(c, "title"); t != nil {
		out.Title = readTitle(t)
	}
	if l := child(c, "legend"); l != nil {
		out.Legend = readLegend(l)
	}
	return out, nil
}

// mcStyle is the c:style an AlternateContent of the chart space falls back
// to, beside the c14:style of Office 2010.
func mcStyle(space *xmldom.Element) *xmldom.Element {
	for _, e := range space.Elements() {
		if e.Local != "AlternateContent" {
			continue
		}
		for _, alt := range e.Elements() {
			for _, s := range alt.Elements() {
				if s.Space == NS && s.Local == "style" {
					return s
				}
			}
		}
	}
	return nil
}

func readPlot(e *xmldom.Element) Plot {
	kind := strings.TrimSuffix(e.Local, "Chart")
	p := Plot{Kind: kind}
	if strings.HasSuffix(kind, "3D") {
		p.Kind, p.ThreeD = strings.TrimSuffix(kind, "3D"), true
	}
	if p.Kind == "ofPie" {
		p.Kind = "pie"
	}
	p.Dir = val(child(e, "barDir"))
	p.Grouping = val(child(e, "grouping"))
	p.Vary = boolVal(child(e, "varyColors"))
	if g := child(e, "gapWidth"); g != nil {
		n := intVal(g)
		p.Gap = &n
	}
	p.Overlap = intVal(child(e, "overlap"))
	p.Hole = intVal(child(e, "holeSize"))
	p.Angle = intVal(child(e, "firstSliceAng"))
	p.Style = val(child(e, "scatterStyle"))
	if s := val(child(e, "radarStyle")); s != "" {
		p.Style = s
	}
	p.Markers = boolVal(child(e, "marker"))
	if l := child(e, "dLbls"); l != nil {
		p.Labels = readLabels(l)
	}
	for _, c := range e.Elements() {
		if c.Space != NS {
			continue
		}
		switch c.Local {
		case "axId":
			p.Axes = append(p.Axes, axisID(c))
		case "ser":
			p.Series = append(p.Series, readSeries(c))
		}
	}
	if p.Series == nil {
		p.Series = []Series{}
	}
	return p
}

func readSeries(e *xmldom.Element) Series {
	s := Series{
		Index:     intVal(child(e, "idx")),
		Order:     intVal(child(e, "order")),
		Shape:     shape(e),
		Smooth:    boolVal(child(e, "smooth")),
		Explosion: intVal(child(e, "explosion")),
		Invert:    boolVal(child(e, "invertIfNegative")),
	}
	if tx := child(e, "tx"); tx != nil {
		if v := child(tx, "v"); v != nil {
			s.Name = &Data{Count: 1, Str: []string{text(v)}}
		} else {
			s.Name = readData(tx)
		}
	}
	for _, k := range []struct {
		local string
		into  **Data
	}{{"cat", &s.Cat}, {"xVal", &s.Cat}, {"val", &s.Val}, {"yVal", &s.Val}, {"bubbleSize", &s.Size}} {
		if c := child(e, k.local); c != nil {
			*k.into = readData(c)
		}
	}
	if m := child(e, "marker"); m != nil {
		s.Marker = readMarker(m)
	}
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "dPt" {
			pt := Point{
				Idx:       intVal(child(c, "idx")),
				Shape:     shape(c),
				Explosion: intVal(child(c, "explosion")),
			}
			if m := child(c, "marker"); m != nil {
				pt.Marker = readMarker(m)
			}
			if i := child(c, "invertIfNegative"); i != nil {
				b := boolVal(i)
				pt.Invert = &b
			}
			s.Points = append(s.Points, pt)
		}
	}
	if l := child(e, "dLbls"); l != nil {
		s.Labels = readLabels(l)
	}
	return s
}

func readMarker(e *xmldom.Element) *Marker {
	return &Marker{Symbol: val(child(e, "symbol")), Size: intVal(child(e, "size")), Shape: shape(e)}
}

// readData reads the reference or literal under e: a cat, val, xVal, yVal,
// bubbleSize or tx.
func readData(e *xmldom.Element) *Data {
	for _, c := range e.Elements() {
		if c.Space != NS {
			continue
		}
		d := &Data{}
		if f := child(c, "f"); f != nil {
			d.Ref = strings.TrimSpace(text(f))
		}
		switch c.Local {
		case "numRef":
			points(d, child(c, "numCache"), true)
		case "numLit":
			points(d, c, true)
		case "strRef":
			points(d, child(c, "strCache"), false)
		case "strLit":
			points(d, c, false)
		case "multiLvlStrRef":
			cache := child(c, "multiLvlStrCache")
			if cache == nil {
				return d
			}
			d.Count = min(max(intVal(child(cache, "ptCount")), 0), maxPoints)
			for _, lvl := range cache.Elements() {
				if lvl.Space == NS && lvl.Local == "lvl" {
					level := &Data{Count: d.Count}
					points(level, lvl, false)
					d.Levels = append(d.Levels, level.Str)
				}
			}
			if len(d.Levels) > 0 {
				d.Str = d.Levels[0]
			}
		default:
			continue
		}
		return d
	}
	return nil
}

// points reads the c:pt of a cache or literal into d.
func points(d *Data, e *xmldom.Element, numbers bool) {
	if e == nil {
		return
	}
	if d.Count == 0 {
		d.Count = min(max(intVal(child(e, "ptCount")), 0), maxPoints)
	}
	if f := child(e, "formatCode"); f != nil {
		d.Format = text(f)
	}
	if numbers {
		d.Num = make([]*float64, d.Count)
	} else {
		d.Str = make([]string, d.Count)
	}
	for _, pt := range e.Elements() {
		if pt.Space != NS || pt.Local != "pt" {
			continue
		}
		i, err := strconv.Atoi(pt.Get("idx"))
		if err != nil || i < 0 || i >= d.Count {
			continue
		}
		text := text(child(pt, "v"))
		if !numbers {
			d.Str[i] = text
			continue
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(text), 64); err == nil && !math.IsInf(v, 0) && !math.IsNaN(v) {
			d.Num[i] = &v
		}
	}
}

func readLabels(e *xmldom.Element) *Labels {
	l := &Labels{
		Delete:  boolVal(child(e, "delete")),
		Val:     boolVal(child(e, "showVal")),
		Percent: boolVal(child(e, "showPercent")),
		Cat:     boolVal(child(e, "showCatName")),
		Ser:     boolVal(child(e, "showSerName")),
		Key:     boolVal(child(e, "showLegendKey")),
		Pos:     val(child(e, "dLblPos")),
		Text:    textProps(child(e, "txPr")),
		Shape:   shape(e),
	}
	if f := child(e, "numFmt"); f != nil {
		l.Format = f.Get("formatCode")
	}
	if s := child(e, "separator"); s != nil {
		l.Sep = text(s)
	}
	if e.Local == "dLbl" {
		i := intVal(child(e, "idx"))
		l.Idx = &i
		return l
	}
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "dLbl" {
			l.Points = append(l.Points, *readLabels(c))
		}
	}
	return l
}

func readAxis(e *xmldom.Element) Axis {
	a := Axis{
		ID:        axisID(child(e, "axId")),
		Kind:      strings.TrimSuffix(e.Local, "Ax"),
		Pos:       val(child(e, "axPos")),
		Delete:    boolVal(child(e, "delete")),
		Cross:     axisID(child(e, "crossAx")),
		Crosses:   val(child(e, "crosses")),
		CrossesAt: floatPtr(child(e, "crossesAt")),
		Between:   val(child(e, "crossBetween")),
		Labels:    val(child(e, "tickLblPos")),
		Tick:      val(child(e, "majorTickMark")),
		MinorTick: val(child(e, "minorTickMark")),
		Skip:      intVal(child(e, "tickLblSkip")),
		Shape:     shape(e),
		Text:      textProps(child(e, "txPr")),
		Rot:       rotation(child(e, "txPr")),
	}
	if s := child(e, "scaling"); s != nil {
		a.Min = floatPtr(child(s, "min"))
		a.Max = floatPtr(child(s, "max"))
		a.Reverse = val(child(s, "orientation")) == "maxMin"
		if lb := floatPtr(child(s, "logBase")); lb != nil {
			a.Log = *lb
		}
	}
	if u := floatPtr(child(e, "majorUnit")); u != nil {
		a.Major = *u
	}
	if u := floatPtr(child(e, "minorUnit")); u != nil {
		a.Minor = *u
	}
	a.Grid = gridlines(child(e, "majorGridlines"))
	a.MinorGrid = gridlines(child(e, "minorGridlines"))
	if f := child(e, "numFmt"); f != nil {
		a.Format = f.Get("formatCode")
		a.Linked = truthy(f.Get("sourceLinked"), false)
	}
	if t := child(e, "title"); t != nil {
		a.Title = readTitle(t)
	}
	return a
}

func gridlines(e *xmldom.Element) *ot.Values {
	if e == nil {
		return nil
	}
	s := shape(e)
	if s == nil {
		s = ot.Values{}
	}
	return &s
}

func readTitle(e *xmldom.Element) *Title {
	t := &Title{
		Overlay: boolVal(child(e, "overlay")),
		Layout:  layout(child(e, "layout")),
		Shape:   shape(e),
		Props:   textProps(child(e, "txPr")),
		Rot:     rotation(child(e, "txPr")),
	}
	tx := child(e, "tx")
	if tx == nil {
		return t
	}
	if rich := child(tx, "rich"); rich != nil {
		var lines []string
		var para, run drawingml.Props
		for _, p := range rich.Elements() {
			if p.Space != aNS || p.Local != "p" {
				continue
			}
			var b strings.Builder
			for _, r := range p.Elements() {
				if r.Space != aNS {
					continue
				}
				switch r.Local {
				case "pPr":
					if para == nil {
						para = drawingml.RunProps(r.Child(aNS, "defRPr"), nil)
					}
				case "r", "fld":
					b.WriteString(text(r.Child(aNS, "t")))
					if run == nil {
						run = drawingml.RunProps(r.Child(aNS, "rPr"), nil)
					}
				case "br":
					b.WriteString("\n")
				}
			}
			lines = append(lines, b.String())
		}
		t.Text = strings.Join(lines, "\n")
		t.Props = merge(merge(t.Props, para), run)
		if rot := rotation(rich); rot != nil {
			t.Rot = rot
		}
		return t
	}
	if d := readData(tx); d != nil && len(d.Str) > 0 {
		t.Text = d.Str[0]
	}
	return t
}

func readLegend(e *xmldom.Element) *Legend {
	l := &Legend{
		Pos:     val(child(e, "legendPos")),
		Overlay: boolVal(child(e, "overlay")),
		Text:    textProps(child(e, "txPr")),
		Shape:   shape(e),
		Layout:  layout(child(e, "layout")),
	}
	if l.Pos == "" {
		l.Pos = "r"
	}
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "legendEntry" && boolVal(child(c, "delete")) {
			l.Hidden = append(l.Hidden, intVal(child(c, "idx")))
		}
	}
	return l
}

func layout(e *xmldom.Element) *Layout {
	m := child(e, "manualLayout")
	if m == nil {
		return nil
	}
	l := &Layout{
		Inner: val(child(m, "layoutTarget")) == "inner",
		XMode: val(child(m, "xMode")),
		YMode: val(child(m, "yMode")),
	}
	for _, f := range []struct {
		local string
		into  *float64
	}{{"x", &l.X}, {"y", &l.Y}, {"w", &l.W}, {"h", &l.H}} {
		if v := floatPtr(child(m, f.local)); v != nil {
			*f.into = *v
		}
	}
	if l.XMode == "" {
		l.XMode = "factor"
	}
	if l.YMode == "" {
		l.YMode = "factor"
	}
	return l
}

// textProps are the run properties a txPr gives by default.
func textProps(txPr *xmldom.Element) drawingml.Props {
	if txPr == nil {
		return nil
	}
	for _, p := range txPr.Elements() {
		if p.Space != aNS || p.Local != "p" {
			continue
		}
		if ppr := p.Child(aNS, "pPr"); ppr != nil {
			if props := drawingml.RunProps(ppr.Child(aNS, "defRPr"), nil); len(props) > 0 {
				return props
			}
		}
	}
	return nil
}

// rotation is the rotation of a text body, in 60000ths of a degree.
func rotation(body *xmldom.Element) *int {
	if body == nil {
		return nil
	}
	pr := body.Child(aNS, "bodyPr")
	if pr == nil {
		return nil
	}
	if v, err := strconv.Atoi(pr.Get("rot")); err == nil {
		return &v
	}
	return nil
}

// shape is the fill and line of the spPr under e.
func shape(e *xmldom.Element) ot.Values {
	v := drawingml.ShapeProps(child(e, "spPr"), func(string) string { return "" })
	delete(v, "xfrm")
	delete(v, "geom")
	if len(v) == 0 {
		return nil
	}
	return v
}

func merge(base, over drawingml.Props) drawingml.Props {
	if len(base) == 0 {
		return over
	}
	out := drawingml.Props{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

const aNS = "http://schemas.openxmlformats.org/drawingml/2006/main"

func child(e *xmldom.Element, local string) *xmldom.Element {
	if e == nil {
		return nil
	}
	return e.Child(NS, local)
}

func text(e *xmldom.Element) string {
	if e == nil {
		return ""
	}
	return e.Text()
}

func val(e *xmldom.Element) string {
	if e == nil {
		return ""
	}
	return e.Get("val")
}

func intVal(e *xmldom.Element) int {
	n, _ := strconv.Atoi(val(e))
	return n
}

// axisID is the id of an axis, which Office writes signed as often as not.
func axisID(e *xmldom.Element) int64 {
	n, err := strconv.ParseInt(val(e), 10, 64)
	if err != nil {
		u, _ := strconv.ParseUint(val(e), 10, 32)
		return int64(u)
	}
	return n
}

func floatPtr(e *xmldom.Element) *float64 {
	if e == nil {
		return nil
	}
	v, err := strconv.ParseFloat(val(e), 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return nil
	}
	return &v
}

// boolVal is a CT_Boolean: an element present with no value is true.
func boolVal(e *xmldom.Element) bool {
	if e == nil {
		return false
	}
	return truthy(e.Get("val"), true)
}

func truthy(s string, empty bool) bool {
	switch s {
	case "":
		return empty
	case "1", "true":
		return true
	}
	return false
}

// JSON is the chart as the attribute of a node holds it.
func (c *Chart) JSON() json.RawMessage {
	data, _ := json.Marshal(c)
	return data
}
