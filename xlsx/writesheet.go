package xlsx

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/ot"
)

// worksheetOrder is the order of the children of a worksheet.
var worksheetOrder = []string{"sheetPr", "dimension", "sheetViews", "sheetFormatPr", "cols", "sheetData", "sheetCalcPr",
	"sheetProtection", "protectedRanges", "scenarios", "autoFilter", "sortState", "dataConsolidate", "customSheetViews",
	"mergeCells", "phoneticPr", "conditionalFormatting", "dataValidations", "hyperlinks", "printOptions", "pageMargins",
	"pageSetup", "headerFooter", "rowBreaks", "colBreaks", "customProperties", "cellWatches", "ignoredErrors",
	"smartTags", "drawing", "legacyDrawing", "legacyDrawingHF", "drawingHF", "picture", "oleObjects", "controls",
	"webPublishItems", "tableParts", "extLst"}

var sheetPrOrder = []string{"tabColor", "outlinePr", "pageSetUpPr"}

var sheetViewOrder = []string{"pane", "selection", "pivotSelection", "extLst"}

// sheet writes a worksheet from its node.
func (w *writer) sheet(n *ot.Node, p *sheetPart) error {
	root := p.doc.Root.Clone()
	if len(w.moves) > 0 {
		w.mover().sheet(root, n.ID)
	}
	old := ot.Values{}
	if loaded := w.d.loaded.Node(n.ID); loaded != nil && loaded.Type == "sheet" {
		old = loaded.Attrs
	}
	w.settings(root, old, n.Attrs)

	data := root.Child(mainNS, "sheetData")
	if data == nil {
		data = child(root, "sheetData", worksheetOrder)
	}
	c := &sheetWriter{w: w, prefix: prefixOf(data), buf: &bytes.Buffer{}}
	if n.Grid != nil {
		c.buf.Grow(64 * n.Grid.Len())
		c.cells(n.Grid)
	}
	data.Content = []xmldom.Node{xmldom.Raw(c.buf.Bytes())}
	if c.buf.Len() == 0 {
		data.Content = nil
	}

	dim := "A1"
	if c.maxRow > 0 {
		dim = formula.CellName(c.minRow, c.minCol)
		if c.maxRow != c.minRow || c.maxCol != c.minCol {
			dim += ":" + formula.CellName(c.maxRow, c.maxCol)
		}
	}
	child(root, "dimension", worksheetOrder).Set("ref", dim)

	c.columns(root, tail(n.Attrs))
	if merges := root.Child(mainNS, "mergeCells"); merges != nil {
		root.Remove(merges)
	}
	if len(c.merges) > 0 {
		merges := child(root, "mergeCells", worksheetOrder)
		merges.Set("count", strconv.Itoa(len(c.merges)))
		for _, ref := range c.merges {
			merges.Append(xmldom.New(mainNS, prefixOf(merges)+"mergeCell", "ref", ref))
		}
	}
	w.stringRefs[p] = c.strings
	doc := &xmldom.Document{Prolog: p.doc.Prolog, Root: root, Tail: p.doc.Tail}
	return w.put(p.name, typeWorksheet, doc.Bytes())
}

func tail(attrs ot.Values) *Tail {
	var t Tail
	if json.Unmarshal(attrs["tail"], &t) != nil || t.From < 1 || t.To < t.From {
		return nil
	}
	return &t
}

