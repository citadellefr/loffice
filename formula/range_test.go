package formula

import (
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestLargeRanges(t *testing.T) {
	b := book(map[string]any{"A1": 1, "B2": 2, "D3": 3, "B4": 7, "A5": "x", "D5": "five"})
	for _, tc := range []struct{ f, want string }{
		{"SUM($A$1:$D$1048576)", "13"},
		{"SUM(A2:D1048576)", "12"},
		{"SUM(B1:C1048576+1)", "2097161"},
		{"COUNTBLANK(A1:D1048576)", "4194298"},
		{"SUMPRODUCT(A1:B1048576,C1:D1048576)", "0"},
		{"SUMPRODUCT((B1:C1048576>0)*(B1:C1048576+1))", "11"},
		{"SUMPRODUCT(B1:C1048576+1)", "2097161"},
		{"COUNTIFS(A1:D1048576,\">0\")", "4"},
		{"SUBTOTAL(9,A1:D1048576)", "13"},
		{"AGGREGATE(9,6,A1:D1048576)", "13"},
		{"CORREL(A1:B1048576,C1:D1048576)", "#DIV/0!"},
		{"SUMX2PY2(A1:B1048576,A1:B1048576)", "108"},
		{"INDEX(A1:D1048576+1,1,1)", "#NUM!"},
		{"SORT(A1:D1048576)", "#NUM!"},
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
		{"A1:B1048576&\"\"", 1048576, 2, `""`},
		{"A1:B1048576+A1:A5", 1048576, 2, "#N/A"},
		{"ISBLANK(A1:B1048576)", 1048576, 2, "TRUE"},
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

// smallBook is a sheet of 15 cells and another of a few.
func smallBook() *mapBook {
	return book(map[string]any{"A1": 1, "B1": 2, "C1": 3, "A2": 4, "B2": "x", "C2": 6, "A3": 7, "B3": 8,
		"C3": true, "D4": 10, "A5": 11, "B5": 12, "C5": 13, "D5": "y", "E5": 15, "Data!A1": 1, "Data!A2": 2})
}

// measure evaluates a formula, telling how long it took and how many
// bytes it allocated.
func measure(t *testing.T, b Book, f string) (Value, time.Duration, uint64) {
	t.Helper()
	e, err := Parse(f)
	if err != nil {
		t.Fatalf("%s: %v", f, err)
	}
	c := &Context{Book: b, Sheet: 0, Row: 20, Col: 10, Locale: English,
		Now: time.Date(2024, 12, 25, 18, 30, 0, 0, time.UTC), Rand: func() float64 { return 0.5 }}
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	start := time.Now()
	v := c.Eval(e)
	d := time.Since(start)
	runtime.ReadMemStats(&after)
	return v, d, after.TotalAlloc - before.TotalAlloc
}

func TestHugeArrays(t *testing.T) {
	b := smallBook()
	for _, tc := range []struct{ f, want string }{
		{"MUNIT(100000)", "#VALUE!"},
		{"SEQUENCE(100000,100000)", "#NUM!"},
		{"MAKEARRAY(100000,100000,LAMBDA(r,c,r))", "#NUM!"},
		{"SUM(MUNIT(100000))", "#VALUE!"},
		{"A1:XFD1048576*1", "#NUM!"},
		{"SUM(A1:XFD1048576*1)", "#NUM!"},
		{"COUNT(A1:XFD1048576*1)", "0"},
		{"SUM(A1:B1048576+A1:XFD1)", "#NUM!"},
		{"MEDIAN(A:XFD+1)", "#VALUE!"},
		{"SUM(A:XFD+1)", "#VALUE!"},
		{"MEDIAN(A:A+1)", "5"},
		{"SUM(A:D+1)", "#VALUE!"},
		{"SUM(A:A+1)", "28"},
	} {
		v, d, n := measure(t, b, tc.f)
		if got := (&Context{}).scalar(v).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
		if d > time.Second || n > 16<<20 {
			t.Errorf("%s took %v and %d bytes", tc.f, d, n)
		}
	}
}

// TestFunctionsOfHugeArguments calls every function with ranges and
// arrays of a huge size, but few cells: none may panic, take long, spend
// much memory, or give a table that spans more than maxSize cells without
// holding them.
func TestFunctionsOfHugeArguments(t *testing.T) {
	numbers := book(map[string]any{"A1": 1, "B1": 2, "C1": 3, "D1": 4, "A2": 5, "B2": 6, "C2": 7, "D2": 8,
		"A3": 9, "B3": 10, "C3": 11, "D3": 12})
	t.Run("mixed", func(t *testing.T) {
		hugeArguments(t, smallBook(), []string{"A:XFD+1", "A1:XFD1048576*1", "A:XFD", "A1:A70000", "A1:D1048576+1", "A1:XFD1048576"})
	})
	t.Run("numbers", func(t *testing.T) {
		hugeArguments(t, numbers, []string{"A1:D1048576", "A:XFD", "A1:A70000"})
	})
}

func hugeArguments(t *testing.T, b Book, huge []string) {
	names := make([]string, 0, len(functions))
	for name := range functions {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		f := functions[name]
		least, most := max(f.min, 1), f.max
		if most < 0 || most > 3 {
			most = 3
		}
		if testing.Short() {
			most = least
		}
		for n := least; n <= most; n++ {
			for _, h := range huge {
				for _, at := range []int{-1, 0, n - 1} {
					args := make([]string, n)
					for i := range args {
						args[i] = "2"
						if at < 0 || i == at {
							args[i] = h
						}
					}
					check(t, b, name+"("+strings.Join(args, ",")+")")
					if testing.Short() {
						break
					}
				}
			}
		}
	}
}

func check(t *testing.T, b Book, f string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: panic %v", f, r)
		}
	}()
	v, d, n := measure(t, b, f)
	if d > 10*time.Second || n > 64<<20 {
		t.Errorf("%s took %v and %d bytes", f, d, n)
	}
	if v.Type == TypeArray && v.Arr.Rows*v.Arr.Cols > max(len(v.Arr.Cells), maxSize) {
		t.Errorf("%s spans %d by %d", f, v.Arr.Rows, v.Arr.Cols)
	}
}

func TestShapeOfArraysFromRanges(t *testing.T) {
	b := book(map[string]any{"A1": 1, "A10": 10, "Data!A1": 1, "Data!A10": 10})
	for _, tc := range []struct{ f, want string }{
		{"ROWS(VSTACK(A1:A70000))", "70000"},
		{"COLUMNS(TRANSPOSE(A1:A70000))", "70000"},
		{"ROWS(SORT(A1:A70000))", "70000"},
		{"ROWS(A:A*2)", "10"},
		{"T.TEST(A1:A70000,Data!A1:A70000,2,1)", "#DIV/0!"},
		{"CHISQ.TEST(A1:A70000,A2:A70001)", "#VALUE!"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
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
		{"INDEX(A:D,MATCH(2999991,A:A,0),4)", "2999994"},
		{"VLOOKUP(1500001.5,A:D,4)", "1500004"},
		{"SUM(A1:D300000*2)", "3.600018e+12"},
		{"SUMPRODUCT(A:D,A:D/A:D)", "1.800009e+12"},
		{"COUNTIFS(A:D,\">0\")", "1200000"},
	} {
		if got := eval(t, b, English, tc.f).show(); got != tc.want {
			t.Errorf("%s = %s, want %s", tc.f, got, tc.want)
		}
	}
}

func TestSparseArea(t *testing.T) {
	b := book(map[string]any{"A1": 1, "D1048576": 1})
	if got := eval(t, b, English, "SUM(A1:D1048576*1)").show(); got != "#NUM!" {
		t.Errorf("sheet of two far cells: %s", got)
	}
	if got := eval(t, b, English, "SUM(A1:D1048576)").show(); got != "2" {
		t.Errorf("sum of two far cells: %s", got)
	}
}
