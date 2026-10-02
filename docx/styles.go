package docx

import (
	"encoding/json"
	"strconv"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/ot"
)

// Style is a style of styles.xml, its formatting as the keys of Props for
// paragraphs and runs, as the attributes of tables, rows and cells for
// the rest.
type Style struct {
	Type     string                     `json:"type"`
	Name     string                     `json:"name,omitempty"`
	BasedOn  string                     `json:"basedOn,omitempty"`
	Next     string                     `json:"next,omitempty"`
	Link     string                     `json:"link,omitempty"`
	Default  bool                       `json:"default,omitempty"`
	Hidden   bool                       `json:"hidden,omitempty"`
	Quick    bool                       `json:"quick,omitempty"`
	Priority *int                       `json:"priority,omitempty"`
	P        Props                      `json:"p,omitempty"`
	R        Props                      `json:"r,omitempty"`
	Tbl      map[string]json.RawMessage `json:"tbl,omitempty"`
	Tr       map[string]json.RawMessage `json:"tr,omitempty"`
	Tc       map[string]json.RawMessage `json:"tc,omitempty"`
	// Cond is the formatting of parts of tables: "firstRow", "band1Horz"…
	Cond map[string]*Style `json:"cond,omitempty"`
	// Builtin is a style of Word the file does not define, written into it
	// when a paragraph takes it.
	Builtin bool `json:"builtin,omitempty"`
}

// Level is a level of a list.
type Level struct {
	Start int    `json:"start"`
	Fmt   string `json:"fmt,omitempty"`
	// Text is what the number reads: "%1." the first level's number and a
	// period.
	Text string `json:"text"`
	Jc   string `json:"jc,omitempty"`
	// Suff follows the number: "tab", "space" or "nothing".
	Suff    string `json:"suff,omitempty"`
	Restart *int   `json:"restart,omitempty"`
	Legal   bool   `json:"legal,omitempty"`
	PStyle  string `json:"pstyle,omitempty"`
	P       Props  `json:"p,omitempty"`
	R       Props  `json:"r,omitempty"`
	// Override tells that the list starts again at Start rather than
	// going on from the other lists of the same abstract numbering.
	Override bool `json:"override,omitempty"`
}

// Numbering is a list, numId of numbering.xml: its levels, and the
// abstract numbering it shares its counters with.
type Numbering struct {
	Abstract string   `json:"abstract"`
	Levels   []*Level `json:"levels"`
}

// Settings are what settings.xml says of the layout.
type Settings struct {
	Tab        int  `json:"tab"`
	EvenOdd    bool `json:"evenOdd,omitempty"`
	Mirror     bool `json:"mirror,omitempty"`
	GutterTop  bool `json:"gutterTop,omitempty"`
	Hyphen     bool `json:"hyphen,omitempty"`
	Compat     int  `json:"compat,omitempty"`
	NoHTMLAuto bool `json:"noHTMLAuto,omitempty"`
}

