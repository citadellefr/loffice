package xlsx

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// totalCodes are the SUBTOTAL functions of the totals row, by its
// function.
var totalCodes = map[string]int{"average": 101, "countNums": 102, "count": 103, "max": 104, "min": 105, "stdDev": 107,
	"sum": 109, "var": 110}

// readTables notes the tables of the sheets, as the engine reads them.
func (c *Calc) readTables() {
	c.tables, c.defs = map[string][]Table{}, nil
	for h, id := range c.sheets {
		n := c.tree.Node(id)
		var tables []Table
		if n == nil || json.Unmarshal(n.Attrs[tablesKey], &tables) != nil {
			continue
		}
		c.tables[id] = tables
		for _, t := range tables {
			c.tableID = max(c.tableID, t.ID)
			c.defs = append(c.defs, t.def(h))
		}
	}
}

// def is a table as structured references read it.
func (t *Table) def(sheet int) formula.TableDef {
	d := formula.TableDef{Name: t.Name, Sheet: sheet, Area: t.area(), Header: !t.NoHeader, Totals: t.Totals}
	for _, c := range t.Columns {
		d.Columns = append(d.Columns, c.Name)
	}
	return d
}

func (c *Calc) Table(name string) (formula.TableDef, bool) {
	for _, d := range c.defs {
		if strings.EqualFold(d.Name, name) {
			return d, true
		}
	}
	return formula.TableDef{}, false
}

func (c *Calc) TableAt(sheet, row, col int) (formula.TableDef, bool) {
	for _, d := range c.defs {
		if d.Sheet == sheet && d.Area.Contains(row, col) {
			return d, true
		}
	}
	return formula.TableDef{}, false
}

// shiftTables moves the tables of a sheet with rows or columns inserted
// or removed. Rows inserted inside a table take the formulas of its
// calculated columns when fill is set.
func (f *follow) shiftTables(id, sheet string, rows bool, at, n int, fill bool) {
	var tables []Table
	if f.attr(id, tablesKey, &tables); len(tables) == 0 {
		return
	}
	kept := make([]Table, 0, len(tables))
	for _, t := range tables {
		g, ok := t.shifted(sheet, rows, at, n)
		if !ok {
			f.goneTables = append(f.goneTables, t.Name)
			continue
		}
		if first, last := g.body(); fill && rows && n > 0 {
			f.fill(id, &g, max(at, first), min(at+n-1, last))
		}
		kept = append(kept, g)
	}
	setList(f, id, tablesKey, kept)
}

// fill gives the rows from r1 to r2 of a table the formulas of its
// calculated columns, where they are blank.
func (f *follow) fill(id string, t *Table, r1, r2 int) {
	g := f.c.tree.Node(id).Grid
	a := t.area()
	first, _ := t.body()
	for i, col := range t.Columns {
		if col.F == "" {
			continue
		}
		for r := r1; r <= r2; r++ {
			if g != nil && len(g.Cell(r, a.C1+i)) > 2 {
				continue
			}
			if text, err := formula.Translate(col.F, r-first, 0); err == nil {
				f.setCell(id, r, a.C1+i, "f", text)
			}
		}
	}
}

// tables follows the tables an edit changed: those added are numbered,
// their headers hold the names of their columns, cells typed in a header
// rename its column, a totals row shown or hidden takes a row or gives it
// back, rows added at their end take the formulas of their calculated
// columns, and a formula typed in a blank column makes it calculated.
// Formulas follow the tables and columns renamed; those of a table taken
// away read its cells, and those of a table removed become #REF!.
func (f *follow) tables(e ot.Edit) {
	c := f.c
	typed := map[string]map[[2]int]json.RawMessage{}
	for _, ch := range e {
		if ch.Op != ot.OpCel && !(ch.Op == ot.OpNew && ch.Cells != nil) {
			continue
		}
		if typed[ch.ID] == nil {
			typed[ch.ID] = map[[2]int]json.RawMessage{}
		}
		for _, cell := range ch.Cells {
			typed[ch.ID][[2]int{cell.Row, cell.Col}] = cell.Fields
		}
	}
	var renames []tableRename
	var dropped []droppedTable
	news := map[string][]Table{}
	for _, n := range c.tree.Children("book") {
		if n.Type != "sheet" {
			continue
		}
		var tables []Table
		if f.attr(n.ID, tablesKey, &tables); len(tables)+len(c.tables[n.ID]) == 0 {
			continue
		}
		old := map[int]Table{}
		for _, t := range c.tables[n.ID] {
			old[t.ID] = t
		}
		for i := range tables {
			t := &tables[i]
			if t.ID == 0 {
				c.tableID++
				t.ID = c.tableID
			}
			o, had := old[t.ID]
			delete(old, t.ID)
			if had && o.Totals != t.Totals && o.Ref == t.Ref {
				f.totals(n.ID, t)
			}
			if first, last := t.body(); had && !f.moved && o.area().R1 == t.area().R1 && o.area().R2 < t.area().R2 {
				f.fill(n.ID, t, max(first, o.area().R2+1-boolInt(o.Totals)), last)
			}
			f.headers(n, t, typed[n.ID])
			if had {
				renames = append(renames, renamed(o, *t)...)
			}
			f.calculated(n, t, typed[n.ID])
		}
		for _, t := range old {
			if !slices.Contains(f.goneTables, t.Name) {
				dropped = append(dropped, droppedTable{t, n.ID})
			}
		}
		news[n.ID] = tables
	}
	f.c.defs = nil
	for id, tables := range news {
		for _, t := range tables {
			f.c.defs = append(f.c.defs, t.def(c.handles[id]))
		}
	}
	rewrite := f.restructure(renames, dropped)
	if rewrite != nil {
		f.formulasAt(rewrite)
	}
	for id, tables := range news {
		for i := range tables {
			for j := range tables[i].Columns {
				if col := &tables[i].Columns[j]; rewrite != nil {
					col.F, col.TotalF = rewrite(col.F, id, 0, 0), rewrite(col.TotalF, id, 0, 0)
				}
			}
		}
		raw := json.RawMessage("null")
		if len(tables) > 0 {
			raw = mustJSON(tables)
		}
		if before, _ := json.Marshal(c.tables[id]); !bytes.Equal(raw, before) {
			f.set(id, tablesKey, raw)
			f.retable = true
		}
	}
	f.qualify(typed)
	f.retable = f.retable || len(f.goneTables) > 0
}

