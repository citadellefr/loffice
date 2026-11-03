package formula

import "math"

const (
	sqrt2Pi = 2.5066282746310002
	epsilon = 1e-16
)

func normCDF(z float64) float64 {
	return 0.5 * math.Erfc(-z/math.Sqrt2)
}

func normPDF(z float64) float64 {
	return math.Exp(-z*z/2) / sqrt2Pi
}

// normInv is the z whose normal distribution is p, 0 < p < 1: the
// inverse error function, refined where it loses its digits, in the tails.
func normInv(p float64) float64 {
	if p > 0.5 {
		return -normInv(1 - p)
	}
	z := -math.Sqrt2 * math.Erfcinv(2*p)
	for range 3 {
		u := (normCDF(z) - p) / normPDF(z)
		z -= u / (1 + z*u/2)
	}
	return z
}

func logGamma(x float64) float64 {
	l, _ := math.Lgamma(x)
	return l
}

func logBeta(a, b float64) float64 {
	return logGamma(a) + logGamma(b) - logGamma(a+b)
}

// gammaP is the regularized lower incomplete gamma function P(a, x), and
// gammaQ its complement.
func gammaP(a, x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x < a+1 {
		return gammaSeries(a, x)
	}
	return 1 - gammaFraction(a, x)
}

func gammaQ(a, x float64) float64 {
	if x <= 0 {
		return 1
	}
	if x < a+1 {
		return 1 - gammaSeries(a, x)
	}
	return gammaFraction(a, x)
}

func gammaSeries(a, x float64) float64 {
	sum := 1 / a
	term := sum
	for n := 1; n < 100000; n++ {
		term *= x / (a + float64(n))
		sum += term
		if math.Abs(term) < math.Abs(sum)*epsilon {
			break
		}
	}
	return sum * math.Exp(-x+a*math.Log(x)-logGamma(a))
}

// gammaFraction is Q(a, x) by Lentz's continued fraction.
func gammaFraction(a, x float64) float64 {
	const tiny = 1e-300
	b := x + 1 - a
	c := 1 / tiny
	d := 1 / b
	h := d
	for i := 1; i < 100000; i++ {
		an := -float64(i) * (float64(i) - a)
		b += 2
		d = an*d + b
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = b + an/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < epsilon {
			break
		}
	}
	return h * math.Exp(-x+a*math.Log(x)-logGamma(a))
}

// betaI is the regularized incomplete beta function I_x(a, b).
func betaI(x, a, b float64) float64 {
	switch {
	case x <= 0:
		return 0
	case x >= 1:
		return 1
	}
	front := math.Exp(a*math.Log(x) + b*math.Log1p(-x) - logBeta(a, b))
	if x < (a+1)/(a+b+2) {
		return front * betaFraction(x, a, b) / a
	}
	return 1 - front*betaFraction(1-x, b, a)/b
}

func betaFraction(x, a, b float64) float64 {
	const tiny = 1e-300
	qab, qap, qam := a+b, a+1, a-1
	c := 1.0
	d := 1 - qab*x/qap
	if math.Abs(d) < tiny {
		d = tiny
	}
	d = 1 / d
	h := d
	for m := 1; m < 200000; m++ {
		fm := float64(m)
		m2 := 2 * fm
		aa := fm * (b - fm) * x / ((qam + m2) * (a + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		h *= d * c
		aa = -(a + fm) * (qab + fm) * x / ((a + m2) * (qap + m2))
		d = 1 + aa*d
		if math.Abs(d) < tiny {
			d = tiny
		}
		c = 1 + aa/c
		if math.Abs(c) < tiny {
			c = tiny
		}
		d = 1 / d
		delta := d * c
		h *= delta
		if math.Abs(delta-1) < epsilon {
			break
		}
	}
	return h
}

// tTail is the probability that a Student variable of df degrees of
// freedom exceeds t >= 0.
func tTail(t, df float64) float64 {
	return 0.5 * betaI(df/(df+t*t), df/2, 0.5)
}

func tCDF(t, df float64) float64 {
	if t < 0 {
		return tTail(-t, df)
	}
	return 1 - tTail(t, df)
}

func tPDF(t, df float64) float64 {
	return math.Exp(logGamma((df+1)/2)-logGamma(df/2)-(df+1)/2*math.Log1p(t*t/df)) / math.Sqrt(df*math.Pi)
}

// tInvTail is the t whose right tail is p, 0 < p < 0.5.
func tInvTail(p, df float64) float64 {
	hi := 1.0
	for tTail(hi, df) > p {
		hi *= 2
	}
	lo := 0.0
	for range 200 {
		mid := (lo + hi) / 2
		if mid == lo || mid == hi {
			break
		}
		if tTail(mid, df) > p {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2
}

// tInv is the t whose distribution is p, 0 < p < 1.
func tInv(p, df float64) float64 {
	switch {
	case p == 0.5:
		return 0
	case p < 0.5:
		return -tInvTail(p, df)
	}
	return tInvTail(1-p, df)
}

// fTail is the probability that an F variable exceeds x >= 0.
func fTail(x, d1, d2 float64) float64 {
	return betaI(d2/(d2+d1*x), d2/2, d1/2)
}

func fCDF(x, d1, d2 float64) float64 {
	return betaI(d1*x/(d1*x+d2), d1/2, d2/2)
}

func fPDF(x, d1, d2 float64) float64 {
	if x == 0 {
		switch {
		case d1 < 2:
			return math.Inf(1)
		case d1 == 2:
			return 1
		}
		return 0
	}
	return math.Exp(d1/2*math.Log(d1*x) + d2/2*math.Log(d2) - (d1+d2)/2*math.Log(d1*x+d2) - math.Log(x) - logBeta(d1/2, d2/2))
}

// chiPDF is the density of the chi-squared distribution, as the gamma
// density of shape df/2 and scale 2.
func chiPDF(x, df float64) float64 {
	return gammaPDF(x, df/2, 2)
}

func gammaPDF(x, alpha, beta float64) float64 {
	if x == 0 {
		switch {
		case alpha < 1:
			return math.Inf(1)
		case alpha == 1:
			return 1 / beta
		}
		return 0
	}
	return math.Exp((alpha-1)*math.Log(x) - x/beta - alpha*math.Log(beta) - logGamma(alpha))
}

func betaPDF(x, alpha, beta float64) float64 {
	switch {
	case x == 0 && alpha < 1, x == 1 && beta < 1:
		return math.Inf(1)
	case x == 0 && alpha == 1:
		return beta
	case x == 1 && beta == 1:
		return alpha
	case x == 0, x == 1:
		return 0
	}
	return math.Exp((alpha-1)*math.Log(x) + (beta-1)*math.Log1p(-x) - logBeta(alpha, beta))
}
