package formula

import (
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Pos is a cell of a workbook, its sheet given by the engine's handle.
type Pos struct{ Sheet, Row, Col int }

// Source is the workbook an engine calculates: the values its cells hold,
// the last calculated of those with a formula.
type Source interface {
	Value(p Pos) Value
	// Each calls f with the cells of an area that hold a value or a
	// formula, row by row, until f returns false.
	Each(a Area3, f func(row, col int, v Value) bool)
	Size(sheet int) (rows, cols int)
	Sheets(first, last string) ([]int, bool)
	// Name is the formula of a defined name, local to sheet or global.
	Name(name string, sheet int) (string, bool)
}

// Rows is a source or a book that knows the rows it hides, which SUBTOTAL
// and AGGREGATE may leave out.
type Rows interface {
	// Hidden tells whether a row is hidden, and whether by a filter.
	Hidden(sheet, row int) (hidden, filtered bool)
}

// Options tunes an engine.
type Options struct {
	Locale   *Locale
	Date1904 bool
	// Now is the time of NOW and TODAY, in the workbook's time zone.
	Now  func() time.Time
	Rand func() float64
}

// Engine keeps the formulas of a workbook and calculates again those a
// change reaches, in the order they depend on each other.
type Engine struct {
	src      Source
	opt      Options
	formulas map[Pos]*cellFormula
	// readers are the formulas reading a cell by itself; areas those
	// reading a range, by column, or in wide when it spans many.
	readers  map[Pos][]*cellFormula
	areas    map[[2]int][]areaReader
	wide     map[int][]areaReader
	volatile map[Pos]bool
	// subtotals are the formulas calling SUBTOTAL or AGGREGATE, which
	// read whether rows are hidden.
	subtotals map[Pos]bool
	// arrays gives each cell of an array formula its anchor, and each cell
	// a dynamic one spilled over.
	arrays map[Pos]Pos
	// spills are the dynamic array formulas, by anchor.
	spills map[Pos]*cellFormula
	// orphans are the cells the dynamic formulas taken away spilled over,
	// which hold their values until the next calculation clears them.
	orphans map[Pos]bool

	// what a calculation knows, besides what its formulas hold
	dirty []*cellFormula
	round int
	// grown are the cells the dynamic formulas calculated spill over anew,
	// whose readers are calculated in another round.
	grown []Pos
	// filled are the values of the cells of the array formulas calculated.
	filled map[Pos]Value
	names  map[string]Expr
	// bounds are, by sheet, the area of the formulas calculated, which
	// ranges outside read the source alone.
	bounds map[int]Area
	// ranges are the cells of the areas outside bounds read more than
	// once, kept up to cachedCells in all: many formulas read the same.
	ranges map[Area3][]cachedCell
	cached int
}

type cachedCell struct {
	row, col int32
	typ      Type
	num      float64
	str      string
}

// cachedCells bounds what a calculation keeps of the areas it reads.
const cachedCells = 1 << 18

type cellFormula struct {
	pos   Pos
	expr  Expr
	array *Area // the cells an array formula fills, from its anchor
	reads []Area3
	// dynamic formulas spill their result over array; reach is where it
	// went or would go, which the cells changed there calculate again.
	dynamic bool
	reach   Area
	// fresh is set on a dynamic formula set without its spill, until it
	// first spills.
	fresh bool
	calc
}

// calc is what a calculation knows of a formula, cleared once it is over.
type calc struct {
	dirty, busy, done bool
	round             int
	// prior is the area a dynamic formula spilled over before, spilled
	// whether its result is an array.
	prior   Area
	spilled bool
	// readers are the formulas reading this one, pending those not
	// calculated yet among the formulas it reads.
	readers []*cellFormula
	pending int
	value   Value
}

type areaReader struct {
	area Area3
	f    *cellFormula
}

// wideColumns is how many columns an area may span and still be indexed
// column by column.
const wideColumns = 64

func NewEngine(src Source, opt Options) *Engine {
	if opt.Locale == nil {
		opt.Locale = French
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Rand == nil {
		opt.Rand = rand.Float64
	}
	return &Engine{src: src, opt: opt, formulas: map[Pos]*cellFormula{}, readers: map[Pos][]*cellFormula{},
		areas: map[[2]int][]areaReader{}, wide: map[int][]areaReader{}, volatile: map[Pos]bool{}, subtotals: map[Pos]bool{}, arrays: map[Pos]Pos{},
		spills: map[Pos]*cellFormula{}, orphans: map[Pos]bool{}}
}

// Len is the number of formulas.
func (e *Engine) Len() int {
	return len(e.formulas)
}

// Set gives a cell a formula, or takes it away when f is "". An array
// formula fills array, its anchor the cell. A formula that does not read
// is kept with none.
func (e *Engine) Set(p Pos, f string, array *Area) {
	e.set(p, f, array, false)
}

// SetDynamic gives a cell a dynamic array formula, which spills its result
// over the cells next to it, or takes it away when f is "". spill is the
// area it spilled over last, nil for its cell alone.
func (e *Engine) SetDynamic(p Pos, f string, spill *Area) {
	e.set(p, f, spill, true)
}

func (e *Engine) set(p Pos, f string, array *Area, dynamic bool) {
	if old := e.formulas[p]; old != nil {
		e.unindex(p, old)
		delete(e.formulas, p)
	}
	if f == "" {
		return
	}
	x, err := Parse(f)
	if err != nil {
		return
	}
	cf := &cellFormula{pos: p, expr: e.bind(x, p.Sheet), array: array}
	if dynamic {
		if array == nil || array.R1 != p.Row || array.C1 != p.Col {
			one := spillArea(p, 1, 1)
			cf.array = &one
		}
		cf.dynamic, cf.fresh, cf.reach = true, array == nil, *cf.array
		// a formula saved blocked spills as far as the sheet goes
		if v := e.src.Value(p); v.Type == TypeError && v.Str == ErrSpill.Str {
			cf.reach = Area{R1: p.Row, C1: p.Col, R2: MaxRows, C2: MaxCols}
		}
		e.spills[p] = cf
	}
	e.reads(x, p.Sheet, 0, func(a Area3) { cf.reads = append(cf.reads, a) }, func(name string) {
		if f := functions[name]; f != nil && f.volatile {
			e.volatile[p] = true
		}
		if name == "SUBTOTAL" || name == "AGGREGATE" {
			e.subtotals[p] = true
		}
	})
	e.formulas[p] = cf
	for _, a := range cf.reads {
		if a.R1 == a.R2 && a.C1 == a.C2 {
			q := Pos{a.Sheet, a.R1, a.C1}
			e.readers[q] = append(e.readers[q], cf)
			continue
		}
		r := areaReader{a, cf}
		if a.C2-a.C1 >= wideColumns {
			e.wide[a.Sheet] = append(e.wide[a.Sheet], r)
			continue
		}
		for c := a.C1; c <= a.C2; c++ {
			k := [2]int{a.Sheet, c}
			e.areas[k] = append(e.areas[k], r)
		}
	}
	if a := cf.array; a != nil {
		for r := a.R1; r <= a.R2; r++ {
			for c := a.C1; c <= a.C2; c++ {
				e.arrays[Pos{p.Sheet, r, c}] = p
			}
		}
	}
}

// Spill is the area the dynamic formula of a cell spills over, nil when
// it holds none.
func (e *Engine) Spill(p Pos) *Area {
	if cf := e.spills[p]; cf != nil {
		return cf.array
	}
	return nil
}

func (e *Engine) unindex(p Pos, cf *cellFormula) {
	delete(e.volatile, p)
	delete(e.subtotals, p)
	for _, a := range cf.reads {
		if a.R1 == a.R2 && a.C1 == a.C2 {
			q := Pos{a.Sheet, a.R1, a.C1}
			e.readers[q] = slices.DeleteFunc(e.readers[q], func(x *cellFormula) bool { return x == cf })
			if len(e.readers[q]) == 0 {
				delete(e.readers, q)
			}
			continue
		}
		drop := func(list []areaReader) []areaReader {
			return slices.DeleteFunc(list, func(r areaReader) bool { return r.f == cf })
		}
		if a.C2-a.C1 >= wideColumns {
			e.wide[a.Sheet] = drop(e.wide[a.Sheet])
			continue
		}
		for c := a.C1; c <= a.C2; c++ {
			k := [2]int{a.Sheet, c}
			if e.areas[k] = drop(e.areas[k]); len(e.areas[k]) == 0 {
				delete(e.areas, k)
			}
		}
	}
	if cf.array != nil {
		for r := cf.array.R1; r <= cf.array.R2; r++ {
			for c := cf.array.C1; c <= cf.array.C2; c++ {
				q := Pos{p.Sheet, r, c}
				if e.arrays[q] == p {
					delete(e.arrays, q)
					if cf.dynamic && q != p {
						e.orphans[q] = true
					}
				}
			}
		}
	}
	delete(e.spills, p)
}

// reads calls f with the areas a formula reads, those of the names it
// uses included, and calls calls with the functions it calls.
func (e *Engine) reads(x Expr, sheet, depth int, f func(Area3), calls func(name string)) {
	switch x := x.(type) {
	case refExpr:
		r := x.ref
		if r.Invalid || r.Book != "" {
			return
		}
		sheets := []int{sheet}
		if r.Sheet != "" {
			var ok bool
			if sheets, ok = e.src.Sheets(r.Sheet, r.LastSheet); !ok {
				return
			}
		}
		for _, s := range sheets {
			f(Area3{s, r.Area})
		}
	case nameExpr:
		s := sheet
		if x.sheet != "" {
			if ss, ok := e.src.Sheets(x.sheet, ""); ok {
				s = ss[0]
			}
		}
		if text, ok := e.src.Name(x.name, s); ok && depth < maxDepth {
			if y, err := Parse(text); err == nil {
				e.reads(y, s, depth+1, f, calls)
			}
		}
	case unaryExpr:
		e.reads(x.x, sheet, depth, f, calls)
	case percentExpr:
		e.reads(x.x, sheet, depth, f, calls)
	case spillExpr:
		e.reads(x.x, sheet, depth, f, calls)
	case binaryExpr:
		e.reads(x.x, sheet, depth, f, calls)
		e.reads(x.y, sheet, depth, f, calls)
	case callExpr:
		calls(x.name)
		for _, a := range x.args {
			e.reads(a, sheet, depth, f, calls)
		}
	}
}

// bind resolves the sheets of the references of a formula on sheet, so
// that calculating it does not.
func (e *Engine) bind(x Expr, sheet int) Expr {
	switch x := x.(type) {
	case refExpr:
		r := x.ref
		if r.Invalid || r.Book != "" {
			return boundExpr{}
		}
		sheets := []int{sheet}
		if r.Sheet != "" {
			var ok bool
			if sheets, ok = e.src.Sheets(r.Sheet, r.LastSheet); !ok {
				return boundExpr{}
			}
		}
		refs := make([]Area3, len(sheets))
		for i, s := range sheets {
			refs[i] = Area3{s, r.Area}
		}
		return boundExpr{refs}
	case unaryExpr:
		x.x = e.bind(x.x, sheet)
		return x
	case percentExpr:
		x.x = e.bind(x.x, sheet)
		return x
	case spillExpr:
		x.x = e.bind(x.x, sheet)
		return x
	case binaryExpr:
		x.x, x.y = e.bind(x.x, sheet), e.bind(x.y, sheet)
		return x
	case callExpr:
		args := make([]Expr, len(x.args))
		for i, a := range x.args {
			args[i] = e.bind(a, sheet)
		}
		x.args = args
		return x
	}
	return x
}

// Result is a cell whose value a calculation changed.
type Result struct {
	Pos
	Value Value
	// Spill is, on the anchor of a dynamic array formula, the cells it
	// spills over, when they changed or it was set.
	Spill *Area
}

// maxRounds bounds how many times a calculation follows dynamic formulas
// that spill over cells other formulas read.
const maxRounds = 16

// Recalc calculates again the formulas that read the cells changed, those
// that read them in turn, and the volatile ones, and returns the cells
// whose values changed. Cells whose formula was set count as changed; a
// change in column 0 is that of a row hidden or shown, which reaches the
// subtotals reading the row, or all the rows of the sheet in row 0.
// Formulas the engine cannot calculate keep
// their values.
func (e *Engine) Recalc(changed []Pos) []Result {
	e.filled, e.names = map[Pos]Value{}, map[string]Expr{}
	e.bounds = map[int]Area{}
	e.ranges, e.cached = map[Area3][]cachedCell{}, 0
	defer func() {
		for _, cf := range e.dirty {
			cf.calc = calc{}
		}
		e.dirty, e.filled, e.names, e.bounds, e.ranges, e.grown = nil, nil, nil, nil, nil, nil
		e.orphans = map[Pos]bool{}
	}()

	var batch []*cellFormula
	mark := func(cf *cellFormula) { batch = append(batch, cf) }
	for _, p := range changed {
		if p.Col == 0 {
			e.subtotalsOf(p, mark)
			continue
		}
		if anchor, ok := e.arrays[p]; ok && anchor != p && e.spills[anchor] != nil {
			delete(e.arrays, p)
		}
		delete(e.orphans, p)
		e.spillsAt(p, mark)
		if cf := e.formulas[p]; cf != nil {
			mark(cf)
			continue
		}
		e.readersOf(p, mark)
	}
	for p := range e.orphans {
		if _, array := e.arrays[p]; array || e.formulas[p] != nil {
			continue
		}
		e.filled[p] = Value{}
		e.readersOf(p, mark)
		e.spillsAt(p, mark)
	}
	for p := range e.volatile {
		mark(e.formulas[p])
	}
	e.run(batch)
	for round := 0; len(e.grown) > 0 && round < maxRounds; round++ {
		batch = nil
		for _, p := range e.grown {
			e.readersOf(p, mark)
			e.spillsAt(p, mark)
		}
		e.grown = nil
		e.run(batch)
	}

	var out []Result
	add := func(r Result) {
		if r.Spill == nil && (r.Value.Type == TypeError && r.Value.Str == ErrUnsupported.Str || sameValue(r.Value, e.src.Value(r.Pos))) {
			return
		}
		out = append(out, r)
	}
	for _, cf := range e.dirty {
		if cf.array == nil {
			add(Result{Pos: cf.pos, Value: cf.value})
		}
	}
	for p, v := range e.filled {
		r := Result{Pos: p, Value: v}
		if cf := e.spills[p]; cf != nil && cf.dirty && cf.done && (cf.fresh && cf.spilled || cf.prior != *cf.array) {
			r.Spill = cf.array
			cf.fresh = false
		}
		add(r)
	}
	slices.SortFunc(out, func(a, b Result) int { return comparePos(a.Pos, b.Pos) })
	return out
}

// run calculates a batch of formulas, and those they reach, in the order
// they read each other. Formulas calculated in a round before are
// calculated again.
func (e *Engine) run(batch []*cellFormula) {
	e.round++
	var all []*cellFormula
	join := func(cf *cellFormula) {
		if cf.round == e.round {
			return
		}
		if !cf.dirty {
			cf.dirty = true
			if cf.array != nil {
				cf.prior = *cf.array
			}
			e.dirty = append(e.dirty, cf)
		}
		cf.round, cf.done, cf.readers, cf.pending = e.round, false, nil, 0
		all = append(all, cf)
	}
	for _, cf := range batch {
		join(cf)
	}
	for i := 0; i < len(all); i++ {
		cf := all[i]
		e.cellsOf(cf, func(q Pos) {
			e.readersOf(q, func(f *cellFormula) {
				cf.readers = append(cf.readers, f)
				join(f)
			})
		})
	}

	for _, cf := range all {
		p := cf.pos
		a := Area{R1: p.Row, C1: p.Col, R2: p.Row, C2: p.Col}
		if cf.array != nil {
			a = *cf.array
		}
		e.widen(p.Sheet, a)
	}

	// the formulas in the order they read each other
	for _, cf := range all {
		for _, f := range cf.readers {
			if f != cf {
				f.pending++
			}
		}
	}
	var ready []*cellFormula
	for _, cf := range all {
		if cf.pending == 0 {
			ready = append(ready, cf)
		}
	}
	slices.SortFunc(ready, func(a, b *cellFormula) int { return comparePos(a.pos, b.pos) })
	for len(ready) > 0 {
		cf := ready[0]
		ready = ready[1:]
		e.compute(cf)
		for _, f := range cf.readers {
			if f == cf {
				continue
			}
			if f.pending--; f.pending == 0 {
				ready = append(ready, f)
			}
		}
	}
	// what is left reads itself in a circle
	for _, cf := range all {
		e.compute(cf)
	}
}

// widen adds an area to the bounds of the formulas calculated on a sheet.
func (e *Engine) widen(sheet int, a Area) {
	if b, ok := e.bounds[sheet]; ok {
		a = Area{R1: min(a.R1, b.R1), C1: min(a.C1, b.C1), R2: max(a.R2, b.R2), C2: max(a.C2, b.C2)}
	}
	e.bounds[sheet] = a
}

// spillsAt calls f with the dynamic formulas, other than one at p, that
// spill or would spill over p.
func (e *Engine) spillsAt(p Pos, f func(*cellFormula)) {
	for q, cf := range e.spills {
		if q != p && q.Sheet == p.Sheet && cf.reach.Contains(p.Row, p.Col) {
			f(cf)
		}
	}
}

func comparePos(a, b Pos) int {
	switch {
	case a.Sheet != b.Sheet:
		return a.Sheet - b.Sheet
	case a.Row != b.Row:
		return a.Row - b.Row
	}
	return a.Col - b.Col
}

// cellsOf calls f with the cells a formula fills.
func (e *Engine) cellsOf(cf *cellFormula, f func(Pos)) {
	if cf.array == nil {
		f(cf.pos)
		return
	}
	for r := cf.array.R1; r <= cf.array.R2; r++ {
		for c := cf.array.C1; c <= cf.array.C2; c++ {
			f(Pos{cf.pos.Sheet, r, c})
		}
	}
}

// readersOf calls f with the formulas reading a cell.
func (e *Engine) readersOf(p Pos, f func(*cellFormula)) {
	for _, r := range e.readers[p] {
		f(r)
	}
	for _, r := range e.areas[[2]int{p.Sheet, p.Col}] {
		if r.area.Contains(p.Row, p.Col) {
			f(r.f)
		}
	}
	for _, r := range e.wide[p.Sheet] {
		if r.area.Contains(p.Row, p.Col) {
			f(r.f)
		}
	}
}

// subtotalsOf calls f with the subtotals reading the row of p, any row of
// its sheet in row 0.
func (e *Engine) subtotalsOf(p Pos, f func(*cellFormula)) {
	for q := range e.subtotals {
		cf := e.formulas[q]
		for _, a := range cf.reads {
			if a.Sheet == p.Sheet && (p.Row == 0 || a.R1 <= p.Row && p.Row <= a.R2) {
				f(cf)
				break
			}
		}
	}
}

// compute calculates a formula, once per calculation; a formula read
// while it is being calculated reads 0, as Excel does in a circle.
func (e *Engine) compute(cf *cellFormula) {
	if cf.done || cf.busy {
		return
	}
	p := cf.pos
	cf.busy = true
	c := &Context{Book: (*engineBook)(e), Sheet: p.Sheet, Row: p.Row, Col: p.Col, Locale: e.opt.Locale,
		Now: e.opt.Now(), Rand: e.opt.Rand, Date1904: e.opt.Date1904}
	v := c.Eval(cf.expr)
	cf.busy, cf.done = false, true
	switch {
	case cf.dynamic:
		e.spill(c, cf, v)
		return
	case cf.array == nil:
		cf.value = cellValue(c.scalar(v))
		return
	}
	a := cf.array
	t := c.array(v)
	for r := a.R1; r <= a.R2; r++ {
		for k := a.C1; k <= a.C2; k++ {
			i, j := r-a.R1, k-a.C1
			if t.Rows == 1 {
				i = 0
			}
			if t.Cols == 1 {
				j = 0
			}
			x := ErrNA
			if i < t.Rows && j < t.Cols {
				x = cellValue(t.At(i, j))
			}
			e.filled[Pos{p.Sheet, r, k}] = x
		}
	}
}

// spill gives a dynamic formula the cells its result takes, those next to
// its cell, or #SPILL! when they are not free.
func (e *Engine) spill(c *Context, cf *cellFormula, v Value) {
	p := cf.pos
	if v.Type == TypeError && v.Str == ErrUnsupported.Str {
		e.filled[p] = v
		return
	}
	single := v.Type != TypeArray && (v.Type != TypeRange || len(v.Refs) == 1 && v.Refs[0].R1 == v.Refs[0].R2 && v.Refs[0].C1 == v.Refs[0].C2)
	t := &Table{Rows: 1, Cols: 1, Cells: []Value{c.scalar(v)}}
	if !single {
		t = c.array(v)
	}
	cf.spilled = !single
	want := spillArea(p, t.Rows, t.Cols)
	cf.reach = spillArea(p, min(t.Rows, MaxRows-p.Row+1), min(t.Cols, MaxCols-p.Col+1))
	area := want
	if want != cf.reach || !e.free(cf, want) {
		area = spillArea(p, 1, 1)
		t = &Table{Rows: 1, Cols: 1, Cells: []Value{ErrSpill}}
	}
	old := *cf.array
	for r := old.R1; r <= old.R2; r++ {
		for k := old.C1; k <= old.C2; k++ {
			q := Pos{p.Sheet, r, k}
			if !area.Contains(r, k) && e.arrays[q] == p {
				delete(e.arrays, q)
				e.filled[q] = Value{}
			}
		}
	}
	for r := area.R1; r <= area.R2; r++ {
		for k := area.C1; k <= area.C2; k++ {
			q := Pos{p.Sheet, r, k}
			if _, owned := e.arrays[q]; !owned && !old.Contains(r, k) {
				e.grown = append(e.grown, q)
			}
			e.arrays[q] = p
			e.filled[q] = cellValue(t.At(r-area.R1, k-area.C1))
		}
	}
	cf.value = e.filled[p]
	if area != old {
		cf.array = &area
		e.widen(p.Sheet, area)
		e.widen(p.Sheet, old)
	}
}

// spillArea is the area of rows and columns from p, a cell when it is one.
func spillArea(p Pos, rows, cols int) Area {
	return Area{R1: p.Row, C1: p.Col, R2: p.Row + rows - 1, C2: p.Col + cols - 1, Cell: rows == 1 && cols == 1}
}

// free tells whether the cells of an area, but the anchor's, hold nothing
// a dynamic formula may not spill over: values, formulas, other arrays.
func (e *Engine) free(cf *cellFormula, a Area) bool {
	p := cf.pos
	for q, other := range e.spills {
		b := other.array
		if q != p && q.Sheet == p.Sheet && b.R1 <= a.R2 && a.R1 <= b.R2 && b.C1 <= a.C2 && a.C1 <= b.C2 {
			return false
		}
	}
	ok := true
	e.src.Each(Area3{p.Sheet, a}, func(row, col int, v Value) bool {
		q := Pos{p.Sheet, row, col}
		if owner, array := e.arrays[q]; q != p && e.formulas[q] != nil || array && owner != p {
			ok = false
		} else if !array && !e.orphans[q] {
			if x, cleared := e.filled[q]; cleared {
				v = x
			}
			ok = v.Type == TypeBlank
		}
		return ok
	})
	return ok
}

// cellValue is a value as a cell holds it: an empty cell read gives 0.
func cellValue(v Value) Value {
	switch v.Type {
	case TypeBlank:
		return Num(0)
	case TypeArray, TypeRange:
		return ErrValue
	}
	return v
}

// sameValue tells whether a value is the one a cell holds, numbers
// compared to the 15 digits Excel keeps: what a workbook saved, calculated
// again, stays the same.
func sameValue(a, b Value) bool {
	if a.Type == TypeNumber && b.Type == TypeNumber {
		if a.Num == b.Num {
			return true
		}
		if math.Abs(a.Num-b.Num) > 1e-14*max(math.Abs(a.Num), math.Abs(b.Num)) {
			return false
		}
		return strconv.FormatFloat(a.Num, 'g', 15, 64) == strconv.FormatFloat(b.Num, 'g', 15, 64)
	}
	return a.Type == b.Type && a.Num == b.Num && a.Str == b.Str
}

// engineBook is the workbook as the formulas being calculated read it:
// the values just calculated over those the source holds.
type engineBook Engine

func (b *engineBook) value(p Pos) Value {
	e := (*Engine)(b)
	v, ok := Value{}, false
	if anchor, array := e.arrays[p]; array {
		if cf := e.formulas[anchor]; cf != nil && cf.dirty {
			e.compute(cf)
			v, ok = e.filled[p]
			if !ok && cf.busy && anchor == p {
				return Num(0)
			}
		}
	} else if cf := e.formulas[p]; cf != nil && cf.dirty {
		e.compute(cf)
		if cf.busy {
			return Num(0)
		}
		v, ok = cf.value, true
	} else if v, ok = e.filled[p]; ok {
		// a cell a dynamic formula no longer spills over
		return v
	}
	if !ok || v.Type == TypeError && v.Str == ErrUnsupported.Str {
		return e.src.Value(p)
	}
	return v
}

func (b *engineBook) Cell(sheet, row, col int) Value {
	return b.value(Pos{sheet, row, col})
}

func (b *engineBook) Cells(a Area3, f func(row, col int, v Value) bool) {
	e := (*Engine)(b)
	if d, ok := e.bounds[a.Sheet]; !ok || d.R2 < a.R1 || d.R1 > a.R2 || d.C2 < a.C1 || d.C1 > a.C2 {
		b.sourceCells(a, f)
		return
	}
	if b.spilt(a) {
		b.spiltCells(a, f)
		return
	}
	e.src.Each(a, func(row, col int, v Value) bool {
		p := Pos{a.Sheet, row, col}
		if _, array := e.arrays[p]; array {
			v = b.value(p)
		} else if cf := e.formulas[p]; cf != nil && cf.dirty {
			v = b.value(p)
		}
		if v.Type == TypeBlank {
			return true
		}
		return f(row, col, v)
	})
}

// spilt tells whether an area meets the cells a dynamic formula of the
// calculation spills over, or spilled over before it.
func (b *engineBook) spilt(a Area3) bool {
	for q, cf := range b.spills {
		if q.Sheet == a.Sheet && cf.dirty && (overlaps(*cf.array, a.Area) || overlaps(cf.prior, a.Area)) {
			return true
		}
	}
	return false
}

func overlaps(a, b Area) bool {
	return a.R1 <= b.R2 && b.R1 <= a.R2 && a.C1 <= b.C2 && b.C1 <= a.C2
}

// spiltCells calls f with the cells of an area the dynamic formulas of
// the calculation spill over, which the source may not hold yet, and those
// it holds.
func (b *engineBook) spiltCells(a Area3, f func(row, col int, v Value) bool) {
	e := (*Engine)(b)
	seen := map[Pos]bool{}
	var cells []Pos
	add := func(p Pos) {
		if !seen[p] && a.Contains(p.Row, p.Col) {
			seen[p] = true
			cells = append(cells, p)
		}
	}
	e.src.Each(a, func(row, col int, _ Value) bool {
		add(Pos{a.Sheet, row, col})
		return true
	})
	for p := range e.filled {
		if p.Sheet == a.Sheet {
			add(p)
		}
	}
	slices.SortFunc(cells, comparePos)
	for _, p := range cells {
		if v := b.value(p); v.Type != TypeBlank && !f(p.Row, p.Col, v) {
			return
		}
	}
}

// sourceCells calls f with the cells of an area no formula calculated
// reaches, kept from the second time it is read.
func (b *engineBook) sourceCells(a Area3, f func(row, col int, v Value) bool) {
	e := (*Engine)(b)
	cells, seen := e.ranges[a]
	if !seen || cells == nil && e.cached >= cachedCells {
		if !seen {
			e.ranges[a] = nil
		}
		e.src.Each(a, func(row, col int, v Value) bool {
			return v.Type == TypeBlank || f(row, col, v)
		})
		return
	}
	if cells == nil {
		cells = []cachedCell{}
		e.src.Each(a, func(row, col int, v Value) bool {
			if v.Type != TypeBlank {
				cells = append(cells, cachedCell{int32(row), int32(col), v.Type, v.Num, v.Str})
			}
			return true
		})
		e.ranges[a] = cells
		e.cached += len(cells)
	}
	for _, c := range cells {
		if !f(int(c.row), int(c.col), Value{Type: c.typ, Num: c.num, Str: c.str}) {
			return
		}
	}
}

func (b *engineBook) Size(sheet int) (int, int) {
	rows, cols := b.src.Size(sheet)
	if d, ok := b.bounds[sheet]; ok {
		rows, cols = max(rows, d.R2), max(cols, d.C2)
	}
	return rows, cols
}

// Spill is the area the dynamic formula of a cell spills over, its error
// when it cannot; false when the cell holds none.
func (b *engineBook) Spill(sheet, row, col int) (Value, bool) {
	cf := b.spills[Pos{sheet, row, col}]
	if cf == nil {
		return Value{}, false
	}
	if v := b.value(cf.pos); v.Type == TypeError && v.Str == ErrSpill.Str {
		return v, true
	}
	return Value{Type: TypeRange, Refs: []Area3{{sheet, *cf.array}}}, true
}

func (b *engineBook) Sheets(first, last string) ([]int, bool) {
	return b.src.Sheets(first, last)
}

func (b *engineBook) Name(name string, sheet int) (Expr, bool) {
	key := strings.ToUpper(name) + "!" + strconv.Itoa(sheet)
	if x, ok := b.names[key]; ok {
		return x, x != nil
	}
	text, ok := b.src.Name(name, sheet)
	var x Expr
	if ok {
		x, _ = Parse(text)
	}
	b.names[key] = x
	return x, x != nil
}

// Hidden tells whether a row is hidden, and whether by a filter, when the
// source knows.
func (b *engineBook) Hidden(sheet, row int) (hidden, filtered bool) {
	if r, ok := b.src.(Rows); ok {
		return r.Hidden(sheet, row)
	}
	return false, false
}

// Subtotal tells whether a cell calls SUBTOTAL or AGGREGATE, which those
// leave out.
func (b *engineBook) Subtotal(sheet, row, col int) bool {
	return b.subtotals[Pos{sheet, row, col}]
}

// HasFormula tells whether a cell holds a formula, for ISFORMULA.
func (b *engineBook) HasFormula(sheet, row, col int) bool {
	_, ok := b.formulas[Pos{sheet, row, col}]
	return ok
}
