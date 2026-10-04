package docx

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// Attributes of tables, rows and cells, each a JSON value; widths are in
// twentieths of a point unless their type says otherwise:
//
//	tbl  style, w {"w":5000,"type":"pct"}, jc, ind {"w","type"}, borders
//	     (sides as Border), layout "fixed", mar {"top","left","bottom","right"},
//	     look {"firstRow":true,…}, spacing, shd, float (tblpPr, read only), grid [widths]
//	tr   h, hRule, cantSplit, header, before and after (grid columns left empty)
//	tc   w, span, vMerge "restart" or "continue", hMerge, borders, shd,
//	     vAlign, mar, noWrap, dir
type tprop struct {
	key   string
	read  func(e *xmldom.Element) any
	write func(e *xmldom.Element, v json.RawMessage)
}

var tblPrOrder = []string{
	"tblStyle", "tblpPr", "tblOverlap", "bidiVisual", "tblStyleRowBandSize", "tblStyleColBandSize", "tblW",
	"jc", "tblCellSpacing", "tblInd", "tblBorders", "shd", "tblLayout", "tblCellMar", "tblLook",
	"tblCaption", "tblDescription", "tblPrChange",
}

var trPrOrder = []string{
	"cnfStyle", "divId", "gridBefore", "gridAfter", "wBefore", "wAfter", "cantSplit", "trHeight",
	"tblHeader", "tblCellSpacing", "jc", "hidden", "ins", "del", "trPrChange",
}

var tcPrOrder = []string{
	"cnfStyle", "tcW", "gridSpan", "hMerge", "vMerge", "tcBorders", "shd", "noWrap", "tcMar",
	"textDirection", "tcFitText", "vAlign", "hideMark", "headers", "cellIns", "cellDel", "cellMerge",
	"tcPrChange",
}

// Width is a measure of a table or cell.
type Width struct {
	W    int    `json:"w"`
	Type string `json:"type,omitempty"`
}

var widthTypes = enum("nil", "pct", "dxa", "auto")

func widthProp(key, local string, order []string) tprop {
	return tprop{key,
		func(e *xmldom.Element) any {
			c := child(e, local)
			if c == nil {
				return nil
			}
			w := Width{Type: attr(c, "type")}
			v := attr(c, "w")
			if n, err := strconv.Atoi(v); err == nil {
				w.W = n
			} else if f, err := strconv.ParseFloat(v[:max(len(v)-1, 0)], 64); err == nil && len(v) > 0 && v[len(v)-1] == '%' {
				w.W, w.Type = int(f*50), "pct"
			}
			return w
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, local)
			var w Width
			if json.Unmarshal(v, &w) != nil || w.Type != "" && !widthTypes(w.Type) || w.W < -31680 || w.W > 31680*20 {
				return
			}
			c := newW(e, local, "w", strconv.Itoa(w.W))
			if w.Type != "" {
				setAttr(c, "type", w.Type)
			}
			insert(e, c, order)
		}}
}

// valTprop is the val of a child, as a string.
func valTprop(key, local string, order []string, valid func(string) bool) tprop {
	return tprop{key,
		func(e *xmldom.Element) any {
			if c := child(e, local); c != nil {
				return attr(c, "val")
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, local)
			var s string
			if json.Unmarshal(v, &s) == nil && valid(s) {
				insert(e, newW(e, local, "val", s), order)
			}
		}}
}

// intTprop is the val of a child, as a number.
func intTprop(key, local string, order []string, lo, hi int) tprop {
	return tprop{key,
		func(e *xmldom.Element) any {
			if n, err := strconv.Atoi(attr(child(e, local), "val")); err == nil {
				return n
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, local)
			var n int
			if json.Unmarshal(v, &n) == nil && lo <= n && n <= hi {
				insert(e, newW(e, local, "val", strconv.Itoa(n)), order)
			}
		}}
}

// flagTprop is an on/off child, as a boolean.
func flagTprop(key, local string, order []string) tprop {
	return tprop{key,
		func(e *xmldom.Element) any {
			switch on(child(e, local)) {
			case "1":
				return true
			case "0":
				return false
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, local)
			switch string(v) {
			case "true":
				insert(e, newW(e, local), order)
			case "false":
				insert(e, newW(e, local, "val", "0"), order)
			}
		}}
}

func bordersTprop(key, local string, sides, order []string) tprop {
	return tprop{key,
		func(e *xmldom.Element) any {
			if b := Borders(child(e, local)); b != nil {
				return b
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, local)
			var b map[string]Border
			if json.Unmarshal(v, &b) != nil || len(b) == 0 {
				return
			}
			c := newW(e, local)
			setBorders(c, b, sides)
			if len(c.Content) > 0 {
				insert(e, c, order)
			}
		}}
}

