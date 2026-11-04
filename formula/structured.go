package formula

import (
	"strings"
)

// TableDef is a table of a workbook (a ListObject) as structured
// references read it.
type TableDef struct {
	Name  string
	Sheet int
	// Area holds the header row and the totals row, when there are some.
	Area    Area
	Header  bool
	Totals  bool
	Columns []string
}

// Tables is a source or a book that has tables, whose structured
// references its formulas may use. An engine reads the tables when a
// formula is set and when it is calculated: it must be built again, or
// its formulas set again, when a table changes its area or its columns.
type Tables interface {
	// Table finds a table by its name, whatever the case.
	Table(name string) (TableDef, bool)
	// TableAt finds the table that holds a cell.
	TableAt(sheet, row, col int) (TableDef, bool)
}

// Structured is a structured reference: Table[Col], Table[[#Totals],[A]:[B]]
// or [@Col]. Table is "" when the reference is not qualified, which stands
// for the table holding the formula.
type Structured struct {
	Table string
	// Items are among "#All", "#Data", "#Headers", "#Totals" and
	// "#This Row", which "@" stands for; none means "#Data".
	Items []string
	// First and Last are the columns, from First to Last. First is "" for
	// all of them, and Last for a single one.
	First, Last string
	// Text is what RewriteStructured writes in place of the reference when
	// change sets it, any piece of a formula: an A1 reference, "#REF!".
	Text string
}

var items = []string{"#All", "#Data", "#Headers", "#Totals", "#This Row"}

// item is the canonical form of the item s spells, whatever the case.
func item(s string) (string, bool) {
	for _, it := range items {
		if strings.EqualFold(s, it) {
			return it, true
		}
	}
	return "", false
}

// ParseStructured reads the text of a name token as a structured
// reference: "T[Col]", "[@Col]", "T[[#Headers],[A]:[B]]", "T[]", "T[@]".
func ParseStructured(s string) (Structured, bool) {
	var r Structured
	i := strings.IndexByte(s, '[')
	if i < 0 || identLen(s) != i {
		return r, false
	}
	if n, ok := bracketLen(s[i:]); !ok || n != len(s)-i {
		return r, false
	}
	r.Table = s[:i]
	inner := s[i+1 : len(s)-1]
	if strings.HasPrefix(inner, "@") {
		r.Items = []string{"#This Row"}
		inner = inner[1:]
	}
	switch {
	case inner == "":
	case inner[0] == '[':
		if !r.terms(inner) {
			return Structured{}, false
		}
	case r.Items == nil:
		if it, ok := item(inner); ok {
			r.Items = []string{it}
		} else {
			r.First = unescape(inner)
		}
	default:
		r.First = unescape(inner)
	}
	return r, true
}

// terms reads the bracketed items and columns of s, which an inner "," or
// ":" separates.
func (r *Structured) terms(s string) bool {
	for s != "" {
		name, rest, ok := group(s)
		if !ok {
			return false
		}
		s = rest
		if strings.HasPrefix(name, "#") {
			it, ok := item(name)
			if !ok || s != "" && s[0] == ':' {
				return false
			}
			r.Items = append(r.Items, it)
		} else {
			if r.First != "" {
				return false
			}
			r.First = unescape(name)
			if strings.HasPrefix(s, ":") {
				if r.Last, s, ok = group(s[1:]); !ok || strings.HasPrefix(r.Last, "#") {
					return false
				}
				r.Last = unescape(r.Last)
			}
		}
		if s = strings.TrimLeft(s, " "); s == "" {
			return true
		}
		if s[0] != ',' {
			return false
		}
		s = strings.TrimLeft(s[1:], " ")
	}
	return false
}

// group reads the "[...]" s starts with, and what follows it.
func group(s string) (inner, rest string, ok bool) {
	if !strings.HasPrefix(s, "[") {
		return "", "", false
	}
	n, ok := bracketLen(s)
	if !ok {
		return "", "", false
	}
	return s[1 : n-1], s[n:], true
}

