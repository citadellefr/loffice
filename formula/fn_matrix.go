package formula

import (
	"math"
	"sort"
)

func init() {
	add(map[string]*function{
		"MMULT": fn(2, 2, func(c *Context, v []Value) Value {
			a, err := c.matrix(v[0])
			if err != nil {
				return *err
			}
			b, err := c.matrix(v[1])
			if err != nil {
				return *err
			}
			if a.cols != b.rows {
				return ErrValue
			}
			out := newMatrix(a.rows, b.cols)
			for i := range a.rows {
				for j := range b.cols {
					s := 0.0
					for k := range a.cols {
						s += a.at(i, k) * b.at(k, j)
					}
					out.a[i*out.cols+j] = s
				}
			}
			return out.value()
		}),
		"MDETERM": fn(1, 1, func(c *Context, v []Value) Value {
			m, err := c.matrix(v[0])
			if err != nil {
				return *err
			}
			if m.rows != m.cols {
				return ErrValue
			}
			d, _ := m.determinant()
			return Num(d)
		}),
		"MINVERSE": fn(1, 1, func(c *Context, v []Value) Value {
			m, err := c.matrix(v[0])
			if err != nil {
				return *err
			}
			if m.rows != m.cols {
				return ErrValue
			}
			inv, ok := m.inverse()
			if !ok {
				return ErrNum
			}
			return inv.value()
		}),
		"MUNIT": fn(1, 1, func(c *Context, v []Value) Value {
			n, err := c.number(v[0])
			if err != nil {
				return *err
			}
			n = math.Trunc(n)
			if n < 1 || n*n > maxCells {
				return ErrValue
			}
			m := newMatrix(int(n), int(n))
			for i := range m.rows {
				m.a[i*m.cols+i] = 1
			}
			return m.value()
		}),
		"RSQ": pairs(func(ys, xs []float64) Value {
			r := correl(ys, xs)
			if r.Type == TypeError {
				return r
			}
			return Num(r.Num * r.Num)
		}),
		"FREQUENCY": fn(2, 2, frequency),
		"LINEST":    {min: 1, max: 4, call: regress(false, false)},
		"LOGEST":    {min: 1, max: 4, call: regress(true, false)},
		"TREND":     {min: 1, max: 4, call: regress(false, true)},
		"GROWTH":    {min: 1, max: 4, call: regress(true, true)},
	})
}

// matrix is a table of numbers, row by row.
type matrix struct {
	rows, cols int
	a          []float64
}

func newMatrix(rows, cols int) *matrix {
	return &matrix{rows: rows, cols: cols, a: make([]float64, rows*cols)}
}

func (m *matrix) at(i, j int) float64 {
	return m.a[i*m.cols+j]
}

func (m *matrix) clone() *matrix {
	return &matrix{rows: m.rows, cols: m.cols, a: append([]float64(nil), m.a...)}
}

func (m *matrix) value() Value {
	t := &Table{Rows: m.rows, Cols: m.cols, Cells: make([]Value, len(m.a))}
	for i, x := range m.a {
		t.Cells[i] = Num(x)
	}
	return Value{Type: TypeArray, Arr: t}
}

// matrix reads an array of numbers: #VALUE! when a cell is not a number,
// the error of a cell that is one.
func (c *Context) matrix(v Value) (*matrix, *Value) {
	t := c.array(v)
	if len(t.Cells) == 0 {
		return nil, &ErrValue
	}
	m := &matrix{rows: t.Rows, cols: t.Cols, a: make([]float64, len(t.Cells))}
	for i, x := range t.Cells {
		switch x.Type {
		case TypeNumber:
			m.a[i] = x.Num
		case TypeError:
			return nil, &x
		default:
			return nil, &ErrValue
		}
	}
	return m, nil
}

