package formula

import (
	"math"
	"slices"
	"sort"
)

func init() {
	add(map[string]*function{
		"AVERAGE": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			sum, n := 0.0, 0
			if err := c.numbers(args, func(x float64) { sum, n = sum+x, n+1 }); err != nil {
				return *err
			}
			if n == 0 {
				return ErrDiv0
			}
			return Num(sum / float64(n))
		}},
		"AVERAGEA": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numbersA(args)
			if err != nil {
				return *err
			}
			if len(xs) == 0 {
				return ErrDiv0
			}
			return Num(sumOf(xs) / float64(len(xs)))
		}},
		"COUNT": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			n := 0
			c.each(args, func(v Value, direct bool) bool {
				if v.Type == TypeNumber {
					n++
				} else if direct && v.Type != TypeError {
					if _, err := c.number(v); err == nil && v.Type != TypeBlank {
						n++
					}
				}
				return true
			})
			return Num(float64(n))
		}},
		"COUNTA": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			n := 0
			for _, a := range args {
				if _, missing := a.(missingExpr); missing {
					n++
					continue
				}
				c.eachOf(c.Eval(a), func(v Value, direct bool) bool {
					if v.Type != TypeBlank || direct {
						n++
					}
					return true
				})
			}
			return Num(float64(n))
		}},
		"COUNTBLANK": {min: 1, max: 1, call: func(c *Context, args []Expr) Value {
			v := c.Eval(args[0])
			if v.Type != TypeRange || len(v.Refs) != 1 {
				return ErrValue
			}
			a := v.Refs[0]
			total := (a.R2 - a.R1 + 1) * (a.C2 - a.C1 + 1)
			c.Book.Cells(a, func(_, _ int, x Value) bool {
				if !(x.Type == TypeText && x.Str == "") {
					total--
				}
				return true
			})
			return Num(float64(total))
		}},
		"COUNTIF": {min: 2, max: 2, call: func(c *Context, args []Expr) Value {
			n := 0
			blanks, err := c.ifOne(args[:2], func(Value) { n++ })
			if err != nil {
				return *err
			}
			return Num(float64(n + blanks))
		}},
		"COUNTIFS": {min: 2, max: 254, call: func(c *Context, args []Expr) Value {
			n := 0
			blanks, err := c.ifs(args, false, func(Value) { n++ })
			if err != nil {
				return *err
			}
			return Num(float64(n + blanks))
		}},
		"AVERAGEIF": {min: 2, max: 3, call: func(c *Context, args []Expr) Value {
			sum, n := 0.0, 0
			if _, err := c.ifOne(args, func(v Value) {
				if v.Type == TypeNumber {
					sum, n = sum+v.Num, n+1
				}
			}); err != nil {
				return *err
			}
			if n == 0 {
				return ErrDiv0
			}
			return Num(sum / float64(n))
		}},
		"AVERAGEIFS": {min: 3, max: 255, call: func(c *Context, args []Expr) Value {
			sum, n := 0.0, 0
			if _, err := c.ifs(args, true, func(v Value) {
				if v.Type == TypeNumber {
					sum, n = sum+v.Num, n+1
				}
			}); err != nil {
				return *err
			}
			if n == 0 {
				return ErrDiv0
			}
			return Num(sum / float64(n))
		}},
		"MAXIFS": extremeIfs(math.Max),
		"MINIFS": extremeIfs(math.Min),
		"MAX":    extreme(math.Max, false),
		"MIN":    extreme(math.Min, false),
		"MAXA":   extreme(math.Max, true),
		"MINA":   extreme(math.Min, true),
		"MEDIAN": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			if len(xs) == 0 {
				return ErrNum
			}
			return Num(percentile(xs, 0.5))
		}},
		"MODE":      mode(),
		"MODE.SNGL": mode(),
		"LARGE":     nth(true),
		"SMALL":     nth(false),
		"RANK":      rank(false),
		"RANK.EQ":   rank(false),
		"RANK.AVG":  rank(true),
		"STDEV":     spread(true, true, false),
		"STDEV.S":   spread(true, true, false),
		"STDEVA":    spread(true, true, true),
		"STDEVP":    spread(false, true, false),
		"STDEV.P":   spread(false, true, false),
		"STDEVPA":   spread(false, true, true),
		"VAR":       spread(true, false, false),
		"VAR.S":     spread(true, false, false),
		"VARA":      spread(true, false, true),
		"VARP":      spread(false, false, false),
		"VAR.P":     spread(false, false, false),
		"VARPA":     spread(false, false, true),
		"PERCENTILE": quantile(func(xs []float64, k float64) Value {
			if k < 0 || k > 1 {
				return ErrNum
			}
			return Num(percentile(xs, k))
		}),
		"PERCENTILE.INC": quantile(func(xs []float64, k float64) Value {
			if k < 0 || k > 1 {
				return ErrNum
			}
			return Num(percentile(xs, k))
		}),
		"PERCENTILE.EXC": quantile(percentileExc),
		"QUARTILE": quantile(func(xs []float64, q float64) Value {
			q = math.Trunc(q)
			if q < 0 || q > 4 {
				return ErrNum
			}
			return Num(percentile(xs, q/4))
		}),
		"QUARTILE.INC": quantile(func(xs []float64, q float64) Value {
			q = math.Trunc(q)
			if q < 0 || q > 4 {
				return ErrNum
			}
			return Num(percentile(xs, q/4))
		}),
		"QUARTILE.EXC": quantile(func(xs []float64, q float64) Value {
			q = math.Trunc(q)
			if q <= 0 || q >= 4 {
				return ErrNum
			}
			return percentileExc(xs, q/4)
		}),
		"AVEDEV": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			if len(xs) == 0 {
				return ErrNum
			}
			m := sumOf(xs) / float64(len(xs))
			d := 0.0
			for _, x := range xs {
				d += math.Abs(x - m)
			}
			return Num(d / float64(len(xs)))
		}},
		"DEVSQ": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			m := sumOf(xs) / float64(max(len(xs), 1))
			d := 0.0
			for _, x := range xs {
				d += (x - m) * (x - m)
			}
			return Num(d)
		}},
		"GEOMEAN": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			if len(xs) == 0 {
				return ErrNum
			}
			l := 0.0
			for _, x := range xs {
				if x <= 0 {
					return ErrNum
				}
				l += math.Log(x)
			}
			return Num(math.Exp(l / float64(len(xs))))
		}},
		"HARMEAN": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			if len(xs) == 0 {
				return ErrNum
			}
			s := 0.0
			for _, x := range xs {
				if x <= 0 {
					return ErrNum
				}
				s += 1 / x
			}
			return Num(float64(len(xs)) / s)
		}},
		"CORREL":          pairs(correl),
		"PEARSON":         pairs(correl),
		"SLOPE":           pairs(func(ys, xs []float64) Value { return regression(ys, xs, true) }),
		"INTERCEPT":       pairs(func(ys, xs []float64) Value { return regression(ys, xs, false) }),
		"FORECAST":        forecast(),
		"FORECAST.LINEAR": forecast(),
		"COVARIANCE.P":    pairs(func(ys, xs []float64) Value { return covariance(ys, xs, false) }),
		"COVAR":           pairs(func(ys, xs []float64) Value { return covariance(ys, xs, false) }),
		"COVARIANCE.S":    pairs(func(ys, xs []float64) Value { return covariance(ys, xs, true) }),
		"STANDARDIZE": fn(3, 3, func(c *Context, v []Value) Value {
			var n [3]float64
			for i := range n {
				x, err := c.number(v[i])
				if err != nil {
					return *err
				}
				n[i] = x
			}
			if n[2] <= 0 {
				return ErrNum
			}
			return Num((n[0] - n[1]) / n[2])
		}),
	})
}

