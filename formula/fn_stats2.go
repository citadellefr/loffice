package formula

import (
	"math"
	"sort"
)

func init() {
	add(map[string]*function{
		"NORM.DIST": dist(4, 4, normal),
		"NORMDIST":  dist(4, 4, normal),
		"NORM.S.DIST": dist(2, 2, func(a []float64, cum bool) Value {
			if cum {
				return Num(normCDF(a[0]))
			}
			return Num(normPDF(a[0]))
		}),
		"NORMSDIST":    numbersFn(1, 1, func(a []float64) Value { return Num(normCDF(a[0])) }),
		"NORM.INV":     numbersFn(3, 3, normalInverse),
		"NORMINV":      numbersFn(3, 3, normalInverse),
		"NORM.S.INV":   numbersFn(1, 1, standardInverse),
		"NORMSINV":     numbersFn(1, 1, standardInverse),
		"LOGNORM.DIST": dist(4, 4, lognormal),
		"LOGNORMDIST":  numbersFn(3, 3, func(a []float64) Value { return lognormal(a, true) }),
		"T.DIST": dist(3, 3, func(a []float64, cum bool) Value {
			x, df := a[0], math.Trunc(a[1])
			if df < 1 {
				return ErrNum
			}
			if cum {
				return Num(tCDF(x, df))
			}
			return Num(tPDF(x, df))
		}),
		"T.DIST.RT": numbersFn(2, 2, func(a []float64) Value {
			df := math.Trunc(a[1])
			if df < 1 {
				return ErrNum
			}
			return Num(1 - tCDF(a[0], df))
		}),
		"T.DIST.2T": numbersFn(2, 2, func(a []float64) Value {
			return studentTails(a[0], a[1], 2)
		}),
		"TDIST": numbersFn(3, 3, func(a []float64) Value {
			switch math.Trunc(a[2]) {
			case 1:
				return studentTails(a[0], a[1], 1)
			case 2:
				return studentTails(a[0], a[1], 2)
			}
			return ErrNum
		}),
		"T.INV": numbersFn(2, 2, func(a []float64) Value {
			df := math.Trunc(a[1])
			if a[0] <= 0 || a[0] >= 1 || df < 1 {
				return ErrNum
			}
			return Num(tInv(a[0], df))
		}),
		"T.INV.2T": numbersFn(2, 2, studentInverse2T),
		"TINV":     numbersFn(2, 2, studentInverse2T),
		"T.TEST":   studentTest(),
		"TTEST":    studentTest(),
		"CHISQ.DIST": dist(3, 3, func(a []float64, cum bool) Value {
			x, df := a[0], math.Trunc(a[1])
			if x < 0 || df < 1 || df > 1e10 {
				return ErrNum
			}
			if cum {
				return Num(gammaP(df/2, x/2))
			}
			return Num(chiPDF(x, df))
		}),
		"CHISQ.DIST.RT": numbersFn(2, 2, chiRightTail),
		"CHIDIST":       numbersFn(2, 2, chiRightTail),
		"CHISQ.TEST":    chiTest(),
		"CHITEST":       chiTest(),
		"F.DIST": dist(4, 4, func(a []float64, cum bool) Value {
			x, d1, d2 := a[0], math.Trunc(a[1]), math.Trunc(a[2])
			if x < 0 || d1 < 1 || d2 < 1 || d1 >= 1e10 || d2 >= 1e10 {
				return ErrNum
			}
			if cum {
				return Num(fCDF(x, d1, d2))
			}
			return Num(fPDF(x, d1, d2))
		}),
		"F.DIST.RT":       numbersFn(3, 3, fRightTail),
		"FDIST":           numbersFn(3, 3, fRightTail),
		"BINOM.DIST":      dist(4, 4, binomial),
		"BINOMDIST":       dist(4, 4, binomial),
		"POISSON.DIST":    dist(3, 3, poisson),
		"POISSON":         dist(3, 3, poisson),
		"EXPON.DIST":      dist(3, 3, exponential),
		"EXPONDIST":       dist(3, 3, exponential),
		"GAMMA.DIST":      dist(4, 4, gamma),
		"GAMMADIST":       dist(4, 4, gamma),
		"BETA.DIST":       betaDist(true),
		"BETADIST":        betaDist(false),
		"CONFIDENCE.NORM": numbersFn(3, 3, confidenceNorm),
		"CONFIDENCE":      numbersFn(3, 3, confidenceNorm),
		"CONFIDENCE.T": numbersFn(3, 3, func(a []float64) Value {
			alpha, sd, size := a[0], a[1], math.Trunc(a[2])
			switch {
			case alpha <= 0 || alpha >= 1 || sd <= 0 || size < 1:
				return ErrNum
			case size == 1:
				return ErrDiv0
			}
			return Num(tInvTail(alpha/2, size-1) * sd / math.Sqrt(size))
		}),
		"SKEW": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			n := float64(len(xs))
			m, s := meanDeviation(xs)
			if len(xs) < 3 || s == 0 {
				return ErrDiv0
			}
			sum := 0.0
			for _, x := range xs {
				sum += math.Pow((x-m)/s, 3)
			}
			return Num(n / ((n - 1) * (n - 2)) * sum)
		}},
		"KURT": {min: 1, max: 255, call: func(c *Context, args []Expr) Value {
			xs, err := c.numberList(args)
			if err != nil {
				return *err
			}
			n := float64(len(xs))
			m, s := meanDeviation(xs)
			if len(xs) < 4 || s == 0 {
				return ErrDiv0
			}
			sum := 0.0
			for _, x := range xs {
				sum += math.Pow((x-m)/s, 4)
			}
			return Num(n*(n+1)/((n-1)*(n-2)*(n-3))*sum - 3*(n-1)*(n-1)/((n-2)*(n-3)))
		}},
		"TRIMMEAN": fn(2, 2, func(c *Context, v []Value) Value {
			xs, err := c.numbersIn(v[0])
			if err != nil {
				return *err
			}
			p, err := c.number(v[1])
			if err != nil {
				return *err
			}
			if p < 0 || p >= 1 || len(xs) == 0 {
				return ErrNum
			}
			sort.Float64s(xs)
			k := int(math.Floor(float64(len(xs)) * p / 2))
			kept := xs[k : len(xs)-k]
			return Num(sumOf(kept) / float64(len(kept)))
		}),
		"PERCENTRANK.INC": percentRank(),
		"PERCENTRANK":     percentRank(),
	})
}