// determinant is the determinant of a square matrix, and false when it is
// singular.
func (m *matrix) determinant() (float64, bool) {
	a := m.clone()
	scale := a.largest()
	det := 1.0
	for k := range a.rows {
		p := k
		for i := k + 1; i < a.rows; i++ {
			if math.Abs(a.at(i, k)) > math.Abs(a.at(p, k)) {
				p = i
			}
		}
		if math.Abs(a.at(p, k)) <= singular*scale {
			return 0, false
		}
		if p != k {
			a.swapRows(p, k)
			det = -det
		}
		det *= a.at(k, k)
		for i := k + 1; i < a.rows; i++ {
			f := a.at(i, k) / a.at(k, k)
			for j := k; j < a.cols; j++ {
				a.a[i*a.cols+j] -= f * a.at(k, j)
			}
		}
	}
	return det, true
}

// singular is how small a pivot is, against the largest of the matrix,
// for the matrix to be taken as singular.
const singular = 1e-14

func (m *matrix) largest() float64 {
	s := 0.0
	for _, x := range m.a {
		s = math.Max(s, math.Abs(x))
	}
	return s
}

func (m *matrix) swapRows(i, j int) {
	for k := range m.cols {
		m.a[i*m.cols+k], m.a[j*m.cols+k] = m.a[j*m.cols+k], m.a[i*m.cols+k]
	}
}

// inverse is the inverse of a square matrix, false when it is singular.
func (m *matrix) inverse() (*matrix, bool) {
	n := m.rows
	a, inv := m.clone(), newMatrix(n, n)
	for i := range n {
		inv.a[i*n+i] = 1
	}
	scale := a.largest()
	for k := range n {
		p := k
		for i := k + 1; i < n; i++ {
			if math.Abs(a.at(i, k)) > math.Abs(a.at(p, k)) {
				p = i
			}
		}
		if math.Abs(a.at(p, k)) <= singular*scale {
			return nil, false
		}
		a.swapRows(p, k)
		inv.swapRows(p, k)
		pivot := a.at(k, k)
		for j := range n {
			a.a[k*n+j] /= pivot
			inv.a[k*n+j] /= pivot
		}
		for i := range n {
			if i == k {
				continue
			}
			f := a.at(i, k)
			for j := range n {
				a.a[i*n+j] -= f * a.at(k, j)
				inv.a[i*n+j] -= f * inv.at(k, j)
			}
		}
	}
	return inv, true
}

// frequency counts the data in the intervals the bins end: the values up
// to each bin and past the earlier ones, then those past the last.
func frequency(c *Context, v []Value) Value {
	data, err := c.numbersIn(v[0])
	if err != nil {
		return *err
	}
	bins, err := c.numbersIn(v[1])
	if err != nil {
		return *err
	}
	if len(bins) == 0 {
		return Num(float64(len(data)))
	}
	order := make([]int, len(bins))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool { return bins[order[i]] < bins[order[j]] })
	counts := make([]float64, len(bins)+1)
	for _, x := range data {
		k := sort.Search(len(order), func(i int) bool { return bins[order[i]] >= x })
		if k < len(order) {
			k = order[k]
		} else {
			k = len(bins)
		}
		counts[k]++
	}
	m := &matrix{rows: len(counts), cols: 1, a: counts}
	return m.value()
}

// fit is a least squares regression of y on the columns of x.
type fit struct {
	coef         []float64 // by column of x, 0 for a column the others explain
	se           []float64
	intercept    float64
	seIntercept  float64
	constant     bool
	r2, sey, f   float64
	df           float64
	ssReg, ssRes float64
}

