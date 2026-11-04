package formula

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

func init() {
	add(map[string]*function{
		"VLOOKUP": fn(3, 4, func(c *Context, v []Value) Value { return c.tableLookup(v, false) }),
		"HLOOKUP": fn(3, 4, func(c *Context, v []Value) Value { return c.tableLookup(v, true) }),
		"LOOKUP": fn(2, 3, func(c *Context, v []Value) Value {
			x := c.scalar(v[0])
			if x.Type == TypeError {
				return x
			}
			t := c.grid(v[1])
			var keys []Value
			var result func(i int) Value
			n := max(t.Rows, t.Cols)
			switch {
			case len(v) > 2:
				keys = vector(t)
				r := c.grid(v[2])
				result = func(i int) Value {
					if i >= max(r.Rows, r.Cols) {
						return ErrNA
					}
					if r.Rows == 1 {
						return r.At(0, i)
					}
					return r.At(i, 0)
				}
			case t.Cols > t.Rows:
				keys = row(t, 0)
				result = func(i int) Value { return t.At(t.Rows-1, i) }
			default:
				keys = column(t, 0)
				result = func(i int) Value { return t.At(i, t.Cols-1) }
			}
			i := approximate(keys, x, 1)
			if i < 0 {
				return ErrNA
			}
			return result(last(i, keys, n))
		}),
		"MATCH": fn(2, 3, func(c *Context, v []Value) Value {
			x := c.scalar(v[0])
			if x.Type == TypeError {
				return x
			}
			t := c.grid(v[1])
			if t.Rows > 1 && t.Cols > 1 {
				return ErrNA
			}
			kind := 1.0
			if len(v) > 2 && v[2].Type != TypeBlank {
				var err *Value
				if kind, err = c.number(v[2]); err != nil {
					return *err
				}
			}
			keys := vector(t)
			var i int
			switch {
			case kind == 0:
				i = exact(keys, x, true, false)
			case kind > 0:
				i = last(approximate(keys, x, 1), keys, t.Rows*t.Cols)
			default:
				i = last(approximate(keys, x, -1), keys, t.Rows*t.Cols)
			}
			if i < 0 {
				return ErrNA
			}
			return Num(float64(i + 1))
		}),
		"XMATCH": fn(2, 4, func(c *Context, v []Value) Value {
			t := c.grid(v[1])
			if t.Rows > 1 && t.Cols > 1 {
				return ErrValue
			}
			mode, search, err := c.modes(v, 2)
			if err != nil {
				return *err
			}
			return c.mapValues(v[0], func(x Value) Value {
				if x.Type == TypeError {
					return x
				}
				i := xfind(vector(t), x, mode, search, t.Rows*t.Cols)
				if i < 0 {
					return ErrNA
				}
				return Num(float64(i + 1))
			})
		}),
		"XLOOKUP": {min: 3, max: 6, call: func(c *Context, args []Expr) Value {
			v := make([]Value, len(args))
			for i, a := range args {
				if i != 3 {
					v[i] = c.Eval(a)
				}
			}
			keys := c.grid(v[1])
			if keys.Rows > 1 && keys.Cols > 1 {
				return ErrValue
			}
			mode, search, err := c.modes(v, 4)
			if err != nil {
				return *err
			}
			byRow := keys.Cols == 1
			find := func(x Value) Value {
				if x.Type == TypeError {
					return x
				}
				i := xfind(vector(keys), x, mode, search, keys.Rows*keys.Cols)
				if i < 0 {
					if len(args) > 3 {
						if _, missing := args[3].(missingExpr); !missing {
							return c.Eval(args[3])
						}
					}
					return ErrNA
				}
				if r := v[2]; r.Type == TypeRange && len(r.Refs) == 1 {
					a := r.Refs[0]
					if byRow {
						return Value{Type: TypeRange, Refs: []Area3{{a.Sheet, Area{R1: a.R1 + i, R2: a.R1 + i, C1: a.C1, C2: a.C2}}}}
					}
					return Value{Type: TypeRange, Refs: []Area3{{a.Sheet, Area{R1: a.R1, R2: a.R2, C1: a.C1 + i, C2: a.C1 + i}}}}
				}
				t := c.grid(v[2])
				if byRow {
					if i >= t.Rows {
						return ErrValue
					}
					return table(row(t, i), 1, t.Cols)
				}
				if i >= t.Cols {
					return ErrValue
				}
				return table(column(t, i), t.Rows, 1)
			}
			if lookup := c.array(v[0]); len(lookup.Cells) > 1 {
				return c.xlookupEach(lookup, c.array(v[2]), byRow, find)
			}
			return find(c.scalar(v[0]))
		}},
		"INDEX": fn(2, 4, func(c *Context, v []Value) Value {
			var n [3]float64
			for i := range n {
				if i+1 < len(v) && v[i+1].Type != TypeBlank {
					x, err := c.number(v[i+1])
					if err != nil {
						return *err
					}
					n[i] = math.Trunc(x)
				}
			}
			r, k, area := int(n[0]), int(n[1]), max(int(n[2]), 1)
			if r < 0 || k < 0 {
				return ErrValue
			}
			if v[0].Type == TypeRange {
				if area > len(v[0].Refs) {
					return ErrRef
				}
				a := v[0].Refs[area-1]
				rows, cols := a.R2-a.R1+1, a.C2-a.C1+1
				if len(v) == 2 && rows == 1 {
					r, k = 1, r
				}
				if r > rows || k > cols {
					return ErrRef
				}
				out := a
				if r > 0 {
					out.R1, out.R2 = a.R1+r-1, a.R1+r-1
				}
				if k > 0 {
					out.C1, out.C2 = a.C1+k-1, a.C1+k-1
				}
				out.Rows, out.Cols, out.Cell = false, false, false
				return Value{Type: TypeRange, Refs: []Area3{out}}
			}
			t := c.grid(v[0])
			if len(v) == 2 && t.Rows == 1 {
				r, k = 1, r
			}
			if r > t.Rows || k > t.Cols {
				return ErrRef
			}
			switch {
			case r > 0 && k > 0:
				return t.At(r-1, k-1)
			case r > 0:
				return table(row(t, r-1), 1, t.Cols)
			case k > 0:
				return table(column(t, k-1), t.Rows, 1)
			}
			return Value{Type: TypeArray, Arr: t}
		}),
		"ROW": {max: 1, call: func(c *Context, args []Expr) Value {
			if len(args) == 0 {
				return Num(float64(c.Row))
			}
			v := c.Eval(args[0])
			if v.Type != TypeRange {
				return ErrValue
			}
			return Num(float64(v.Refs[0].R1))
		}},
		"COLUMN": {max: 1, call: func(c *Context, args []Expr) Value {
			if len(args) == 0 {
				return Num(float64(c.Col))
			}
			v := c.Eval(args[0])
			if v.Type != TypeRange {
				return ErrValue
			}
			return Num(float64(v.Refs[0].C1))
		}},
		"ROWS": fn(1, 1, func(c *Context, v []Value) Value {
			if v[0].Type == TypeRange {
				return Num(float64(v[0].Refs[0].R2 - v[0].Refs[0].R1 + 1))
			}
			return Num(float64(c.grid(v[0]).Rows))
		}),
		"COLUMNS": fn(1, 1, func(c *Context, v []Value) Value {
			if v[0].Type == TypeRange {
				return Num(float64(v[0].Refs[0].C2 - v[0].Refs[0].C1 + 1))
			}
			return Num(float64(c.grid(v[0]).Cols))
		}),
		"OFFSET": {min: 3, max: 5, volatile: true, call: func(c *Context, args []Expr) Value {
			v := c.Eval(args[0])
			if v.Type != TypeRange || len(v.Refs) != 1 {
				return ErrValue
			}
			a := v.Refs[0]
			var n [4]float64
			n[2], n[3] = float64(a.R2-a.R1+1), float64(a.C2-a.C1+1)
			for i := 1; i < len(args); i++ {
				if _, missing := args[i].(missingExpr); missing {
					continue
				}
				x, err := c.number(c.Eval(args[i]))
				if err != nil {
					return *err
				}
				n[i-1] = math.Trunc(x)
			}
			if n[2] == 0 || n[3] == 0 {
				return ErrRef
			}
			r1, c1 := a.R1+int(n[0]), a.C1+int(n[1])
			r2, c2 := r1+int(n[2])-1, c1+int(n[3])-1
			if n[2] < 0 {
				r1, r2 = r1+int(n[2])+1, r1
			}
			if n[3] < 0 {
				c1, c2 = c1+int(n[3])+1, c1
			}
			if r1 < 1 || c1 < 1 || r2 > MaxRows || c2 > MaxCols {
				return ErrRef
			}
			return Value{Type: TypeRange, Refs: []Area3{{a.Sheet, Area{R1: r1, C1: c1, R2: r2, C2: c2}}}}
		}},
		"INDIRECT": {min: 1, max: 2, volatile: true, call: func(c *Context, args []Expr) Value {
			s, err := c.text(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			if len(args) > 1 {
				a1, err := c.boolean(c.Eval(args[1]))
				if err != nil {
					return *err
				}
				if !a1 {
					return c.indirectL1C1(strings.TrimSpace(s))
				}
			}
			tokens, e := Tokens(strings.TrimSpace(s))
			if e != nil || len(tokens) != 1 {
				return ErrRef
			}
			switch tokens[0].Kind {
			case Reference:
				return c.ref(tokens[0].Ref)
			case Name:
				x := c.name(nameExpr{name: tokens[0].Text})
				if x.Type != TypeRange {
					return ErrRef
				}
				return x
			}
			return ErrRef
		}},
		"ADDRESS": fn(2, 5, func(c *Context, v []Value) Value {
			r, err := c.number(v[0])
			if err != nil {
				return *err
			}
			k, err := c.number(v[1])
			if err != nil {
				return *err
			}
			abs := 1.0
			if len(v) > 2 && v[2].Type != TypeBlank {
				if abs, err = c.number(v[2]); err != nil {
					return *err
				}
			}
			a1 := true
			if len(v) > 3 && v[3].Type != TypeBlank {
				if a1, err = c.boolean(v[3]); err != nil {
					return *err
				}
			}
			row, col := int(r), int(k)
			if row < 1 || col < 1 || row > MaxRows || col > MaxCols || abs < 1 || abs > 4 {
				return ErrValue
			}
			absRow, absCol := abs == 1 || abs == 2, abs == 1 || abs == 3
			var s string
			if a1 {
				s = colName(col, absCol) + rowName(row, absRow)
			} else {
				rc := func(prefix string, n int, abs bool) string {
					if abs {
						return prefix + strconv.Itoa(n)
					}
					return prefix + "[" + strconv.Itoa(n) + "]"
				}
				s = rc("R", row, absRow) + rc("C", col, absCol)
			}
			if len(v) > 4 && v[4].Type != TypeBlank {
				sheet, err := c.text(v[4])
				if err != nil {
					return *err
				}
				s = (&Ref{Sheet: sheet}).Prefix() + s
			}
			return Str(s)
		}),
		"TRANSPOSE": fn(1, 1, func(c *Context, v []Value) Value {
			t := c.array(v[0])
			out := &Table{Rows: t.Cols, Cols: t.Rows, Cells: make([]Value, len(t.Cells))}
			for i := range t.Rows {
				for j := range t.Cols {
					out.Cells[j*t.Rows+i] = t.At(i, j)
				}
			}
			return Value{Type: TypeArray, Arr: out}
		}),
	})
}

func table(cells []Value, rows, cols int) Value {
	return Value{Type: TypeArray, Arr: &Table{Rows: rows, Cols: cols, Cells: cells}}
}

func row(t *Table, i int) []Value {
	i = min(i, t.held()-1)
	return t.Cells[i*t.Cols : (i+1)*t.Cols]
}

// column is the values of a column in the rows the table holds.
func column(t *Table, j int) []Value {
	out := make([]Value, t.held())
	for i := range out {
		out[i] = t.At(i, j)
	}
	return out
}

// vector is the values of a single row or column, or of a table row by
// row, in the rows the table holds.
func vector(t *Table) []Value {
	return t.Cells
}

// last is the index of a key found among the keys of a vector of n, held
// only up to the last that repeats: the last of the repeated ones when it
// is that key, for a search that keeps the last match.
func last(i int, keys []Value, n int) int {
	if i >= 0 && i == len(keys)-1 {
		return n - 1
	}
	return i
}

// tableLookup is VLOOKUP, or HLOOKUP when across. Of a range, it reads
// only the keys and the cell found.
func (c *Context) tableLookup(v []Value, across bool) Value {
	x := c.scalar(v[0])
	if x.Type == TypeError {
		return x
	}
	n, err := c.number(v[2])
	if err != nil {
		return *err
	}
	sorted := true
	if len(v) > 3 && v[3].Type != TypeBlank {
		if sorted, err = c.boolean(v[3]); err != nil {
			return *err
		}
	}
	k := int(n)
	if k < 1 {
		return ErrValue
	}
	var keys []Value
	var size, length int
	var at func(i, j int) Value
	if r := v[1]; r.Type == TypeRange && len(r.Refs) == 1 {
		a := r.Refs[0]
		first := a
		size = a.C2 - a.C1 + 1
		if across {
			first.R2, size = a.R1, a.R2-a.R1+1
		} else {
			first.C2 = a.C1
		}
		keys = vector(c.table(first))
		length = len(keys)
		at = func(i, j int) Value { return c.Book.Cell(a.Sheet, a.R1+i, a.C1+j) }
	} else {
		t := c.grid(r)
		keys, size, length = column(t, 0), t.Cols, t.Rows
		if across {
			keys, size, length = row(t, 0), t.Rows, t.Cols
		}
		at = t.At
	}
	if k > size {
		return ErrRef
	}
	var i int
	if sorted {
		i = last(approximate(keys, x, 1), keys, length)
	} else {
		i = exact(keys, x, true, false)
	}
	if i < 0 {
		return ErrNA
	}
	if across {
		return at(k-1, i)
	}
	return at(i, k-1)
}

// same tells whether a key equals the value looked up: text without
// regard to case, with wildcards when asked.
func same(key, x Value, wild bool) bool {
	if x.Type == TypeText && key.Type == TypeText && wild && strings.ContainsAny(x.Str, "*?~") {
		return wildcard(x.Str).MatchString(key.Str)
	}
	if key.Type != x.Type && !(key.Type == TypeBlank || x.Type == TypeBlank) {
		return false
	}
	return compare(key, x) == 0 && (key.Type != TypeBlank || x.Type == TypeBlank)
}

// exact is the index of the first key, or last when backwards, equal to x.
func exact(keys []Value, x Value, wild, backwards bool) int {
	for n := range keys {
		i := n
		if backwards {
			i = len(keys) - 1 - n
		}
		if same(keys[i], x, wild) {
			return i
		}
	}
	return -1
}

// comparable tells whether a key is of the kind of the value looked up,
// the only keys an approximate match considers.
func comparable(key, x Value) bool {
	switch x.Type {
	case TypeNumber, TypeBlank:
		return key.Type == TypeNumber
	}
	return key.Type == x.Type
}

// approximate is the index of the last key no greater than x in keys
// sorted in ascending order, or with order -1 of the last key no smaller
// in keys sorted in descending order. Like Excel, it searches by halves.
func approximate(keys []Value, x Value, order int) int {
	lo, hi, found := 0, len(keys)-1, -1
	for lo <= hi {
		mid := (lo + hi) / 2
		// the nearest key of the right kind, from mid on
		m := mid
		for m <= hi && !comparable(keys[m], x) {
			m++
		}
		if m > hi {
			hi = mid - 1
			continue
		}
		r := compare(keys[m], x) * order
		if r <= 0 {
			found, lo = m, m+1
		} else {
			hi = mid - 1
		}
	}
	return found
}

// modes reads the match and search modes of XMATCH and XLOOKUP, the
// arguments from i on.
func (c *Context) modes(v []Value, i int) (int, int, *Value) {
	mode, search := 0.0, 1.0
	var err *Value
	if len(v) > i && v[i].Type != TypeBlank {
		if mode, err = c.number(v[i]); err != nil {
			return 0, 0, err
		}
	}
	if len(v) > i+1 && v[i+1].Type != TypeBlank {
		if search, err = c.number(v[i+1]); err != nil {
			return 0, 0, err
		}
	}
	if mode < -1 || mode > 2 || search != 1 && search != -1 && search != 2 && search != -2 {
		return 0, 0, &ErrValue
	}
	return int(mode), int(search), nil
}

// xfind is the search of XMATCH and XLOOKUP: exact, or else the next
// smaller (-1) or larger (1) key, or with wildcards (2).
func xfind(keys []Value, x Value, mode, search, n int) int {
	if search != 1 {
		return last(xfindHeld(keys, x, mode, search), keys, n)
	}
	return xfindHeld(keys, x, mode, search)
}

// xfindHeld is xfind among the keys held.
func xfindHeld(keys []Value, x Value, mode, search int) int {
	switch {
	case search == 2 && mode != 2:
		return approximate(keys, x, 1)
	case search == -2 && mode != 2:
		return approximate(keys, x, -1)
	case mode == 0 || mode == 2:
		return exact(keys, x, mode == 2, search < 0)
	}
	best := -1
	for n := range keys {
		i := n
		if search < 0 {
			i = len(keys) - 1 - n
		}
		k := keys[i]
		if !comparable(k, x) {
			continue
		}
		r := compare(k, x)
		if r == 0 {
			return i
		}
		if r*mode > 0 && (best < 0 || compare(k, keys[best])*mode < 0) {
			best = i
		}
	}
	return best
}

// indirectL1C1 is INDIRECT of a reference written as R1C1.
func (c *Context) indirectL1C1(s string) Value {
	r := &Ref{}
	if i := strings.LastIndexByte(s, '!'); i >= 0 {
		r.Book, r.Sheet, r.LastSheet = splitPrefix(s[:i])
		s = s[i+1:]
	}
	a, ok := readL1C1(s, c.Row, c.Col)
	if !ok {
		return ErrRef
	}
	r.Area = a
	return c.ref(r)
}

// xlookupEach is XLOOKUP of several values: the result for each, a row of
// the return array or a column, one under another for a column of values,
// side by side for a row of them. A result that is a single value, an
// error or what is given for no match, fills its place.
func (c *Context) xlookupEach(lookup, ret *Table, byRow bool, find func(Value) Value) Value {
	h, w := ret.Rows, ret.Cols
	if byRow {
		h = 1
	} else {
		w = 1
	}
	column, line := lookup.Cols == 1, lookup.Rows == 1
	if column && h != 1 || line && w != 1 || !column && !line && h*w != 1 {
		return ErrValue
	}
	parts := make([][]Value, len(lookup.Cells))
	for i, x := range lookup.Cells {
		r := c.array(find(x))
		switch {
		case len(r.Cells) == 1:
			parts[i] = slices.Repeat(r.Cells, h*w)
		case r.Rows == h && r.Cols == w:
			parts[i] = r.Cells
		default:
			return ErrValue
		}
	}
	n := len(parts)
	switch {
	case column:
		return table(slices.Concat(parts...), n, w)
	case line:
		cells := make([]Value, h*n)
		for j, p := range parts {
			for i, x := range p {
				cells[i*n+j] = x
			}
		}
		return table(cells, h, n)
	}
	cells := make([]Value, n)
	for i, p := range parts {
		cells[i] = p[0]
	}
	return table(cells, lookup.Rows, lookup.Cols)
}
