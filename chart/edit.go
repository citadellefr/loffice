package chart

import (
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// Kinds are the kinds of plot a chart is given or changed into.
var Kinds = map[string]bool{"bar": true, "line": true, "pie": true, "doughnut": true, "area": true, "scatter": true}

const skeleton = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<c:chartSpace xmlns:c="` + NS + `" xmlns:a="` + aNS + `" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
	`<c:date1904 val="0"/><c:roundedCorners val="0"/><c:chart><c:autoTitleDeleted val="1"/><c:plotArea><c:layout/></c:plotArea>` +
	`<c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/></c:chart></c:chartSpace>`

var (
	chartOrder = []string{"title", "autoTitleDeleted", "pivotFmts", "view3D", "floor", "sideWall", "backWall", "plotArea", "legend",
		"plotVisOnly", "dispBlanksAs", "showDLblsOverMax", "extLst"}
	titleOrder  = []string{"tx", "layout", "overlay", "spPr", "txPr", "extLst"}
	legendOrder = []string{"legendPos", "legendEntry", "layout", "overlay", "spPr", "txPr", "extLst"}
	plotOrder   = []string{"barDir", "grouping", "scatterStyle", "radarStyle", "varyColors", "ser", "dLbls", "dropLines", "hiLowLines",
		"upDownBars", "marker", "smooth", "gapWidth", "gapDepth", "overlap", "serLines", "shape", "firstSliceAng", "holeSize",
		"bubble3D", "bubbleScale", "showNegBubbles", "sizeRepresents", "axId", "extLst"}
	seriesOrder = []string{"idx", "order", "tx", "spPr", "invertIfNegative", "pictureOptions", "marker", "explosion", "dPt", "dLbls",
		"trendline", "errBars", "cat", "val", "xVal", "yVal", "bubbleSize", "shape", "smooth", "bubble3D", "extLst"}
	axisOrder = []string{"axId", "scaling", "delete", "axPos", "majorGridlines", "minorGridlines", "title", "numFmt", "majorTickMark",
		"minorTickMark", "tickLblPos", "spPr", "txPr", "crossAx", "crosses", "crossesAt", "crossBetween", "auto", "lblAlgn", "lblOffset",
		"baseTimeUnit", "majorUnit", "majorTimeUnit", "minorUnit", "minorTimeUnit", "tickLblSkip", "tickMarkSkip", "dispUnits",
		"noMultiLvlLbl", "extLst"}
	// axisCommon are the children an axis keeps when it changes kind.
	axisCommon = []string{"axId", "scaling", "delete", "axPos", "majorGridlines", "minorGridlines", "title", "numFmt", "majorTickMark",
		"minorTickMark", "tickLblPos", "spPr", "txPr", "crossAx", "crosses", "crossesAt"}
)

// New is a chart part drawing want.
func New(want *Chart) []byte {
	doc, err := xmldom.Parse([]byte(skeleton))
	if err != nil {
		panic(err)
	}
	Patch(doc.Root, want)
	return doc.Bytes()
}

// Normalize gives each plot of c a grouping its kind takes and the axes
// it needs: none for a pie, a category and a value axis, or two value
// axes for a scatter, placed for the direction of its bars; a plot of
// another kind than Kinds keeps its own. Axes no plot uses go.
func Normalize(c *Chart) {
	next := int64(1)
	byID := map[int64]int{}
	for i, a := range c.Axes {
		byID[a.ID] = i
		next = max(next, a.ID+1)
	}
	used := map[int64]bool{}
	for i := range c.Plots {
		p := &c.Plots[i]
		switch {
		case p.Kind == "bar" && p.Dir == "":
			p.Dir = "col"
		case (p.Kind == "line" || p.Kind == "area") && p.Grouping == "clustered":
			p.Grouping = "standard"
		}
		if !Kinds[p.Kind] {
			for _, id := range p.Axes {
				used[id] = true
			}
			continue
		}
		if !hasAxes(p.Kind) {
			p.Axes = nil
			continue
		}
		if !p.ThreeD && len(p.Axes) > 2 {
			p.Axes = p.Axes[:2]
		}
		_, okX := byID[first(p.Axes, 0)]
		_, okY := byID[first(p.Axes, 1)]
		if len(p.Axes) < 2 || !okX || !okY {
			x, y := next, next+1
			next += 2
			between := "between"
			if p.Kind == "scatter" || p.Kind == "area" {
				between = "midCat"
			}
			c.Axes = append(c.Axes,
				Axis{ID: x, Kind: "cat", Cross: y, Crosses: "autoZero", Labels: "nextTo", Between: between},
				Axis{ID: y, Kind: "val", Cross: x, Crosses: "autoZero", Labels: "nextTo", Between: between, Grid: &ot.Values{}})
			byID[x], byID[y] = len(c.Axes)-2, len(c.Axes)-1
			p.Axes = []int64{x, y}
		}
		x, y := &c.Axes[byID[p.Axes[0]]], &c.Axes[byID[p.Axes[1]]]
		switch {
		case p.Kind == "scatter":
			if x.Kind != "val" {
				x.Kind, x.Between = "val", "midCat"
			}
		case x.Kind == "val":
			x.Kind = "cat"
		}
		y.Kind = "val"
		across := p.Kind == "bar" && p.Dir == "bar"
		x.Pos = side(x.Pos, across)
		y.Pos = side(y.Pos, !across)
		for _, id := range p.Axes {
			used[id] = true
		}
	}
	c.Axes = slices.DeleteFunc(c.Axes, func(a Axis) bool { return !used[a.ID] })
	if len(c.Axes) == 0 {
		c.Axes = nil
	}
}

func first(ids []int64, i int) int64 {
	if i < len(ids) {
		return ids[i]
	}
	return -1
}

// side is pos if it is on the left or right of the plot when vertical is
// set, at its top or bottom otherwise; the left or bottom if not.
func side(pos string, vertical bool) string {
	if vertical {
		if pos == "l" || pos == "r" {
			return pos
		}
		return "l"
	}
	if pos == "b" || pos == "t" {
		return pos
	}
	return "b"
}

func hasAxes(kind string) bool { return kind != "pie" && kind != "doughnut" }

// Patch changes the chartSpace element space so that it draws want: its
// title, legend, the kind and series of its plots, its axes and their
// titles, and the cells and values of its series. What does not differ
// from what space holds is left as it is, byte for byte.
func Patch(space *xmldom.Element, want *Chart) bool {
	have, err := ReadElement(space)
	if err != nil {
		return false
	}
	p := &patcher{c: prefix(space), a: "a:"}
	if a, ok := spaceOf(space, aNS); ok {
		p.a = a
	} else {
		space.Set("xmlns:a", aNS)
	}
	c := child(space, "chart")
	p.chartTitle(c, have.Title, want.Title)
	plots := child(c, "plotArea")
	if plots == nil {
		plots = p.el("plotArea")
		plots.Append(p.el("layout"))
		c.Insert(plots, chartOrder)
		p.changed = true
	}
	p.plots(plots, have, want)
	p.axes(plots, have, want)
	p.legend(c, have.Legend, want.Legend)
	return p.changed
}

// spaceOf is the prefix e declares for a namespace.
func spaceOf(e *xmldom.Element, space string) (string, bool) {
	for k, v := range e.Spaces() {
		if v == space {
			if k == "" {
				return "", true
			}
			return k + ":", true
		}
	}
	return "", false
}

func prefix(e *xmldom.Element) string {
	if i := strings.IndexByte(e.Name, ':'); i >= 0 {
		return e.Name[:i+1]
	}
	return ""
}

type patcher struct {
	c, a    string
	changed bool
}

func (p *patcher) el(local string, attrs ...string) *xmldom.Element {
	return xmldom.New(NS, p.c+local, attrs...)
}

func (p *patcher) val(local, v string) *xmldom.Element { return p.el(local, "val", v) }

func (p *patcher) drawing(local string, attrs ...string) *xmldom.Element {
	return xmldom.New(aNS, p.a+local, attrs...)
}

// setVal sets the val of the child of e with that name, added where order
// puts it.
func (p *patcher) setVal(e *xmldom.Element, local, v string, order []string) {
	c := child(e, local)
	if c == nil {
		e.Insert(p.val(local, v), order)
		p.changed = true
		return
	}
	if old, ok := c.Attr("val"); !ok || old != v {
		c.Set("val", v)
		p.changed = true
	}
}

func (p *patcher) remove(e *xmldom.Element, local string) {
	if c := child(e, local); c != nil {
		e.Remove(c)
		p.changed = true
	}
}

func (p *patcher) chartTitle(c *xmldom.Element, have, want *Title) {
	if have != nil && want == nil {
		p.setVal(c, "autoTitleDeleted", "1", chartOrder)
	}
	if have == nil && want != nil && child(c, "autoTitleDeleted") != nil {
		p.setVal(c, "autoTitleDeleted", "0", chartOrder)
	}
	p.title(c, have, want, chartOrder, false)
}

// title adds, removes or rewrites the title of e, a chart or an axis.
func (p *patcher) title(e *xmldom.Element, have, want *Title, order []string, vertical bool) {
	t := child(e, "title")
	switch {
	case want == nil:
		p.remove(e, "title")
	case t == nil:
		t = p.el("title")
		if want.Text != "" {
			t.Append(p.rich(want.Text, nil, vertical))
		}
		t.Append(p.val("overlay", "0"))
		e.Insert(t, order)
		p.changed = true
	case have == nil || have.Text != want.Text:
		tx := child(t, "tx")
		switch {
		case want.Text == "":
			p.remove(t, "tx")
		case tx == nil:
			t.Insert(p.rich(want.Text, nil, vertical), titleOrder)
		default:
			t.Replace(tx, p.rich(want.Text, child(tx, "rich"), vertical))
		}
		p.changed = true
	}
}

// rich is a c:tx of rich text, which keeps the body, list styles and the
// formatting of the first paragraph and run of old.
func (p *patcher) rich(text string, old *xmldom.Element, vertical bool) *xmldom.Element {
	var body, list, para, run *xmldom.Element
	if old != nil {
		body, list = old.Child(aNS, "bodyPr"), old.Child(aNS, "lstStyle")
		if first := old.Child(aNS, "p"); first != nil {
			para = first.Child(aNS, "pPr")
			if r := first.Child(aNS, "r"); r != nil {
				run = r.Child(aNS, "rPr")
			}
		}
	}
	if body == nil {
		body = p.drawing("bodyPr")
		if vertical {
			body.Set("rot", "-5400000")
			body.Set("vert", "horz")
		}
	}
	r := p.el("rich")
	r.Append(body.Clone())
	if list == nil {
		list = p.drawing("lstStyle")
	}
	r.Append(list.Clone())
	for _, line := range strings.Split(text, "\n") {
		para2 := p.drawing("p")
		if para != nil {
			para2.Append(para.Clone())
		}
		if line != "" {
			rn := p.drawing("r")
			if run != nil {
				rn.Append(run.Clone())
			}
			t := p.drawing("t")
			t.Append(xmldom.EscapeText(line))
			rn.Append(t)
			para2.Append(rn)
		}
		r.Append(para2)
	}
	tx := p.el("tx")
	tx.Append(r)
	return tx
}

func (p *patcher) legend(c *xmldom.Element, have, want *Legend) {
	l := child(c, "legend")
	switch {
	case want == nil:
		p.remove(c, "legend")
	case l == nil:
		l = p.el("legend")
		l.Append(p.val("legendPos", legendPos(want.Pos)))
		l.Append(p.val("overlay", "0"))
		c.Insert(l, chartOrder)
		p.changed = true
	case have == nil || legendPos(have.Pos) != legendPos(want.Pos):
		p.setVal(l, "legendPos", legendPos(want.Pos), legendOrder)
		p.remove(l, "layout")
	}
}

func legendPos(pos string) string {
	if pos == "" {
		return "r"
	}
	return pos
}

// plotElements are the plots of a plot area.
func plotElements(area *xmldom.Element) []*xmldom.Element {
	var out []*xmldom.Element
	for _, e := range area.Elements() {
		if e.Space == NS && strings.HasSuffix(e.Local, "Chart") {
			out = append(out, e)
		}
	}
	return out
}

func axisElements(area *xmldom.Element) []*xmldom.Element {
	var out []*xmldom.Element
	for _, e := range area.Elements() {
		if e.Space == NS && strings.HasSuffix(e.Local, "Ax") {
			out = append(out, e)
		}
	}
	return out
}

// insertAfter puts c after the child after of e, first when after is nil.
func insertAfter(e, after, c *xmldom.Element) {
	if len(e.Content) == 0 {
		e.Append(c)
		return
	}
	at := 0
	for i, n := range e.Content {
		if n == xmldom.Node(after) {
			at = i + 1
		}
	}
	e.Content = slices.Insert(e.Content, at, xmldom.Node(c))
}

func last(list []*xmldom.Element) *xmldom.Element {
	if len(list) == 0 {
		return nil
	}
	return list[len(list)-1]
}

func (p *patcher) plots(area *xmldom.Element, have, want *Chart) {
	used := map[int]bool{}
	for _, pl := range have.Plots {
		for _, s := range pl.Series {
			used[s.Index] = true
		}
	}
	elements := plotElements(area)
	for i, w := range want.Plots {
		if i >= len(elements) {
			e := p.newPlot(w)
			insertAfter(area, firstOf(last(plotElements(area)), child(area, "layout")), e)
			p.changed = true
			p.series(e, Plot{}, w, used)
			continue
		}
		e, h := elements[i], have.Plots[i]
		if Kinds[w.Kind] && (h.Kind != w.Kind || h.ThreeD && !w.ThreeD || w.Kind == "bar" && dir(h.Dir) != dir(w.Dir)) {
			n := p.retype(e, w)
			area.Replace(e, n)
			e, h = n, readPlot(n)
			p.changed = true
		}
		if w.Grouping != "" && w.Grouping != h.Grouping && (w.Kind == "bar" || w.Kind == "line" || w.Kind == "area") {
			p.setVal(e, "grouping", w.Grouping, plotOrder)
			if w.Kind == "bar" {
				p.overlap(e, w.Grouping)
			}
		}
		if !slices.Equal(h.Axes, w.Axes) && hasAxes(w.Kind) {
			for _, c := range e.Elements() {
				if c.Space == NS && c.Local == "axId" {
					e.Remove(c)
				}
			}
			for _, id := range w.Axes {
				e.Insert(p.val("axId", strconv.FormatInt(id, 10)), plotOrder)
			}
			p.changed = true
		}
		p.series(e, h, w, used)
	}
	for _, e := range elements[min(len(want.Plots), len(elements)):] {
		area.Remove(e)
		p.changed = true
	}
}

func firstOf(list ...*xmldom.Element) *xmldom.Element {
	for _, e := range list {
		if e != nil {
			return e
		}
	}
	return nil
}

func dir(d string) string {
	if d == "" {
		return "col"
	}
	return d
}

func (p *patcher) overlap(e *xmldom.Element, grouping string) {
	if grouping == "stacked" || grouping == "percentStacked" {
		p.setVal(e, "overlap", "100", plotOrder)
	} else if o := child(e, "overlap"); o != nil && o.Get("val") == "100" {
		p.remove(e, "overlap")
	}
}

// newPlot is a plot of the kind of w, without series.
func (p *patcher) newPlot(w Plot) *xmldom.Element {
	e := p.el(w.Kind + "Chart")
	grouping := w.Grouping
	switch w.Kind {
	case "bar":
		if grouping == "" || grouping == "standard" {
			grouping = "clustered"
		}
		e.Append(p.val("barDir", dir(w.Dir)))
		e.Append(p.val("grouping", grouping))
		e.Append(p.val("varyColors", "0"))
		e.Append(p.val("gapWidth", "150"))
		p.overlap(e, grouping)
	case "line", "area":
		if grouping == "" || grouping == "clustered" {
			grouping = "standard"
		}
		e.Append(p.val("grouping", grouping))
		e.Append(p.val("varyColors", "0"))
		if w.Kind == "line" {
			e.Append(p.val("marker", "1"))
		}
	case "pie", "doughnut":
		e.Append(p.val("varyColors", "1"))
		e.Append(p.val("firstSliceAng", "0"))
		if w.Kind == "doughnut" {
			e.Append(p.val("holeSize", "50"))
		}
	case "scatter":
		e.Append(p.val("scatterStyle", "lineMarker"))
		e.Append(p.val("varyColors", "0"))
	}
	if hasAxes(w.Kind) {
		for _, id := range w.Axes {
			e.Append(p.val("axId", strconv.FormatInt(id, 10)))
		}
	}
	return e
}

// retype is e, a plot, changed into one of the kind of w: its series keep
// their names and values, not their looks, which differ from a kind to
// another.
func (p *patcher) retype(e *xmldom.Element, w Plot) *xmldom.Element {
	n := p.newPlot(w)
	xy := w.Kind == "scatter"
	for _, s := range e.Elements() {
		if s.Space != NS || s.Local != "ser" {
			continue
		}
		ser := p.el("ser")
		for _, local := range []string{"idx", "order", "tx"} {
			if c := child(s, local); c != nil {
				ser.Append(c.Clone())
			}
		}
		for _, d := range [][3]string{{"cat", "xVal", "cat"}, {"xVal", "xVal", "cat"}, {"val", "yVal", "val"}, {"yVal", "yVal", "val"}} {
			c := child(s, d[0])
			if c == nil {
				continue
			}
			c = c.Clone()
			if xy {
				c.Rename(d[1])
			} else {
				c.Rename(d[2])
			}
			ser.Insert(c, seriesOrder)
		}
		p.seriesDefaults(ser, w.Kind)
		n.Insert(ser, plotOrder)
	}
	return n
}

// seriesDefaults adds to a series what its kind draws it with by default.
func (p *patcher) seriesDefaults(ser *xmldom.Element, kind string) {
	switch kind {
	case "bar":
		ser.Insert(p.val("invertIfNegative", "0"), seriesOrder)
	case "line":
		m := p.el("marker")
		m.Append(p.val("symbol", "none"))
		ser.Insert(m, seriesOrder)
		ser.Insert(p.val("smooth", "0"), seriesOrder)
	case "scatter":
		sp := p.el("spPr")
		ln := p.drawing("ln", "w", "19050")
		ln.Append(p.drawing("noFill"))
		sp.Append(ln)
		ser.Insert(sp, seriesOrder)
		ser.Insert(p.val("smooth", "0"), seriesOrder)
	}
}

// series patches the series of e, a plot read as h, to be those of w,
// each the one of the same index. used are the indexes of the series of
// the chart.
func (p *patcher) series(e *xmldom.Element, h, w Plot, used map[int]bool) {
	elements := map[int]*xmldom.Element{}
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "ser" {
			if i := intVal(child(c, "idx")); elements[i] == nil {
				elements[i] = c
			}
		}
	}
	read := map[int]Series{}
	for _, s := range h.Series {
		read[s.Index] = s
	}
	xy := strings.HasPrefix(e.Local, "scatter") || strings.HasPrefix(e.Local, "bubble")
	cat, val := "cat", "val"
	if xy {
		cat, val = "xVal", "yVal"
	}
	kind := strings.TrimSuffix(e.Local, "Chart")
	kept := map[*xmldom.Element]bool{}
	for _, s := range w.Series {
		if ser := elements[s.Index]; ser != nil && !kept[ser] {
			kept[ser] = true
			hs := read[s.Index]
			p.data(ser, "tx", hs.Name, s.Name)
			p.data(ser, cat, hs.Cat, s.Cat)
			p.data(ser, val, hs.Val, s.Val)
			continue
		}
		ser := p.el("ser")
		idx := max(s.Index, 0)
		for used[idx] {
			idx++
		}
		used[idx] = true
		ser.Append(p.val("idx", strconv.Itoa(idx)))
		ser.Append(p.val("order", strconv.Itoa(idx)))
		p.seriesDefaults(ser, kind)
		e.Insert(ser, plotOrder)
		kept[ser] = true
		p.changed = true
		p.data(ser, "tx", nil, s.Name)
		p.data(ser, cat, nil, s.Cat)
		p.data(ser, val, nil, s.Val)
	}
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "ser" && !kept[c] {
			e.Remove(c)
			p.changed = true
		}
	}
}

