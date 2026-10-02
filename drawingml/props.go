package drawingml

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Props are formatting as the attributes of a flow have it: string keys and
// values. A key absent is inherited from the list styles above.
//
// Run keys, on characters and on paragraph marks (the end of paragraph run):
//
//	b i         "1" or "0", bold and italic
//	u           underline: "sng" "dbl" "none"…
//	strike      "sngStrike" "dblStrike" "noStrike"
//	sz          size in hundredths of a point
//	baseline    in 1000ths of a percent: superscript above 0
//	cap         "all" "small" "none"
//	spc         letter spacing in hundredths of a point
//	fill        the fill of the letters, as JSON
//	hl          highlight color, as JSON
//	font ea cs sym  typefaces, "+mj-lt" naming the theme's
//	link        {"url":"…"}, read only
//
// Paragraph keys, on marks and list style levels:
//
//	lvl         outline level, "0" to "8"
//	algn        "l" "ctr" "r" "just" "dist"…
//	marL marR indent  in EMU
//	lnSpc spcBef spcAft  "p90000" in 1000ths of a percent of the line, "t1200" in hundredths of a point
//	rtl         "1" or "0"
//	defTabSz    in EMU
//	fontAlgn    "auto" "t" "ctr" "base" "b"
//	buClr       "tx" or a color as JSON
//	buSz        "tx", "p100000" or "t1200"
//	buFont      "tx" or a typeface
//	bu          "none", "char:•", "auto:arabicPeriod:1" (scheme, first number), "blip"
//	tabs        "l:914400 ctr:1828800"
//
// Body keys, of a text body:
//
//	anchor      "t" "ctr" "b" "just" "dist"
//	anchorCtr upright rtlCol  "1" or "0"
//	lIns tIns rIns bIns  insets in EMU
//	wrap        "square" or "none"
//	vert        "horz" "vert" "vert270" "wordArtVert" "eaVert" "mongolianVert" "wordArtVertRtl"
//	rot         in 60000ths of a degree
//	numCol      columns, spcCol the space between them in EMU
//	fit         "none" "norm" "shape", fontScale and lnSpcReduction in 1000ths of a percent
//
// The XML a run, paragraph or field was read from rides along: "r" holds
// the rPr, endParaRPr or the br's rPr, "p" the pPr, "fld" the field without
// its text. What these keys model is written over it.
type Props = map[string]string

// paraKeys are the keys of paragraphs; all others belong to runs.
var paraKeys = map[string]bool{
	"lvl": true, "algn": true, "marL": true, "marR": true, "indent": true, "lnSpc": true,
	"spcBef": true, "spcAft": true, "rtl": true, "defTabSz": true, "fontAlgn": true,
	"buClr": true, "buSz": true, "buFont": true, "bu": true, "tabs": true, "p": true,
}

// IsParaKey tells whether a key of Props belongs to paragraphs.
func IsParaKey(k string) bool {
	return paraKeys[k]
}

// A prop is one key of Props: how it reads from an element and writes back.
type prop struct {
	key  string
	read func(e *xmldom.Element) string
	// write sets the value, "" removing it; a value that is not valid
	// removes it too.
	write func(e *xmldom.Element, v string)
}

func boolAttr(key string) prop {
	return prop{key,
		func(e *xmldom.Element) string {
			v, ok := e.Attr(key)
			if !ok {
				return ""
			}
			if flag(v) {
				return "1"
			}
			return "0"
		},
		func(e *xmldom.Element, v string) {
			switch v {
			case "1", "0":
				e.Set(key, v)
			default:
				e.Unset(key)
			}
		}}
}

func intAttr(key string, lo, hi int64) prop {
	return prop{key,
		func(e *xmldom.Element) string { return e.Get(key) },
		func(e *xmldom.Element, v string) {
			if n, ok := number(v); ok && lo <= n && n <= hi {
				e.Set(key, strconv.FormatInt(n, 10))
			} else {
				e.Unset(key)
			}
		}}
}

func enumAttr(key string, values ...string) prop {
	return prop{key,
		func(e *xmldom.Element) string { return e.Get(key) },
		func(e *xmldom.Element, v string) { setEnum(e, key, v, values...) }}
}

// childProp is a key stored in a child element, one of group, kept in
// order.
func childProp(key string, group, order []string, read func(c *xmldom.Element) string, build func(v string) *xmldom.Element) prop {
	return prop{key,
		func(e *xmldom.Element) string {
			for _, g := range group {
				if c := child(e, g); c != nil {
					return read(c)
				}
			}
			return ""
		},
		func(e *xmldom.Element, v string) {
			var c *xmldom.Element
			if v != "" {
				c = build(v)
			}
			setChild(e, group, c, order)
		}}
}