func sumOf(xs []float64) float64 {
	s := 0.0
	for _, x := range xs {
		s += x
	}
	return s
}

// numbersA is the values of the arguments as AVERAGEA takes them: text in
// ranges counts as 0, booleans as 1 or 0.
func (c *Context) numbersA(args []Expr) ([]float64, *Value) {
	var out []float64
	var err *Value
	c.each(args, func(v Value, direct bool) bool {
		switch v.Type {
		case TypeError:
			found := v
			err = &found
			return false
		case TypeNumber, TypeBool:
			out = append(out, v.Num)
		case TypeText:
			if direct {
				n, e := c.number(v)
				if e != nil {
					err = e
					return false
				}
				out = append(out, n)
			} else {
				out = append(out, 0)
			}
		}
		return true
	})
	return out, err
}

func extreme(f func(a, b float64) float64, a bool) *function {
	return &function{min: 1, max: 255, call: func(c *Context, args []Expr) Value {
		var xs []float64
		var err *Value
		if a {
			xs, err = c.numbersA(args)
		} else {
			xs, err = c.numberList(args)
		}
		if err != nil {
			return *err
		}
		if len(xs) == 0 {
			return Num(0)
		}
		m := xs[0]
		for _, x := range xs[1:] {
			m = f(m, x)
		}
		return Num(m)
	}}
}

