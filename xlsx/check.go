package xlsx

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf16"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/ot"
)

var ErrReadOnly = errors.New("xlsx: this part of the workbook cannot be edited")

// The limits of Excel on what a cell holds.
const (
	maxText    = 32767
	maxFormula = 8192
)

// editable are the attributes a change may set, by node type.
var editable = map[string]map[string]bool{
	"book":  {"names": true},
	"sheet": {"name": true, "state": true, "frozen": true, "grid": true, "rtl": true, "zoom": true, "tab": true, "dw": true, "dh": true, "filter": true},
	"kept":  {"name": true, "state": true},
}

// Check tells whether an edit only changes what can be edited: sheets,
// their settings and cells, and cell formats added to those of the
// workbook. An edit that inserts or removes rows or columns does nothing
// else, so that the formulas it moves are those of the workbook before it.
func (d *Document) Check(tree *ot.Tree, e ot.Edit) error {
	created := map[string]string{}
	typeOf := func(id string) string {
		if t, ok := created[id]; ok {
			return t
		}
		if n := tree.Node(id); n != nil {
			return n.Type
		}
		return ""
	}
	shifts := 0
	for _, c := range e {
		if c.Op == ot.OpIns || c.Op == ot.OpRem {
			shifts++
		}
	}
	if shifts > 0 && shifts < len(e) {
		return ErrReadOnly
	}
	for _, c := range e {
		t := typeOf(c.ID)
		switch c.Op {
		case ot.OpNew:
			switch {
			case c.Type == "sheet" && c.Parent == "book":
				if c.Cells == nil || !d.checkAttrs("sheet", c.Attrs) || !d.checkCells(typeOf, c.Cells) {
					return ErrReadOnly
				}
			case c.Type == "xf" && c.Parent == "":
				if !d.checkStyle(tree, c.Attrs) {
					return ErrReadOnly
				}
			default:
				return ErrReadOnly
			}
			created[c.ID] = c.Type
		case ot.OpDel:
			if t != "" && t != "sheet" && t != "kept" {
				return ErrReadOnly
			}
		case ot.OpSet:
			if t == "" {
				continue
			}
			if c.Key != "" && t != "sheet" && t != "kept" || !d.checkAttrs(t, c.Attrs) {
				return ErrReadOnly
			}
			if names, ok := c.Attrs["names"]; ok && !checkNames(typeOf, names) {
				return ErrReadOnly
			}
		case ot.OpCel:
			if t != "" && (t != "sheet" || !d.checkCells(typeOf, c.Cells)) {
				return ErrReadOnly
			}
		case ot.OpIns, ot.OpRem:
			if t != "" && t != "sheet" {
				return ErrReadOnly
			}
			if err := d.checkTables(c); err != nil {
				return err
			}
		default:
			return ErrReadOnly
		}
	}
	return nil
}

func (d *Document) checkAttrs(typ string, attrs ot.Values) bool {
	for k, v := range attrs {
		if !editable[typ][k] {
			return false
		}
		if bytes.Equal(v, []byte("null")) {
			continue
		}
		var ok bool
		switch k {
		case "name":
			var s string
			ok = json.Unmarshal(v, &s) == nil && s != "" && len(utf16.Encode([]rune(s))) <= 31
		case "state":
			var s string
			ok = json.Unmarshal(v, &s) == nil && (s == "hidden" || s == "veryHidden")
		case "frozen":
			var f frozen
			ok = json.Unmarshal(v, &f) == nil && f.Rows >= 0 && f.Rows < formula.MaxRows && f.Cols >= 0 && f.Cols < formula.MaxCols
		case "grid", "rtl":
			var b bool
			ok = json.Unmarshal(v, &b) == nil
		case "zoom":
			var z int
			ok = json.Unmarshal(v, &z) == nil && z >= 10 && z <= 400
		case "tab":
			var c Color
			ok = json.Unmarshal(v, &c) == nil && validColor(&c)
		case "dw":
			var w float64
			ok = json.Unmarshal(v, &w) == nil && w > 0 && w <= maxWidth
		case "dh":
			var h float64
			ok = json.Unmarshal(v, &h) == nil && h > 0 && h <= maxHeight
		case "names":
			ok = true
		case filterKey:
			ok = checkFilter(v)
		}
		if !ok {
			return false
		}
	}
	return true
}

