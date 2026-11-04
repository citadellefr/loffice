package formula

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// sheetSource is a workbook of values and formulas for an engine, on the
// sheets of mapBook, whose formulas' values it keeps up to date.
type sheetSource struct {
	*mapBook
	formulas map[Pos]string
	// hidden are the rows of the first sheet hidden, true those by a filter.
	hidden map[int]bool
}

func (s *sheetSource) Hidden(sheet, row int) (bool, bool) {
	filtered, hidden := s.hidden[row]
	return sheet == 0 && hidden, sheet == 0 && filtered
}

func (s *sheetSource) Value(p Pos) Value {
	return s.cells[[3]int{p.Sheet, p.Row, p.Col}]
}

func (s *sheetSource) Each(a Area3, f func(row, col int, v Value) bool) {
	s.Cells(a, f)
}

func (s *sheetSource) Name(name string, sheet int) (string, bool) {
	f, ok := s.names[name]
	return f, ok
}

func (s *sheetSource) set(e *Engine, cell string, v any) []Result {
	r, c, _ := ParseCell(cell)
	p := Pos{0, r, c}
	switch v := v.(type) {
	case string:
		if len(v) > 0 && v[0] == '=' {
			s.formulas[p] = v[1:]
			e.Set(p, v[1:], nil)
		} else {
			s.cells[[3]int{0, r, c}] = Str(v)
		}
	case int:
		s.cells[[3]int{0, r, c}] = Num(float64(v))
	}
	if _, ok := v.(string); !ok || v.(string)[0] != '=' {
		delete(s.formulas, p)
		e.Set(p, "", nil)
	}
	out := e.Recalc([]Pos{p})
	for _, x := range out {
		s.cells[[3]int{x.Sheet, x.Row, x.Col}] = x.Value
	}
	return out
}

func newSource() (*sheetSource, *Engine) {
	s := &sheetSource{mapBook: book(nil), formulas: map[Pos]string{}, hidden: map[int]bool{}}
	e := NewEngine(s, Options{Now: func() time.Time { return time.Date(2024, 12, 25, 12, 0, 0, 0, time.UTC) }})
	return s, e
}

func results(out []Result) string {
	var b []string
	for _, r := range out {
		b = append(b, fmt.Sprintf("%s=%s", CellName(r.Row, r.Col), r.Value.show()))
	}
	return fmt.Sprint(b)
}

func TestEngine(t *testing.T) {
	s, e := newSource()
	s.set(e, "A1", 1)
	if got := results(s.set(e, "B1", "=A1*2")); got != "[B1=2]" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "C1", "=SUM(A1:B1)")); got != "[C1=3]" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "A1", 5)); got != "[B1=10 C1=15]" {
		t.Fatal(got)
	}
	// a change that leaves a value alone reports nothing for it
	s.set(e, "D1", "=IF(A1>0,1,0)")
	if got := results(s.set(e, "A1", 6)); got != "[B1=12 C1=18]" {
		t.Fatal(got)
	}
	// a formula taken away is no longer calculated
	delete(s.formulas, Pos{0, 1, 2})
	e.Set(Pos{0, 1, 2}, "", nil)
	if got := results(s.set(e, "A1", 7)); got != "[C1=19]" {
		t.Fatal(got)
	}
	// circles end, reading 0
	s.set(e, "E1", "=F1+1")
	s.set(e, "F1", "=E1+1")
	if got := results(s.set(e, "A1", 8)); got != "[C1=20]" {
		t.Fatal(got)
	}
	// unknown functions keep their values
	s.cells[[3]int{0, 1, 7}] = Num(42)
	s.set(e, "G1", "=NOSUCH(A1)+1")
	s.set(e, "H1", "=G1*2")
	if got := s.Value(Pos{0, 1, 8}).show(); got != "84" {
		t.Fatalf("H1 = %s", got)
	}
	// volatile functions are calculated each time
	s.set(e, "I1", "=NOW()")
	if got := results(s.set(e, "Z9", 1)); got != "[]" {
		t.Fatal(got)
	}
}

