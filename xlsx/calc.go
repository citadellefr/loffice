package xlsx

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/ot"
)

// Calc keeps the formulas of a workbook calculated while it is edited. It
// follows each edit with the changes the server makes to the tree: the
// formulas moved with the rows and columns inserted or removed, renamed
// with their sheet, and the values of those the edit reaches calculated
// again.
type Calc struct {
	tree *ot.Tree
	opt  formula.Options
	// sheets are the ids of the sheets in the order of the workbook, the
	// engine's handles their indexes.
	sheets  []string
	handles map[string]int
	// names are the names of the sheets as the formulas know them.
	names   map[string]string
	defined []Name
	engine  *formula.Engine
	bounds  map[int][2]int
	// filters are, by sheet, the rows under the first of a filter that
	// filters columns, read once per calculation.
	filters map[int]*formula.Area
}

// NewCalc keeps the formulas of the tree calculated. Their values are
// taken as they are: the file holds them.
func NewCalc(tree *ot.Tree, opt formula.Options) *Calc {
	if book := tree.Node("book"); book != nil {
		_ = json.Unmarshal(book.Attrs["date1904"], &opt.Date1904)
	}
	if opt.Locale == nil {
		opt.Locale = formula.French
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	if opt.Rand == nil {
		opt.Rand = rand.Float64
	}
	c := &Calc{tree: tree, opt: opt}
	c.rebuild()
	for _, id := range c.sheets {
		if e := c.looksChanges(id, c.looks(id)); e != nil {
			_ = tree.Apply(e)
		}
	}
	return c
}

// rebuild reads the sheets and the formulas of the tree again.
func (c *Calc) rebuild() {
	c.sheets, c.handles, c.names = nil, map[string]int{}, map[string]string{}
	for _, n := range c.tree.Children("book") {
		if n.Type == "sheet" || n.Type == "kept" {
			c.handles[n.ID] = len(c.sheets)
			c.sheets = append(c.sheets, n.ID)
			c.names[n.ID] = str(n, "name")
		}
	}
	c.defined = nil
	if book := c.tree.Node("book"); book != nil {
		_ = json.Unmarshal(book.Attrs["names"], &c.defined)
	}
	c.bounds, c.filters = map[int][2]int{}, map[int]*formula.Area{}
	c.engine = formula.NewEngine(c, c.opt)
	for h, id := range c.sheets {
		n := c.tree.Node(id)
		if n.Grid == nil {
			continue
		}
		n.Grid.Each(1, formula.MaxRows, func(row, col int, raw json.RawMessage) bool {
			if col > 0 && bytes.Contains(raw, []byte(`"f":`)) {
				c.set(formula.Pos{Sheet: h, Row: row, Col: col}, raw)
			}
			return true
		})
	}
}

// set gives the engine the formula of a cell, or takes it away.
func (c *Calc) set(p formula.Pos, raw json.RawMessage) {
	var f cellFields
	if raw != nil && bytes.Contains(raw, []byte(`"f":`)) {
		_ = json.Unmarshal(raw, &f)
	}
	var array *formula.Area
	if f.FA != "" {
		if a, ok := formula.ParseArea(f.FA); ok && a.Contains(p.Row, p.Col) {
			array = &a
		}
	}
	c.engine.Set(p, f.F, array)
}

// Formulas is the number of formulas the engine calculates.
func (c *Calc) Formulas() int {
	return c.engine.Len()
}

// Follow makes the changes that follow an edit the tree just took, and
// returns them. since are the edits the edit was made without, which it
// was rebased over: the formulas it sets are moved by the rows they
// inserted or removed.
func (c *Calc) Follow(e ot.Edit, since []ot.Edit) ot.Edit {
	f := &follow{c: c, cells: map[string]map[[2]int]map[string]json.RawMessage{}, sheets: map[string]ot.Values{}}
	// whether sheets came, went, moved or were renamed, or names changed
	books := false
	for _, ch := range e {
		h, sheet := c.handles[ch.ID]
		_, name := ch.Attrs["name"]
		_, names := ch.Attrs["names"]
		switch ch.Op {
		case ot.OpCel, ot.OpIns, ot.OpRem:
			if sheet {
				delete(c.bounds, h)
			}
		case ot.OpNew:
			books = books || ch.Type == "sheet"
		case ot.OpDel:
			books = books || sheet
		case ot.OpSet:
			books = books || sheet && (name || ch.Key != "") || ch.ID == "book" && names
		}
	}
	if books {
		f.sheetsChanged()
	}
	for _, ch := range e {
		if ch.Op == ot.OpIns || ch.Op == ot.OpRem {
			f.shift(ch)
		}
	}
	f.rebased(e, since)
	f.flush()
	if books || f.moved {
		c.rebuild()
	}
	f.recalc(e)
	f.flush()
	f.restyle(e, books)
	return f.out
}

// follow is what the server does after an edit.
type follow struct {
	c     *Calc
	out   ot.Edit
	moved bool
	// cells are the fields to set, by sheet and cell.
	cells map[string]map[[2]int]map[string]json.RawMessage
	// book are the attributes of the book to set.
	book ot.Values
	// sheets are the attributes of sheets to set, by sheet.
	sheets map[string]ot.Values
}

// attr reads an attribute of a sheet into v, the value to set if any.
func (f *follow) attr(id, key string, v any) {
	raw, ok := f.sheets[id][key]
	if !ok {
		if n := f.c.tree.Node(id); n != nil {
			raw = n.Attrs[key]
		}
	}
	_ = json.Unmarshal(raw, v)
}

// set sets an attribute of a sheet.
func (f *follow) set(id, key string, raw json.RawMessage) {
	if f.sheets[id] == nil {
		f.sheets[id] = ot.Values{}
	}
	f.sheets[id][key] = raw
}

// setList sets a list attribute of a sheet, removed when it is empty.
func setList[T any](f *follow, id, key string, v []T) {
	raw := json.RawMessage("null")
	if len(v) > 0 {
		raw = mustJSON(v)
	}
	f.set(id, key, raw)
}

// shiftFilter moves the filter of a sheet with rows or columns inserted
// or removed.
func (f *follow) shiftFilter(id, sheet string, rows bool, at, n int) {
	var filter *AutoFilter
	if f.attr(id, filterKey, &filter); filter == nil {
		return
	}
	raw := json.RawMessage("null")
	if g := filter.shifted(sheet, rows, at, n); g != nil {
		raw = mustJSON(g)
	}
	f.set(id, filterKey, raw)
}

func (f *follow) setCell(sheet string, row, col int, key string, v any) {
	m := f.cells[sheet]
	if m == nil {
		m = map[[2]int]map[string]json.RawMessage{}
		f.cells[sheet] = m
	}
	fields := m[[2]int{row, col}]
	if fields == nil {
		fields = map[string]json.RawMessage{}
		m[[2]int{row, col}] = fields
	}
	raw, ok := v.(json.RawMessage)
	if !ok {
		raw = mustJSON(v)
	}
	fields[key] = raw
}

// flush applies what the follow-up has to set, and adds it to its edit.
func (f *follow) flush() {
	var e ot.Edit
	for _, id := range slices.Sorted(mapKeys(f.cells)) {
		var cells []ot.Cell
		for _, at := range slices.SortedFunc(mapKeys(f.cells[id]), func(a, b [2]int) int {
			if a[0] != b[0] {
				return a[0] - b[0]
			}
			return a[1] - b[1]
		}) {
			cells = append(cells, ot.Cell{Row: at[0], Col: at[1], Fields: object(f.cells[id][at])})
		}
		e = append(e, ot.Change{Op: ot.OpCel, ID: id, Cells: cells})
	}
	for _, id := range slices.Sorted(mapKeys(f.sheets)) {
		e = append(e, ot.Change{Op: ot.OpSet, ID: id, Attrs: f.sheets[id]})
	}
	if len(f.book) > 0 {
		e = append(e, ot.Change{Op: ot.OpSet, ID: "book", Attrs: f.book})
	}
	f.cells, f.book, f.sheets = map[string]map[[2]int]map[string]json.RawMessage{}, nil, map[string]ot.Values{}
	if len(e) == 0 {
		return
	}
	if err := f.c.tree.Apply(e); err != nil {
		return
	}
	f.out = append(f.out, e...)
	for _, ch := range e {
		if h, ok := f.c.handles[ch.ID]; ok && ch.Op == ot.OpCel {
			delete(f.c.bounds, h)
		}
	}
}

// object writes fields as compact JSON, keys in order.
func object(fields map[string]json.RawMessage) json.RawMessage {
	b := []byte{'{'}
	for _, k := range slices.Sorted(mapKeys(fields)) {
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = strconv.AppendQuote(b, k)
		b = append(b, ':')
		b = append(b, fields[k]...)
	}
	return append(b, '}')
}

func mapKeys[K comparable, V any](m map[K]V) func(func(K) bool) {
	return func(yield func(K) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// formulas calls rewrite with every formula of the workbook, those of the
// defined names, list validations, conditional formats and charts included,
// and sets those it changes.
func (f *follow) formulas(rewrite func(text, sheet string) string) {
	c := f.c
	for _, id := range c.sheets {
		n := c.tree.Node(id)
		if n == nil || n.Grid == nil {
			continue
		}
		n.Grid.Each(1, formula.MaxRows, func(row, col int, raw json.RawMessage) bool {
			if col == 0 || !bytes.Contains(raw, []byte(`"f":`)) {
				return true
			}
			var fields cellFields
			_ = json.Unmarshal(raw, &fields)
			if fields.F != "" {
				if g := rewrite(fields.F, id); g != fields.F {
					f.setCell(id, row, col, "f", g)
				}
			}
			return true
		})
		var lists []List
		f.attr(id, listsKey, &lists)
		changed := false
		for i, l := range lists {
			if g := rewrite(l.Src, id); g != l.Src {
				lists[i].Src = g
				changed = true
			}
		}
		if changed {
			setList(f, id, listsKey, lists)
		}
		var rules []Conditional
		f.attr(id, conditionalKey, &rules)
		changed = false
		for i, r := range rules {
			for j, x := range r.F {
				if g := rewrite(x, id); g != x {
					rules[i].F[j] = g
					changed = true
				}
			}
			for j, t := range r.Stops {
				if g := rewrite(t.Val, id); t.Val != "" && g != t.Val {
					rules[i].Stops[j].Val = g
					changed = true
				}
			}
		}
		if changed {
			setList(f, id, conditionalKey, rules)
		}
		var charts []Placed
		f.attr(id, chartsKey, &charts)
		changed = false
		for _, p := range charts {
			if p.Chart != nil && chartRefs(p.Chart, func(ref string) string { return rewrite(ref, id) }) {
				changed = true
			}
		}
		if changed {
			setList(f, id, chartsKey, charts)
		}
	}
	changed := false
	names := slices.Clone(c.defined)
	for i, n := range names {
		if g := rewrite(n.Ref, n.Sheet); g != n.Ref {
			names[i].Ref = g
			changed = true
		}
	}
	if changed {
		f.book = ot.Values{"names": mustJSON(names)}
		c.defined = names
	}
}

// sheetsChanged follows sheets renamed, added and deleted: formulas take
// the new names, those of deleted sheets become #REF!, and names Excel
// would refuse are made ones it takes.
func (f *follow) sheetsChanged() {
	c := f.c
	used := map[string]bool{}
	var renamed [][2]string
	for i, n := range c.tree.Children("book") {
		if n.Type != "sheet" && n.Type != "kept" {
			continue
		}
		name := str(n, "name")
		fixed := sheetName(name, i, used)
		if fixed != name {
			f.out = append(f.out, ot.Change{Op: ot.OpSet, ID: n.ID, Attrs: ot.Values{"name": mustJSON(fixed)}})
			_ = c.tree.Apply(ot.Edit{f.out[len(f.out)-1]})
		}
		if old, ok := c.names[n.ID]; ok && !strings.EqualFold(old, fixed) {
			renamed = append(renamed, [2]string{old, fixed})
		}
	}
	var gone []string
	for id, name := range c.names {
		if n := c.tree.Node(id); n == nil {
			gone = append(gone, name)
		}
	}
	if len(renamed) == 0 && len(gone) == 0 {
		return
	}
	f.formulas(func(text, _ string) string {
		for _, name := range gone {
			text, _ = formula.Drop(text, name)
		}
		// through names no sheet has, so that swapped names do not mix
		for i, r := range renamed {
			text, _ = formula.Rename(text, r[0], "\x00"+strconv.Itoa(i))
		}
		for i, r := range renamed {
			text, _ = formula.Rename(text, "\x00"+strconv.Itoa(i), r[1])
		}
		return text
	})
	f.flush()
}

// shift follows rows or columns inserted or removed on a sheet: the
// formulas that point there move, as do the merged cells and array
// formulas that span them and the cells lists validate.
func (f *follow) shift(ch ot.Change) {
	c := f.c
	sheet := c.names[ch.ID]
	if sheet == "" {
		return
	}
	n := ch.N
	if ch.Op == ot.OpRem {
		n = -n
	}
	rows := ch.Dim == ot.DimRows
	f.formulas(func(text, id string) string {
		g, err := formula.Shift(text, sheet, id == ch.ID, rows, ch.At, n)
		if err != nil {
			return text
		}
		return g
	})
	var lists []List
	if f.attr(ch.ID, listsKey, &lists); len(lists) > 0 {
		var kept []List
		for _, l := range lists {
			if l.Ref = shiftRef(l.Ref, sheet, rows, ch.At, n); l.Ref != "" {
				kept = append(kept, l)
			}
		}
		setList(f, ch.ID, listsKey, kept)
	}
	var rules []Conditional
	if f.attr(ch.ID, conditionalKey, &rules); len(rules) > 0 {
		var kept []Conditional
		for _, r := range rules {
			if r.Ref = shiftRef(r.Ref, sheet, rows, ch.At, n); r.Ref != "" {
				kept = append(kept, r)
			}
		}
		setList(f, ch.ID, conditionalKey, kept)
	}
	var charts []Placed
	if f.attr(ch.ID, chartsKey, &charts); len(charts) > 0 {
		shiftCharts(charts, rows, ch.At, n)
		setList(f, ch.ID, chartsKey, charts)
	}
	f.shiftFilter(ch.ID, sheet, rows, ch.At, n)
	if looks := looksID(ch.ID); c.tree.Node(looks) != nil {
		mv := ot.Edit{{Op: ch.Op, ID: looks, Dim: ch.Dim, At: ch.At, N: ch.N}}
		if c.tree.Apply(mv) == nil {
			f.out = append(f.out, mv...)
		}
	}
	node := c.tree.Node(ch.ID)
	if node != nil && node.Grid != nil {
		node.Grid.Each(1, formula.MaxRows, func(row, col int, raw json.RawMessage) bool {
			if col == 0 || !bytes.Contains(raw, []byte(`"m":`)) && !bytes.Contains(raw, []byte(`"fa":`)) {
				return true
			}
			var fields cellFields
			_ = json.Unmarshal(raw, &fields)
			if fields.M != nil {
				if m := spanShifted(*fields.M, row, col, rows, ch.At, n); m != *fields.M {
					f.setCell(ch.ID, row, col, "m", m)
				}
			}
			if fields.FA != "" {
				if fa, err := formula.Shift(fields.FA, sheet, true, rows, ch.At, n); err == nil && fa != fields.FA {
					f.setCell(ch.ID, row, col, "fa", fa)
				}
			}
			return true
		})
	}
	f.moved = true
}

// spanShifted is the span of cells merged from row and col once rows or
// columns are inserted or removed at at: those inserted inside it widen
// it, those removed from it narrow it. The first cell has moved already.
func spanShifted(m [2]int, row, col int, rows bool, at, n int) [2]int {
	first, span := col, &m[1]
	if rows {
		first, span = row, &m[0]
	}
	if first >= at {
		return m
	}
	last := first + *span - 1
	switch {
	case n > 0 && at <= last:
		*span += n
	case n < 0 && at <= last:
		*span -= min(last, at-n-1) - at + 1
	}
	return m
}

// restyle calculates again the looks conditional formats give cells: on
// the sheets whose cells or rules changed, those whose rules read other
// sheets when any did, all when sheets or names changed.
func (f *follow) restyle(e ot.Edit, all bool) {
	changed := map[string]bool{}
	for _, ch := range slices.Concat(e, f.out) {
		switch {
		case ch.Op == ot.OpCel || ch.Op == ot.OpIns || ch.Op == ot.OpRem || ch.Op == ot.OpNew && ch.Cells != nil:
			changed[ch.ID] = true
		case ch.Op == ot.OpSet && ch.Attrs[conditionalKey] != nil:
			changed[ch.ID] = true
		}
	}
	if len(changed) == 0 && !all {
		return
	}
	c := f.c
	for _, id := range c.sheets {
		n := c.tree.Node(id)
		var rules []Conditional
		if n != nil {
			_ = json.Unmarshal(n.Attrs[conditionalKey], &rules)
		}
		if len(rules) == 0 && c.tree.Node(looksID(id)) == nil || !all && !changed[id] && !reachesOut(rules) {
			continue
		}
		if edit := c.looksChanges(id, c.looks(id)); edit != nil && c.tree.Apply(edit) == nil {
			f.out = append(f.out, edit...)
		}
	}
}

// rebased moves the formulas and the filters an edit sets with the rows
// and columns the edits it was rebased over inserted or removed.
func (f *follow) rebased(e ot.Edit, since []ot.Edit) {
	var shifts []ot.Change
	for _, h := range since {
		for _, ch := range h {
			if ch.Op == ot.OpIns || ch.Op == ot.OpRem {
				shifts = append(shifts, ch)
			}
		}
	}
	if len(shifts) == 0 {
		return
	}
	for _, ch := range e {
		if ch.Op == ot.OpSet && ch.Attrs[filterKey] != nil {
			for _, s := range shifts {
				n := s.N
				if s.Op == ot.OpRem {
					n = -n
				}
				if s.ID == ch.ID {
					f.shiftFilter(ch.ID, f.c.names[ch.ID], s.Dim == ot.DimRows, s.At, n)
				}
			}
		}
		if ch.Op != ot.OpCel && !(ch.Op == ot.OpNew && ch.Cells != nil) {
			continue
		}
		for _, cell := range ch.Cells {
			var fields map[string]json.RawMessage
			if json.Unmarshal(cell.Fields, &fields) != nil || fields["f"] == nil {
				continue
			}
			var text string
			if json.Unmarshal(fields["f"], &text) != nil || text == "" {
				continue
			}
			moved := text
			for _, s := range shifts {
				n := s.N
				if s.Op == ot.OpRem {
					n = -n
				}
				moved, _ = formula.Shift(moved, f.c.names[s.ID], s.ID == ch.ID, s.Dim == ot.DimRows, s.At, n)
			}
			if moved != text {
				f.setCell(ch.ID, cell.Row, cell.Col, "f", moved)
			}
		}
	}
}

// recalc calculates again what the edit and the follow-up reach, and sets
// the values that changed.
func (f *follow) recalc(e ot.Edit) {
	c := f.c
	var changed []formula.Pos
	seen := map[formula.Pos]bool{}
	c.filters = map[int]*formula.Area{}
	// cells in column 0 are rows hidden or shown, row 0 all of a sheet's
	add := func(id string, row, col int) {
		h, ok := c.handles[id]
		p := formula.Pos{Sheet: h, Row: row, Col: col}
		if !ok || row == 0 && col != 0 || seen[p] {
			return
		}
		seen[p] = true
		changed = append(changed, p)
		if n := c.tree.Node(id); col > 0 && n != nil && n.Grid != nil {
			c.set(p, n.Grid.Cell(row, col))
		}
	}
	for _, ch := range append(slices.Clone(e), f.out...) {
		switch {
		case ch.Op == ot.OpCel || ch.Op == ot.OpNew && ch.Cells != nil:
			for _, cell := range ch.Cells {
				if cell.Row > 0 {
					add(ch.ID, cell.Row, cell.Col)
				}
			}
		case ch.Op == ot.OpSet && ch.Attrs[filterKey] != nil:
			add(ch.ID, 0, 0)
		case ch.Op == ot.OpIns || ch.Op == ot.OpRem:
			// what moved may read other cells now: ROW(), OFFSET
			h := c.handles[ch.ID]
			n := c.tree.Node(ch.ID)
			if n == nil || n.Grid == nil {
				continue
			}
			n.Grid.Each(1, formula.MaxRows, func(row, col int, raw json.RawMessage) bool {
				moved := ch.Dim == ot.DimRows && row >= ch.At || ch.Dim == ot.DimCols && col >= ch.At
				if col > 0 && moved && bytes.Contains(raw, []byte(`"f":`)) && !seen[formula.Pos{Sheet: h, Row: row, Col: col}] {
					seen[formula.Pos{Sheet: h, Row: row, Col: col}] = true
					changed = append(changed, formula.Pos{Sheet: h, Row: row, Col: col})
				}
				return true
			})
		}
	}
	for _, r := range c.engine.Recalc(changed) {
		id := c.sheets[r.Sheet]
		raw := c.tree.Node(id).Grid.Cell(r.Row, r.Col)
		if r.Value.Type == formula.TypeError {
			f.setCell(id, r.Row, r.Col, "e", r.Value.Str)
			if bytes.Contains(raw, []byte(`"v":`)) {
				f.setCell(id, r.Row, r.Col, "v", nil)
			}
			continue
		}
		if bytes.Contains(raw, []byte(`"e":`)) {
			f.setCell(id, r.Row, r.Col, "e", nil)
		}
		var v any
		switch r.Value.Type {
		case formula.TypeNumber:
			v = json.RawMessage(number(r.Value.Num))
		case formula.TypeText:
			v = r.Value.Str
		case formula.TypeBool:
			v = r.Value.Num != 0
		}
		f.setCell(id, r.Row, r.Col, "v", v)
	}
}

// The workbook as the engine reads it.

func (c *Calc) grid(sheet int) *ot.Grid {
	if sheet < 0 || sheet >= len(c.sheets) {
		return nil
	}
	if n := c.tree.Node(c.sheets[sheet]); n != nil {
		return n.Grid
	}
	return nil
}

func (c *Calc) Value(p formula.Pos) formula.Value {
	g := c.grid(p.Sheet)
	if g == nil {
		return formula.Value{}
	}
	return valueOf(g.Cell(p.Row, p.Col))
}

func (c *Calc) Each(a formula.Area3, f func(row, col int, v formula.Value) bool) {
	g := c.grid(a.Sheet)
	if g == nil {
		return
	}
	rows, _ := c.Size(a.Sheet)
	g.EachIn(max(a.R1, 1), min(a.R2, rows), max(a.C1, 1), a.C2, func(row, col int, raw json.RawMessage) bool {
		return f(row, col, valueOf(raw))
	})
}

func (c *Calc) Size(sheet int) (int, int) {
	if b, ok := c.bounds[sheet]; ok {
		return b[0], b[1]
	}
	g := c.grid(sheet)
	if g == nil {
		return 0, 0
	}
	rows, cols := g.Bounds()
	c.bounds[sheet] = [2]int{rows, cols}
	return rows, cols
}

// Hidden tells whether a row is hidden, and whether by the sheet's filter.
func (c *Calc) Hidden(sheet, row int) (hidden, filtered bool) {
	g := c.grid(sheet)
	if g == nil || !bytes.Contains(g.Cell(row, 0), []byte(`"hide":true`)) {
		return false, false
	}
	a, ok := c.filters[sheet]
	if !ok {
		var f AutoFilter
		if raw := c.tree.Node(c.sheets[sheet]).Attrs[filterKey]; raw != nil && json.Unmarshal(raw, &f) == nil && len(f.Cols) > 0 {
			if area, ok := formula.ParseArea(f.Ref); ok {
				a = &area
			}
		}
		c.filters[sheet] = a
	}
	return true, a != nil && a.R1 < row && row <= a.R2
}

func (c *Calc) Sheets(first, last string) ([]int, bool) {
	find := func(name string) int {
		for h, id := range c.sheets {
			if strings.EqualFold(c.names[id], name) {
				return h
			}
		}
		return -1
	}
	i := find(first)
	if i < 0 {
		return nil, false
	}
	if last == "" {
		return []int{i}, true
	}
	j := find(last)
	if j < 0 {
		return nil, false
	}
	var out []int
	for h := min(i, j); h <= max(i, j); h++ {
		out = append(out, h)
	}
	return out, true
}

func (c *Calc) Name(name string, sheet int) (string, bool) {
	global := ""
	found := false
	for _, n := range c.defined {
		if !strings.EqualFold(n.Name, name) {
			continue
		}
		if n.Sheet == "" {
			global, found = n.Ref, true
		} else if sheet >= 0 && sheet < len(c.sheets) && n.Sheet == c.sheets[sheet] {
			return n.Ref, true
		}
	}
	return global, found
}

// valueOf is the value of a cell: its error, or its value.
func valueOf(raw json.RawMessage) formula.Value {
	if len(raw) == 0 {
		return formula.Value{}
	}
	var e, v []byte
	for key, val := range members(raw) {
		switch string(key) {
		case "e":
			e = val
		case "v":
			v = val
		}
	}
	if e != nil {
		var s string
		_ = json.Unmarshal(e, &s)
		return formula.Err(s)
	}
	switch {
	case len(v) == 0:
		return formula.Value{}
	case v[0] == '"':
		if bytes.IndexByte(v, '\\') < 0 {
			return formula.Str(string(v[1 : len(v)-1]))
		}
		var s string
		_ = json.Unmarshal(v, &s)
		return formula.Str(s)
	case v[0] == 't':
		return formula.Boolean(true)
	case v[0] == 'f':
		return formula.Boolean(false)
	}
	n, err := strconv.ParseFloat(string(v), 64)
	if err != nil {
		return formula.Value{}
	}
	return formula.Num(n)
}

// members are the keys and values of a JSON object, written compact.
func members(raw []byte) func(func(key, val []byte) bool) {
	return func(yield func(key, val []byte) bool) {
		i := 1
		for i < len(raw) && raw[i] == '"' {
			end := i + 1
			for end < len(raw) && raw[end] != '"' {
				if raw[end] == '\\' {
					end++
				}
				end++
			}
			key := raw[i+1 : end]
			j := end + 2
			k := j
			depth := 0
			for k < len(raw) {
				c := raw[k]
				if c == '"' {
					k++
					for k < len(raw) && raw[k] != '"' {
						if raw[k] == '\\' {
							k++
						}
						k++
					}
				} else if c == '{' || c == '[' {
					depth++
				} else if c == '}' || c == ']' {
					if depth == 0 {
						break
					}
					depth--
				} else if c == ',' && depth == 0 {
					break
				}
				k++
			}
			if k > len(raw) || !yield(key, raw[j:k]) {
				return
			}
			i = k + 1
		}
	}
}
