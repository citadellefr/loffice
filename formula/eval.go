package formula

import (
	"math"
	"strings"
	"time"
)

// Book is the workbook formulas read. Sheets are known by handles the
// Book gives them.
type Book interface {
	// Cell is the value of a cell, its formula calculated first when it
	// must be.
	Cell(sheet, row, col int) Value
	// Cells calls f with the cells of an area that are not blank, row by
	// row, until f returns false.
	Cells(a Area3, f func(row, col int, v Value) bool)
	// Size is the last row and column of a sheet holding a cell.
	Size(sheet int) (rows, cols int)
	// Sheets are the handles of the sheets from first to last, in the
	// order of the workbook; first alone when last is "".
	Sheets(first, last string) ([]int, bool)
	// Name is the formula of a defined name, local to sheet or global.
	Name(name string, sheet int) (Expr, bool)
}

// Context is where a formula is calculated.
type Context struct {
	Book Book
	// Sheet, Row and Col are the cell of the formula.
	Sheet, Row, Col int
	Locale          *Locale
	Now             time.Time
	Rand            func() float64
	Date1904        bool
	// MoveRows and MoveCols move the relative references of a formula
	// written for another cell, as those of conditional formats are for
	// the first cell they format.
	MoveRows, MoveCols int
	depth              int
	// vars are the names LET and LAMBDA give values, and calls the LAMBDA
	// functions being calculated within one another.
	vars  *variable
	calls int
}

// ErrUnsupported is what a formula gives when it calls a function the
// engine does not know: the cell keeps the value Excel last saved.
var ErrUnsupported = Err("#UNSUPPORTED")

// maxDepth bounds names defined by names.
const maxDepth = 64

// Eval calculates a formula in its cell. The result may be an array, or a
// reference when the formula is one.
func (c *Context) Eval(e Expr) Value {
	switch e := e.(type) {
	case numberExpr:
		return Num(float64(e))
	case textExpr:
		return Str(string(e))
	case boolExpr:
		return Boolean(bool(e))
	case errorExpr:
		return Err(string(e))
	case missingExpr:
		return Value{}
	case valueExpr:
		return e.v
	case refExpr:
		return c.ref(e.ref)
	case boundExpr:
		if e.refs == nil {
			return ErrRef
		}
		return Value{Type: TypeRange, Refs: e.refs}
	case nameExpr:
		return c.name(e)
	case structExpr:
		return c.structured(e.s)
	case unaryExpr:
		x := c.Eval(e.x)
		switch e.op {
		case '@':
			return c.scalar(x)
		case '+':
			return x
		}
		return c.mapValues(x, func(v Value) Value {
			n, err := c.number(v)
			if err != nil {
				return *err
			}
			return Num(-n)
		})
	case percentExpr:
		return c.mapValues(c.Eval(e.x), func(v Value) Value {
			n, err := c.number(v)
			if err != nil {
				return *err
			}
			return Num(n / 100)
		})
	case spillExpr:
		return c.spill(c.Eval(e.x))
	case binaryExpr:
		return c.binary(e)
	case callExpr:
		f := functions[e.name]
		if v := c.vars.find(e.name); v != nil && (f == nil || v.value.Type == TypeLambda) {
			return c.invoke(v.value, e.args)
		}
		if f == nil {
			return c.named(e)
		}
		if len(e.args) < f.min || f.max >= 0 && len(e.args) > f.max {
			return ErrValue
		}
		return f.call(c, e.args)
	case invokeExpr:
		return c.invoke(c.Eval(e.fn), e.args)
	case arrayExpr:
		t := &Table{Rows: e.rows, Cols: e.cols, Cells: make([]Value, len(e.cells))}
		for i, x := range e.cells {
			t.Cells[i] = c.Eval(x)
		}
		return Value{Type: TypeArray, Arr: t}
	}
	return ErrValue
}

// Result is the single value a formula gives in its cell: that of the
// cell a reference points to, the first of an array.
func (c *Context) Result(e Expr) Value {
	return c.scalar(c.Eval(e))
}

