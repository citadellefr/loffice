package formula

import (
	"math"
	"time"
)

func init() {
	add(map[string]*function{
		"COUPDAYBS": coupon(func(b *bond) float64 { return b.daysSincePrevious() }),
		"COUPDAYS":  coupon(func(b *bond) float64 { return b.period() }),
		"COUPDAYSNC": coupon(func(b *bond) float64 {
			return b.daysToNext()
		}),
		"COUPNCD": coupon(func(b *bond) float64 { return b.next }),
		"COUPPCD": coupon(func(b *bond) float64 { return b.previous }),
		"COUPNUM": coupon(func(b *bond) float64 { return b.coupons }),
		"PRICE": numbersIn(6, 7, func(c *Context, n []float64) Value {
			b, rate, yld, redemption, err := c.priced(n)
			if err != nil {
				return *err
			}
			return Num(b.price(rate, yld, redemption))
		}),
		"YIELD": numbersIn(6, 7, func(c *Context, n []float64) Value {
			b, rate, price, redemption, err := c.priced(n)
			if err != nil {
				return *err
			}
			if price <= 0 {
				return ErrNum
			}
			return b.yield(rate, price, redemption)
		}),
		"DURATION": numbersIn(5, 6, func(c *Context, n []float64) Value {
			return c.duration(n, false)
		}),
		"MDURATION": numbersIn(5, 6, func(c *Context, n []float64) Value {
			return c.duration(n, true)
		}),
		"ACCRINT": numbersIn(6, 8, func(c *Context, n []float64) Value {
			issue, first, settle, rate, par := math.Trunc(n[0]), math.Trunc(n[1]), math.Trunc(n[2]), n[3], n[4]
			if rate <= 0 || par <= 0 || issue >= settle {
				return ErrNum
			}
			b, err := c.schedule(n[5], at(n, 6))
			if err != nil {
				return *err
			}
			start := issue
			if len(n) > 7 && n[7] == 0 && settle > first {
				start = first
			}
			return Num(par * rate / n[5] * b.accrued(first, start, settle))
		}),
		"ACCRINTM": numbersIn(3, 5, func(c *Context, n []float64) Value {
			issue, settle, rate, par := math.Trunc(n[0]), math.Trunc(n[1]), n[2], 1000.0
			if len(n) > 3 {
				par = n[3]
			}
			if rate <= 0 || par <= 0 || issue >= settle {
				return ErrNum
			}
			return c.discounted(issue, settle, at(n, 4), func(frac float64) float64 {
				return par * rate * frac
			})
		}),
		"PRICEDISC": numbersIn(4, 5, func(c *Context, n []float64) Value {
			if n[2] <= 0 || n[3] <= 0 {
				return ErrNum
			}
			return c.discounted(n[0], n[1], at(n, 4), func(frac float64) float64 {
				return n[3] - n[2]*n[3]*frac
			})
		}),
		"YIELDDISC": numbersIn(4, 5, func(c *Context, n []float64) Value {
			if n[2] <= 0 || n[3] <= 0 {
				return ErrNum
			}
			return c.discounted(n[0], n[1], at(n, 4), func(frac float64) float64 {
				return (n[3] - n[2]) / n[2] / frac
			})
		}),
		"DISC": numbersIn(4, 5, func(c *Context, n []float64) Value {
			if n[2] <= 0 || n[3] <= 0 {
				return ErrNum
			}
			return c.discounted(n[0], n[1], at(n, 4), func(frac float64) float64 {
				return (n[3] - n[2]) / n[3] / frac
			})
		}),
		"INTRATE": numbersIn(4, 5, func(c *Context, n []float64) Value {
			if n[2] <= 0 || n[3] <= 0 {
				return ErrNum
			}
			return c.discounted(n[0], n[1], at(n, 4), func(frac float64) float64 {
				return (n[3] - n[2]) / n[2] / frac
			})
		}),
		"RECEIVED": numbersIn(4, 5, func(c *Context, n []float64) Value {
			if n[2] <= 0 || n[3] <= 0 {
				return ErrNum
			}
			return c.discounted(n[0], n[1], at(n, 4), func(frac float64) float64 {
				return n[2] / (1 - n[3]*frac)
			})
		}),
		"TBILLEQ": numbersIn(3, 3, func(c *Context, n []float64) Value {
			return c.bill(n, func(days float64) float64 {
				return 365 * n[2] / (360 - n[2]*days)
			})
		}),
		"TBILLPRICE": numbersIn(3, 3, func(c *Context, n []float64) Value {
			return c.bill(n, func(days float64) float64 {
				return 100 * (1 - n[2]*days/360)
			})
		}),
		"TBILLYIELD": numbersIn(3, 3, func(c *Context, n []float64) Value {
			return c.bill(n, func(days float64) float64 {
				return (100 - n[2]) / n[2] * 360 / days
			})
		}),
	})
}

