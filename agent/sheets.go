package agent

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

// maxRead is the most cells Range gives at once.
const maxRead = 10000

// SheetInfo is a sheet of a workbook and the area its cells use, in A1
// style, "" when it has none.
type SheetInfo struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Hidden bool   `json:"hidden,omitempty"`
	Used   string `json:"used,omitempty"`
}

// Cell is a cell of a sheet: its value or error, the formula that makes
// it, starting with =, and its number format.
type Cell struct {
	Ref     string `json:"ref"`
	Value   any    `json:"value,omitempty"`
	Error   string `json:"error,omitempty"`
	Formula string `json:"formula,omitempty"`
	Format  string `json:"format,omitempty"`
}

// Range is an area of a sheet, row by row.
type Range struct {
	Sheet string   `json:"sheet"`
	Area  string   `json:"area"`
	Rows  [][]Cell `json:"rows"`
}

// Sheets are the sheets of a workbook.
func (a *Agent) Sheets() (version string, sheets []SheetInfo, err error) {
	err = a.on(a.isSheets, func(s *state) error {
		version = a.version()
		for _, n := range s.tree.Children("book") {
			if n.Type != "sheet" {
				continue
			}
			info := SheetInfo{ID: n.ID, Name: str(n.Attrs, "name"), Hidden: str(n.Attrs, "state") != ""}
			if used, ok := usedArea(n); ok {
				info.Used = areaName(used)
			}
			sheets = append(sheets, info)
		}
		return nil
	})
	return version, sheets, err
}

// Range is the area of the sheet named sheet, in A1 style, the area its
// cells use when empty.
func (a *Agent) Range(sheet, area string) (version string, r Range, err error) {
	err = a.on(a.isSheets, func(s *state) error {
		n, err := s.sheet(sheet)
		if err != nil {
			return err
		}
		at, err := readArea(n, area)
		if err != nil {
			return err
		}
		version = a.version()
		r = Range{Sheet: str(n.Attrs, "name")}
		if at.R2 == 0 {
			return nil
		}
		r.Area = areaName(at)
		r.Rows = make([][]Cell, at.R2-at.R1+1)
		for i := range r.Rows {
			r.Rows[i] = make([]Cell, at.C2-at.C1+1)
			for j := range r.Rows[i] {
				r.Rows[i][j].Ref = formula.CellName(at.R1+i, at.C1+j)
			}
		}
		formats := map[string]string{}
		n.Grid.EachIn(at.R1, at.R2, at.C1, at.C2, func(row, col int, fields json.RawMessage) bool {
			r.Rows[row-at.R1][col-at.C1] = s.read(row, col, fields, formats)
			return true
		})
		return nil
	})
	return version, r, err
}

// sheet finds a sheet by name, as Excel does regardless of case.
func (s *state) sheet(name string) (*ot.Node, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: name the sheet", ErrInvalid)
	}
	var names []string
	for _, n := range s.tree.Children("book") {
		if n.Type != "sheet" {
			continue
		}
		if strings.EqualFold(str(n.Attrs, "name"), name) {
			return n, nil
		}
		names = append(names, strconv.Quote(str(n.Attrs, "name")))
	}
	return nil, fmt.Errorf("%w: no sheet %q, the sheets are %s", ErrGone, name, strings.Join(names, ", "))
}

// usedArea is the area the cells of a sheet use, without its rows and
// columns settings.
func usedArea(n *ot.Node) (used formula.Area, ok bool) {
	if n.Grid == nil {
		return used, false
	}
	n.Grid.Each(1, formula.MaxRows, func(row, col int, _ json.RawMessage) bool {
		if col == 0 {
			return true
		}
		if !ok {
			used, ok = formula.Area{R1: row, C1: col, R2: row, C2: col}, true
		}
		used.R1, used.C1, used.R2, used.C2 = min(used.R1, row), min(used.C1, col), max(used.R2, row), max(used.C2, col)
		return true
	})
	return used, ok
}

func areaName(a formula.Area) string {
	if a.R1 == a.R2 && a.C1 == a.C2 {
		return formula.CellName(a.R1, a.C1)
	}
	return formula.CellName(a.R1, a.C1) + ":" + formula.CellName(a.R2, a.C2)
}

