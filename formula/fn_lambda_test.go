package formula

import (
	"testing"
)

func TestLambda(t *testing.T) {
	b := book(map[string]any{"A1": 1, "A2": "x", "A3": 3, "B1": 10, "B2": 20})
	b.names["FACTR"] = "_xlfn.LAMBDA(_xlpm.n,IF(_xlpm.n<=1,1,_xlpm.n*FACTR(_xlpm.n-1)))"
	b.names["LOOP"] = "LAMBDA(n,LOOP(n+1))"
	b.names["ADDY"] = "LAMBDA(x,x+y)"
	b.names["TWICE"] = "LAMBDA(x,2*x)"
	b.names["TAUX"] = "0.5"
	for f, want := range map[string]string{
		`LET(x,2,y,x*3,x+y)`:                           "8",
		`_xlfn.LET(_xlpm.x,2,_xlpm.x*_xlpm.x)`:         "4",
		`LET(x,1,LET(x,2,x)+x)`:                        "3",
		`LET(x,A1:A3,ROWS(x)&SUM(x))`:                  `"34"`,
		`LET(x,TAUX,x*4)`:                              "2",
		`LET(x,1,y)`:                                   "#NAME?",
		`LET(A1,1,2)`:                                  "#VALUE!",
		`LET(x,1,y,2)`:                                 "#VALUE!",
		`LET(x,1)`:                                     "#VALUE!",
		`LAMBDA(x,x+1)(2)`:                             "3",
		`_xlfn.LAMBDA(_xlpm.x,_xlpm.x+1)(2)`:           "3",
		`LAMBDA(x,y,x*y)(3,4)`:                         "12",
		`LAMBDA(42)()`:                                 "42",
		`LAMBDA(x,x)(1,2)`:                             "#VALUE!",
		`LAMBDA(x,x+1)`:                                "#CALC!",
		`LAMBDA(x,x+1)+1`:                              "#CALC!",
		`LAMBDA(1,2)`:                                  "#VALUE!",
		`LAMBDA(x,x,1)(1,2)`:                           "#VALUE!",
		`LET(f,LAMBDA(x,x*2),f(3)+f(4))`:               "14",
		`LET(_xlpm.f,LAMBDA(x,x*2),_xlpm.f(3))`:        "6",
		`LET(y,10,f,LAMBDA(x,x+y),LET(y,100,f(1)))`:    "11",
		`LET(add,LAMBDA(a,LAMBDA(b,a+b)),add(2)(3))`:   "5",
		`LET(f,5,f(1))`:                                "#VALUE!",
		`LET(sum,5,SUM(sum,1))`:                        "6",
		`IF(TRUE,LAMBDA(x,x*3),0)(2)`:                  "6",
		`LAMBDA(x,x)(#N/A)`:                            "#N/A",
		`LAMBDA(x,x+1)(A1:A3)`:                         "2;#VALUE!;4",
		`LAMBDA(r,ROWS(r))(A1:A3)`:                     "3",
		`LAMBDA(a,b,IF(ISOMITTED(b),"omis",a+b))(1)`:   `"omis"`,
		`LAMBDA(a,b,IF(ISOMITTED(b),"omis",a+b))(1,)`:  `"omis"`,
		`LAMBDA(a,b,IF(ISOMITTED(b),"omis",a+b))(1,2)`: "3",
		`LAMBDA(a,b,ISOMITTED(a))(Z99,1)`:              "FALSE",
		`ISOMITTED(A1)`:                                "FALSE",
		`FACTR(5)`:                                     "120",
		`SUM(FACTR(3),TWICE(4))`:                       "14",
		`LOOP(1)`:                                      "#NUM!",
		`LET(y,5,ADDY(1))`:                             "#NAME?",
		`TAUX(1)`:                                      "#VALUE!",
		`NOSUCH(1)`:                                    "#UNSUPPORTED",
		`MAP({1,2;3,4},LAMBDA(x,x*10))`:                "10,20;30,40",
		`_xlfn.MAP(A1:A3,_xlfn.LAMBDA(_xlpm.v,ISTEXT(_xlpm.v)))`:     "FALSE;TRUE;FALSE",
		`MAP({1,2},{10;20},LAMBDA(a,b,a+b))`:                         "11,12;21,22",
		`MAP({1,2,3},{1,2},LAMBDA(a,b,a+b))`:                         "2,4,#N/A",
		`MAP(A1:A2,B1:B2,LAMBDA(a,b,a&b))`:                           `"110";"x20"`,
		`MAP({1,2},LAMBDA(x,{1,2}))`:                                 "#CALC!,#CALC!",
		`MAP({1,2},LAMBDA(a,b,a))`:                                   "#VALUE!",
		`MAP({1,2},5)`:                                               "#VALUE!",
		`MAP({1,2},#N/A)`:                                            "#N/A",
		`REDUCE(0,{1,2,3},LAMBDA(a,v,a+v))`:                          "6",
		`REDUCE(,A1:A3,LAMBDA(a,v,IF(ISNUMBER(v),a+v,a)))`:           "4",
		`REDUCE(1,{1,2,3,4},LAMBDA(a,v,a*v))`:                        "24",
		`REDUCE(0,{1,2},LAMBDA(a,a))`:                                "#VALUE!",
		`COLUMNS(REDUCE(0,{1,2},LAMBDA(a,v,{1,2,3}+a)))`:             "3",
		`SCAN(0,{1,2,3},LAMBDA(a,v,a+v))`:                            "1,3,6",
		`SCAN("",{"a","b";"c","d"},LAMBDA(a,v,a&v))`:                 `"a","ab";"abc","abcd"`,
		`BYROW({1,2;3,4},LAMBDA(r,SUM(r)))`:                          "3;7",
		`BYCOL({1,2;3,4},LAMBDA(c,SUM(c)))`:                          "4,6",
		`BYROW(A1:B2,LAMBDA(r,COUNT(r)))`:                            "2;1",
		`BYROW({1,2},LAMBDA(r,r))`:                                   "#CALC!",
		`BYCOL({1,2},LAMBDA(c,MAX(c)))`:                              "1,2",
		`MAKEARRAY(2,3,LAMBDA(r,c,r*c))`:                             "1,2,3;2,4,6",
		`MAKEARRAY(1.9,1,LAMBDA(r,c,r+c))`:                           "2",
		`MAKEARRAY(0,1,LAMBDA(r,c,1))`:                               "#VALUE!",
		`MAKEARRAY(2,2,1)`:                                           "#VALUE!",
		`MAKEARRAY(1E+300,1E+300,LAMBDA(r,c,1))`:                     "#NUM!",
		`_xlfn.MAKEARRAY(2,1,_xlfn.LAMBDA(_xlpm.r,_xlpm.c,_xlpm.r))`: "1;2",
	} {
		if got := evalTable(t, b, f, nil); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func TestLambdaEngine(t *testing.T) {
	s, e := newSource()
	s.names["SCALE"] = "_xlfn.LAMBDA(_xlpm.x,_xlpm.x*$A$1)"
	s.names["FACTR"] = "LAMBDA(n,IF(n<=1,$A$2,n*FACTR(n-1)*FACTR(1)))"
	s.set(e, "A1", 3)
	s.set(e, "A2", 1)
	s.set(e, "B1", "=SCALE(2)")
	s.set(e, "B2", "=LAMBDA(x,x+A3)(1)")
	s.set(e, "B3", "=LET(v,A1,MAP({1,2},LAMBDA(x,x+v)))")
	s.set(e, "B4", "=FACTR(4)")
	if got := s.show("B1") + " " + s.show("B2") + " " + s.show("B4"); got != "6 1 24" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "A1", 5)); got != "[B1=10 B3=6]" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "A3", 7)); got != "[B2=8]" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "A2", 2)); got != "[B4=384]" {
		t.Fatal(got)
	}
}