func typefaceProp(key, local string, order []string) prop {
	return childProp(key, []string{local}, order,
		func(c *xmldom.Element) string { return c.Get("typeface") },
		func(v string) *xmldom.Element { return newA(local, "typeface", v) })
}

// jsonProp is a key whose value is a JSON document of type T.
func jsonProp[T any](key string, group, order []string, read func(e *xmldom.Element) *T, build func(v *T) *xmldom.Element) prop {
	return prop{key,
		func(e *xmldom.Element) string {
			v := read(e)
			if v == nil {
				return ""
			}
			data, _ := json.Marshal(v)
			return string(data)
		},
		func(e *xmldom.Element, v string) {
			var c *xmldom.Element
			var t T
			if v != "" && json.Unmarshal([]byte(v), &t) == nil {
				c = build(&t)
			}
			setChild(e, group, c, order)
		}}
}

var rPrOrder = []string{
	"ln", "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill", "effectLst", "effectDag",
	"highlight", "uLnTx", "uLn", "uFillTx", "uFill", "latin", "ea", "cs", "sym", "hlinkClick",
	"hlinkMouseOver", "rtl", "extLst",
}

var runProps = []prop{
	boolAttr("b"),
	boolAttr("i"),
	enumAttr("u", "none", "words", "sng", "dbl", "heavy", "dotted", "dottedHeavy", "dash", "dashHeavy",
		"dashLong", "dashLongHeavy", "dotDash", "dotDashHeavy", "dotDotDash", "dotDotDashHeavy", "wavy",
		"wavyHeavy", "wavyDbl"),
	enumAttr("strike", "noStrike", "sngStrike", "dblStrike"),
	intAttr("sz", 100, 400000),
	intAttr("baseline", -1000000, 1000000),
	enumAttr("cap", "none", "small", "all"),
	intAttr("spc", -400000, 400000),
	jsonProp("fill", fillKinds, rPrOrder,
		func(e *xmldom.Element) *Fill {
			if f := FillIn(e, nil); f != nil && f.Blip == nil && !f.Group {
				return f
			}
			return nil
		},
		func(f *Fill) *xmldom.Element {
			if f.Blip != nil || f.Group {
				return nil
			}
			return f.Element(nil)
		}),
	jsonProp("hl", []string{"highlight"}, rPrOrder,
		func(e *xmldom.Element) *Color { return ColorIn(child(e, "highlight")) },
		func(c *Color) *xmldom.Element {
			hl := newA("highlight")
			hl.Append(c.Element())
			return hl
		}),
	typefaceProp("font", "latin", rPrOrder),
	typefaceProp("ea", "ea", rPrOrder),
	typefaceProp("cs", "cs", rPrOrder),
	typefaceProp("sym", "sym", rPrOrder),
}

var pPrOrder = []string{
	"lnSpc", "spcBef", "spcAft", "buClrTx", "buClr", "buSzTx", "buSzPct", "buSzPts", "buFontTx", "buFont",
	"buNone", "buAutoNum", "buChar", "buBlip", "tabLst", "defRPr", "extLst",
}

func spacingProp(key string) prop {
	return childProp(key, []string{key}, pPrOrder,
		func(c *xmldom.Element) string {
			if p := child(c, "spcPct"); p != nil {
				return "p" + p.Get("val")
			}
			if p := child(c, "spcPts"); p != nil {
				return "t" + p.Get("val")
			}
			return ""
		},
		func(v string) *xmldom.Element {
			n, ok := number(v[1:])
			if !ok || n < 0 {
				return nil
			}
			e := newA(key)
			switch v[0] {
			case 'p':
				e.Append(newA("spcPct", "val", strconv.FormatInt(min(n, 13200000), 10)))
			case 't':
				e.Append(newA("spcPts", "val", strconv.FormatInt(min(n, 158400), 10)))
			default:
				return nil
			}
			return e
		})
}