// boolInt is 1 when b is set.
func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// totals shows the totals row of a table under its body, or hides it.
func (f *follow) totals(id string, t *Table) {
	a := t.area()
	if t.Totals {
		a.R2++
		if t.Columns[len(t.Columns)-1].Total == "" && slices.IndexFunc(t.Columns, func(c TableColumn) bool { return c.Total != "" || c.Label != "" }) < 0 {
			if len(t.Columns) > 1 {
				t.Columns[0].Label = "Total"
			}
			t.Columns[len(t.Columns)-1].Total = "sum"
		}
	} else {
		for i := range t.Columns {
			f.setCell(id, a.R2, a.C1+i, "f", nil)
			f.setCell(id, a.R2, a.C1+i, "v", nil)
			f.setCell(id, a.R2, a.C1+i, "e", nil)
		}
		a.R2--
	}
	t.Ref = a.String()
	if !t.Totals {
		return
	}
	for i, col := range t.Columns {
		ref := formula.Structured{Table: t.Name, First: col.Name}.String()
		switch {
		case col.Total == "custom" && col.TotalF != "":
			f.setCell(id, a.R2, a.C1+i, "f", col.TotalF)
		case totalCodes[col.Total] > 0:
			f.setCell(id, a.R2, a.C1+i, "f", "SUBTOTAL("+strconv.Itoa(totalCodes[col.Total])+","+ref+")")
		case col.Label != "":
			f.setCell(id, a.R2, a.C1+i, "v", col.Label)
		}
	}
}

// headers keeps the header of a table holding the names of its columns:
// a cell typed there renames its column, others take the name of theirs.
func (f *follow) headers(n *ot.Node, t *Table, typed map[[2]int]json.RawMessage) {
	if t.NoHeader {
		return
	}
	a := t.area()
	renamed := make([]bool, len(t.Columns))
	for i := range t.Columns {
		at := [2]int{a.R1, a.C1 + i}
		if _, ok := typed[at]; ok {
			t.Columns[i].Name = valueOf(n.Grid.Cell(at[0], at[1])).String()
			renamed[i] = true
		}
	}
	for i := range t.Columns {
		name := strings.TrimSpace(t.Columns[i].Name)
		taken := func(s string) bool {
			for j, c := range t.Columns {
				if j != i && (j < i || !renamed[j]) && strings.EqualFold(c.Name, s) {
					return true
				}
			}
			return false
		}
		switch {
		case name == "":
			name = numbered("Colonne", 1, taken)
		case taken(name):
			name = numbered(name, 2, taken)
		}
		t.Columns[i].Name = name
		var fields cellFields
		_ = json.Unmarshal(n.Grid.Cell(a.R1, a.C1+i), &fields)
		if fields.F != "" || fields.E != "" || string(fields.V) != string(mustJSON(name)) {
			f.setCell(n.ID, a.R1, a.C1+i, "v", name)
			if fields.F != "" {
				f.setCell(n.ID, a.R1, a.C1+i, "f", nil)
			}
			if fields.E != "" {
				f.setCell(n.ID, a.R1, a.C1+i, "e", nil)
			}
		}
		if t.Columns[i].ID == 0 {
			t.Columns[i].ID = slices.MaxFunc(t.Columns, func(x, y TableColumn) int { return x.ID - y.ID }).ID + 1
		}
	}
}