// data patches the child of ser holding a name, categories or values, read
// as have, so that it holds want: its reference and the values cached.
func (p *patcher) data(ser *xmldom.Element, local string, have, want *Data) {
	e := child(ser, local)
	switch {
	case want == nil:
		if e != nil && have != nil {
			ser.Remove(e)
			p.changed = true
		}
		return
	case e == nil || have == nil:
		if e != nil {
			ser.Remove(e)
		}
		ser.Insert(p.newData(local, want), seriesOrder)
		p.changed = true
		return
	case local == "tx" && want.Ref == "":
		if have.Ref != "" || !slices.Equal(have.Str, want.Str) {
			ser.Replace(e, p.newData(local, want))
			p.changed = true
		}
		return
	}
	ref := firstElement(e)
	numbers := numeric(local, want, ref == nil || ref.Local == "numRef" || ref.Local == "numLit")
	kind := map[bool]string{true: "numRef", false: "strRef"}[numbers]
	if want.Ref == "" {
		kind = map[bool]string{true: "numLit", false: "strLit"}[numbers]
	}
	if ref == nil || ref.Local != kind && !(ref.Local == "multiLvlStrRef" && want.Levels != nil) {
		ser.Replace(e, p.newData(local, want))
		p.changed = true
		return
	}
	if want.Ref != have.Ref && want.Ref != "" {
		if f := child(ref, "f"); f != nil {
			f.Content = []xmldom.Node{xmldom.EscapeText(want.Ref)}
		} else {
			f = p.el("f")
			f.Append(xmldom.EscapeText(want.Ref))
			ref.Insert(f, []string{"f"})
		}
		p.changed = true
	}
	if sameCache(have, want, numbers) || ref.Local == "multiLvlStrRef" {
		return
	}
	cache := p.cache(want, numbers, kind)
	if strings.HasSuffix(kind, "Lit") {
		ser.Replace(e, p.newData(local, want))
	} else if old := child(ref, map[bool]string{true: "numCache", false: "strCache"}[numbers]); old != nil {
		ref.Replace(old, cache)
	} else {
		ref.Append(cache)
	}
	p.changed = true
}

