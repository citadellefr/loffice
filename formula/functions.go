package formula

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// function is a function formulas may call.
type function struct {
	min, max int // arguments; max -1 for as many as Excel takes
	call     func(c *Context, args []Expr) Value
	// volatile functions give another value each time: the engine
	// calculates them, and what depends on them, after every change.
	volatile bool
}

var functions = map[string]*function{}

func add(fs map[string]*function) {
	for name, f := range fs {
		functions[name] = f
	}
}

// Volatile tells whether a function gives another value each time.
func Volatile(name string) bool {
	f := functions[FunctionName(name)]
	return f != nil && f.volatile
}

// Known tells whether the engine calculates a function.
func Known(name string) bool {
	return functions[FunctionName(name)] != nil
}

// fn is a function of fixed arguments, each calculated first.
func fn(min, max int, f func(c *Context, v []Value) Value) *function {
	return &function{min: min, max: max, call: func(c *Context, args []Expr) Value {
		v := make([]Value, len(args))
		for i, a := range args {
			v[i] = c.Eval(a)
		}
		return f(c, v)
	}}
}

// volatile marks a function to calculate again after every edit.
func volatile(f *function) *function {
	f.volatile = true
	return f
}

// math1 is a function of one number, applied to each value of an array.
func math1(f func(n float64) Value) *function {
	return fn(1, 1, func(c *Context, v []Value) Value {
		return c.mapValues(v[0], func(x Value) Value {
			n, err := c.number(x)
			if err != nil {
				return *err
			}
			return f(n)
		})
	})
}

// math2 is a function of two numbers, applied element by element.
func math2(min int, def float64, f func(a, b float64) Value) *function {
	return fn(min, 2, func(c *Context, v []Value) Value {
		b := Num(def)
		if len(v) > 1 && v[1].Type != TypeBlank {
			b = v[1]
		}
		return c.zip(v[0], b, func(x, y Value) Value {
			m, err := c.number(x)
			if err != nil {
				return *err
			}
			n, err := c.number(y)
			if err != nil {
				return *err
			}
			return f(m, n)
		})
	})
}

// each calls f with every value the arguments hold, telling whether it
// was given directly rather than by a range or an array. The blank cells
// of ranges are skipped, the blank arguments kept.
func (c *Context) each(args []Expr, f func(v Value, direct bool) bool) {
	for _, a := range args {
		if !c.eachOf(c.Eval(a), f) {
			return
		}
	}
}

func (c *Context) eachOf(v Value, f func(v Value, direct bool) bool) bool {
	switch v.Type {
	case TypeRange:
		ok := true
		for _, a := range v.Refs {
			c.Book.Cells(a, func(_, _ int, x Value) bool {
				ok = f(x, false)
				return ok
			})
			if !ok {
				return false
			}
		}
		return true
	case TypeArray:
		for _, x := range v.Arr.Cells {
			if !f(x, false) {
				return false
			}
		}
		return true
	}
	return f(v, true)
}

// numbers calls f with the numbers the arguments hold, as SUM takes them:
// the numbers of ranges and arrays, and the values given directly that
// read as numbers. The first error stops it, and is returned.
func (c *Context) numbers(args []Expr, f func(n float64)) *Value {
	var err *Value
	c.each(args, func(v Value, direct bool) bool {
		switch {
		case v.Type == TypeError:
			found := v
			err = &found
			return false
		case v.Type == TypeNumber:
			f(v.Num)
		case direct:
			n, e := c.number(v)
			if e != nil {
				err = e
				return false
			}
			f(n)
		}
		return true
	})
	return err
}

// numberList is the numbers the arguments hold, as numbers takes them.
func (c *Context) numberList(args []Expr) ([]float64, *Value) {
	var out []float64
	err := c.numbers(args, func(n float64) { out = append(out, n) })
	return out, err
}

