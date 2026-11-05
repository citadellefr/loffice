package xlsx

import (
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

var (
	// ErrChartTable is a table of data a chart cannot take.
	ErrChartTable = errors.New("xlsx: this table does not fit the data of the chart")
	// ErrChartCells is a table of data that would write over other cells
	// of the workbook.
	ErrChartCells = errors.New("xlsx: the cells next to the data of this chart hold something else")
	// ErrChartLayout is a chart whose series do not read a table of cells
	// of the workbook.
	ErrChartLayout = errors.New("xlsx: the series of this chart do not read a table of cells")
)

var null = json.RawMessage("null")

// A ChartTable is the data of a chart as its data pane shows them. A cell
// is a number, a text or null.
type ChartTable struct {
	// Categories are nil when the series have none.
	Categories []any         `json:"categories,omitempty"`
	Series     []ChartColumn `json:"series"`
}

// ChartColumn is a series of a table: the one of its index in the chart,
// or a new one under an index the chart has not.
type ChartColumn struct {
	Index  int    `json:"index"`
	Name   string `json:"name,omitempty"`
	Values []any  `json:"values"`
}

// A ChartBook is the workbook embedded with a chart, whose series read a
// table of its cells. It is not changed: a table gives the workbook and
// the chart as they become.
type ChartBook struct {
	doc  *Document
	tree *ot.Tree
	own  string
	// table is what the workbook holds, cats the cells of its categories,
	// lines those of each series by index, in the order of the chart.
	table *ChartTable
	cats  *cellLine
	lines map[int]seriesCells
	order []int
}

// seriesCells are the cells of a series; name is nil for one named in the
// chart or not at all.
type seriesCells struct {
	val  cellLine
	name *cellLine
}

// A cellLine is a row or column of cells of a sheet.
type cellLine struct {
	ref      string
	sheet    *ot.Node
	row, col int
	n        int
	down     bool
}

func (l cellLine) at(i int) (row, col int) {
	if l.down {
		return l.row + i, l.col
	}
	return l.row, l.col + i
}

// across is the line k rows or columns over, the way series follow one
// another.
func (l cellLine) across(k int) cellLine {
	if l.down {
		l.col += k
	} else {
		l.row += k
	}
	l.ref = ""
	return l
}

// within tells whether the n first cells of the line are on a sheet.
func (l cellLine) within(n int) bool {
	row, col := l.at(n - 1)
	return n > 0 && row <= formula.MaxRows && col <= formula.MaxCols
}

// reference is the line as a reference, n cells long.
func (l cellLine) reference(n int) string {
	if n == l.n && l.ref != "" {
		return l.ref
	}
	row, col := l.at(n - 1)
	return absolute(formula.Ref{Sheet: str(l.sheet, "name"), Area: formula.Area{R1: l.row, C1: l.col, R2: row, C2: col}})
}

func (l cellLine) cell(tree *ot.Tree, i int) json.RawMessage {
	n := tree.Node(l.sheet.ID)
	if n.Grid == nil {
		return nil
	}
	return n.Grid.Cell(l.at(i))
}

// line reads a reference of a chart as a line of cells of the workbook.
func (b *ChartBook) line(ref string) (cellLine, bool) {
	r, ok := chartRef(ref, b.own)
	if !ok {
		return cellLine{}, false
	}
	n := sheetNamed(b.tree, r.Sheet)
	a := r.Area
	if n == nil || a.R1 != a.R2 && a.C1 != a.C2 || (a.R2-a.R1+1)*(a.C2-a.C1+1) > maxChartCells {
		return cellLine{}, false
	}
	return cellLine{ref: ref, sheet: n, row: a.R1, col: a.C1, n: (a.R2 - a.R1 + 1) * (a.C2 - a.C1 + 1), down: a.C1 == a.C2}, true
}

// OpenChartBook reads a workbook embedded with c, as a table.
// ErrChartLayout tells a chart whose series read anything else: another
// workbook, cells apart, or sizes, categories or directions that differ.
func OpenChartBook(data []byte, c *chart.Chart, limits Limits) (*ChartBook, error) {
	doc, tree, err := Open(data, limits)
	if err != nil {
		return nil, err
	}
	b := &ChartBook{doc: doc, tree: tree, lines: map[int]seriesCells{}}
	for _, n := range tree.Children("book") {
		if n.Type == "sheet" {
			b.own = str(n, "name")
			break
		}
	}
	if !b.lay(c) {
		return nil, ErrChartLayout
	}
	b.read(c)
	return b, nil
}

// lay finds the cells of the series of c.
func (b *ChartBook) lay(c *chart.Chart) bool {
	if c.Pivot {
		return false
	}
	var first *cellLine
	for _, p := range c.Plots {
		for _, s := range p.Series {
			if _, dup := b.lines[s.Index]; dup || s.Val == nil || s.Size != nil || s.Index < 0 {
				return false
			}
			val, ok := b.line(s.Val.Ref)
			if !ok {
				return false
			}
			cells := seriesCells{val: val}
			if s.Name != nil && s.Name.Ref != "" {
				name, ok := b.line(s.Name.Ref)
				if !ok || name.n != 1 {
					return false
				}
				if val.n == 1 && name.row == val.row && name.col < val.col {
					cells.val.down = false
				}
				name.down = cells.val.down
				cells.name = &name
			}
			if first == nil {
				first = &cells.val
				if !b.layCategories(s, val.n, cells.val.down) {
					return false
				}
			} else if val.n != first.n || cells.val.down != first.down || !b.sameCategories(s) {
				return false
			}
			b.lines[s.Index] = cells
			b.order = append(b.order, s.Index)
		}
	}
	return first != nil
}

func (b *ChartBook) layCategories(s chart.Series, n int, down bool) bool {
	if s.Cat == nil {
		return true
	}
	if s.Cat.Levels != nil {
		return false
	}
	cats, ok := b.line(s.Cat.Ref)
	if !ok || cats.n != n {
		return false
	}
	if n == 1 {
		cats.down = down
	}
	b.cats = &cats
	return true
}

func (b *ChartBook) sameCategories(s chart.Series) bool {
	if b.cats == nil || s.Cat == nil {
		return b.cats == nil && s.Cat == nil
	}
	cats, ok := b.line(s.Cat.Ref)
	return ok && s.Cat.Levels == nil && cats.n == b.cats.n && cats.sheet == b.cats.sheet && cats.row == b.cats.row && cats.col == b.cats.col
}

// read gives the book the table its cells hold, named by c where no cell
// does.
func (b *ChartBook) read(c *chart.Chart) {
	var columns []ChartColumn
	for _, p := range c.Plots {
		for _, s := range p.Series {
			column := ChartColumn{Index: s.Index}
			if s.Name != nil && len(s.Name.Str) > 0 {
				column.Name = s.Name.Str[0]
			}
			columns = append(columns, column)
		}
	}
	b.table = b.tableIn(b.tree, columns, b.lines, b.lines[b.order[0]].val.n)
}

// tableIn is the table of rows rows the cells of a tree hold, for the series
// of columns.
func (b *ChartBook) tableIn(tree *ot.Tree, columns []ChartColumn, lines map[int]seriesCells, rows int) *ChartTable {
	cells := newChartCells(tree)
	_, general := cells.format("")
	t := &ChartTable{}
	if b.cats != nil {
		for i := range rows {
			t.Categories = append(t.Categories, category(cells, b.cats.cell(tree, i), general))
		}
	}
	for _, column := range columns {
		l := lines[column.Index]
		if l.name != nil {
			column.Name = cells.text(valueOf(l.name.cell(tree, 0)), general)
		}
		column.Values = nil
		for i := range rows {
			column.Values = append(column.Values, tableValue(cells, l.val.cell(tree, i), general))
		}
		t.Series = append(t.Series, column)
	}
	return t
}

func tableValue(cells *chartCells, raw json.RawMessage, general *formula.Format) any {
	v := valueOf(raw)
	switch v.Type {
	case formula.TypeBlank:
		return nil
	case formula.TypeNumber:
		return v.Num
	}
	return cells.text(v, general)
}

// category is a cell as a category: as it shows when it has a number
// format, a date above all.
func category(cells *chartCells, raw json.RawMessage, general *formula.Format) any {
	var fields struct {
		S string `json:"s"`
	}
	_ = json.Unmarshal(raw, &fields)
	v := valueOf(raw)
	if code, f := cells.format(fields.S); code != "General" && v.Type == formula.TypeNumber {
		return cells.text(v, f)
	}
	return tableValue(cells, raw, general)
}

// Table is what the workbook holds, as a table.
func (b *ChartBook) Table() *ChartTable { return b.table }

// Same tells whether t is the table of the workbook.
func (b *ChartBook) Same(t *ChartTable) bool { return reflect.DeepEqual(t, b.table) }

// Check tells whether the workbook and the chart can take t.
func (b *ChartBook) Check(t *ChartTable) error {
	if len(t.Series) == 0 || len(t.Series) > maxChartSeries || (b.cats == nil) != (t.Categories == nil) {
		return ErrChartTable
	}
	rows := len(t.Series[0].Values)
	if rows == 0 || rows > maxChartCells || b.cats != nil && len(t.Categories) != rows || !tableCells(t.Categories) {
		return ErrChartTable
	}
	seen := map[int]bool{}
	for _, s := range t.Series {
		if s.Index < 0 || seen[s.Index] || len(s.Values) != rows || !tableCells(s.Values) || !tableText(s.Name) {
			return ErrChartTable
		}
		seen[s.Index] = true
	}
	lines, err := b.placed(t)
	if err != nil {
		return err
	}
	if b.cats != nil && !b.free(*b.cats, rows) {
		return ErrChartCells
	}
	for _, l := range lines {
		if !b.free(l.val, rows) {
			return ErrChartCells
		}
	}
	return nil
}

func tableCells(cells []any) bool {
	return !slices.ContainsFunc(cells, func(c any) bool {
		switch v := c.(type) {
		case nil, float64:
			return false
		case string:
			return !tableText(v)
		}
		return true
	})
}

func tableText(s string) bool { return len(s) <= maxTableText && chartText(s) }

// maxTableText bounds the length of a cell of a table, in bytes: what
// Excel takes is less than a third of it in the worst case.
const maxTableText = 1 << 16

// free tells whether a line can grow to n cells: those it does not read
// yet are on the sheet and blank.
func (b *ChartBook) free(l cellLine, n int) bool {
	if !l.within(n) {
		return false
	}
	for i := l.n; i < n; i++ {
		if !blankCell(l.cell(b.tree, i)) {
			return false
		}
	}
	return true
}

func blankCell(raw json.RawMessage) bool {
	return !holdsFormula(raw) && valueOf(raw).Type == formula.TypeBlank
}

func holdsFormula(raw json.RawMessage) bool {
	var fields struct {
		F string `json:"f"`
	}
	_ = json.Unmarshal(raw, &fields)
	return fields.F != ""
}

// placed are the cells of the series of t by index, the new ones after
// the last of the chart, where the cells are blank.
func (b *ChartBook) placed(t *ChartTable) (map[int]seriesCells, error) {
	out := map[int]seriesCells{}
	rows := len(t.Series[0].Values)
	base := b.lines[b.order[0]]
	for _, i := range b.order[1:] {
		if l := b.lines[i]; l.val.row > base.val.row || l.val.col > base.val.col {
			base = l
		}
	}
	k := 0
	for _, s := range t.Series {
		if l, ok := b.lines[s.Index]; ok {
			out[s.Index] = l
			continue
		}
		for {
			k++
			if k > maxChartSeries {
				return nil, ErrChartCells
			}
			l := seriesCells{val: base.val.across(k)}
			l.val.n = 0
			if base.name != nil {
				name := base.name.across(k)
				l.name = &name
			}
			if !l.val.within(rows) || l.name != nil && !l.name.within(1) {
				return nil, ErrChartCells
			}
			if b.free(l.val, rows) && (l.name == nil || blankCell(l.name.cell(b.tree, 0))) {
				out[s.Index] = l
				break
			}
		}
	}
	return out, nil
}

// edit is the workbook with t in its cells, the cells of its series and
// the table it then holds. A cell with a formula is written over only when
// t changes the value read and the one the formula gives once calculated.
func (b *ChartBook) edit(t *ChartTable) (*ot.Tree, map[int]seriesCells, *ChartTable, error) {
	lines, err := b.placed(t)
	if err != nil {
		return nil, nil, nil, err
	}
	rows := len(t.Series[0].Values)
	tree := b.tree.Clone()
	calc := NewCalc(tree, formula.Options{})
	if err := applied(tree, calc, b.changes(t, b.table, lines, false)); err != nil {
		return nil, nil, nil, err
	}
	calculated := b.tableIn(tree, t.Series, lines, rows)
	if err := applied(tree, calc, b.changes(typed(t, b.table, calculated), calculated, lines, true)); err != nil {
		return nil, nil, nil, err
	}
	return tree, lines, b.tableIn(tree, t.Series, lines, rows), nil
}

// typed is t with the cells it holds as read taken from calculated: a
// value the client did not change is the one its formula gives now.
func typed(t, read, calculated *ChartTable) *ChartTable {
	out := &ChartTable{Categories: keep(t.Categories, read.Categories, calculated.Categories)}
	before := map[int]ChartColumn{}
	for _, s := range read.Series {
		before[s.Index] = s
	}
	for i, s := range t.Series {
		was, now := before[s.Index], calculated.Series[i]
		if s.Name == was.Name {
			s.Name = now.Name
		}
		s.Values = keep(s.Values, was.Values, now.Values)
		out.Series = append(out.Series, s)
	}
	return out
}

func keep(want, read, calculated []any) []any {
	out := slices.Clone(want)
	for i := range out {
		if i < len(read) && read[i] == out[i] {
			out[i] = calculated[i]
		}
	}
	return out
}

func applied(tree *ot.Tree, calc *Calc, e ot.Edit) error {
	if err := tree.Apply(e); err != nil {
		return err
	}
	calc.Follow(e, nil)
	return nil
}

// changes are the cells to write for a workbook that holds the table from
// to hold t, those with a formula only when formulas is set.
func (b *ChartBook) changes(t, from *ChartTable, lines map[int]seriesCells, formulas bool) ot.Edit {
	cells := map[string][]ot.Cell{}
	put := func(l cellLine, i int, v any, style json.RawMessage) {
		value, _ := json.Marshal(v)
		fields := ot.Values{"v": value, "f": null, "e": null, "rich": null}
		if style != nil {
			fields["s"] = style
		}
		data, _ := json.Marshal(fields)
		row, col := l.at(i)
		cells[l.sheet.ID] = append(cells[l.sheet.ID], ot.Cell{Row: row, Col: col, Fields: data})
	}
	column := func(l cellLine, read, want []any) {
		for i, v := range want {
			if i < len(read) && read[i] == v || !formulas && holdsFormula(l.cell(b.tree, i)) {
				continue
			}
			var style json.RawMessage
			if i >= l.n && l.n > 0 {
				style = cellStyle(l.cell(b.tree, l.n-1))
			}
			put(l, i, v, style)
		}
		for i := len(want); i < len(read); i++ {
			put(l, i, nil, nil)
		}
	}
	if b.cats != nil {
		column(*b.cats, from.Categories, t.Categories)
	}
	read := map[int]ChartColumn{}
	for _, s := range from.Series {
		read[s.Index] = s
	}
	for _, s := range t.Series {
		l := lines[s.Index]
		column(l.val, read[s.Index].Values, s.Values)
		if l.name != nil && s.Name != read[s.Index].Name && (formulas || !holdsFormula(l.name.cell(b.tree, 0))) {
			put(*l.name, 0, s.Name, nil)
		}
	}
	for _, s := range from.Series {
		if _, kept := lines[s.Index]; !kept {
			l := b.lines[s.Index]
			column(l.val, s.Values, nil)
			if l.name != nil {
				put(*l.name, 0, nil, nil)
			}
		}
	}
	var e ot.Edit
	for _, id := range slices.Sorted(maps.Keys(cells)) {
		e = append(e, ot.Change{Op: ot.OpCel, ID: id, Cells: cells[id]})
	}
	return e
}

func cellStyle(raw json.RawMessage) json.RawMessage {
	var fields struct {
		S json.RawMessage `json:"s"`
	}
	_ = json.Unmarshal(raw, &fields)
	return fields.S
}

// Save is the workbook with t in its cells.
func (b *ChartBook) Save(t *ChartTable) ([]byte, error) {
	tree, _, _, err := b.edit(t)
	if err != nil {
		return nil, err
	}
	return b.doc.Save(tree)
}

// Chart gives c, the chart as it is edited, the series of t: their
// references and their values cached, as the workbook has them once it
// holds t. It returns the table the workbook then holds, whose formulas
// are calculated.
func (b *ChartBook) Chart(t *ChartTable, c *chart.Chart) (*ChartTable, error) {
	tree, lines, table, err := b.edit(t)
	if err != nil {
		return nil, err
	}
	columns := map[int]ChartColumn{}
	for _, s := range t.Series {
		columns[s.Index] = s
	}
	rows := len(t.Series[0].Values)
	var cat *chart.Data
	if b.cats != nil {
		cat = &chart.Data{Ref: b.cats.reference(rows)}
	}
	placed := map[int]bool{}
	order := 0
	for i := range c.Plots {
		p := &c.Plots[i]
		for _, s := range p.Series {
			order = max(order, s.Order+1)
		}
		p.Series = slices.DeleteFunc(p.Series, func(s chart.Series) bool {
			_, kept := columns[s.Index]
			return !kept
		})
		for j := range p.Series {
			s := &p.Series[j]
			placed[s.Index] = true
			if s.Cat != nil && cat != nil {
				cat.Num, cat.Str, cat.Format = s.Cat.Num, s.Cat.Str, s.Cat.Format
			}
			setSeries(s, columns[s.Index], lines[s.Index], rows, cat)
		}
	}
	for _, column := range t.Series {
		if !placed[column.Index] {
			p := &c.Plots[len(c.Plots)-1]
			s := chart.Series{Index: column.Index, Order: order}
			order++
			setSeries(&s, column, lines[column.Index], rows, cat)
			p.Series = append(p.Series, s)
		}
	}
	refresh(c, nil, b.own, newChartCells(tree), nil)
	return table, nil
}

// setSeries points s to its cells.
func setSeries(s *chart.Series, column ChartColumn, cells seriesCells, rows int, cat *chart.Data) {
	switch {
	case cells.name != nil:
		s.Name = &chart.Data{Ref: cells.name.reference(1), Count: 1}
	case column.Name != "":
		s.Name = &chart.Data{Count: 1, Str: []string{column.Name}}
	default:
		s.Name = nil
	}
	if cat != nil {
		d := *cat
		s.Cat = &d
	}
	if s.Val == nil {
		s.Val = &chart.Data{}
	}
	s.Val.Ref = cells.val.reference(rows)
}
