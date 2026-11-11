package formula

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
)

func init() {
	randArray := fn(0, 5, func(c *Context, v []Value) Value {
		rows, cols, err := c.dimensions(v)
		if err != nil {
			return *err
		}
		lo, err := c.numberOr(v, 2, 0)
		if err != nil {
			return *err
		}
		hi, err := c.numberOr(v, 3, 1)
		if err != nil {
			return *err
		}
		whole := false
		if given(v, 4) {
			if whole, err = c.boolean(v[4]); err != nil {
				return *err
			}
		}
		if lo > hi || whole && (lo != math.Trunc(lo) || hi != math.Trunc(hi)) {
			return ErrValue
		}
		cells := make([]Value, rows*cols)
		for i := range cells {
			if whole {
				cells[i] = Num(min(lo+math.Floor(c.Rand()*(hi-lo+1)), hi))
			} else {
				cells[i] = Num(lo + c.Rand()*(hi-lo))
			}
		}
		return table(cells, rows, cols)
	})
	randArray.volatile = true

	add(map[string]*function{
		// how files write A1# and @A1
		"ANCHORARRAY": fn(1, 1, func(c *Context, v []Value) Value { return c.spill(v[0]) }),
		"SINGLE":      fn(1, 1, func(c *Context, v []Value) Value { return c.scalar(v[0]) }),
		"SEQUENCE": fn(1, 4, func(c *Context, v []Value) Value {
			rows, cols, err := c.dimensions(v)
			if err != nil {
				return *err
			}
			start, err := c.numberOr(v, 2, 1)
			if err != nil {
				return *err
			}
			step, err := c.numberOr(v, 3, 1)
			if err != nil {
				return *err
			}
			cells := make([]Value, rows*cols)
			for i := range cells {
				cells[i] = Num(start + step*float64(i))
			}
			return table(cells, rows, cols)
		}),
		"RANDARRAY": randArray,
		"UNIQUE": fn(1, 3, func(c *Context, v []Value) Value {
			t, err := c.arrayArg(v[0])
			if err != nil {
				return *err
			}
			byCol, onceOnly := false, false
			if given(v, 1) {
				if byCol, err = c.boolean(v[1]); err != nil {
					return *err
				}
			}
			if given(v, 2) {
				if onceOnly, err = c.boolean(v[2]); err != nil {
					return *err
				}
			}
			all := lines(t, byCol)
			keys := make([]string, len(all))
			count := map[string]int{}
			for i, l := range all {
				keys[i] = lineKey(l)
				count[keys[i]]++
			}
			var out [][]Value
			seen := map[string]bool{}
			for i, l := range all {
				if seen[keys[i]] || onceOnly && count[keys[i]] > 1 {
					continue
				}
				seen[keys[i]] = true
				out = append(out, l)
			}
			if len(out) == 0 {
				return ErrCalc
			}
			return assemble(out, byCol)
		}),
		"SORT": fn(1, 4, func(c *Context, v []Value) Value {
			t, err := c.arrayArg(v[0])
			if err != nil {
				return *err
			}
			byCol := false
			if given(v, 3) {
				if byCol, err = c.boolean(v[3]); err != nil {
					return *err
				}
			}
			indexes, err := c.numbersArg(v, 1)
			if err != nil {
				return *err
			}
			orders, err := c.sortOrders(v, 2)
			if err != nil {
				return *err
			}
			if len(orders) == 1 {
				orders = slices.Repeat(orders, len(indexes))
			}
			all := lines(t, byCol)
			if len(orders) != len(indexes) {
				return ErrValue
			}
			keys := make([][]Value, len(indexes))
			for k, n := range indexes {
				if n < 1 || n > float64(len(all[0])) {
					return ErrValue
				}
				keys[k] = make([]Value, len(all))
				for i, l := range all {
					keys[k][i] = l[int(n)-1]
				}
			}
			return assemble(sortLines(all, keys, orders), byCol)
		}),
		"SORTBY": fn(2, -1, func(c *Context, v []Value) Value {
			t, err := c.arrayArg(v[0])
			if err != nil {
				return *err
			}
			first := c.array(v[1])
			var byCol bool
			switch {
			case first.Cols == 1 && first.Rows == t.Rows:
			case first.Rows == 1 && first.Cols == t.Cols:
				byCol = true
			default:
				return ErrValue
			}
			all := lines(t, byCol)
			var keys [][]Value
			var orders []int
			for i := 1; i < len(v); i += 2 {
				by := c.array(v[i])
				if len(by.Cells) != len(all) || byCol && by.Rows != 1 || !byCol && by.Cols != 1 {
					return ErrValue
				}
				keys = append(keys, by.Cells)
				o := []int{1}
				if i+1 < len(v) {
					if o, err = c.sortOrders(v, i+1); err != nil {
						return *err
					}
				}
				if len(o) != 1 {
					return ErrValue
				}
				orders = append(orders, o[0])
			}
			return assemble(sortLines(all, keys, orders), byCol)
		}),
		"FILTER": fn(2, 3, func(c *Context, v []Value) Value {
			t, err := c.arrayArg(v[0])
			if err != nil {
				return *err
			}
			include, err := c.arrayArg(v[1])
			if err != nil {
				return *err
			}
			var byCol bool
			switch {
			case include.Cols == 1 && include.Rows == t.Rows:
			case include.Rows == 1 && include.Cols == t.Cols:
				byCol = true
			default:
				return ErrValue
			}
			var out [][]Value
			for i, l := range lines(t, byCol) {
				x := include.Cells[i]
				switch x.Type {
				case TypeNumber, TypeBool, TypeBlank:
					if x.Num == 0 {
						continue
					}
				case TypeError:
					return x
				default:
					return ErrValue
				}
				out = append(out, l)
			}
			if len(out) > 0 {
				return assemble(out, byCol)
			}
			if !given(v, 2) {
				return ErrCalc
			}
			if v[2].Type == TypeError {
				return v[2]
			}
			return Value{Type: TypeArray, Arr: c.array(v[2])}
		}),
	})
}

