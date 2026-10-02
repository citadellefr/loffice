package xlsx

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Style is a cell format, what an "xf" node holds. A style made in the
// editor names the style of the file it derives from in Base: what the
// model leaves out is taken from it.
type Style struct {
	Base   string      `json:"base,omitempty"`
	Format string      `json:"fmt,omitempty"`
	Font   *Font       `json:"font,omitempty"`
	Fill   *Fill       `json:"fill,omitempty"`
	Border *Border     `json:"border,omitempty"`
	Align  *Align      `json:"align,omitempty"`
	Prot   *Protection `json:"prot,omitempty"`
}

// Color is a color as a workbook writes it: RGB, a color of the theme,
// lightened or darkened by Tint, or the automatic color.
type Color struct {
	RGB   string  `json:"rgb,omitempty"`
	Theme *int    `json:"theme,omitempty"`
	Tint  float64 `json:"tint,omitempty"`
	Auto  bool    `json:"auto,omitempty"`
}

type Font struct {
	Name      string  `json:"name,omitempty"`
	Size      float64 `json:"sz,omitempty"`
	Bold      bool    `json:"b,omitempty"`
	Italic    bool    `json:"i,omitempty"`
	Underline string  `json:"u,omitempty"`
	Strike    bool    `json:"strike,omitempty"`
	Color     *Color  `json:"color,omitempty"`
	VertAlign string  `json:"va,omitempty"`
	// Scheme names the theme font standing for Name: "minor" or "major".
	Scheme string `json:"scheme,omitempty"`
}

// Fill is a pattern, "solid" for a plain color (Fg), or a gradient shown
// by its colors.
type Fill struct {
	Pattern  string   `json:"pattern,omitempty"`
	Fg       *Color   `json:"fg,omitempty"`
	Bg       *Color   `json:"bg,omitempty"`
	Gradient []*Color `json:"gradient,omitempty"`
}

type Border struct {
	Left     *Edge `json:"l,omitempty"`
	Right    *Edge `json:"r,omitempty"`
	Top      *Edge `json:"t,omitempty"`
	Bottom   *Edge `json:"b,omitempty"`
	Diagonal *Edge `json:"d,omitempty"`
	Up       bool  `json:"up,omitempty"`
	Down     bool  `json:"down,omitempty"`
}

type Edge struct {
	Style string `json:"style"`
	Color *Color `json:"color,omitempty"`
}

type Align struct {
	H      string `json:"h,omitempty"`
	V      string `json:"v,omitempty"`
	Wrap   bool   `json:"wrap,omitempty"`
	Indent int    `json:"indent,omitempty"`
	Rotate int    `json:"rot,omitempty"`
	Shrink bool   `json:"shrink,omitempty"`
}

type Protection struct {
	Unlocked bool `json:"unlocked,omitempty"`
	Hidden   bool `json:"hidden,omitempty"`
}

// styles is styles.xml as read.
type styles struct {
	name    string
	doc     *xmldom.Document
	fonts   []*xmldom.Element
	fills   []*xmldom.Element
	borders []*xmldom.Element
	xfs     []*xmldom.Element
	dxfs    []*xmldom.Element
	formats map[int]string
	palette []string
}

func readStyles(name string, doc *xmldom.Document) *styles {
	s := &styles{name: name, doc: doc, formats: map[int]string{}, palette: defaultPalette[:]}
	root := doc.Root
	s.fonts = elements(root.Child(mainNS, "fonts"), "font")
	s.fills = elements(root.Child(mainNS, "fills"), "fill")
	s.borders = elements(root.Child(mainNS, "borders"), "border")
	s.xfs = elements(root.Child(mainNS, "cellXfs"), "xf")
	s.dxfs = elements(root.Child(mainNS, "dxfs"), "dxf")
	for _, f := range elements(root.Child(mainNS, "numFmts"), "numFmt") {
		if id, err := strconv.Atoi(f.Get("numFmtId")); err == nil {
			s.formats[id] = f.Get("formatCode")
		}
	}
	if colors := root.Child(mainNS, "colors"); colors != nil {
		if indexed := elements(colors.Child(mainNS, "indexedColors"), "rgbColor"); len(indexed) > 0 {
			s.palette = make([]string, len(indexed))
			for i, c := range indexed {
				s.palette[i] = rgb(c.Get("rgb"))
			}
		}
	}
	return s
}