func shdTprop(order []string) tprop {
	p := shdProp("shd", order)
	return tprop{"shd",
		func(e *xmldom.Element) any {
			if v := p.read(e); v != "" {
				return v
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			var s string
			_ = json.Unmarshal(v, &s)
			p.write(e, s)
		}}
}

// marginsTprop are the margins of cells, in twentieths of a point.
func marginsTprop(key, local string, order []string) tprop {
	sides := []string{"top", "left", "start", "bottom", "right", "end"}
	return tprop{key,
		func(e *xmldom.Element) any {
			c := child(e, local)
			if c == nil {
				return nil
			}
			out := map[string]int{}
			for _, s := range c.Elements() {
				if n, err := strconv.Atoi(attr(s, "w")); err == nil && s.Space == NS {
					switch s.Local {
					case "start":
						out["left"] = n
					case "end":
						out["right"] = n
					default:
						out[s.Local] = n
					}
				}
			}
			return out
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, local)
			var m map[string]int
			if json.Unmarshal(v, &m) != nil {
				return
			}
			c := newW(e, local)
			for _, s := range sides {
				if n, ok := m[s]; ok && 0 <= n && n <= 31680 {
					c.Append(newW(c, s, "w", strconv.Itoa(n), "type", "dxa"))
				}
			}
			if len(c.Content) > 0 {
				insert(e, c, order)
			}
		}}
}

var lookBits = []struct {
	name string
	bit  int
}{{"firstRow", 0x20}, {"lastRow", 0x40}, {"firstColumn", 0x80}, {"lastColumn", 0x100}, {"noHBand", 0x200}, {"noVBand", 0x400}}

var lookTprop = tprop{"look",
	func(e *xmldom.Element) any {
		c := child(e, "tblLook")
		if c == nil {
			return nil
		}
		out := map[string]bool{}
		bits, err := strconv.ParseInt(attr(c, "val"), 16, 32)
		for _, l := range lookBits {
			if v := attr(c, l.name); v != "" {
				out[l.name] = truthy(v)
			} else if err == nil {
				out[l.name] = int(bits)&l.bit != 0
			}
		}
		return out
	},
	func(e *xmldom.Element, v json.RawMessage) {
		remove(e, "tblLook")
		var look map[string]bool
		if json.Unmarshal(v, &look) != nil {
			return
		}
		c := newW(e, "tblLook")
		bits := 0
		for _, l := range lookBits {
			if look[l.name] {
				bits |= l.bit
			}
		}
		setAttr(c, "val", fmt.Sprintf("%04X", bits))
		for _, l := range lookBits {
			setAttr(c, l.name, map[bool]string{true: "1", false: "0"}[look[l.name]])
		}
		insert(e, c, tblPrOrder)
	}}

// floatTprop is where a floating table sits, read only.
var floatTprop = tprop{"float",
	func(e *xmldom.Element) any {
		c := child(e, "tblpPr")
		if c == nil {
			return nil
		}
		out := map[string]string{}
		for _, a := range c.Attrs {
			if _, local, ok := strings.Cut(a.Name, ":"); ok {
				out[local] = a.Value
			}
		}
		return out
	},
	nil}

var borderSides = []string{"top", "left", "start", "bottom", "right", "end", "insideH", "insideV", "tl2br", "tr2bl"}

var tblProps = []tprop{
	valTprop("style", "tblStyle", tblPrOrder, name),
	floatTprop,
	widthProp("w", "tblW", tblPrOrder),
	valTprop("jc", "jc", tblPrOrder, enum("start", "center", "end", "left", "right")),
	widthProp("spacing", "tblCellSpacing", tblPrOrder),
	widthProp("ind", "tblInd", tblPrOrder),
	bordersTprop("borders", "tblBorders", borderSides[:8], tblPrOrder),
	shdTprop(tblPrOrder),
	tprop{"layout",
		func(e *xmldom.Element) any {
			if c := child(e, "tblLayout"); c != nil {
				return attr(c, "type")
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, "tblLayout")
			if string(v) == `"fixed"` || string(v) == `"autofit"` {
				insert(e, newW(e, "tblLayout", "type", string(v[1:len(v)-1])), tblPrOrder)
			}
		}},
	marginsTprop("mar", "tblCellMar", tblPrOrder),
	lookTprop,
	intTprop("rowBand", "tblStyleRowBandSize", tblPrOrder, 0, 100),
	intTprop("colBand", "tblStyleColBandSize", tblPrOrder, 0, 100),
}

