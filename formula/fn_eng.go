package formula

import (
	"math"
	"strconv"
	"strings"
)

// maxBit is what the bit functions take: numbers below 2^48.
const maxBit = 1 << 48

// baseBits is how many bits the negative numbers of a base are written on.
var baseBits = map[int]uint{2: 10, 8: 30, 16: 40}

func init() {
	add(map[string]*function{
		"BITAND":    bitwise(func(a, b uint64) uint64 { return a & b }),
		"BITOR":     bitwise(func(a, b uint64) uint64 { return a | b }),
		"BITXOR":    bitwise(func(a, b uint64) uint64 { return a ^ b }),
		"BITLSHIFT": shift(1),
		"BITRSHIFT": shift(-1),
		"BIN2DEC":   baseFunction(2, 10),
		"BIN2OCT":   baseFunction(2, 8),
		"BIN2HEX":   baseFunction(2, 16),
		"OCT2BIN":   baseFunction(8, 2),
		"OCT2DEC":   baseFunction(8, 10),
		"OCT2HEX":   baseFunction(8, 16),
		"HEX2BIN":   baseFunction(16, 2),
		"HEX2OCT":   baseFunction(16, 8),
		"HEX2DEC":   baseFunction(16, 10),
		"DEC2BIN":   baseFunction(10, 2),
		"DEC2OCT":   baseFunction(10, 8),
		"DEC2HEX":   baseFunction(10, 16),
		"ROMAN": fn(1, 2, func(c *Context, v []Value) Value {
			n, err := c.number(v[0])
			if err != nil {
				return *err
			}
			form := 0.0
			if len(v) > 1 {
				if v[1].Type == TypeBool {
					form = 4 * (1 - v[1].Num)
				} else if form, err = c.number(v[1]); err != nil {
					return *err
				}
			}
			n, form = math.Trunc(n), math.Trunc(form)
			if n < 0 || n > 3999 || form < 0 || form > 4 {
				return ErrValue
			}
			return Str(roman(int(n), int(form)))
		}),
		"ARABIC": text1Value(func(s string) Value {
			n, ok := arabic(s)
			if !ok {
				return ErrValue
			}
			return Num(float64(n))
		}),
		"CONVERT": fn(3, 3, func(c *Context, v []Value) Value {
			n, err := c.number(v[0])
			if err != nil {
				return *err
			}
			from, err := c.text(v[1])
			if err != nil {
				return *err
			}
			to, err := c.text(v[2])
			if err != nil {
				return *err
			}
			x, ok := convert(n, from, to)
			if !ok {
				return ErrNA
			}
			return finite(x)
		}),
		"DELTA":  math2(1, 0, func(a, b float64) Value { return Num(b2f(a == b)) }),
		"GESTEP": math2(1, 0, func(a, b float64) Value { return Num(b2f(a >= b)) }),
		"ERF": fn(1, 2, func(c *Context, v []Value) Value {
			lower, err := c.number(v[0])
			if err != nil {
				return *err
			}
			if len(v) == 1 {
				return Num(math.Erf(lower))
			}
			upper, err := c.number(v[1])
			if err != nil {
				return *err
			}
			return Num(math.Erf(upper) - math.Erf(lower))
		}),
		"ERFC": math1(func(x float64) Value { return Num(math.Erfc(x)) }),
		"BESSELJ": math2(2, 0, func(x, n float64) Value {
			if n < 0 {
				return ErrNum
			}
			return finite(math.Jn(int(n), x))
		}),
	})
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// bitNumber is an argument of the bit functions: a whole number from 0 to
// below 2^48.
func (c *Context) bitNumber(v Value) (uint64, *Value) {
	n, err := c.number(v)
	if err != nil {
		return 0, err
	}
	if n < 0 || n != math.Trunc(n) || n >= maxBit {
		return 0, &ErrNum
	}
	return uint64(n), nil
}

func bitwise(f func(a, b uint64) uint64) *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		a, err := c.bitNumber(v[0])
		if err != nil {
			return *err
		}
		b, err := c.bitNumber(v[1])
		if err != nil {
			return *err
		}
		return Num(float64(f(a, b)))
	})
}

