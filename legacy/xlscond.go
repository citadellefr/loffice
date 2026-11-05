package legacy

import (
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/xlsx"
)

// The blocks of a format that a CF holds, by the flag that tells it is
// there, and the flags that tell what the blocks leave alone.
const (
	cfNumber      = 0x02000000
	cfFont        = 0x04000000
	cfAlign       = 0x08000000
	cfBorder      = 0x10000000
	cfArea        = 0x20000000
	cfNoFormat    = 0x00080000
	cfNoPattern   = 0x00010000
	cfNoFore      = 0x00020000
	cfNoBack      = 0x00040000
	cfNoStyle     = 0x00000002
	cfNoStrike    = 0x00000080
	cfNoUnderline = 0x00000001
	cfItalic      = 0x00000002
	cfStrike      = 0x00000080
	cfFontSize    = 118
	cfAlignSize   = 8
	cfUserFormat  = 0x0001
	cfRuleCell    = 1
	cfRuleFormula = 2
	// cfFirstExtended is the first kind of rule only a CF12 can hold:
	// scales, bars, icons and the rules on the values of a range.
	cfFirstExtended = 3
)

// cfOperators are the operators of a rule on the value of a cell, as the
// rules of an .xlsx name them.
var cfOperators = [...]string{"", "between", "notBetween", "equal", "notEqual", "greaterThan", "lessThan",
	"greaterThanOrEqual", "lessThanOrEqual"}

// conditionalFormat is the CFHeader being read: the areas its rules cover,
// and the cell their relative references are counted from.
type conditionalFormat struct {
	ref    string
	origin origin
}

// sqref reads n areas, each told by its first row, its last, its first
// column and its last, as text, and the first of them.
func sqref(d []byte, n int) (string, [4]int, bool) {
	if n < 1 || len(d) < 8*n {
		return "", [4]int{}, false
	}
	var refs []string
	var first [4]int
	for i := range n {
		at, ok := areaOf(d[8*i:])
		if !ok || at[1] >= biffRows {
			return "", first, false
		}
		if i == 0 {
			first = at
		}
		a := origin{}.area(d[8*i:], false)
		a.AbsR1, a.AbsC1, a.AbsR2, a.AbsC2, a.Cell = false, false, false, false, true
		refs = append(refs, a.String())
	}
	return strings.Join(refs, " "), first, true
}

// conditional reads the records of the conditional formats of a sheet:
// CFHeader, which tells the cells, and the CF that follow it, one rule
// each. The extensions of Excel 2007 are left out and told when they are
// what the rule is: CF12 for scales, bars, icons and the rules on the
// values of a range.
func (s *sheet) conditional(id uint16, d []byte) {
	switch id {
	case recCondFmt:
		s.cfLast, s.cf = -1, nil
		if len(d) < 14 {
			s.b.lose(Conditional)
			return
		}
		ref, first, ok := sqref(d[14:], int(le.Uint16(d[12:])))
		if !ok {
			s.b.lose(Conditional)
			return
		}
		s.cf = &conditionalFormat{ref, origin{row: first[0], col: first[2], rows: biffRows, cols: biffCols}}
	case recCF:
		s.cfLast = -1
		if s.cf == nil || !s.rule(d) {
			s.b.lose(Conditional)
		}
	case recCF12:
		if len(d) > 12 && d[12] >= cfFirstExtended {
			if s.cfLast >= 0 {
				s.conditionals = append(s.conditionals[:s.cfLast], s.conditionals[s.cfLast+1:]...)
				s.cfLast = -1
			}
			s.b.lose(Conditional)
		}
	}
}

// rule reads a CF, a rule that holds for the value of a cell or for a
// formula, with the format its cells take.
func (s *sheet) rule(d []byte) bool {
	if len(d) < 12 || len(s.conditionals) >= xlsx.MaxRules {
		return false
	}
	kind, op, size1, size2, flags := d[0], int(d[1]), int(le.Uint16(d[2:])), int(le.Uint16(d[4:])), le.Uint32(d[6:])
	style, d, ok := s.b.conditionalStyle(flags, d[10:])
	if !ok || len(d) < size1+size2 || style == nil {
		return false
	}
	c := xlsx.Conditional{Ref: s.cf.ref, Type: "formula", Pri: len(s.conditionals) + 1, Style: style}
	for _, code := range [][]byte{d[:size1], d[size1 : size1+size2]} {
		if len(code) == 0 {
			continue
		}
		text, err := s.b.formula(code, nil, s.cf.origin)
		if err != nil {
			s.b.loseFormula(err)
			return false
		}
		c.F = append(c.F, text)
	}
	switch kind {
	case cfRuleCell:
		if op >= len(cfOperators) {
			return false
		}
		anchor := formula.CellName(s.cf.origin.row+1, s.cf.origin.col+1)
		f, ok := xlsx.CellIsFormula(cfOperators[op], anchor, c.F)
		if !ok {
			return false
		}
		c.F = []string{f}
	case cfRuleFormula:
	default:
		return false
	}
	if !c.Valid() {
		return false
	}
	s.cfLast = len(s.conditionals)
	s.conditionals = append(s.conditionals, c)
	return true
}

