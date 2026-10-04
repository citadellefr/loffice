package pptx

import (
	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const relTableStyles = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/tableStyles"

// TableStyle is how a style paints the parts of a table: "wholeTbl",
// "band1H" "band2H" "band1V" "band2V", "firstRow" "lastRow" "firstCol"
// "lastCol" and the corners "nwCell" "neCell" "swCell" "seCell".
type TableStyle map[string]*TableStylePart

// TableStylePart is the text and the cells of a part of a table.
type TableStylePart struct {
	// B and I are "on" or "off" when the part sets them.
	B string `json:"b,omitempty"`
	I string `json:"i,omitempty"`
	// Font is a typeface, or "+mn-lt" and "+mj-lt" for the theme's.
	Font    string           `json:"font,omitempty"`
	Color   *drawingml.Color `json:"color,omitempty"`
	Fill    *drawingml.Fill  `json:"fill,omitempty"`
	FillRef *StyleRef        `json:"fillRef,omitempty"`
	// Borders are by edge: "left" "right" "top" "bottom", "insideH"
	// "insideV" between the cells of the part, "tl2br" "tr2bl" across.
	Borders map[string]*Border `json:"borders,omitempty"`
}

// Border is a line of its own or one of the theme's.
type Border struct {
	Line *drawingml.Line `json:"ln,omitempty"`
	Ref  *StyleRef       `json:"ref,omitempty"`
}

var tableParts = []string{"wholeTbl", "band1H", "band2H", "band1V", "band2V", "lastCol", "firstCol", "lastRow", "firstRow", "seCell", "swCell", "neCell", "nwCell"}

// tableStyles reads the styles of a tableStyles part, by id, and the
// default one; Office's default style is there even when the part lacks
// it.
func tableStyles(lst *xmldom.Element, media func(string) string) (map[string]TableStyle, string) {
	out := map[string]TableStyle{}
	var def string
	if lst != nil {
		def = lst.Get("def")
		for _, s := range elements(lst, aNS, "tblStyle") {
			if id := s.Get("styleId"); id != "" {
				out[id] = readTableStyle(s, media)
			}
		}
	}
	if _, ok := out[mediumStyle2]; !ok {
		e, _ := xmldom.ParseFragment([]byte(mediumStyle2XML), map[string]string{"a": aNS})
		out[mediumStyle2] = readTableStyle(e, media)
	}
	return out, def
}

func readTableStyle(s *xmldom.Element, media func(string) string) TableStyle {
	style := TableStyle{}
	for _, name := range tableParts {
		if e := s.Child(aNS, name); e != nil {
			style[name] = readTablePart(e, media)
		}
	}
	return style
}

func readTablePart(e *xmldom.Element, media func(string) string) *TableStylePart {
	p := &TableStylePart{}
	if tx := e.Child(aNS, "tcTxStyle"); tx != nil {
		p.B, p.I = onOff(tx.Get("b")), onOff(tx.Get("i"))
		p.Color = drawingml.ColorIn(tx)
		if ref := tx.Child(aNS, "fontRef"); ref != nil {
			switch ref.Get("idx") {
			case "major":
				p.Font = "+mj-lt"
			case "minor":
				p.Font = "+mn-lt"
			}
			if p.Color == nil {
				p.Color = drawingml.ColorIn(ref)
			}
		}
		if font := tx.Child(aNS, "font"); font != nil {
			if latin := font.Child(aNS, "latin"); latin != nil {
				p.Font = latin.Get("typeface")
			}
		}
	}
	st := e.Child(aNS, "tcStyle")
	if st == nil {
		return p
	}
	if f := st.Child(aNS, "fill"); f != nil {
		p.Fill = drawingml.FillIn(f, media)
	}
	if ref := st.Child(aNS, "fillRef"); ref != nil {
		p.FillRef = &StyleRef{Idx: ref.Get("idx"), Color: drawingml.ColorIn(ref)}
	}
	if bdr := st.Child(aNS, "tcBdr"); bdr != nil {
		for _, edge := range bdr.Elements() {
			b := &Border{Line: drawingml.ReadLine(edge.Child(aNS, "ln"))}
			if ref := edge.Child(aNS, "lnRef"); ref != nil {
				b.Ref = &StyleRef{Idx: ref.Get("idx"), Color: drawingml.ColorIn(ref)}
			}
			if b.Line == nil && b.Ref == nil {
				continue
			}
			if p.Borders == nil {
				p.Borders = map[string]*Border{}
			}
			p.Borders[edge.Local] = b
		}
	}
	return p
}

func onOff(s string) string {
	if s == "on" || s == "off" {
		return s
	}
	return ""
}

// mediumStyle2 is Office's default table style, Medium Style 2 - Accent 1.
const mediumStyle2 = "{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"

const mediumStyle2XML = `<a:tblStyle xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" styleId="{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}" styleName="Medium Style 2 - Accent 1">` +
	`<a:wholeTbl><a:tcTxStyle><a:fontRef idx="minor"><a:prstClr val="black"/></a:fontRef><a:schemeClr val="dk1"/></a:tcTxStyle><a:tcStyle><a:tcBdr>` +
	`<a:left><a:ln w="12700" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:left>` +
	`<a:right><a:ln w="12700" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:right>` +
	`<a:top><a:ln w="12700" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:top>` +
	`<a:bottom><a:ln w="12700" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:bottom>` +
	`<a:insideH><a:ln w="12700" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:insideH>` +
	`<a:insideV><a:ln w="12700" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:insideV>` +
	`</a:tcBdr><a:fill><a:solidFill><a:schemeClr val="accent1"><a:tint val="20000"/></a:schemeClr></a:solidFill></a:fill></a:tcStyle></a:wholeTbl>` +
	`<a:band1H><a:tcStyle><a:tcBdr/><a:fill><a:solidFill><a:schemeClr val="accent1"><a:tint val="40000"/></a:schemeClr></a:solidFill></a:fill></a:tcStyle></a:band1H>` +
	`<a:band2H><a:tcStyle><a:tcBdr/></a:tcStyle></a:band2H>` +
	`<a:band1V><a:tcStyle><a:tcBdr/><a:fill><a:solidFill><a:schemeClr val="accent1"><a:tint val="40000"/></a:schemeClr></a:solidFill></a:fill></a:tcStyle></a:band1V>` +
	`<a:band2V><a:tcStyle><a:tcBdr/></a:tcStyle></a:band2V>` +
	`<a:lastCol><a:tcTxStyle b="on"><a:fontRef idx="minor"><a:prstClr val="black"/></a:fontRef><a:schemeClr val="lt1"/></a:tcTxStyle><a:tcStyle><a:tcBdr/><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill></a:tcStyle></a:lastCol>` +
	`<a:firstCol><a:tcTxStyle b="on"><a:fontRef idx="minor"><a:prstClr val="black"/></a:fontRef><a:schemeClr val="lt1"/></a:tcTxStyle><a:tcStyle><a:tcBdr/><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill></a:tcStyle></a:firstCol>` +
	`<a:lastRow><a:tcTxStyle b="on"><a:fontRef idx="minor"><a:prstClr val="black"/></a:fontRef><a:schemeClr val="lt1"/></a:tcTxStyle><a:tcStyle><a:tcBdr><a:top><a:ln w="38100" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:top></a:tcBdr><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill></a:tcStyle></a:lastRow>` +
	`<a:firstRow><a:tcTxStyle b="on"><a:fontRef idx="minor"><a:prstClr val="black"/></a:fontRef><a:schemeClr val="lt1"/></a:tcTxStyle><a:tcStyle><a:tcBdr><a:bottom><a:ln w="38100" cmpd="sng"><a:solidFill><a:schemeClr val="lt1"/></a:solidFill></a:ln></a:bottom></a:tcBdr><a:fill><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></a:fill></a:tcStyle></a:firstRow>` +
	`</a:tblStyle>`
