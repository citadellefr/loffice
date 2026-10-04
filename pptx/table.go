package pptx

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"

	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// A table is a frame whose "tbl" holds the options of its tblPr, and
// "grid" the widths of its columns, in EMU. Its children are rows:
//
//	frame "s256-4"        frame "table", tbl, grid
//	  tr "s256-4-r1"      h, the height the row has at least
//	    tc "s256-4-r1-c1" a cell: its text, as a shape's, and its tcPr
//
// A cell holds "gridSpan" and "rowSpan" when it spans columns or rows,
// "hMerge" or "vMerge" when another's span covers it; "fill", the borders
// "lnL" "lnR" "lnT" "lnB" "lnTlToBr" "lnBlToTr", "mar" (l, r, t, b) and
// "anchor" from its tcPr.

const tableURI = "http://schemas.openxmlformats.org/drawingml/2006/table"

// Table is the tblPr of a table.
type Table struct {
	Style    string          `json:"style,omitempty"`
	FirstRow bool            `json:"firstRow,omitempty"`
	FirstCol bool            `json:"firstCol,omitempty"`
	LastRow  bool            `json:"lastRow,omitempty"`
	LastCol  bool            `json:"lastCol,omitempty"`
	BandRow  bool            `json:"bandRow,omitempty"`
	BandCol  bool            `json:"bandCol,omitempty"`
	RTL      bool            `json:"rtl,omitempty"`
	Fill     *drawingml.Fill `json:"fill,omitempty"`
}

