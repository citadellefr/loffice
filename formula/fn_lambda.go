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
		if v.Arr.Rows*v.Arr.Cols == 1 {
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

// lines calls a LAMBDA of one parameter with each row of an array, or each
// column, for BYROW and BYCOL.
func (c *Context) lines(args []Expr, rows bool) Value {
	l, err := c.lambdaOf(args[1], 1)
	if err != nil {
		return *err
	}
	v := c.Eval(args[0])
	t := c.grid(v)
	n, size := t.held(), t.Cols
	if !rows {
		t = c.array(v)
		n, size = t.Cols, t.Rows
	}
	cells := make([]Value, n)
	for i := range cells {
		line := &Table{Rows: 1, Cols: size, Cells: t.Cells[i*size : (i+1)*size]}
		if !rows {
			line = &Table{Rows: size, Cols: 1, Cells: make([]Value, size)}
			for k := range size {
				line.Cells[k] = t.At(k, i)
			}
		}
		cells[i] = c.element(c.call(l, []Value{{Type: TypeArray, Arr: line}}, nil))
	}
	if rows {
		return Value{Type: TypeArray, Arr: &Table{Rows: t.Rows, Cols: 1, Cells: cells}}
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
			tables := make([]*Table, n)
			rows, cols := 0, 0
			for i, a := range args[:n] {
				tables[i] = c.grid(c.Eval(a))
				rows, cols = max(rows, tables[i].Rows), max(cols, tables[i].Cols)
			}
			held := 0
			for _, t := range tables {
				held = max(held, changing(t, rows))
			}
			held = min(held, rows)
			if held*cols > maxCells {
				return ErrNum
			}
			out := &Table{Rows: rows, Cols: cols, Cells: make([]Value, held*cols)}
			values := make([]Value, n)
			for i := range held {
				for j := range cols {
					for k, t := range tables {
						values[k] = stretched(t, i, j)
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