// style is the model of the cell format at index i.
func (s *styles) style(i int) Style {
	var st Style
	if i < 0 || i >= len(s.xfs) {
		return st
	}
	xf := s.xfs[i]
	st.Format = s.format(atoi(xf.Get("numFmtId")))
	if f := atoi(xf.Get("fontId")); f < len(s.fonts) {
		st.Font = s.font(s.fonts[f])
	}
	if f := atoi(xf.Get("fillId")); f < len(s.fills) {
		st.Fill = s.fill(s.fills[f])
	}
	if b := atoi(xf.Get("borderId")); b < len(s.borders) {
		st.Border = s.border(s.borders[b])
	}
	if a := xf.Child(mainNS, "alignment"); a != nil {
		st.Align = &Align{
			H:      a.Get("horizontal"),
			V:      a.Get("vertical"),
			Wrap:   truthy(a.Get("wrapText")),
			Indent: atoi(a.Get("indent")),
			Rotate: atoi(a.Get("textRotation")),
			Shrink: truthy(a.Get("shrinkToFit")),
		}
		if *st.Align == (Align{}) {
			st.Align = nil
		}
	}
	if p := xf.Child(mainNS, "protection"); p != nil {
		v, set := p.Attr("locked")
		st.Prot = &Protection{Unlocked: set && !truthy(v), Hidden: truthy(p.Get("hidden"))}
		if *st.Prot == (Protection{}) {
			st.Prot = nil
		}
	}
	return st
}

func (s *styles) format(id int) string {
	if code, ok := s.formats[id]; ok {
		return code
	}
	if code, ok := builtinFormats[id]; ok {
		return code
	}
	return "General"
}

func (s *styles) font(e *xmldom.Element) *Font {
	f := &Font{}
	for _, c := range e.Elements() {
		val := c.Get("val")
		switch c.Local {
		case "name":
			f.Name = val
		case "sz":
			f.Size, _ = strconv.ParseFloat(val, 64)
		case "b":
			f.Bold = flag(c)
		case "i":
			f.Italic = flag(c)
		case "strike":
			f.Strike = flag(c)
		case "u":
			if _, ok := c.Attr("val"); !ok {
				val = "single"
			}
			if val != "none" {
				f.Underline = val
			}
		case "color":
			f.Color = s.color(c)
		case "vertAlign":
			if val != "baseline" {
				f.VertAlign = val
			}
		case "scheme":
			if val != "none" {
				f.Scheme = val
			}
		}
	}
	return f
}

func (s *styles) fill(e *xmldom.Element) *Fill {
	if g := e.Child(mainNS, "gradientFill"); g != nil {
		f := &Fill{}
		for _, stop := range elements(g, "stop") {
			if c := stop.Child(mainNS, "color"); c != nil {
				f.Gradient = append(f.Gradient, s.color(c))
			}
		}
		return f
	}
	p := e.Child(mainNS, "patternFill")
	if p == nil {
		return nil
	}
	pattern, set := p.Attr("patternType")
	if !set && p.Child(mainNS, "fgColor") != nil {
		pattern = "solid"
	}
	if pattern == "" || pattern == "none" {
		return nil
	}
	f := &Fill{Pattern: pattern}
	if c := p.Child(mainNS, "fgColor"); c != nil {
		f.Fg = s.color(c)
	}
	if c := p.Child(mainNS, "bgColor"); c != nil {
		f.Bg = s.color(c)
	}
	return f
}

func (s *styles) border(e *xmldom.Element) *Border {
	b := &Border{Up: truthy(e.Get("diagonalUp")), Down: truthy(e.Get("diagonalDown"))}
	edge := func(local string) *Edge {
		c := e.Child(mainNS, local)
		if c == nil || c.Get("style") == "" || c.Get("style") == "none" {
			return nil
		}
		ed := &Edge{Style: c.Get("style")}
		if col := c.Child(mainNS, "color"); col != nil {
			ed.Color = s.color(col)
		}
		return ed
	}
	b.Left, b.Right, b.Top, b.Bottom, b.Diagonal = edge("left"), edge("right"), edge("top"), edge("bottom"), edge("diagonal")
	if b.Left == nil {
		b.Left = edge("start")
	}
	if b.Right == nil {
		b.Right = edge("end")
	}
	if *b == (Border{}) {
		return nil
	}
	return b
}

