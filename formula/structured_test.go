package formula

import (
	"slices"
	"strings"
	"testing"
)

func TestStructuredRoundTrip(t *testing.T) {
	for in, want := range map[string]string{
		"Table1[Col]":                     "Table1[Col]",
		"Table1[Col A]":                   "Table1[Col A]",
		"Table1[]":                        "Table1[]",
		"Table1[#All]":                    "Table1[#All]",
		"Table1[#Totals]":                 "Table1[#Totals]",
		"Table1[#data]":                   "Table1[#Data]",
		"Table1[[#Data]]":                 "Table1[#Data]",
		"Table1[[#This Row]]":             "Table1[#This Row]",
		"Table1[@]":                       "Table1[#This Row]",
		"Table1[@Col]":                    "Table1[[#This Row],[Col]]",
		"Table1[@[Col A]]":                "Table1[[#This Row],[Col A]]",
		"Table1[@[A]:[B]]":                "Table1[[#This Row],[A]:[B]]",
		"[@Col]":                          "[[#This Row],[Col]]",
		"[Col]":                           "[Col]",
		"[[#This Row],[Col]]":             "[[#This Row],[Col]]",
		"Table1[[#Headers],[#Data]]":      "Table1[[#Headers],[#Data]]",
		"Table1[[#headers], [A]:[B]]":     "Table1[[#Headers],[A]:[B]]",
		"Table1[[A]:[B]]":                 "Table1[[A]:[B]]",
		"Table1[[Col]]":                   "Table1[Col]",
		"Table1[[#Totals],[Col]]":         "Table1[[#Totals],[Col]]",
		"Table1[Prix '[EUR']]":            "Table1[Prix '[EUR']]",
		"Table1['#Total]":                 "Table1['#Total]",
		"Table1[It''s]":                   "Table1[It''s]",
		"Table1[[#Headers],[It''s '#1]]":  "Table1[[#Headers],[It''s '#1]]",
		"Table1[[#Data],[#Totals],[Col]]": "Table1[[#Data],[#Totals],[Col]]",
		"Tableau1[[#En-têtes],[Col]]":     "",
		"Table1[[#Data],[Col],[Other]]":   "",
		"Table1[[#Headers]:[Col]]":        "",
		"Table1[[#Nothing],[Col]]":        "",
		"Table1[Col]x":                    "",
		"Table1[[A],]":                    "",
		"Table1 [Col]":                    "",
		"Table1":                          "",
	} {
		s, ok := ParseStructured(in)
		if want == "" {
			if ok {
				t.Errorf("%s read as %+v", in, s)
			}
			continue
		}
		if !ok || s.String() != want {
			t.Errorf("%s: %+v, %v, %s", in, s, ok, s.String())
			continue
		}
		if again, ok := ParseStructured(s.String()); !ok || !slices.Equal(again.Items, s.Items) || again.First != s.First || again.Last != s.Last {
			t.Errorf("%s: written %s read as %+v", in, s.String(), again)
		}
	}
	s, _ := ParseStructured("Table1[[#Headers],[It''s '#1]:[B'[]]")
	if s.First != "It's #1" || s.Last != "B[" || !slices.Equal(s.Items, []string{"#Headers"}) {
		t.Errorf("%+v", s)
	}
}