// linearFit finds the coefficients of y ~ x by orthogonalizing the
// columns of x, after centering them when the regression has a constant.
func linearFit(y []float64, x *matrix, constant bool) *fit {
	n, k := len(y), x.cols
	cols := make([][]float64, k)
	means := make([]float64, k)
	for j := range cols {
		cols[j] = make([]float64, n)
		for i := range n {
			cols[j][i] = x.at(i, j)
		}
	}
	yc := append([]float64(nil), y...)
	my := 0.0
	if constant {
		for j := range cols {
			means[j] = sumOf(cols[j]) / float64(n)
			for i := range cols[j] {
				cols[j][i] -= means[j]
			}
		}
		my = sumOf(y) / float64(n)
		for i := range yc {
			yc[i] -= my
		}
	}

	var used []int
	var q [][]float64
	r := make([][]float64, k)
	for j := range r {
		r[j] = make([]float64, k)
	}
	for j := range cols {
		v := append([]float64(nil), cols[j]...)
		first := norm(v)
		for range 2 {
			for t, i := range used {
				d := dot(q[t], v)
				r[i][j] += d
				for s := range v {
					v[s] -= d * q[t][s]
				}
			}
		}
		if rest := norm(v); first > 0 && rest > 1e-10*first {
			r[j][j] = rest
			for s := range v {
				v[s] /= rest
			}
			q, used = append(q, v), append(used, j)
		}
	}

	f := &fit{coef: make([]float64, k), se: make([]float64, k), constant: constant}
	beta := make([]float64, len(used))
	for t := len(used) - 1; t >= 0; t-- {
		s := dot(q[t], yc)
		for u := t + 1; u < len(used); u++ {
			s -= r[used[t]][used[u]] * beta[u]
		}
		beta[t] = s / r[used[t]][used[t]]
	}
	for t, j := range used {
		f.coef[j] = beta[t]
	}
	f.intercept = my
	for j := range cols {
		f.intercept -= f.coef[j] * means[j]
	}

	total := dot(yc, yc)
	for i := range n {
		e := yc[i]
		for j := range cols {
			e -= f.coef[j] * cols[j][i]
		}
		f.ssRes += e * e
	}
	f.ssReg = total - f.ssRes
	f.r2 = f.ssReg / total
	f.df = float64(n - len(used))
	if constant {
		f.df--
	}
	if f.df > 0 {
		f.sey = math.Sqrt(f.ssRes / f.df)
		f.f = f.ssReg / float64(len(used)) / (f.ssRes / f.df)
	}
	if f.df <= 0 {
		return f
	}
	m := len(used)
	inv := make([][]float64, m)
	for t := range inv {
		inv[t] = make([]float64, m)
	}
	for u := range m {
		inv[u][u] = 1 / r[used[u]][used[u]]
		for t := u - 1; t >= 0; t-- {
			s := 0.0
			for w := t + 1; w <= u; w++ {
				s += r[used[t]][used[w]] * inv[w][u]
			}
			inv[t][u] = -s / r[used[t]][used[t]]
		}
	}
	for t, j := range used {
		f.se[j] = f.sey * norm(inv[t])
	}
	if constant {
		spread := 0.0
		for u := range m {
			w := 0.0
			for t := range m {
				w += inv[t][u] * means[used[t]]
			}
			spread += w * w
		}
		f.seIntercept = f.sey * math.Sqrt(1/float64(n)+spread)
	}
	return f
}