// bond is a security that pays interest every so often: the days it is
// settled and matures, and the coupons around the settlement.
type bond struct {
	c                *Context
	settle, maturity float64
	frequency        float64
	basis            int
	previous, next   float64
	coupons          float64
}

// schedule is a bond paid every so often and counted as the basis says,
// or the error its arguments call for.
func (c *Context) schedule(frequency, basis float64) (*bond, *Value) {
	frequency, basis = math.Trunc(frequency), math.Trunc(basis)
	if basis < 0 || basis > 4 || frequency != 1 && frequency != 2 && frequency != 4 {
		return nil, &ErrNum
	}
	return &bond{c: c, frequency: frequency, basis: int(basis)}, nil
}

// bondOf is a bond with its coupons around the settlement, or the error
// its arguments call for.
func (c *Context) bondOf(settle, maturity, frequency, basis float64) (*bond, *Value) {
	b, err := c.schedule(frequency, basis)
	if err != nil {
		return nil, err
	}
	settle, maturity = math.Trunc(settle), math.Trunc(maturity)
	if settle < 0 || maturity > 2958465 || settle >= maturity {
		return nil, &ErrNum
	}
	b.settle, b.maturity = settle, maturity
	k := 1.0
	for b.shifted(maturity, -k) > settle {
		k++
	}
	b.previous, b.next, b.coupons = b.shifted(maturity, -k), b.shifted(maturity, -k+1), k
	return b, nil
}

// shifted is the day a number of coupon periods after the one given, on
// the same day of the month, or the last day of it if the one given was.
func (b *bond) shifted(day, periods float64) float64 {
	d := dayOf(day, b.c.Date1904)
	months := d.y*12 + d.m - 1 + int(periods*12/b.frequency)
	y, m := months/12, months%12+1
	last := daysIn(y, m)
	if d.d == daysIn(d.y, d.m) {
		d.d = last
	}
	return Serial(time.Date(y, time.Month(m), min(d.d, last), 0, 0, 0, 0, time.UTC), b.c.Date1904)
}

// count is the days between two days, counted as the basis says.
func (b *bond) count(from, to float64) float64 {
	if b.basis == 0 || b.basis == 4 {
		return days360(dayOf(from, b.c.Date1904), dayOf(to, b.c.Date1904), b.basis == 4)
	}
	return to - from
}

// period is the days of a coupon period.
func (b *bond) period() float64 {
	switch b.basis {
	case 1:
		return b.next - b.previous
	case 3:
		return 365 / b.frequency
	}
	return 360 / b.frequency
}

func (b *bond) daysSincePrevious() float64 { return b.count(b.previous, b.settle) }

func (b *bond) daysToNext() float64 {
	if b.basis == 0 || b.basis == 4 {
		return b.period() - b.daysSincePrevious()
	}
	return b.next - b.settle
}

// accrued is the coupon periods of interest earned from start to the
// settlement, the periods counted from the day the first coupon is paid.
func (b *bond) accrued(first, start, settle float64) float64 {
	k := 0.0
	for b.shifted(first, k) > start {
		k--
	}
	for b.shifted(first, k+1) <= start {
		k++
	}
	sum := 0.0
	for from := b.shifted(first, k); from < settle; k++ {
		to := b.shifted(first, k+1)
		length := to - from
		if b.basis != 1 {
			length = 360 / b.frequency
			if b.basis == 3 {
				length = 365 / b.frequency
			}
		}
		sum += b.count(max(from, start), min(to, settle)) / length
		from = to
	}
	return sum
}

