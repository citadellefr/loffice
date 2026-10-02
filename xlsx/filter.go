package xlsx

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const filterKey = "filter"

// An AutoFilter is the filter of a sheet: arrows on the first row of Ref,
// and the values the filtered columns show. The rows it hides are hidden
// by the editor as any other.
type AutoFilter struct {
	Ref  string         `json:"ref"`
	Cols []FilterColumn `json:"cols,omitempty"`
}

// A FilterColumn shows the rows whose cell in column Col of the filter,
// counted from 0, reads one of Vals as it is shown, or is blank when
// Blank. Kept is a filter of another kind, the Kept-th of the file, which
// the editor shows and may clear but does not change.
type FilterColumn struct {
	Col   int      `json:"col"`
	Vals  []string `json:"vals,omitempty"`
	Blank bool     `json:"blank,omitempty"`
	Kept  int      `json:"kept,omitempty"`
}

// maxFilterValues bounds the values a column shows.
const maxFilterValues = 10000

var autoFilterOrder = []string{"filterColumn", "sortState", "extLst"}

// autoFilter is the filter of a worksheet, if any.
func autoFilter(root *xmldom.Element) *AutoFilter {
	e := root.Child(mainNS, "autoFilter")
	if e == nil {
		return nil
	}
	if _, ok := formula.ParseArea(e.Get("ref")); !ok {
		return nil
	}
	f := &AutoFilter{Ref: e.Get("ref")}
	for i, c := range elements(e, "filterColumn") {
		col := FilterColumn{Col: atoi(c.Get("colId"))}
		values := c.Child(mainNS, "filters")
		if values != nil && len(elements(values, "dateGroupItem")) == 0 && len(c.Elements()) == 1 {
			col.Blank = truthy(values.Get("blank"))
			for _, v := range elements(values, "filter") {
				col.Vals = append(col.Vals, v.Get("val"))
			}
		} else {
			col.Kept = i + 1
		}
		f.Cols = append(f.Cols, col)
	}
	return f
}

// checkFilter tells whether a filter is one the editor may set.
func checkFilter(raw json.RawMessage) bool {
	var f AutoFilter
	if json.Unmarshal(raw, &f) != nil {
		return false
	}
	a, ok := formula.ParseArea(f.Ref)
	if !ok || a.Rows || a.Cols {
		return false
	}
	seen := map[int]bool{}
	for _, c := range f.Cols {
		if c.Col < 0 || c.Col > a.C2-a.C1 || seen[c.Col] || c.Kept < 0 || len(c.Vals) > maxFilterValues {
			return false
		}
		for _, v := range c.Vals {
			if len(v) > maxText {
				return false
			}
		}
		seen[c.Col] = true
	}
	return true
}

// writeFilter gives a worksheet the filter of its sheet. The columns read
// from the file, aligned with its elements, keep them when they are still
// there, moved or not; the others are written.
func writeFilter(root *xmldom.Element, oldRaw, newRaw json.RawMessage) {
	var f AutoFilter
	e := root.Child(mainNS, "autoFilter")
	if json.Unmarshal(newRaw, &f) != nil || f.Ref == "" {
		if e != nil {
			root.Remove(e)
		}
		return
	}
	var old AutoFilter
	_ = json.Unmarshal(oldRaw, &old)
	e = child(root, "autoFilter", worksheetOrder)
	e.Set("ref", f.Ref)
	columns := elements(e, "filterColumn")
	for _, c := range columns {
		e.Remove(c)
	}
	used := map[int]bool{}
	for _, c := range f.Cols {
		i := c.Kept - 1
		for j, o := range old.Cols {
			if c.Kept == 0 && i < 0 && o.Kept == 0 && o.Blank == c.Blank && slices.Equal(o.Vals, c.Vals) && !used[j] {
				i = j
			}
		}
		var fc *xmldom.Element
		switch {
		case i >= 0 && i < len(columns) && !used[i]:
			used[i] = true
			fc = columns[i]
		case c.Kept > 0:
			continue
		default:
			fc = filterColumn(prefixOf(e), c)
		}
		fc.Set("colId", strconv.Itoa(c.Col))
		e.Insert(fc, autoFilterOrder)
	}
}

func filterColumn(prefix string, c FilterColumn) *xmldom.Element {
	fc := xmldom.New(mainNS, prefix+"filterColumn")
	values := xmldom.New(mainNS, prefix+"filters")
	if c.Blank {
		values.Set("blank", "1")
	}
	for _, v := range c.Vals {
		values.Append(xmldom.New(mainNS, prefix+"filter", "val", v))
	}
	fc.Append(values)
	return fc
}

// shifted is the filter once rows or columns are inserted or removed on
// its sheet, named sheet: nil when its area is gone. The columns it
// filters move with theirs.
func (f *AutoFilter) shifted(sheet string, rows bool, at, n int) *AutoFilter {
	before, _ := formula.ParseArea(f.Ref)
	ref := shiftRef(f.Ref, sheet, rows, at, n)
	after, ok := formula.ParseArea(ref)
	if !ok {
		return nil
	}
	out := &AutoFilter{Ref: ref}
	for _, c := range f.Cols {
		col := before.C1 + c.Col
		if !rows {
			switch {
			case col < at:
			case n < 0 && col < at-n:
				continue
			default:
				col += n
			}
		}
		c.Col = col - after.C1
		if c.Col >= 0 && c.Col <= after.C2-after.C1 {
			out.Cols = append(out.Cols, c)
		}
	}
	return out
}
