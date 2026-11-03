package formula

import (
	"strings"
	"testing"
	"time"
)

// evalArray calculates a formula and writes its result as a table: cells
// separated by "," and rows by ";", a scalar shown alone.
func evalArray(t *testing.T, b Book, f string, rand func() float64) string {
	t.Helper()
	e, err := Parse(f)
	if err != nil {
		t.Fatalf("%s: %v", f, err)
	}
	c := &Context{Book: b, Row: 20, Col: 10, Locale: English, Rand: rand,
		Now: time.Date(2024, 12, 25, 18, 30, 0, 0, time.UTC)}
	v := c.Eval(e)
	if v.Type != TypeArray {
		return c.scalar(v).show()
	}
	rows := make([]string, v.Arr.Rows)
	for i := range rows {
		cells := make([]string, v.Arr.Cols)
		for j := range cells {
			cells[j] = v.Arr.At(i, j).show()
		}
		rows[i] = strings.Join(cells, ",")
	}
	return strings.Join(rows, ";")
}

var arraySheet = map[string]any{
	"A1": "b", "B1": 2, "C1": true,
	"A2": "A", "B2": 1, "C2": false,
	"A3": "b", "B3": 2, "C3": true,
	"A4": 10, "B4": "1", "C4": "#N/A",
	"A5": "a", "B5": 1,
	"D1": 1, "D2": 0, "D3": 1, "D4": 0, "D5": 1,
	"F1": 3, "G1": 1, "H1": 2,
	"F2": "x", "G2": "y", "H2": "z",
	"J1": "b", "J2": "A", "J3": "c", "J4": "a",
	"L1": 1, "L2": "a", "L3": true, "L4": "#N/A", "L5": nil, "L6": 2,
}

