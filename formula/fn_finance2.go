package formula

import "math"

func init() {
	add(map[string]*function{
		"XNPV": fn(3, 3, func(c *Context, v []Value) Value {
			rate, err := c.number(v[0])
			if err != nil {
				return *err
			}
			flows, days, e := c.dated(v[1], v[2])
			if e != nil {
				return *e
			}
			if rate <= -1 {
				return ErrNum
			}
			sum := 0.0
			for i, f := range flows {
				sum += f / math.Pow(1+rate, days[i]/365)
			}
			return Num(sum)
		}),
		"XIRR": fn(2, 3, func(c *Context, v []Value) Value {
			flows, days, e := c.dated(v[0], v[1])
			if e != nil {
				return *e
			}
			guess := 0.1
			if len(v) > 2 && v[2].Type != TypeBlank {
				var err *Value
				if guess, err = c.number(v[2]); err != nil {
					return *err
				}
			}
			if guess <= -1 || !mixed(flows) {
				return ErrNum
			}
			return xirr(flows, days, guess)
		}),
		"MIRR": fn(3, 3, func(c *Context, v []Value) Value {
			flows, e := c.series(v[0])
			if e != nil {
				return *e
			}
			finance, err := c.number(v[1])
			if err != nil {
				return *err
			}
			reinvest, err := c.number(v[2])
			if err != nil {
				return *err
			}
			if !mixed(flows) {
				return ErrDiv0
			}
			var in, out float64
			for i, f := range flows {
				if f > 0 {
					in += f * math.Pow(1+reinvest, float64(len(flows)-1-i))
				} else {
					out += f / math.Pow(1+finance, float64(i))
				}
			}
			return Num(math.Pow(-in/out, 1/float64(len(flows)-1)) - 1)
		}),
		"DB": numbers(4, 5, func(n []float64) Value {
			cost, salvage, life, per := n[0], n[1], math.Trunc(n[2]), math.Trunc(n[3])
			month := 12.0
			if len(n) > 4 {
				month = math.Trunc(n[4])
			}
			if cost <= 0 || salvage < 0 || life < 1 || per < 1 || month < 1 || month > 12 || per > life+1 || per > life && month == 12 {
				return ErrNum
			}
			rate := math.Round((1-math.Pow(salvage/cost, 1/life))*1000) / 1000
			total, dep := 0.0, 0.0
			for p := 1.0; p <= per; p++ {
				switch {
				case p == 1:
					dep = cost * rate * month / 12
				case p == life+1:
					dep = (cost - total) * rate * (12 - month) / 12
				default:
					dep = (cost - total) * rate
				}
				total += dep
			}
			return Num(dep)
		}),
		"VDB": numbers(5, 7, func(n []float64) Value {
			cost, salvage, life, start, end := n[0], n[1], n[2], n[3], n[4]
			factor := 2.0
			if len(n) > 5 {
				factor = n[5]
			}
			if cost < 0 || salvage < 0 || life <= 0 || start < 0 || end < start || end > life || factor < 0 {
				return ErrNum
			}
			return Num(vdb(cost, salvage, life, start, end, factor, at(n, 6) != 0))
		}),
		"CUMIPMT": numbers(6, 6, func(n []float64) Value {
			i, _, err := cumulative(n)
			if err != nil {
				return *err
			}
			return Num(i)
		}),
		"CUMPRINC": numbers(6, 6, func(n []float64) Value {
			_, p, err := cumulative(n)
			if err != nil {
				return *err
			}
			return Num(p)
		}),
		"RRI": numbers(3, 3, func(n []float64) Value {
			nper, pv, fv := n[0], n[1], n[2]
			if nper <= 0 || pv == 0 {
				return ErrNum
			}
			r := math.Pow(fv/pv, 1/nper) - 1
			if math.IsNaN(r) || math.IsInf(r, 0) {
				return ErrNum
			}
			return Num(r)
		}),
		"FVSCHEDULE": fn(2, 2, func(c *Context, v []Value) Value {
			principal, err := c.number(v[0])
			if err != nil {
				return *err
			}
			rates, e := c.series(v[1])
			if e != nil {
				return *e
			}
			for _, r := range rates {
				principal *= 1 + r
			}
			return Num(principal)
		}),
		"DOLLARDE": numbers(2, 2, func(n []float64) Value {
			return dollar(n, func(whole, part, fraction, digits float64) float64 {
				return whole + part*digits/fraction
			})
		}),
		"DOLLARFR": numbers(2, 2, func(n []float64) Value {
			return dollar(n, func(whole, part, fraction, digits float64) float64 {
				return whole + part*fraction/digits
			})
		}),
	})
}

