package formula

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Type is the type of a value.
type Type uint8

const (
	// TypeBlank is an empty cell, which reads as 0 or "".
	TypeBlank Type = iota
	TypeNumber
	TypeText
	TypeBool
	TypeError
	// TypeArray is a table of values, from an area or a constant.
	TypeArray
	// TypeRange is a reference, kept as such for the functions that look
	// at cells rather than values: SUM skips the text of a range, ROW
	// reads where it is.
	TypeRange
	// TypeLambda is a function LAMBDA makes, which a cell cannot hold.
	TypeLambda
)

// Value is what a formula computes.
type Value struct {
	Type Type
	Num  float64 // a number, or a boolean as 0 or 1
	Str  string  // a text, or the code of an error
	Arr  *Table
	Refs []Area3
	fn   *lambda
}

// Table is an array of values, row by row. Cells may hold only its first
// rows, the rows past them repeating the last one held: so a range keeps
// the blank rows below the cells of its sheet, and an array calculated
// from it its shape, without the memory they would take.
type Table struct {
	Rows, Cols int
	Cells      []Value
}

// At is the value at row i and column j, counted from 0.
func (t *Table) At(i, j int) Value {
	return t.Cells[min(i, t.held()-1)*t.Cols+j]
}

// held is the number of rows Cells holds.
func (t *Table) held() int {
	return len(t.Cells) / t.Cols
}

// cut is a table of its first n rows, n no more than it has.
func (t *Table) cut(n int) *Table {
	out := &Table{Rows: n, Cols: t.Cols, Cells: make([]Value, n*t.Cols)}
	for i := range n {
		copy(out.Cells[i*t.Cols:], row(t, i))
	}
	return out
}

// blankTail tells whether the rows past those held are blank.
func (t *Table) blankTail() bool {
	if t.held() == t.Rows {
		return true
	}
	for _, x := range row(t, t.held()-1) {
		if x.Type != TypeBlank {
			return false
		}
	}
	return true
}

// Area3 is an area of a sheet, the sheet given by the engine's handle.
type Area3 struct {
	Sheet int
	Area
}

// Errors.
var (
	ErrNull  = Err("#NULL!")
	ErrDiv0  = Err("#DIV/0!")
	ErrValue = Err("#VALUE!")
	ErrRef   = Err("#REF!")
	ErrName  = Err("#NAME?")
	ErrNum   = Err("#NUM!")
	ErrNA    = Err("#N/A")
	ErrCalc  = Err("#CALC!")
	ErrSpill = Err("#SPILL!")
)

func Num(n float64) Value {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return ErrNum
	}
	return Value{Type: TypeNumber, Num: n}
}

func Str(s string) Value {
	return Value{Type: TypeText, Str: s}
}

func Boolean(b bool) Value {
	if b {
		return Value{Type: TypeBool, Num: 1}
	}
	return Value{Type: TypeBool}
}

func Err(code string) Value {
	return Value{Type: TypeError, Str: code}
}

func (v Value) IsError() bool {
	return v.Type == TypeError
}

// String is the value as a cell shows it without a format.
func (v Value) String() string {
	switch v.Type {
	case TypeNumber:
		return formatGeneral(v.Num)
	case TypeText, TypeError:
		return v.Str
	case TypeBool:
		if v.Num != 0 {
			return "TRUE"
		}
		return "FALSE"
	}
	return ""
}

// formatGeneral writes a number as Excel turns it into text: 15
// significant digits, in exponent notation past them or below 1E-9.
func formatGeneral(n float64) string {
	if n == math.Trunc(n) && math.Abs(n) < 1e15 {
		return strconv.FormatInt(int64(n), 10)
	}
	mant, exp, _ := strings.Cut(strconv.FormatFloat(n, 'e', 14, 64), "e")
	e, _ := strconv.Atoi(exp)
	if e >= 15 || e < -9 {
		mant = strings.TrimRight(strings.TrimRight(mant, "0"), ".")
		sign := "+"
		if e < 0 {
			sign, e = "-", -e
		}
		return mant + "E" + sign + fmt.Sprintf("%02d", e)
	}
	rounded, _ := strconv.ParseFloat(mant+"e"+exp, 64)
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// number reads text as a number the way Excel converts it: "1,5" in a
// French workbook, " 12 ", "1e3", "50%", "(3)".
func number(s string, l *Locale) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	neg := false
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		neg, s = true, s[1:len(s)-1]
	}
	percent := false
	if strings.HasSuffix(s, "%") {
		percent, s = true, strings.TrimSpace(s[:len(s)-1])
	}
	for _, g := range l.Groups {
		s = strings.ReplaceAll(s, g, "")
	}
	if l.Decimal != "." {
		if strings.Contains(s, ".") {
			return 0, false
		}
		s = strings.Replace(s, l.Decimal, ".", 1)
	}
	if s == "" || strings.ContainsAny(s, "xXpP_") || strings.EqualFold(s, "inf") || strings.EqualFold(s, "infinity") || strings.EqualFold(s, "nan") {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsInf(n, 0) {
		return 0, false
	}
	if percent {
		n /= 100
	}
	if neg {
		n = -n
	}
	return n, true
}