func TestLambdaStored(t *testing.T) {
	for f, want := range map[string]string{
		`LET(x,1,x+1)`:                                           `_xlfn.LET(_xlpm.x,1,_xlpm.x+1)`,
		`LAMBDA(x,y,x+y)(1,2)`:                                   `_xlfn.LAMBDA(_xlpm.x,_xlpm.y,_xlpm.x+_xlpm.y)(1,2)`,
		`LET(f,LAMBDA(x,x*2),f(3)+Taux)`:                         `_xlfn.LET(_xlpm.f,_xlfn.LAMBDA(_xlpm.x,_xlpm.x*2),_xlpm.f(3)+Taux)`,
		`MAP(A1:A3,LAMBDA(v,IF(ISOMITTED(v),0,v)))`:              `_xlfn.MAP(A1:A3,_xlfn.LAMBDA(_xlpm.v,IF(_xlfn.ISOMITTED(_xlpm.v),0,_xlpm.v)))`,
		`LET(x,{1,2},y,x,SUM(x,y))`:                              `_xlfn.LET(_xlpm.x,{1,2},_xlpm.y,_xlpm.x,SUM(_xlpm.x,_xlpm.y))`,
		`LET(x,1,x)+x+Feuil1!x`:                                  `_xlfn.LET(_xlpm.x,1,_xlpm.x)+x+Feuil1!x`,
		`LET( x , 1 , x )`:                                       `_xlfn.LET( _xlpm.x , 1 , _xlpm.x )`,
		`_xlfn.LET(_xlpm.x,1,_xlpm.x)`:                           `_xlfn.LET(_xlpm.x,1,_xlpm.x)`,
		`REDUCE(,A1:A3,LAMBDA(a,v,a+v))`:                         `_xlfn.REDUCE(,A1:A3,_xlfn.LAMBDA(_xlpm.a,_xlpm.v,_xlpm.a+_xlpm.v))`,
		`SCAN(0,A1:A3,LAMBDA(a,v,a+v))`:                          `_xlfn.SCAN(0,A1:A3,_xlfn.LAMBDA(_xlpm.a,_xlpm.v,_xlpm.a+_xlpm.v))`,
		`BYROW(A1:B2,LAMBDA(r,SUM(r)))+BYCOL(A1:B2,LAMBDA(c,1))`: `_xlfn.BYROW(A1:B2,_xlfn.LAMBDA(_xlpm.r,SUM(_xlpm.r)))+_xlfn.BYCOL(A1:B2,_xlfn.LAMBDA(_xlpm.c,1))`,
		`MAKEARRAY(2,2,LAMBDA(r,c,r))`:                           `_xlfn.MAKEARRAY(2,2,_xlfn.LAMBDA(_xlpm.r,_xlpm.c,_xlpm.r))`,
		`FACTR(3)`:                                               `FACTR(3)`,
	} {
		got := Stored(f)
		if got != want {
			t.Errorf("%s: %s, want %s", f, got, want)
		}
		if _, err := Parse(got); err != nil {
			t.Errorf("%s: %v", got, err)
		}
	}
}