// numbersFn is a function of numbers only, each an error when it is not
// one.
func numbersFn(min, max int, f func(a []float64) Value) *function {
	return fn(min, max, func(c *Context, v []Value) Value {
		a := make([]float64, len(v))
		for i, x := range v {
			n, err := c.number(x)
			if err != nil {
				return *err
			}
			a[i] = n
		}
		return f(a)
	})
}

// dist is a distribution function whose last argument says whether it
// gives the cumulative distribution or the density.
func dist(min, max int, f func(a []float64, cumulative bool) Value) *function {
	return fn(min, max, func(c *Context, v []Value) Value {
		a := make([]float64, len(v)-1)
		for i := range a {
			n, err := c.number(v[i])
			if err != nil {
				return *err
			}
			a[i] = n
		}
		cum, err := c.boolean(v[len(v)-1])
		if err != nil {
			return *err
		}
		return f(a, cum)
	})
}

func normal(a []float64, cum bool) Value {
	x, mean, sd := a[0], a[1], a[2]
	if sd <= 0 {
		return ErrNum
	}
	z := (x - mean) / sd
	if cum {
		return Num(normCDF(z))
	}
	return Num(normPDF(z) / sd)
}

func standardInverse(a []float64) Value {
	if a[0] <= 0 || a[0] >= 1 {
		return ErrNum
	}
	return Num(normInv(a[0]))
}

func normalInverse(a []float64) Value {
	p, mean, sd := a[0], a[1], a[2]
	if p <= 0 || p >= 1 || sd <= 0 {
		return ErrNum
	}
	return Num(mean + sd*normInv(p))
}

func lognormal(a []float64, cum bool) Value {
	x, mean, sd := a[0], a[1], a[2]
	if x <= 0 || sd <= 0 {
		return ErrNum
	}
	z := (math.Log(x) - mean) / sd
	if cum {
		return Num(normCDF(z))
	}
	return Num(normPDF(z) / (x * sd))
}

func studentTails(x, df float64, tails int) Value {
	df = math.Trunc(df)
	if df < 1 || x < 0 {
		return ErrNum
	}
	return Num(float64(tails) * tTail(x, df))
}

func studentInverse2T(a []float64) Value {
	p, df := a[0], math.Trunc(a[1])
	if p <= 0 || p > 1 || df < 1 {
		return ErrNum
	}
	if p == 1 {
		return Num(0)
	}
	return Num(tInvTail(p/2, df))
}

func chiRightTail(a []float64) Value {
	x, df := a[0], math.Trunc(a[1])
	if x < 0 || df < 1 || df > 1e10 {
		return ErrNum
	}
	return Num(gammaQ(df/2, x/2))
}