var trProps = []tprop{
	intTprop("before", "gridBefore", trPrOrder, 0, 63),
	intTprop("after", "gridAfter", trPrOrder, 0, 63),
	flagTprop("cantSplit", "cantSplit", trPrOrder),
	tprop{"h",
		func(e *xmldom.Element) any {
			if n, err := strconv.Atoi(attr(child(e, "trHeight"), "val")); err == nil {
				return n
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			c := child(e, "trHeight")
			var n int
			if json.Unmarshal(v, &n) != nil || n < 0 || n > 31680 {
				if c != nil {
					e.Remove(c)
				}
				return
			}
			if c == nil {
				c = newW(e, "trHeight")
				insert(e, c, trPrOrder)
			}
			setAttr(c, "val", strconv.Itoa(n))
		}},
	tprop{"hRule",
		func(e *xmldom.Element) any {
			if c := child(e, "trHeight"); c != nil && attr(c, "hRule") != "" {
				return attr(c, "hRule")
			}
			return nil
		},
		func(e *xmldom.Element, v json.RawMessage) {
			c := child(e, "trHeight")
			if c == nil {
				return
			}
			var s string
			if json.Unmarshal(v, &s) == nil && (s == "exact" || s == "atLeast" || s == "auto") {
				setAttr(c, "hRule", s)
			} else {
				unsetAttr(c, "hRule")
			}
		}},
	flagTprop("header", "tblHeader", trPrOrder),
}

var tcProps = []tprop{
	widthProp("w", "tcW", tcPrOrder),
	intTprop("span", "gridSpan", tcPrOrder, 1, 63),
	tprop{"vMerge",
		func(e *xmldom.Element) any {
			c := child(e, "vMerge")
			if c == nil {
				return nil
			}
			if attr(c, "val") == "restart" {
				return "restart"
			}
			return "continue"
		},
		func(e *xmldom.Element, v json.RawMessage) {
			remove(e, "vMerge")
			switch string(v) {
			case `"restart"`:
				insert(e, newW(e, "vMerge", "val", "restart"), tcPrOrder)
			case `"continue"`:
				insert(e, newW(e, "vMerge"), tcPrOrder)
			}
		}},
	valTprop("hMerge", "hMerge", tcPrOrder, enum("restart", "continue")),
	bordersTprop("borders", "tcBorders", borderSides, tcPrOrder),
	shdTprop(tcPrOrder),
	flagTprop("noWrap", "noWrap", tcPrOrder),
	marginsTprop("mar", "tcMar", tcPrOrder),
	valTprop("dir", "textDirection", tcPrOrder, enum("lrTb", "tbRl", "btLr", "lrTbV", "tbRlV", "tbLrV", "tb", "rl", "lr", "tbV", "rlV", "lrV")),
	valTprop("vAlign", "vAlign", tcPrOrder, enum("top", "center", "bottom", "both")),
}

// readTprops reads the attributes of a table, row or cell from its
// properties.
func readTprops(e *xmldom.Element, list []tprop, into map[string]json.RawMessage) {
	if e == nil {
		return
	}
	for _, p := range list {
		if v := p.read(e); v != nil {
			data, _ := json.Marshal(v)
			into[p.key] = data
		}
	}
}

// writeTprops patches the properties e for the attributes that changed.
func writeTprops(e *xmldom.Element, list []tprop, old, attrs ot.Values) {
	for _, p := range list {
		if p.write != nil && string(old[p.key]) != string(attrs[p.key]) {
			p.write(e, attrs[p.key])
		}
	}
}

func (r *reader) table(e *xmldom.Element, parent, key string) {
	attrs := ot.Values{"xml": r.d.rawValue(shell(e, func(c *xmldom.Element) bool {
		return c.Space == NS && (c.Local == "tblPr" || c.Local == "tblGrid")
	}))}
	readTprops(child(e, "tblPr"), tblProps, attrs)
	if g := grid(child(e, "tblGrid")); g != nil {
		attrs["grid"], _ = json.Marshal(g)
	}
	id := r.add("tbl", parent, key, attrs, nil)
	rows := e.Elements()
	rows = slices.DeleteFunc(rows, func(c *xmldom.Element) bool {
		return c.Space == NS && (c.Local == "tblPr" || c.Local == "tblGrid")
	})
	keys := ot.Keys(len(rows))
	for i, tr := range rows {
		if tr.Space != NS || tr.Local != "tr" {
			r.add("other", id, keys[i], ot.Values{"xml": r.d.rawValue(tr)}, nil)
			continue
		}
		r.row(tr, id, keys[i])
	}
}

// grid is the width of the columns of a table, nil when it gives none.
func grid(e *xmldom.Element) []int {
	if e == nil {
		return nil
	}
	var out []int
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "gridCol" {
			n, _ := strconv.Atoi(attr(c, "w"))
			out = append(out, n)
		}
	}
	return out
}

func (r *reader) row(e *xmldom.Element, parent, key string) {
	attrs := ot.Values{"xml": r.d.rawValue(shell(e, func(c *xmldom.Element) bool {
		return c.Space == NS && (c.Local == "trPr" || c.Local == "tblPrEx")
	}))}
	readTprops(child(e, "trPr"), trProps, attrs)
	id := r.add("tr", parent, key, attrs, nil)
	cells := slices.DeleteFunc(e.Elements(), func(c *xmldom.Element) bool {
		return c.Space == NS && (c.Local == "trPr" || c.Local == "tblPrEx")
	})
	keys := ot.Keys(len(cells))
	for i, tc := range cells {
		if tc.Space != NS || tc.Local != "tc" {
			r.add("other", id, keys[i], ot.Values{"xml": r.d.rawValue(tc)}, nil)
			continue
		}
		attrs := ot.Values{"xml": r.d.rawValue(shell(tc, func(c *xmldom.Element) bool {
			return c.Space == NS && c.Local == "tcPr"
		}))}
		readTprops(child(tc, "tcPr"), tcProps, attrs)
		cell := r.add("tc", id, keys[i], attrs, nil)
		r.blocks(tc, cell)
	}
}