// given tells whether argument i was passed and not left out.
func given(v []Value, i int) bool {
	return i < len(v) && v[i].Type != TypeBlank
}

// numberOr is argument i as a number, def when it is left out.
func (c *Context) numberOr(v []Value, i int, def float64) (float64, *Value) {
	if !given(v, i) {
		return def, nil
	}
	return c.number(v[i])
}

// arrayArg is a value as a table, or the error it is.
func (c *Context) arrayArg(v Value) (*Table, *Value) {
	if v.Type == TypeError {
		return nil, &v
	}
	return c.array(v), nil
}

// dimensions reads the rows and columns, both 1 by default, of SEQUENCE
// and RANDARRAY from their first two arguments.
func (c *Context) dimensions(v []Value) (int, int, *Value) {
	rows, err := c.numberOr(v, 0, 1)
	if err != nil {
		return 0, 0, err
	}
	cols, err := c.numberOr(v, 1, 1)
	if err != nil {
		return 0, 0, err
	}
	rows, cols = math.Trunc(rows), math.Trunc(cols)
	switch {
	case rows < 0 || cols < 0:
		return 0, 0, &ErrValue
	case rows == 0 || cols == 0:
		return 0, 0, &ErrCalc
	case rows*cols > maxCells:
		return 0, 0, &ErrNum
	}
	return int(rows), int(cols), nil
}

// numbersArg is argument i as a list of numbers, an array's or a single
// one, 1 when it is left out.
func (c *Context) numbersArg(v []Value, i int) ([]float64, *Value) {
	if !given(v, i) {
		return []float64{1}, nil
	}
	t := c.array(v[i])
	out := make([]float64, len(t.Cells))
	for k, x := range t.Cells {
		n, err := c.number(x)
		if err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, nil
}

// sortOrders is argument i as a list of orders, 1 or -1, 1 when it is
// left out.
func (c *Context) sortOrders(v []Value, i int) ([]int, *Value) {
	ns, err := c.numbersArg(v, i)
	if err != nil {
		return nil, err
	}
	out := make([]int, len(ns))
	for k, n := range ns {
		if n != 1 && n != -1 {
			return nil, &ErrValue
		}
		out[k] = int(n)
	}
	return out, nil
}

// lines is the rows of a table, or its columns.
func lines(t *Table, byCol bool) [][]Value {
	if byCol {
		out := make([][]Value, t.Cols)
		for j := range out {
			out[j] = column(t, j)
		}
		return out
	}
	out := make([][]Value, t.Rows)
	for i := range out {
		out[i] = row(t, i)
	}
	return out
}

// assemble is the table of lines, as its rows or as its columns.
func assemble(ls [][]Value, byCol bool) Value {
	n, size := len(ls), len(ls[0])
	if !byCol {
		cells := make([]Value, 0, n*size)
		for _, l := range ls {
			cells = append(cells, l...)
		}
		return table(cells, n, size)
	}
	cells := make([]Value, n*size)
	for j, l := range ls {
		for i, x := range l {
			cells[i*n+j] = x
		}
	}
	return table(cells, size, n)
}

// lineKey is equal for the lines UNIQUE takes as the same: text without
// regard to case, a number and its text apart.
func lineKey(l []Value) string {
	var b strings.Builder
	for _, v := range l {
		s := v.Str
		switch v.Type {
		case TypeNumber, TypeBool:
			s = strconv.FormatFloat(v.Num+0, 'g', -1, 64) // +0 turns -0 into 0
		case TypeText:
			s = foldCase(s)
		}
		fmt.Fprintf(&b, "%d%d:%s", v.Type, len(s), s)
	}
	return b.String()
}

// sortLines orders lines stably by their keys in turn: keys[k][i] is the
// kth key of line i, orders[k] 1 to ascend or -1 to descend.
func sortLines(ls [][]Value, keys [][]Value, orders []int) [][]Value {
	perm := make([]int, len(ls))
	for i := range perm {
		perm[i] = i
	}
	slices.SortStableFunc(perm, func(a, b int) int {
		for k, key := range keys {
			if r := sortCompare(key[a], key[b]) * orders[k]; r != 0 {
				return r
			}
		}
		return 0
	})
	out := make([][]Value, len(ls))
	for i, p := range perm {
		out[i] = ls[p]
	}
	return out
}

// sortCompare orders values as SORT does: numbers, text without regard to
// case, booleans, errors, blanks.
func sortCompare(a, b Value) int {
	rank := func(v Value) int {
		switch v.Type {
		case TypeNumber:
			return 0
		case TypeText:
			return 1
		case TypeBool:
			return 2
		case TypeError:
			return 3
		}
		return 4
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}
	switch a.Type {
	case TypeNumber, TypeBool:
		return cmp.Compare(a.Num, b.Num)
	case TypeText:
		return strings.Compare(foldCase(a.Str), foldCase(b.Str))
	}
	return 0
}
