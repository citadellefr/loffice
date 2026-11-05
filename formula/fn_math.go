package formula

import (
	"math"
	"slices"
	"strconv"
)

func init() {
	add(map[string]*function{
		"SUM": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			sum := 0.0
			if err := c.numbers(args, func(n float64) { sum += n }); err != nil {
				return *err
			}
			return Num(sum)
		}},
		"PRODUCT": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			p, any := 1.0, false
			if err := c.numbers(args, func(n float64) { p, any = p*n, true }); err != nil {
				return *err
			}
			if !any {
				return Num(0)
			}
			return Num(p)
		}},
		"SUMSQ": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			sum := 0.0
			if err := c.numbers(args, func(n float64) { sum += n * n }); err != nil {
				return *err
			}
			return Num(sum)
		}},
		"ABS":  math1(func(n float64) Value { return Num(math.Abs(n)) }),
		"SIGN": math1(func(n float64) Value { return Num(float64(compareFloat(n, 0))) }),
		"INT":  math1(func(n float64) Value { return Num(math.Floor(n)) }),
		"TRUNC": math2(1, 0, func(n, d float64) Value {
			return Num(roundWith(n, int(d), math.Trunc))
		}),
		"ROUND": math2(2, 0, func(n, d float64) Value {
			return Num(roundWith(n, int(d), math.Round))
		}),
		"ROUNDUP": math2(2, 0, func(n, d float64) Value {
			return Num(roundWith(n, int(d), func(x float64) float64 {
				if x < 0 {
					return -math.Ceil(-x)
				}
				return math.Ceil(x)
			}))
		}),
		"ROUNDDOWN": math2(2, 0, func(n, d float64) Value {
			return Num(roundWith(n, int(d), math.Trunc))
		}),
		"MROUND": math2(2, 0, func(n, m float64) Value {
			if m == 0 {
				return Num(0)
			}
			if n*m < 0 {
				return ErrNum
			}
			return Num(roundWith(n/m, 0, math.Round) * m)
		}),
		"CEILING": math2(2, 0, func(n, s float64) Value {
			if s == 0 {
				return Num(0)
			}
			if n > 0 && s < 0 {
				return ErrNum
			}
			return Num(multiple(n, s, math.Ceil))
		}),
		"FLOOR": math2(2, 0, func(n, s float64) Value {
			if s == 0 {
				if n == 0 {
					return Num(0)
				}
				return ErrDiv0
			}
			if n > 0 && s < 0 {
				return ErrNum
			}
			return Num(multiple(n, s, math.Floor))
		}),
		"CEILING.MATH":    roundTo(math.Ceil, true),
		"CEILING.PRECISE": roundTo(math.Ceil, false),
		"ISO.CEILING":     roundTo(math.Ceil, false),
		"FLOOR.MATH":      roundTo(math.Floor, true),
		"FLOOR.PRECISE":   roundTo(math.Floor, false),
		"EVEN": math1(func(n float64) Value {
			m := math.Ceil(math.Abs(n)/2) * 2
			return Num(math.Copysign(m, n))
		}),
		"ODD": math1(func(n float64) Value {
			m := math.Ceil((math.Abs(n)+1)/2)*2 - 1
			return Num(math.Copysign(m, n))
		}),
		"MOD": math2(2, 0, func(n, d float64) Value {
			if d == 0 {
				return ErrDiv0
			}
			return Num(n - d*math.Floor(n/d))
		}),
		"QUOTIENT": math2(2, 0, func(n, d float64) Value {
			if d == 0 {
				return ErrDiv0
			}
			return Num(math.Trunc(n / d))
		}),
		"POWER": math2(2, 0, func(n, p float64) Value { return power(n, p) }),
		"SQRT": math1(func(n float64) Value {
			if n < 0 {
				return ErrNum
			}
			return Num(math.Sqrt(n))
		}),
		"SQRTPI": math1(func(n float64) Value {
			if n < 0 {
				return ErrNum
			}
			return Num(math.Sqrt(n * math.Pi))
		}),
		"EXP": math1(func(n float64) Value { return Num(math.Exp(n)) }),
		"LN": math1(func(n float64) Value {
			if n <= 0 {
				return ErrNum
			}
			return Num(math.Log(n))
		}),
		"LOG10": math1(func(n float64) Value {
			if n <= 0 {
				return ErrNum
			}
			return Num(math.Log10(n))
		}),
		"LOG": math2(1, 10, func(n, b float64) Value {
			if n <= 0 || b <= 0 {
				return ErrNum
			}
			if b == 1 {
				return ErrDiv0
			}
			return Num(math.Log(n) / math.Log(b))
		}),
		"PI": fn(0, 0, func(*Context, []Value) Value { return Num(math.Pi) }),
		"RAND": {max: 0, volatile: true, call: func(c *Context, _ []Expr) Value {
			return Num(c.Rand())
		}},
		"RANDBETWEEN": {min: 2, max: 2, volatile: true, call: func(c *Context, args []Expr) Value {
			lo, err := c.number(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			hi, err := c.number(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			lo, hi = math.Ceil(lo), math.Floor(hi)
			if lo > hi {
				return ErrNum
			}
			return Num(lo + math.Floor(c.Rand()*(hi-lo+1)))
		}},
		"FACT": math1(func(n float64) Value {
			if n < 0 {
				return ErrNum
			}
			f := 1.0
			for i := 2.0; i <= math.Trunc(n); i++ {
				f *= i
			}
			return Num(f)
		}),
		"FACTDOUBLE": math1(func(n float64) Value {
			if n < -1 {
				return ErrNum
			}
			f := 1.0
			for i := math.Trunc(n); i > 1; i -= 2 {
				f *= i
			}
			return Num(f)
		}),
		"COMBIN": math2(2, 0, func(n, k float64) Value {
			n, k = math.Trunc(n), math.Trunc(k)
			if n < 0 || k < 0 || k > n {
				return ErrNum
			}
			return Num(combin(n, k))
		}),
		"COMBINA": math2(2, 0, func(n, k float64) Value {
			n, k = math.Trunc(n), math.Trunc(k)
			if n < 0 || k < 0 || n == 0 && k > 0 {
				return ErrNum
			}
			return Num(combin(n+k-1, k))
		}),
		"PERMUT": math2(2, 0, func(n, k float64) Value {
			n, k = math.Trunc(n), math.Trunc(k)
			if n < 0 || k < 0 || k > n {
				return ErrNum
			}
			p := 1.0
			for i := n - k + 1; i <= n; i++ {
				p *= i
			}
			return Num(p)
		}),
		"GCD": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			g := 0.0
			bad := false
			if err := c.numbers(args, func(n float64) {
				if n < 0 {
					bad = true
				}
				g = gcd(g, math.Trunc(n))
			}); err != nil {
				return *err
			}
			if bad {
				return ErrNum
			}
			return Num(g)
		}},
		"LCM": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			l := 1.0
			bad := false
			if err := c.numbers(args, func(n float64) {
				n = math.Trunc(n)
				if n < 0 {
					bad = true
				}
				if n == 0 || l == 0 {
					l = 0
					return
				}
				l = l / gcd(l, n) * n
			}); err != nil {
				return *err
			}
			if bad {
				return ErrNum
			}
			return Num(l)
		}},
		"DEGREES": math1(func(n float64) Value { return Num(n * 180 / math.Pi) }),
		"RADIANS": math1(func(n float64) Value { return Num(n * math.Pi / 180) }),
		"SIN":     math1(func(n float64) Value { return Num(math.Sin(n)) }),
		"COS":     math1(func(n float64) Value { return Num(math.Cos(n)) }),
		"TAN":     math1(func(n float64) Value { return Num(math.Tan(n)) }),
		"ASIN": math1(func(n float64) Value {
			if n < -1 || n > 1 {
				return ErrNum
			}
			return Num(math.Asin(n))
		}),
		"ACOS": math1(func(n float64) Value {
			if n < -1 || n > 1 {
				return ErrNum
			}
			return Num(math.Acos(n))
		}),
		"ATAN": math1(func(n float64) Value { return Num(math.Atan(n)) }),
		"ATAN2": math2(2, 0, func(x, y float64) Value {
			if x == 0 && y == 0 {
				return ErrDiv0
			}
			return Num(math.Atan2(y, x))
		}),
		"SINH":  math1(func(n float64) Value { return Num(math.Sinh(n)) }),
		"COSH":  math1(func(n float64) Value { return Num(math.Cosh(n)) }),
		"TANH":  math1(func(n float64) Value { return Num(math.Tanh(n)) }),
		"ASINH": math1(func(n float64) Value { return Num(math.Asinh(n)) }),
		"ACOSH": math1(func(n float64) Value {
			if n < 1 {
				return ErrNum
			}
			return Num(math.Acosh(n))
		}),
		"ATANH": math1(func(n float64) Value {
			if n <= -1 || n >= 1 {
				return ErrNum
			}
			return Num(math.Atanh(n))
		}),
		"SUMPRODUCT": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			var tables []*Table
			for _, a := range args {
				v := c.Eval(a)
				if v.Type == TypeError {
					return v
				}
				t := c.grid(v)
				if len(tables) > 0 && (t.Rows != tables[0].Rows || t.Cols != tables[0].Cols) {
					return ErrValue
				}
				tables = append(tables, t)
			}
			held := 0
			for _, t := range tables {
				held = max(held, t.held())
			}
			sum := 0.0
			for i := range held {
				for j := range tables[0].Cols {
					p := 1.0
					for _, t := range tables {
						v := t.At(i, j)
						if v.Type == TypeError {
							return v
						}
						if v.Type != TypeNumber {
							p = 0
						}
						p *= v.Num
					}
					if i == held-1 {
						p *= float64(tables[0].Rows - i)
					}
					sum += p
				}
			}
			return Num(sum)
		}},
		"SUMIF": {min: 2, max: 3, call: func(c *Context, args []Expr) Value {
			sum := 0.0
			_, err := c.ifOne(args, func(v Value) {
				if v.Type == TypeNumber {
					sum += v.Num
				}
			})
			if err != nil {
				return *err
			}
			return Num(sum)
		}},
		"SUMIFS": {min: 3, max: 255, call: func(c *Context, args []Expr) Value {
			sum := 0.0
			if _, err := c.ifs(args, true, func(v Value) {
				if v.Type == TypeNumber {
					sum += v.Num
				}
			}); err != nil {
				return *err
			}
			return Num(sum)
		}},
		"SUBTOTAL": {min: 2, max: 255, call: func(c *Context, args []Expr) Value {
			n, err := c.number(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			name, ok := subtotals[int(n)%100]
			if !ok || n < 1 || n > 111 {
				return ErrValue
			}
			skip := skipNested | skipFiltered
			if n > 100 {
				skip = skipNested | skipHidden
			}
			refs, err := c.visible(args[1:], skip, true)
			if err != nil {
				return *err
			}
			return functions[name].call(c, refs)
		}},
		"AGGREGATE": {min: 3, max: 255, call: func(c *Context, args []Expr) Value {
			n, err := c.number(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			o, err := c.number(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			if n < 1 || n >= float64(len(aggregates)+1) || o < 0 || o >= float64(len(aggregateSkips)) {
				return ErrValue
			}
			name, skip := aggregates[int(n)-1], aggregateSkips[int(o)]
			values := args[2:]
			if n >= 14 {
				if len(args) != 4 {
					return ErrValue
				}
				values = args[2:3]
			}
			refs, err := c.visible(values, skip, false)
			if err != nil {
				return *err
			}
			if n >= 14 {
				refs = append(refs, args[3])
			}
			return functions[name].call(c, refs)
		}},
	})
}