// settings writes the settings of a sheet that differ from those read.
func (w *writer) settings(root *xmldom.Element, old, attrs ot.Values) {
	changed := func(key string) bool { return !bytes.Equal(old[key], attrs[key]) }
	view := func() *xmldom.Element {
		views := child(root, "sheetViews", worksheetOrder)
		v := views.Child(mainNS, "sheetView")
		if v == nil {
			v = xmldom.New(mainNS, prefixOf(views)+"sheetView", "workbookViewId", "0")
			views.Append(v)
		}
		return v
	}
	if changed("grid") {
		grid := true
		_ = json.Unmarshal(attrs["grid"], &grid)
		if grid {
			view().Unset("showGridLines")
		} else {
			view().Set("showGridLines", "0")
		}
	}
	if changed("rtl") {
		var rtl bool
		_ = json.Unmarshal(attrs["rtl"], &rtl)
		setAttr(view(), "rightToLeft", boolAttr(rtl))
	}
	if changed("zoom") {
		var zoom int
		_ = json.Unmarshal(attrs["zoom"], &zoom)
		if zoom < 10 || zoom > 400 {
			view().Unset("zoomScale")
		} else {
			view().Set("zoomScale", strconv.Itoa(zoom))
		}
	}
	if changed("frozen") {
		var f frozen
		_ = json.Unmarshal(attrs["frozen"], &f)
		freeze(view(), f)
	}
	if changed("tab") {
		pr := child(root, "sheetPr", worksheetOrder)
		if tab := pr.Child(mainNS, "tabColor"); tab != nil {
			pr.Remove(tab)
		}
		var c Color
		if json.Unmarshal(attrs["tab"], &c) == nil && c != (Color{}) {
			pr.Insert(colorElement(pr, "tabColor", &c), sheetPrOrder)
		}
	}
	if changed(filterKey) {
		writeFilter(root, old[filterKey], attrs[filterKey])
	}
	if changed("dw") || changed("dh") {
		f := child(root, "sheetFormatPr", worksheetOrder)
		var dw, dh float64
		_ = json.Unmarshal(attrs["dw"], &dw)
		_ = json.Unmarshal(attrs["dh"], &dh)
		if changed("dw") {
			f.Unset("baseColWidth")
			f.Unset("defaultColWidth")
			if dw > 0 {
				f.Set("defaultColWidth", formatFloat(dw))
			}
		}
		if dh <= 0 {
			dh = 15
		}
		f.Set("defaultRowHeight", formatFloat(dh))
	}
}

// freeze keeps the first rows and columns of a sheet in view. The
// selections of the panes that were go with them: Excel makes new ones.
func freeze(view *xmldom.Element, f frozen) {
	for _, c := range view.Elements() {
		if c.Space == mainNS && (c.Local == "pane" || c.Local == "selection") {
			view.Remove(c)
		}
	}
	if f.Rows <= 0 && f.Cols <= 0 {
		return
	}
	pane := xmldom.New(mainNS, prefixOf(view)+"pane")
	if f.Cols > 0 {
		pane.Set("xSplit", strconv.Itoa(f.Cols))
	}
	if f.Rows > 0 {
		pane.Set("ySplit", strconv.Itoa(f.Rows))
	}
	active := "bottomRight"
	switch {
	case f.Cols <= 0:
		active = "bottomLeft"
	case f.Rows <= 0:
		active = "topRight"
	}
	pane.Set("topLeftCell", formula.CellName(max(f.Rows, 0)+1, max(f.Cols, 0)+1))
	pane.Set("activePane", active)
	pane.Set("state", "frozen")
	view.Insert(pane, sheetViewOrder)
}

// colorElement is a color written as a workbook writes it.
func colorElement(parent *xmldom.Element, local string, c *Color) *xmldom.Element {
	e := xmldom.New(mainNS, prefixOf(parent)+local)
	switch {
	case c.Auto:
		e.Set("auto", "1")
	case c.Theme != nil:
		e.Set("theme", strconv.Itoa(*c.Theme))
	case validRGB(c.RGB):
		e.Set("rgb", "FF"+c.RGB)
	default:
		e.Set("auto", "1")
	}
	if c.Tint != 0 && c.Tint >= -1 && c.Tint <= 1 {
		e.Set("tint", formatFloat(c.Tint))
	}
	return e
}

