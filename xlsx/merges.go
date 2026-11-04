package xlsx

import (
	"bytes"
	"encoding/json"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// merges are the areas merged in the sheets an edit changes, by sheet and
// first cell, as the edit leaves them.
type merges map[string]map[[2]int][2]int

// apply notes what cells set of the merged areas of a sheet, and tells
// whether the areas that result lie in the sheet and none overlaps another.
func (m merges) apply(tree *ot.Tree, sheet string, cells []ot.Cell) bool {
	set, ok := m[sheet]
	if !ok {
		set = spansOf(tree.Node(sheet))
		m[sheet] = set
	}
	var changed [][2]int
	for _, c := range cells {
		if c.Row == 0 || c.Col == 0 {
			continue
		}
		var f struct{ M json.RawMessage }
		if json.Unmarshal(c.Fields, &f) != nil || f.M == nil {
			continue
		}
		at := [2]int{c.Row, c.Col}
		var span [2]int
		if json.Unmarshal(f.M, &span) != nil || span[0] <= 1 && span[1] <= 1 {
			delete(set, at)
			continue
		}
		if c.Row+span[0]-1 > formula.MaxRows || c.Col+span[1]-1 > formula.MaxCols {
			return false
		}
		set[at] = span
		changed = append(changed, at)
	}
	for _, at := range changed {
		for other, span := range set {
			if other != at && overlap(at, set[at], other, span) {
				return false
			}
		}
	}
	return true
}

// overlap tells whether the areas of the given first cell and span share a
// cell.
func overlap(a, aSpan, b, bSpan [2]int) bool {
	return a[0] < b[0]+bSpan[0] && b[0] < a[0]+aSpan[0] && a[1] < b[1]+bSpan[1] && b[1] < a[1]+aSpan[1]
}

// spansOf are the merged areas of a sheet.
func spansOf(n *ot.Node) map[[2]int][2]int {
	set := map[[2]int][2]int{}
	if n == nil || n.Grid == nil {
		return set
	}
	n.Grid.Each(1, formula.MaxRows, func(r, c int, raw json.RawMessage) bool {
		if c > 0 && bytes.Contains(raw, []byte(`"m":`)) {
			var f cellFields
			if json.Unmarshal(raw, &f) == nil && f.M != nil {
				set[[2]int{r, c}] = *f.M
			}
		}
		return true
	})
	return set
}