var subtotals = map[int]string{1: "AVERAGE", 2: "COUNT", 3: "COUNTA", 4: "MAX", 5: "MIN", 6: "PRODUCT",
	7: "STDEV", 8: "STDEVP", 9: "SUM", 10: "VAR", 11: "VARP"}

var aggregates = []string{"AVERAGE", "COUNT", "COUNTA", "MAX", "MIN", "PRODUCT", "STDEV.S", "STDEV.P", "SUM",
	"VAR.S", "VAR.P", "MEDIAN", "MODE.SNGL", "LARGE", "SMALL", "PERCENTILE.INC", "QUARTILE.INC",
	"PERCENTILE.EXC", "QUARTILE.EXC"}

// What SUBTOTAL and AGGREGATE leave out.
const (
	skipNested = 1 << iota
	skipHidden
	skipFiltered
	skipErrors
)

// aggregateSkips are the options of AGGREGATE.
var aggregateSkips = []int{skipNested, skipNested | skipHidden, skipNested | skipErrors,
	skipNested | skipHidden | skipErrors, 0, skipHidden, skipErrors, skipHidden | skipErrors}

// visible are the arguments of SUBTOTAL and AGGREGATE as the function
// they call reads them: arrays whose cells left out are blank. Only
// references are taken when refs.
func (c *Context) visible(args []Expr, skip int, refs bool) ([]Expr, *Value) {
	out := make([]Expr, 0, len(args))
	for _, a := range args {
		v := c.Eval(a)
		switch {
		case v.Type == TypeRange:
			for _, r := range v.Refs {
				out = append(out, valueExpr{c.visibleCells(r, skip)})
			}
		case refs && v.Type == TypeError:
			return nil, &v
		case refs:
			return nil, &ErrValue
		case skip&skipErrors != 0 && v.Type == TypeArray:
			t := &Table{Rows: v.Arr.Rows, Cols: v.Arr.Cols, Cells: slices.Clone(v.Arr.Cells)}
			for i, x := range t.Cells {
				if x.Type == TypeError {
					t.Cells[i] = Value{}
				}
			}
			out = append(out, valueExpr{Value{Type: TypeArray, Arr: t}})
		case skip&skipErrors != 0 && v.Type == TypeError:
			out = append(out, valueExpr{})
		default:
			out = append(out, valueExpr{v})
		}
	}
	return out, nil
}