func extremeIfs(f func(a, b float64) float64) *function {
	return &function{min: 3, max: 255, call: func(c *Context, args []Expr) Value {
		m, any := 0.0, false
		if _, err := c.ifs(args, true, func(v Value) {
			if v.Type != TypeNumber {
				return
			}
			if !any {
				m, any = v.Num, true
			} else {
				m = f(m, v.Num)
			}
		}); err != nil {
			return *err
		}
		return Num(m)
	}}
}

// ifOne is COUNTIF and SUMIF: f is given the values of the sum range, or
// of the range, where the range meets the criterion. The blank cells past
// those a sheet holds are not given, but counted when they meet it.
func (c *Context) ifOne(args []Expr, f func(Value)) (int, *Value) {
	r := c.Eval(args[0])
	if r.Type != TypeRange || len(r.Refs) != 1 {
		return 0, &ErrValue
	}
	cr := c.criterion(c.Eval(args[1]))
	area := r.Refs[0]
	target := area
	if len(args) > 2 {
		s := c.Eval(args[2])
		if s.Type != TypeRange || len(s.Refs) != 1 {
			return 0, &ErrValue
		}
		// the sum range takes the shape of the range, from its first cell
		t := s.Refs[0]
		target = Area3{t.Sheet, Area{R1: t.R1, C1: t.C1, R2: t.R1 + area.R2 - area.R1, C2: t.C1 + area.C2 - area.C1}}
	}
	tables := c.tables([]Area3{area, target})
	for i, v := range tables[0].Cells {
		if cr.match(v) {
			f(tables[1].Cells[i])
		}
	}
	return blanksPast(area, tables[0], cr.match(Value{})), nil
}

// blanksPast counts the cells of an area past those of its table, all
// blank, when blanks count.
func blanksPast(a Area3, t *Table, count bool) int {
	if !count {
		return 0
	}
	return (a.R2-a.R1+1)*(a.C2-a.C1+1) - len(t.Cells)
}

// ifs is COUNTIFS and SUMIFS, whose arguments are a range f is given the
// values of when sum is set, then ranges and their criteria.
func (c *Context) ifs(args []Expr, sum bool, f func(Value)) (int, *Value) {
	var areas []Area3
	var crits []criterion
	rest := args
	if sum {
		rest = args[1:]
	}
	if len(rest)%2 != 0 {
		return 0, &ErrValue
	}
	if sum {
		s := c.Eval(args[0])
		if s.Type != TypeRange || len(s.Refs) != 1 {
			return 0, &ErrValue
		}
		areas = append(areas, s.Refs[0])
	}
	for i := 0; i < len(rest); i += 2 {
		r := c.Eval(rest[i])
		if r.Type != TypeRange || len(r.Refs) != 1 {
			return 0, &ErrValue
		}
		areas = append(areas, r.Refs[0])
		crits = append(crits, c.criterion(c.Eval(rest[i+1])))
	}
	for _, a := range areas[1:] {
		if a.R2-a.R1 != areas[0].R2-areas[0].R1 || a.C2-a.C1 != areas[0].C2-areas[0].C1 {
			return 0, &ErrValue
		}
	}
	tables := c.tables(areas)
	first := 0
	if sum {
		first = 1
	}
	blank := true
cells:
	for i := range tables[0].Cells {
		for j, cr := range crits {
			if !cr.match(tables[first+j].Cells[i]) {
				continue cells
			}
		}
		f(tables[0].Cells[i])
	}
	for _, cr := range crits {
		blank = blank && cr.match(Value{})
	}
	return blanksPast(areas[0], tables[0], blank), nil
}

