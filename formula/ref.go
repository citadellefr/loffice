package formula

import (
	"strconv"
	"strings"
)

// The size of a sheet.
const (
	MaxRows = 1 << 20
	MaxCols = 1 << 14
)

// Ref is a reference as a formula writes it.
type Ref struct {
	// Sheet is the sheet it points to, "" for the formula's own, and
	// LastSheet the last of a 3D reference (Sheet1:Sheet3!A1).
	Sheet, LastSheet string
	// Book is the external workbook of "[1]Sheet1!A1", without brackets.
	Book string
	// Invalid is a reference to cells that were deleted: #REF!.
	Invalid bool
	Area    Area
}

// Area is a rectangle of cells, from R1 C1 to R2 C2 counted from 1. Whole
// columns (A:C) span every row and whole rows (1:3) every column.
type Area struct {
	R1, C1, R2, C2             int
	AbsR1, AbsC1, AbsR2, AbsC2 bool
	Cols, Rows                 bool
	// Cell is a single cell written without a colon.
	Cell bool
}

// Contains tells whether the area holds the cell.
func (a Area) Contains(row, col int) bool {
	return a.R1 <= row && row <= a.R2 && a.C1 <= col && col <= a.C2
}

// readRef reads the reference f holds at i, and its length.
func readRef(f string, i int) (*Ref, int, bool) {
	r := &Ref{}
	n := 0
	if p, ok := prefixLen(f[i:]); ok {
		r.Book, r.Sheet, r.LastSheet = splitPrefix(f[i : i+p-1])
		n = p
	}
	rest := f[i+n:]
	if hasPrefixFold(rest, "#REF!") {
		r.Invalid = true
		return r, n + 5, true
	}
	a, m, ok := readArea(rest)
	if !ok || n+m < len(f)-i && (identLen(f[i+n+m:]) > 0 || f[i+n+m] == '(') {
		return nil, 0, false
	}
	r.Area = a
	return r, n + m, true
}

// splitPrefix reads a sheet prefix without its "!".
func splitPrefix(p string) (book, sheet, last string) {
	if strings.HasPrefix(p, "[") {
		end := strings.IndexByte(p, ']')
		book, p = p[1:end], p[end+1:]
	}
	if strings.HasPrefix(p, "'") {
		p = strings.ReplaceAll(p[1:len(p)-1], "''", "'")
		if strings.HasPrefix(p, "[") {
			if end := strings.IndexByte(p, ']'); end > 0 {
				book, p = p[1:end], p[end+1:]
			}
		}
	}
	sheet, last, _ = strings.Cut(p, ":")
	if sheet == "" || last == "" {
		return book, p, ""
	}
	return book, sheet, last
}

func readArea(s string) (Area, int, bool) {
	var a Area
	if r1, c1, abs1, n, ok := readCell(s); ok {
		a = Area{R1: r1, C1: c1, R2: r1, C2: c1, AbsR1: abs1[0], AbsC1: abs1[1], AbsR2: abs1[0], AbsC2: abs1[1], Cell: true}
		if n < len(s) && s[n] == ':' {
			if r2, c2, abs2, m, ok := readCell(s[n+1:]); ok {
				a = Area{R1: r1, C1: c1, R2: r2, C2: c2, AbsR1: abs1[0], AbsC1: abs1[1], AbsR2: abs2[0], AbsC2: abs2[1]}
				n += 1 + m
			}
		}
		return a.normal(), n, true
	}
	if c1, abs1, n, ok := readCol(s); ok && n < len(s) && s[n] == ':' {
		if c2, abs2, m, ok := readCol(s[n+1:]); ok {
			a = Area{R1: 1, C1: c1, R2: MaxRows, C2: c2, AbsC1: abs1, AbsC2: abs2, Cols: true}
			return a.normal(), n + 1 + m, true
		}
	}
	if r1, abs1, n, ok := readRow(s); ok && n < len(s) && s[n] == ':' {
		if r2, abs2, m, ok := readRow(s[n+1:]); ok {
			a = Area{R1: r1, C1: 1, R2: r2, C2: MaxCols, AbsR1: abs1, AbsR2: abs2, Rows: true}
			return a.normal(), n + 1 + m, true
		}
	}
	return a, 0, false
}

// normal orders the corners of the area, as Excel does.
func (a Area) normal() Area {
	if a.R1 > a.R2 {
		a.R1, a.R2, a.AbsR1, a.AbsR2 = a.R2, a.R1, a.AbsR2, a.AbsR1
	}
	if a.C1 > a.C2 {
		a.C1, a.C2, a.AbsC1, a.AbsC2 = a.C2, a.C1, a.AbsC2, a.AbsC1
	}
	return a
}