// visibleCells are the values of an area, blank in the cells left out.
func (c *Context) visibleCells(a Area3, skip int) Value {
	t := c.table(a)
	rows, _ := c.Book.(Rows)
	nested, _ := c.Book.(interface {
		Subtotal(sheet, row, col int) bool
	})
	for i := range t.held() {
		row := a.R1 + i
		out := false
		if rows != nil && skip&(skipHidden|skipFiltered) != 0 {
			hidden, filtered := rows.Hidden(a.Sheet, row)
			out = skip&skipHidden != 0 && hidden || skip&skipFiltered != 0 && filtered
		}
		for j := range t.Cols {
			k := i*t.Cols + j
			x := t.Cells[k]
			if x.Type != TypeBlank && (out || skip&skipErrors != 0 && x.Type == TypeError ||
				skip&skipNested != 0 && nested != nil && nested.Subtotal(a.Sheet, row, a.C1+j)) {
				t.Cells[k] = Value{}
			}
		}
	}
	return Value{Type: TypeArray, Arr: t}
}

// roundWith rounds n to d decimal places with f, on the number as Excel
// shows it, 15 significant digits, so that 2.675 rounds up.
func roundWith(n float64, d int, f func(float64) float64) float64 {
	if d > 15 || n == 0 {
		return n
	}
	p := math.Pow(10, float64(d))
	x, _ := strconv.ParseFloat(strconv.FormatFloat(n*p, 'e', 14, 64), 64)
	return f(x) / p
}