// tables are the values of areas of the same shape, bounded alike by the
// cells their sheets hold.
func (c *Context) tables(areas []Area3) []*Table {
	rows, cols := 0, 0
	for _, a := range areas {
		r, k := c.Book.Size(a.Sheet)
		rows, cols = max(rows, r-a.R1+1), max(cols, k-a.C1+1)
	}
	rows = min(max(rows, 1), areas[0].R2-areas[0].R1+1)
	cols = min(max(cols, 1), areas[0].C2-areas[0].C1+1)
	out := make([]*Table, len(areas))
	for i, a := range areas {
		a.R2, a.C2 = a.R1+rows-1, a.C1+cols-1
		out[i] = c.array(Value{Type: TypeRange, Refs: []Area3{a}})
	}
	return out
}

func mode() *function {
	return &function{min: 1, max: 255, call: func(c *Context, args []Expr) Value {
		xs, err := c.numberList(args)
		if err != nil {
			return *err
		}
		counts := map[float64]int{}
		best, bestN := 0.0, 0
		for _, x := range xs {
			counts[x]++
			if n := counts[x]; n > bestN {
				best, bestN = x, n
			}
		}
		if bestN < 2 {
			return ErrNA
		}
		return Num(best)
	}}
}

func nth(largest bool) *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		var xs []float64
		var err *Value
		c.eachOf(v[0], func(x Value, direct bool) bool {
			switch {
			case x.Type == TypeError:
				found := x
				err = &found
				return false
			case x.Type == TypeNumber:
				xs = append(xs, x.Num)
			}
			return true
		})
		if err != nil {
			return *err
		}
		k, err := c.number(v[1])
		if err != nil {
			return *err
		}
		i := int(math.Ceil(k))
		if i < 1 || i > len(xs) {
			return ErrNum
		}
		sort.Float64s(xs)
		if largest {
			return Num(xs[len(xs)-i])
		}
		return Num(xs[i-1])
	})
}

func rank(average bool) *function {
	return fn(2, 3, func(c *Context, v []Value) Value {
		x, err := c.number(v[0])
		if err != nil {
			return *err
		}
		if v[1].Type != TypeRange {
			return ErrValue
		}
		ascending := false
		if len(v) > 2 {
			o, err := c.number(v[2])
			if err != nil {
				return *err
			}
			ascending = o != 0
		}
		before, same := 0, 0
		var e *Value
		c.eachOf(v[1], func(y Value, _ bool) bool {
			if y.Type == TypeError {
				e = &y
				return false
			}
			if y.Type != TypeNumber {
				return true
			}
			switch {
			case y.Num == x:
				same++
			case ascending && y.Num < x, !ascending && y.Num > x:
				before++
			}
			return true
		})
		if e != nil {
			return *e
		}
		if same == 0 {
			return ErrNA
		}
		if average {
			return Num(float64(before) + float64(same+1)/2)
		}
		return Num(float64(before + 1))
	})
}

// spread is the variance or the standard deviation, of a sample or of the
// whole population.
func spread(sample, root, a bool) *function {
	return &function{min: 1, max: 255, call: func(c *Context, args []Expr) Value {
		var xs []float64
		var err *Value
		if a {
			xs, err = c.numbersA(args)
		} else {
			xs, err = c.numberList(args)
		}
		if err != nil {
			return *err
		}
		n := float64(len(xs))
		if sample && n < 2 || n < 1 {
			return ErrDiv0
		}
		m := sumOf(xs) / n
		d := 0.0
		for _, x := range xs {
			d += (x - m) * (x - m)
		}
		if sample {
			d /= n - 1
		} else {
			d /= n
		}
		if root {
			return Num(math.Sqrt(d))
		}
		return Num(d)
	}}
}

