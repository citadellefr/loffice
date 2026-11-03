package formula

import (
	"testing"
	"time"
)

func TestInfoAndEngineeringFunctions(t *testing.T) {
	b := book(map[string]any{"A1": 3, "A2": 4, "B1": 5, "B2": 12, "C3": "x", "Data!A1": 7, "T1": 1, "A3": 6, "B3": 8})
	b.cells[[3]int{0, 1, 4}] = Str("")
	b.cells[[3]int{0, 5, 1}] = Boolean(true)
	for f, want := range map[string]string{
		`SHEET()`:                                    "1",
		`SHEET(Data!A1)`:                             "2",
		`SHEET("Data")`:                              "2",
		`SHEET("Nope")`:                              "#N/A",
		`SHEETS(Feuil1:Data!A1)`:                     "2",
		`AREAS(A1:B2)`:                               "1",
		`CELL("address",B2)`:                         `"$B$2"`,
		`CELL("col",B2)`:                             "2",
		`CELL("row",B2)`:                             "2",
		`CELL("contents",B2)`:                        "12",
		`CELL("type",C3)`:                            `"l"`,
		`CELL("type",B2)`:                            `"v"`,
		`CELL("type",Z9)`:                            `"b"`,
		`CELL("nope",B2)`:                            "#VALUE!",
		`INFO("recalc")`:                             `"Automatique"`,
		`INFO("nope")`:                               "#VALUE!",
		`HYPERLINK("http://a.b","lien")`:             `"lien"`,
		`HYPERLINK("http://a.b")`:                    `"http://a.b"`,
		`VALUETOTEXT("a")`:                           `"a"`,
		`VALUETOTEXT("a",1)`:                         `""a""`,
		`VALUETOTEXT(1234)`:                          `"1234"`,
		`ARRAYTOTEXT({1,2;3,4})`:                     `"1, 2, 3, 4"`,
		`ARRAYTOTEXT({1,2;3,"a"},1)`:                 `"{1,2;3,"a"}"`,
		`LENB("été")`:                                "3",
		`LEFTB("abc",2)`:                             `"ab"`,
		`RIGHTB("abc",2)`:                            `"bc"`,
		`MIDB("abcd",2,2)`:                           `"bc"`,
		`ASC("ＡＢＣ")`:                                 `"ABC"`,
		`SUMX2MY2({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})`: "-55",
		`SUMX2PY2({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})`: "521",
		`SUMXMY2({2,3,9,1,8,7,5},{6,5,11,7,5,4,4})`:  "79",
		`SERIESSUM(2,0,2,{1,2,3})`:                   "57",
		`ROUND(SERIESSUM(PI()/4,0,2,{1,-0.5,0.0416666666666667,-0.00138888888888889}),8)`: "0.70710321",
		`MULTINOMIAL(2,3,4)`:      "1260",
		`BITAND(1,5)`:             "1",
		`BITOR(23,10)`:            "31",
		`BITXOR(5,3)`:             "6",
		`BITLSHIFT(4,2)`:          "16",
		`BITRSHIFT(13,2)`:         "3",
		`BITAND(-1,5)`:            "#NUM!",
		`BIN2DEC(1100100)`:        "100",
		`BIN2DEC(1111111111)`:     "-1",
		`BIN2HEX(11111011,4)`:     `"00FB"`,
		`BIN2HEX(1111111111)`:     `"FFFFFFFFFF"`,
		`BIN2OCT(1001,3)`:         `"011"`,
		`DEC2BIN(9,4)`:            `"1001"`,
		`DEC2BIN(-100)`:           `"1110011100"`,
		`DEC2HEX(100,4)`:          `"0064"`,
		`DEC2HEX(-54)`:            `"FFFFFFFFCA"`,
		`DEC2OCT(58,3)`:           `"072"`,
		`DEC2BIN(512)`:            "#NUM!",
		`HEX2BIN("F",8)`:          `"00001111"`,
		`HEX2DEC("FF")`:           "255",
		`HEX2DEC("FFFFFFFF5B")`:   "-165",
		`HEX2OCT("F",3)`:          `"017"`,
		`OCT2BIN(3,3)`:            `"011"`,
		`OCT2DEC(54)`:             "44",
		`OCT2DEC(7777777533)`:     "-165",
		`OCT2HEX(100,4)`:          `"0040"`,
		`ROMAN(499)`:              `"CDXCIX"`,
		`ROMAN(499,1)`:            `"LDVLIV"`,
		`ROMAN(499,4)`:            `"ID"`,
		`ROMAN(2024)`:             `"MMXXIV"`,
		`ARABIC("LVII")`:          "57",
		`ARABIC("MCMXII")`:        "1912",
		`ARABIC("a")`:             "#VALUE!",
		`CONVERT(1,"lbm","kg")`:   "0.45359237",
		`CONVERT(68,"F","C")`:     "20",
		`CONVERT(2.5,"ft","sec")`: "#N/A",
		`CONVERT(1,"mi","km")`:    "1.609344",
		`CONVERT(1,"hr","mn")`:    "60",
		`CONVERT(1,"gal","l")`:    "3.785411784",
		`CONVERT(1,"km2","m2")`:   "1000000",
		`DELTA(5,4)`:              "0",
		`DELTA(1.5,1.5)`:          "1",
		`GESTEP(5,4)`:             "1",
		`GESTEP(-1)`:              "0",
		`ROUND(ERF(1),9)`:         "0.842700793",
		`ROUND(ERF(0,1),9)`:       "0.842700793",
		`ROUND(ERFC(1),9)`:        "0.157299207",
		`ROUND(BESSELJ(1.9,2),6)`: "0.329926",
	} {
		if got := eval(t, b, French, f).show(); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func TestIndirectL1C1(t *testing.T) {
	b := book(map[string]any{"A1": 1, "B2": 2, "C3": 30, "D4": 40, "E5": 50, "Data!B2": 99})
	for f, want := range map[string]string{
		`INDIRECT("L3C3",FALSE)`:                         "30",
		`INDIRECT("R3C3",FALSE)`:                         "30",
		`INDIRECT("L[-18]C[-8]",FALSE)`:                  "2",
		`INDIRECT("L[-19]C[-9]",FALSE)`:                  "1",
		`INDIRECT("LC",FALSE)`:                           "blank",
		`SUM(INDIRECT("L3C3:L5C5",FALSE))`:               "120",
		`SUM(INDIRECT("L[-18]C[-8]:L[-16]C[-6]",FALSE))`: "72",
		`INDIRECT("Data!L2C2",FALSE)`:                    "99",
		`SUM(INDIRECT("C3",FALSE))`:                      "30",
		`SUM(INDIRECT("L3",FALSE))`:                      "30",
		`INDIRECT("L0C1",FALSE)`:                         "#REF!",
		`INDIRECT("L[-30]C1",FALSE)`:                     "#REF!",
		`INDIRECT("A1",FALSE)`:                           "#REF!",
		`INDIRECT("L3C3")`:                               "#REF!",
	} {
		if got := eval(t, b, French, f).show(); got != want {
			t.Errorf("%s = %s, want %s", f, got, want)
		}
	}
}

func TestFormulaText(t *testing.T) {
	s := &sheetSource{mapBook: book(nil), formulas: map[Pos]string{}}
	e := NewEngine(s, Options{Locale: English, Now: func() time.Time { return time.Time{} }})
	s.set(e, "A1", 2)
	s.set(e, "B1", "=A1*2")
	s.set(e, "C1", "=FORMULATEXT(B1)")
	s.set(e, "C2", "=FORMULATEXT(A1)")
	if got := s.cells[[3]int{0, 1, 3}]; got.Str != "=A1*2" {
		t.Errorf("FORMULATEXT(B1) = %v", got)
	}
	if got := s.cells[[3]int{0, 2, 3}]; got.Str != "#N/A" {
		t.Errorf("FORMULATEXT(A1) = %v", got)
	}
}
