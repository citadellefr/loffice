package agent

import "github.com/citadellefr/trame/ot"

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
	return "", nil, ErrUnsupported
}

// Range is the area of the sheet named sheet, in A1 style, the area its
// cells use when empty.
func (a *Agent) Range(sheet, area string) (version string, r Range, err error) {
	return "", Range{}, ErrUnsupported
}

// Markdown is the range as a table, columns and rows named.
func (r Range) Markdown() string { return "" }

func (p *planner) sheetEdit(e Edit) (ot.Edit, error) { return nil, ErrUnsupported }

func previewSheets(s *state, e ot.Edit) string { return "" }