// unescape drops the "'" that protects "[", "]", "#" and "'" in a column
// name.
func unescape(s string) string {
	if !strings.Contains(s, "'") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\'' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

var escaper = strings.NewReplacer("[", "'[", "]", "']", "#", "'#", "'", "''")

// String is the reference as a workbook stores it: "@" is written as the
// item "#This Row", and a column is bracketed twice when an item comes
// with it.
func (s Structured) String() string {
	var b strings.Builder
	b.WriteString(s.Table)
	switch {
	case s.First == "" && len(s.Items) == 0:
		b.WriteString("[]")
	case s.First == "" && len(s.Items) == 1:
		b.WriteString("[" + s.Items[0] + "]")
	case len(s.Items) == 0 && s.Last == "":
		b.WriteString("[" + escaper.Replace(s.First) + "]")
	default:
		var parts []string
		for _, it := range s.Items {
			parts = append(parts, "["+it+"]")
		}
		if s.First != "" {
			col := "[" + escaper.Replace(s.First) + "]"
			if s.Last != "" {
				col += ":[" + escaper.Replace(s.Last) + "]"
			}
			parts = append(parts, col)
		}
		b.WriteString("[" + strings.Join(parts, ",") + "]")
	}
	return b.String()
}

// column is the index of a column of the table, counted from 0, whatever
// the case.
func (t TableDef) column(name string) int {
	for i, c := range t.Columns {
		if strings.EqualFold(c, name) {
			return i
		}
	}
	return -1
}

// Resolve is the area a structured reference stands for, in the formula of
// row row: the data of the table by default, a part of it by its items and
// columns. When it stands for none, it is nil and the error is what the
// formula gives: #REF! for a part the table lacks, #VALUE! for "#This Row"
// outside of the data. The area is absolute, as converting a table to a
// range makes it.
func (t TableDef) Resolve(s Structured, row int) (Area, *Value) {
	first, last := t.Area.R1, t.Area.R2
	if t.Header {
		first++
	}
	if t.Totals {
		last--
	}
	if first > last {
		return Area{}, &ErrRef
	}
	a := t.Area
	var all, headers, data, totals bool
	for _, it := range s.Items {
		switch it {
		case "#All":
			all = true
		case "#Data":
			data = true
		case "#Headers":
			headers = true
		case "#Totals":
			totals = true
		case "#This Row":
			if len(s.Items) > 1 {
				return Area{}, &ErrRef
			}
			if row < first || row > last {
				return Area{}, &ErrValue
			}
			a.R1, a.R2 = row, row
		}
	}
	if len(s.Items) == 0 {
		data = true
	}
	if all {
		a.R1, a.R2 = t.Area.R1, t.Area.R2
	} else if headers || data || totals {
		if headers && !t.Header || totals && !t.Totals || headers && totals && !data {
			return Area{}, &ErrRef
		}
		switch {
		case headers:
			a.R1 = t.Area.R1
		case data:
			a.R1 = first
		default:
			a.R1 = t.Area.R2
		}
		switch {
		case totals:
			a.R2 = t.Area.R2
		case data:
			a.R2 = last
		default:
			a.R2 = t.Area.R1
		}
	}
	if s.First != "" {
		from := t.column(s.First)
		to := from
		if s.Last != "" {
			to = t.column(s.Last)
		}
		if from < 0 || to < 0 {
			return Area{}, &ErrRef
		}
		if from > to {
			from, to = to, from
		}
		a.C1, a.C2 = t.Area.C1+from, t.Area.C1+to
	}
	a.AbsR1, a.AbsC1, a.AbsR2, a.AbsC2 = true, true, true, true
	a.Cell = a.R1 == a.R2 && a.C1 == a.C2
	return a, nil
}

// tableOf is the table a reference names, or that holding the cell of the
// formula when it names none.
func tableOf(tb Tables, s Structured, p Pos) (TableDef, bool) {
	if s.Table == "" {
		return tb.TableAt(p.Sheet, p.Row, p.Col)
	}
	return tb.Table(s.Table)
}

// resolve is the area a reference stands for in the formula of a cell.
func resolve(tb Tables, s Structured, p Pos) (Area3, *Value) {
	t, ok := tableOf(tb, s, p)
	if !ok {
		return Area3{}, &ErrRef
	}
	a, err := t.Resolve(s, p.Row)
	return Area3{t.Sheet, a}, err
}

// RewriteStructured gives each structured reference of f to change, and
// writes back those it changed, as String writes them, or as their Text
// when change sets it; the rest of f is kept as written. Table[Col] and
// [@Col] are structured references, a table's name alone is not.
func RewriteStructured(f string, change func(s *Structured) bool) (string, error) {
	tokens, err := Tokens(f)
	if err != nil {
		return f, err
	}
	var b strings.Builder
	last := 0
	for _, t := range tokens {
		if t.Kind != Name {
			continue
		}
		s, ok := ParseStructured(t.Text)
		if !ok || !change(&s) {
			continue
		}
		b.WriteString(f[last:t.Pos])
		if s.Text != "" {
			b.WriteString(s.Text)
		} else {
			b.WriteString(s.String())
		}
		last = t.Pos + len(t.Text)
	}
	if last == 0 {
		return f, nil
	}
	b.WriteString(f[last:])
	return b.String(), nil
}