// priced reads the arguments PRICE and YIELD share: settlement, maturity,
// rate, yield or price, redemption, frequency, basis.
func (c *Context) priced(n []float64) (b *bond, rate, x, redemption float64, err *Value) {
	b, err = c.bondOf(n[0], n[1], n[5], at(n, 6))
	if err != nil {
		return nil, 0, 0, 0, err
	}
	rate, x, redemption = n[2], n[3], n[4]
	if rate < 0 || x < 0 || redemption <= 0 {
		return nil, 0, 0, 0, &ErrNum
	}
	return b, rate, x, redemption, nil
}

// price is what a security of 100 is worth for a yield.
func (b *bond) price(rate, yld, redemption float64) float64 {
	e, a := b.period(), b.daysSincePrevious()
	next := b.daysToNext() / e
	coupon := 100 * rate / b.frequency
	if b.coupons == 1 {
		return (redemption+coupon)/(1+next*yld/b.frequency) - coupon*a/e
	}
	growth := 1 + yld/b.frequency
	p := redemption/math.Pow(growth, b.coupons-1+next) - coupon*a/e
	for k := 0.0; k < b.coupons; k++ {
		p += coupon / math.Pow(growth, k+next)
	}
	return p
}

// yield is the yield that gives a security of 100 a price.
func (b *bond) yield(rate, price, redemption float64) Value {
	if b.coupons == 1 {
		e, a := b.period(), b.daysSincePrevious()
		coupon := 100 * rate / b.frequency
		paid := price + coupon*a/e
		return Num(((redemption + coupon - paid) / paid) * b.frequency * e / b.daysToNext())
	}
	return newton(func(y float64) float64 { return b.price(rate, y, redemption) - price }, 0.1)
}

// duration is the weighted average of the times the cash flows of a
// security are paid at, in years, or its modified form.
func (c *Context) duration(n []float64, modified bool) Value {
	b, err := c.bondOf(n[0], n[1], n[4], at(n, 5))
	if err != nil {
		return *err
	}
	rate, yld := n[2], n[3]
	if rate < 0 || yld < 0 {
		return ErrNum
	}
	coupon := 100 * rate / b.frequency
	growth := 1 + yld/b.frequency
	next := b.daysToNext() / b.period()
	var weighted, value float64
	for k := 0.0; k < b.coupons; k++ {
		flow := coupon
		if k == b.coupons-1 {
			flow += 100
		}
		t := k + next
		pv := flow / math.Pow(growth, t)
		weighted += t * pv
		value += pv
	}
	d := weighted / value / b.frequency
	if modified {
		d /= growth
	}
	return Num(d)
}

// coupon is a function of the days around a coupon of a bond: settlement,
// maturity, frequency, basis.
func coupon(f func(b *bond) float64) *function {
	return numbersIn(3, 4, func(c *Context, n []float64) Value {
		b, err := c.bondOf(n[0], n[1], n[2], at(n, 3))
		if err != nil {
			return *err
		}
		return Num(f(b))
	})
}

// discounted calls f with the years from settlement to maturity, counted
// as the basis says.
func (c *Context) discounted(settle, maturity, basis float64, f func(frac float64) float64) Value {
	settle, maturity, basis = math.Trunc(settle), math.Trunc(maturity), math.Trunc(basis)
	if settle < 0 || maturity > 2958465 || settle >= maturity || basis < 0 || basis > 4 {
		return ErrNum
	}
	frac := c.yearFrac(settle, maturity, int(basis))
	return Num(f(frac.Num))
}

// bill calls f with the days from settlement to maturity of a treasury
// bill of a year at most, for a rate or a price greater than nothing.
func (c *Context) bill(n []float64, f func(days float64) float64) Value {
	settle, maturity := math.Trunc(n[0]), math.Trunc(n[1])
	if settle < 0 || settle >= maturity || maturity-settle > 365 || n[2] <= 0 {
		return ErrNum
	}
	r := f(maturity - settle)
	if math.IsNaN(r) || math.IsInf(r, 0) {
		return ErrNum
	}
	return Num(r)
}