func fRightTail(a []float64) Value {
	x, d1, d2 := a[0], math.Trunc(a[1]), math.Trunc(a[2])
	if x < 0 || d1 < 1 || d2 < 1 || d1 >= 1e10 || d2 >= 1e10 {
		return ErrNum
	}
	return Num(fTail(x, d1, d2))
}

func binomial(a []float64, cum bool) Value {
	k, n, p := math.Trunc(a[0]), math.Trunc(a[1]), a[2]
	if k < 0 || k > n || p < 0 || p > 1 {
		return ErrNum
	}
	if cum {
		if k == n {
			return Num(1)
		}
		return Num(betaI(1-p, n-k, k+1))
	}
	switch {
	case p == 0:
		return indicator(k == 0)
	case p == 1:
		return indicator(k == n)
	}
	return Num(math.Exp(logGamma(n+1) - logGamma(k+1) - logGamma(n-k+1) + k*math.Log(p) + (n-k)*math.Log1p(-p)))
}

func poisson(a []float64, cum bool) Value {
	x, mean := math.Trunc(a[0]), a[1]
	if x < 0 || mean < 0 {
		return ErrNum
	}
	if mean == 0 {
		return indicator(x == 0 || cum)
	}
	if cum {
		return Num(gammaQ(x+1, mean))
	}
	return Num(math.Exp(-mean + x*math.Log(mean) - logGamma(x+1)))
}

func exponential(a []float64, cum bool) Value {
	x, lambda := a[0], a[1]
	if x < 0 || lambda <= 0 {
		return ErrNum
	}
	if cum {
		return Num(-math.Expm1(-lambda * x))
	}
	return Num(lambda * math.Exp(-lambda*x))
}

func gamma(a []float64, cum bool) Value {
	x, alpha, beta := a[0], a[1], a[2]
	if x < 0 || alpha <= 0 || beta <= 0 {
		return ErrNum
	}
	if cum {
		return Num(gammaP(alpha, x/beta))
	}
	return Num(gammaPDF(x, alpha, beta))
}

// indicator is 1 when ok, 0 otherwise.
func indicator(ok bool) Value {
	if ok {
		return Num(1)
	}
	return Num(0)
}

// betaDist is BETA.DIST, with a cumulative argument, or BETADIST, which
// has none and gives the cumulative distribution.
func betaDist(modern bool) *function {
	min, max := 3, 5
	if modern {
		min, max = 4, 6
	}
	return fn(min, max, func(c *Context, v []Value) Value {
		cum := true
		if modern {
			var err *Value
			if cum, err = c.boolean(v[3]); err != nil {
				return *err
			}
			v = append(v[:3:3], v[4:]...)
		}
		n := []float64{0, 0, 0, 0, 1}
		for i, x := range v {
			if i >= 3 && x.Type == TypeBlank {
				continue
			}
			f, err := c.number(x)
			if err != nil {
				return *err
			}
			n[i] = f
		}
		x, alpha, beta, lo, hi := n[0], n[1], n[2], n[3], n[4]
		if alpha <= 0 || beta <= 0 || x < lo || x > hi || lo == hi {
			return ErrNum
		}
		z := (x - lo) / (hi - lo)
		if cum {
			return Num(betaI(z, alpha, beta))
		}
		return Num(betaPDF(z, alpha, beta) / (hi - lo))
	})
}

func confidenceNorm(a []float64) Value {
	alpha, sd, size := a[0], a[1], math.Trunc(a[2])
	if alpha <= 0 || alpha >= 1 || sd <= 0 || size < 1 {
		return ErrNum
	}
	return Num(-normInv(alpha/2) * sd / math.Sqrt(size))
}

// meanDeviation is the mean and the standard deviation of a sample.
func meanDeviation(xs []float64) (float64, float64) {
	n := float64(len(xs))
	m := sumOf(xs) / n
	d := 0.0
	for _, x := range xs {
		d += (x - m) * (x - m)
	}
	return m, math.Sqrt(d / (n - 1))
}

// numbersIn is the numbers an array or a range holds.
func (c *Context) numbersIn(v Value) ([]float64, *Value) {
	var out []float64
	var err *Value
	c.eachOf(v, func(x Value, _ bool) bool {
		switch x.Type {
		case TypeError:
			found := x
			err = &found
			return false
		case TypeNumber:
			out = append(out, x.Num)
		}
		return true
	})
	return out, err
}

