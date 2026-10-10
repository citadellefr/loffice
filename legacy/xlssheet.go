package legacy

import (
	"math"
	"slices"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

// cell are the fields of a cell, as package xlsx names them.
type cell struct {
	E  string  `json:"e,omitempty"`
	F  string  `json:"f,omitempty"`
	FA string  `json:"fa,omitempty"`
	M  *[2]int `json:"m,omitempty"`
	S  string  `json:"s,omitempty"`
	V  any     `json:"v,omitempty"`
}

// line are the fields of a row or of a column.
type line struct {
	CH   bool    `json:"ch,omitempty"`
	CL   bool    `json:"cl,omitempty"`
	CW   bool    `json:"cw,omitempty"`
	H    float64 `json:"h,omitempty"`
	Hide bool    `json:"hide,omitempty"`
	OL   int     `json:"ol,omitempty"`
	S    string  `json:"s,omitempty"`
	W    float64 `json:"w,omitempty"`
}

type frozen struct {
	Rows int `json:"r,omitempty"`
	Cols int `json:"c,omitempty"`
}

// The sizes a sheet of today takes.
const (
	maxWidth  = 255
	maxHeight = 409.5
)

// The kinds of drawings that are not reported as such: a chart is one
// of its own, the arrow of a filter comes back with its filter, and a
// comment is told by its note.
const (
	objChart    = 0x05
	objDropdown = 0x14
	objNote     = 0x19
)

type sheet struct {
	b     *book
	attrs ot.Values
	// cells and lines are placed as in a grid: rows and columns counted
	// from 1, the fields of a row in column 0, those of a column in row 0.
	cells map[[2]int]*cell
	lines map[[2]int]*line

	// formulas are those read, written once the sheet is: a formula may
	// be shared by the cells of an area or belong to an array, told by a
	// record after the first cell that holds it.
	formulas []cellFormula
	last     [2]int
	shared   map[[2]int][]byte
	arrays   map[[2]int]arrayFormula
	// text is the cell whose formula gives a string, in the next record.
	text *cell

	isFrozen bool
	pane     frozen
	filtered bool
}

type cellFormula struct {
	at          [2]int
	code, extra []byte
}

type arrayFormula struct {
	area        string
	code, extra []byte
}

// sheet reads the worksheet at an index of the workbook.
func (b *book) sheet(stream []byte, index int, key string) (ot.Change, error) {
	bound := b.sheets[index]
	if int64(bound.offset) >= int64(len(stream)) {
		return ot.Change{}, ErrInvalid
	}
	r := records{stream[bound.offset:]}
	if bof, ok := r.next(); !ok || bof.id != recBOF {
		return ot.Change{}, ErrInvalid
	}
	s := &sheet{
		b:      b,
		attrs:  ot.Values{"name": mustJSON(bound.name)},
		cells:  map[[2]int]*cell{},
		lines:  map[[2]int]*line{},
		shared: map[[2]int][]byte{},
		arrays: map[[2]int]arrayFormula{},
	}
	switch bound.state {
	case 1:
		s.attrs["state"] = mustJSON("hidden")
	case 2:
		s.attrs["state"] = mustJSON("veryHidden")
	}
	// a chart on the sheet is a substream of its own inside it
	for depth := 0; ; {
		rec, ok := r.next()
		if !ok {
			break
		}
		switch {
		case rec.id == recBOF:
			depth++
			b.lose(Charts)
		case rec.id == recEOF && depth > 0:
			depth--
		case rec.id == recEOF:
			return s.change(index, key)
		case depth == 0:
			s.read(rec.id, rec.data)
		}
		if b.cells > MaxCells {
			return ot.Change{}, ErrTooLarge
		}
	}
	return s.change(index, key)
}

func (s *sheet) cell(d []byte) *cell {
	at := [2]int{int(le.Uint16(d)) + 1, int(le.Uint16(d[2:])) + 1}
	return s.at(at, int(le.Uint16(d[4:])))
}

func (s *sheet) at(at [2]int, xf int) *cell {
	c := s.get(at)
	c.S = s.b.xf(xf)
	return c
}

// get is the cell at a place, made when the sheet has none there. Past
// MaxCells it is one the sheet does not keep.
func (s *sheet) get(at [2]int) *cell {
	c := s.cells[at]
	if c == nil {
		c = &cell{}
		if s.b.cells++; s.b.cells <= MaxCells {
			s.cells[at] = c
		}
	}
	return c
}

func (s *sheet) line(row, col int) *line {
	l := s.lines[[2]int{row, col}]
	if l == nil {
		l = &line{}
		s.lines[[2]int{row, col}] = l
	}
	return l
}

func (s *sheet) read(id uint16, d []byte) {
	b := s.b
	switch id {
	case recBlank:
		if len(d) >= 6 {
			s.cell(d)
		}
	case recMulBlank:
		for i := 0; len(d) >= 6+2*i+2; i++ {
			s.at([2]int{int(le.Uint16(d)) + 1, int(le.Uint16(d[2:])) + 1 + i}, int(le.Uint16(d[4+2*i:])))
		}
	case recNumber:
		if len(d) >= 14 {
			s.cell(d).number(math.Float64frombits(le.Uint64(d[6:])))
		}
	case recRK:
		if len(d) >= 10 {
			s.cell(d).number(rk(le.Uint32(d[6:])))
		}
	case recMulRK:
		for i := 0; len(d) >= 4+6*i+6+2; i++ {
			e := d[4+6*i:]
			s.at([2]int{int(le.Uint16(d)) + 1, int(le.Uint16(d[2:])) + 1 + i}, int(le.Uint16(e))).number(rk(le.Uint32(e[2:])))
		}
	case recLabelSST:
		if len(d) >= 10 {
			if i := le.Uint32(d[6:]); int64(i) < int64(len(b.strings)) {
				s.cell(d).V = b.strings[i]
			}
		}
	case recLabel, recRString:
		if len(d) >= 8 {
			if text, size, rich := longText(d[6:]); size > 0 {
				s.cell(d).V = text
				if rich || id == recRString {
					b.lose(RichText)
				}
			}
		}
	case recBoolErr:
		if len(d) >= 8 {
			if c := s.cell(d); d[7] != 0 {
				c.E = errorName(d[6])
			} else {
				c.V = d[6] != 0
			}
		}
	case recFormula:
		s.formula(d)
	case recString:
		if s.text != nil {
			if text, size, _ := longText(d); size > 0 {
				s.text.V = text
			}
			s.text = nil
		}
	case recSharedFormula:
		if len(d) >= 10 && 10+int(le.Uint16(d[8:])) <= len(d) {
			s.shared[s.last] = d[10 : 10+int(le.Uint16(d[8:]))]
		}
	case recArray:
		if len(d) >= 14 && 14+int(le.Uint16(d[12:])) <= len(d) {
			size := int(le.Uint16(d[12:]))
			area := formula.Area{R1: int(le.Uint16(d)) + 1, R2: int(le.Uint16(d[2:])) + 1, C1: int(d[4]) + 1, C2: int(d[5]) + 1, Cell: true}
			s.arrays[s.last] = arrayFormula{area.String(), d[14 : 14+size], d[14+size:]}
		}
	case recMergedCells:
		for d = d[min(2, len(d)):]; len(d) >= 8; d = d[8:] {
			r1, r2, c1, c2 := int(le.Uint16(d)), int(le.Uint16(d[2:])), int(le.Uint16(d[4:])), int(le.Uint16(d[6:]))
			if r2 < r1 || c2 < c1 || c2 >= biffCols || r1 == r2 && c1 == c2 {
				continue
			}
			s.get([2]int{r1 + 1, c1 + 1}).M = &[2]int{r2 - r1 + 1, c2 - c1 + 1}
		}
	case recRow:
		if len(d) >= 16 {
			l := s.line(int(le.Uint16(d))+1, 0)
			height, flags := le.Uint16(d[6:]), le.Uint32(d[12:])
			if height&0x8000 == 0 && height > 0 {
				l.H = min(float64(height)/20, maxHeight)
				l.CH = flags&0x40 != 0
			}
			l.OL, l.CL, l.Hide = int(flags&0x07), flags&0x10 != 0, flags&0x20 != 0
			if flags&0x80 != 0 {
				l.S = b.xf(int(flags >> 16 & 0x0FFF))
			}
		}
	case recColInfo:
		if len(d) >= 10 {
			width, flags := le.Uint16(d[4:]), le.Uint16(d[8:])
			f := line{Hide: flags&0x01 != 0, OL: int(flags >> 8 & 0x07), CL: flags&0x1000 != 0, S: b.xf(int(le.Uint16(d[6:])))}
			if width > 0 {
				f.W, f.CW = min(float64(width)/256, maxWidth), true
			}
			for col := int(le.Uint16(d)); col <= int(le.Uint16(d[2:])) && col < biffCols; col++ {
				*s.line(0, col+1) = f
			}
		}
	case recDefColWidth:
		// in characters, to which Excel adds the room it leaves around them
		if len(d) >= 2 && le.Uint16(d) != 8 && le.Uint16(d) <= maxWidth-1 && s.attrs["dw"] == nil {
			s.attrs["dw"] = mustJSON(float64(le.Uint16(d)) + 5.0/7)
		}
	case recStandardWidth:
		if len(d) >= 2 && le.Uint16(d) > 0 {
			s.attrs["dw"] = mustJSON(min(float64(le.Uint16(d))/256, maxWidth))
		}
	case recDefRowHeight:
		if len(d) >= 4 && le.Uint16(d[2:]) > 0 {
			s.attrs["dh"] = mustJSON(min(float64(le.Uint16(d[2:]))/20, maxHeight))
		}
	case recWindow2:
		if len(d) >= 2 {
			flags := le.Uint16(d)
			if flags&0x02 == 0 {
				s.attrs["grid"] = mustJSON(false)
			}
			if flags&0x40 != 0 {
				s.attrs["rtl"] = mustJSON(true)
			}
			s.isFrozen = flags&0x08 != 0
		}
	case recPane:
		if len(d) >= 4 {
			s.pane = frozen{Rows: int(le.Uint16(d[2:])), Cols: int(le.Uint16(d))}
		}
	case recScl:
		if len(d) >= 4 && le.Uint16(d[2:]) > 0 {
			if zoom := int(le.Uint16(d)) * 100 / int(le.Uint16(d[2:])); zoom >= 10 && zoom <= 400 && zoom != 100 {
				s.attrs["zoom"] = mustJSON(zoom)
			}
		}
	case recSheetExt:
		if len(d) >= 20 {
			if c := b.color(int(le.Uint32(d[16:]) & 0x7F)); c != nil && !c.Auto {
				s.attrs["tab"] = mustJSON(c)
			}
		}
	case recAutoFilterInfo:
		s.filtered = true
	case recAutoFilter:
		b.lose(Filters)
	case recNote:
		b.lose(Comments)
	case recHyperlink:
		b.lose(Links)
	case recCondFmt:
		b.lose(Conditional)
	case recValidation:
		b.lose(Validation)
	case recPivotView:
		b.lose(Pivots)
	case recHPageBreaks, recVPageBreaks:
		b.lose(Print)
	case recHeader, recFooter:
		if len(d) > 0 {
			b.lose(Print)
		}
	case recProtect:
		if len(d) >= 2 && le.Uint16(d) == 1 {
			b.lose(Protection)
		}
	case recObj:
		if len(d) >= 6 {
			switch le.Uint16(d[4:]) {
			case objChart:
				b.lose(Charts)
			case objDropdown, objNote:
			default:
				b.lose(Drawings)
			}
		}
	}
}

// number gives the cell a number, unless it is one no cell can show.
func (c *cell) number(f float64) {
	if !math.IsNaN(f) && !math.IsInf(f, 0) {
		c.V = f
	}
}

// formula reads a cell that holds a formula, and the value it last gave.
func (s *sheet) formula(d []byte) {
	if len(d) < 22 {
		return
	}
	c := s.cell(d)
	s.last = [2]int{int(le.Uint16(d)) + 1, int(le.Uint16(d[2:])) + 1}
	s.text = nil
	if value := d[6:14]; le.Uint16(value[6:]) != 0xFFFF {
		c.number(math.Float64frombits(le.Uint64(value)))
	} else {
		switch value[0] {
		case 0:
			s.text = c
		case 1:
			c.V = value[2] != 0
		case 2:
			c.E = errorName(value[2])
		case 3:
			c.V = ""
		}
	}
	size := int(le.Uint16(d[20:]))
	if 22+size > len(d) {
		s.b.lose(Formulas)
		return
	}
	s.formulas = append(s.formulas, cellFormula{s.last, d[22 : 22+size], d[22+size:]})
}

// write gives the cells their formulas.
func (s *sheet) write() {
	for _, f := range s.formulas {
		c := s.cells[f.at]
		if c == nil {
			continue
		}
		code, extra := f.code, f.extra
		if len(code) == 5 && code[0] == ptgExp {
			first := [2]int{int(le.Uint16(code[1:])) + 1, int(le.Uint16(code[3:])) + 1}
			if array, ok := s.arrays[first]; ok {
				// the other cells of an array only hold what it gives them
				if first != f.at {
					continue
				}
				code, extra, c.FA = array.code, array.extra, array.area
			} else if code, ok = s.shared[first]; ok {
				extra = nil
			}
		}
		text, err := s.b.formula(code, extra, origin{row: f.at[0] - 1, col: f.at[1] - 1, rows: biffRows, cols: biffCols})
		if err != nil {
			c.FA = ""
			s.b.loseFormula(err)
			continue
		}
		c.F = text
	}
}

// change creates the sheet.
func (s *sheet) change(index int, key string) (ot.Change, error) {
	s.write()
	if s.isFrozen && s.pane != (frozen{}) {
		s.attrs["frozen"] = mustJSON(s.pane)
	}
	if ref := s.b.filters[index]; s.filtered && ref != "" {
		// the name of the area is written 'Sheet'!$A$1:$C$9
		if a, ok := formula.ParseArea(strings.ReplaceAll(ref[strings.LastIndexByte(ref, '!')+1:], "$", "")); ok {
			s.attrs["filter"] = mustJSON(xlsx.AutoFilter{Ref: a.String()})
		}
	}
	cells := make([]ot.Cell, 0, len(s.lines)+len(s.cells))
	for at, l := range s.lines {
		if *l != (line{}) {
			cells = append(cells, ot.Cell{Row: at[0], Col: at[1], Fields: mustJSON(l)})
		}
	}
	for at, c := range s.cells {
		if fields := mustJSON(c); len(fields) > 2 {
			cells = append(cells, ot.Cell{Row: at[0], Col: at[1], Fields: fields})
			if s.b.size += len(fields); s.b.size > xlsx.MaxCellData {
				return ot.Change{}, ErrTooLarge
			}
		}
	}
	slices.SortFunc(cells, func(a, b ot.Cell) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return ot.Change{Op: ot.OpNew, ID: sheetID(index), Type: "sheet", Parent: "book", Key: key, Attrs: s.attrs, Cells: cells}, nil
}