// styles reads styles.xml, numbering.xml, the theme and the settings into
// the attributes of the document node.
func (r *reader) styles(attrs ot.Values, rels *partrel.Rels) {
	put := func(key string, v any) {
		attrs[key], _ = json.Marshal(v)
	}
	styles := map[string]*Style{}
	if root := r.optional(rels.OfType(r.d.main, relStyles)); root != nil {
		defaults := map[string]Props{"p": {}, "r": {}}
		if d := child(root, "docDefaults"); d != nil {
			defaults["p"] = ParaProps(child(child(d, "pPrDefault"), "pPr"))
			defaults["r"] = RunProps(child(child(d, "rPrDefault"), "rPr"))
		}
		put("defaults", defaults)
		for _, s := range root.Elements() {
			if s.Space != NS || s.Local != "style" || attr(s, "styleId") == "" {
				continue
			}
			styles[attr(s, "styleId")] = readStyle(s)
		}
	}
	r.d.fileStyles = map[string]bool{}
	for id := range styles {
		r.d.fileStyles[id] = true
	}
	for _, b := range builtinStyles {
		if styles[b.id] == nil {
			st := readStyle(builtin(b.xml))
			st.Builtin = true
			styles[b.id] = st
		}
	}
	put("styles", styles)

	numbering := map[string]*Numbering{}
	if root := r.optional(rels.OfType(r.d.main, relNumbering)); root != nil {
		numbering = readNumbering(root, styles)
	}
	for name, xml := range builtinLists {
		numbering[name] = &Numbering{Abstract: "bref-" + name, Levels: levelsOf(builtin(xml))}
	}
	put("numbering", numbering)

	if root := r.optional(rels.OfType(r.d.main, relTheme)); root != nil {
		fonts, colors := readTheme(root)
		put("fonts", fonts)
		put("colors", colors)
	}

	settings := Settings{Tab: 720}
	if root := r.optional(rels.OfType(r.d.main, relSettings)); root != nil {
		settings.Tab = number(child(root, "defaultTabStop"), "val", 720)
		settings.EvenOdd = on(child(root, "evenAndOddHeaders")) == "1"
		settings.Mirror = on(child(root, "mirrorMargins")) == "1"
		settings.GutterTop = on(child(root, "gutterAtTop")) == "1"
		settings.Hyphen = on(child(root, "autoHyphenation")) == "1"
		settings.NoHTMLAuto = on(child(child(root, "compat"), "doNotUseHTMLParagraphAutoSpacing")) == "1"
		for _, c := range elements(child(root, "compat")) {
			if c.Local == "compatSetting" && attr(c, "name") == "compatibilityMode" {
				settings.Compat = number(c, "val", 0)
			}
		}
		if on(child(root, "trackRevisions")) == "1" {
			attrs["track"] = json.RawMessage("true")
		}
	}
	put("settings", settings)
}

// optional is the root of a part that may be missing or unreadable.
func (r *reader) optional(name string) *xmldom.Element {
	if name == "" {
		return nil
	}
	data, err := r.d.pkg.Read(name)
	if err != nil {
		return nil
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil
	}
	return doc.Root
}

func readStyle(s *xmldom.Element) *Style {
	st := &Style{
		Type:    attr(s, "type"),
		Name:    attr(child(s, "name"), "val"),
		BasedOn: attr(child(s, "basedOn"), "val"),
		Next:    attr(child(s, "next"), "val"),
		Link:    attr(child(s, "link"), "val"),
		Default: truthy(attr(s, "default")),
		Hidden:  on(child(s, "semiHidden")) == "1" || on(child(s, "hidden")) == "1",
		Quick:   on(child(s, "qFormat")) == "1",
	}
	if p, err := strconv.Atoi(attr(child(s, "uiPriority"), "val")); err == nil {
		st.Priority = &p
	}
	formatting(st, s)
	for _, c := range s.Elements() {
		if c.Space == NS && c.Local == "tblStylePr" {
			if st.Cond == nil {
				st.Cond = map[string]*Style{}
			}
			cond := &Style{}
			formatting(cond, c)
			st.Cond[attr(c, "type")] = cond
		}
	}
	return st
}

// formatting reads the pPr, rPr, tblPr, trPr and tcPr of a style.
func formatting(st *Style, s *xmldom.Element) {
	if p := child(s, "pPr"); p != nil {
		st.P = ParaProps(p)
	}
	if p := child(s, "rPr"); p != nil {
		st.R = RunProps(p)
	}
	for _, t := range []struct {
		local string
		list  []tprop
		into  *map[string]json.RawMessage
	}{{"tblPr", tblProps, &st.Tbl}, {"trPr", trProps, &st.Tr}, {"tcPr", tcProps, &st.Tc}} {
		if p := child(s, t.local); p != nil {
			*t.into = map[string]json.RawMessage{}
			readTprops(p, t.list, *t.into)
		}
	}
}

func readLevel(l *xmldom.Element) *Level {
	lv := &Level{
		Start:  number(child(l, "start"), "val", 0),
		Fmt:    attr(child(l, "numFmt"), "val"),
		Text:   attr(child(l, "lvlText"), "val"),
		Jc:     attr(child(l, "lvlJc"), "val"),
		Suff:   attr(child(l, "suff"), "val"),
		Legal:  on(child(l, "isLgl")) == "1",
		PStyle: attr(child(l, "pStyle"), "val"),
	}
	if n, err := strconv.Atoi(attr(child(l, "lvlRestart"), "val")); err == nil {
		lv.Restart = &n
	}
	if p := child(l, "pPr"); p != nil {
		lv.P = ParaProps(p)
	}
	if p := child(l, "rPr"); p != nil {
		lv.R = RunProps(p)
	}
	return lv
}

