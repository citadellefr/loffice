package legacy

import (
	"strings"

	"github.com/citadellefr/loffice/xlsx"
)

// The kinds of values a condition of a filter compares with, and its
// operator "equal".
const (
	operString = 0x06
	operBlanks = 0x0C
	operEqual  = 2
	filterOr   = 1
	filterTop  = 0x10
)

// autoFilter reads what a column of a filter shows. Only the values a
// column is equal to, and its blanks, are kept: the other conditions are
// told as lost.
func (s *sheet) autoFilter(d []byte) {
	if len(d) < 24 {
		s.b.lose(Filters)
		return
	}
	col, flags := int(le.Uint16(d)), le.Uint16(d[2:])
	if flags&filterTop != 0 || col >= biffCols {
		s.b.lose(Filters)
		return
	}
	column := xlsx.FilterColumn{Col: col}
	strs := d[24:]
	conds := 0
	for _, o := range [][]byte{d[4:14], d[14:24]} {
		switch o[0] {
		case 0:
			continue
		case operBlanks:
			column.Blank = true
		case operString:
			text, size, _ := text(strs, int(o[6]))
			if size == 0 || o[1] != operEqual || strings.ContainsAny(text, "*?~") {
				s.b.lose(Filters)
				return
			}
			column.Vals, strs = append(column.Vals, text), strs[size:]
		default:
			s.b.lose(Filters)
			return
		}
		conds++
	}
	if conds == 2 && flags&3 != filterOr || conds == 0 {
		s.b.lose(Filters)
		return
	}
	s.filterColumns = append(s.filterColumns, column)
}