func TestEngineSubtotal(t *testing.T) {
	s, e := newSource()
	for i, v := range []int{1, 2, 4, 8} {
		s.set(e, fmt.Sprintf("A%d", i+1), v)
	}
	s.set(e, "A5", "=SUBTOTAL(9,A1:A4)")
	s.set(e, "B1", "=SUBTOTAL(9,A1:A5)")
	s.set(e, "B2", "=SUBTOTAL(109,A1:A5)")
	s.set(e, "B3", "=AGGREGATE(9,5,A1:A5)")
	s.set(e, "B4", "=SUM(A1:A5)")
	if got := results(e.Recalc([]Pos{{0, 1, 1}})); got != "[]" {
		t.Fatal(got)
	}
	// a row hidden by hand, then one by a filter
	s.hidden[2] = false
	out := e.Recalc([]Pos{{0, 2, 0}})
	if got := results(out); got != "[B2=13 B3=28]" {
		t.Fatal(got)
	}
	for _, r := range out {
		s.cells[[3]int{r.Sheet, r.Row, r.Col}] = r.Value
	}
	s.hidden[3] = true
	if got := results(e.Recalc([]Pos{{0, 3, 0}})); got != "[B1=11 B2=9 B3=20 B4=26 A5=11]" {
		t.Fatal(got)
	}
}

func TestEngineArray(t *testing.T) {
	s, e := newSource()
	for i, v := range []int{1, 2, 3} {
		s.set(e, fmt.Sprintf("A%d", i+1), v)
	}
	e.Set(Pos{0, 1, 2}, "A1:A3*10", &Area{R1: 1, C1: 2, R2: 3, C2: 2})
	out := e.Recalc([]Pos{{0, 1, 2}})
	if got := results(out); got != "[B1=10 B2=20 B3=30]" {
		t.Fatal(got)
	}
	for _, r := range out {
		s.cells[[3]int{r.Sheet, r.Row, r.Col}] = r.Value
	}
	s.set(e, "C1", "=SUM(B1:B3)")
	if got := results(s.set(e, "A2", 5)); got != "[C1=90 B2=50]" {
		t.Fatal(got)
	}
}

// A chain of formulas, each reading the one before, is calculated in order
// whatever its length.
func TestEngineChain(t *testing.T) {
	s, e := newSource()
	s.set(e, "A1", 1)
	const n = 20000
	for i := 2; i <= n; i++ {
		p := Pos{0, i, 1}
		e.Set(p, fmt.Sprintf("A%d+1", i-1), nil)
		s.cells[[3]int{0, i, 1}] = Num(float64(i))
	}
	start := time.Now()
	out := s.set(e, "A1", 10)
	if len(out) != n-1 || out[len(out)-1].Value.Num != n+9 {
		t.Fatalf("%d results, last %v", len(out), out[len(out)-1])
	}
	t.Logf("%d formulas in %v", n, time.Since(start))
}

// gridSource finds the cells of an area as the grid of a sheet does, row
// by row.
type gridSource struct {
	*sheetSource
}

func (g gridSource) Each(a Area3, f func(row, col int, v Value) bool) {
	rows, cols := g.Size(a.Sheet)
	for r := a.R1; r <= min(a.R2, rows); r++ {
		for c := a.C1; c <= min(a.C2, cols); c++ {
			if v, ok := g.cells[[3]int{a.Sheet, r, c}]; ok && !f(r, c, v) {
				return
			}
		}
	}
}

func (g gridSource) Size(int) (int, int) {
	return 10000, 2
}

// 10 000 formulas read the cell changed.
func BenchmarkRecalc(b *testing.B) {
	s, _ := newSource()
	e := NewEngine(gridSource{s}, Options{})
	s.cells[[3]int{0, 1, 1}] = Num(1)
	for i := 1; i <= 10000; i++ {
		s.cells[[3]int{0, i, 1}] = Num(float64(i))
		e.Set(Pos{0, i, 2}, fmt.Sprintf("$A$1*%d+SUM(A1:A%d)", i, min(i, 50)), nil)
	}
	for b.Loop() {
		s.cells[[3]int{0, 1, 1}] = Num(float64(b.N))
		if len(e.Recalc([]Pos{{0, 1, 1}})) != 10000 {
			b.Fatal("not all calculated")
		}
	}
}

