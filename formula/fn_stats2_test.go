package formula

import (
	"fmt"
	"maps"
	"math"
	"strings"
	"testing"
	"time"
)

func evalArray(t *testing.T, b Book, f string) Value {
	t.Helper()
	e, err := Parse(f)
	if err != nil {
		t.Fatalf("%s: %v", f, err)
	}
	c := &Context{Book: b, Locale: English, Now: time.Date(2024, 12, 25, 0, 0, 0, 0, time.UTC)}
	v := c.Eval(e)
	if v.Type == TypeRange {
		v = c.deref(v)
	}
	return v
}

func near(got, want, tol float64) bool {
	return math.Abs(got-want) <= tol*math.Max(math.Abs(want), 1e-300)
}

func TestDistributions(t *testing.T) {
	b := book(map[string]any{})
	for _, c := range []struct {
		f    string
		want float64
		tol  float64
	}{
		{`NORM.S.DIST(1.333333,TRUE)`, 0.908788726, 1e-9},
		{`NORM.S.DIST(1.333333,FALSE)`, 0.164010148, 1e-8},
		{`NORMSDIST(1.333333)`, 0.908788726, 1e-9},
		{`NORM.DIST(42,40,1.5,TRUE)`, 0.908788780, 1e-9},
		{`NORM.DIST(42,40,1.5,FALSE)`, 0.10934005, 1e-7},
		{`NORMDIST(42,40,1.5,TRUE)`, 0.908788780, 1e-9},
		{`NORM.INV(0.908789,40,1.5)`, 42.00000201, 1e-9},
		{`NORMINV(0.908789,40,1.5)`, 42.00000201, 1e-9},
		{`NORM.S.INV(0.908789)`, 1.3333347, 1e-7},
		{`NORMSINV(0.975)`, 1.959963984540054, 1e-14},
		{`NORM.S.INV(1E-10)`, -6.361340902404056, 1e-13},
		{`LOGNORM.DIST(4,3.5,1.2,TRUE)`, 0.0390835557, 1e-8},
		{`LOGNORM.DIST(4,3.5,1.2,FALSE)`, 0.0176175, 1e-4},
		{`LOGNORMDIST(4,3.5,1.2)`, 0.0390835557, 1e-8},
		{`T.DIST(60,1,TRUE)`, 0.99469533, 1e-8},
		{`T.DIST(8,3,FALSE)`, 0.00073691, 1e-5},
		{`T.DIST.RT(1,2)`, 0.211324865, 1e-8},
		{`T.DIST.2T(1.959999998,60)`, 0.0546449, 1e-5},
		{`TDIST(1.959999998,60,2)`, 0.054644927, 1e-6},
		{`TDIST(1.959999998,60,1)`, 0.027322464, 1e-6},
		{`T.INV(0.75,2)`, 0.816496581, 1e-9},
		{`T.INV(0.975,10)`, 2.228138851986274, 1e-12},
		{`T.INV(0.025,10)`, -2.228138851986274, 1e-12},
		{`T.INV.2T(0.054645,60)`, 1.96, 1e-6},
		{`TINV(0.05,10)`, 2.228138851986274, 1e-12},
		{`T.TEST({3,4,5,8,9,1,2,4,5},{6,19,3,2,14,4,5,17,1},2,1)`, 0.196016, 1e-5},
		{`TTEST({3,4,5,8,9,1,2,4,5},{6,19,3,2,14,4,5,17,1},2,1)`, 0.196016, 1e-5},
		{`T.TEST({3,4,5,8,9,1,2,4,5},{6,19,3,2,14,4,5,17,1},1,2)`, 0.09599794338, 1e-9},
		{`T.TEST({3,4,5,8,9,1,2,4,5},{6,19,3,2,14,4,5,17,1},2,3)`, 0.2022939234, 1e-9},
		{`CHISQ.DIST(0.5,1,TRUE)`, 0.52049988, 1e-7},
		{`CHISQ.DIST(0.5,1,FALSE)`, 0.439391289, 1e-8},
		{`CHISQ.DIST.RT(0.5,1)`, 0.47950012, 1e-7},
		{`CHIDIST(18.307,10)`, 0.0500006, 1e-5},
		{`CHISQ.TEST({58,35;11,25;10,23},{45.35,47.65;17.56,18.44;16.09,16.91})`, 0.000308192, 1e-6},
		{`CHITEST({58,35;11,25;10,23},{45.35,47.65;17.56,18.44;16.09,16.91})`, 0.000308192, 1e-6},
		{`F.DIST(15.2069,6,4,TRUE)`, 0.99, 1e-4},
		{`F.DIST(15.2069,6,4,FALSE)`, 0.0012238, 1e-4},
		{`F.DIST.RT(15.2069,6,4)`, 0.01, 1e-3},
		{`FDIST(15.2069,6,4)`, 0.01, 1e-3},
		{`BINOM.DIST(6,10,0.5,FALSE)`, 0.205078125, 1e-12},
		{`BINOM.DIST(6,10,0.5,TRUE)`, 0.828125, 1e-12},
		{`BINOMDIST(0,10,0.5,TRUE)`, 0.0009765625, 1e-12},
		{`POISSON.DIST(2,5,FALSE)`, 0.0842243, 1e-6},
		{`POISSON.DIST(2,5,TRUE)`, 0.124652, 1e-6},
		{`POISSON(2,5,TRUE)`, 0.124652, 1e-6},
		{`EXPON.DIST(0.2,10,TRUE)`, 0.864664717, 1e-9},
		{`EXPON.DIST(0.2,10,FALSE)`, 1.353352832, 1e-9},
		{`EXPONDIST(0.2,10,TRUE)`, 0.864664717, 1e-9},
		{`GAMMA.DIST(10.00001131,9,2,FALSE)`, 0.032639, 1e-4},
		{`GAMMA.DIST(10.00001131,9,2,TRUE)`, 0.068094, 1e-4},
		{`GAMMADIST(10.00001131,9,2,TRUE)`, 0.068094, 1e-4},
		{`BETA.DIST(2,8,10,TRUE,1,3)`, 0.6854706, 1e-6},
		{`BETA.DIST(2,8,10,FALSE,1,3)`, 1.4837646, 1e-6},
		{`BETA.DIST(0.5,2,3,TRUE)`, 0.6875, 1e-12},
		{`BETADIST(2,8,10,1,3)`, 0.6854706, 1e-6},
		{`CONFIDENCE.NORM(0.05,2.5,50)`, 0.692951912, 1e-9},
		{`CONFIDENCE(0.05,2.5,50)`, 0.692951912, 1e-9},
		{`CONFIDENCE.T(0.05,1,50)`, 0.284196855, 1e-8},
		{`STANDARDIZE(42,40,1.5)`, 1.33333333333, 1e-9},
		{`NORM.S.DIST(-8,TRUE)`, 6.22096057427178e-16, 1e-9},
		{`CHIDIST(3.841458820694124,1)`, 0.05, 1e-9},
		{`CHISQ.DIST.RT(18.307038053275146,10)`, 0.05, 1e-9},
		{`T.INV.2T(0.05,1)`, 12.706204736174694, 1e-12},
		{`F.DIST.RT(4.964602743730711,1,10)`, 0.05, 1e-9},
	} {
		got := eval(t, b, English, c.f)
		if got.Type != TypeNumber || !near(got.Num, c.want, c.tol) {
			t.Errorf("%s = %s, want %v", c.f, got.show(), c.want)
		}
	}
}