func TestArrays(t *testing.T) {
	b := book(arraySheet)
	for f, want := range map[string]string{
		`SEQUENCE(3)`:                            "1;2;3",
		`SEQUENCE(2,3)`:                          "1,2,3;4,5,6",
		`SEQUENCE(2,2,10,5)`:                     "10,15;20,25",
		`SEQUENCE(3,,5)`:                         "5;6;7",
		`SEQUENCE(1,3,,2)`:                       "1,3,5",
		`SEQUENCE(2.9,1.5)`:                      "1;2",
		`SEQUENCE(1,3,1,-0.5)`:                   "1,0.5,0",
		`SEQUENCE(1)`:                            "1",
		`_xlfn.SEQUENCE(2)`:                      "1;2",
		`SEQUENCE(0)`:                            "#CALC!",
		`SEQUENCE(2,0)`:                          "#CALC!",
		`SEQUENCE(0.5)`:                          "#CALC!",
		`SEQUENCE(-1)`:                           "#VALUE!",
		`SEQUENCE(2,-1)`:                         "#VALUE!",
		`SEQUENCE("x")`:                          "#VALUE!",
		`SEQUENCE(20000,20000)`:                  "#NUM!",
		`SEQUENCE(1E+300,1E+300)`:                "#NUM!",
		`SEQUENCE(#N/A)`:                         "#N/A",
		`SEQUENCE()`:                             "#VALUE!",
		`SEQUENCE(1,1,1,1,1)`:                    "#VALUE!",
		`RANDARRAY()`:                            "0.5",
		`RANDARRAY(2,2)`:                         "0.5,0.5;0.5,0.5",
		`RANDARRAY(1,2,10,20)`:                   "15,15",
		`RANDARRAY(,2,-1,1)`:                     "0,0",
		`RANDARRAY(2,1,1,6,TRUE)`:                "4;4",
		`RANDARRAY(1,1,5,5,TRUE)`:                "5",
		`RANDARRAY(1,1,1.5,3,FALSE)`:             "2.25",
		`RANDARRAY(1,1,1.5,3,TRUE)`:              "#VALUE!",
		`RANDARRAY(1,1,1,3.5,TRUE)`:              "#VALUE!",
		`RANDARRAY(1,1,2,1)`:                     "#VALUE!",
		`RANDARRAY(0)`:                           "#CALC!",
		`RANDARRAY(-1)`:                          "#VALUE!",
		`_xlfn.RANDARRAY(1,2)`:                   "0.5,0.5",
		`UNIQUE(J1:J4)`:                          `"b";"A";"c"`,
		`UNIQUE({1;2;1;"1";"a";"A"})`:            `1;2;"1";"a"`,
		`UNIQUE(A1:B5)`:                          `"b",2;"A",1;10,"1"`,
		`UNIQUE(A1:C3)`:                          `"b",2,TRUE;"A",1,FALSE`,
		`UNIQUE(A1:B5,,TRUE)`:                    `10,"1"`,
		`UNIQUE({1,2,1;3,4,3},TRUE)`:             "1,2;3,4",
		`UNIQUE({1,2,1;3,4,3},TRUE,TRUE)`:        "2;4",
		`UNIQUE({1,1;1,1},FALSE,TRUE)`:           "#CALC!",
		`UNIQUE({1;1},,TRUE)`:                    "#CALC!",
		`UNIQUE({1;2;2;3;3},FALSE,TRUE)`:         "1",
		`UNIQUE(Z1:Z3)`:                          "blank",
		`UNIQUE({"a";#N/A;#N/A})`:                `"a";#N/A`,
		`UNIQUE(#REF!)`:                          "#REF!",
		`_xlfn.UNIQUE({1;1;2})`:                  "1;2",
		`UNIQUE({1;1;2},"x")`:                    "#VALUE!",
		`UNIQUE(J1)`:                             `"b"`,
		`SORT(J1:J4)`:                            `"A";"a";"b";"c"`,
		`SORT(J1:J4,1,-1)`:                       `"c";"b";"A";"a"`,
		`SORT({3;1;2})`:                          "1;2;3",
		`SORT({3;1;2},,-1)`:                      "3;2;1",
		`SORT(A1:B5,2)`:                          `"A",1;"a",1;"b",2;"b",2;10,"1"`,
		`SORT(A1:B3,2,-1)`:                       `"b",2;"b",2;"A",1`,
		`SORT(A1:C4,3)`:                          `"A",1,FALSE;"b",2,TRUE;"b",2,TRUE;10,"1",#N/A`,
		`SORT(A1:C4,3,-1)`:                       `10,"1",#N/A;"b",2,TRUE;"b",2,TRUE;"A",1,FALSE`,
		`SORT({2,1;"b","a"},2,1,TRUE)`:           `1,2;"a","b"`,
		`SORT({2,1;"b","a"},2,1,FALSE)`:          `2,1;"b","a"`,
		`SORT({3,1,2;"c","a","b"},1,1,TRUE)`:     `1,2,3;"a","b","c"`,
		`SORT({3,1,2;"c","a","b"},2,-1,TRUE)`:    `3,2,1;"c","b","a"`,
		`SORT({1,"b";1,"a";0,"z"},{1,2},{1,-1})`: `0,"z";1,"b";1,"a"`,
		`SORT({1,"b";1,"a";0,"z"},{1;2},{-1;1})`: `1,"a";1,"b";0,"z"`,
		`SORT({1,"b";1,"a";0,"z"},{1,2})`:        `0,"z";1,"a";1,"b"`,
		`SORT({1,"b";1,"a";0,"z"},{1,2},-1)`:     `1,"b";1,"a";0,"z"`,
		`SORT({1,5;1,4;1,3},1)`:                  "1,5;1,4;1,3",
		`SORT(L1:L6)`:                            `1;2;"a";TRUE;#N/A;blank`,
		`SORT(L1:L6,1,-1)`:                       `blank;#N/A;TRUE;"a";2;1`,
		`SORT({1,2},1)`:                          "1,2",
		`SORT({1,2},2)`:                          "1,2",
		`SORT({1,2},3)`:                          "#VALUE!",
		`SORT({1;2},2)`:                          "#VALUE!",
		`SORT({1;2},0)`:                          "#VALUE!",
		`SORT({1;2},1,0)`:                        "#VALUE!",
		`SORT({1;2},1,2)`:                        "#VALUE!",
		`SORT({1;2},1,1,TRUE)`:                   "1;2",
		`SORT({1,2},{1,2},{1,-1,1})`:             "#VALUE!",
		`SORT(#N/A)`:                             "#N/A",
		`SORT(1)`:                                "1",
		`_xlfn._xlws.SORT({2;1})`:                "1;2",
		`SORTBY(J1:J4,J1:J4)`:                    `"A";"a";"b";"c"`,
		`SORTBY(J1:J4,J1:J4,-1)`:                 `"c";"b";"A";"a"`,
		`SORTBY(F1:H2,F1:H1)`:                    `1,2,3;"y","z","x"`,
		`SORTBY(F1:H2,F1:H1,-1)`:                 `3,2,1;"x","z","y"`,
		`SORTBY({"a";"b";"c"},{3;1;2})`:          `"b";"c";"a"`,
		`SORTBY({"a";"b";"c";"d"},{1;0;1;0},-1,{4;3;2;1},1)`: `"c";"a";"d";"b"`,
		`SORTBY({"a";"b";"c";"d"},{1;0;1;0},1,{4;3;2;1})`:    `"d";"b";"c";"a"`,
		`SORTBY({"a";"b";"c"},{3;1})`:                        "#VALUE!",
		`SORTBY({"a";"b";"c"},{3,1,2})`:                      "#VALUE!",
		`SORTBY({"a";"b";"c"},A1:B3)`:                        "#VALUE!",
		`SORTBY({"a";"b";"c"},{3;1;2},{1;2})`:                "#VALUE!",
		`SORTBY({"a";"b";"c"},{3;1;2},2)`:                    "#VALUE!",
		`SORTBY({"a";"b";"c"},{3;1;2},1,{1,2,3},1)`:          "#VALUE!",
		`SORTBY({"a";"b"},{1;2},1,{3;4;5})`:                  "#VALUE!",
		`SORTBY(#N/A,{1;2})`:                                 "#N/A",
		`_xlfn.SORTBY({"a";"b"},{2;1})`:                      `"b";"a"`,
		`FILTER(J1:J4,D1:D4)`:                                `"b";"c"`,
		`FILTER(A1:B3,D1:D3)`:                                `"b",2;"b",2`,
		`FILTER(J1:J4,D1:D4=0)`:                              `"A";"a"`,
		`FILTER(F1:H2,{1,0,1})`:                              `3,2;"x","z"`,
		`FILTER(J1:J4,{1;0;1;0})`:                            `"b";"c"`,
		`FILTER(J1:J4,{5;0;TRUE;0})`:                         `"b";"c"`,
		`FILTER(J1:J4,{0;0;0;0})`:                            "#CALC!",
		`FILTER(J1:J4,{0;0;0;0},"none")`:                     `"none"`,
		`FILTER(J1:J4,{0;0;0;0},)`:                           "#CALC!",
		`FILTER(J1:J4,{0;0;0;0},0)`:                          "0",
		`FILTER(J1:J4,{0;0;0;0},J2)`:                         `"A"`,
		`FILTER(J1:J4,{0;0;0;0},#N/A)`:                       "#N/A",
		`FILTER(J1:J4,{1;1;1;1},"none")`:                     `"b";"A";"c";"a"`,
		`FILTER(J1:J4,{1;"a";0;0})`:                          "#VALUE!",
		`FILTER(J1:J4,{1;#DIV/0!;0;0})`:                      "#DIV/0!",
		`FILTER(J1:J4,{1;0;1})`:                              "#VALUE!",
		`FILTER(J1:J4,{1,0,1,0})`:                            "#VALUE!",
		`FILTER(F1:H2,{1;0})`:                                "3,1,2",
		`FILTER(F1:H2,{1,0})`:                                "#VALUE!",
		`FILTER(J1:J4,A1:B4)`:                                "#VALUE!",
		`FILTER(#N/A,{1})`:                                   "#N/A",
		`FILTER(1,1)`:                                        "1",
		`_xlfn._xlws.FILTER(J1:J4,D1:D4)`:                    `"b";"c"`,
		`FILTER(J1:J4,D1:D4,"x","y")`:                        "#VALUE!",
	} {
		if got := evalArray(t, b, f, func() float64 { return 0.5 }); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func TestRandArrayRand(t *testing.T) {
	values := []float64{0, 0.999, 0.25}
	next := func() func() float64 {
		i := -1
		return func() float64 { i++; return values[i%len(values)] }
	}
	b := book(nil)
	for f, want := range map[string]string{
		`RANDARRAY(1,3)`:           "0,0.999,0.25",
		`RANDARRAY(1,3,0,10)`:      "0,9.99,2.5",
		`RANDARRAY(1,3,1,3,TRUE)`:  "1,3,1",
		`RANDARRAY(1,3,0,3,TRUE)`:  "0,3,1",
		`RANDARRAY(1,3,-2,2,TRUE)`: "-2,2,-1",
	} {
		if got := evalArray(t, b, f, next()); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func TestRandArrayVolatile(t *testing.T) {
	if !Volatile("RANDARRAY") || !Volatile("_xlfn.RANDARRAY") {
		t.Error("RANDARRAY is not volatile")
	}
	for _, name := range []string{"SEQUENCE", "UNIQUE", "SORT", "SORTBY", "FILTER"} {
		if !Known(name) || Volatile(name) {
			t.Errorf("%s: known %v, volatile %v", name, Known(name), Volatile(name))
		}
	}
}
