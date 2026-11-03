package formula

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var fuzzSeeds = []string{
	`1+2*3`, `-2^2`, `10%`, `"a"&1.5`, `{1,2;3,4}`, `A1:B2 B2:C3`, `SUM((A1,C1))`, `Data!A1+Data!A2`,
	`SUM(Feuil1:Data!A1)`, `IF(A1,,2)`, `A1#`, `@A1`, `$A$1:$B$2`, `1:3`, `A:C`, `'Feuil1'!A1`, `[1]Data!A1`,
	`_xlfn.CONCAT("a","b")`, `Table1[[#This Row],[Col]]`, `INDEX(A1:A5,MATCH(2,B1:B5,0))`,
	`VLOOKUP(A1,C1:D5,2,FALSE)`, `TEXT(A1,"0.00")`, `REPT("ab",3)`, `SEQUENCE(3,3)`, `#REF!+1`, `SUMIF(A1:A5,">1")`,
	`IF(A1>0,IF(A2>0,1,2),3)`, `LET(x,1,x+1)`, `SUBTOTAL(9,A1:A5)`, `DATE(2024,12,25)+NOW()`,
	`TAUX*C1`, `SUM(PRIX)`, `E1+F1`, `F1+E1`,
}

// the engine of a workbook whose formulas read each other
func fuzzEngine(f string) {
	s, e := newSource()
	s.names["TAUX"] = "0.2"
	s.names["PRIX"] = "Feuil1!$C$1:$C$5"
	s.set(e, "A1", 1)
	s.set(e, "A2", "x")
	s.set(e, "C1", 10)
	s.set(e, "F1", "=E1+1")
	s.set(e, "E1", "=F1+1")
	s.set(e, "B1", "="+f)
	s.set(e, "B2", "=B1+A1")
	s.set(e, "A1", 2)
	s.cells[[3]int{0, 3, 1}] = Num(3)
	s.set(e, "A3", "=A3+1")
}

func FuzzFormula(f *testing.F) {
	for _, s := range fuzzSeeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, formula string) {
		x, err := Parse(formula)
		if err != nil {
			if !errors.Is(err, ErrSyntax) && !errors.Is(err, ErrDepth) {
				t.Fatalf("unexpected error: %v", err)
			}
			return
		}
		b := book(sheet)
		b.names["TAUX"] = "0.2"
		c := &Context{Book: b, Row: 20, Col: 10, Locale: French,
			Now: time.Date(2024, 12, 25, 18, 30, 0, 0, time.UTC), Rand: func() float64 { return 0.5 }}
		c.scalar(c.Eval(x))
		fuzzEngine(formula)

		// the text of a formula written again reads the same
		again, err := Rewrite(formula, func(*Ref) bool { return true })
		if err != nil {
			t.Fatalf("%q: rewritten: %v", formula, err)
		}
		y, err := Parse(again)
		if err != nil {
			t.Fatalf("%q: %q does not read: %v", formula, again, err)
		}
		if !reflect.DeepEqual(x, y) {
			t.Fatalf("%q reads differently once written %q", formula, again)
		}
		Translate(formula, 1, 1)
		Shift(formula, "Feuil1", true, true, 1, 1)
		Shift(formula, "Feuil1", false, false, 1, -1)
		Rename(formula, "Data", "Autre")
		Drop(formula, "Data")
	})
}

func TestParseDepth(t *testing.T) {
	deep := map[string]string{
		"parentheses": strings.Repeat("(", 100000) + "1" + strings.Repeat(")", 100000),
		"functions":   strings.Repeat("SUM(", 100000) + "1" + strings.Repeat(")", 100000),
		"unary":       strings.Repeat("-", 100000) + "1",
		"unclosed":    strings.Repeat("(", 100000),
	}
	for name, f := range deep {
		if _, err := Parse(f); !errors.Is(err, ErrDepth) {
			t.Errorf("%s: %v", name, err)
		}
		if got, err := Rewrite(f, func(*Ref) bool { return true }); err != nil || got != f {
			t.Errorf("%s: rewritten: %v", name, err)
		}
	}
	level := func(n int) string { return strings.Repeat("SUM(", n) + "1" + strings.Repeat(")", n) }
	if _, err := Parse(level(MaxDepth)); err != nil {
		t.Errorf("%d levels: %v", MaxDepth, err)
	}
	if _, err := Parse(level(MaxDepth + 1)); !errors.Is(err, ErrDepth) {
		t.Errorf("%d levels: %v", MaxDepth+1, err)
	}
}