// recalc calculates what the cells changed reach, and keeps the values.
func (s *sheetSource) recalc(e *Engine, cells ...string) []Result {
	var changed []Pos
	for _, cell := range cells {
		r, c, _ := ParseCell(cell)
		changed = append(changed, Pos{0, r, c})
	}
	out := e.Recalc(changed)
	for _, x := range out {
		s.cells[[3]int{x.Sheet, x.Row, x.Col}] = x.Value
	}
	return out
}

func (s *sheetSource) dynamic(e *Engine, cell, f string) []Result {
	r, c, _ := ParseCell(cell)
	e.SetDynamic(Pos{0, r, c}, f, e.Spill(Pos{0, r, c}))
	return s.recalc(e, cell)
}

func spillOf(out []Result) string {
	for _, r := range out {
		if r.Spill != nil {
			return CellName(r.Row, r.Col) + ":" + r.Spill.String()
		}
	}
	return ""
}

func TestEngineSpillArrays(t *testing.T) {
	s, e := newSource()
	out := s.dynamic(e, "A1", `TEXTSPLIT("a,b;c,d",",",";")`)
	if got := results(out); got != `[A1="a" B1="b" A2="c" B2="d"]` {
		t.Fatal(got)
	}
	if got := spillOf(out); got != "A1:A1:B2" {
		t.Fatal(got)
	}
	out = s.dynamic(e, "D1", "VSTACK(SEQUENCE(1,3),SEQUENCE(1,2))")
	if got := results(out); got != "[D1=1 E1=2 F1=3 D2=1 E2=2 F2=#N/A]" {
		t.Fatal(got)
	}
	if got := results(s.dynamic(e, "H1", "TAKE(SEQUENCE(3,3),-2,-2)")); got != "[H1=5 I1=6 H2=8 I2=9]" {
		t.Fatal(got)
	}
}

func TestEngineSpill(t *testing.T) {
	s, e := newSource()
	s.set(e, "C1", "=A2*10")
	s.set(e, "D1", "=A6")
	out := s.dynamic(e, "A1", "SEQUENCE(3)")
	if got := results(out); got != "[A1=1 C1=20 A2=2 A3=3]" {
		t.Fatal(got)
	}
	if got := spillOf(out); got != "A1:A1:A3" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "B1", "=SUM(A1#)")); got != "[B1=6]" {
		t.Fatal(got)
	}
	// the spill grows over a cell another formula reads
	out = s.dynamic(e, "A1", "SEQUENCE(6)")
	if got := results(out); got != "[A1=1 B1=21 D1=6 A4=4 A5=5 A6=6]" {
		t.Fatal(got)
	}
	if got := spillOf(out); got != "A1:A1:A6" {
		t.Fatal(got)
	}
	// a value in the way blocks it, and its going frees it
	s.cells[[3]int{0, 3, 1}] = Str("x")
	out = s.recalc(e, "A3")
	if got := results(out); got != "[A1=#SPILL! B1=#SPILL! C1=0 D1=0 A2=blank A4=blank A5=blank A6=blank]" {
		t.Fatal(got)
	}
	if got := spillOf(out); got != "A1:A1" {
		t.Fatal(got)
	}
	delete(s.cells, [3]int{0, 3, 1})
	if got := results(s.recalc(e, "A3")); got != "[A1=1 B1=21 C1=20 D1=6 A2=2 A3=3 A4=4 A5=5 A6=6]" {
		t.Fatal(got)
	}
	// a formula in the way, or another spill
	if got := results(s.set(e, "A5", "=1")); got != "[A1=#SPILL! B1=#SPILL! C1=0 D1=0 A2=blank A3=blank A4=blank A5=1 A6=blank]" {
		t.Fatal(got)
	}
	e.Set(Pos{0, 5, 1}, "", nil)
	delete(s.cells, [3]int{0, 5, 1})
	if got := results(s.recalc(e, "A5")); got != "[A1=1 B1=21 C1=20 D1=6 A2=2 A3=3 A4=4 A5=5 A6=6]" {
		t.Fatal(got)
	}
	if got := results(s.dynamic(e, "A4", "{1;2}")); got != "[A1=#SPILL! B1=#SPILL! C1=0 D1=0 A2=blank A3=blank A4=1 A5=2 A6=blank]" {
		t.Fatal(got)
	}
	// shrinking the one in the way
	delete(s.cells, [3]int{0, 4, 1})
	if got := results(s.dynamic(e, "A4", "")); got != "[A1=1 B1=21 C1=20 D1=6 A2=2 A3=3 A4=4 A5=5 A6=6]" {
		t.Fatal(got)
	}
	if got := results(s.dynamic(e, "A1", "SEQUENCE(2)")); got != "[A1=1 B1=3 D1=0 A3=blank A4=blank A5=blank A6=blank]" {
		t.Fatal(got)
	}
	// a formula taken away leaves no values behind
	e.SetDynamic(Pos{0, 1, 1}, "", nil)
	delete(s.cells, [3]int{0, 1, 1})
	if got := results(s.recalc(e, "A1")); got != "[B1=#REF! C1=0 A2=blank]" {
		t.Fatal(got)
	}
	// a range spills, an edge too close does not, A1# of a value is #REF!
	s.set(e, "F1", 1)
	s.set(e, "F2", 2)
	if got := results(s.dynamic(e, "G1", "F1:F2*2")); got != "[G1=2 G2=4]" {
		t.Fatal(got)
	}
	if got := results(s.dynamic(e, "H1048575", "SEQUENCE(3)")); got != "[H1048575=#SPILL!]" {
		t.Fatal(got)
	}
	if got := results(s.dynamic(e, "I1", "SUM(F1#)+ROWS(_xlfn.ANCHORARRAY(G1))")); got != "[I1=#REF!]" {
		t.Fatal(got)
	}
	if got := results(s.dynamic(e, "I1", "ROWS(_xlfn.ANCHORARRAY(G1))+_xlfn.SINGLE(F1:F2)")); got != "[I1=3]" {
		t.Fatal(got)
	}
}

