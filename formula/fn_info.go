package formula

import (
	"math"
	"strings"
)

// A Book may know more than formulas ask of every one: the formula of a
// cell, the file name of a sheet, how many sheets the workbook has.
type (
	formulaTexts interface {
		FormulaText(sheet, row, col int) (string, bool)
	}
	fileNames interface {
		FileName(sheet int) string
	}
	sheetCounter interface {
		SheetCount() int
	}
)

func init() {
	add(map[string]*function{
		"SHEET": {min: 0, max: 1, call: func(c *Context, args []Expr) Value {
			if len(args) == 0 {
				return Num(float64(c.Sheet + 1))
			}
			v := c.Eval(args[0])
			if v.Type == TypeRange {
				return Num(float64(v.Refs[0].Sheet + 1))
			}
			name, err := c.text(v)
			if err != nil {
				return *err
			}
			sheets, ok := c.Book.Sheets(name, "")
			if !ok {
				return ErrNA
			}
			return Num(float64(sheets[0] + 1))
		}},
		"SHEETS": {min: 0, max: 1, structural: true, call: func(c *Context, args []Expr) Value {
			if len(args) == 0 {
				b, ok := c.Book.(sheetCounter)
				if !ok {
					return ErrNA
				}
				return Num(float64(b.SheetCount()))
			}
			v := c.Eval(args[0])
			if v.Type != TypeRange {
				return ErrValue
			}
			return Num(float64(len(v.Refs)))
		}},
		"AREAS": {min: 1, max: 1, call: func(c *Context, args []Expr) Value {
			v := c.Eval(args[0])
			if v.Type != TypeRange {
				return ErrValue
			}
			return Num(float64(len(v.Refs)))
		}},
		"FORMULATEXT": {min: 1, max: 1, call: func(c *Context, args []Expr) Value {
			v := c.Eval(args[0])
			if v.Type != TypeRange {
				return ErrValue
			}
			b, ok := c.Book.(formulaTexts)
			if !ok {
				return ErrNA
			}
			a := v.Refs[0]
			text, ok := b.FormulaText(a.Sheet, a.R1, a.C1)
			if !ok {
				return ErrNA
			}
			return Str(text)
		}},
		"HYPERLINK": fn(1, 2, func(c *Context, v []Value) Value {
			if len(v) > 1 {
				return c.scalar(v[1])
			}
			return c.scalar(v[0])
		}),
		"CELL": {min: 1, max: 2, structural: true, call: func(c *Context, args []Expr) Value {
			kind, err := c.text(c.Eval(args[0]))
			if err != nil {
				return *err
			}
			at := Area3{c.Sheet, Area{R1: c.Row, C1: c.Col, R2: c.Row, C2: c.Col}}
			if len(args) > 1 {
				v := c.Eval(args[1])
				if v.Type != TypeRange {
					return ErrValue
				}
				at = v.Refs[0]
			}
			return c.cell(strings.ToLower(kind), at)
		}},
		"INFO": structural(fn(1, 1, func(c *Context, v []Value) Value {
			kind, err := c.text(v[0])
			if err != nil {
				return *err
			}
			return c.info(strings.ToLower(kind))
		})),
		"VALUETOTEXT": fn(1, 2, func(c *Context, v []Value) Value {
			strict, err := c.textFormat(v)
			if err != nil {
				return *err
			}
			s, err := c.display(c.scalar(v[0]), strict)
			if err != nil {
				return *err
			}
			return Str(s)
		}),
		"ARRAYTOTEXT": fn(1, 2, func(c *Context, v []Value) Value {
			strict, err := c.textFormat(v)
			if err != nil {
				return *err
			}
			t := c.array(v[0])
			var b strings.Builder
			if strict {
				b.WriteByte('{')
			}
			for i := range t.Rows {
				for j := range t.Cols {
					switch {
					case j > 0 && strict:
						b.WriteByte(',')
					case j > 0 || i > 0 && !strict:
						b.WriteString(", ")
					case i > 0:
						b.WriteByte(';')
					}
					x, _ := c.display(t.At(i, j), strict)
					b.WriteString(x)
				}
			}
			if strict {
				b.WriteByte('}')
			}
			return Str(b.String())
		}),
		"LENB":   byteFunction("LEN", 1, 1),
		"LEFTB":  byteFunction("LEFT", 1, 2),
		"RIGHTB": byteFunction("RIGHT", 1, 2),
		"MIDB":   byteFunction("MID", 3, 3),
		"ASC": text1(func(s string) string {
			return strings.Map(func(r rune) rune {
				switch {
				case r >= 0xff01 && r <= 0xff5e:
					return r - 0xfee0
				case r == 0x3000:
					return ' '
				}
				return r
			}, s)
		}),
		"SUMX2MY2": sumOfPairs(func(x, y float64) float64 { return x*x - y*y }),
		"SUMX2PY2": sumOfPairs(func(x, y float64) float64 { return x*x + y*y }),
		"SUMXMY2":  sumOfPairs(func(x, y float64) float64 { return (x - y) * (x - y) }),
		"SERIESSUM": fn(4, 4, func(c *Context, v []Value) Value {
			var n [3]float64
			for i := range n {
				x, err := c.number(v[i])
				if err != nil {
					return *err
				}
				n[i] = x
			}
			coeffs, err := c.numbersIn(v[3])
			if err != nil {
				return *err
			}
			sum := 0.0
			for i, a := range coeffs {
				sum += a * math.Pow(n[0], n[1]+float64(i)*n[2])
			}
			return finite(sum)
		}),
		"MULTINOMIAL": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			var total, log float64
			bad := false
			err := c.numbers(args, func(n float64) {
				n = math.Trunc(n)
				bad = bad || n < 0
				total += n
				log -= logGamma(n + 1)
			})
			if err != nil {
				return *err
			}
			if bad {
				return ErrNum
			}
			return finite(math.Round(math.Exp(log + logGamma(total+1))))
		}},
	})
}

