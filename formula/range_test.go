package formula

import (
	"runtime"
	"testing"
	"time"
)

func TestLargeRanges(t *testing.T) {
	b := book(map[string]any{"A1": 1, "B2": 2, "D3": 3, "B4": 7, "A5": "x", "D5": "five"})
	for _, tc := range []struct{ f, want string }{
		{"SUM($A$1:$D$1048576)", "13"},
		{"SUM(A2:D1048576)", "12"},
		{"SUM(B1:C1048576+1)", "2097161"},
		{"SUMPRODUCT(A1:B1048576,C1:D1048576)", "0"},
		{"SUMPRODUCT((B1:B1048576>0)*(B1:B1048576+1))", "11"},
		{"SUMPRODUCT(B1:C1048576+1)", "2097161"},
		{"COUNTIFS(A1:D1048576,\">0\")", "4"},
		{"SUBTOTAL(9,A1:D1048576)", "13"},
		{"AGGREGATE(9,6,A1:D1048576)", "13"},
		{"VLOOKUP(\"x\",A1:D1048576,4,FALSE)", `"five"`},
		{"VLOOKUP(\"x\",A:F,6,FALSE)", "blank"},
		{"INDEX(B1:D1048576+1,1048576,3)", "1"},
		{"MATCH(1,B1:B1048576*0+1,1)", "1048576"},
		{"XMATCH(1,B1:B1048576*0+1,0,-1)", "1048576"},
		{"XMATCH(1,B1:B1048576*0+1)", "1"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestLargeRangeShape(t *testing.T) {
	b := book(map[string]any{"A1": 1, "B2": 2})
	for _, tc := range []struct {
		f          string
		rows, cols int
		last       string
	}{
		{"A1:D1048576+1", 1048576, 4, "1"},
		{"A:D*2", 1048576, 4, "0"},
		{"INDEX(A1:D1048576+1,0,2)", 1048576, 1, "1"},
		{"A1:A1048576+B1:B5", 1048576, 1, "#N/A"},
		{"A1:A1048576&\"\"", 1048576, 1, `""`},
	} {
		e, err := Parse(tc.f)
		if err != nil {
			t.Fatal(err)
		}
		c := &Context{Book: b, Locale: English, Now: time.Now()}
		v := c.Eval(e)
		if v.Type != TypeArray || v.Arr.Rows != tc.rows || v.Arr.Cols != tc.cols {
			t.Errorf("%s: %s, want an array of %d by %d", tc.f, v.show(), tc.rows, tc.cols)
			continue
		}
		if got := v.Arr.At(tc.rows-1, tc.cols-1).show(); got != tc.last {
			t.Errorf("%s: last cell %s, want %s", tc.f, got, tc.last)
		}
		if len(v.Arr.Cells) > 16*tc.cols {
			t.Errorf("%s: holds %d cells", tc.f, len(v.Arr.Cells))
		}
	}
}

func TestHugeArrays(t *testing.T) {
	b := book(map[string]any{"A1": 1})
	for _, f := range []string{"MUNIT(100000)", "SEQUENCE(100000,100000)", "MAKEARRAY(100000,100000,LAMBDA(r,c,r))",
		"SUM(MUNIT(100000))"} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		v := eval(t, b, English, f)
		runtime.ReadMemStats(&after)
		if v.Type != TypeError {
			t.Errorf("%s = %s, want an error", f, v.show())
		}
		if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
			t.Errorf("%s took %d bytes", f, n)
		}
	}
}

func TestWholeColumnMemory(t *testing.T) {
	b := book(map[string]any{"A1": 1, "A2": 2, "B1": 3, "B2": 4})
	for _, tc := range []struct{ f, want string }{
		{"INDEX(SCAN(0,A:A,LAMBDA(a,b,a+b)),2)", "3"},
		{"ROWS(VSTACK(A:A))", "2"},
		{"COLUMNS(TRANSPOSE(A:A))", "2"},
		{"ROWS(SORT(A:A,1,-1))", "2"},
		{"INDEX(SORT(A:A+1,1,-1),1)", "3"},
		{"INDEX(EXPAND(A:A,,2),2,1)", "2"},
		{"INDEX(EXPAND(A:A,,2,0),1048576,2)", "0"},
		{"ROWS(EXPAND(A:A,,2))", "1048576"},
		{"SUM(EXPAND(A1:A2,1048576,2))", "#NUM!"},
	} {
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		got := eval(t, b, English, tc.f).show()
		runtime.ReadMemStats(&after)
		if got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
		if n := after.TotalAlloc - before.TotalAlloc; n > 1<<20 {
			t.Errorf("%s took %d bytes", tc.f, n)
		}
	}
}

func TestOffsetAreasOfLongSheet(t *testing.T) {
	b := &mapBook{sheets: []string{"Feuil1"}, cells: map[[3]int]Value{}}
	for r := 1; r <= 100_000; r++ {
		b.cells[[3]int{0, r, 1}] = Num(1)
	}
	for _, tc := range []struct{ f, want string }{
		{"COUNTIFS(A1:A100000,\">0\",A80001:A180000,\">0\")", "20000"},
		{"SUMIF(A1:A100000,\">0\",A80001:A80001)", "20000"},
		{"SUMIFS(A80001:A180000,A1:A100000,1)", "20000"},
		{"COUNTIFS(A80001:A180000,\"\",A1:A100000,1)", "80000"},
		{"COUNTIF(A80001:A180000,\"\")", "80000"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestRowsOfTwoSheets(t *testing.T) {
	b := &mapBook{sheets: []string{"Feuil1", "Feuil2"}, cells: map[[3]int]Value{}}
	for c := 1; c <= 4; c++ {
		b.cells[[3]int{0, 1, c}] = Num(1)
	}
	b.cells[[3]int{1, 1, 1}] = Num(7)
	b.cells[[3]int{1, 1, 2}] = Num(1)
	for _, tc := range []struct{ f, want string }{
		{"COUNTIFS(Feuil1!1:1,\">0\",Feuil2!1:1,\">0\")", "2"},
		{"COUNTIFS(Feuil2!1:1,\">0\",Feuil1!1:1,\">0\")", "2"},
		{"SUMIFS(Feuil2!1:1,Feuil1!1:1,1)", "8"},
		{"SUMIF(Feuil1!1:1,1,Feuil2!1:1)", "8"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestLookupLongSheet(t *testing.T) {
	if testing.Short() {
		t.Skip("300 000 rows")
	}
	const rows = 300_000
	b := &mapBook{sheets: []string{"Feuil1"}, cells: map[[3]int]Value{}}
	for r := 1; r <= rows; r++ {
		for c := 1; c <= 4; c++ {
			b.cells[[3]int{0, r, c}] = Num(float64(r*10 + c))
		}
	}
	for _, tc := range []struct{ f, want string }{
		{"VLOOKUP(2999991,A:D,3,FALSE)", "2999993"},
		{"INDEX(D:D,MATCH(2999991,A:A,0))", "2999994"},
		{"VLOOKUP(1500001.5,A:D,4)", "1500004"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestWholeColumnArrays(t *testing.T) {
	b := book(map[string]any{"A1": 1, "B1": 10, "A2": 2, "B2": 20, "A3": 2, "D3": 4})
	for _, tc := range []struct{ f, want string }{
		{"ROWS(TAKE(A:D,2))", "2"},
		{"SUM(TAKE(A:D,2))", "33"},
		{"SUM(DROP(A:D,1))", "28"},
		{"ROWS(DROP(A:D,1))", "1048575"},
		{"ROWS(CHOOSECOLS(A:D,2,4))", "1048576"},
		{"SUM(CHOOSECOLS(A:D,2,4))", "34"},
		{"SUM(CHOOSEROWS(A:D,1,1048576))", "11"},
		{"ROWS(HSTACK(A:A,C:D))", "1048576"},
		{"SUM(HSTACK(A:B,D:D))", "39"},
		{"ROWS(TOCOL(A:D,1))", "6"},
		{"SUM(TOCOL(A:D,1))", "39"},
		{"ROWS(SORT(A:B))", "1048576"},
		{"INDEX(SORT(A:B,1,1),2,2)", "20"},
		{"INDEX(SORT(A:B,1,-1),2,1)", "2"},
		{"INDEX(SORT(A1:A60000,1,-1),60000,1)", "1"},
		{"ROWS(SORT(A1:A60000,1,-1))", "60000"},
		{"ROWS(SORT(A:A,1,-1))", "3"},
		{"INDEX(SORT(A:A,1,-1),3,1)", "1"},
		{"INDEX(SORTBY(A:B,A:A,-1),3,1)", "1"},
		{"ROWS(SORTBY(A:B,A:A))", "1048576"},
		{"ROWS(SORTBY(A:A,A:A,-1))", "3"},
		{"ROWS(FILTER(A:B,A:A=2))", "2"},
		{"ROWS(FILTER(A:B,A:A=\"\"))", "1048573"},
		{"ROWS(UNIQUE(A:B))", "4"},
		{"ROWS(UNIQUE(A:A,FALSE,TRUE))", "1"},
		{"SUM(MAP(A:D,LAMBDA(x,x+1)))", "4194343"},
		{"ROWS(BYROW(A:D,LAMBDA(r,SUM(r))))", "1048576"},
		{"SUM(BYROW(A:D,LAMBDA(r,SUM(r))))", "39"},
		{"CORREL(A:B,A:B)", "1"},
		{"SUMX2MY2(A:A,B:B)", "-495"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestSparseAndCrowdedAreas(t *testing.T) {
	sparse := book(map[string]any{"A1": 1, "D1048576": 1})
	if got := eval(t, sparse, English, "SUM(A1:D1048576*1)").show(); got != "#NUM!" {
		t.Errorf("sheet of two far cells: %s", got)
	}
	if got := eval(t, sparse, English, "SUM(A1:D1048576)").show(); got != "2" {
		t.Errorf("sum of two far cells: %s", got)
	}
	if testing.Short() {
		return
	}
	crowded := &mapBook{sheets: []string{"Feuil1"}, cells: map[[3]int]Value{}}
	for r := 1; r <= 300_000; r++ {
		for c := 1; c <= 4; c++ {
			crowded.cells[[3]int{0, r, c}] = Num(1)
		}
	}
	for _, tc := range []struct{ f, want string }{
		{"SUM(A1:D300000*2)", "2400000"},
		{"SUMPRODUCT(A:D,A:D)", "1200000"},
		{"COUNTIFS(A:D,1)", "1200000"},
	} {
		if got := eval(t, crowded, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestLargeSpill(t *testing.T) {
	s, e := newSource()
	s.set(e, "A1", 1)
	if got := results(s.dynamic(e, "B2", "A:A*2")); got != "[B2=#SPILL!]" {
		t.Errorf("past the sheet: %s", got)
	}
	if got := results(s.dynamic(e, "F1", "A:D*2")); got != "[F1=#NUM!]" {
		t.Errorf("too many: %s", got)
	}
	if testing.Short() {
		return
	}
	if got := spillOf(s.dynamic(e, "H1", "A1:A1000000*2")); got != "H1:H1:H1000000" {
		t.Errorf("column: %s", got)
	}
}