func (c *Context) ref(r *Ref) Value {
	if r.Invalid || r.Book != "" {
		return ErrRef
	}
	sheets := []int{c.Sheet}
	if r.Sheet != "" {
		var ok bool
		if sheets, ok = c.Book.Sheets(r.Sheet, r.LastSheet); !ok {
			return ErrRef
		}
	}
	area := r.Area
	if c.MoveRows != 0 || c.MoveCols != 0 {
		var ok bool
		if area, ok = area.translate(c.MoveRows, c.MoveCols); !ok {
			return ErrRef
		}
	}
	v := Value{Type: TypeRange}
	for _, s := range sheets {
		v.Refs = append(v.Refs, Area3{s, area})
	}
	return v
}

// spill is the area the dynamic formula of a cell spills over, A1#.
func (c *Context) spill(v Value) Value {
	if v.Type == TypeError {
		return v
	}
	if v.Type != TypeRange || len(v.Refs) != 1 || v.Refs[0].R1 != v.Refs[0].R2 || v.Refs[0].C1 != v.Refs[0].C2 {
		return ErrRef
	}
	b, ok := c.Book.(interface {
		Spill(sheet, row, col int) (Value, bool)
	})
	if !ok {
		return ErrRef
	}
	a := v.Refs[0]
	if s, ok := b.Spill(a.Sheet, a.R1, a.C1); ok {
		return s
	}
	return ErrRef
}

// structured is the area a structured reference stands for in the cell of
// the formula, when the book has tables.
func (c *Context) structured(s Structured) Value {
	tb, ok := c.Book.(Tables)
	if !ok {
		return ErrUnsupported
	}
	a, err := resolve(tb, s, Pos{c.Sheet, c.Row, c.Col})
	if err != nil {
		return *err
	}
	return Value{Type: TypeRange, Refs: []Area3{a}}
}

func (c *Context) name(e nameExpr) Value {
	if s, ok := ParseStructured(e.name); ok && e.sheet == "" {
		return c.structured(s)
	}
	if e.sheet == "" {
		if v := c.vars.find(e.name); v != nil {
			return v.value
		}
	}
	sheet := c.Sheet
	if e.sheet != "" {
		s, ok := c.Book.Sheets(e.sheet, "")
		if !ok {
			return ErrRef
		}
		sheet = s[0]
	}
	x, ok := c.Book.Name(e.name, sheet)
	if !ok {
		if strings.ContainsRune(e.name, '[') {
			return ErrUnsupported
		}
		if tb, ok := c.Book.(Tables); ok {
			if _, ok := tb.Table(e.name); ok {
				return c.structured(Structured{Table: e.name})
			}
		}
		return ErrName
	}
	return c.defined(x)
}

// defined calculates the formula of a defined name, out of reach of the
// names LET and LAMBDA bind where it is used.
func (c *Context) defined(x Expr) Value {
	if c.depth >= maxDepth {
		return ErrRef
	}
	c.depth++
	vars := c.vars
	c.vars = nil
	defer func() { c.depth--; c.vars = vars }()
	return c.Eval(x)
}

// deref turns a reference into the values it points to: the value of a
// cell, or an array.
func (c *Context) deref(v Value) Value {
	if v.Type != TypeRange {
		return v
	}
	if len(v.Refs) != 1 {
		return ErrValue
	}
	a := v.Refs[0]
	if a.R1 == a.R2 && a.C1 == a.C2 {
		return c.Book.Cell(a.Sheet, a.R1, a.C1)
	}
	return Value{Type: TypeArray, Arr: c.table(a)}
}

// trim is an area of whole rows or columns bounded by the cells its sheet
// holds.
func (c *Context) trim(a Area3) Area3 {
	rows, cols := c.Book.Size(a.Sheet)
	if a.Rows || a.Cols {
		a.R2, a.C2 = min(a.R2, max(rows, a.R1)), min(a.C2, max(cols, a.C1))
	}
	return a
}

// table is the values of an area, bounded by the cells its sheet holds.
func (c *Context) table(a Area3) *Table {
	a = c.trim(a)
	rows, _ := c.Book.Size(a.Sheet)
	t := &Table{Rows: a.R2 - a.R1 + 1, Cols: a.C2 - a.C1 + 1}
	held := min(t.Rows, max(rows-a.R1+2, 1))
	a.R2 = a.R1 + held - 1
	if held*t.Cols > maxCells && !c.crowded(a) {
		return &Table{Rows: 1, Cols: 1, Cells: []Value{ErrNum}}
	}
	t.Cells = make([]Value, held*t.Cols)
	c.Book.Cells(a, func(row, col int, v Value) bool {
		t.Cells[(row-a.R1)*t.Cols+col-a.C1] = v
		return true
	})
	return t
}