var paraProps = []prop{
	intAttr("lvl", 0, 8),
	enumAttr("algn", "l", "ctr", "r", "just", "justLow", "dist", "thaiDist"),
	intAttr("marL", 0, 51206400),
	intAttr("marR", 0, 51206400),
	intAttr("indent", -51206400, 51206400),
	boolAttr("rtl"),
	intAttr("defTabSz", 0, 51206400),
	enumAttr("fontAlgn", "auto", "t", "ctr", "base", "b"),
	spacingProp("lnSpc"),
	spacingProp("spcBef"),
	spacingProp("spcAft"),
	childProp("buClr", []string{"buClrTx", "buClr"}, pPrOrder,
		func(c *xmldom.Element) string {
			if c.Local == "buClrTx" {
				return "tx"
			}
			if color := ColorIn(c); color != nil {
				data, _ := json.Marshal(color)
				return string(data)
			}
			return ""
		},
		func(v string) *xmldom.Element {
			if v == "tx" {
				return newA("buClrTx")
			}
			var c Color
			if json.Unmarshal([]byte(v), &c) != nil {
				return nil
			}
			e := newA("buClr")
			e.Append(c.Element())
			return e
		}),
	childProp("buSz", []string{"buSzTx", "buSzPct", "buSzPts"}, pPrOrder,
		func(c *xmldom.Element) string {
			switch c.Local {
			case "buSzPct":
				return "p" + c.Get("val")
			case "buSzPts":
				return "t" + c.Get("val")
			}
			return "tx"
		},
		func(v string) *xmldom.Element {
			if v == "tx" {
				return newA("buSzTx")
			}
			n, ok := number(v[1:])
			switch {
			case !ok:
			case v[0] == 'p' && 25000 <= n && n <= 400000:
				return newA("buSzPct", "val", strconv.FormatInt(n, 10))
			case v[0] == 't' && 100 <= n && n <= 400000:
				return newA("buSzPts", "val", strconv.FormatInt(n, 10))
			}
			return nil
		}),
	childProp("buFont", []string{"buFontTx", "buFont"}, pPrOrder,
		func(c *xmldom.Element) string {
			if c.Local == "buFontTx" {
				return "tx"
			}
			return c.Get("typeface")
		},
		func(v string) *xmldom.Element {
			if v == "tx" {
				return newA("buFontTx")
			}
			return newA("buFont", "typeface", v)
		}),
	childProp("bu", []string{"buNone", "buAutoNum", "buChar", "buBlip"}, pPrOrder,
		func(c *xmldom.Element) string {
			switch c.Local {
			case "buNone":
				return "none"
			case "buChar":
				return "char:" + c.Get("char")
			case "buAutoNum":
				start := c.Get("startAt")
				if start == "" {
					start = "1"
				}
				return "auto:" + c.Get("type") + ":" + start
			}
			return "blip"
		},
		func(v string) *xmldom.Element {
			kind, rest, _ := strings.Cut(v, ":")
			switch kind {
			case "none":
				return newA("buNone")
			case "char":
				if rest == "" {
					return nil
				}
				return newA("buChar", "char", rest)
			case "auto":
				scheme, start, _ := strings.Cut(rest, ":")
				if !autoNumbers[scheme] {
					return nil
				}
				e := newA("buAutoNum", "type", scheme)
				if n, ok := number(start); ok && n > 1 && n <= 32767 {
					e.Set("startAt", strconv.FormatInt(n, 10))
				}
				return e
			}
			return nil
		}),
	childProp("tabs", []string{"tabLst"}, pPrOrder,
		func(c *xmldom.Element) string {
			var tabs []string
			for _, t := range children(c) {
				algn := t.Get("algn")
				if algn == "" {
					algn = "l"
				}
				tabs = append(tabs, algn+":"+t.Get("pos"))
			}
			return strings.Join(tabs, " ")
		},
		func(v string) *xmldom.Element {
			e := newA("tabLst")
			for _, tab := range strings.Fields(v) {
				algn, pos, _ := strings.Cut(tab, ":")
				n, ok := number(pos)
				if !ok || n < 0 || algn != "l" && algn != "ctr" && algn != "r" && algn != "dec" {
					continue
				}
				e.Append(newA("tab", "pos", strconv.FormatInt(n, 10), "algn", algn))
			}
			return e
		}),
}

var autoNumbers = map[string]bool{}

func init() {
	for _, n := range []string{
		"alphaLcParenBoth", "alphaUcParenBoth", "alphaLcParenR", "alphaUcParenR", "alphaLcPeriod", "alphaUcPeriod",
		"arabicParenBoth", "arabicParenR", "arabicPeriod", "arabicPlain", "romanLcParenBoth", "romanUcParenBoth",
		"romanLcParenR", "romanUcParenR", "romanLcPeriod", "romanUcPeriod", "circleNumDbPlain",
		"circleNumWdBlackPlain", "circleNumWdWhitePlain", "arabicDbPeriod", "arabicDbPlain", "ea1ChsPeriod",
		"ea1ChsPlain", "ea1ChtPeriod", "ea1ChtPlain", "ea1JpnChsDbPeriod", "ea1JpnKorPlain", "ea1JpnKorPeriod",
		"arabic1Minus", "arabic2Minus", "hebrew2Minus", "thaiAlphaPeriod", "thaiAlphaParenR", "thaiAlphaParenBoth",
		"thaiNumPeriod", "thaiNumParenR", "thaiNumParenBoth", "hindiAlphaPeriod", "hindiNumPeriod",
		"hindiNumParenR", "hindiAlpha1Period",
	} {
		autoNumbers[n] = true
	}
}

var bodyOrder = []string{"prstTxWarp", "noAutofit", "normAutofit", "spAutoFit", "scene3d", "sp3d", "flatTx", "extLst"}

