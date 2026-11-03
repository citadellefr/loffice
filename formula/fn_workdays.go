package formula

import (
	"math"
	"strings"
)

func init() {
	add(map[string]*function{
		"NETWORKDAYS.INTL": {min: 2, max: 4, call: func(c *Context, args []Expr) Value {
			a, err := c.serialOf(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			b, err := c.serialOf(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			off, holidays, e := c.calendar(args[2:])
			if e != nil {
				return *e
			}
			sign := 1.0
			a, b = math.Floor(a), math.Floor(b)
			if a > b {
				a, b, sign = b, a, -1
			}
			n := 0.0
			for d := a; d <= b; d++ {
				if !off[c.weekday(d)] && !holidays[d] {
					n++
				}
			}
			return Num(sign * n)
		}},
		"WORKDAY.INTL": {min: 2, max: 4, call: func(c *Context, args []Expr) Value {
			a, err := c.serialOf(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			k, err := c.number(c.Eval(args[1]))
			if err != nil {
				return *err
			}
			off, holidays, e := c.calendar(args[2:])
			if e != nil {
				return *e
			}
			d, step := math.Floor(a), 1.0
			if k < 0 {
				step = -1
			}
			for left := math.Abs(math.Trunc(k)); left > 0; {
				d += step
				if d < 0 || d > 2958465 {
					return ErrNum
				}
				if !off[c.weekday(d)] && !holidays[d] {
					left--
				}
			}
			return Num(d)
		}},
	})
}

// calendar is the days of the week that are not worked, from Sunday, and
// the holidays, from the weekend and holidays arguments.
func (c *Context) calendar(args []Expr) ([7]bool, map[float64]bool, *Value) {
	weekend := Value{}
	if len(args) > 0 {
		weekend = c.scalar(c.Eval(args[0]))
	}
	off, err := c.weekend(weekend)
	if err != nil {
		return off, nil, err
	}
	holidays, err := c.holidays(args[min(len(args), 1):])
	return off, holidays, err
}

// weekend is the days of the week a weekend argument makes days off: a
// number naming two days or one, or seven digits from Monday, 1 for a day
// off. A blank is Saturday and Sunday.
func (c *Context) weekend(v Value) ([7]bool, *Value) {
	var off [7]bool
	if v.Type == TypeText && len(v.Str) == 7 && onlyDigits(v.Str, "01") {
		for i, r := range v.Str {
			off[(i+1)%7] = r == '1'
		}
		if v.Str == "1111111" {
			return off, &ErrValue
		}
		return off, nil
	}
	code := 1.0
	if v.Type != TypeBlank {
		var err *Value
		if code, err = c.number(v); err != nil {
			return off, err
		}
	}
	switch code = math.Trunc(code); {
	case code >= 1 && code <= 7:
		off[(int(code)+5)%7], off[(int(code)+6)%7] = true, true
	case code >= 11 && code <= 17:
		off[int(code)-11] = true
	default:
		return off, &ErrNum
	}
	return off, nil
}

// onlyDigits tells whether s is made of the digits given.
func onlyDigits(s, digits string) bool {
	for _, r := range s {
		if !strings.ContainsRune(digits, r) {
			return false
		}
	}
	return true
}
