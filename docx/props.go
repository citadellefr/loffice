package docx

import (
	"encoding/json"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Props are formatting as the attributes of a flow have it: string keys and
// values, in the units of Word. A key absent is inherited from the styles.
//
// Run keys, on characters and on paragraph marks:
//
//	rstyle      the character style
//	b i caps smallCaps strike dstrike vanish  "1" or "0"
//	u           underline: "single" "double" "none"…
//	sz          size in half points
//	font fontEa fontCs  typefaces, "+major" or "+minor" naming the theme's
//	color       "RRGGBB" or "auto"
//	hl          highlight: "yellow"…
//	shd         shading, "RRGGBB"
//	va          "superscript" "subscript" "baseline"
//	spc         letter spacing in twentieths of a point
//	pos         raised or lowered, in half points
//	scale       width in percent
//
// Paragraph keys, on marks:
//
//	pstyle      the paragraph style
//	jc          "left" "center" "right" "both" "start" "end"…
//	ind.left ind.right  in twentieths of a point
//	ind.first   first line indent, negative when hanging
//	sp.before sp.after  in twentieths of a point
//	sp.line     in 240ths of a line when sp.rule is "auto", twentieths of a point otherwise
//	sp.rule     "auto" "exact" "atLeast"
//	sp.beforeAuto sp.afterAuto  "1" or "0", spacing as in HTML
//	num lvl     the numbering and its level, "0" for none
//	keepNext keepLines pageBreakBefore widowControl contextualSpacing bidi  "1" or "0"
//	outline     outline level, "0" to "9"
//	tabs        "left:720 center:4680:dot clear:1440", kind, position and leader
//	pshd        shading, "RRGGBB"
//	pbdr        borders, as JSON: {"top":{"val":"single","sz":4,"space":1,"color":"auto"},…}
//
// The XML a run or paragraph was read from rides along: "r" holds the rPr,
// "p" the pPr. What these keys model is written over it.
type Props = map[string]string

var paraKeys = map[string]bool{
	"pstyle": true, "jc": true, "ind.left": true, "ind.right": true, "ind.first": true,
	"sp.before": true, "sp.after": true, "sp.line": true, "sp.rule": true, "sp.beforeAuto": true, "sp.afterAuto": true,
	"num": true, "lvl": true, "keepNext": true, "keepLines": true, "pageBreakBefore": true, "widowControl": true,
	"contextualSpacing": true, "bidi": true, "outline": true, "tabs": true, "pshd": true, "pbdr": true,
}

// IsParaKey tells whether a key of Props belongs to paragraphs.
func IsParaKey(k string) bool {
	return paraKeys[k]
}

type prop struct {
	key  string
	read func(e *xmldom.Element) string
	// write sets the value, "" removing it; a value that is not valid
	// removes it too.
	write func(e *xmldom.Element, v string)
}

var rPrOrder = []string{
	"ins", "del", "moveFrom", "moveTo", "rStyle", "rFonts", "b", "bCs", "i", "iCs", "caps", "smallCaps",
	"strike", "dstrike", "outline", "shadow", "emboss", "imprint", "noProof", "snapToGrid", "vanish",
	"webHidden", "color", "spacing", "w", "kern", "position", "sz", "szCs", "highlight", "u", "effect",
	"bdr", "shd", "fitText", "vertAlign", "rtl", "cs", "em", "lang", "eastAsianLayout", "specVanish",
	"oMath", "rPrChange",
}

var pPrOrder = []string{
	"pStyle", "keepNext", "keepLines", "pageBreakBefore", "framePr", "widowControl", "numPr",
	"suppressLineNumbers", "pBdr", "shd", "tabs", "suppressAutoHyphens", "kinsoku", "wordWrap",
	"overflowPunct", "topLinePunct", "autoSpaceDE", "autoSpaceDN", "bidi", "adjustRightInd", "snapToGrid",
	"spacing", "ind", "contextualSpacing", "mirrorIndents", "suppressOverlap", "jc", "textDirection",
	"textAlignment", "textboxTightWrap", "outlineLvl", "divId", "cnfStyle", "rPr", "sectPr", "pPrChange",
}

// child is the first child of e in the main namespace with that name.
func child(e *xmldom.Element, local string) *xmldom.Element {
	if e == nil {
		return nil
	}
	return e.Child(NS, local)
}

// elements are the children of e, none when e is nil.
func elements(e *xmldom.Element) []*xmldom.Element {
	if e == nil {
		return nil
	}
	return e.Elements()
}

// prefix is the prefix of an element's name, "" when it has none.
func prefix(e *xmldom.Element) string {
	p, _, found := strings.Cut(e.Name, ":")
	if !found {
		return ""
	}
	return p
}

// attr is an attribute of an element of the main namespace, which Word
// qualifies with the element's prefix.
func attr(e *xmldom.Element, local string) string {
	if e == nil {
		return ""
	}
	if p := prefix(e); p != "" {
		if v, ok := e.Attr(p + ":" + local); ok {
			return v
		}
	}
	return e.Get(local)
}

func qualified(e *xmldom.Element, local string) string {
	if p := prefix(e); p != "" {
		return p + ":" + local
	}
	return "w:" + local
}

func setAttr(e *xmldom.Element, local, v string) {
	e.Set(qualified(e, local), v)
}

func unsetAttr(e *xmldom.Element, local string) {
	e.Unset(qualified(e, local))
	e.Unset(local)
}

// newW is an element of the main namespace, named with the prefix of the
// element it goes in; attrs alternate local names and values.
func newW(in *xmldom.Element, local string, attrs ...string) *xmldom.Element {
	name := qualified(in, local)
	p, _, _ := strings.Cut(name, ":")
	e := xmldom.New(NS, name)
	for i := 0; i+1 < len(attrs); i += 2 {
		e.Set(p+":"+attrs[i], attrs[i+1])
	}
	return e
}

// insert adds a child of the main namespace where the sequence of the
// schema, order, puts it: before the first child that comes after it.
// Children of other namespaces, extensions of later versions of Word, come
// last.
func insert(e, c *xmldom.Element, order []string) {
	rank := func(x *xmldom.Element) int {
		if x.Space == NS {
			if i := slices.Index(order, x.Local); i >= 0 {
				return i
			}
		}
		return len(order)
	}
	r := rank(c)
	for i, n := range e.Content {
		if x, ok := n.(*xmldom.Element); ok && rank(x) > r {
			e.Content = slices.Insert(e.Content, i, xmldom.Node(c))
			return
		}
	}
	e.Append(c)
}

// ensure is the child of e with that name, added where the order puts it.
func ensure(e *xmldom.Element, local string, order []string) *xmldom.Element {
	if c := child(e, local); c != nil {
		return c
	}
	c := newW(e, local)
	insert(e, c, order)
	return c
}

func remove(e *xmldom.Element, local string) {
	for c := child(e, local); c != nil; c = child(e, local) {
		e.Remove(c)
	}
}

func truthy(v string) bool {
	return v == "1" || v == "true" || v == "on"
}

// on reads an on/off property: absent, on or off.
func on(c *xmldom.Element) string {
	if c == nil {
		return ""
	}
	switch attr(c, "val") {
	case "", "1", "true", "on":
		return "1"
	}
	return "0"
}

func onOff(key, local string, order []string, twin string) prop {
	return prop{key,
		func(e *xmldom.Element) string { return on(child(e, local)) },
		func(e *xmldom.Element, v string) {
			for _, name := range []string{local, twin} {
				if name == "" {
					continue
				}
				switch v {
				case "1":
					c := ensure(e, name, order)
					unsetAttr(c, "val")
				case "0":
					setAttr(ensure(e, name, order), "val", "0")
				default:
					remove(e, name)
				}
			}
		}}
}

// valProp is the attribute of a child, the child added or removed as the
// value comes and goes; its other attributes stay.
func valProp(key, local, name string, order []string, valid func(string) bool, twin string) prop {
	return prop{key,
		func(e *xmldom.Element) string { return attr(child(e, local), name) },
		func(e *xmldom.Element, v string) {
			for _, l := range []string{local, twin} {
				if l == "" {
					continue
				}
				if v == "" || !valid(v) {
					remove(e, l)
					continue
				}
				setAttr(ensure(e, l, order), name, v)
			}
		}}
}

func enum(values ...string) func(string) bool {
	return func(v string) bool { return slices.Contains(values, v) }
}

func integer(lo, hi int) func(string) bool {
	return func(v string) bool {
		n, err := strconv.Atoi(v)
		return err == nil && lo <= n && n <= hi
	}
}

var hexColor = regexp.MustCompile(`^[0-9A-Fa-f]{6}$`)

func color(v string) bool {
	return v == "auto" || hexColor.MatchString(v)
}

func name(v string) bool {
	return v != "" && len(v) <= 253 && !strings.ContainsAny(v, "\x00\n\r\t")
}

var underlines = enum("single", "words", "double", "thick", "dotted", "dottedHeavy", "dash", "dashedHeavy",
	"dashLong", "dashLongHeavy", "dotDash", "dashDotHeavy", "dotDotDash", "dashDotDotHeavy", "wave",
	"wavyHeavy", "wavyDouble", "none")

var highlights = enum("black", "blue", "cyan", "green", "magenta", "red", "yellow", "white", "darkBlue",
	"darkCyan", "darkGreen", "darkMagenta", "darkRed", "darkYellow", "darkGray", "lightGray", "none")

var justifications = enum("start", "center", "end", "both", "mediumKashida", "distribute", "numTab",
	"highKashida", "lowKashida", "thaiDistribute", "left", "right")

// fontProp is a typeface of rFonts: its attribute, or its theme's.
func fontProp(key, attr1, attr2, theme1, theme2 string) prop {
	return prop{key,
		func(e *xmldom.Element) string {
			f := child(e, "rFonts")
			if t := attr(f, theme1); t != "" {
				if strings.HasPrefix(t, "major") {
					return "+major"
				}
				return "+minor"
			}
			return attr(f, attr1)
		},
		func(e *xmldom.Element, v string) {
			f := child(e, "rFonts")
			if v == "" || !name(v) || len(v) > 31 {
				if f != nil {
					for _, a := range []string{attr1, attr2, theme1, theme2} {
						if a != "" {
							unsetAttr(f, a)
						}
					}
					if len(f.Attrs) == 0 {
						e.Remove(f)
					}
				}
				return
			}
			f = ensure(e, "rFonts", rPrOrder)
			var theme string
			switch v {
			case "+major":
				theme = "majorHAnsi"
			case "+minor":
				theme = "minorHAnsi"
			}
			if key != "font" && theme != "" {
				theme = strings.Replace(theme, "HAnsi", map[string]string{"fontEa": "EastAsia", "fontCs": "Bidi"}[key], 1)
			}
			for _, pair := range [][2]string{{attr1, theme1}, {attr2, theme2}} {
				if pair[0] == "" {
					continue
				}
				if theme != "" {
					setAttr(f, pair[1], theme)
					unsetAttr(f, pair[0])
				} else {
					setAttr(f, pair[0], v)
					unsetAttr(f, pair[1])
				}
			}
		}}
}

func colorProp(key string, order []string) prop {
	return prop{key,
		func(e *xmldom.Element) string { return attr(child(e, "color"), "val") },
		func(e *xmldom.Element, v string) {
			if !color(v) {
				remove(e, "color")
				return
			}
			c := ensure(e, "color", order)
			setAttr(c, "val", v)
			for _, a := range []string{"themeColor", "themeTint", "themeShade"} {
				unsetAttr(c, a)
			}
		}}
}

// shdProp is the fill of a shading.
func shdProp(key string, order []string) prop {
	return prop{key,
		func(e *xmldom.Element) string {
			s := child(e, "shd")
			if s == nil || attr(s, "val") != "clear" && attr(s, "val") != "solid" && attr(s, "val") != "nil" {
				return ""
			}
			if attr(s, "val") == "solid" {
				return attr(s, "color")
			}
			if attr(s, "val") == "nil" {
				return "auto"
			}
			return attr(s, "fill")
		},
		func(e *xmldom.Element, v string) {
			remove(e, "shd")
			if !color(v) {
				return
			}
			insert(e, newW(e, "shd", "val", "clear", "color", "auto", "fill", v), order)
		}}
}

var runProps = []prop{
	valProp("rstyle", "rStyle", "val", rPrOrder, name, ""),
	fontProp("font", "ascii", "hAnsi", "asciiTheme", "hAnsiTheme"),
	fontProp("fontEa", "eastAsia", "", "eastAsiaTheme", ""),
	fontProp("fontCs", "cs", "", "cstheme", ""),
	onOff("b", "b", rPrOrder, "bCs"),
	onOff("i", "i", rPrOrder, "iCs"),
	onOff("caps", "caps", rPrOrder, ""),
	onOff("smallCaps", "smallCaps", rPrOrder, ""),
	onOff("strike", "strike", rPrOrder, ""),
	onOff("dstrike", "dstrike", rPrOrder, ""),
	onOff("vanish", "vanish", rPrOrder, ""),
	colorProp("color", rPrOrder),
	valProp("spc", "spacing", "val", rPrOrder, integer(-31680, 31680), ""),
	valProp("scale", "w", "val", rPrOrder, integer(1, 600), ""),
	valProp("pos", "position", "val", rPrOrder, integer(-3168, 3168), ""),
	valProp("sz", "sz", "val", rPrOrder, integer(1, 3276), "szCs"),
	valProp("hl", "highlight", "val", rPrOrder, highlights, ""),
	valProp("u", "u", "val", rPrOrder, underlines, ""),
	shdProp("shd", rPrOrder),
	valProp("va", "vertAlign", "val", rPrOrder, enum("baseline", "superscript", "subscript"), ""),
}

// indProp is a side of ind: left and right, or the first line.
func indProp(key string) prop {
	names := map[string][]string{"ind.left": {"left", "start"}, "ind.right": {"right", "end"}}[key]
	return prop{key,
		func(e *xmldom.Element) string {
			ind := child(e, "ind")
			if key == "ind.first" {
				if v := attr(ind, "hanging"); v != "" {
					if n, err := strconv.Atoi(v); err == nil && n != 0 {
						return strconv.Itoa(-n)
					}
					return "0"
				}
				return attr(ind, "firstLine")
			}
			for _, n := range names {
				if v := attr(ind, n); v != "" {
					return v
				}
			}
			return ""
		},
		func(e *xmldom.Element, v string) {
			ind := ensure(e, "ind", pPrOrder)
			n, err := strconv.Atoi(v)
			valid := err == nil && -31680 <= n && n <= 31680
			if key == "ind.first" {
				for _, a := range []string{"hanging", "firstLine", "hangingChars", "firstLineChars"} {
					unsetAttr(ind, a)
				}
				switch {
				case !valid:
				case n < 0:
					setAttr(ind, "hanging", strconv.Itoa(-n))
				default:
					setAttr(ind, "firstLine", strconv.Itoa(n))
				}
			} else {
				for _, a := range names {
					unsetAttr(ind, a)
					unsetAttr(ind, a+"Chars")
				}
				if valid {
					setAttr(ind, names[0], v)
				}
			}
			if len(ind.Attrs) == 0 {
				e.Remove(ind)
			}
		}}
}

// spacingProp is an attribute of the spacing of a paragraph.
func spacingProp(key, local string, valid func(string) bool, overridden string) prop {
	return prop{key,
		func(e *xmldom.Element) string {
			v := attr(child(e, "spacing"), local)
			if strings.HasSuffix(local, "Autospacing") && v != "" {
				return on(child(e, "spacing"))
			}
			return v
		},
		func(e *xmldom.Element, v string) {
			sp := ensure(e, "spacing", pPrOrder)
			if overridden != "" {
				unsetAttr(sp, overridden)
			}
			if strings.HasSuffix(local, "Autospacing") && (v == "1" || v == "0") {
				setAttr(sp, local, v)
			} else if v != "" && valid(v) {
				setAttr(sp, local, v)
			} else {
				unsetAttr(sp, local)
			}
			if len(sp.Attrs) == 0 {
				e.Remove(sp)
			}
		}}
}

// numProp is a child of numPr.
func numProp(key, local string) prop {
	return prop{key,
		func(e *xmldom.Element) string { return attr(child(child(e, "numPr"), local), "val") },
		func(e *xmldom.Element, v string) {
			numPr := ensure(e, "numPr", pPrOrder)
			remove(numPr, local)
			if integer(0, 1<<31-1)(v) && (local == "numId" || integer(0, 8)(v)) {
				insert(numPr, newW(numPr, local, "val", v), []string{"ilvl", "numId", "numberingChange", "ins"})
			}
			if len(numPr.Elements()) == 0 {
				e.Remove(numPr)
			}
		}}
}

var tabKinds = enum("clear", "start", "center", "end", "decimal", "bar", "num", "left", "right")
var tabLeaders = enum("none", "dot", "hyphen", "underscore", "heavy", "middleDot")

var tabsProp = prop{"tabs",
	func(e *xmldom.Element) string {
		tabs := child(e, "tabs")
		if tabs == nil {
			return ""
		}
		var out []string
		for _, t := range tabs.Elements() {
			if t.Space != NS || t.Local != "tab" {
				continue
			}
			s := attr(t, "val") + ":" + attr(t, "pos")
			if l := attr(t, "leader"); l != "" && l != "none" {
				s += ":" + l
			}
			out = append(out, s)
		}
		return strings.Join(out, " ")
	},
	func(e *xmldom.Element, v string) {
		remove(e, "tabs")
		tabs := newW(e, "tabs")
		for _, t := range strings.Fields(v) {
			parts := strings.Split(t, ":")
			if len(parts) < 2 || len(parts) > 3 || !tabKinds(parts[0]) || !integer(-31680, 31680)(parts[1]) {
				continue
			}
			tab := newW(tabs, "tab", "val", parts[0])
			if len(parts) == 3 && tabLeaders(parts[2]) {
				setAttr(tab, "leader", parts[2])
			}
			setAttr(tab, "pos", parts[1])
			tabs.Append(tab)
		}
		if len(tabs.Content) > 0 {
			insert(e, tabs, pPrOrder)
		}
	}}

// Border is a line along a side of a paragraph, table or cell.
type Border struct {
	Val   string `json:"val"`
	Sz    int    `json:"sz,omitempty"`    // eighths of a point
	Space int    `json:"space,omitempty"` // points
	Color string `json:"color,omitempty"`
}

var borderKinds = enum("nil", "none", "single", "thick", "double", "dotted", "dashed", "dotDash", "dotDotDash",
	"triple", "thinThickSmallGap", "thickThinSmallGap", "thinThickThinSmallGap", "thinThickMediumGap",
	"thickThinMediumGap", "thinThickThinMediumGap", "thinThickLargeGap", "thickThinLargeGap",
	"thinThickThinLargeGap", "wave", "doubleWave", "dashSmallGap", "dashDotStroked", "threeDEmboss",
	"threeDEngrave", "outset", "inset")

// Borders reads the sides of a pBdr, tblBorders or tcBorders.
func Borders(e *xmldom.Element) map[string]Border {
	if e == nil {
		return nil
	}
	out := map[string]Border{}
	for _, c := range e.Elements() {
		if c.Space != NS {
			continue
		}
		b := Border{Val: attr(c, "val"), Color: attr(c, "color")}
		b.Sz, _ = strconv.Atoi(attr(c, "sz"))
		b.Space, _ = strconv.Atoi(attr(c, "space"))
		out[c.Local] = b
	}
	return out
}

// setBorders writes the sides of a borders element, in the order given;
// sides the order does not name are left out.
func setBorders(e *xmldom.Element, sides map[string]Border, order []string) {
	e.Content = nil
	for _, side := range order {
		b, ok := sides[side]
		if !ok || !borderKinds(b.Val) || b.Color != "" && !color(b.Color) {
			continue
		}
		c := newW(e, side, "val", b.Val)
		if b.Sz > 0 {
			setAttr(c, "sz", strconv.Itoa(min(b.Sz, 96)))
		}
		setAttr(c, "space", strconv.Itoa(min(max(b.Space, 0), 31)))
		if b.Color != "" {
			setAttr(c, "color", b.Color)
		}
		e.Append(c)
	}
}

var pBdrSides = []string{"top", "left", "bottom", "right", "between", "bar"}

var pbdrProp = prop{"pbdr",
	func(e *xmldom.Element) string {
		b := Borders(child(e, "pBdr"))
		if b == nil {
			return ""
		}
		data, _ := json.Marshal(b)
		return string(data)
	},
	func(e *xmldom.Element, v string) {
		remove(e, "pBdr")
		var sides map[string]Border
		if json.Unmarshal([]byte(v), &sides) != nil || len(sides) == 0 {
			return
		}
		b := newW(e, "pBdr")
		setBorders(b, sides, pBdrSides)
		if len(b.Content) > 0 {
			insert(e, b, pPrOrder)
		}
	}}

var twips = integer(-31680, 31680)

var paraProps = []prop{
	valProp("pstyle", "pStyle", "val", pPrOrder, name, ""),
	onOff("keepNext", "keepNext", pPrOrder, ""),
	onOff("keepLines", "keepLines", pPrOrder, ""),
	onOff("pageBreakBefore", "pageBreakBefore", pPrOrder, ""),
	onOff("widowControl", "widowControl", pPrOrder, ""),
	numProp("num", "numId"),
	numProp("lvl", "ilvl"),
	pbdrProp,
	shdProp("pshd", pPrOrder),
	tabsProp,
	onOff("bidi", "bidi", pPrOrder, ""),
	spacingProp("sp.before", "before", integer(0, 31680), "beforeLines"),
	spacingProp("sp.beforeAuto", "beforeAutospacing", nil, ""),
	spacingProp("sp.after", "after", integer(0, 31680), "afterLines"),
	spacingProp("sp.afterAuto", "afterAutospacing", nil, ""),
	spacingProp("sp.line", "line", integer(0, 31680), ""),
	spacingProp("sp.rule", "lineRule", enum("auto", "exact", "atLeast"), ""),
	indProp("ind.left"),
	indProp("ind.right"),
	indProp("ind.first"),
	onOff("contextualSpacing", "contextualSpacing", pPrOrder, ""),
	valProp("jc", "jc", "val", pPrOrder, justifications, ""),
	valProp("outline", "outlineLvl", "val", pPrOrder, integer(0, 9), ""),
}

func readProps(e *xmldom.Element, list []prop, into Props) Props {
	if into == nil {
		into = Props{}
	}
	if e == nil {
		return into
	}
	for _, p := range list {
		if v := p.read(e); v != "" {
			into[p.key] = v
		}
	}
	return into
}

// writeProps patches e so that it holds the keys of props that differ from
// old, the keys e was read as.
func writeProps(e *xmldom.Element, list []prop, old, props Props) {
	for _, p := range list {
		if old[p.key] != props[p.key] {
			p.write(e, props[p.key])
		}
	}
}

// RunProps reads an rPr.
func RunProps(e *xmldom.Element) Props {
	return readProps(e, runProps, nil)
}

// ParaProps reads a pPr, leaving out the rPr of its mark.
func ParaProps(e *xmldom.Element) Props {
	return readProps(e, paraProps, nil)
}