// checkNames tells whether names are names Excel takes, each defined once.
func checkNames(typeOf func(string) string, raw json.RawMessage) bool {
	var names []Name
	if json.Unmarshal(raw, &names) != nil {
		return false
	}
	seen := map[[2]string]bool{}
	for _, n := range names {
		k := [2]string{strings.ToLower(n.Name), n.Sheet}
		if seen[k] || !validName(n.Name) || n.Sheet != "" && typeOf(n.Sheet) != "sheet" {
			return false
		}
		if _, err := formula.Tokens(n.Ref); err != nil || n.Ref == "" || len(n.Ref) > maxFormula {
			return false
		}
		seen[k] = true
	}
	return true
}

// validName tells whether a defined name is one Excel takes: a letter, "_"
// or "\" first, then letters, digits, "_", "." and "\", and not a cell.
func validName(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	tokens, err := formula.Tokens(s)
	if err != nil || len(tokens) != 1 || tokens[0].Kind != formula.Name || strings.ContainsAny(s, "![]'") {
		return false
	}
	c := s[0]
	return c == '_' || c == '\\' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= 0x80
}

// checkCells tells whether cells hold only what a workbook can: in row 0
// the fields of columns, in column 0 those of rows.
func (d *Document) checkCells(typeOf func(string) string, cells []ot.Cell) bool {
	for _, c := range cells {
		var f map[string]json.RawMessage
		if json.Unmarshal(c.Fields, &f) != nil {
			return false
		}
		for k, v := range f {
			if bytes.Equal(v, []byte("null")) {
				if !slices.Contains(cellKeys, k) && !slices.Contains(lineKeys, k) && !(d.csv && k == "src") {
					return false
				}
				continue
			}
			var ok bool
			switch {
			case c.Row == 0 && c.Col == 0:
			case c.Row == 0 || c.Col == 0:
				ok = checkLine(k, v, c.Row == 0, typeOf)
			default:
				ok = d.checkCell(k, v, typeOf)
			}
			if !ok {
				return false
			}
		}
	}
	return true
}

var (
	cellKeys = []string{"ca", "cm", "e", "f", "fa", "fx", "m", "rich", "s", "v", "vm"}
	lineKeys = []string{"bf", "ch", "cl", "cw", "h", "hide", "ol", "s", "w"}
)

func checkLine(k string, v json.RawMessage, column bool, typeOf func(string) string) bool {
	var b bool
	var n float64
	switch k {
	case "hide", "cl":
		return json.Unmarshal(v, &b) == nil
	case "bf", "cw":
		return column && json.Unmarshal(v, &b) == nil
	case "ch":
		return !column && json.Unmarshal(v, &b) == nil
	case "w":
		return column && json.Unmarshal(v, &n) == nil && n > 0 && n <= maxWidth
	case "h":
		return !column && json.Unmarshal(v, &n) == nil && n > 0 && n <= maxHeight
	case "ol":
		var i int
		return json.Unmarshal(v, &i) == nil && i >= 0 && i <= maxOutline
	case "s":
		var s string
		return json.Unmarshal(v, &s) == nil && typeOf(s) == "xf"
	}
	return false
}

