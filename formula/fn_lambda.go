package formula

import (
	"math"
	"strings"
)

// lambda is a function made by LAMBDA: its parameters, its formula, and
// the names bound where it was made, which it sees wherever it is called.
type lambda struct {
	params []string
	body   Expr
	vars   *variable
}

// variable is a name LET or a call of a LAMBDA binds, in a chain from the
// innermost.
type variable struct {
	name    string
	value   Value
	omitted bool
	next    *variable
}

func (v *variable) find(name string) *variable {
	for ; v != nil; v = v.next {
		if strings.EqualFold(v.name, name) {
			return v
		}
	}
	return nil
}

// maxCalls bounds the LAMBDA functions called within one another, as
// Excel bounds recursion.
const maxCalls = 1024

// invoke calls f, which should be a LAMBDA, with arguments; those left out
// are omitted.
func (c *Context) invoke(f Value, args []Expr) Value {
	if f.Type == TypeError {
		return f
	}
	if f.Type != TypeLambda || len(args) > len(f.fn.params) {
		return ErrValue
	}
	values := make([]Value, len(f.fn.params))
	omitted := make([]bool, len(f.fn.params))
	for i := range values {
		if i >= len(args) {
			omitted[i] = true
		} else if _, ok := args[i].(missingExpr); ok {
			omitted[i] = true
		} else {
			values[i] = c.Eval(args[i])
		}
	}
	return c.call(f.fn, values, omitted)
}

// call calculates a LAMBDA with the values of its parameters, those
// omitted marked when omitted is not nil.
func (c *Context) call(l *lambda, values []Value, omitted []bool) Value {
	if c.calls >= maxCalls {
		return ErrNum
	}
	vars := l.vars
	for i, p := range l.params {
		vars = &variable{name: p, value: values[i], omitted: omitted != nil && omitted[i], next: vars}
	}
	outer := c.vars
	c.vars = vars
	c.calls++
	defer func() { c.vars = outer; c.calls-- }()
	return c.Eval(l.body)
}

// named calls the LAMBDA of a defined name.
func (c *Context) named(e callExpr) Value {
	x, ok := c.Book.Name(e.name, c.Sheet)
	if !ok {
		return ErrUnsupported
	}
	return c.invoke(c.defined(x), e.args)
}

// lambdaOf is the LAMBDA an argument gives, which must take n parameters.
func (c *Context) lambdaOf(arg Expr, n int) (*lambda, *Value) {
	v := c.Eval(arg)
	if v.Type == TypeError {
		return nil, &v
	}
	if v.Type != TypeLambda || len(v.fn.params) != n {
		return nil, &ErrValue
	}
	return v.fn, nil
}

// element is what a LAMBDA gives for an element of an array: a single
// value, #CALC! for more, since arrays do not nest.
func (c *Context) element(v Value) Value {
	switch v.Type {
	case TypeArray:
		if len(v.Arr.Cells) == 1 {
			return v.Arr.Cells[0]
		}
		return ErrCalc
	case TypeRange:
		if singleCell(v) {
			return c.deref(v)
		}
		return ErrCalc
	case TypeLambda:
		return ErrCalc
	}
	return v
}

// variableName is the name an argument of LET or LAMBDA declares.
func variableName(x Expr) (string, bool) {
	n, ok := x.(nameExpr)
	return n.name, ok && n.sheet == ""
}

// scan calls a LAMBDA of two parameters with an accumulator, the first
// argument at first, and each value of the array of the second in turn:
// REDUCE gives the last accumulator, SCAN all of them.
func (c *Context) scan(args []Expr, all bool) Value {
	l, err := c.lambdaOf(args[2], 2)
	if err != nil {
		return *err
	}
	acc := c.Eval(args[0])
	t := c.array(c.Eval(args[1]))
	var out *Table
	if all {
		out = &Table{Rows: t.Rows, Cols: t.Cols, Cells: make([]Value, len(t.Cells))}
	}
	for i, x := range t.Cells {
		acc = c.call(l, []Value{acc, x}, nil)
		if all {
			out.Cells[i] = c.element(acc)
		}
	}
	if all {
		return Value{Type: TypeArray, Arr: out}
	}
	return acc
}

// operand is an array MAP, BYROW and BYCOL call a LAMBDA over: a range
// gives references to its cells, rows or columns, any other array its values.
type operand struct {
	t          *Table
	area       Area3
	rows, cols int
}

func (c *Context) operand(v Value) operand {
	if v.Type == TypeRange && len(v.Refs) == 1 {
		a := c.trim(v.Refs[0])
		if o := (operand{area: a, rows: a.R2 - a.R1 + 1, cols: a.C2 - a.C1 + 1}); o.rows*o.cols <= maxCells {
			return o
		}
	}
	t := c.array(v)
	return operand{t: t, rows: t.Rows, cols: t.Cols}
}

// sub is the reference to rows r1 to r2 and columns c1 to c2 of a range.
func (o operand) sub(r1, r2, c1, c2 int) Value {
	a := o.area
	return Value{Type: TypeRange, Refs: []Area3{{a.Sheet, Area{R1: a.R1 + r1, R2: a.R1 + r2, C1: a.C1 + c1, C2: a.C1 + c2}}}}
}