func (s *sheetSource) show(cell string) string {
	r, c, _ := ParseCell(cell)
	return s.Value(Pos{0, r, c}).show()
}

func circular(e *Engine) string {
	var b []string
	for _, p := range e.Circular() {
		b = append(b, CellName(p.Row, p.Col))
	}
	return fmt.Sprint(b)
}

// The formulas of a circle hold 0, as in Excel, whatever the order they
// are calculated in; those reading them read 0.
func TestEngineCircle(t *testing.T) {
	for _, order := range [][]string{{"A1", "B1", "C1"}, {"C1", "B1", "A1"}, {"B1", "C1", "A1"}} {
		s, e := newSource()
		s.set(e, "D1", 5)
		formulas := map[string]string{"A1": "=B1+D1", "B1": "=C1*2", "C1": "=A1+1"}
		for _, cell := range order {
			s.set(e, cell, formulas[cell])
		}
		s.set(e, "E1", "=A1+B1+C1+D1")
		if got := s.show("A1") + " " + s.show("B1") + " " + s.show("C1") + " " + s.show("E1"); got != "0 0 0 5" {
			t.Fatalf("%v: %s", order, got)
		}
		if got := results(s.set(e, "D1", 7)); got != "[E1=7]" {
			t.Fatalf("%v: %s", order, got)
		}
		if got := circular(e); got != "[A1 B1 C1]" {
			t.Fatalf("%v: %s", order, got)
		}
	}
}

func TestEngineCircleThrough(t *testing.T) {
	s, e := newSource()
	s.mapBook.names["Total"] = "Feuil1!$C$1"
	for cell, v := range map[string]any{"A1": "=SUM(A2:A4)", "A2": 1, "A3": "=A1*2", "B1": "=B1+1",
		"C1": "=Total+1", "D1": "=ROW(D1)+COLUMN(D1:D3)", "E1": `=INDIRECT("E2")+1`, "E2": "=E1",
		"F1": `=CELL("width",F:F)`} {
		s.set(e, cell, v)
	}
	for cell, want := range map[string]string{"A1": "0", "A3": "0", "B1": "0", "C1": "0", "D1": "5", "E1": "0", "E2": "0"} {
		if got := s.show(cell); got != want {
			t.Errorf("%s = %s, want %s", cell, got, want)
		}
	}
	if got := circular(e); got != "[A1 B1 C1 E1 E2 A3]" {
		t.Fatal(got)
	}
	// the references alone tell the circles of formulas not calculated
	fresh := NewEngine(s, Options{})
	for p, f := range s.formulas {
		fresh.Set(p, f, nil)
	}
	if got := circular(fresh); got != "[A1 B1 C1 A3]" {
		t.Fatal(got)
	}
}