// color reads a color element: indexed colors are resolved by the palette,
// the last two of which are the system's text and background.
func (s *styles) color(e *xmldom.Element) *Color {
	c := &Color{}
	switch {
	case truthy(e.Get("auto")):
		c.Auto = true
	case e.Get("rgb") != "":
		c.RGB = rgb(e.Get("rgb"))
	case e.Get("theme") != "":
		t := atoi(e.Get("theme"))
		c.Theme = &t
	case e.Get("indexed") != "":
		i := atoi(e.Get("indexed"))
		if i < len(s.palette) {
			c.RGB = s.palette[i]
		} else {
			c.Auto = true
		}
	default:
		return nil
	}
	if t := e.Get("tint"); t != "" {
		c.Tint, _ = strconv.ParseFloat(t, 64)
	}
	return c
}

// rgb is an ARGB or RGB hex color as RRGGBB.
func rgb(s string) string {
	if len(s) == 8 {
		s = s[2:]
	}
	return strings.ToUpper(s)
}

func flag(e *xmldom.Element) bool {
	v, ok := e.Attr("val")
	return !ok || truthy(v)
}

func truthy(v string) bool {
	return v == "1" || v == "true"
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func elements(e *xmldom.Element, local string) []*xmldom.Element {
	var out []*xmldom.Element
	if e == nil {
		return out
	}
	for _, c := range e.Elements() {
		if c.Space == mainNS && c.Local == local {
			out = append(out, c)
		}
	}
	return out
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

// builtinFormats are the number formats Excel does not write out, as the
// French version of Excel shows them.
var builtinFormats = map[int]string{
	0: "General", 1: "0", 2: "0.00", 3: "#,##0", 4: "#,##0.00",
	5: `#,##0 "€";-#,##0 "€"`, 6: `#,##0 "€";[Red]-#,##0 "€"`,
	7: `#,##0.00 "€";-#,##0.00 "€"`, 8: `#,##0.00 "€";[Red]-#,##0.00 "€"`,
	9: "0%", 10: "0.00%", 11: "0.00E+00", 12: "# ?/?", 13: "# ??/??",
	14: "dd/mm/yyyy", 15: "d-mmm-yy", 16: "d-mmm", 17: "mmm-yy",
	18: "h:mm AM/PM", 19: "h:mm:ss AM/PM", 20: "h:mm", 21: "h:mm:ss", 22: "dd/mm/yyyy hh:mm",
	37: "#,##0 ;(#,##0)", 38: "#,##0 ;[Red](#,##0)", 39: "#,##0.00;(#,##0.00)", 40: "#,##0.00;[Red](#,##0.00)",
	45: "mm:ss", 46: "[h]:mm:ss", 47: "mmss.0", 48: "##0.0E+0", 49: "@",
}

// defaultPalette are the indexed colors of a workbook that does not list
// its own; 64 and 65 are the system's text and background.
var defaultPalette = [...]string{
	"000000", "FFFFFF", "FF0000", "00FF00", "0000FF", "FFFF00", "FF00FF", "00FFFF",
	"000000", "FFFFFF", "FF0000", "00FF00", "0000FF", "FFFF00", "FF00FF", "00FFFF",
	"800000", "008000", "000080", "808000", "800080", "008080", "C0C0C0", "808080",
	"9999FF", "993366", "FFFFCC", "CCFFFF", "660066", "FF8080", "0066CC", "CCCCFF",
	"000080", "FF00FF", "FFFF00", "00FFFF", "800080", "800000", "008080", "0000FF",
	"00CCFF", "CCFFFF", "CCFFCC", "FFFF99", "99CCFF", "FF99CC", "CC99FF", "FFCC99",
	"3366FF", "33CCCC", "99CC00", "FFCC00", "FF9900", "FF6600", "666699", "969696",
	"003366", "339966", "003300", "333300", "993300", "993366", "333399", "333333",
}