func firstElement(e *xmldom.Element) *xmldom.Element {
	for _, c := range e.Elements() {
		if c.Space == NS {
			return c
		}
	}
	return nil
}

func sameCache(have, want *Data, numbers bool) bool {
	if have.Count != want.Count {
		return false
	}
	if !numbers {
		return slices.Equal(have.Str, want.Str)
	}
	if want.Format != "" && have.Format != want.Format {
		return false
	}
	return slices.EqualFunc(have.Num, want.Num, func(a, b *float64) bool {
		return a == nil && b == nil || a != nil && b != nil && *a == *b
	})
}

// numeric tells whether the child of a series named local holds d as
// numbers: values always, categories when d has numbers, or none and
// fallback is set.
func numeric(local string, d *Data, fallback bool) bool {
	switch local {
	case "tx":
		return false
	case "cat", "xVal":
		return d.Num != nil || d.Str == nil && d.Levels == nil && fallback
	}
	return true
}

// newData is the child of a series holding d: a name, categories or
// values.
func (p *patcher) newData(local string, d *Data) *xmldom.Element {
	e := p.el(local)
	if local == "tx" && d.Ref == "" {
		v := p.el("v")
		if len(d.Str) > 0 {
			v.Append(xmldom.EscapeText(d.Str[0]))
		}
		e.Append(v)
		return e
	}
	numbers := numeric(local, d, false)
	var ref *xmldom.Element
	if d.Ref == "" {
		ref = p.cache(d, numbers, map[bool]string{true: "numLit", false: "strLit"}[numbers])
	} else {
		ref = p.el(map[bool]string{true: "numRef", false: "strRef"}[numbers])
		f := p.el("f")
		f.Append(xmldom.EscapeText(d.Ref))
		ref.Append(f)
		ref.Append(p.cache(d, numbers, ""))
	}
	e.Append(ref)
	return e
}