// shift is BITLSHIFT for 1 and BITRSHIFT for -1.
func shift(dir int) *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		a, err := c.bitNumber(v[0])
		if err != nil {
			return *err
		}
		s, err := c.number(v[1])
		if err != nil {
			return *err
		}
		s = math.Trunc(s)
		if math.Abs(s) > 53 {
			return ErrNum
		}
		by := int(s) * dir
		var r uint64
		if by >= 0 {
			r = a << by
		} else {
			r = a >> -by
		}
		if r >= maxBit || by > 0 && r>>by != a {
			return ErrNum
		}
		return Num(float64(r))
	})
}

// baseFunction is the conversion of a number written in a base to another,
// with the places of the result for the bases written in digits.
func baseFunction(from, to int) *function {
	return fn(1, 2, func(c *Context, v []Value) Value {
		var n int64
		if from == 10 {
			x, err := c.number(v[0])
			if err != nil {
				return *err
			}
			x = math.Trunc(x)
			if math.Abs(x) >= 1<<40 {
				return ErrNum
			}
			n = int64(x)
		} else {
			s, err := c.text(v[0])
			if err != nil {
				return *err
			}
			var ok bool
			if n, ok = readBase(s, from); !ok {
				return ErrNum
			}
		}
		if to == 10 {
			return Num(float64(n))
		}
		places := 0
		if len(v) > 1 {
			p, err := c.number(v[1])
			if err != nil {
				return *err
			}
			if p = math.Trunc(p); p < 1 || p > 10 {
				return ErrNum
			}
			places = int(p)
		}
		bits := baseBits[to]
		if n < -1<<(bits-1) || n >= 1<<(bits-1) {
			return ErrNum
		}
		if n < 0 {
			n += 1 << bits
			places = 0
		}
		s := strings.ToUpper(strconv.FormatInt(n, to))
		if len(s) > 10 || places > 0 && len(s) > places {
			return ErrNum
		}
		if len(s) < places {
			s = strings.Repeat("0", places-len(s)) + s
		}
		return Str(s)
	})
}

// readBase reads a number of at most ten digits in a base, negative as
// the two's complement of the bits of the base.
func readBase(s string, base int) (int64, bool) {
	if len(s) > 10 {
		return 0, false
	}
	if s == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(s, base, 64)
	if err != nil || strings.ContainsAny(s, "+-_") {
		return 0, false
	}
	bits := baseBits[base]
	if n >= 1<<(bits-1) {
		n -= 1 << bits
	}
	return n, true
}

var romans = []struct {
	n    int
	form int
	text string
}{
	{1000, 0, "M"}, {900, 0, "CM"}, {500, 0, "D"}, {499, 4, "ID"}, {495, 3, "VD"},
	{490, 2, "XD"}, {450, 1, "LD"}, {400, 0, "CD"}, {100, 0, "C"}, {99, 4, "IC"}, {95, 3, "VC"},
	{90, 0, "XC"}, {50, 0, "L"}, {49, 2, "IL"}, {45, 1, "VL"}, {40, 0, "XL"}, {10, 0, "X"},
	{9, 0, "IX"}, {5, 0, "V"}, {4, 0, "IV"}, {1, 0, "I"},
}

func roman(n, form int) string {
	var b strings.Builder
	for _, r := range romans {
		if r.form > form {
			continue
		}
		for n >= r.n {
			b.WriteString(r.text)
			n -= r.n
		}
	}
	return b.String()
}

func arabic(s string) (int, bool) {
	s = strings.ToUpper(s)
	sign := 1
	if strings.HasPrefix(s, "-") {
		sign, s = -1, s[1:]
	}
	if len(s) > 255 {
		return 0, false
	}
	digits := map[byte]int{'I': 1, 'V': 5, 'X': 10, 'L': 50, 'C': 100, 'D': 500, 'M': 1000}
	total, last := 0, 0
	for i := len(s) - 1; i >= 0; i-- {
		d, ok := digits[s[i]]
		if !ok {
			return 0, false
		}
		if d < last {
			total -= d
		} else {
			total += d
			last = d
		}
	}
	return sign * total, true
}