var (
	tblPrOrder = []string{"noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "effectLst", "effectDag", "tableStyle", "tableStyleId", "extLst"}
	tcPrOrder  = []string{"lnL", "lnR", "lnT", "lnB", "lnTlToBr", "lnBlToTr", "cell3D", "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "headers", "extLst"}
	cellLines  = []string{"lnL", "lnR", "lnT", "lnB", "lnTlToBr", "lnBlToTr"}
	margins    = map[string]string{"l": "marL", "r": "marR", "t": "marT", "b": "marB"}
)

func readTable(tblPr *xmldom.Element, media func(string) string) *Table {
	if tblPr == nil {
		return nil
	}
	t := &Table{Fill: drawingml.FillIn(tblPr, media)}
	if id := tblPr.Child(aNS, "tableStyleId"); id != nil {
		t.Style = id.Text()
	}
	for f, p := range t.flags() {
		*p = isOn(tblPr.Get(f))
	}
	return t
}

// flags are the options of the table, by the attribute of the tblPr.
func (t *Table) flags() map[string]*bool {
	return map[string]*bool{"rtl": &t.RTL, "firstRow": &t.FirstRow, "firstCol": &t.FirstCol, "lastRow": &t.LastRow, "lastCol": &t.LastCol, "bandRow": &t.BandRow, "bandCol": &t.BandCol}
}

func isOn(s string) bool { return s == "1" || s == "true" }

// tableOf is the a:tbl of a graphic frame, nil when it shows none.
func tableOf(frame *xmldom.Element) *xmldom.Element {
	g := frame.Child(aNS, "graphic")
	if g == nil {
		return nil
	}
	gd := g.Child(aNS, "graphicData")
	if gd == nil || gd.Get("uri") != tableURI {
		return nil
	}
	return gd.Child(aNS, "tbl")
}

func readGrid(tbl *xmldom.Element) []int64 {
	var out []int64
	for _, c := range elements(tbl.Child(aNS, "tblGrid"), aNS, "gridCol") {
		w, _ := strconv.ParseInt(c.Get("w"), 10, 64)
		out = append(out, max(w, 0))
	}
	return out
}

// table reads the rows and cells of a table frame under it, and takes
// them out of its XML.
func (r *reader) table(tbl *xmldom.Element, frame string, attrs ot.Values) {
	t := readTable(tbl.Child(aNS, "tblPr"), r.d.media)
	if t != nil && t.Style != "" {
		r.tableStyles[t.Style] = true
	}
	putJSON(attrs, "tbl", t)
	putJSON(attrs, "grid", readGrid(tbl))
	rows := elements(tbl, aNS, "tr")
	keys := ot.Keys(len(rows))
	for i, tr := range rows {
		id := frame + "-r" + strconv.Itoa(i+1)
		h, _ := strconv.ParseInt(tr.Get("h"), 10, 64)
		row := ot.Values{"h": json.RawMessage(strconv.FormatInt(max(h, 0), 10))}
		at := len(r.nodes)
		r.add(id, "tr", frame, keys[i], nil, nil)
		cells := elements(tr, aNS, "tc")
		ckeys := ot.Keys(len(cells))
		for j, tc := range cells {
			r.cell(tc, id+"-c"+strconv.Itoa(j+1), id, ckeys[j])
			tr.Remove(tc)
		}
		row["xml"] = r.d.raw(tr)
		r.nodes[at].Attrs = row
		tbl.Remove(tr)
	}
}

func (r *reader) cell(tc *xmldom.Element, id, parent, key string) {
	attrs := cellProps(tc, r.d.media)
	text := ot.Delta{{Insert: "\n"}}
	if body := tc.Child(aNS, "txBody"); body != nil {
		putJSON(attrs, "lst", drawingml.ListStyle(body.Child(aNS, "lstStyle")))
		text = r.flow(body)
		for _, p := range elements(body, aNS, "p") {
			body.Remove(p)
		}
	}
	attrs["xml"] = r.d.raw(tc)
	r.add(id, "tc", parent, key, attrs, text)
}

// cellProps reads the spans of a cell and its tcPr.
func cellProps(tc *xmldom.Element, media func(string) string) ot.Values {
	v := ot.Values{}
	for _, k := range []string{"gridSpan", "rowSpan"} {
		if n, err := strconv.ParseInt(tc.Get(k), 10, 64); err == nil && n > 1 {
			v[k] = json.RawMessage(strconv.FormatInt(n, 10))
		}
	}
	for _, k := range []string{"hMerge", "vMerge"} {
		if isOn(tc.Get(k)) {
			v[k] = json.RawMessage("true")
		}
	}
	pr := tc.Child(aNS, "tcPr")
	if pr == nil {
		return v
	}
	putJSON(v, "fill", drawingml.FillIn(pr, media))
	for _, k := range cellLines {
		putJSON(v, k, drawingml.ReadLine(pr.Child(aNS, k)))
	}
	mar := map[string]int64{}
	for k, attr := range margins {
		if n, err := strconv.ParseInt(pr.Get(attr), 10, 64); err == nil {
			mar[k] = n
		}
	}
	putJSON(v, "mar", mar)
	putString(v, "anchor", pr.Get("anchor"))
	return v
}

// table writes the rows of a table frame into its a:tbl. Rows cover as
// many columns as the grid has: a row created while a column was, or the
// other way around, is filled with empty cells.
func (w *writer) table(frame *xmldom.Element, n *ot.Node) {
	tbl := tableOf(frame)
	if tbl == nil {
		return
	}
	for _, tr := range elements(tbl, aNS, "tr") {
		tbl.Remove(tr)
	}
	pr := tbl.Child(aNS, "tblPr")
	if pr == nil {
		pr = xmldom.New(aNS, "a:tblPr")
		tbl.Content = append([]xmldom.Node{pr}, tbl.Content...)
	}
	if old := mustJSON(readTable(pr, w.d.media)); !bytes.Equal(old, n.Attrs["tbl"]) {
		w.setTable(pr, n.Attrs["tbl"])
	}
	rows := w.tree.Children(n.ID)
	grid := readGrid(tbl)
	var want []int64
	if json.Unmarshal(n.Attrs["grid"], &want) != nil {
		want = grid
	}
	cols := len(want)
	for _, row := range rows {
		cols = max(cols, width(w.tree.Children(row.ID)))
	}
	for len(want) < cols {
		width := int64(914400)
		if len(want) > 0 {
			width = want[len(want)-1]
		}
		want = append(want, width)
	}
	if !slices.Equal(grid, want) {
		g := tbl.Child(aNS, "tblGrid")
		if g == nil {
			g = xmldom.New(aNS, "a:tblGrid")
			tbl.Insert(g, []string{"tblPr", "tblGrid"})
		}
		have := elements(g, aNS, "gridCol")
		for i, width := range want {
			if i == len(have) {
				have = append(have, xmldom.New(aNS, "a:gridCol"))
				g.Append(have[i])
			}
			have[i].Set("w", strconv.FormatInt(max(width, 0), 10))
		}
		for _, c := range have[len(want):] {
			g.Remove(c)
		}
	}
	for i, row := range rows {
		tbl.Append(w.row(row, cols, len(rows)-i))
	}
}

func (w *writer) setTable(pr *xmldom.Element, value json.RawMessage) {
	var t Table
	_ = json.Unmarshal(value, &t)
	for f, on := range t.flags() {
		if *on {
			pr.Set(f, "1")
		} else {
			pr.Unset(f)
		}
	}
	drawingml.SetFill(pr, t.Fill, w.embed, tblPrOrder)
	if s := pr.Child(aNS, "tableStyle"); s != nil && t.Style != "" {
		pr.Remove(s)
	}
	id := pr.Child(aNS, "tableStyleId")
	switch {
	case t.Style == "" && id != nil:
		pr.Remove(id)
	case t.Style != "":
		if id == nil {
			id = xmldom.New(aNS, "a:tableStyleId")
			pr.Insert(id, tblPrOrder)
		}
		id.Content = []xmldom.Node{xmldom.EscapeText(t.Style)}
	}
}

// row writes a row with cols cells; left is how many rows it ends, itself
// included, which bounds the spans of its cells.
func (w *writer) row(n *ot.Node, cols, left int) *xmldom.Element {
	tr := w.d.fragment(str(n, "xml"))
	if tr == nil || tr.Space != aNS || tr.Local != "tr" {
		tr = xmldom.New(aNS, "a:tr")
	}
	for _, tc := range elements(tr, aNS, "tc") {
		tr.Remove(tc)
	}
	var h int64
	_ = json.Unmarshal(n.Attrs["h"], &h)
	tr.Set("h", strconv.FormatInt(max(h, 0), 10))
	var at []xmldom.Node
	col, covered := 0, 0
	for _, c := range w.tree.Children(n.ID) {
		if covered > 0 && merged(c) {
			covered--
			at = append(at, w.cell(c, 1, left))
			continue
		}
		at = append(at, w.cell(c, cols-col, left))
		col += span(c)
		covered = span(c) - 1
	}
	for ; col < cols; col++ {
		at = append(at, newCell())
	}
	tr.Content = append(at, tr.Content...)
	return tr
}

func span(n *ot.Node) int {
	var s int
	_ = json.Unmarshal(n.Attrs["gridSpan"], &s)
	return max(s, 1)
}

func merged(n *ot.Node) bool { return string(n.Attrs["hMerge"]) == "true" }

// width is the number of columns the cells of a row take: those a span
// covers are hMerge, or missing.
func width(cells []*ot.Node) int {
	n, covered := 0, 0
	for _, c := range cells {
		if covered > 0 && merged(c) {
			covered--
			continue
		}
		n += span(c)
		covered = span(c) - 1
	}
	return n
}

func newCell() *xmldom.Element {
	e, _ := xmldom.ParseFragment([]byte(`<a:tc><a:txBody><a:bodyPr/><a:lstStyle/><a:p><a:endParaRPr lang="fr-FR"/></a:p></a:txBody><a:tcPr/></a:tc>`), standardSpaces())
	return e
}

// cell writes a cell whose spans may cover cols columns and rows rows.
func (w *writer) cell(n *ot.Node, cols, rows int) *xmldom.Element {
	tc := w.d.fragment(str(n, "xml"))
	if tc == nil || tc.Space != aNS || tc.Local != "tc" {
		tc = newCell()
	}
	old := cellProps(tc, w.d.media)
	for k, limit := range map[string]int{"gridSpan": cols, "rowSpan": rows} {
		var span int
		_ = json.Unmarshal(n.Attrs[k], &span)
		if span = min(span, limit); span > 1 {
			tc.Set(k, strconv.Itoa(span))
		} else {
			tc.Unset(k)
		}
	}
	for _, k := range []string{"hMerge", "vMerge"} {
		if string(n.Attrs[k]) == "true" {
			tc.Set(k, "1")
		} else {
			tc.Unset(k)
		}
	}
	pr := tc.Child(aNS, "tcPr")
	if pr == nil {
		pr = xmldom.New(aNS, "a:tcPr")
		tc.Insert(pr, []string{"txBody", "tcPr", "extLst"})
	}
	w.setCellProps(pr, old, n.Attrs)
	if n.Text == nil {
		return tc
	}
	flow := n.Text.Delta()
	body := tc.Child(aNS, "txBody")
	if body == nil {
		if len(flow) == 1 && flow[0].Insert == "\n" && flow[0].Attrs == nil {
			return tc
		}
		body = xmldom.New(aNS, "a:txBody")
		body.Append(xmldom.New(aNS, "a:bodyPr"))
		body.Append(xmldom.New(aNS, "a:lstStyle"))
		tc.Content = append([]xmldom.Node{body}, tc.Content...)
	}
	drawingml.SetFlow(body, flow, w.d.fragment)
	return tc
}

func (w *writer) setCellProps(pr *xmldom.Element, old, values ot.Values) {
	if changed(old, values, "fill") {
		var f *drawingml.Fill
		if json.Unmarshal(values["fill"], &f) != nil {
			f = nil
		}
		drawingml.SetFill(pr, f, w.embed, tcPrOrder)
	}
	for _, k := range cellLines {
		if !changed(old, values, k) {
			continue
		}
		e := pr.Child(aNS, k)
		var l drawingml.Line
		if values[k] == nil || json.Unmarshal(values[k], &l) != nil {
			if e != nil {
				pr.Remove(e)
			}
			continue
		}
		if e == nil {
			e = xmldom.New(aNS, "a:"+k)
			pr.Insert(e, tcPrOrder)
			drawingml.SetLine(e, nil, &l)
		} else {
			drawingml.SetLine(e, drawingml.ReadLine(e), &l)
		}
	}
	if changed(old, values, "mar") {
		var mar map[string]int64
		_ = json.Unmarshal(values["mar"], &mar)
		for k, attr := range margins {
			if v, ok := mar[k]; ok {
				pr.Set(attr, strconv.FormatInt(max(v, 0), 10))
			} else {
				pr.Unset(attr)
			}
		}
	}
	if changed(old, values, "anchor") {
		var a string
		_ = json.Unmarshal(values["anchor"], &a)
		switch a {
		case "t", "ctr", "b", "just", "dist":
			pr.Set("anchor", a)
		default:
			pr.Unset("anchor")
		}
	}
}

// newTableFrame is a graphic frame for a table created by a client.
func newTableFrame() *xmldom.Element {
	e, _ := xmldom.ParseFragment([]byte(`<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="0" name=""/>`+
		`<p:cNvGraphicFramePr><a:graphicFrameLocks noGrp="1"/></p:cNvGraphicFramePr><p:nvPr/></p:nvGraphicFramePr>`+
		`<p:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></p:xfrm>`+
		`<a:graphic><a:graphicData uri="`+tableURI+`"><a:tbl><a:tblPr/><a:tblGrid/></a:tbl></a:graphicData></a:graphic></p:graphicFrame>`), standardSpaces())
	return e
}