var bodyProps = []prop{
	enumAttr("anchor", "t", "ctr", "b", "just", "dist"),
	boolAttr("anchorCtr"),
	boolAttr("upright"),
	boolAttr("rtlCol"),
	intAttr("lIns", 0, 51206400),
	intAttr("tIns", 0, 51206400),
	intAttr("rIns", 0, 51206400),
	intAttr("bIns", 0, 51206400),
	enumAttr("wrap", "square", "none"),
	enumAttr("vert", "horz", "vert", "vert270", "wordArtVert", "eaVert", "mongolianVert", "wordArtVertRtl"),
	intAttr("rot", -21600000, 21600000),
	intAttr("numCol", 1, 16),
	intAttr("spcCol", 0, 51206400),
	childProp("fit", []string{"noAutofit", "normAutofit", "spAutoFit"}, bodyOrder,
		func(c *xmldom.Element) string {
			switch c.Local {
			case "normAutofit":
				return "norm"
			case "spAutoFit":
				return "shape"
			}
			return "none"
		},
		func(v string) *xmldom.Element {
			switch v {
			case "none":
				return newA("noAutofit")
			case "norm":
				return newA("normAutofit")
			case "shape":
				return newA("spAutoFit")
			}
			return nil
		}),
	autofitProp("fontScale"),
	autofitProp("lnSpcReduction"),
}

// autofitProp is an attribute of the normAutofit of a bodyPr.
func autofitProp(attr string) prop {
	return prop{attr,
		func(e *xmldom.Element) string {
			if fit := child(e, "normAutofit"); fit != nil {
				return fit.Get(attr)
			}
			return ""
		},
		func(e *xmldom.Element, v string) { autofitAttr(e, attr, v) }}
}

func autofitAttr(e *xmldom.Element, attr, v string) {
	fit := child(e, "normAutofit")
	if fit == nil {
		return
	}
	if n, ok := number(v); ok && 0 <= n && n <= 100000 {
		fit.Set(attr, strconv.FormatInt(n, 10))
	} else {
		fit.Unset(attr)
	}
}

func readProps(e *xmldom.Element, props []prop, into Props) Props {
	if into == nil {
		into = Props{}
	}
	if e == nil {
		return into
	}
	for _, p := range props {
		if v := p.read(e); v != "" {
			into[p.key] = v
		}
	}
	return into
}

// writeProps patches e so that it holds the keys of props that differ
// from old, the keys e was read as.
func writeProps(e *xmldom.Element, list []prop, old, props Props) {
	for _, p := range list {
		if old[p.key] != props[p.key] {
			p.write(e, props[p.key])
		}
	}
}

// RunProps reads an rPr, endParaRPr or defRPr; link names the target of a
// hyperlink's relationship.
func RunProps(e *xmldom.Element, link func(rid string) string) Props {
	props := readProps(e, runProps, nil)
	if h := child(e, "hlinkClick"); h != nil {
		l := map[string]string{}
		if rid := h.Get("r:id"); rid != "" && link != nil {
			l["url"] = link(rid)
		}
		if action := h.Get("action"); action != "" {
			l["action"] = action
		}
		data, _ := json.Marshal(l)
		props["link"] = string(data)
	}
	return props
}

// SetRunProps patches an rPr read as old to hold props.
func SetRunProps(e *xmldom.Element, old, props Props) {
	writeProps(e, runProps, old, props)
}

// ParaProps reads a pPr, or a level of a list style with its defRPr.
func ParaProps(e *xmldom.Element) Props {
	props := readProps(e, paraProps, nil)
	if d := child(e, "defRPr"); d != nil {
		readProps(d, runProps, props)
	}
	return props
}

// SetParaProps patches a pPr read as old to hold the paragraph keys of
// props.
func SetParaProps(e *xmldom.Element, old, props Props) {
	writeProps(e, paraProps, old, props)
}

// BodyProps reads a bodyPr.
func BodyProps(e *xmldom.Element) Props {
	return readProps(e, bodyProps, nil)
}

// SetBodyProps patches a bodyPr read as old to hold props.
func SetBodyProps(e *xmldom.Element, old, props Props) {
	writeProps(e, bodyProps, old, props)
}

// ListStyle reads the levels of a list style, a lstStyle or the txStyles
// of a master: "0" for its defPPr, "1" to "9" for its levels.
func ListStyle(e *xmldom.Element) map[string]Props {
	out := map[string]Props{}
	for _, c := range children(e) {
		switch {
		case c.Local == "defPPr":
			out["0"] = ParaProps(c)
		case strings.HasPrefix(c.Local, "lvl") && strings.HasSuffix(c.Local, "pPr") && len(c.Local) == 7:
			out[c.Local[3:4]] = ParaProps(c)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