type unit struct {
	kind   string
	factor float64 // of the base unit of its kind
	prefix bool    // the unit takes the prefixes of the metric system
}

const (
	pound = 453.59237 // grams
	inch  = 0.0254    // meters
	hour  = 3600.0
	day   = 86400.0
)

var units = map[string]unit{
	"g": {"mass", 1, true}, "sg": {"mass", 14593.90294, false}, "lbm": {"mass", pound, false},
	"u": {"mass", 1.66053906660e-24, true}, "ozm": {"mass", pound / 16, false},
	"grain": {"mass", pound / 7000, false}, "cwt": {"mass", 100 * pound, false}, "shweight": {"mass", 100 * pound, false},
	"uk_cwt": {"mass", 112 * pound, false}, "lcwt": {"mass", 112 * pound, false}, "hweight": {"mass", 112 * pound, false},
	"stone": {"mass", 14 * pound, false}, "ton": {"mass", 2000 * pound, false}, "uk_ton": {"mass", 2240 * pound, false},
	"LTON": {"mass", 2240 * pound, false}, "brton": {"mass", 2240 * pound, false},

	"m": {"length", 1, true}, "mi": {"length", 1609.344, false}, "Nmi": {"length", 1852, false},
	"in": {"length", inch, false}, "ft": {"length", 12 * inch, false}, "yd": {"length", 36 * inch, false},
	"ang": {"length", 1e-10, true}, "ell": {"length", 45 * inch, false}, "ly": {"length", 9.46073047258e15, false},
	"parsec": {"length", 3.08567758149137e16, false}, "pc": {"length", 3.08567758149137e16, false},
	"Pica": {"length", inch / 72, false}, "pica": {"length", inch / 6, false},
	"survey_mi": {"length", 6336000 / 3937.0, false},

	"yr": {"time", 365.25 * day, false}, "day": {"time", day, false}, "d": {"time", day, false},
	"hr": {"time", hour, false}, "mn": {"time", 60, false}, "min": {"time", 60, false},
	"sec": {"time", 1, true}, "s": {"time", 1, true},

	"Pa": {"pressure", 1, true}, "p": {"pressure", 1, true}, "atm": {"pressure", 101325, true}, "at": {"pressure", 101325, true},
	"mmHg": {"pressure", 133.322, true}, "psi": {"pressure", 6894.757293168, false}, "Torr": {"pressure", 101325 / 760.0, false},

	"N": {"force", 1, true}, "dyn": {"force", 1e-5, true}, "dy": {"force", 1e-5, true},
	"lbf": {"force", 4.4482216152605, false}, "pond": {"force", 0.00980665, true},

	"J": {"energy", 1, true}, "e": {"energy", 1e-7, true}, "c": {"energy", 4.184, true}, "cal": {"energy", 4.1868, true},
	"eV": {"energy", 1.602176565e-19, true}, "ev": {"energy", 1.602176565e-19, true},
	"HPh": {"energy", 2684519.537696172, false}, "hh": {"energy", 2684519.537696172, false},
	"Wh": {"energy", 3600, true}, "wh": {"energy", 3600, true}, "flb": {"energy", 1.3558179483314, false},
	"BTU": {"energy", 1055.05585262, false}, "btu": {"energy", 1055.05585262, false},

	"W": {"power", 1, true}, "w": {"power", 1, true}, "HP": {"power", 745.69987158227, false}, "h": {"power", 745.69987158227, false},
	"PS": {"power", 735.49875, false},

	"T": {"magnetism", 1, true}, "ga": {"magnetism", 1e-4, true},

	"l": {"volume", 0.001, true}, "L": {"volume", 0.001, true}, "lt": {"volume", 0.001, true},
	"m3": {"volume", 1, true}, "tsp": {"volume", 4.92892159375e-6, false}, "tspm": {"volume", 5e-6, false},
	"tbs": {"volume", 1.478676478125e-5, false}, "oz": {"volume", 2.95735295625e-5, false},
	"cup": {"volume", 2.365882365e-4, false}, "pt": {"volume", 4.73176473e-4, false}, "us_pt": {"volume", 4.73176473e-4, false},
	"uk_pt": {"volume", 5.6826125e-4, false}, "qt": {"volume", 9.46352946e-4, false}, "gal": {"volume", 3.785411784e-3, false},
	"uk_qt": {"volume", 1.1365225e-3, false}, "uk_gal": {"volume", 4.54609e-3, false}, "ang3": {"volume", 1e-30, true},
	"barrel": {"volume", 0.158987294928, false}, "bushel": {"volume", 0.03523907016688, false},

	"m2": {"area", 1, true}, "ha": {"area", 1e4, true}, "ar": {"area", 100, true}, "in2": {"area", inch * inch, false},
	"ft2": {"area", 144 * inch * inch, false}, "yd2": {"area", 1296 * inch * inch, false},
	"mi2": {"area", 1609.344 * 1609.344, false}, "Nmi2": {"area", 1852 * 1852, false}, "morgen": {"area", 2500, false},
	"uk_acre": {"area", 4046.8564224, false}, "us_acre": {"area", 4046.87261, false},

	"m/s": {"speed", 1, true}, "m/sec": {"speed", 1, true}, "m/h": {"speed", 1 / hour, true}, "m/hr": {"speed", 1 / hour, true},
	"mph": {"speed", 1609.344 / hour, false}, "kn": {"speed", 1852 / hour, false}, "admkn": {"speed", 1853.184 / hour, false},
}