func readCell(s string) (row, col int, abs [2]bool, n int, ok bool) {
	col, abs[1], n, ok = readCol(s)
	if !ok {
		return
	}
	var m int
	row, abs[0], m, ok = readRow(s[n:])
	return row, col, abs, n + m, ok
}

func readCol(s string) (col int, abs bool, n int, ok bool) {
	if strings.HasPrefix(s, "$") {
		abs, n = true, 1
	}
	start := n
	for n < len(s) && n-start < 3 {
		c := s[n] | 0x20
		if c < 'a' || c > 'z' {
			break
		}
		col = col*26 + int(c-'a') + 1
		n++
	}
	if n == start || col > MaxCols || n < len(s) && (s[n]|0x20) >= 'a' && (s[n]|0x20) <= 'z' {
		return 0, false, 0, false
	}
	return col, abs, n, true
}

func readRow(s string) (row int, abs bool, n int, ok bool) {
	if strings.HasPrefix(s, "$") {
		abs, n = true, 1
	}
	start := n
	for n < len(s) && s[n] >= '0' && s[n] <= '9' && n-start < 8 {
		row = row*10 + int(s[n]-'0')
		n++
	}
	if n == start || row < 1 || row > MaxRows || n < len(s) && s[n] >= '0' && s[n] <= '9' {
		return 0, false, 0, false
	}
	return row, abs, n, true
}

// String writes the reference back.
func (r *Ref) String() string {
	var b strings.Builder
	b.WriteString(r.Prefix())
	if r.Invalid {
		b.WriteString("#REF!")
	} else {
		b.WriteString(r.Area.String())
	}
	return b.String()
}

// Prefix is the sheet prefix of the reference, "!" included, "" when it
// has none.
func (r *Ref) Prefix() string {
	if r.Sheet == "" && r.LastSheet == "" && r.Book == "" {
		return ""
	}
	name := r.Sheet
	if r.LastSheet != "" {
		name += ":" + r.LastSheet
	}
	book := ""
	if r.Book != "" {
		book = "[" + r.Book + "]"
	}
	if needsQuotes(r.Sheet) || r.LastSheet != "" && needsQuotes(r.LastSheet) {
		return "'" + strings.ReplaceAll(book+name, "'", "''") + "'!"
	}
	return book + name + "!"
}

// needsQuotes tells whether a sheet name must be quoted in a reference.
func needsQuotes(name string) bool {
	if name == "" {
		return false
	}
	if identLen(name) != len(name) || name[0] >= '0' && name[0] <= '9' || strings.HasPrefix(name, ".") {
		return true
	}
	if _, _, _, n, ok := readCell(name); ok && n == len(name) {
		return true
	}
	upper := strings.ToUpper(name)
	if upper == "TRUE" || upper == "FALSE" {
		return true
	}
	// R1C1 references: R, C, R1C1, RC2…
	if upper[0] == 'R' || upper[0] == 'C' {
		rest := strings.TrimLeft(upper, "RC0123456789")
		if rest == "" {
			return true
		}
	}
	return false
}

func (a Area) String() string {
	switch {
	case a.Cols:
		return colName(a.C1, a.AbsC1) + ":" + colName(a.C2, a.AbsC2)
	case a.Rows:
		return rowName(a.R1, a.AbsR1) + ":" + rowName(a.R2, a.AbsR2)
	case a.Cell && a.R1 == a.R2 && a.C1 == a.C2:
		return colName(a.C1, a.AbsC1) + rowName(a.R1, a.AbsR1)
	}
	return colName(a.C1, a.AbsC1) + rowName(a.R1, a.AbsR1) + ":" + colName(a.C2, a.AbsC2) + rowName(a.R2, a.AbsR2)
}

// ColumnName is the letters of a column: 1 is A, 27 is AA.
func ColumnName(col int) string {
	var b [4]byte
	i := len(b)
	for col > 0 {
		col--
		i--
		b[i] = byte('A' + col%26)
		col /= 26
	}
	return string(b[i:])
}

func colName(col int, abs bool) string {
	if abs {
		return "$" + ColumnName(col)
	}
	return ColumnName(col)
}

func rowName(row int, abs bool) string {
	if abs {
		return "$" + strconv.Itoa(row)
	}
	return strconv.Itoa(row)
}

// CellName is the A1 name of a cell.
func CellName(row, col int) string {
	return ColumnName(col) + strconv.Itoa(row)
}

// ParseCell reads an A1 cell name, "$" allowed.
func ParseCell(s string) (row, col int, ok bool) {
	row, col, _, n, ok := readCell(s)
	return row, col, ok && n == len(s)
}

// ParseArea reads an area: A1, A1:B2, A:C, 1:3.
func ParseArea(s string) (Area, bool) {
	a, n, ok := readArea(s)
	return a, ok && n == len(s)
}
