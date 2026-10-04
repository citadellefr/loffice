package xlsx

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

// looksType is the node under a sheet whose grid holds the looks its
// conditional formats give its cells, which the server alone sets.
const looksType = "looks"

func looksID(sheet string) string {
	return sheet + ".cf"
}

// A look is what the conditional formats of a sheet give a cell: R the
// rules whose styles it takes, in order; S the place of its value on a
// color scale, [rule, 0 to 1]; B the length of a data bar, [rule, percent
// of the cell]; I an icon, [rule, index in its set].
type look struct {
	R []int     `json:"r,omitempty"`
	S []float64 `json:"s,omitempty"`
	B []float64 `json:"b,omitempty"`
	I []int     `json:"i,omitempty"`
}

// marshal writes a look as JSON, its fields in the order the grid keeps
// them.
func (l *look) marshal() json.RawMessage {
	b := []byte{'{'}
	list := func(key string, v []float64) {
		if v == nil {
			return
		}
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = append(b, key...)
		b = append(b, `":[`...)
		for i, x := range v {
			if i > 0 {
				b = append(b, ',')
			}
			b = strconv.AppendFloat(b, x, 'f', -1, 64)
		}
		b = append(b, ']')
	}
	ints := func(v []int) []float64 {
		if v == nil {
			return nil
		}
		out := make([]float64, len(v))
		for i, x := range v {
			out[i] = float64(x)
		}
		return out
	}
	list("b", l.B)
	list("i", ints(l.I))
	list("r", ints(l.R))
	list("s", l.S)
	return append(b, '}')
}

// settled is the workbook as the last calculation left it, which the
// formulas of conditional formats read.
type settled struct {
	c     *Calc
	names map[string]formula.Expr
}

func (s *settled) Cell(sheet, row, col int) formula.Value {
	return s.c.Value(formula.Pos{Sheet: sheet, Row: row, Col: col})
}

func (s *settled) Cells(a formula.Area3, f func(row, col int, v formula.Value) bool) {
	s.c.Each(a, f)
}

func (s *settled) Size(sheet int) (int, int) {
	return s.c.Size(sheet)
}

func (s *settled) Sheets(first, last string) ([]int, bool) {
	return s.c.Sheets(first, last)
}

func (s *settled) Name(name string, sheet int) (formula.Expr, bool) {
	key := strings.ToUpper(name) + "!" + strconv.Itoa(sheet)
	if x, ok := s.names[key]; ok {
		return x, x != nil
	}
	var x formula.Expr
	if text, ok := s.c.Name(name, sheet); ok {
		x, _ = formula.Parse(text)
	}
	s.names[key] = x
	return x, x != nil
}

// looks calculates what the conditional formats of a sheet give its
// cells, those it holds or that lie among them.
func (c *Calc) looks(id string) map[[2]int]*look {
	n := c.tree.Node(id)
	h, ok := c.handles[id]
	var rules []Conditional
	if n != nil {
		_ = json.Unmarshal(n.Attrs[conditionalKey], &rules)
	}
	if !ok || len(rules) == 0 {
		return nil
	}
	e := &evaluation{c: c, book: &settled{c: c, names: map[string]formula.Expr{}}, sheet: h, out: map[[2]int]*look{}, stopped: map[[2]int]bool{}}
	e.rows, e.cols = c.Size(h)
	for i, r := range rules {
		e.rule(i, r)
	}
	for at, l := range e.out {
		if l.R == nil && l.S == nil && l.B == nil && l.I == nil {
			delete(e.out, at)
		}
	}
	return e.out
}

// reachesOut tells whether rules read other sheets than their own, whose
// changes may change their looks.
func reachesOut(rules []Conditional) bool {
	for _, r := range rules {
		texts := slices.Clone(r.F)
		for _, s := range r.Stops {
			texts = append(texts, s.Val)
		}
		for _, f := range texts {
			tokens, err := formula.Tokens(f)
			if err != nil || strings.Contains(strings.ToUpper(f), "INDIRECT") {
				return true
			}
			for _, t := range tokens {
				if t.Kind == formula.Name || t.Kind == formula.Reference && t.Ref.Sheet != "" {
					return true
				}
			}
		}
	}
	return false
}

type evaluation struct {
	c          *Calc
	book       *settled
	sheet      int
	rows, cols int
	out        map[[2]int]*look
	// stopped are the cells a rule that holds left out of the next ones.
	stopped map[[2]int]bool
}

func (e *evaluation) at(row, col int) *look {
	l := e.out[[2]int{row, col}]
	if l == nil {
		l = &look{}
		e.out[[2]int{row, col}] = l
	}
	return l
}

