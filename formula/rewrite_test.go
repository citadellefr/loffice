package formula

import (
	"strings"
	"testing"
)

func TestTokens(t *testing.T) {
	for f, want := range map[string]string{
		`SUM(A1:B2,$C$3)*2`:                   "Function Reference Comma Reference Close Operator Number",
		`'My ''sheet'''!A1+Sheet2!B:B`:        "Reference Operator Reference",
		`IF(A1>=1,"a""b",#N/A)`:               "Function Reference Operator Number Comma String Comma Error Close",
		`LOG10(100)+A1B`:                      "Function Number Close Operator Name",
		`{1,2;3,4}`:                           "OpenArray Number Comma Number Semicolon Number Comma Number CloseArray",
		`Sales[[#This Row],[Amount]] * 1.5e3`: "Name Space Operator Space Number",
		`Sheet1:Sheet3!A1 + [1]Data!$A$1`:     "Reference Space Operator Space Reference",
		`1:3 A:A`:                             "Reference Space Reference",
		`Sheet1!#REF!+Total+Sheet1!Total`:     "Reference Operator Name Operator Name",
		`_xlfn.XLOOKUP(A1,B:B,C:C)`:           "Function Reference Comma Reference Comma Reference Close",
		`TRUE+false+A1#`:                      "Bool Operator Bool Operator Reference Operator",
		`XFD1048576+XFE1+A1048577`:            "Reference Operator Name Operator Name",
		`-50%`:                                "Operator Number Operator",
	} {
		tokens, err := Tokens(f)
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		var kinds []string
		var joined strings.Builder
		for _, tok := range tokens {
			kinds = append(kinds, kindNames[tok.Kind])
			joined.WriteString(tok.Text)
		}
		if got := strings.Join(kinds, " "); got != want || joined.String() != f {
			t.Errorf("%s: %s, want %s", f, got, want)
		}
	}
	for _, f := range []string{`"open`, `'sheet!A1`, `Table[x`} {
		if _, err := Tokens(f); err == nil {
			t.Errorf("%s: no error", f)
		}
	}
}

var kindNames = map[Kind]string{
	Number: "Number", String: "String", Bool: "Bool", Error: "Error", Reference: "Reference", Name: "Name",
	Function: "Function", Operator: "Operator", Open: "Open", Close: "Close", Comma: "Comma",
	Semicolon: "Semicolon", OpenArray: "OpenArray", CloseArray: "CloseArray", Space: "Space",
}

func TestTranslate(t *testing.T) {
	for _, c := range []struct {
		f      string
		dr, dc int
		want   string
	}{
		{"A1+$B$2+B$3+$C4", 1, 1, "B2+$B$2+C$3+$C5"},
		{"SUM(A1:A10)", 2, 0, "SUM(A3:A12)"},
		{"SUM(A:A,1:1)", 3, 2, "SUM(C:C,4:4)"},
		{"'My sheet'!A1*2", 0, 1, "'My sheet'!B1*2"},
		{"A1+B2", -1, 0, "#REF!+B1"},
		{`"A1"&A1`, 1, 0, `"A1"&A2`},
	} {
		got, err := Translate(c.f, c.dr, c.dc)
		if err != nil || got != c.want {
			t.Errorf("Translate(%s, %d, %d) = %s, %v; want %s", c.f, c.dr, c.dc, got, err, c.want)
		}
	}
}

func TestShift(t *testing.T) {
	for _, c := range []struct {
		f         string
		own, rows bool
		at, n     int
		want      string
	}{
		{"A1+A5+SUM(A2:A8)", true, true, 3, 2, "A1+A7+SUM(A2:A10)"},
		{"A1+A5+SUM(A2:A8)", true, true, 3, -2, "A1+A3+SUM(A2:A6)"},
		{"A3+SUM(A3:A4)+SUM(A1:A2)", true, true, 3, -2, "#REF!+SUM(#REF!)+SUM(A1:A2)"},
		{"Data!B2+B2", false, false, 1, 1, "Data!C2+B2"},
		{"data!B2+B2", true, false, 1, 1, "data!C2+C2"},
		{"Other!B2+B:B+1:1", true, false, 2, -1, "Other!B2+#REF!+1:1"},
		{"SUM(A1:C1)", true, false, 2, -1, "SUM(A1:B1)"},
	} {
		got, err := Shift(c.f, "Data", c.own, c.rows, c.at, c.n)
		if err != nil || got != c.want {
			t.Errorf("Shift(%s, %v, %v, %d, %d) = %s, %v; want %s", c.f, c.own, c.rows, c.at, c.n, got, err, c.want)
		}
	}
}