func TestStructuredTokens(t *testing.T) {
	for f, want := range map[string]string{
		"SUM(Table1[Col])":              "Table1[Col]",
		"[@Price]*[@Qty]":               "[@Price] [@Qty]",
		"Table1[[#This Row],[A]]+1":     "Table1[[#This Row],[A]]",
		"Table1[Col '[x']]&\"]\"":       "Table1[Col '[x']]",
		"[1]Sheet1!A1+[@Col]":           "[@Col]",
		"[@Col] [@Other]":               "[@Col] [@Other]",
		"Table1[[#Headers],[A]:[B]]:B2": "Table1[[#Headers],[A]:[B]]",
	} {
		tokens, err := Tokens(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		var names []string
		for _, tok := range tokens {
			if tok.Kind == Name {
				names = append(names, tok.Text)
			}
		}
		if got := strings.Join(names, " "); got != want {
			t.Errorf("%s: names %q, want %q", f, got, want)
		}
	}
	for _, f := range []string{"[@Col", "Table1[[A]", "Table1[Col'"} {
		if _, err := Tokens(f); err == nil {
			t.Errorf("%s: no error", f)
		}
	}
}

// salesTable is Sales on sheet 0, from B2 to E6: a header row, three rows
// of data and a totals row.
var salesTable = TableDef{Name: "Sales", Area: Area{R1: 2, C1: 2, R2: 6, C2: 5}, Header: true, Totals: true, Columns: []string{"Item", "Price", "Qty", "Amount"}}

func TestResolve(t *testing.T) {
	plain := salesTable
	plain.Header, plain.Totals = false, false
	for _, c := range []struct {
		t    TableDef
		ref  string
		row  int
		want string
	}{
		{salesTable, "Sales[Price]", 8, "$C$3:$C$5"},
		{salesTable, "sales[PRICE]", 8, "$C$3:$C$5"},
		{salesTable, "Sales[]", 8, "$B$3:$E$5"},
		{salesTable, "Sales[#Data]", 8, "$B$3:$E$5"},
		{salesTable, "Sales[#All]", 8, "$B$2:$E$6"},
		{salesTable, "Sales[#Headers]", 8, "$B$2:$E$2"},
		{salesTable, "Sales[#Totals]", 8, "$B$6:$E$6"},
		{salesTable, "Sales[[#Headers],[#Data]]", 8, "$B$2:$E$5"},
		{salesTable, "Sales[[#Data],[#Totals]]", 8, "$B$3:$E$6"},
		{salesTable, "Sales[[#Totals],[#Data]]", 8, "$B$3:$E$6"},
		{salesTable, "Sales[[#All],[Qty]]", 8, "$D$2:$D$6"},
		{salesTable, "Sales[[#Headers],[Price]:[Amount]]", 8, "$C$2:$E$2"},
		{salesTable, "Sales[[Amount]:[Price]]", 8, "$C$3:$E$5"},
		{salesTable, "Sales[[#Totals],[Amount]]", 8, "$E$6"},
		{salesTable, "Sales[@Qty]", 4, "$D$4"},
		{salesTable, "Sales[@]", 4, "$B$4:$E$4"},
		{salesTable, "Sales[@[Item]:[Qty]]", 3, "$B$3:$D$3"},
		{salesTable, "Sales[@Qty]", 2, "#VALUE!"},
		{salesTable, "Sales[@Qty]", 6, "#VALUE!"},
		{salesTable, "Sales[@Qty]", 9, "#VALUE!"},
		{salesTable, "Sales[Nothing]", 4, "#REF!"},
		{salesTable, "Sales[[#Headers],[#Totals]]", 4, "#REF!"},
		{salesTable, "Sales[[#This Row],[#Data],[Qty]]", 4, "#REF!"},
		{plain, "Sales[#Headers]", 4, "#REF!"},
		{plain, "Sales[#Totals]", 4, "#REF!"},
		{plain, "Sales[#All]", 4, "$B$2:$E$6"},
		{plain, "Sales[Qty]", 4, "$D$2:$D$6"},
		{plain, "Sales[@Qty]", 2, "$D$2"},
	} {
		s, ok := ParseStructured(c.ref)
		if !ok {
			t.Fatal(c.ref)
		}
		a, err := c.t.Resolve(s, c.row)
		got := ""
		if err != nil {
			got = err.Str
		} else {
			got = (&Ref{Area: a}).String()
		}
		if got != c.want {
			t.Errorf("%s at row %d: %s, want %s", c.ref, c.row, got, c.want)
		}
	}
}

func TestRewriteStructured(t *testing.T) {
	rename := func(s *Structured) bool {
		if s.First != "Price" {
			return false
		}
		s.First = "Cost [EUR]"
		return true
	}
	for f, want := range map[string]string{
		"SUM(Sales[Price])+A1":                    "SUM(Sales[Cost '[EUR']])+A1",
		"[@Price] * Sales[[#This Row],[Qty]]":     "[[#This Row],[Cost '[EUR']]] * Sales[[#This Row],[Qty]]",
		"SUM( Sales[[#Totals],[Price]] , \"x\" )": "SUM( Sales[[#Totals],[Cost '[EUR']]] , \"x\" )",
		"Sales[Qty]+Sales":                        "Sales[Qty]+Sales",
	} {
		if got, err := RewriteStructured(f, rename); err != nil || got != want {
			t.Errorf("%s: %s, %v", f, got, err)
		}
	}
	got, err := RewriteStructured("[@Price]+Sales[Price]", func(s *Structured) bool {
		if s.Table != "" {
			return false
		}
		s.Table = "Sales"
		return true
	})
	if want := "Sales[[#This Row],[Price]]+Sales[Price]"; got != want || err != nil {
		t.Error(got, err)
	}
	if _, err := RewriteStructured("Sales[Price", rename); err == nil {
		t.Error("no error")
	}
}

func TestRewriteStructuredText(t *testing.T) {
	got, err := RewriteStructured("SUM(Sales[Price])*[@Qty]", func(s *Structured) bool {
		if s.Table != "Sales" {
			return false
		}
		s.Text = "$C$3:$C$5"
		return true
	})
	if want := "SUM($C$3:$C$5)*[@Qty]"; got != want || err != nil {
		t.Error(got, err)
	}
}

// tableSource is a source with the table Sales, on sheet 0 as in
// salesTable.
type tableSource struct {
	*sheetSource
}

func (s tableSource) Table(name string) (TableDef, bool) {
	return salesTable, strings.EqualFold(name, salesTable.Name)
}

func (s tableSource) TableAt(sheet, row, col int) (TableDef, bool) {
	return salesTable, sheet == 0 && salesTable.Area.Contains(row, col)
}

func TestEngineStructured(t *testing.T) {
	s, _ := newSource()
	e := NewEngine(tableSource{s}, Options{})
	for _, row := range [][3]int{{3, 10, 2}, {4, 5, 3}, {5, 1, 7}} {
		s.set(e, CellName(row[0], 3), row[1])
		s.set(e, CellName(row[0], 4), row[2])
		s.set(e, CellName(row[0], 5), "=[@Price]*[@Qty]")
	}
	s.set(e, "E6", "=SUBTOTAL(109,Sales[Amount])")
	s.set(e, "G1", "=SUM(Sales[Amount])")
	s.set(e, "G2", "=Sales[[#Totals],[Amount]]")
	s.set(e, "G3", "=SUM(Sales)")
	s.set(e, "G4", "=ROWS(Sales[#All])")
	s.set(e, "G5", "=Sales[[#This Row],[Price]]")
	s.set(e, "G6", "=Sales[[#This Row],[Price]]")
	s.set(e, "G10", "=INDEX(Sales[Qty],2)+SUM(INDIRECT(\"Sales[Price]\"))")
	s.set(e, "G7", "=[@Price]")
	s.set(e, "G8", "=SUM(Sales[Nothing])")
	s.set(e, "G9", "=COUNTA(Sales[#Headers])")
	for cell, want := range map[string]string{
		"E3": "20", "E4": "15", "E5": "7", "E6": "42",
		"G1": "42", "G2": "42", "G3": "70", "G4": "5",
		"G5": "1", "G6": "#VALUE!", "G10": "19", "G7": "#REF!", "G8": "#REF!", "G9": "0",
	} {
		r, c, _ := ParseCell(cell)
		got := s.cells[[3]int{0, r, c}].show()
		if got != want {
			t.Errorf("%s = %s, want %s", cell, got, want)
		}
	}
	if got := sorted(s.set(e, "D4", 10)); got != "[E4=50 E6=77 G1=77 G10=26 G2=77 G3=112]" {
		t.Error(got)
	}
	if got := sorted(s.set(e, "C3", 3)); got != "[E3=6 E6=63 G1=63 G10=19 G2=63 G3=91]" {
		t.Error(got)
	}
}

// sorted is results in the order of the names of the cells.
func sorted(out []Result) string {
	slices.SortFunc(out, func(a, b Result) int { return strings.Compare(CellName(a.Row, a.Col), CellName(b.Row, b.Col)) })
	return results(out)
}