// conditionalStyle reads the blocks of the format a CF holds, and what
// follows them: the flags of the format, then the number format, the font,
// the alignment, the border and the area. It is nil when the rule changes
// nothing.
func (b *book) conditionalStyle(flags uint32, d []byte) (*xlsx.Style, []byte, bool) {
	if len(d) < 2 {
		return nil, nil, false
	}
	user, d := le.Uint16(d)&cfUserFormat != 0, d[2:]
	st := &xlsx.Style{}
	if flags&cfNumber != 0 {
		format, size, ok := b.conditionalFormat(user, d)
		if !ok {
			return nil, nil, false
		}
		if flags&cfNoFormat == 0 {
			st.Format = format
		}
		d = d[size:]
	}
	if flags&cfFont != 0 {
		if len(d) < cfFontSize {
			return nil, nil, false
		}
		st.Font = b.conditionalFont(d[:cfFontSize])
		d = d[cfFontSize:]
	}
	if flags&cfAlign != 0 {
		if len(d) < cfAlignSize {
			return nil, nil, false
		}
		d = d[cfAlignSize:]
	}
	if flags&cfBorder != 0 {
		if len(d) < 8 {
			return nil, nil, false
		}
		st.Border = b.conditionalBorder(flags, d[:8])
		d = d[8:]
	}
	if flags&cfArea != 0 {
		if len(d) < 4 {
			return nil, nil, false
		}
		st.Fill = b.conditionalFill(flags, d[:4])
		d = d[4:]
	}
	if st.Format == "" && st.Font == nil && st.Border == nil && st.Fill == nil {
		return nil, d, true
	}
	return st, d, true
}

// conditionalFormat reads the number format a CF holds, and the bytes it
// takes: the code of a format of the workbook or a built-in one, or the
// format itself.
func (b *book) conditionalFormat(user bool, d []byte) (string, int, bool) {
	if len(d) < 2 {
		return "", 0, false
	}
	if !user {
		return b.format(int(d[1])), 2, true
	}
	code, size, _ := longText(d[2:])
	if size == 0 || int(le.Uint16(d)) < size {
		return "", 0, false
	}
	return code, 2 + size, true
}

// conditionalFont is what the font block changes of a font: the style, the
// weight, the line, the strikeout and the color, unless the block leaves
// them alone.
func (b *book) conditionalFont(d []byte) *xlsx.Font {
	style, weight := le.Uint32(d[68:]), le.Uint16(d[72:])
	color, flags, underline := le.Uint32(d[80:]), le.Uint32(d[88:]), le.Uint32(d[96:])
	f := &xlsx.Font{}
	if flags&cfNoStyle == 0 {
		f.Italic, f.Bold = style&cfItalic != 0, weight >= 600
	}
	f.Strike = flags&cfNoStrike == 0 && style&cfStrike != 0
	if underline&cfNoUnderline == 0 {
		f.Underline = underlines[d[76]]
	}
	if color < colorText {
		f.Color = b.color(int(color))
	}
	if *f == (xlsx.Font{}) {
		return nil
	}
	return f
}

// conditionalBorder is the edges a border block changes.
func (b *book) conditionalBorder(flags uint32, d []byte) *xlsx.Border {
	lines, colors, more := le.Uint16(d), le.Uint16(d[2:]), le.Uint32(d[4:])
	edge := func(shift uint, kept uint32, color uint32) *xlsx.Edge {
		style := lines >> shift & 0x0F
		if flags&kept != 0 || style == 0 || int(style) >= len(borderStyles) {
			return nil
		}
		return &xlsx.Edge{Style: borderStyles[style], Color: b.color(int(color & 0x7F))}
	}
	border := xlsx.Border{
		Left:   edge(0, 0x400, uint32(colors)),
		Right:  edge(4, 0x800, uint32(colors>>7)),
		Top:    edge(8, 0x1000, more),
		Bottom: edge(12, 0x2000, more>>7),
	}
	if border == (xlsx.Border{}) {
		return nil
	}
	return &border
}

// conditionalFill is what the area block changes of the pattern: with none
// of the pattern, as Excel writes a plain color, the background is solid.
func (b *book) conditionalFill(flags uint32, d []byte) *xlsx.Fill {
	pattern, colors := int(le.Uint16(d)>>10), le.Uint16(d[2:])
	fore, back := b.color(int(colors&0x7F)), b.color(int(colors>>7&0x7F))
	setPattern, setFore, setBack := flags&cfNoPattern == 0, flags&cfNoFore == 0, flags&cfNoBack == 0
	switch {
	case setPattern && pattern > 1 && pattern < len(patterns):
		fill := &xlsx.Fill{Pattern: patterns[pattern]}
		if setFore {
			fill.Fg = fore
		}
		if setBack {
			fill.Bg = back
		}
		return fill
	case setPattern && pattern == 1 && setFore:
		return &xlsx.Fill{Pattern: "solid", Fg: fore}
	case setBack:
		return &xlsx.Fill{Pattern: "solid", Fg: back}
	case setFore:
		return &xlsx.Fill{Pattern: "solid", Fg: fore}
	}
	return nil
}