func TestDistributionErrors(t *testing.T) {
	b := book(map[string]any{})
	for f, want := range map[string]string{
		`NORM.DIST(1,0,0,TRUE)`:                         "#NUM!",
		`NORM.INV(0,0,1)`:                               "#NUM!",
		`NORM.INV(0.5,0,-1)`:                            "#NUM!",
		`NORM.S.INV(1)`:                                 "#NUM!",
		`NORM.S.DIST("x",TRUE)`:                         "#VALUE!",
		`LOGNORM.DIST(0,0,1,TRUE)`:                      "#NUM!",
		`T.DIST(1,0,TRUE)`:                              "#NUM!",
		`T.INV(1,5)`:                                    "#NUM!",
		`T.INV(0.5,0)`:                                  "#NUM!",
		`TDIST(1,5,3)`:                                  "#NUM!",
		`TDIST(-1,5,2)`:                                 "#NUM!",
		`T.TEST({1,2,3},{1,2},2,1)`:                     "#N/A",
		`T.TEST({1,2,3},{1,2,4},3,1)`:                   "#NUM!",
		`T.TEST({1,2,3},{1,2,4},2,4)`:                   "#NUM!",
		`T.TEST({1,1,1},{2,2,2},2,2)`:                   "#DIV/0!",
		`CHISQ.DIST(-1,2,TRUE)`:                         "#NUM!",
		`CHISQ.DIST(1,0,TRUE)`:                          "#NUM!",
		`CHISQ.TEST({1,2},{1,2,3})`:                     "#N/A",
		`CHISQ.TEST({1,2},{0,2})`:                       "#DIV/0!",
		`F.DIST(-1,2,2,TRUE)`:                           "#NUM!",
		`F.DIST(1,0,2,TRUE)`:                            "#NUM!",
		`BINOM.DIST(11,10,0.5,TRUE)`:                    "#NUM!",
		`BINOM.DIST(1,10,1.5,TRUE)`:                     "#NUM!",
		`POISSON.DIST(-1,5,TRUE)`:                       "#NUM!",
		`EXPON.DIST(1,0,TRUE)`:                          "#NUM!",
		`GAMMA.DIST(1,0,1,TRUE)`:                        "#NUM!",
		`BETA.DIST(5,2,3,TRUE,1,3)`:                     "#NUM!",
		`BETA.DIST(1,0,3,TRUE)`:                         "#NUM!",
		`CONFIDENCE.NORM(0,2.5,50)`:                     "#NUM!",
		`CONFIDENCE.NORM(0.05,2.5,0)`:                   "#NUM!",
		`CONFIDENCE.T(0.05,1,1)`:                        "#DIV/0!",
		`SKEW(1,2)`:                                     "#DIV/0!",
		`SKEW(2,2,2)`:                                   "#DIV/0!",
		`KURT(1,2,3)`:                                   "#DIV/0!",
		`TRIMMEAN({1,2,3},1)`:                           "#NUM!",
		`PERCENTRANK.INC({1,2,3},0)`:                    "#N/A",
		`PERCENTRANK.INC({1,2,3},2,0)`:                  "#NUM!",
		`NORM.DIST(1,0,1)`:                              "#VALUE!",
		`PERCENTRANK({13,12,11,8,4,3,2,1,1,1},4)`:       "0.555",
		`PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},8,2)`: "0.66",
		`PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},2)`:   "0.333",
		`PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},5)`:   "0.583",
		`PERCENTRANK.INC({13,12,11,8,4,3,2,1,1,1},5,6)`: "0.583333",
		`TRIMMEAN({4,5,6,7,2,3,4,5,1,2,3},0.2)`:         "3.777777778",
		`SKEW(3,4,5,2,3,4,5,6,4,7)`:                     "0.3595430714",
		`KURT(3,4,5,2,3,4,5,6,4,7)`:                     "-0.1517996372",
		`RSQ({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})`:         "0.05795019157",
		`RSQ({2,3,9},{6,5})`:                            "#N/A",
		`RSQ({2},{6})`:                                  "#DIV/0!",
	} {
		if got := eval(t, b, English, f).show(); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func arrayOf(v Value) (rows, cols int, cells []string) {
	if v.Type != TypeArray {
		return 1, 1, []string{v.show()}
	}
	for _, x := range v.Arr.Cells {
		cells = append(cells, x.show())
	}
	return v.Arr.Rows, v.Arr.Cols, cells
}

func TestMatrixFunctions(t *testing.T) {
	b := book(map[string]any{})
	for _, c := range []struct {
		f          string
		rows, cols int
		want       string
	}{
		{`MMULT({1,2;3,4},{5,6;7,8})`, 2, 2, "19 22 43 50"},
		{`MMULT({1,2,3},{1;2;3})`, 1, 1, "14"},
		{`MMULT({1,2},{1,2})`, 1, 1, "#VALUE!"},
		{`MMULT({1,"a"},{1;2})`, 1, 1, "#VALUE!"},
		{`MDETERM({3,6,1;1,1,0;3,10,2})`, 1, 1, "1"},
		{`MDETERM({1,3,8,5;1,3,6,1;1,1,1,0;7,3,10,2})`, 1, 1, "88"},
		{`MDETERM({1,2;2,4})`, 1, 1, "0"},
		{`MDETERM({1,2,3})`, 1, 1, "#VALUE!"},
		{`MINVERSE({4,-1;2,0})`, 2, 2, "0 0.5 -1 2"},
		{`MINVERSE({1,2;2,4})`, 1, 1, "#NUM!"},
		{`MINVERSE({1,2,3;4,5,6;7,8,9})`, 1, 1, "#NUM!"},
		{`MINVERSE({1,2,3})`, 1, 1, "#VALUE!"},
		{`MMULT({4,-1;2,0},MINVERSE({4,-1;2,0}))`, 2, 2, "1 0 0 1"},
		{`MUNIT(3)`, 3, 3, "1 0 0 0 1 0 0 0 1"},
		{`MUNIT(0)`, 1, 1, "#VALUE!"},
		{`FREQUENCY({79,85,78,85,50,81,95,88,97},{70,79,89})`, 4, 1, "1 2 4 2"},
		{`FREQUENCY({1,2,3},{0.5})`, 2, 1, "0 3"},
		{`FREQUENCY({1,"a",2,TRUE},{1,2})`, 3, 1, "1 1 0"},
		{`FREQUENCY({5,1,9},{8,4})`, 3, 1, "1 1 1"},
		{`FREQUENCY({1,2,3},{3,1})`, 3, 1, "2 1 0"},
		{`FREQUENCY({1,2,3},"x")`, 1, 1, "3"},
		{`SUM(FREQUENCY({1,2,3},{1,2}))`, 1, 1, "3"},
	} {
		rows, cols, cells := arrayOf(evalArray(t, b, c.f))
		got := strings.Join(cells, " ")
		if rows != c.rows || cols != c.cols || !closeCells(got, c.want) {
			t.Errorf("%s = %dx%d %s, want %dx%d %s", c.f, rows, cols, got, c.rows, c.cols, c.want)
		}
	}
}

func closeCells(got, want string) bool {
	return closeCellsTol(got, want, 1e-9)
}

func closeCellsTol(got, want string, tol float64) bool {
	g, w := strings.Fields(got), strings.Fields(want)
	if len(g) != len(w) {
		return false
	}
	for i := range g {
		var x, y float64
		if _, err := fmt.Sscan(g[i], &x); err != nil {
			if g[i] != w[i] {
				return false
			}
			continue
		}
		if _, err := fmt.Sscan(w[i], &y); err != nil || math.Abs(x-y) > tol*math.Max(1, math.Abs(y)) {
			return false
		}
	}
	return true
}

func TestRegression(t *testing.T) {
	b := book(map[string]any{})
	const y = `{3;7;8;12;13;19;21;24}`
	const x = `{1,2;2,1;3,5;4,3;5,8;6,4;7,9;8,6}`
	const months = `{133890;135000;135790;137300;138130;139100;139900;141120;141890;143230;144000;145290}`
	const units = `{33100;47300;69000;102000;150000;220000}`
	for _, c := range []struct {
		f          string
		rows, cols int
		want       string
	}{
		{`LINEST({1,9,5,7},{0,4,2,3})`, 1, 2, "2 1"},
		{`LINEST({1;9;5;7})`, 1, 2, "1.4 2"},
		{`LINEST({1;9;5;7},{0;4;2;3},FALSE)`, 1, 2, "2.3103448275862069 0"},
		{`LINEST(` + y + `,` + x + `)`, 1, 3, "-0.41139240506329117 3.330922242314647 0.3399638336347197"},
		{`LINEST(` + y + `,` + x + `,TRUE,TRUE)`, 5, 3, "-0.41139240506329117 3.330922242314647 0.3399638336347197 " +
			"0.1353559741129499 0.15559630342864209 0.5578915354980319 " +
			"0.9936830204538257 0.6945929115957488 #N/A " +
			"393.2587612443778 5 #N/A " +
			"379.4627034358047 2.4122965641952985 #N/A"},
		{`LINEST(` + y + `,` + x + `,FALSE,TRUE)`, 5, 3, "-0.3913805185704275 3.370707778556412 0 " +
			"0.12424214569135207 0.13363181310744057 #N/A " +
			"0.9985706284635106 0.6571974076403554 #N/A " +
			"2095.8244997295837 6 #N/A " +
			"1810.4085494043447 2.5914505956552207 #N/A"},
		{`LINEST({3,7,8,12},{1,2,3,4;2,4,6,8})`, 1, 3, "0 2.8 0.5"},
		{`LINEST({1;2;3},{1;2})`, 1, 1, "#REF!"},
		{`LINEST({1,2;3,4})`, 1, 1, "#REF!"},
		{`LINEST({1;2;"a"})`, 1, 1, "#VALUE!"},
		{`TREND(` + months + `,{1;2;3;4;5;6;7;8;9;10;11;12},{13;14;15;16;17})`, 5, 1,
			"146171.51515151517 147189.69696969696 148207.87878787878 149226.0606060606 150244.24242424243"},
		{`TREND(` + months + `,,{13,14})`, 1, 2, "146171.51515151517 147189.69696969696"},
		{`TREND({1,9,5,7},{0,4,2,3})`, 1, 4, "1 9 5 7"},
		{`TREND({1,9,5,7},{0,4,2,3},{5;6})`, 2, 1, "11 13"},
		{`TREND(` + y + `,` + x + `,{9,7})`, 1, 1, "27.43851718"},
		{`TREND({1;9;5;7},{0;4;2;3},5,FALSE)`, 1, 1, "11.55172413793"},
		{`TREND({1;2;3},{1;2})`, 1, 1, "#REF!"},
		{`GROWTH(` + units + `,{11;12;13;14;15;16},{17;18})`, 2, 1, "320196.7183634738 468536.05418404966"},
		{`GROWTH(` + units + `,{11;12;13;14;15;16})`, 6, 1, "32618.20377 47729.42261 69841.30086 102197.0734 149542.4867 218821.8762"},
		{`GROWTH({2,4,8},{1,2,3},{4})`, 1, 1, "16"},
		{`GROWTH({2,0,8},{1,2,3},{4})`, 1, 1, "#NUM!"},
		{`LOGEST(` + units + `,{11;12;13;14;15;16})`, 1, 2, "1.4632756281161756 495.3047701587292"},
		{`LOGEST({2;4;8;16},{1;2;3;4},FALSE)`, 1, 2, "2 1"},
		{`LOGEST({2;5;7;19;40},{1;2;3;4;5},TRUE,TRUE)`, 5, 2, "2.0805797095246756 0.978654168027802 0.05303415035504189 0.17589437780296757 0.9845236126400446 0.16770870889375988 190.84368782099583 3 5.367709839078464 0.08437863311643569"},
	} {
		rows, cols, cells := arrayOf(evalArray(t, b, c.f))
		got := strings.Join(cells, " ")
		if rows != c.rows || cols != c.cols || !closeCellsTol(got, c.want, 1e-9) {
			t.Errorf("%s = %dx%d %s, want %dx%d %s", c.f, rows, cols, got, c.rows, c.cols, c.want)
		}
	}
}

func TestEngineMatrixFormulas(t *testing.T) {
	s, e := newSource()
	for cell, v := range map[string]int{"A1": 1, "B1": 2, "A2": 3, "B2": 4, "D1": 5, "E1": 6, "D2": 7, "E2": 8} {
		s.set(e, cell, v)
	}
	e.Set(Pos{0, 4, 1}, "MMULT(A1:B2,D1:E2)", &Area{R1: 4, C1: 1, R2: 5, C2: 2})
	e.Set(Pos{0, 7, 1}, "FREQUENCY(A1:B2,{1;3})", &Area{R1: 7, C1: 1, R2: 9, C2: 1})
	e.Set(Pos{0, 4, 4}, "MINVERSE(A1:B2)", &Area{R1: 4, C1: 4, R2: 5, C2: 5})
	e.Set(Pos{0, 7, 4}, "LINEST(A1:A2,D1:D2)", &Area{R1: 7, C1: 4, R2: 7, C2: 5})
	got := map[string]string{}
	for _, r := range e.Recalc([]Pos{{0, 4, 1}, {0, 7, 1}, {0, 4, 4}, {0, 7, 4}}) {
		got[fmt.Sprintf("%d,%d", r.Row, r.Col)] = r.Value.show()
	}
	want := map[string]string{"4,1": "19", "4,2": "22", "5,1": "43", "5,2": "50", "7,1": "1", "8,1": "2", "9,1": "1",
		"4,4": "-2", "4,5": "1", "5,4": "1.5", "5,5": "-0.5", "7,4": "1", "7,5": "-4"}
	if !maps.Equal(got, want) {
		t.Errorf("%v, want %v", got, want)
	}
}