// levelsOf reads the levels of an abstract numbering.
func levelsOf(a *xmldom.Element) []*Level {
	out := make([]*Level, 9)
	for _, l := range a.Elements() {
		if l.Space == NS && l.Local == "lvl" {
			if i, err := strconv.Atoi(attr(l, "ilvl")); err == nil && 0 <= i && i < 9 {
				out[i] = readLevel(l)
			}
		}
	}
	return out
}

// readNumbering reads the lists, their overrides applied, and those whose
// abstract numbering is a numbering style's resolved.
func readNumbering(root *xmldom.Element, styles map[string]*Style) map[string]*Numbering {
	abstract := map[string]*xmldom.Element{}
	nums := map[string]*xmldom.Element{}
	for _, c := range root.Elements() {
		if c.Space != NS {
			continue
		}
		switch c.Local {
		case "abstractNum":
			abstract[attr(c, "abstractNumId")] = c
		case "num":
			nums[attr(c, "numId")] = c
		}
	}
	// levels are those of an abstract numbering, following the numbering
	// style it links to.
	var levels func(id string, depth int) (string, []*Level)
	levels = func(id string, depth int) (string, []*Level) {
		a := abstract[id]
		if a == nil || depth > 4 {
			return id, nil
		}
		if link := attr(child(a, "numStyleLink"), "val"); link != "" {
			if st := styles[link]; st != nil && st.P["num"] != "" {
				if n := nums[st.P["num"]]; n != nil {
					return levels(attr(child(n, "abstractNumId"), "val"), depth+1)
				}
			}
		}
		return id, levelsOf(a)
	}
	out := map[string]*Numbering{}
	for id, n := range nums {
		aid, lv := levels(attr(child(n, "abstractNumId"), "val"), 0)
		if lv == nil {
			continue
		}
		num := &Numbering{Abstract: aid, Levels: make([]*Level, 9)}
		copy(num.Levels, lv)
		for _, o := range n.Elements() {
			if o.Space != NS || o.Local != "lvlOverride" {
				continue
			}
			i, err := strconv.Atoi(attr(o, "ilvl"))
			if err != nil || i < 0 || i >= 9 {
				continue
			}
			if l := child(o, "lvl"); l != nil {
				num.Levels[i] = readLevel(l)
			}
			if s := child(o, "startOverride"); s != nil {
				l := &Level{}
				if num.Levels[i] != nil {
					c := *num.Levels[i]
					l = &c
				}
				l.Start, l.Override = number(s, "val", 0), true
				num.Levels[i] = l
			}
		}
		for i, l := range num.Levels {
			if l == nil {
				num.Levels[i] = &Level{Fmt: "none"}
			}
		}
		out[id] = num
	}
	return out
}

// readTheme reads the fonts and colors of a theme.
func readTheme(root *xmldom.Element) (map[string]string, map[string]string) {
	const a = "http://schemas.openxmlformats.org/drawingml/2006/main"
	fonts, colors := map[string]string{}, map[string]string{}
	elements := root.Child(a, "themeElements")
	if elements == nil {
		return fonts, colors
	}
	if scheme := elements.Child(a, "fontScheme"); scheme != nil {
		for _, f := range []string{"major", "minor"} {
			font := scheme.Child(a, f+"Font")
			if font == nil {
				continue
			}
			for _, script := range []struct{ local, suffix string }{{"latin", ""}, {"ea", "Ea"}, {"cs", "Cs"}} {
				if c := font.Child(a, script.local); c != nil && c.Get("typeface") != "" {
					fonts[f+script.suffix] = c.Get("typeface")
				}
			}
		}
	}
	if scheme := elements.Child(a, "clrScheme"); scheme != nil {
		for _, c := range scheme.Elements() {
			for _, v := range c.Elements() {
				switch v.Local {
				case "srgbClr":
					colors[c.Local] = v.Get("val")
				case "sysClr":
					colors[c.Local] = v.Get("lastClr")
				}
			}
		}
	}
	return fonts, colors
}