// at is the element at row i and column j, stretched like a table.
func (o operand) at(i, j int) Value {
	if o.t != nil {
		return stretched(o.t, i, j)
	}
	if o.rows == 1 {
		i = 0
	}
	if o.cols == 1 {
		j = 0
	}
	if i >= o.rows || j >= o.cols {
		return ErrNA
	}
	return o.sub(i, i, j, j)
}

// line is the row i, or the column.
func (o operand) line(i int, row bool) Value {
	switch {
	case o.t == nil && row:
		return o.sub(i, i, 0, o.cols-1)
	case o.t == nil:
		return o.sub(0, o.rows-1, i, i)
	case row:
		return Value{Type: TypeArray, Arr: &Table{Rows: 1, Cols: o.cols, Cells: o.t.Cells[i*o.cols : (i+1)*o.cols]}}
	}
	col := &Table{Rows: o.rows, Cols: 1, Cells: make([]Value, o.rows)}
	for k := range o.rows {
		col.Cells[k] = o.t.At(k, i)
	}
	return Value{Type: TypeArray, Arr: col}
}

// lines calls a LAMBDA of one parameter with each row of an array, or each
// column, for BYROW and BYCOL.
func (c *Context) lines(args []Expr, rows bool) Value {
	l, err := c.lambdaOf(args[1], 1)
	if err != nil {
		return *err
	}
	o := c.operand(c.Eval(args[0]))
	n := o.rows
	if !rows {
		n = o.cols
	}
	cells := make([]Value, n)
	for i := range cells {
		cells[i] = c.element(c.call(l, []Value{o.line(i, rows)}, nil))
	}
	if rows {
		return Value{Type: TypeArray, Arr: &Table{Rows: n, Cols: 1, Cells: cells}}
	}
	return Value{Type: TypeArray, Arr: &Table{Rows: 1, Cols: n, Cells: cells}}
}

func init() {
	add(map[string]*function{
		"LET": {min: 3, max: 253, call: func(c *Context, args []Expr) Value {
			if len(args)%2 == 0 {
				return ErrValue
			}
			outer := c.vars
			defer func() { c.vars = outer }()
			for i := 0; i < len(args)-1; i += 2 {
				name, ok := variableName(args[i])
				if !ok {
					return ErrValue
				}
				c.vars = &variable{name: name, value: c.Eval(args[i+1]), next: c.vars}
			}
			return c.Eval(args[len(args)-1])
		}},
		"LAMBDA": {min: 1, max: 254, call: func(c *Context, args []Expr) Value {
			l := &lambda{params: make([]string, len(args)-1), body: args[len(args)-1], vars: c.vars}
			for i, a := range args[:len(args)-1] {
				name, ok := variableName(a)
				if !ok {
					return ErrValue
				}
				for _, p := range l.params[:i] {
					if strings.EqualFold(p, name) {
						return ErrValue
					}
				}
				l.params[i] = name
			}
			return Value{Type: TypeLambda, fn: l}
		}},
		"ISOMITTED": {min: 1, max: 1, call: func(c *Context, args []Expr) Value {
			if name, ok := variableName(args[0]); ok {
				if v := c.vars.find(name); v != nil {
					return Boolean(v.omitted)
				}
			}
			return Boolean(false)
		}},
		"MAP": {min: 2, max: 254, call: func(c *Context, args []Expr) Value {
			n := len(args) - 1
			l, err := c.lambdaOf(args[n], n)
			if err != nil {
				return *err
			}
			operands := make([]operand, n)
			rows, cols := 0, 0
			for i, a := range args[:n] {
				operands[i] = c.operand(c.Eval(a))
				rows, cols = max(rows, operands[i].rows), max(cols, operands[i].cols)
			}
			if rows*cols > maxCells {
				return ErrNum
			}
			out := &Table{Rows: rows, Cols: cols, Cells: make([]Value, rows*cols)}
			values := make([]Value, n)
			for i := range rows {
				for j := range cols {
					for k, o := range operands {
						values[k] = o.at(i, j)
					}
					out.Cells[i*cols+j] = c.element(c.call(l, values, nil))
				}
			}
			return Value{Type: TypeArray, Arr: out}
		}},
		"REDUCE": {min: 3, max: 3, call: func(c *Context, args []Expr) Value {
			return c.scan(args, false)
		}},
		"SCAN": {min: 3, max: 3, call: func(c *Context, args []Expr) Value {
			return c.scan(args, true)
		}},
		"BYROW": {min: 2, max: 2, call: func(c *Context, args []Expr) Value {
			return c.lines(args, true)
		}},
		"BYCOL": {min: 2, max: 2, call: func(c *Context, args []Expr) Value {
			return c.lines(args, false)
		}},
		"MAKEARRAY": {min: 3, max: 3, call: func(c *Context, args []Expr) Value {
			rows, err := c.number(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			cols, err := c.number(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			l, err := c.lambdaOf(args[2], 2)
			if err != nil {
				return *err
			}
			rows, cols = math.Trunc(rows), math.Trunc(cols)
			if rows < 1 || cols < 1 {
				return ErrValue
			}
			if rows*cols > maxCells {
				return ErrNum
			}
			r, k := int(rows), int(cols)
			out := &Table{Rows: r, Cols: k, Cells: make([]Value, r*k)}
			for i := range r {
				for j := range k {
					out.Cells[i*k+j] = c.element(c.call(l, []Value{Num(float64(i + 1)), Num(float64(j + 1))}, nil))
				}
			}
			return Value{Type: TypeArray, Arr: out}
		}},
	})
}