func TestLambdaReferences(t *testing.T) {
	b := book(map[string]any{"A1": 1, "A2": "x", "A3": 3, "B1": 10, "B2": 20, "B3": 30, "C1": 5, "C2": 6})
	for f, want := range map[string]string{
		`MAP(A1:A3,LAMBDA(c,ROW(c)))`:                      "1;2;3",
		`MAP(A1:A3,LAMBDA(c,ISREF(c)))`:                    "TRUE;TRUE;TRUE",
		`MAP(A1:B2,LAMBDA(c,COLUMN(c)))`:                   "1,2;1,2",
		`MAP(A1:A2,B1:B2,LAMBDA(a,b,ROW(a)+COLUMN(b)))`:    "3;4",
		`MAP(A1:A3,B1:B2,LAMBDA(a,b,ISREF(b)))`:            "TRUE;TRUE;FALSE",
		`MAP(A1:A3,LAMBDA(c,OFFSET(c,0,1)))`:               "10;20;30",
		`MAP(A1:A3,LAMBDA(c,ROWS(c)*10+COLUMNS(c)))`:       "11;11;11",
		`MAP(A1:A3,LAMBDA(c,c))`:                           `1;"x";3`,
		`MAP({1;2;3},LAMBDA(c,ISREF(c)))`:                  "FALSE;FALSE;FALSE",
		`MAP(TRANSPOSE(A1:A3),LAMBDA(c,ISREF(c)))`:         "FALSE,FALSE,FALSE",
		`BYROW(A1:C2,LAMBDA(r,ROW(r)))`:                    "1;2",
		`BYROW(A1:C2,LAMBDA(r,COLUMN(r)))`:                 "1;1",
		`BYROW(A1:C2,LAMBDA(r,ROWS(r)*10+COLUMNS(r)))`:     "13;13",
		`BYROW(A1:C2,LAMBDA(r,ISREF(r)))`:                  "TRUE;TRUE",
		`BYROW(A1:C2,LAMBDA(r,SUM(OFFSET(r,1,0))))`:        "26;33",
		`BYROW(A1:C3,LAMBDA(r,INDEX(r,2)))`:                "10;20;30",
		`BYCOL(A1:C2,LAMBDA(c,COLUMN(c)))`:                 "1,2,3",
		`BYCOL(A1:C2,LAMBDA(c,ROW(c)))`:                    "1,1,1",
		`BYCOL(A1:C2,LAMBDA(c,ROWS(c)*10+COLUMNS(c)))`:     "21,21,21",
		`BYCOL(A1:C2,LAMBDA(c,ISREF(c)))`:                  "TRUE,TRUE,TRUE",
		`BYCOL(A1:C2,LAMBDA(c,SUM(OFFSET(c,0,1))))`:        "30,11,0",
		`BYROW({1,2;3,4},LAMBDA(r,ISREF(r)))`:              "FALSE;FALSE",
		`BYCOL({1,2;3,4},LAMBDA(c,ISREF(c)))`:              "FALSE,FALSE",
		`REDUCE(0,A1:A3,LAMBDA(a,v,a+IF(ISREF(v),100,1)))`: "3",
		`SCAN(0,A1:A2,LAMBDA(a,v,a+IF(ISREF(v),100,1)))`:   "1;2",
		`MAKEARRAY(1,1,LAMBDA(r,c,IF(ISREF(r),100,1)))`:    "1",
		`ROWS(BYROW(A:A,LAMBDA(r,ROW(r))))`:                "3",
	} {
		if got := evalTable(t, b, f, nil); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func TestLambdaReferencesEngine(t *testing.T) {
	s, e := newSource()
	s.set(e, "A1", 1)
	s.set(e, "A2", 2)
	s.set(e, "B1", "=SUM(MAP(A1:A2,LAMBDA(c,ROW(c)*c)))")
	s.set(e, "B2", "=SUM(BYROW(A1:A2,LAMBDA(r,SUM(r))))")
	s.set(e, "B3", "=SUM(BYCOL(A1:A2,LAMBDA(c,SUM(c))))")
	if got := s.show("B1") + " " + s.show("B2") + " " + s.show("B3"); got != "5 3 3" {
		t.Fatal(got)
	}
	if got := results(s.set(e, "A2", 5)); got != "[B1=11 B2=6 B3=6]" {
		t.Fatal(got)
	}
}