// criterion is a condition of COUNTIF and its kin: "<5", "apple*",
// "<>", a number.
type criterion struct {
	op   string
	num  float64
	isN  bool
	text string
	re   *regexp.Regexp
	err  string
	bool int // -1 when the criterion is not a boolean
}

func (c *Context) criterion(v Value) criterion {
	v = c.scalar(v)
	cr := criterion{op: "=", bool: -1}
	switch v.Type {
	case TypeNumber:
		cr.num, cr.isN = v.Num, true
		return cr
	case TypeBool:
		cr.bool = int(v.Num)
		return cr
	case TypeError:
		cr.err = v.Str
		return cr
	case TypeBlank:
		cr.num, cr.isN = 0, true
		return cr
	}
	s := v.Str
	for _, op := range []string{"<=", ">=", "<>", "<", ">", "="} {
		if strings.HasPrefix(s, op) {
			cr.op, s = op, s[len(op):]
			break
		}
	}
	if n, ok := c.parseNumber(s); ok && s != "" {
		cr.num, cr.isN = n, true
		return cr
	}
	switch {
	case strings.EqualFold(s, "TRUE") || strings.EqualFold(s, c.Locale.True):
		cr.bool = 1
		return cr
	case strings.EqualFold(s, "FALSE") || strings.EqualFold(s, c.Locale.False):
		cr.bool = 0
		return cr
	case strings.HasPrefix(s, "#") && IsError(strings.ToUpper(s)):
		cr.err = strings.ToUpper(s)
		return cr
	}
	cr.text = s
	if (cr.op == "=" || cr.op == "<>") && strings.ContainsAny(s, "*?") {
		cr.re = wildcard(s)
	}
	return cr
}

// wildcard is the pattern of text with the wildcards of Excel: * any
// characters, ? one, ~ escaping them.
func wildcard(s string) *regexp.Regexp {
	return regexp.MustCompile("(?is)^" + wildcardPattern(s) + "$")
}

func wildcardPattern(s string) string {
	s = strings.ToValidUTF8(s, "\uFFFD")
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '~':
			if i+1 < len(s) {
				_, n := utf8.DecodeRuneInString(s[i+1:])
				b.WriteString(regexp.QuoteMeta(s[i+1 : i+1+n]))
				i += n
			} else {
				b.WriteString("~")
			}
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			j := i
			for j < len(s) && s[j] != '~' && s[j] != '*' && s[j] != '?' {
				j++
			}
			b.WriteString(regexp.QuoteMeta(s[i:j]))
			i = j - 1
		}
	}
	return b.String()
}

// match tells whether a value meets the criterion.
func (cr *criterion) match(v Value) bool {
	switch {
	case cr.err != "":
		is := v.Type == TypeError && v.Str == cr.err
		return is == (cr.op == "=")
	case cr.bool >= 0:
		if v.Type != TypeBool {
			return cr.op == "<>"
		}
		return cmpOp(cr.op, int(v.Num)-cr.bool)
	case cr.isN:
		if v.Type != TypeNumber {
			return cr.op == "<>"
		}
		return cmpOp(cr.op, compareFloat(v.Num, cr.num))
	}
	if cr.text == "" && (cr.op == "=" || cr.op == "<>") {
		empty := v.Type == TypeBlank || v.Type == TypeText && v.Str == ""
		return empty == (cr.op == "=")
	}
	if cr.re != nil {
		ok := v.Type == TypeText && cr.re.MatchString(v.Str)
		return ok == (cr.op == "=")
	}
	if v.Type != TypeText {
		return cr.op == "<>"
	}
	return cmpOp(cr.op, strings.Compare(foldCase(v.Str), foldCase(cr.text)))
}

func compareFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func cmpOp(op string, r int) bool {
	switch op {
	case "=":
		return r == 0
	case "<>":
		return r != 0
	case "<":
		return r < 0
	case ">":
		return r > 0
	case "<=":
		return r <= 0
	}
	return r >= 0
}