// calculated makes a formula typed in a column of a table whose other
// cells are blank the formula of the column, which all its rows take.
func (f *follow) calculated(n *ot.Node, t *Table, typed map[[2]int]json.RawMessage) {
	a := t.area()
	first, last := t.body()
	for at, fields := range typed {
		if at[0] < first || at[0] > last || at[1] < a.C1 || at[1] > a.C2 || !typedFormula(fields) {
			continue
		}
		col := &t.Columns[at[1]-a.C1]
		if col.F != "" {
			continue
		}
		var cell cellFields
		_ = json.Unmarshal(n.Grid.Cell(at[0], at[1]), &cell)
		blank := true
		n.Grid.EachIn(first, last, at[1], at[1], func(row, _ int, raw json.RawMessage) bool {
			blank = row == at[0] || len(raw) <= 2
			return blank
		})
		if !blank || cell.F == "" {
			continue
		}
		text, err := formula.Translate(f.qualified(n.ID, at[0], at[1], cell.F), first-at[0], 0)
		if err != nil {
			continue
		}
		col.F = text
		f.fill(n.ID, t, first, last)
	}
}

// A tableRename is a table, or a column of it when column is set, whose
// name changed.
type tableRename struct {
	table, column, to string
}

// renamed are the names of a table and its columns changed from o to t.
func renamed(o, t Table) []tableRename {
	var out []tableRename
	for _, c := range t.Columns {
		for _, d := range o.Columns {
			if c.ID == d.ID && c.Name != d.Name {
				out = append(out, tableRename{table: o.Name, column: d.Name, to: c.Name})
			}
		}
	}
	if o.Name != t.Name {
		out = append(out, tableRename{table: o.Name, to: t.Name})
	}
	return out
}

// A droppedTable is a table taken away from a sheet, its cells kept.
type droppedTable struct {
	Table
	sheet string
}

// restructure is what rewrites the structured references of a formula,
// in a cell of a sheet or elsewhere (row 0), once tables and columns were
// renamed, tables dropped and others removed with their cells; nil when
// nothing changed.
func (f *follow) restructure(renames []tableRename, dropped []droppedTable) func(text, sheet string, row, col int) string {
	if len(renames)+len(dropped)+len(f.goneTables) == 0 {
		return nil
	}
	c := f.c
	return func(text, sheet string, row, _ int) string {
		if !strings.Contains(text, "[") && len(dropped)+len(f.goneTables) == 0 {
			return text
		}
		g, err := formula.RewriteStructured(text, func(s *formula.Structured) bool {
			table := s.Table
			changed := false
			for _, r := range renames {
				if r.column != "" && strings.EqualFold(table, r.table) {
					for _, p := range []*string{&s.First, &s.Last} {
						if strings.EqualFold(*p, r.column) {
							*p, changed = r.to, true
						}
					}
				}
			}
			for _, r := range renames {
				if r.column == "" && strings.EqualFold(table, r.table) {
					s.Table, changed = r.to, true
				}
			}
			for _, name := range f.goneTables {
				if strings.EqualFold(table, name) {
					s.Text, changed = "#REF!", true
				}
			}
			for _, t := range dropped {
				if !strings.EqualFold(table, t.Name) {
					continue
				}
				s.Text, changed = "#REF!", true
				if a, err := t.def(0).Resolve(*s, row); err == nil {
					r := formula.Ref{Area: a}
					if t.sheet != sheet {
						r.Sheet = c.names[t.sheet]
					}
					s.Text = r.String()
				}
			}
			return changed
		})
		if err != nil {
			return text
		}
		return g
	}
}

// qualify writes the structured references of the formulas typed as files
// hold them: Table1[[#This Row],[Price]] for [@Price] typed in Table1.
func (f *follow) qualify(typed map[string]map[[2]int]json.RawMessage) {
	for id, cells := range typed {
		for at, fields := range cells {
			if !typedFormula(fields) || !bytes.Contains(fields, []byte("[")) {
				continue
			}
			var cell cellFields
			_ = json.Unmarshal(fields, &cell)
			if g := f.qualified(id, at[0], at[1], cell.F); g != cell.F {
				f.setCell(id, at[0], at[1], "f", g)
			}
		}
	}
}

// qualified is a formula of a cell with its structured references
// qualified.
func (f *follow) qualified(id string, row, col int, text string) string {
	h, ok := f.c.handles[id]
	if !ok {
		return text
	}
	g, err := formula.RewriteStructured(text, func(s *formula.Structured) bool {
		if s.Table == "" {
			d, ok := f.c.TableAt(h, row, col)
			if !ok {
				return false
			}
			s.Table = d.Name
		}
		return true
	})
	if err != nil {
		return text
	}
	return g
}