func finite(n float64) Value {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return ErrNum
	}
	return Num(n)
}

// byteFunction is a function counting bytes, which in the languages Excel
// counts characters for are those of the function counting characters.
func byteFunction(name string, min, max int) *function {
	return &function{min: min, max: max, call: func(c *Context, args []Expr) Value {
		return functions[name].call(c, args)
	}}
}

// sumOfPairs sums f over the pairs of numbers two arrays hold, leaving
// out those whose number is missing in one.
func sumOfPairs(f func(x, y float64) float64) *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		x, y := c.pair(v[0], v[1])
		if x.Rows != y.Rows || x.Cols != y.Cols {
			return ErrNA
		}
		sum := 0.0
		for i, a := range x.Cells {
			b := y.Cells[i]
			if a.Type == TypeError {
				return a
			}
			if b.Type == TypeError {
				return b
			}
			if a.Type == TypeNumber && b.Type == TypeNumber {
				sum += f(a.Num, b.Num)
			}
		}
		return finite(sum)
	})
}

// textFormat tells whether the format argument of VALUETOTEXT is strict.
func (c *Context) textFormat(v []Value) (bool, *Value) {
	if len(v) < 2 {
		return false, nil
	}
	n, err := c.number(v[1])
	if err != nil {
		return false, err
	}
	if n != 0 && n != 1 {
		return false, &ErrValue
	}
	return n == 1, nil
}

// display writes a value as VALUETOTEXT does: the text quoted when strict.
func (c *Context) display(v Value, strict bool) (string, *Value) {
	if v.Type == TypeText && strict {
		return `"` + strings.ReplaceAll(v.Str, `"`, `""`) + `"`, nil
	}
	if v.Type == TypeError {
		return v.Str, nil
	}
	return c.text(v)
}

// cellReadsValue tells whether CELL reads the value of its cell for the
// information asked, not only its place.
func cellReadsValue(kind string) bool {
	switch kind {
	case "contents", "contenu", "type":
		return true
	}
	return false
}

func (c *Context) cell(kind string, at Area3) Value {
	switch kind {
	case "address", "adresse":
		return Str(colName(at.C1, true) + rowName(at.R1, true))
	case "col", "colonne":
		return Num(float64(at.C1))
	case "row", "ligne":
		return Num(float64(at.R1))
	case "contents", "contenu":
		return c.Book.Cell(at.Sheet, at.R1, at.C1)
	case "type":
		switch v := c.Book.Cell(at.Sheet, at.R1, at.C1); v.Type {
		case TypeBlank:
			return Str("b")
		case TypeText:
			return Str("l")
		}
		return Str("v")
	case "filename", "nomfichier":
		if b, ok := c.Book.(fileNames); ok {
			return Str(b.FileName(at.Sheet))
		}
		return Str("")
	}
	return ErrValue
}

func (c *Context) info(kind string) Value {
	french := c.Locale.True == French.True
	switch kind {
	case "recalc", "recalcul":
		if french {
			return Str("Automatique")
		}
		return Str("Automatic")
	case "system", "système", "systeme":
		return Str("pcdos")
	case "osversion", "versionsyst":
		return Str("Windows (32-bit) NT 10.00")
	case "release", "version":
		return Str("16.0")
	case "origin", "origine":
		return Str("$A:$A1")
	case "numfile", "nbfich":
		if b, ok := c.Book.(sheetCounter); ok {
			return Num(float64(b.SheetCount()))
		}
		return ErrNA
	}
	return ErrValue
}
