package formula

import (
	"math"
	"slices"
	"strings"
)

func init() {
	add(map[string]*function{
		"TAKE":       fn(2, 3, func(c *Context, v []Value) Value { return c.cut(v, true) }),
		"DROP":       fn(2, 3, func(c *Context, v []Value) Value { return c.cut(v, false) }),
		"CHOOSECOLS": fn(2, -1, func(c *Context, v []Value) Value { return c.choose(v, true) }),
		"CHOOSEROWS": fn(2, -1, func(c *Context, v []Value) Value { return c.choose(v, false) }),
		"EXPAND": fn(2, 4, func(c *Context, v []Value) Value {
			t, err := c.gridArg(v[0])
			if err != nil {
				return *err
			}
			rows, cols := t.Rows, t.Cols
			if given(v, 1) {
				if rows, err = c.count(v[1]); err != nil {
					return *err
				}
			}
			if given(v, 2) {
				if cols, err = c.count(v[2]); err != nil {
					return *err
				}
			}
			if rows < t.Rows || cols < t.Cols {
				return ErrValue
			}
			held := t.held()
			if rows > t.Rows {
				held = t.Rows + 1
			}
			if held*cols > maxCells || rows*cols > maxCells && (rows > t.Rows || cols > 1<<14) {
				return ErrNum
			}
			pad := padding(c, v, 3)
			out := slices.Repeat([]Value{pad}, held*cols)
			for i := range min(held, t.Rows) {
				for j := range t.Cols {
					out[i*cols+j] = t.At(i, j)
				}
			}
			return Value{Type: TypeArray, Arr: &Table{Rows: rows, Cols: cols, Cells: out}}
		}),
		"VSTACK":   fn(1, -1, func(c *Context, v []Value) Value { return c.stack(v, false) }),
		"HSTACK":   fn(1, -1, func(c *Context, v []Value) Value { return c.stack(v, true) }),
		"TOCOL":    fn(1, 3, func(c *Context, v []Value) Value { return c.flatten(v, true) }),
		"TOROW":    fn(1, 3, func(c *Context, v []Value) Value { return c.flatten(v, false) }),
		"WRAPCOLS": fn(2, 3, func(c *Context, v []Value) Value { return c.wrap(v, true) }),
		"WRAPROWS": fn(2, 3, func(c *Context, v []Value) Value { return c.wrap(v, false) }),
		"TEXTSPLIT": fn(2, 6, func(c *Context, v []Value) Value {
			s, err := c.text(v[0])
			if err != nil {
				return *err
			}
			cols, err := c.delimiters(v, 1)
			if err != nil {
				return *err
			}
			rows, err := c.delimiters(v, 2)
			if err != nil {
				return *err
			}
			if cols == nil && rows == nil {
				return ErrValue
			}
			skip, fold := false, false
			if given(v, 3) {
				if skip, err = c.boolean(v[3]); err != nil {
					return *err
				}
			}
			if given(v, 4) {
				m, err := c.number(v[4])
				if err != nil {
					return *err
				}
				if m != 0 && m != 1 {
					return ErrValue
				}
				fold = m == 1
			}
			pad := padding(c, v, 5)
			var grid [][]string
			for _, r := range splitText(s, rows, fold, skip) {
				if cells := splitText(r, cols, fold, skip); len(cells) > 0 {
					grid = append(grid, cells)
				}
			}
			width := 0
			for _, l := range grid {
				width = max(width, len(l))
			}
			if width == 0 || len(grid)*width > maxCells {
				return ErrCalc
			}
			out := slices.Repeat([]Value{pad}, len(grid)*width)
			for i, l := range grid {
				for j, x := range l {
					out[i*width+j] = Str(x)
				}
			}
			return table(out, len(grid), width)
		}),
	})
}

// count reads a number of rows or columns, truncated.
func (c *Context) count(v Value) (int, *Value) {
	n, err := c.number(v)
	if err != nil {
		return 0, err
	}
	return int(math.Max(-maxCells, math.Min(maxCells, math.Trunc(n)))), nil
}

// padding is argument i, what fills the cells a result lacks: #N/A when
// it is left out.
func padding(c *Context, v []Value, i int) Value {
	if !given(v, i) {
		return ErrNA
	}
	return c.scalar(v[i])
}

// span is the part of n lines TAKE keeps, or DROP leaves, for the count
// k: from the start when positive, from the end when negative.
func span(n, k int, take bool) (int, int) {
	if take {
		if k < 0 {
			return max(n+k, 0), n
		}
		return 0, min(k, n)
	}
	if k < 0 {
		return 0, max(n+k, 0)
	}
	return min(k, n), n
}

// cut is TAKE, or DROP when take is false.
func (c *Context) cut(v []Value, take bool) Value {
	t, err := c.gridArg(v[0])
	if err != nil {
		return *err
	}
	var lo, hi [2]int
	for i, n := range []int{t.Rows, t.Cols} {
		lo[i], hi[i] = 0, n
		if !given(v, i+1) {
			continue
		}
		k, err := c.count(v[i+1])
		if err != nil {
			return *err
		}
		if k == 0 && take {
			return ErrCalc
		}
		if k != 0 {
			lo[i], hi[i] = span(n, k, take)
		}
	}
	rows, cols := hi[0]-lo[0], hi[1]-lo[1]
	if rows == 0 || cols == 0 {
		return ErrCalc
	}
	held := min(rows, max(t.held()-lo[0], 1))
	out := make([]Value, 0, held*cols)
	for i := range held {
		out = append(out, row(t, lo[0]+i)[lo[1]:hi[1]]...)
	}
	return table(out, rows, cols)
}