func validRGB(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, c := range []byte(s) {
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// sheetWriter writes the rows of a sheet.
type sheetWriter struct {
	w      *writer
	prefix string
	buf    *bytes.Buffer
	// cols are the fields of whole columns, row 0 of the grid.
	cols   []ot.Cell
	merges []string
	// the corners of the cells written
	minRow, minCol, maxRow, maxCol int
	strings                        int
}

func (c *sheetWriter) cells(g *ot.Grid) {
	row := 0
	open := false
	g.Each(0, formula.MaxRows, func(r, col int, raw json.RawMessage) bool {
		if r == 0 {
			if col > 0 {
				c.cols = append(c.cols, ot.Cell{Row: r, Col: col, Fields: raw})
			}
			return true
		}
		if r != row {
			if open {
				c.end("row")
			}
			row, open = r, true
			if col == 0 {
				c.row(r, raw)
				return true
			}
			c.row(r, nil)
		}
		if col > 0 {
			c.cell(r, col, raw)
		}
		return true
	})
	if open {
		c.end("row")
	}
}

func (c *sheetWriter) end(local string) {
	c.buf.WriteString("</")
	c.buf.WriteString(c.prefix)
	c.buf.WriteString(local)
	c.buf.WriteByte('>')
}

func (c *sheetWriter) attr(name, value string) {
	c.buf.WriteByte(' ')
	c.buf.WriteString(name)
	c.buf.WriteString(`="`)
	c.buf.Write(escapeAttr(value))
	c.buf.WriteByte('"')
}

// row opens a row, with its fields.
func (c *sheetWriter) row(r int, raw json.RawMessage) {
	var f lineFields
	_ = json.Unmarshal(raw, &f)
	c.buf.WriteByte('<')
	c.buf.WriteString(c.prefix)
	c.buf.WriteString("row")
	c.attr("r", strconv.Itoa(r))
	if s, ok := c.w.styles.index(f.S); ok && s > 0 {
		c.attr("s", strconv.Itoa(s))
		c.attr("customFormat", "1")
	}
	if f.H > 0 && f.H <= maxHeight {
		c.attr("ht", formatFloat(f.H))
		if f.CH {
			c.attr("customHeight", "1")
		}
	}
	if f.Hide {
		c.attr("hidden", "1")
	}
	if f.OL > 0 && f.OL <= maxOutline {
		c.attr("outlineLevel", strconv.Itoa(f.OL))
	}
	if f.CL {
		c.attr("collapsed", "1")
	}
	c.buf.WriteByte('>')
}

// cell writes a cell, which has at least one field.
func (c *sheetWriter) cell(r, col int, raw json.RawMessage) {
	var f cellFields
	if json.Unmarshal(raw, &f) != nil {
		return
	}
	if c.maxRow == 0 {
		c.minRow, c.minCol = r, col
	}
	c.minCol = min(c.minCol, col)
	c.maxRow, c.maxCol = r, max(c.maxCol, col)
	if f.M != nil && f.M[0] >= 1 && f.M[1] >= 1 && (f.M[0] > 1 || f.M[1] > 1) &&
		r+f.M[0]-1 <= formula.MaxRows && col+f.M[1]-1 <= formula.MaxCols {
		c.merges = append(c.merges, formula.CellName(r, col)+":"+formula.CellName(r+f.M[0]-1, col+f.M[1]-1))
	}

	typ, value := c.value(&f)
	c.buf.WriteByte('<')
	c.buf.WriteString(c.prefix)
	c.buf.WriteString("c")
	c.attr("r", formula.CellName(r, col))
	if s, ok := c.w.styles.index(f.S); ok && s > 0 {
		c.attr("s", strconv.Itoa(s))
	}
	if typ != "" {
		c.attr("t", typ)
	}
	if f.CM > 0 {
		c.attr("cm", strconv.Itoa(f.CM))
	}
	if f.VM > 0 {
		c.attr("vm", strconv.Itoa(f.VM))
	}
	fx := f.FX != "" && c.w.d.isTrusted(f.FX)
	if f.F == "" && !fx && value == nil {
		c.buf.WriteString("/>")
		return
	}
	c.buf.WriteByte('>')
	switch {
	case fx:
		c.buf.WriteString(f.FX)
	case f.F != "":
		c.buf.WriteByte('<')
		c.buf.WriteString(c.prefix)
		c.buf.WriteString("f")
		if _, ok := formula.ParseArea(f.FA); ok && f.FA != "" {
			c.attr("t", "array")
			c.attr("ref", f.FA)
		}
		if f.CA {
			c.attr("ca", "1")
		}
		c.buf.WriteByte('>')
		c.buf.Write(escapeText(f.F))
		c.end("f")
	}
	if value != nil {
		c.buf.WriteByte('<')
		c.buf.WriteString(c.prefix)
		c.buf.WriteString("v>")
		c.buf.Write(value)
		c.end("v")
	}
	c.end("c")
}

// value is the type and the escaped value of a cell, nil when it has none.
func (c *sheetWriter) value(f *cellFields) (string, []byte) {
	if f.E != "" {
		if !formula.IsError(f.E) {
			return "", nil
		}
		return "e", []byte(f.E)
	}
	if len(f.V) == 0 {
		return "", nil
	}
	switch f.V[0] {
	case '"':
		var s string
		if json.Unmarshal(f.V, &s) != nil {
			return "", nil
		}
		if f.F != "" {
			return "str", escapeText(encodeEscapes(s))
		}
		item := sharedString{text: s}
		if f.Rich != "" && c.w.d.isTrusted(f.Rich) {
			if rich, err := readItem([]byte(f.Rich)); err == nil && rich.text == s {
				item.rich = f.Rich
			}
		}
		c.strings++
		return "s", strconv.AppendInt(nil, int64(c.w.strings.add(item)), 10)
	case 't':
		return "b", []byte("1")
	case 'f':
		return "b", []byte("0")
	case '{', '[', 'n':
		return "", nil
	}
	n, err := strconv.ParseFloat(string(f.V), 64)
	if err != nil || math.IsInf(n, 0) || math.IsNaN(n) {
		return "", nil
	}
	return "", f.V
}

// columns writes the fields of whole columns, those next to each other
// that share them together.
func (c *sheetWriter) columns(root *xmldom.Element, t *Tail) {
	if cols := root.Child(mainNS, "cols"); cols != nil {
		root.Remove(cols)
	}
	if len(c.cols) == 0 && t == nil {
		return
	}
	cols := child(root, "cols", worksheetOrder)
	put := func(lo, hi int, f lineFields) {
		e := xmldom.New(mainNS, prefixOf(cols)+"col", "min", strconv.Itoa(lo), "max", strconv.Itoa(hi))
		if f.W > 0 && f.W <= maxWidth {
			e.Set("width", formatFloat(f.W))
		}
		if s, ok := c.w.styles.index(f.S); ok && s > 0 {
			e.Set("style", strconv.Itoa(s))
		}
		setAttr(e, "hidden", boolAttr(f.Hide))
		setAttr(e, "bestFit", boolAttr(f.BF))
		setAttr(e, "customWidth", boolAttr(f.CW))
		if f.OL > 0 && f.OL <= maxOutline {
			e.Set("outlineLevel", strconv.Itoa(f.OL))
		}
		setAttr(e, "collapsed", boolAttr(f.CL))
		cols.Append(e)
	}
	for i := 0; i < len(c.cols); {
		j := i + 1
		for j < len(c.cols) && c.cols[j].Col == c.cols[j-1].Col+1 && bytes.Equal(c.cols[j].Fields, c.cols[i].Fields) {
			j++
		}
		var f lineFields
		_ = json.Unmarshal(c.cols[i].Fields, &f)
		put(c.cols[i].Col, c.cols[j-1].Col, f)
		i = j
	}
	if t != nil {
		lo := t.From
		if len(c.cols) > 0 {
			lo = max(lo, c.cols[len(c.cols)-1].Col+1)
		}
		if lo <= min(t.To, formula.MaxCols) {
			put(lo, min(t.To, formula.MaxCols), t.lineFields)
		}
	}
	if len(cols.Content) == 0 {
		root.Remove(cols)
	}
}

// escapeText is s as element content, carriage returns kept.
func escapeText(s string) []byte {
	return bytes.ReplaceAll(xmldom.EscapeText(s), []byte("\r"), []byte("&#13;"))
}

// escapeAttr is s as an attribute value.
func escapeAttr(s string) []byte {
	var b []byte
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			b = append(b, "&amp;"...)
		case '<':
			b = append(b, "&lt;"...)
		case '>':
			b = append(b, "&gt;"...)
		case '"':
			b = append(b, "&quot;"...)
		case '\t', '\n', '\r':
			b = append(b, "&#"...)
			b = strconv.AppendInt(b, int64(c), 10)
			b = append(b, ';')
		default:
			b = append(b, c)
		}
	}
	return b
}