// cells calls f with each cell of the areas of a rule up to the last row
// and column holding cells.
func (e *evaluation) cells(ref string, f func(row, col int)) {
	for _, s := range strings.Fields(ref) {
		a, ok := formula.ParseArea(s)
		if !ok {
			continue
		}
		for row := a.R1; row <= min(a.R2, e.rows); row++ {
			for col := a.C1; col <= min(a.C2, e.cols); col++ {
				if !e.stopped[[2]int{row, col}] {
					f(row, col)
				}
			}
		}
	}
}

// numbers are the numbers of the cells of a rule.
func (e *evaluation) numbers(ref string) []float64 {
	var out []float64
	e.cells(ref, func(row, col int) {
		if v := e.value(row, col); v.Type == formula.TypeNumber {
			out = append(out, v.Num)
		}
	})
	return out
}

func (e *evaluation) value(row, col int) formula.Value {
	return e.c.Value(formula.Pos{Sheet: e.sheet, Row: row, Col: col})
}

// eval calculates a formula of a rule written for cell r0 c0 in cell row
// col.
func (e *evaluation) eval(x formula.Expr, r0, c0, row, col int) formula.Value {
	ctx := &formula.Context{Book: e.book, Sheet: e.sheet, Row: row, Col: col, Locale: e.c.opt.Locale,
		Now: e.c.opt.Now(), Rand: e.c.opt.Rand, Date1904: e.c.opt.Date1904, MoveRows: row - r0, MoveCols: col - c0}
	return ctx.Result(x)
}

func (e *evaluation) rule(i int, r Conditional) {
	holds := func(row, col int) {
		l := e.at(row, col)
		l.R = append(l.R, i)
		if r.Stop {
			e.stopped[[2]int{row, col}] = true
		}
	}
	switch r.Type {
	case "formula":
		x, err := formula.Parse(r.F[0])
		if err != nil {
			return
		}
		first, ok := formula.ParseArea(strings.Fields(r.Ref)[0])
		if !ok {
			return
		}
		e.cells(r.Ref, func(row, col int) {
			if truth(e.eval(x, first.R1, first.C1, row, col)) {
				holds(row, col)
			}
		})
	case "top10":
		values := e.numbers(r.Ref)
		n := r.Rank
		if r.Percent {
			n = len(values) * r.Rank / 100
		}
		n = min(max(n, 1), len(values))
		if n == 0 {
			return
		}
		slices.Sort(values)
		if !r.Bottom {
			slices.Reverse(values)
		}
		limit := values[n-1]
		e.cells(r.Ref, func(row, col int) {
			if v := e.value(row, col); v.Type == formula.TypeNumber && (!r.Bottom && v.Num >= limit || r.Bottom && v.Num <= limit) {
				holds(row, col)
			}
		})
	case "aboveAverage":
		values := e.numbers(r.Ref)
		if len(values) == 0 {
			return
		}
		mean, dev := stats(values)
		limit := mean + float64(r.StdDev)*dev
		if r.Below {
			limit = mean - float64(r.StdDev)*dev
		}
		e.cells(r.Ref, func(row, col int) {
			v := e.value(row, col)
			if v.Type != formula.TypeNumber {
				return
			}
			if !r.Below && (v.Num > limit || r.Equal && v.Num == limit) || r.Below && (v.Num < limit || r.Equal && v.Num == limit) {
				holds(row, col)
			}
		})
	case "duplicateValues", "uniqueValues":
		count := map[string]int{}
		e.cells(r.Ref, func(row, col int) {
			if k, ok := valueKey(e.value(row, col)); ok {
				count[k]++
			}
		})
		e.cells(r.Ref, func(row, col int) {
			if k, ok := valueKey(e.value(row, col)); ok && (count[k] > 1) == (r.Type == "duplicateValues") {
				holds(row, col)
			}
		})
	case "colorScale", "dataBar", "iconSet":
		e.scale(i, r)
	}
}

// scale places the numbers of a rule between its thresholds.
func (e *evaluation) scale(i int, r Conditional) {
	values := e.numbers(r.Ref)
	if len(values) == 0 {
		return
	}
	slices.Sort(values)
	first, _ := formula.ParseArea(strings.Fields(r.Ref)[0])
	limits := make([]float64, len(r.Stops))
	for k, s := range r.Stops {
		limits[k] = e.threshold(s, values, first.R1, first.C1)
	}
	e.cells(r.Ref, func(row, col int) {
		v := e.value(row, col)
		if v.Type != formula.TypeNumber {
			return
		}
		// a cell shows the first scale, bar and icon of its rules
		l := e.at(row, col)
		switch {
		case r.Type == "colorScale" && l.S == nil:
			l.S = []float64{float64(i), round(scaled(v.Num, limits))}
		case r.Type == "dataBar" && l.B == nil:
			lo, hi := limits[0], limits[1]
			t := 0.0
			if hi > lo {
				t = min(max((v.Num-lo)/(hi-lo), 0), 1)
			} else if v.Num >= hi {
				t = 1
			}
			l.B = []float64{float64(i), round(float64(r.MinLen) + t*float64(r.MaxLen-r.MinLen))}
		case r.Type == "iconSet" && l.I == nil:
			k := 0
			for j := 1; j < len(limits); j++ {
				if v.Num > limits[j] || !r.Stops[j].Above && v.Num == limits[j] {
					k = j
				}
			}
			if r.Reverse {
				k = len(limits) - 1 - k
			}
			l.I = []int{i, k}
		}
	})
}

