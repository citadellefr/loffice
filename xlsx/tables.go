package xlsx

import (
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

const tablesKey = "tables"

// A Table is a table of a sheet (a ListObject). Ref is its area, header
// row first and totals row last when it has them. ID is its id in the
// workbook, 0 for a table the server numbers. Style is the table style it
// shows, Rows and Cols its banded rows and columns, First and Last its
// first and last columns set apart; Filter puts arrows on its header.
type Table struct {
	ID       int           `json:"id,omitempty"`
	Name     string        `json:"name"`
	Ref      string        `json:"ref"`
	NoHeader bool          `json:"noHeader,omitempty"`
	Totals   bool          `json:"totals,omitempty"`
	Filter   bool          `json:"filter,omitempty"`
	Style    string        `json:"style,omitempty"`
	Rows     bool          `json:"rows,omitempty"`
	Cols     bool          `json:"cols,omitempty"`
	First    bool          `json:"first,omitempty"`
	Last     bool          `json:"last,omitempty"`
	Columns  []TableColumn `json:"columns"`
}

// A TableColumn is a column of a table: its name, the header of its
// cell in the header row, F the formula of a calculated column, Total the
// function of its totals row (sum, count, custom...), Label the text it
// shows there instead and TotalF the formula of a custom total. ID is its
// id in the table, 0 for one the server numbers.
type TableColumn struct {
	ID     int    `json:"id,omitempty"`
	Name   string `json:"name"`
	F      string `json:"f,omitempty"`
	Total  string `json:"total,omitempty"`
	Label  string `json:"label,omitempty"`
	TotalF string `json:"tf,omitempty"`
}

// tablePart is the part of a table read, and the sheet it belongs to.
type tablePart struct {
	sheet, name, rid string
}

// tables reads the tables of a sheet.
func (r *reader) tables(id, name string) []Table {
	rels, err := r.d.pkg.Relationships(name)
	if err != nil {
		return nil
	}
	var out []Table
	for _, x := range rels {
		if x.Type != relTable || x.External {
			continue
		}
		part, err := opc.Resolve(name, x.Target)
		if err != nil {
			continue
		}
		doc, err := r.part(part)
		if err != nil {
			continue
		}
		t, ok := readTable(doc.Root)
		if !ok || r.d.tableParts[t.ID].name != "" {
			continue
		}
		r.d.tableParts[t.ID] = tablePart{sheet: id, name: part, rid: x.ID}
		out = append(out, t)
	}
	return out
}

func readTable(root *xmldom.Element) (Table, bool) {
	a, ok := formula.ParseArea(root.Get("ref"))
	if root.Local != "table" || !ok || a.Rows || a.Cols {
		return Table{}, false
	}
	t := Table{
		ID: atoi(root.Get("id")), Name: decodeEscapes(root.Get("displayName")), Ref: a.String(),
		NoHeader: root.Get("headerRowCount") == "0", Totals: atoi(root.Get("totalsRowCount")) > 0,
		Filter: root.Child(mainNS, "autoFilter") != nil,
	}
	if t.Name == "" {
		t.Name = decodeEscapes(root.Get("name"))
	}
	if s := root.Child(mainNS, "tableStyleInfo"); s != nil {
		t.Style = s.Get("name")
		t.Rows, t.Cols = truthy(s.Get("showRowStripes")), truthy(s.Get("showColumnStripes"))
		t.First, t.Last = truthy(s.Get("showFirstColumn")), truthy(s.Get("showLastColumn"))
	}
	for _, c := range elements(root.Child(mainNS, "tableColumns"), "tableColumn") {
		col := TableColumn{ID: atoi(c.Get("id")), Name: decodeEscapes(c.Get("name")), Total: c.Get("totalsRowFunction"),
			Label: decodeEscapes(c.Get("totalsRowLabel"))}
		if f := c.Child(mainNS, "calculatedColumnFormula"); f != nil {
			col.F = f.Text()
		}
		if f := c.Child(mainNS, "totalsRowFormula"); f != nil {
			col.TotalF = f.Text()
		}
		t.Columns = append(t.Columns, col)
	}
	if t.ID <= 0 || t.Name == "" || len(t.Columns) != a.C2-a.C1+1 {
		return Table{}, false
	}
	return t, true
}

// area is the area of a table.
func (t *Table) area() formula.Area {
	a, _ := formula.ParseArea(t.Ref)
	return a
}

// body is the first and last rows of the data of a table.
func (t *Table) body() (int, int) {
	a := t.area()
	first, last := a.R1, a.R2
	if !t.NoHeader {
		first++
	}
	if t.Totals {
		last--
	}
	return first, last
}

// shifted is the table once rows or columns are inserted or removed on its
// sheet, named sheet: false when it is gone. Columns inserted inside it
// become columns of the table, named after the others; a totals row
// removed is no longer one.
func (t Table) shifted(sheet string, rows bool, at, n int) (Table, bool) {
	before := t.area()
	ref := shiftRef(t.Ref, sheet, rows, at, n)
	if ref == "" {
		return t, false
	}
	t.Ref = ref
	shift := func(f string) string {
		if g, err := formula.Shift(f, sheet, true, rows, at, n); err == nil {
			return g
		}
		return f
	}
	cols := make([]TableColumn, 0, len(t.Columns))
	for i, c := range t.Columns {
		col := before.C1 + i
		if !rows && n < 0 && col >= at && col < at-n {
			continue
		}
		if !rows && n > 0 && col == at && at > before.C1 {
			for range n {
				cols = append(cols, TableColumn{Name: freeColumn(t.Columns, cols)})
			}
		}
		c.F, c.TotalF = shift(c.F), shift(c.TotalF)
		cols = append(cols, c)
	}
	t.Columns = cols
	if rows && n < 0 && t.Totals && before.R2 >= at && before.R2 < at-n {
		t.Totals = false
	}
	return t, len(cols) > 0
}

// freeColumn is a name for a new column, Colonne and a number no column
// of a table or of those taken has.
func freeColumn(cols, taken []TableColumn) string {
	return freeName("Colonne", func(name string) bool {
		has := func(c TableColumn) bool { return strings.EqualFold(c.Name, name) }
		return slices.ContainsFunc(cols, has) || slices.ContainsFunc(taken, has)
	})
}

// freeName is base and the first number from 1 that makes a name taken
// does not hold.
func freeName(base string, taken func(string) bool) string {
	for i := 1; ; i++ {
		if name := base + strconv.Itoa(i); !taken(name) {
			return name
		}
	}
}