// crowded tells whether at least half the cells of an area hold a value:
// its table then takes memory in proportion to the book.
func (c *Context) crowded(a Area3) bool {
	want, n := (a.R2-a.R1+1)*(a.C2-a.C1+1)/2, 0
	c.Book.Cells(a, func(_, _ int, _ Value) bool {
		n++
		return n < want
	})
	return n >= want
}

// maxCells bounds the cells of an array made from a size rather than from
// cells a book holds, against memory exhaustion: a size given to a
// function, a sheet whose few cells are far apart.
const maxCells = 1 << 20

// maxRepeat bounds the cells an array spends on rows that only repeat the
// last one held.
const maxRepeat = 1 << 16

// scalar is the single value a formula takes where it expects one: a
// range crossed with the formula's row or column, the first value of an
// array.
func (c *Context) scalar(v Value) Value {
	switch v.Type {
	case TypeRange:
		if len(v.Refs) != 1 {
			return ErrValue
		}
		a := v.Refs[0]
		switch {
		case a.R1 == a.R2 && a.C1 == a.C2:
			return c.Book.Cell(a.Sheet, a.R1, a.C1)
		case a.C1 == a.C2 && a.R1 <= c.Row && c.Row <= a.R2:
			return c.Book.Cell(a.Sheet, c.Row, a.C1)
		case a.R1 == a.R2 && a.C1 <= c.Col && c.Col <= a.C2:
			return c.Book.Cell(a.Sheet, a.R1, c.Col)
		}
		return ErrValue
	case TypeArray:
		if len(v.Arr.Cells) == 0 {
			return ErrValue
		}
		return v.Arr.Cells[0]
	case TypeLambda:
		return ErrCalc
	}
	return v
}

// number is a value as a number, or the error it gives.
func (c *Context) number(v Value) (float64, *Value) {
	v = c.scalar(v)
	switch v.Type {
	case TypeNumber, TypeBool:
		return v.Num, nil
	case TypeBlank:
		return 0, nil
	case TypeText:
		if n, ok := c.parseNumber(v.Str); ok {
			return n, nil
		}
		return 0, &ErrValue
	}
	err := v
	return 0, &err
}

// parseNumber reads text as Excel does where it expects a number: a
// number written in the locale, a date, a time.
func (c *Context) parseNumber(s string) (float64, bool) {
	if n, ok := number(s, c.Locale); ok {
		return n, true
	}
	return c.parseDate(s)
}

// text is a value as text.
func (c *Context) text(v Value) (string, *Value) {
	v = c.scalar(v)
	switch v.Type {
	case TypeNumber:
		return c.formatNumber(v.Num), nil
	case TypeText:
		return v.Str, nil
	case TypeBool:
		if v.Num != 0 {
			return c.Locale.True, nil
		}
		return c.Locale.False, nil
	case TypeBlank:
		return "", nil
	}
	err := v
	return "", &err
}

// formatNumber writes a number as General does, in the locale.
func (c *Context) formatNumber(n float64) string {
	s := formatGeneral(n)
	if c.Locale.Decimal != "." {
		s = strings.Replace(s, ".", c.Locale.Decimal, 1)
	}
	return s
}

// boolean is a value as a boolean.
func (c *Context) boolean(v Value) (bool, *Value) {
	v = c.scalar(v)
	switch v.Type {
	case TypeNumber, TypeBool:
		return v.Num != 0, nil
	case TypeBlank:
		return false, nil
	case TypeText:
		switch {
		case strings.EqualFold(v.Str, "TRUE") || strings.EqualFold(v.Str, c.Locale.True):
			return true, nil
		case strings.EqualFold(v.Str, "FALSE") || strings.EqualFold(v.Str, c.Locale.False):
			return false, nil
		}
		return false, &ErrValue
	}
	err := v
	return false, &err
}