func (d *Document) checkCell(k string, v json.RawMessage, typeOf func(string) string) bool {
	var s string
	switch k {
	case "v":
		switch v[0] {
		case '"':
			return json.Unmarshal(v, &s) == nil && len(utf16.Encode([]rune(s))) <= maxText
		case 't', 'f':
			var b bool
			return json.Unmarshal(v, &b) == nil
		case '{', '[':
			return false
		}
		var n float64
		return json.Unmarshal(v, &n) == nil
	case "e":
		return json.Unmarshal(v, &s) == nil && formula.IsError(s)
	case "f":
		if json.Unmarshal(v, &s) != nil || s == "" || len(s) > maxFormula {
			return false
		}
		_, err := formula.Tokens(s)
		return err == nil
	case "fa":
		a, ok := formula.ParseArea(unquote(v))
		return ok && !a.Rows && !a.Cols
	case "ca":
		var b bool
		return json.Unmarshal(v, &b) == nil
	case "m":
		var m [2]int
		return json.Unmarshal(v, &m) == nil && m[0] >= 1 && m[1] >= 1 && m[0] <= formula.MaxRows && m[1] <= formula.MaxCols
	case "s":
		return typeOf(unquote(v)) == "xf"
	case "rich", "fx":
		return json.Unmarshal(v, &s) == nil && d.isTrusted(s)
	case "src":
		return d.csv && json.Unmarshal(v, &s) == nil && len(s) <= 2*maxText+2
	}
	return false
}

func unquote(v json.RawMessage) string {
	var s string
	_ = json.Unmarshal(v, &s)
	return s
}

// checkStyle tells whether a cell format is one a workbook can hold,
// derived from one it had.
func (d *Document) checkStyle(tree *ot.Tree, attrs ot.Values) bool {
	var st Style
	if len(attrs) != 1 || json.Unmarshal(attrs["style"], &st) != nil {
		return false
	}
	if st.Base != "" && (d.loaded.Node(st.Base) == nil || d.loaded.Node(st.Base).Type != "xf") {
		return false
	}
	return st.valid()
}

var (
	patterns = []string{"none", "solid", "mediumGray", "darkGray", "lightGray", "darkHorizontal", "darkVertical",
		"darkDown", "darkUp", "darkGrid", "darkTrellis", "lightHorizontal", "lightVertical", "lightDown", "lightUp",
		"lightGrid", "lightTrellis", "gray125", "gray0625"}
	borderStyles = []string{"none", "thin", "medium", "dashed", "dotted", "thick", "double", "hair", "mediumDashed",
		"dashDot", "mediumDashDot", "dashDotDot", "mediumDashDotDot", "slantDashDot"}
	horizontal = []string{"", "general", "left", "center", "right", "fill", "justify", "centerContinuous", "distributed"}
	vertical   = []string{"", "top", "center", "bottom", "justify", "distributed"}
	underlines = []string{"", "single", "double", "singleAccounting", "doubleAccounting"}
)

// valid tells whether the format holds only what the schema of a workbook
// allows.
func (st Style) valid() bool {
	if len(st.Format) > 255 {
		return false
	}
	if f := st.Font; f != nil {
		if len(utf16.Encode([]rune(f.Name))) > 31 || f.Size < 0 || f.Size > 409 || !slices.Contains(underlines, f.Underline) ||
			!slices.Contains([]string{"", "superscript", "subscript"}, f.VertAlign) ||
			!slices.Contains([]string{"", "major", "minor"}, f.Scheme) || f.Color != nil && !validColor(f.Color) {
			return false
		}
	}
	if f := st.Fill; f != nil {
		if f.Pattern != "" && !slices.Contains(patterns, f.Pattern) || f.Fg != nil && !validColor(f.Fg) || f.Bg != nil && !validColor(f.Bg) {
			return false
		}
		for _, c := range f.Gradient {
			if c != nil && !validColor(c) {
				return false
			}
		}
	}
	if b := st.Border; b != nil {
		for _, e := range []*Edge{b.Left, b.Right, b.Top, b.Bottom, b.Diagonal} {
			if e != nil && (!slices.Contains(borderStyles, e.Style) || e.Color != nil && !validColor(e.Color)) {
				return false
			}
		}
	}
	if a := st.Align; a != nil {
		if !slices.Contains(horizontal, a.H) || !slices.Contains(vertical, a.V) || a.Indent < 0 || a.Indent > 250 ||
			a.Rotate < 0 || a.Rotate > 180 && a.Rotate != 255 {
			return false
		}
	}
	return true
}

func validColor(c *Color) bool {
	return c.Auto || c.Theme != nil && *c.Theme >= 0 && *c.Theme < 12 || validRGB(c.RGB)
}