func quantile(f func(xs []float64, k float64) Value) *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		var xs []float64
		var err *Value
		c.eachOf(v[0], func(x Value, _ bool) bool {
			switch x.Type {
			case TypeError:
				found := x
				err = &found
				return false
			case TypeNumber:
				xs = append(xs, x.Num)
			}
			return true
		})
		if err != nil {
			return *err
		}
		if len(xs) == 0 {
			return ErrNum
		}
		k, err := c.number(v[1])
		if err != nil {
			return *err
		}
		return f(xs, k)
	})
}

// percentile interpolates between the sorted values, as PERCENTILE.INC.
func percentile(xs []float64, k float64) float64 {
	s := slices.Clone(xs)
	sort.Float64s(s)
	pos := k * float64(len(s)-1)
	i := int(math.Floor(pos))
	if i+1 >= len(s) {
		return s[len(s)-1]
	}
	return s[i] + (pos-float64(i))*(s[i+1]-s[i])
}

func percentileExc(xs []float64, k float64) Value {
	s := slices.Clone(xs)
	sort.Float64s(s)
	pos := k*float64(len(s)+1) - 1
	if pos < 0 || pos > float64(len(s)-1) {
		return ErrNum
	}
	i := int(math.Floor(pos))
	if i+1 >= len(s) {
		return Num(s[i])
	}
	return Num(s[i] + (pos-float64(i))*(s[i+1]-s[i]))
}

// pairs is a function of two ranges of numbers, taken where both hold one.
func pairs(f func(ys, xs []float64) Value) *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		a, b := c.pair(v[0], v[1])
		if len(a.Cells) != len(b.Cells) {
			return ErrNA
		}
		var ys, xs []float64
		for i := range a.Cells {
			y, x := a.Cells[i], b.Cells[i]
			if y.Type == TypeError {
				return y
			}
			if x.Type == TypeError {
				return x
			}
			if y.Type == TypeNumber && x.Type == TypeNumber {
				ys, xs = append(ys, y.Num), append(xs, x.Num)
			}
		}
		return f(ys, xs)
	})
}

func means(ys, xs []float64) (float64, float64) {
	return sumOf(ys) / float64(len(ys)), sumOf(xs) / float64(len(xs))
}

func correl(ys, xs []float64) Value {
	if len(xs) < 2 {
		return ErrDiv0
	}
	my, mx := means(ys, xs)
	var sxy, sxx, syy float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
		syy += (ys[i] - my) * (ys[i] - my)
	}
	if sxx == 0 || syy == 0 {
		return ErrDiv0
	}
	return Num(sxy / math.Sqrt(sxx*syy))
}

func regression(ys, xs []float64, slope bool) Value {
	if len(xs) < 1 {
		return ErrDiv0
	}
	my, mx := means(ys, xs)
	var sxy, sxx float64
	for i := range xs {
		sxy += (xs[i] - mx) * (ys[i] - my)
		sxx += (xs[i] - mx) * (xs[i] - mx)
	}
	if sxx == 0 {
		return ErrDiv0
	}
	b := sxy / sxx
	if slope {
		return Num(b)
	}
	return Num(my - b*mx)
}

func covariance(ys, xs []float64, sample bool) Value {
	n := float64(len(xs))
	if n == 0 || sample && n < 2 {
		return ErrDiv0
	}
	my, mx := means(ys, xs)
	s := 0.0
	for i := range xs {
		s += (xs[i] - mx) * (ys[i] - my)
	}
	if sample {
		return Num(s / (n - 1))
	}
	return Num(s / n)
}

func forecast() *function {
	return fn(3, 3, func(c *Context, v []Value) Value {
		x, err := c.number(v[0])
		if err != nil {
			return *err
		}
		slope := pairs(func(ys, xs []float64) Value { return regression(ys, xs, true) })
		intercept := pairs(func(ys, xs []float64) Value { return regression(ys, xs, false) })
		args := []Expr{valueExpr{v[1]}, valueExpr{v[2]}}
		b := slope.call(c, args)
		if b.Type == TypeError {
			return b
		}
		a := intercept.call(c, args)
		if a.Type == TypeError {
			return a
		}
		return Num(a.Num + b.Num*x)
	})
}

// valueExpr is a value already calculated, passed on to a function.
type valueExpr struct{ v Value }

func (valueExpr) expr() {}