func dot(a, b []float64) float64 {
	s := 0.0
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

func norm(a []float64) float64 {
	return math.Sqrt(dot(a, a))
}

// regress is LINEST and its kin: the coefficients of the line, or of the
// exponential curve when logarithmic, or what they predict for new values
// of x when predicting.
func regress(logarithmic, predicting bool) func(c *Context, args []Expr) Value {
	return func(c *Context, args []Expr) Value {
		v := make([]Value, 4)
		for i, a := range args {
			v[i] = c.Eval(a)
		}
		constant, stats := true, false
		var newX Value
		if predicting {
			newX, v[2], v[3] = v[2], v[3], Value{}
		}
		for i, flag := range []*bool{&constant, &stats} {
			if v[2+i].Type == TypeBlank {
				continue
			}
			b, err := c.boolean(v[2+i])
			if err != nil {
				return *err
			}
			*flag = b
		}
		y, x, err := c.regressors(v[0], v[1])
		if err != nil {
			return *err
		}
		if logarithmic {
			for i, n := range y.a {
				if n <= 0 {
					return ErrNum
				}
				y.a[i] = math.Log(n)
			}
		}
		f := linearFit(y.a, x.m, constant)
		if predicting {
			return c.predict(f, x, newX, logarithmic)
		}
		return f.table(stats, logarithmic)
	}
}

// regressors are the known x of a regression: an observation by row.
type regressors struct {
	m          *matrix
	byRow      bool // the observations lie along a row
	rows, cols int  // of the values given
}

// regressors reads the known y, a row or a column, and the known x, whose
// columns are the variables when y is a column, rows when it is a row;
// 1, 2, 3... when there is none.
func (c *Context) regressors(vy, vx Value) (*matrix, *regressors, *Value) {
	y, err := c.matrix(vy)
	if err != nil {
		return nil, nil, err
	}
	if y.rows != 1 && y.cols != 1 {
		return nil, nil, &ErrRef
	}
	n := len(y.a)
	r := &regressors{byRow: y.rows == 1 && y.cols > 1, rows: y.rows, cols: y.cols}
	if vx.Type == TypeBlank {
		r.m = newMatrix(n, 1)
		for i := range n {
			r.m.a[i] = float64(i + 1)
		}
		return y, r, nil
	}
	x, err := c.matrix(vx)
	if err != nil {
		return nil, nil, err
	}
	r.rows, r.cols = x.rows, x.cols
	if r.byRow {
		x = x.transpose()
	}
	if x.rows != n {
		return nil, nil, &ErrRef
	}
	r.m = x
	return y, r, nil
}

func (m *matrix) transpose() *matrix {
	t := newMatrix(m.cols, m.rows)
	for i := range m.rows {
		for j := range m.cols {
			t.a[j*t.cols+i] = m.at(i, j)
		}
	}
	return t
}

// predict is what the regression gives for the new values of x, or for
// the known ones, in the shape of the values given.
func (c *Context) predict(f *fit, known *regressors, vnew Value, exp bool) Value {
	k := len(f.coef)
	xs, rows, cols := known.m, known.rows, known.cols
	if vnew.Type != TypeBlank {
		m, err := c.matrix(vnew)
		if err != nil {
			return *err
		}
		xs, rows, cols = m, m.rows, m.cols
		if k == 1 {
			xs = &matrix{rows: len(m.a), cols: 1, a: m.a}
		} else if known.byRow {
			xs = m.transpose()
		}
	}
	if xs.cols != k {
		return ErrRef
	}
	if k > 1 {
		rows, cols = xs.rows, 1
		if known.byRow {
			rows, cols = 1, xs.rows
		}
	}
	out := &matrix{rows: rows, cols: cols, a: make([]float64, xs.rows)}
	for i := range xs.rows {
		p := f.intercept
		for j := range k {
			p += f.coef[j] * xs.at(i, j)
		}
		if exp {
			p = math.Exp(p)
		}
		out.a[i] = p
	}
	return out.value()
}

// table is what LINEST gives: the coefficients from the last variable to
// the first and the constant, then the statistics when asked.
func (f *fit) table(stats, exp bool) Value {
	k := len(f.coef)
	rows := 1
	if stats {
		rows = 5
	}
	t := &Table{Rows: rows, Cols: k + 1, Cells: make([]Value, rows*(k+1))}
	for i := range t.Cells {
		t.Cells[i] = ErrNA
	}
	set := func(row, col int, x float64) { t.Cells[row*t.Cols+col] = Num(x) }
	for j := range k {
		coef := f.coef[k-1-j]
		if exp {
			coef = math.Exp(coef)
		}
		set(0, j, coef)
		if stats {
			set(1, j, f.se[k-1-j])
		}
	}
	b := f.intercept
	if exp {
		b = math.Exp(b)
	}
	set(0, k, b)
	if stats {
		if f.constant {
			set(1, k, f.seIntercept)
		}
		set(2, 0, f.r2)
		set(2, 1, f.sey)
		if f.df > 0 {
			set(3, 0, f.f)
		} else {
			t.Cells[3*t.Cols] = ErrNum
		}
		set(3, 1, f.df)
		set(4, 0, f.ssReg)
		set(4, 1, f.ssRes)
	}
	return Value{Type: TypeArray, Arr: t}
}