// A workbook read with its spills holds their values: the engine takes
// them as its own, and a spill saved blocked spills once the way is free.
func TestEngineSpillRead(t *testing.T) {
	s, e := newSource()
	for i, v := range []int{1, 2, 3} {
		s.cells[[3]int{0, i + 1, 1}] = Num(float64(v))
	}
	e.SetDynamic(Pos{0, 1, 1}, "SEQUENCE(4)", &Area{R1: 1, C1: 1, R2: 3, C2: 1})
	if got := results(s.recalc(e, "A1")); got != "[A1=1 A4=4]" {
		t.Fatal(got)
	}
	s.cells[[3]int{0, 1, 2}] = ErrSpill
	s.cells[[3]int{0, 3, 2}] = Str("x")
	e.SetDynamic(Pos{0, 1, 2}, "SEQUENCE(3)", nil)
	delete(s.cells, [3]int{0, 3, 2})
	out := s.recalc(e, "B3")
	if got := results(out); got != "[B1=1 B2=2 B3=3]" {
		t.Fatal(got)
	}
	if got := spillOf(out); got != "B1:B1:B3" {
		t.Fatal(got)
	}
}

// An edit that ends a circle gives its formulas their values back.
func TestEngineCircleBroken(t *testing.T) {
	s, e := newSource()
	s.set(e, "A1", 1)
	s.set(e, "B1", "=A1+1")
	s.set(e, "C1", "=B1*10")
	if got := results(s.set(e, "A1", "=C1")); got != "[A1=0 B1=0 C1=0]" {
		t.Fatal(got)
	}
	if got := circular(e); got != "[A1 B1 C1]" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "A1", 3)); got != "[B1=4 C1=40]" {
		t.Fatal(got)
	}
	if got := circular(e); got != "[]" {
		t.Fatal(got)
	}
	// one read through INDIRECT ends as the text it reads changes
	s.set(e, "D1", "B1")
	s.set(e, "B1", `=INDIRECT(D1)+1`)
	if got := s.show("B1") + " " + circular(e); got != "0 [B1]" {
		t.Fatal(got)
	}
	s.set(e, "D1", "A1")
	if got := s.show("B1") + " " + s.show("C1") + " " + circular(e); got != "4 40 []" {
		t.Fatal(got)
	}
}

// With iteration a circle is calculated again until it settles, or as
// many times as the workbook allows: here interest on a loan that pays
// for it.
func TestEngineIteration(t *testing.T) {
	for _, c := range []struct {
		it       Iteration
		interest float64
	}{
		{Iteration{Count: 100, Delta: 0.001}, 1000 * 0.05 / 0.95},
		{Iteration{Count: 1, Delta: 0.001}, 50},
		{Iteration{Count: 2, Delta: 0.001}, 52.5},
		{Iteration{Count: 100, Delta: 100}, 52.5},
	} {
		s := &sheetSource{mapBook: book(nil), formulas: map[Pos]string{}, hidden: map[int]bool{}}
		e := NewEngine(s, Options{Iteration: &c.it})
		s.set(e, "A1", 1000)
		s.set(e, "B1", "=A1+C1")
		s.set(e, "C1", "=B1*5%")
		s.set(e, "D1", "=C1*2")
		interest := s.Value(Pos{0, 1, 3}).Num
		if math.Abs(interest-c.interest) > c.it.Delta || s.Value(Pos{0, 1, 4}).Num != 2*interest {
			t.Errorf("%+v: interest %v, D1 %s", c.it, interest, s.show("D1"))
		}
		if got := circular(e); got != "[B1 C1]" {
			t.Fatal(got)
		}
	}
	// through INDIRECT, known once calculated
	s := &sheetSource{mapBook: book(nil), formulas: map[Pos]string{}, hidden: map[int]bool{}}
	e := NewEngine(s, Options{Iteration: &Iteration{Count: 100, Delta: 1e-9}})
	s.set(e, "A1", `=INDIRECT("B1")/2+1`)
	s.set(e, "B1", "=A1")
	if a := s.Value(Pos{0, 1, 1}).Num; math.Abs(a-2) > 1e-6 {
		t.Fatalf("A1 = %v", a)
	}
}
