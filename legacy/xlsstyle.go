package legacy

import (
	"encoding/json"
	"strconv"

	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

// What the numbers of a format stand for, in the words of an .xlsx.
var (
	patterns = [...]string{"none", "solid", "mediumGray", "darkGray", "lightGray", "darkHorizontal", "darkVertical",
		"darkDown", "darkUp", "darkGrid", "darkTrellis", "lightHorizontal", "lightVertical", "lightDown", "lightUp",
		"lightGrid", "lightTrellis", "gray125", "gray0625"}
	borderStyles = [...]string{"none", "thin", "medium", "dashed", "dotted", "thick", "double", "hair", "mediumDashed",
		"dashDot", "mediumDashDot", "dashDotDot", "mediumDashDotDot", "slantDashDot"}
	horizontals = [...]string{"", "left", "center", "right", "fill", "justify", "centerContinuous", "distributed"}
	verticals   = [...]string{"top", "center", "", "justify", "distributed", "", "", ""}
	underlines  = map[byte]string{1: "single", 2: "double", 0x21: "singleAccounting", 0x22: "doubleAccounting"}
)

// The colors past the palette: the text and the background of the system.
const (
	colorText       = 64
	colorBackground = 65
)

// color is a color of the palette, nil for the automatic one.
func (b *book) color(i int) *xlsx.Color {
	switch {
	case i < 8:
		return &xlsx.Color{RGB: xlsx.IndexedColor(i)}
	case i < colorText:
		return &xlsx.Color{RGB: b.palette[i-8]}
	case i == colorText || i == colorBackground:
		return &xlsx.Color{Auto: true}
	}
	return nil
}

// font is the font at an index of the file, which counts none at 4.
func (b *book) font(i int) *xlsx.Font {
	if i > 4 {
		i--
	}
	if i >= len(b.fonts) || len(b.fonts[i]) < 15 {
		return nil
	}
	d := b.fonts[i]
	flags := le.Uint16(d[2:])
	f := &xlsx.Font{
		Size:      min(float64(le.Uint16(d))/20, 409),
		Italic:    flags&0x02 != 0,
		Strike:    flags&0x08 != 0,
		Bold:      le.Uint16(d[6:]) >= 600,
		Color:     b.color(int(le.Uint16(d[4:]))),
		Underline: underlines[d[10]],
	}
	switch le.Uint16(d[8:]) {
	case 1:
		f.VertAlign = "superscript"
	case 2:
		f.VertAlign = "subscript"
	}
	f.Name, _ = shortText(d[14:])
	if len(f.Name) > 31 {
		f.Name = ""
	}
	return f
}

// style is the cell format at an index of the file.
func (b *book) style(index int) (xlsx.Style, bool) {
	var st xlsx.Style
	if index >= len(b.xfs) || len(b.xfs[index]) < 20 {
		return st, false
	}
	d := b.xfs[index]
	st.Font = b.font(int(le.Uint16(d)))
	if id := int(le.Uint16(d[2:])); id != 0 {
		if code, ok := b.formats[id]; ok {
			st.Format = code
		} else {
			st.Format = xlsx.BuiltinFormat(id)
		}
		if st.Format == "General" {
			st.Format = ""
		}
	}
	if flags := le.Uint16(d[4:]); flags&0x03 != 0x01 {
		st.Prot = &xlsx.Protection{Unlocked: flags&0x01 == 0, Hidden: flags&0x02 != 0}
	}
	align := xlsx.Align{
		H:      horizontals[d[6]&0x07],
		V:      verticals[d[6]>>4&0x07],
		Wrap:   d[6]&0x08 != 0,
		Rotate: int(d[7]),
		Indent: int(d[8] & 0x0F),
		Shrink: d[8]&0x10 != 0,
	}
	if align.Rotate > 180 && align.Rotate != 255 {
		align.Rotate = 0
	}
	if align != (xlsx.Align{}) {
		st.Align = &align
	}

	lines, colors, fill := le.Uint32(d[10:]), le.Uint32(d[14:]), le.Uint16(d[18:])
	edge := func(style, color uint32) *xlsx.Edge {
		if style == 0 || int(style) >= len(borderStyles) {
			return nil
		}
		return &xlsx.Edge{Style: borderStyles[style], Color: b.color(int(color & 0x7F))}
	}
	border := xlsx.Border{
		Left:     edge(lines&0x0F, lines>>16),
		Right:    edge(lines>>4&0x0F, lines>>23),
		Top:      edge(lines>>8&0x0F, colors),
		Bottom:   edge(lines>>12&0x0F, colors>>7),
		Diagonal: edge(colors>>21&0x0F, colors>>14),
		Down:     lines&(1<<30) != 0,
		Up:       lines&(1<<31) != 0,
	}
	if border.Diagonal == nil {
		border.Up, border.Down = false, false
	}
	if border != (xlsx.Border{}) {
		st.Border = &border
	}
	if p := int(colors >> 26); p > 0 && p < len(patterns) {
		st.Fill = &xlsx.Fill{Pattern: patterns[p], Fg: b.color(int(fill & 0x7F)), Bg: b.color(int(fill >> 7 & 0x7F))}
	}
	return st, true
}

// xf is the node of the cell format at an index of the file, made when a
// cell first takes it: "" when the file has no such format.
func (b *book) xf(index int) string {
	if id, ok := b.xfIDs[index]; ok {
		return id
	}
	id := ""
	if st, ok := b.style(index); ok {
		raw, _ := json.Marshal(st)
		if id = b.styles[string(raw)]; id == "" {
			id = "x" + strconv.Itoa(len(b.styles)+1)
			b.styles[string(raw)] = id
			b.nodes = append(b.nodes, ot.Change{Op: ot.OpNew, ID: id, Type: "xf", Key: "V", Attrs: ot.Values{"style": raw}})
		}
	}
	b.xfIDs[index] = id
	return id
}
