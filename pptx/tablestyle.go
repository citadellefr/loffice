package pptx

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const relTableStyles = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/tableStyles"

// TableStyle is how a style paints the parts of a table: "wholeTbl",
// "band1H" "band2H" "band1V" "band2V", "firstRow" "lastRow" "firstCol"
// "lastCol", the corners "nwCell" "neCell" "swCell" "seCell", and the
// background under all the cells, "tblBg".
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
// default one.
func tableStyles(lst *xmldom.Element, media func(string) string) (map[string]TableStyle, string) {
	out := map[string]TableStyle{}
	if lst == nil {
		return out, ""
	}
	for _, s := range elements(lst, aNS, "tblStyle") {
		if id := s.Get("styleId"); id != "" {
			out[id] = readTableStyle(s, media)
		}
	}
	return out, lst.Get("def")
}

func readTableStyle(s *xmldom.Element, media func(string) string) TableStyle {
	style := TableStyle{}
	for _, name := range tableParts {
		if e := s.Child(aNS, name); e != nil {
			style[name] = readTablePart(e, media)
		}
	}
	if bg := s.Child(aNS, "tblBg"); bg != nil {
		p := &TableStylePart{}
		readCellStyle(p, bg, media)
		style["tblBg"] = p
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
	if st := e.Child(aNS, "tcStyle"); st != nil {
		readCellStyle(p, st, media)
	}
	return p
}

// readCellStyle reads the fill and borders of a tcStyle, or the fill of a
// tblBg.
func readCellStyle(p *TableStylePart, st *xmldom.Element, media func(string) string) {
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
}

func onOff(s string) string {
	if s == "on" || s == "off" {
		return s
	}
	return ""
}

// mediumStyle2 is Office's default table style, Medium Style 2 - Accent 1.
const mediumStyle2 = "{5C22544A-7EE6-4342-B048-85BDC9FD1C3A}"

// builtinJSON are Office's built-in table styles, which a presentation
// may name without holding them; see TestBuiltinStyles.
//
//go:embed tablestyles.json
var builtinJSON []byte

var builtin = sync.OnceValue(func() map[string]json.RawMessage {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(builtinJSON, &m)
	return m
})

// addBuiltin adds to styles the built-in styles of ids it lacks.
func addBuiltin(styles map[string]TableStyle, ids ...string) {
	for _, id := range ids {
		if _, ok := styles[id]; ok {
			continue
		}
		var style TableStyle
		if json.Unmarshal(builtin()[id], &style) == nil {
			styles[id] = style
		}
	}
}