func TestRename(t *testing.T) {
	got, err := Rename("Data!A1+'Data'!B2+Other!C3+A4", "Data", "My data")
	if want := "'My data'!A1+'My data'!B2+Other!C3+A4"; err != nil || got != want {
		t.Errorf("got %s, %v; want %s", got, err, want)
	}
}

func TestNames(t *testing.T) {
	for col, name := range map[int]string{1: "A", 26: "Z", 27: "AA", 702: "ZZ", 703: "AAA", MaxCols: "XFD"} {
		if got := ColumnName(col); got != name {
			t.Errorf("ColumnName(%d) = %s", col, got)
		}
		if r, c, ok := ParseCell(name + "7"); !ok || r != 7 || c != col {
			t.Errorf("ParseCell(%s7) = %d, %d, %v", name, r, c, ok)
		}
	}
}

func TestDrop(t *testing.T) {
	got, err := Drop("Data!A1+'data'!B2:C3+Other!C3+A4", "Data")
	if want := "#REF!+#REF!+Other!C3+A4"; err != nil || got != want {
		t.Errorf("got %s, %v; want %s", got, err, want)
	}
}

func TestStored(t *testing.T) {
	for f, want := range map[string]string{
		"SUM(A1:A3)":                         "SUM(A1:A3)",
		"SORT(UNIQUE(A1:A9))":                "_xlfn._xlws.SORT(_xlfn.UNIQUE(A1:A9))",
		"_xlfn._xlws.filter(A1:A3,B1:B3>0)":  "_xlfn._xlws.filter(A1:A3,B1:B3>0)",
		"SUM(A1#)+Feuil2!$B$2#":              "SUM(_xlfn.ANCHORARRAY(A1))+_xlfn.ANCHORARRAY(Feuil2!$B$2)",
		"sequence(3)&\"#(\"":                 "_xlfn.sequence(3)&\"#(\"",
		"IFERROR(A1,#N/A)":                   "IFERROR(A1,#N/A)",
		"NORM.S.DIST(T.TEST(A:A,B:B,2,1),1)": "_xlfn.NORM.S.DIST(_xlfn.T.TEST(A:A,B:B,2,1),1)",
		"MMULT(MUNIT(2),A1:B2)":              "MMULT(_xlfn.MUNIT(2),A1:B2)",
		"TAKE(VSTACK(A1,B1),1)":              "_xlfn.TAKE(_xlfn.VSTACK(A1,B1),1)",
	} {
		if got := Stored(f); got != want {
			t.Errorf("%s: %s, want %s", f, got, want)
		}
	}
}

func TestStoredNewFunctions(t *testing.T) {
	for _, name := range strings.Fields(`ARABIC ARRAYTOTEXT BETA.DIST BINOM.DIST BITAND BITLSHIFT BITOR BITRSHIFT BITXOR
		CHISQ.DIST CHISQ.DIST.RT CHISQ.TEST CONFIDENCE.NORM CONFIDENCE.T COVARIANCE.P COVARIANCE.S EXPON.DIST F.DIST
		F.DIST.RT FORECAST.LINEAR FORMULATEXT GAMMA.DIST ISFORMULA LOGNORM.DIST MODE.SNGL MUNIT NORM.DIST NORM.INV
		NORM.S.DIST NORM.S.INV PERCENTILE.EXC PERCENTILE.INC PERCENTRANK.INC POISSON.DIST QUARTILE.EXC QUARTILE.INC
		RANK.AVG RANK.EQ RRI SHEET SHEETS STDEV.P STDEV.S T.DIST T.DIST.2T T.DIST.RT T.INV T.INV.2T T.TEST VALUETOTEXT
		VAR.P VAR.S`) {
		if got, want := Stored(strings.ToLower(name)+"(1)"), "_xlfn."+strings.ToLower(name)+"(1)"; got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}
