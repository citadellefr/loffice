package formula

import (
	"strconv"
	"strings"
)

// readL1C1 reads a reference written as R1C1 or, as the French Excel does,
// L1C1: "L2C3", "L[-1]C[2]", "LC", "L2C3:L4C5", "L2", "C3". Offsets are
// counted from the cell at row, col.
func readL1C1(s string, row, col int) (Area, bool) {
	first, second, ranged := strings.Cut(s, ":")
	a, ok := readL1C1Part(first, row, col)
	if !ok {
		return Area{}, false
	}
	if !ranged {
		return a, true
	}
	b, ok := readL1C1Part(second, row, col)
	if !ok {
		return Area{}, false
	}
	switch {
	case a.Rows != b.Rows || a.Cols != b.Cols:
		return Area{}, false
	case a.Cols:
		a.C2 = b.C1
	case a.Rows:
		a.R2 = b.R1
	default:
		a.R2, a.C2 = b.R1, b.C1
	}
	a.Cell = false
	return a.normal(), true
}

func readL1C1Part(s string, row, col int) (Area, bool) {
	s = strings.ToUpper(s)
	if len(s) == 0 || s[0] != 'R' && s[0] != 'L' && s[0] != 'C' {
		return Area{}, false
	}
	var r, c int
	var hasRow, hasCol bool
	if s[0] != 'C' {
		var ok bool
		if r, s, ok = l1c1Number(s[1:], row, MaxRows); !ok {
			return Area{}, false
		}
		hasRow = true
	}
	if s != "" {
		if s[0] != 'C' {
			return Area{}, false
		}
		var ok bool
		if c, s, ok = l1c1Number(s[1:], col, MaxCols); !ok || s != "" {
			return Area{}, false
		}
		hasCol = true
	}
	a := Area{R1: r, C1: c, R2: r, C2: c, Cell: true}
	switch {
	case !hasCol:
		a.C1, a.C2, a.Rows, a.Cell = 1, MaxCols, true, false
	case !hasRow:
		a.R1, a.R2, a.Cols, a.Cell = 1, MaxRows, true, false
	}
	return a, true
}

// l1c1Number reads what follows the R or C: nothing for the cell's own
// place, a number, or an offset in brackets.
func l1c1Number(s string, from, limit int) (int, string, bool) {
	var n int
	switch {
	case s != "" && s[0] == '[':
		end := strings.IndexByte(s, ']')
		if end < 0 {
			return 0, "", false
		}
		d, err := strconv.Atoi(s[1:end])
		if err != nil {
			return 0, "", false
		}
		n, s = from+d, s[end+1:]
	default:
		end := 0
		for end < len(s) && s[end] >= '0' && s[end] <= '9' {
			end++
		}
		if end == 0 {
			n = from
		} else {
			d, err := strconv.Atoi(s[:end])
			if err != nil {
				return 0, "", false
			}
			n = d
		}
		s = s[end:]
	}
	return n, s, n >= 1 && n <= limit
}