// prefixes of the metric system, and their powers of ten.
var prefixes = map[string]int{
	"Y": 24, "Z": 21, "E": 18, "P": 15, "T": 12, "G": 9, "M": 6, "k": 3, "h": 2, "da": 1, "e": 1,
	"d": -1, "c": -2, "m": -3, "u": -6, "n": -9, "p": -12, "f": -15, "a": -18, "z": -21, "y": -24,
}

// squares and cubes of the units: the prefix applies to the length.
var powers = map[string]int{"m2": 2, "m3": 3, "ang3": 3}

func findUnit(name string) (unit, float64, bool) {
	if u, ok := units[name]; ok {
		return u, 1, true
	}
	for p, exp := range prefixes {
		rest, found := strings.CutPrefix(name, p)
		if !found {
			continue
		}
		if u, ok := units[rest]; ok && u.prefix {
			return u, math.Pow(10, float64(exp*max(powers[rest], 1))), true
		}
	}
	return unit{}, 0, false
}

var temperatures = map[string]bool{"C": true, "cel": true, "F": true, "fah": true, "K": true, "kel": true, "Rank": true, "Reau": true}

// kelvin is a temperature in kelvins, and fromKelvin the reverse.
func kelvin(n float64, u string) float64 {
	switch u {
	case "C", "cel":
		return n + 273.15
	case "F", "fah":
		return (n + 459.67) * 5 / 9
	case "Rank":
		return n * 5 / 9
	case "Reau":
		return n*5/4 + 273.15
	}
	return n
}

func fromKelvin(k float64, u string) float64 {
	switch u {
	case "C", "cel":
		return k - 273.15
	case "F", "fah":
		return k*9/5 - 459.67
	case "Rank":
		return k * 9 / 5
	case "Reau":
		return (k - 273.15) * 4 / 5
	}
	return k
}

func convert(n float64, from, to string) (float64, bool) {
	if temperatures[from] || temperatures[to] {
		if !temperatures[from] || !temperatures[to] {
			return 0, false
		}
		return fromKelvin(kelvin(n, from), to), true
	}
	a, fa, ok := findUnit(from)
	if !ok {
		return 0, false
	}
	b, fb, ok := findUnit(to)
	if !ok || a.kind != b.kind {
		return 0, false
	}
	return n * a.factor * fa / (b.factor * fb), true
}