// numbersIn is a function of numbers, each argument converted first.
func numbersIn(min, max int, f func(c *Context, n []float64) Value) *function {
	return fn(min, max, func(c *Context, v []Value) Value {
		n := make([]float64, len(v))
		for i, x := range v {
			var err *Value
			if n[i], err = c.number(x); err != nil {
				return *err
			}
		}
		return f(c, n)
	})
}

// series is the numbers a range or an array holds, the text and the blank
// cells left out, as IRR reads its flows.
func (c *Context) series(v Value) ([]float64, *Value) {
	var out []float64
	var err *Value
	c.eachOf(v, func(x Value, _ bool) bool {
		switch x.Type {
		case TypeError:
			found := x
			err = &found
			return false
		case TypeNumber:
			out = append(out, x.Num)
		}
		return true
	})
	return out, err
}

// dated is the flows and the days since the first date they fall on, as
// XNPV and XIRR take them.
func (c *Context) dated(values, dates Value) (flows, days []float64, err *Value) {
	flows, err = c.series(values)
	if err != nil {
		return nil, nil, err
	}
	days, err = c.series(dates)
	if err != nil {
		return nil, nil, err
	}
	if len(flows) != len(days) || len(flows) == 0 {
		return nil, nil, &ErrNum
	}
	first := math.Floor(days[0])
	for i, d := range days {
		days[i] = math.Floor(d) - first
		if days[i] < 0 {
			return nil, nil, &ErrNum
		}
	}
	return flows, days, nil
}

// mixed tells whether flows hold both a payment and a receipt.
func mixed(flows []float64) bool {
	var pos, neg bool
	for _, f := range flows {
		pos = pos || f > 0
		neg = neg || f < 0
	}
	return pos && neg
}

// xirr is the rate that cancels flows paid after days, found by Newton's
// method as Excel does, in 100 steps at most.
func xirr(flows, days []float64, guess float64) Value {
	r := guess
	for range 100 {
		var y, d float64
		for i, f := range flows {
			t := days[i] / 365
			g := math.Pow(1+r, -t)
			y += f * g
			d -= t * f * g / (1 + r)
		}
		if d == 0 || math.IsNaN(d) || math.IsInf(d, 0) {
			return ErrNum
		}
		next := r - y/d
		if next <= -1 {
			next = (r - 1) / 2
		}
		if math.Abs(next-r) < 1e-12 {
			return Num(next)
		}
		r = next
	}
	return ErrNum
}

// vdb is the depreciation between two periods, the double-declining
// balance switching to the straight line when that depreciates more,
// unless noSwitch. A period only partly between them counts for its part.
func vdb(cost, salvage, life, start, end, factor float64, noSwitch bool) float64 {
	total, book := 0.0, cost
	for p := 1.0; p <= math.Ceil(end); p++ {
		dep := book * factor / life
		if !noSwitch {
			dep = max(dep, (book-salvage)/(life-p+1))
		}
		dep = min(dep, max(book-salvage, 0))
		total += dep * max(min(end, p)-max(start, p-1), 0)
		book -= dep
	}
	return total
}

// cumulative is the interest and the principal paid between two periods
// of a loan: rate, periods, present value, first and last period, type.
func cumulative(n []float64) (float64, float64, *Value) {
	rate, nper, pv, first, last, due := n[0], n[1], n[2], math.Trunc(n[3]), math.Trunc(n[4]), n[5]
	if rate <= 0 || nper <= 0 || pv <= 0 || first < 1 || last < first || last > math.Ceil(nper) || due != 0 && due != 1 {
		return 0, 0, &ErrNum
	}
	var interests, principals float64
	for p := first; p <= last; p++ {
		i, k, err := interest([]float64{rate, p, nper, pv, 0, due})
		if err != nil {
			return 0, 0, err
		}
		interests += i
		principals += k
	}
	return interests, principals, nil
}

// dollar converts a price between a decimal and a fraction written after
// the point: the whole part, the part after the point, the fraction, and
// the power of ten that holds the fraction.
func dollar(n []float64, f func(whole, part, fraction, digits float64) float64) Value {
	fraction := math.Trunc(n[1])
	switch {
	case fraction < 0:
		return ErrNum
	case fraction == 0:
		return ErrDiv0
	}
	whole, part := math.Modf(n[0])
	return Num(f(whole, part, fraction, math.Pow(10, math.Ceil(math.Log10(fraction)))))
}