// choose is CHOOSECOLS, or CHOOSEROWS when byCol is false.
func (c *Context) choose(v []Value, byCol bool) Value {
	t, err := c.gridArg(v[0])
	if err != nil {
		return *err
	}
	n := t.Rows
	if byCol {
		n = t.Cols
	}
	var picked [][]Value
	for _, a := range v[1:] {
		for _, x := range c.array(a).Cells {
			k, err := c.count(x)
			if err != nil {
				return *err
			}
			if k < 0 {
				k += n + 1
			}
			if k < 1 || k > n {
				return ErrValue
			}
			if byCol {
				picked = append(picked, column(t, k-1))
			} else {
				picked = append(picked, row(t, k-1))
			}
		}
	}
	out := assemble(picked, byCol)
	if byCol {
		out.Arr.Rows = t.Rows
	}
	return out
}

// stack is HSTACK, or VSTACK when across is false: the arrays side by
// side or one under another, #N/A filling the shorter.
func (c *Context) stack(v []Value, across bool) Value {
	ts := make([]*Table, len(v))
	rows, cols := 0, 0
	for i, x := range v {
		if across {
			ts[i] = c.grid(x)
			rows, cols = max(rows, ts[i].Rows), cols+ts[i].Cols
			continue
		}
		ts[i] = c.array(x)
		rows, cols = rows+ts[i].Rows, max(cols, ts[i].Cols)
		if rows*cols > maxCells {
			return ErrNum
		}
	}
	held := rows
	if across {
		held = 0
		for _, t := range ts {
			if t.Rows < rows {
				held = max(held, t.Rows+1)
			} else {
				held = max(held, t.held())
			}
		}
	}
	if held*cols > maxCells {
		return ErrNum
	}
	out := slices.Repeat([]Value{ErrNA}, held*cols)
	at := 0
	for _, t := range ts {
		for i := range min(t.Rows, held) {
			if across {
				copy(out[i*cols+at:], row(t, i))
			} else {
				copy(out[(at+i)*cols:], row(t, i))
			}
		}
		if across {
			at += t.Cols
		} else {
			at += t.Rows
		}
	}
	return table(out, rows, cols)
}

// flatten is TOCOL, or TOROW when toCol is false.
func (c *Context) flatten(v []Value, toCol bool) Value {
	t, err := c.gridArg(v[0])
	if err != nil {
		return *err
	}
	skip := 0.0
	if given(v, 1) {
		if skip, err = c.number(v[1]); err != nil {
			return *err
		}
	}
	if skip < 0 || skip > 3 {
		return ErrValue
	}
	byCol := false
	if given(v, 2) {
		if byCol, err = c.boolean(v[2]); err != nil {
			return *err
		}
	}
	if int(skip)&1 == 0 || !t.blankTail() {
		t = c.array(v[0])
	}
	var cells []Value
	for _, l := range lines(t, byCol) {
		for _, x := range l {
			if x.Type == TypeBlank && int(skip)&1 != 0 || x.Type == TypeError && int(skip)&2 != 0 {
				continue
			}
			cells = append(cells, x)
		}
	}
	switch {
	case len(cells) == 0:
		return ErrCalc
	case toCol:
		return table(cells, len(cells), 1)
	}
	return table(cells, 1, len(cells))
}

// wrap is WRAPCOLS, or WRAPROWS when byCol is false: a row or a column of
// values cut in lines of as many values as argument 2 says, #N/A filling
// the last.
func (c *Context) wrap(v []Value, byCol bool) Value {
	t, err := c.arrayArg(v[0])
	if err != nil {
		return *err
	}
	if t.Rows > 1 && t.Cols > 1 {
		return ErrValue
	}
	n, err := c.count(v[1])
	if err != nil {
		return *err
	}
	if n < 1 {
		return ErrNum
	}
	pad := padding(c, v, 2)
	size := (len(t.Cells) + n - 1) / n
	if size*n > maxCells {
		return ErrNum
	}
	var ls [][]Value
	for i := range size {
		l := slices.Repeat([]Value{pad}, n)
		copy(l, t.Cells[i*n:min((i+1)*n, len(t.Cells))])
		ls = append(ls, l)
	}
	return assemble(ls, byCol)
}

// delimiters is argument i of TEXTSPLIT as a list of delimiters, nil when
// it is left out.
func (c *Context) delimiters(v []Value, i int) ([]string, *Value) {
	if !given(v, i) {
		return nil, nil
	}
	var out []string
	for _, x := range c.array(v[i]).Cells {
		s, err := c.text(x)
		if err != nil {
			return nil, err
		}
		if s == "" {
			return nil, &ErrValue
		}
		out = append(out, s)
	}
	return out, nil
}

// splitText cuts s at the delimiters, the longest where several begin
// together, and drops the empty pieces when skip is set.
func splitText(s string, delims []string, fold, skip bool) []string {
	if delims == nil {
		return []string{s}
	}
	hay := s
	if fold {
		hay = strings.ToLower(s)
		delims = slices.Clone(delims)
		for i, d := range delims {
			delims[i] = strings.ToLower(d)
		}
	}
	var out []string
	start := 0
	piece := func(end int) {
		if !skip || end > start {
			out = append(out, s[start:end])
		}
	}
	for i := 0; i < len(hay); {
		n := 0
		for _, d := range delims {
			if len(d) > n && strings.HasPrefix(hay[i:], d) {
				n = len(d)
			}
		}
		if n == 0 {
			i++
			continue
		}
		piece(i)
		i += n
		start = i
	}
	piece(len(s))
	return out
}