// array is a value as a table that holds all its rows: a range's values,
// a single value alone. Past maxRepeat cells, the rows that repeat are left
// out, as past the cells of a sheet.
func (c *Context) array(v Value) *Table {
	t := c.grid(v)
	if len(t.Cells) == t.Rows*t.Cols {
		return t
	}
	if t.Rows*t.Cols > max(maxRepeat, 2*len(t.Cells)) {
		n := max(t.held()-1, 1)
		return &Table{Rows: n, Cols: t.Cols, Cells: t.Cells[:n*t.Cols]}
	}
	out := &Table{Rows: t.Rows, Cols: t.Cols, Cells: make([]Value, t.Rows*t.Cols)}
	n := copy(out.Cells, t.Cells)
	for i := n; i < len(out.Cells); i += t.Cols {
		copy(out.Cells[i:], t.Cells[n-t.Cols:])
	}
	return out
}

// pair is two values read cell by cell, leaving out the pairs of blanks
// past the rows held when they have the same shape.
func (c *Context) pair(x, y Value) (*Table, *Table) {
	a, b := c.grid(x), c.grid(y)
	if a.Rows != b.Rows || a.Cols != b.Cols || !a.blankTail() || !b.blankTail() {
		return c.array(x), c.array(y)
	}
	n := max(a.held(), b.held())
	return a.cut(n), b.cut(n)
}

// grid is a value as a table, which may hold only its first rows.
func (c *Context) grid(v Value) *Table {
	switch v.Type {
	case TypeArray:
		return v.Arr
	case TypeRange:
		if len(v.Refs) != 1 {
			return &Table{Rows: 1, Cols: 1, Cells: []Value{ErrValue}}
		}
		return c.table(v.Refs[0])
	}
	return &Table{Rows: 1, Cols: 1, Cells: []Value{v}}
}

// mapValues applies f to a value, or to each value of an array.
func (c *Context) mapValues(v Value, f func(Value) Value) Value {
	if v.Type != TypeArray && v.Type != TypeRange {
		return f(v)
	}
	if v.Type == TypeRange && len(v.Refs) == 1 && v.Refs[0].R1 == v.Refs[0].R2 && v.Refs[0].C1 == v.Refs[0].C2 {
		return f(c.deref(v))
	}
	t := c.grid(v)
	out := &Table{Rows: t.Rows, Cols: t.Cols, Cells: make([]Value, len(t.Cells))}
	for i, x := range t.Cells {
		out.Cells[i] = f(x)
	}
	return Value{Type: TypeArray, Arr: out}
}

// zip applies f to two values, element by element when either is an
// array: a single row or column is repeated along the other's, cells past
// the smaller are #N/A.
func (c *Context) zip(x, y Value, f func(a, b Value) Value) Value {
	single := func(v Value) bool {
		return v.Type != TypeArray && (v.Type != TypeRange || len(v.Refs) == 1 && v.Refs[0].R1 == v.Refs[0].R2 && v.Refs[0].C1 == v.Refs[0].C2)
	}
	if single(x) && single(y) {
		return f(c.deref(x), c.deref(y))
	}
	a, b := c.grid(x), c.grid(y)
	rows, cols := max(a.Rows, b.Rows), max(a.Cols, b.Cols)
	held := min(rows, max(changing(a, rows), changing(b, rows)))
	if held*cols > max(maxCells, len(a.Cells)+len(b.Cells)) {
		return ErrNum
	}
	out := &Table{Rows: rows, Cols: cols, Cells: make([]Value, held*cols)}
	for i := range held {
		for j := range cols {
			out.Cells[i*cols+j] = f(stretched(a, i, j), stretched(b, i, j))
		}
	}
	return Value{Type: TypeArray, Arr: out}
}

// changing is the number of first rows of a table stretched to rows past
// which its rows are all alike.
func changing(t *Table, rows int) int {
	switch {
	case t.Rows == 1:
		return 1
	case t.Rows < rows:
		return t.Rows + 1
	}
	return t.held()
}

// stretched is the value at row i and column j of a table stretched to a
// larger one: a single row or column repeated, #N/A past the others.
func stretched(t *Table, i, j int) Value {
	if t.Rows == 1 {
		i = 0
	}
	if t.Cols == 1 {
		j = 0
	}
	if i >= t.Rows || j >= t.Cols {
		return ErrNA
	}
	return t.At(i, j)
}