func percentRank() *function {
	return fn(2, 3, func(c *Context, v []Value) Value {
		xs, err := c.numbersIn(v[0])
		if err != nil {
			return *err
		}
		x, err := c.number(v[1])
		if err != nil {
			return *err
		}
		digits := 3.0
		if len(v) > 2 && v[2].Type != TypeBlank {
			if digits, err = c.number(v[2]); err != nil {
				return *err
			}
			digits = math.Trunc(digits)
		}
		if len(xs) == 0 || digits < 1 {
			return ErrNum
		}
		sort.Float64s(xs)
		n := len(xs)
		if x < xs[0] || x > xs[n-1] {
			return ErrNA
		}
		if n == 1 {
			return Num(1)
		}
		i := sort.SearchFloat64s(xs, x)
		rank := float64(i)
		if xs[i] != x {
			rank += (x-xs[i-1])/(xs[i]-xs[i-1]) - 1
		}
		scale := math.Pow(10, digits)
		return Num(math.Floor(rank/float64(n-1)*scale+1e-9) / scale)
	})
}

// studentTest is T.TEST: paired, equal variances or unequal variances.
func studentTest() *function {
	return fn(4, 4, func(c *Context, v []Value) Value {
		var n [2]float64
		for i := range n {
			f, err := c.number(v[2+i])
			if err != nil {
				return *err
			}
			n[i] = math.Trunc(f)
		}
		tails, kind := n[0], n[1]
		a, b := c.array(v[0]), c.array(v[1])
		if kind == 1 && len(a.Cells) != len(b.Cells) {
			return ErrNA
		}
		if tails != 1 && tails != 2 || kind < 1 || kind > 3 {
			return ErrNum
		}
		var xs, ys []float64
		for i, x := range a.Cells {
			if x.Type == TypeError {
				return x
			}
			if kind == 1 {
				if y := b.Cells[i]; y.Type == TypeError {
					return y
				} else if x.Type == TypeNumber && y.Type == TypeNumber {
					xs, ys = append(xs, x.Num), append(ys, y.Num)
				}
			} else if x.Type == TypeNumber {
				xs = append(xs, x.Num)
			}
		}
		if kind != 1 {
			for _, y := range b.Cells {
				if y.Type == TypeError {
					return y
				}
				if y.Type == TypeNumber {
					ys = append(ys, y.Num)
				}
			}
		}
		t, df, ok := studentStatistic(xs, ys, int(kind))
		if !ok {
			return ErrDiv0
		}
		return Num(tails * tTail(math.Abs(t), df))
	})
}

// studentStatistic is the t of a test of two samples and its degrees of
// freedom, false when it has none.
func studentStatistic(xs, ys []float64, kind int) (t, df float64, ok bool) {
	n1, n2 := float64(len(xs)), float64(len(ys))
	if len(xs) < 2 || len(ys) < 2 {
		return 0, 0, false
	}
	if kind == 1 {
		d := make([]float64, len(xs))
		for i := range d {
			d[i] = xs[i] - ys[i]
		}
		m, s := meanDeviation(d)
		if s == 0 {
			return 0, 0, false
		}
		return m / (s / math.Sqrt(n1)), n1 - 1, true
	}
	m1, s1 := meanDeviation(xs)
	m2, s2 := meanDeviation(ys)
	v1, v2 := s1*s1, s2*s2
	if kind == 2 {
		df = n1 + n2 - 2
		pooled := ((n1-1)*v1 + (n2-1)*v2) / df
		if pooled == 0 {
			return 0, 0, false
		}
		return (m1 - m2) / math.Sqrt(pooled*(1/n1+1/n2)), df, true
	}
	e1, e2 := v1/n1, v2/n2
	if e1+e2 == 0 {
		return 0, 0, false
	}
	df = (e1 + e2) * (e1 + e2) / (e1*e1/(n1-1) + e2*e2/(n2-1))
	return (m1 - m2) / math.Sqrt(e1+e2), df, true
}

// chiTest is CHISQ.TEST: the probability of a chi-squared as large as that
// of the observed counts against the expected ones.
func chiTest() *function {
	return fn(2, 2, func(c *Context, v []Value) Value {
		a, b := c.array(v[0]), c.array(v[1])
		if a.Rows != b.Rows || a.Cols != b.Cols {
			return ErrNA
		}
		sum := 0.0
		for i, x := range a.Cells {
			e := b.Cells[i]
			if x.Type == TypeError {
				return x
			}
			if e.Type == TypeError {
				return e
			}
			if x.Type != TypeNumber || e.Type != TypeNumber {
				return ErrValue
			}
			if e.Num == 0 {
				return ErrDiv0
			}
			sum += (x.Num - e.Num) * (x.Num - e.Num) / e.Num
		}
		df := (a.Rows - 1) * (a.Cols - 1)
		if a.Rows == 1 || a.Cols == 1 {
			df = a.Rows*a.Cols - 1
		}
		if df < 1 {
			return ErrDiv0
		}
		return Num(gammaQ(float64(df)/2, sum/2))
	})
}