// cache is the numCache or strCache of d, or the literal named local.
func (p *patcher) cache(d *Data, numbers bool, local string) *xmldom.Element {
	if local == "" || strings.HasSuffix(local, "Ref") {
		local = map[bool]string{true: "numCache", false: "strCache"}[numbers]
	}
	c := p.el(local)
	if numbers {
		f := p.el("formatCode")
		f.Append(xmldom.EscapeText(either(d.Format, "General")))
		c.Append(f)
	}
	c.Append(p.val("ptCount", strconv.Itoa(d.Count)))
	point := func(i int, v string) {
		pt := p.el("pt", "idx", strconv.Itoa(i))
		e := p.el("v")
		e.Append(xmldom.EscapeText(v))
		pt.Append(e)
		c.Append(pt)
	}
	if numbers {
		for i, v := range d.Num {
			if v != nil && !math.IsInf(*v, 0) && !math.IsNaN(*v) {
				point(i, strconv.FormatFloat(*v, 'g', -1, 64))
			}
		}
	} else {
		for i, s := range d.Str {
			if s != "" {
				point(i, s)
			}
		}
	}
	return c
}

func either(s, or string) string {
	if s == "" {
		return or
	}
	return s
}

func (p *patcher) axes(area *xmldom.Element, have, want *Chart) {
	haveByID := map[int64]Axis{}
	for _, a := range have.Axes {
		haveByID[a.ID] = a
	}
	elements := map[int64]*xmldom.Element{}
	for _, e := range axisElements(area) {
		elements[axisID(child(e, "axId"))] = e
	}
	wanted := map[int64]bool{}
	for _, a := range want.Axes {
		wanted[a.ID] = true
		e, h := elements[a.ID], haveByID[a.ID]
		switch {
		case e == nil:
			e = p.newAxis(a)
			insertAfter(area, firstOf(last(axisElements(area)), last(plotElements(area)), child(area, "layout")), e)
			elements[a.ID] = e
			p.changed = true
			continue
		case axisKind(a.Kind) != axisKind(h.Kind):
			n := p.convert(e, a)
			area.Replace(e, n)
			e = n
			p.changed = true
		}
		if a.Pos != "" && a.Pos != h.Pos {
			p.setVal(e, "axPos", a.Pos, axisOrder)
		}
		if a.Kind == "val" && a.Between != "" && a.Between != h.Between {
			p.setVal(e, "crossBetween", a.Between, axisOrder)
		}
		p.title(e, h.Title, a.Title, axisOrder, a.Pos == "l" || a.Pos == "r")
	}
	for id, e := range elements {
		if !wanted[id] {
			area.Remove(e)
			p.changed = true
		}
	}
}