func (c *Context) binary(e binaryExpr) Value {
	switch e.op {
	case ":", ",", " ":
		return c.refOp(e)
	}
	x, y := c.Eval(e.x), c.Eval(e.y)
	switch e.op {
	case "&":
		return c.zip(x, y, func(a, b Value) Value {
			s, err := c.text(a)
			if err != nil {
				return *err
			}
			t, err := c.text(b)
			if err != nil {
				return *err
			}
			return Str(s + t)
		})
	case "=", "<>", "<", ">", "<=", ">=":
		return c.zip(x, y, func(a, b Value) Value {
			if a.Type == TypeError {
				return a
			}
			if b.Type == TypeError {
				return b
			}
			r := compare(a, b)
			switch e.op {
			case "=":
				return Boolean(r == 0)
			case "<>":
				return Boolean(r != 0)
			case "<":
				return Boolean(r < 0)
			case ">":
				return Boolean(r > 0)
			case "<=":
				return Boolean(r <= 0)
			}
			return Boolean(r >= 0)
		})
	}
	return c.zip(x, y, func(a, b Value) Value {
		m, err := c.number(a)
		if err != nil {
			return *err
		}
		n, err := c.number(b)
		if err != nil {
			return *err
		}
		switch e.op {
		case "+":
			return Num(m + n)
		case "-":
			return Num(m - n)
		case "*":
			return Num(m * n)
		case "/":
			if n == 0 {
				return ErrDiv0
			}
			return Num(m / n)
		case "^":
			return power(m, n)
		}
		return ErrValue
	})
}

func power(m, n float64) Value {
	if m == 0 && n == 0 {
		return ErrNum
	}
	if m == 0 && n < 0 {
		return ErrDiv0
	}
	if m < 0 && n != math.Trunc(n) {
		return ErrNum
	}
	return Num(math.Pow(m, n))
}

// compare orders two values as Excel does: numbers, then text without
// regard to case, then booleans; a blank is 0, "" or FALSE, whichever the
// other value is.
func compare(a, b Value) int {
	if a.Type == TypeBlank {
		a = blankLike(b)
	}
	if b.Type == TypeBlank {
		b = blankLike(a)
	}
	rank := func(v Value) int {
		switch v.Type {
		case TypeText:
			return 1
		case TypeBool:
			return 2
		}
		return 0
	}
	if ra, rb := rank(a), rank(b); ra != rb {
		return ra - rb
	}
	if a.Type == TypeText {
		return strings.Compare(foldCase(a.Str), foldCase(b.Str))
	}
	switch {
	case a.Num < b.Num:
		return -1
	case a.Num > b.Num:
		return 1
	}
	return 0
}

func blankLike(v Value) Value {
	switch v.Type {
	case TypeText:
		return Str("")
	case TypeBool:
		return Boolean(false)
	}
	return Num(0)
}

func foldCase(s string) string {
	return strings.ToLower(s)
}

// refOp applies the reference operators: ":" the area two references
// span, "," both, " " the cells they share.
func (c *Context) refOp(e binaryExpr) Value {
	x, y := c.Eval(e.x), c.Eval(e.y)
	if x.Type == TypeError {
		return x
	}
	if y.Type == TypeError {
		return y
	}
	if x.Type != TypeRange || y.Type != TypeRange {
		return ErrValue
	}
	switch e.op {
	case ",":
		return Value{Type: TypeRange, Refs: append(append([]Area3{}, x.Refs...), y.Refs...)}
	case ":":
		if len(x.Refs) != 1 || len(y.Refs) != 1 || x.Refs[0].Sheet != y.Refs[0].Sheet {
			return ErrValue
		}
		a, b := x.Refs[0], y.Refs[0]
		return Value{Type: TypeRange, Refs: []Area3{{a.Sheet, Area{
			R1: min(a.R1, b.R1), C1: min(a.C1, b.C1), R2: max(a.R2, b.R2), C2: max(a.C2, b.C2),
		}}}}
	}
	var out []Area3
	for _, a := range x.Refs {
		for _, b := range y.Refs {
			if a.Sheet != b.Sheet {
				continue
			}
			i := Area3{a.Sheet, Area{R1: max(a.R1, b.R1), C1: max(a.C1, b.C1), R2: min(a.R2, b.R2), C2: min(a.C2, b.C2)}}
			if i.R1 <= i.R2 && i.C1 <= i.C2 {
				out = append(out, i)
			}
		}
	}
	if len(out) == 0 {
		return ErrNull
	}
	return Value{Type: TypeRange, Refs: out}
}