// readArea is the area of a sheet to read: text, whole columns and rows
// cut to the used area, which is all when text is empty. R2 is 0 when
// there is nothing to read.
func readArea(n *ot.Node, text string) (formula.Area, error) {
	used, ok := usedArea(n)
	area := used
	if text = strings.TrimSpace(text); text != "" {
		var valid bool
		if area, valid = formula.ParseArea(text); !valid {
			return area, fmt.Errorf("%w: area %q, write it in A1 style such as B2:D10, A:C or 5:7", ErrInvalid, text)
		}
		switch {
		case area.Cols:
			area.R2 = min(area.R2, used.R2)
		case area.Rows:
			area.C2 = min(area.C2, used.C2)
		}
	}
	if !ok || area.R2 < area.R1 || area.C2 < area.C1 {
		return formula.Area{}, nil
	}
	if cells := (area.R2 - area.R1 + 1) * (area.C2 - area.C1 + 1); cells > maxRead {
		return area, fmt.Errorf("%w: area %s has %d cells, at most %d are read at once: read less", ErrInvalid, areaName(area), cells, maxRead)
	}
	return area, nil
}

// read is the cell at row, col that holds fields, formats caching the
// number formats of styles.
func (s *state) read(row, col int, fields json.RawMessage, formats map[string]string) Cell {
	var f struct {
		V any    `json:"v"`
		E string `json:"e"`
		F string `json:"f"`
		S string `json:"s"`
	}
	_ = json.Unmarshal(fields, &f)
	c := Cell{Ref: formula.CellName(row, col), Value: f.V, Error: f.E}
	if f.F != "" {
		c.Formula = "=" + f.F
	}
	if f.S != "" {
		format, ok := formats[f.S]
		if !ok {
			var style xlsx.Style
			if n := s.tree.Node(f.S); n != nil {
				_ = json.Unmarshal(n.Attrs["style"], &style)
			}
			if format = style.Format; format == "General" {
				format = ""
			}
			formats[f.S] = format
		}
		c.Format = format
	}
	return c
}

// Markdown is the range as a table, columns and rows named.
func (r Range) Markdown() string {
	area, ok := formula.ParseArea(r.Area)
	if !ok {
		return ""
	}
	var head, line strings.Builder
	head.WriteString("| |")
	line.WriteString("|---|")
	for col := area.C1; col <= area.C2; col++ {
		head.WriteString(" " + formula.ColumnName(col) + " |")
		line.WriteString("---|")
	}
	out := head.String() + "\n" + line.String() + "\n"
	for i, row := range r.Rows {
		out += "| " + strconv.Itoa(area.R1+i) + " |"
		for _, c := range row {
			out += " " + escape.Replace(c.text()) + " |"
		}
		out += "\n"
	}
	return out
}

var escape = strings.NewReplacer("|", `\|`, "\r", "", "\n", "<br>")

// text is the cell as a table shows it: its value, with the formula that
// makes it.
func (c Cell) text() string {
	var text string
	switch v := c.Value.(type) {
	case float64:
		text = strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		text = v
	case bool:
		text = strings.ToUpper(strconv.FormatBool(v))
	}
	if c.Error != "" {
		text = c.Error
	}
	switch {
	case c.Formula == "":
		return text
	case text == "":
		return c.Formula
	}
	return c.Formula + " → " + text
}

func (p *planner) sheetEdit(e Edit) (ot.Edit, error) {
	sheet, err := p.s.sheet(e.Sheet)
	if err != nil {
		return nil, err
	}
	if e.Op == "cells" {
		return p.cells(sheet, e)
	}
	if l, ok := lineOps[e.Op]; ok {
		return p.lines(sheet, e, l)
	}
	return nil, fmt.Errorf("%w: unknown op %q, use cells, insert_rows, delete_rows, insert_columns or delete_columns", ErrInvalid, e.Op)
}

// cleared are the fields of a cell that hold what it shows.
var cleared = map[string]any{"v": nil, "f": nil, "e": nil, "rich": nil}

// cellFields is what sets a cell to a value: nil and "" empty it, and a
// string starting with = is a formula.
func cellFields(v any) (json.RawMessage, error) {
	fields := maps.Clone(cleared)
	switch v := v.(type) {
	case nil:
	case string:
		switch {
		case v == "":
		case len(v) > 1 && v[0] == '=':
			if _, err := formula.Parse(v[1:]); err != nil {
				return nil, fmt.Errorf("formula %s: %w", v, err)
			}
			fields["f"] = v[1:]
		default:
			fields["v"] = v
		}
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("number %v", v)
		}
		fields["v"] = v
	case bool:
		fields["v"] = v
	default:
		return nil, fmt.Errorf("value %v: use a number, a string, true, false or null", v)
	}
	return json.Marshal(fields)
}