// multiple is n rounded by f to a multiple of s.
func multiple(n, s float64, f func(float64) float64) float64 {
	q, _ := strconv.ParseFloat(strconv.FormatFloat(n/s, 'e', 14, 64), 64)
	return f(q) * s
}

// roundTo is CEILING.MATH and its kin: to a multiple of the absolute
// significance; with a mode, negative numbers go away from zero.
func roundTo(f func(float64) float64, mode bool) *function {
	max := 2
	if mode {
		max = 3
	}
	return fn(1, max, func(c *Context, v []Value) Value {
		n, err := c.number(v[0])
		if err != nil {
			return *err
		}
		s := 1.0
		if len(v) > 1 && v[1].Type != TypeBlank {
			if s, err = c.number(v[1]); err != nil {
				return *err
			}
		}
		s = math.Abs(s)
		if s == 0 {
			return Num(0)
		}
		away := false
		if len(v) > 2 {
			m, err := c.number(v[2])
			if err != nil {
				return *err
			}
			away = m != 0 && n < 0
		}
		if away {
			return Num(-multiple(-n, s, f))
		}
		return Num(multiple(n, s, f))
	})
}

func combin(n, k float64) float64 {
	k = min(k, n-k)
	r := 1.0
	for i := 1.0; i <= k; i++ {
		r = r * (n - k + i) / i
	}
	return math.Round(r)
}

func gcd(a, b float64) float64 {
	for b != 0 {
		a, b = b, math.Mod(a, b)
	}
	return a
}