// axisKind is "val" for a value axis, "cat" for one of categories or
// dates.
func axisKind(kind string) string {
	if kind == "date" {
		return "cat"
	}
	return kind
}

func (p *patcher) newAxis(a Axis) *xmldom.Element {
	e := p.el(axisKind(a.Kind) + "Ax")
	e.Append(p.val("axId", strconv.FormatInt(a.ID, 10)))
	scaling := p.el("scaling")
	scaling.Append(p.val("orientation", "minMax"))
	e.Append(scaling)
	e.Append(p.val("delete", "0"))
	e.Append(p.val("axPos", either(a.Pos, "b")))
	if a.Grid != nil {
		e.Append(p.el("majorGridlines"))
	}
	if a.Title != nil {
		p.title(e, nil, a.Title, axisOrder, a.Pos == "l" || a.Pos == "r")
	}
	e.Append(p.el("numFmt", "formatCode", "General", "sourceLinked", "1"))
	e.Append(p.val("majorTickMark", "out"))
	e.Append(p.val("minorTickMark", "none"))
	e.Append(p.val("tickLblPos", "nextTo"))
	e.Append(p.val("crossAx", strconv.FormatInt(a.Cross, 10)))
	e.Append(p.val("crosses", "autoZero"))
	p.axisDefaults(e, a)
	return e
}

func (p *patcher) axisDefaults(e *xmldom.Element, a Axis) {
	if axisKind(a.Kind) == "val" {
		e.Append(p.val("crossBetween", either(a.Between, "between")))
		return
	}
	e.Append(p.val("auto", "1"))
	e.Append(p.val("lblAlgn", "ctr"))
	e.Append(p.val("lblOffset", "100"))
	e.Append(p.val("noMultiLvlLbl", "0"))
}

// convert is the axis e changed into one of the kind of a, a value axis
// into one of categories or the reverse.
func (p *patcher) convert(e *xmldom.Element, a Axis) *xmldom.Element {
	n := p.el(axisKind(a.Kind) + "Ax")
	for _, c := range e.Elements() {
		if c.Space == NS && slices.Contains(axisCommon, c.Local) {
			n.Append(c.Clone())
		}
	}
	p.axisDefaults(n, a)
	return n
}
