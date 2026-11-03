package formula

import "math"

func init() {
	add(map[string]*function{
		"PMT": numbers(3, 5, func(n []float64) Value {
			return Num(pmt(n[0], n[1], n[2], at(n, 3), at(n, 4)))
		}),
		"PV": numbers(3, 5, func(n []float64) Value {
			rate, nper, p, fv, due := n[0], n[1], n[2], at(n, 3), at(n, 4)
			if rate == 0 {
				return Num(-p*nper - fv)
			}
			g := math.Pow(1+rate, nper)
			return Num(-(fv + p*(1+rate*due)*(g-1)/rate) / g)
		}),
		"FV": numbers(3, 5, func(n []float64) Value {
			return Num(fv(n[0], n[1], n[2], at(n, 3), at(n, 4)))
		}),
		"NPER": numbers(3, 5, func(n []float64) Value {
			rate, p, pv, fv, due := n[0], n[1], n[2], at(n, 3), at(n, 4)
			if rate == 0 {
				if p == 0 {
					return ErrNum
				}
				return Num(-(pv + fv) / p)
			}
			a := p * (1 + rate*due) / rate
			x := (a - fv) / (a + pv)
			if x <= 0 {
				return ErrNum
			}
			return Num(math.Log(x) / math.Log(1+rate))
		}),
		"RATE": numbers(3, 6, func(n []float64) Value {
			nper, p, pv, fv, due := n[0], n[1], n[2], at(n, 3), at(n, 4)
			guess := 0.1
			if len(n) > 5 {
				guess = n[5]
			}
			f := func(r float64) float64 {
				if math.Abs(r) < 1e-12 {
					return pv + p*nper + fv
				}
				g := math.Pow(1+r, nper)
				return pv*g + p*(1+r*due)*(g-1)/r + fv
			}
			return newton(f, guess)
		}),
		"IPMT": numbers(4, 6, func(n []float64) Value {
			i, _, err := interest(n)
			if err != nil {
				return *err
			}
			return Num(i)
		}),
		"PPMT": numbers(4, 6, func(n []float64) Value {
			_, p, err := interest(n)
			if err != nil {
				return *err
			}
			return Num(p)
		}),
		"NPV": {min: 2, max: 255, call: func(c *Context, args []Expr) Value {
			rate, err := c.number(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			sum, i := 0.0, 1.0
			if err := c.numbers(args[1:], func(x float64) {
				sum += x / math.Pow(1+rate, i)
				i++
			}); err != nil {
				return *err
			}
			return Num(sum)
		}},
		"IRR": fn(1, 2, func(c *Context, v []Value) Value {
			var flows []float64
			var err *Value
			c.eachOf(v[0], func(x Value, _ bool) bool {
				switch x.Type {
				case TypeError:
					found := x
					err = &found
					return false
				case TypeNumber:
					flows = append(flows, x.Num)
				}
				return true
			})
			if err != nil {
				return *err
			}
			guess := 0.1
			if len(v) > 1 && v[1].Type != TypeBlank {
				if guess, err = c.number(v[1]); err != nil {
					return *err
				}
			}
			return newton(func(r float64) float64 {
				s := 0.0
				for i, f := range flows {
					s += f / math.Pow(1+r, float64(i))
				}
				return s
			}, guess)
		}),
		"SLN": numbers(3, 3, func(n []float64) Value {
			if n[2] == 0 {
				return ErrDiv0
			}
			return Num((n[0] - n[1]) / n[2])
		}),
		"SYD": numbers(4, 4, func(n []float64) Value {
			cost, salvage, life, per := n[0], n[1], n[2], n[3]
			if life <= 0 || per <= 0 || per > life {
				return ErrNum
			}
			return Num((cost - salvage) * (life - per + 1) * 2 / (life * (life + 1)))
		}),
		"DDB": numbers(4, 5, func(n []float64) Value {
			cost, salvage, life, per := n[0], n[1], n[2], n[3]
			factor := 2.0
			if len(n) > 4 {
				factor = n[4]
			}
			if life <= 0 || per <= 0 || per > life || factor <= 0 || cost < 0 || salvage < 0 {
				return ErrNum
			}
			value, dep := cost, 0.0
			for p := 1.0; p <= math.Ceil(per); p++ {
				dep = min(value*factor/life, max(value-salvage, 0))
				value -= dep
			}
			return Num(dep)
		}),
		"EFFECT": numbers(2, 2, func(n []float64) Value {
			k := math.Trunc(n[1])
			if n[0] <= 0 || k < 1 {
				return ErrNum
			}
			return Num(math.Pow(1+n[0]/k, k) - 1)
		}),
		"NOMINAL": numbers(2, 2, func(n []float64) Value {
			k := math.Trunc(n[1])
			if n[0] <= 0 || k < 1 {
				return ErrNum
			}
			return Num(k * (math.Pow(1+n[0], 1/k) - 1))
		}),
	})
}

// numbers is a function of numbers, each argument converted first.
func numbers(min, max int, f func(n []float64) Value) *function {
	return numbersIn(min, max, func(_ *Context, n []float64) Value { return f(n) })
}

// at is the i-th number, 0 when left out.
func at(n []float64, i int) float64 {
	if i < len(n) {
		return n[i]
	}
	return 0
}

func pmt(rate, nper, pv, fv, due float64) float64 {
	if rate == 0 {
		return -(pv + fv) / nper
	}
	due = math.Min(1, math.Abs(due))
	g := math.Pow(1+rate, nper)
	return -(rate * (fv + pv*g)) / ((1 + rate*due) * (g - 1))
}

func fv(rate, nper, p, pv, due float64) float64 {
	if rate == 0 {
		return -pv - p*nper
	}
	due = math.Min(1, math.Abs(due))
	g := math.Pow(1+rate, nper)
	return -pv*g - p*(1+rate*due)*(g-1)/rate
}

// interest is the interest and the principal paid in a period, as IPMT
// and PPMT: rate, period, periods, present value, future value, type.
func interest(n []float64) (float64, float64, *Value) {
	rate, per, nper, pv, f, due := n[0], n[1], n[2], n[3], at(n, 4), at(n, 5)
	if per < 1 || per > nper {
		return 0, 0, &ErrNum
	}
	p := pmt(rate, nper, pv, f, due)
	var i float64
	switch {
	case per == 1 && due != 0:
		i = 0
	case due != 0:
		i = fv(rate, per-2, p, pv, 1) * rate
	default:
		i = fv(rate, per-1, p, pv, 0) * rate
	}
	return i, p - i, nil
}

// newton finds where f is zero, from a guess, as RATE and IRR do.
func newton(f func(float64) float64, guess float64) Value {
	r := guess
	for range 100 {
		y := f(r)
		if math.Abs(y) < 1e-10 {
			return Num(r)
		}
		h := 1e-7 * math.Max(1, math.Abs(r))
		d := (f(r+h) - y) / h
		if d == 0 || math.IsNaN(d) {
			return ErrNum
		}
		next := r - y/d
		if math.Abs(next-r) < 1e-12 {
			return Num(next)
		}
		r = next
		if r <= -1 {
			r = -0.999999
		}
	}
	return ErrNum
}