func (p *planner) cells(sheet *ot.Node, e Edit) (ot.Edit, error) {
	area, ok := formula.ParseArea(strings.TrimSpace(e.Range))
	if !ok || area.Cols || area.Rows {
		return nil, fmt.Errorf("%w: range %q, give the top left cell such as B2, or an area such as B2:D4", ErrInvalid, e.Range)
	}
	if len(e.Values) == 0 {
		return nil, fmt.Errorf("%w: no values", ErrInvalid)
	}
	change := ot.Change{Op: ot.OpCel, ID: sheet.ID}
	for i, row := range e.Values {
		if !area.Cell && (i >= area.R2-area.R1+1 || len(row) > area.C2-area.C1+1) {
			return nil, fmt.Errorf("%w: the values go past the range %s", ErrInvalid, e.Range)
		}
		for j, v := range row {
			r, c := area.R1+i, area.C1+j
			if r > formula.MaxRows || c > formula.MaxCols {
				return nil, fmt.Errorf("%w: the values go past the end of the sheet", ErrInvalid)
			}
			fields, err := cellFields(v)
			if err != nil {
				return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, formula.CellName(r, c), err)
			}
			if p.orig.cellChanged(sheet.ID, r, c) > p.base {
				return nil, fmt.Errorf("%w: %s", ErrConflict, p.s.cellName(sheet.ID, r, c))
			}
			change.Cells = append(change.Cells, ot.Cell{Row: r, Col: c, Fields: fields})
		}
	}
	return ot.Edit{change}, nil
}

// lineOps are the ops that insert or delete rows or columns.
var lineOps = map[string]struct{ op, dim string }{
	"insert_rows":    {ot.OpIns, ot.DimRows},
	"delete_rows":    {ot.OpRem, ot.DimRows},
	"insert_columns": {ot.OpIns, ot.DimCols},
	"delete_columns": {ot.OpRem, ot.DimCols},
}

func (p *planner) lines(sheet *ot.Node, e Edit, l struct{ op, dim string }) (ot.Edit, error) {
	r := strings.TrimSpace(e.Range)
	if !strings.Contains(r, ":") {
		r += ":" + r
	}
	area, ok := formula.ParseArea(r)
	if !ok || l.dim == ot.DimRows && !area.Rows || l.dim == ot.DimCols && !area.Cols {
		return nil, fmt.Errorf("%w: range %q, give the rows such as 5:7 or 5, or the columns such as C:D or C", ErrInvalid, e.Range)
	}
	if p.orig.mod[sheet.ID] > p.base {
		return nil, fmt.Errorf("%w: %s", ErrConflict, p.s.name(sheet.ID))
	}
	at, n := area.R1, area.R2-area.R1+1
	if l.dim == ot.DimCols {
		at, n = area.C1, area.C2-area.C1+1
	}
	return ot.Edit{{Op: l.op, ID: sheet.ID, Dim: l.dim, At: at, N: n}}, nil
}

// previewSheets tells what an edit does to the sheets of s, which has
// made it.
func previewSheets(s *state, e ot.Edit) string {
	var b strings.Builder
	formats := map[string]string{}
	for _, c := range e {
		n := s.tree.Node(c.ID)
		if n == nil {
			continue
		}
		name := str(n.Attrs, "name")
		switch c.Op {
		case ot.OpCel:
			for _, cell := range c.Cells {
				fmt.Fprintf(&b, "%s!%s: %s\n", name, formula.CellName(cell.Row, cell.Col), s.read(cell.Row, cell.Col, n.Grid.Cell(cell.Row, cell.Col), formats).shown())
			}
		case ot.OpIns, ot.OpRem:
			what, first, last := "rows", strconv.Itoa(c.At), strconv.Itoa(c.At+c.N-1)
			if c.Dim == ot.DimCols {
				what, first, last = "columns", formula.ColumnName(c.At), formula.ColumnName(c.At+c.N-1)
			}
			done := "inserted"
			if c.Op == ot.OpRem {
				done = "deleted"
			}
			if c.N == 1 {
				fmt.Fprintf(&b, "%s %s of %s %s\n", strings.TrimSuffix(what, "s"), first, name, done)
			} else {
				fmt.Fprintf(&b, "%s %s to %s of %s %s\n", what, first, last, name, done)
			}
		}
	}
	return b.String()
}

// shown is the cell as a preview tells it.
func (c Cell) shown() string {
	switch v := c.Value.(type) {
	case string:
		if c.Formula == "" {
			return strconv.Quote(v)
		}
	case nil:
		if c.Formula == "" && c.Error == "" {
			return "(empty)"
		}
	}
	return c.text()
}
