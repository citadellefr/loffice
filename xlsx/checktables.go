package xlsx

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// maxTableName bounds the names of tables and of their columns.
const maxTableName = 255

var totalFunctions = []string{"", "none", "sum", "min", "max", "average", "count", "countNums", "stdDev", "var", "custom"}

// builtinStyles are the table styles of Excel, by the start of their names
// and how many it has.
var builtinStyles = []struct {
	prefix string
	n      int
}{{"TableStyleLight", 21}, {"TableStyleMedium", 28}, {"TableStyleDark", 11}}

// tableView is what the workbook holds once an edit is applied: the tables
// of each sheet and the names defined.
type tableView struct {
	tables map[string][]Table
	names  []Name
}

// viewTables reads the tables of the sheets and the names of the book as
// the edit leaves them, false if any is unreadable. A table whose totals
// row is turned on or off in the same area takes the area Follow gives it.
func viewTables(tree *ot.Tree, e ot.Edit) (*tableView, bool) {
	tables := map[string]json.RawMessage{}
	before := map[string][]Table{}
	for _, n := range tree.Children("book") {
		if n.Type == "sheet" {
			tables[n.ID] = n.Attrs[tablesKey]
			var list []Table
			if raw := n.Attrs[tablesKey]; len(raw) > 0 && json.Unmarshal(raw, &list) != nil {
				list = nil // an unreadable list before gives no table to follow
			}
			before[n.ID] = list
		}
	}
	var names json.RawMessage
	if book := tree.Node("book"); book != nil {
		names = book.Attrs["names"]
	}
	for _, c := range e {
		switch {
		case c.Op == ot.OpDel:
			delete(tables, c.ID)
		case c.Op == ot.OpNew && c.Type == "sheet", c.Op == ot.OpSet && c.ID != "book":
			if raw, ok := c.Attrs[tablesKey]; ok {
				tables[c.ID] = raw
			}
		case c.Op == ot.OpSet:
			if raw, ok := c.Attrs["names"]; ok {
				names = raw
			}
		}
	}
	v := &tableView{tables: map[string][]Table{}}
	for id, raw := range tables {
		var list []Table
		if len(raw) > 0 && string(raw) != "null" && json.Unmarshal(raw, &list) != nil {
			return nil, false
		}
		for i := range list {
			t := &list[i]
			if j := slices.IndexFunc(before[id], func(o Table) bool { return t.ID > 0 && o.ID == t.ID }); j >= 0 {
				if o := before[id][j]; o.Totals != t.Totals && o.Ref == t.Ref {
					a := t.area()
					a.R2 += 2*boolInt(t.Totals) - 1
					t.Ref = a.String()
				}
			}
		}
		v.tables[id] = list
	}
	if len(names) > 0 && string(names) != "null" && json.Unmarshal(names, &v.names) != nil {
		return nil, false
	}
	return v, true
}

// checkTableList tells whether the tables of a sheet are tables Excel
// takes, unlike those of the other sheets in name and id.
func (d *Document) checkTableList(sheet string, v *tableView) bool {
	taken := map[string]bool{}
	ids := map[int]bool{}
	for _, n := range v.names {
		taken[strings.ToLower(n.Name)] = true
	}
	for id, list := range v.tables {
		if id == sheet {
			continue
		}
		for _, t := range list {
			taken[strings.ToLower(t.Name)] = true
			if t.ID > 0 {
				ids[t.ID] = true
			}
		}
	}
	styles := d.loadedTableStyles()
	var areas []formula.Area
	for _, t := range v.tables[sheet] {
		name := strings.ToLower(t.Name)
		if taken[name] || t.ID < 0 || ids[t.ID] || !d.checkTable(sheet, t, styles) {
			return false
		}
		a := t.area()
		if slices.ContainsFunc(areas, func(b formula.Area) bool { return overlaps(a, b) }) {
			return false
		}
		areas = append(areas, a)
		taken[name] = true
		if t.ID > 0 {
			ids[t.ID] = true
		}
	}
	return true
}

// checkTable tells whether a table is one Excel takes, whatever the other
// tables are.
func (d *Document) checkTable(sheet string, t Table, styles map[string]bool) bool {
	a, ok := formula.ParseArea(t.Ref)
	if !ok || a.Rows || a.Cols || !validTableName(t.Name) {
		return false
	}
	if part, read := d.tableParts[t.ID]; read && part.sheet != sheet {
		return false
	}
	rows := a.R2 - a.R1 + 1
	if !t.NoHeader {
		rows--
	}
	if t.Totals {
		rows--
	}
	if rows < 1 || len(t.Columns) != a.C2-a.C1+1 || t.Style != "" && !builtinStyle(t.Style) && !styles[t.Style] {
		return false
	}
	names := map[string]bool{}
	for _, c := range t.Columns {
		key := strings.ToLower(c.Name)
		if c.ID < 0 || c.Name == "" || utf16Len(c.Name) > maxTableName || names[key] || !slices.Contains(totalFunctions, c.Total) ||
			utf16Len(c.Label) > maxText || !validFormula(c.F) || !validFormula(c.TotalF) {
			return false
		}
		names[key] = true
	}
	return true
}

// validTableName tells whether a name is one Excel takes for a table: a
// defined name, and not R or C, which stand for a row and a column.
func validTableName(s string) bool {
	return validName(s) && !strings.EqualFold(s, "r") && !strings.EqualFold(s, "c")
}

// validFormula tells whether a formula is empty or one that reads.
func validFormula(f string) bool {
	if f == "" {
		return true
	}
	_, err := formula.Tokens(f)
	return err == nil && len(f) <= maxFormula
}

// builtinStyle tells whether Excel has a table style of that name.
func builtinStyle(s string) bool {
	for _, b := range builtinStyles {
		if rest, ok := strings.CutPrefix(s, b.prefix); ok {
			n, err := strconv.Atoi(rest)
			return err == nil && n >= 1 && n <= b.n && rest == strconv.Itoa(n)
		}
	}
	return false
}

// loadedTableStyles are the table styles the tables of the workbook read
// show: the styles it defines itself.
func (d *Document) loadedTableStyles() map[string]bool {
	styles := map[string]bool{}
	for _, n := range d.loaded.Children("book") {
		var tables []Table
		_ = json.Unmarshal(n.Attrs[tablesKey], &tables)
		for _, t := range tables {
			styles[t.Style] = true
		}
	}
	return styles
}

// overlaps tells whether two areas share a cell.
func overlaps(a, b formula.Area) bool {
	return a.R1 <= b.R2 && b.R1 <= a.R2 && a.C1 <= b.C2 && b.C1 <= a.C2
}