// threshold is the value where a scale changes: values are the numbers
// of its cells, in order.
func (e *evaluation) threshold(s Threshold, values []float64, r0, c0 int) float64 {
	lo, hi := values[0], values[len(values)-1]
	var n float64
	if s.Type != "min" && s.Type != "max" {
		if x, err := formula.Parse(s.Val); err == nil {
			if v := e.eval(x, r0, c0, r0, c0); v.Type == formula.TypeNumber {
				n = v.Num
			}
		}
	}
	switch s.Type {
	case "min":
		return lo
	case "max":
		return hi
	case "autoMin":
		return min(lo, 0)
	case "autoMax":
		return max(hi, 0)
	case "percent":
		return lo + (hi-lo)*n/100
	case "percentile":
		p := min(max(n/100, 0), 1) * float64(len(values)-1)
		k := int(p)
		if k+1 >= len(values) {
			return hi
		}
		return values[k] + (p-float64(k))*(values[k+1]-values[k])
	}
	return n
}

// scaled is where v lies on a scale of two or three colors, the middle
// one at 0.5.
func scaled(v float64, limits []float64) float64 {
	part := func(v, lo, hi float64) float64 {
		if hi <= lo {
			if v >= hi {
				return 1
			}
			return 0
		}
		return min(max((v-lo)/(hi-lo), 0), 1)
	}
	if len(limits) == 2 {
		return part(v, limits[0], limits[1])
	}
	if v <= limits[1] {
		return part(v, limits[0], limits[1]) / 2
	}
	return 0.5 + part(v, limits[1], limits[2])/2
}

func round(f float64) float64 {
	return math.Round(f*1000) / 1000
}

// stats are the mean and the standard deviation of a sample.
func stats(values []float64) (float64, float64) {
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	if len(values) < 2 {
		return mean, 0
	}
	var sq float64
	for _, v := range values {
		sq += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(sq / float64(len(values)-1))
}

// truth is whether what a formula gives makes a rule hold: TRUE or a
// number other than 0.
func truth(v formula.Value) bool {
	return (v.Type == formula.TypeNumber || v.Type == formula.TypeBool) && v.Num != 0
}

// valueKey is a value as duplicates compare them, text whatever its case.
func valueKey(v formula.Value) (string, bool) {
	switch v.Type {
	case formula.TypeNumber:
		return "n" + strconv.FormatFloat(v.Num, 'g', -1, 64), true
	case formula.TypeText:
		return "t" + strings.ToLower(v.Str), v.Str != ""
	case formula.TypeBool:
		return "b" + strconv.FormatFloat(v.Num, 'g', -1, 64), true
	}
	return "", false
}

// looksChanges are the changes that give the looks node of a sheet the
// looks calculated: created when it has none, cells set and cleared.
func (c *Calc) looksChanges(id string, looks map[[2]int]*look) ot.Edit {
	node := c.tree.Node(looksID(id))
	var cells []ot.Cell
	for at, l := range looks {
		raw := l.marshal()
		if node == nil || string(node.Grid.Cell(at[0], at[1])) != string(raw) {
			cells = append(cells, ot.Cell{Row: at[0], Col: at[1], Fields: raw})
		}
	}
	if node != nil {
		node.Grid.Each(0, formula.MaxRows, func(row, col int, _ json.RawMessage) bool {
			if looks[[2]int{row, col}] == nil {
				cells = append(cells, ot.Cell{Row: row, Col: col, Fields: json.RawMessage(`{"r":null,"s":null,"b":null,"i":null}`)})
			}
			return true
		})
	}
	if len(cells) == 0 {
		return nil
	}
	slices.SortFunc(cells, func(a, b ot.Cell) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	if node == nil {
		return ot.Edit{{Op: ot.OpNew, ID: looksID(id), Type: looksType, Parent: id, Key: "V", Cells: cells}}
	}
	return ot.Edit{{Op: ot.OpCel, ID: looksID(id), Cells: cells}}
}
