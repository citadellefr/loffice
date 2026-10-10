package legacy

import (
	"errors"
	"testing"
)

func TestFormula(t *testing.T) {
	b := &book{
		sheets:   []boundSheet{{name: "Data"}, {name: "My Sheet"}},
		supbooks: []supbook{{kind: supInternal}, {kind: supAddIn, names: []string{"_xlfn.COUNTIFS", "EDATE"}}, {kind: supExternal}},
		externs:  []extern{{0, 1, 1}, {1, 0xFFFE, 0xFFFE}, {2, 0, 0}, {0, 0xFFFF, 0xFFFF}, {0, 0, 1}},
		names:    []name{{name: "Rate"}},
	}
	cell := origin{row: 5, col: 2, rows: biffRows, cols: biffCols}
	const one = "000000000000f03f"
	tests := []struct {
		code, extra string
		want        string
		err         error
	}{
		{code: "44000000c0" + "1e0200" + "03", want: "A1+2"},
		{code: "250000010000" + "0001c0" + "19100000", want: "SUM($A$1:B2)"},
		{code: "44000000c0" + "1e0000" + "0d" + "17010079" + "16" + "42030100", want: `IF(A1>0,"y",)`},
		{code: "1702006122" + "1700" + "00" + "08", want: `"a"""&""`},
		{code: "1e0500" + "14" + "13" + "15", want: "(-5%)"},
		{code: "1f" + one + "44000000c0" + "410f00" + "06", want: "1/SIN(A1)"},
		{code: "1c07" + "1d01" + "0b", want: "#DIV/0!=TRUE"},
		// a shared formula counts its references from its cell
		{code: "4cffff00c0" + "1e0200" + "05", want: "C5*2"},
		{code: "2dfeff0100ffc001c0", want: "B4:D7"},
		{code: "250000ffff00000000", want: "$A:$A"},
		{code: "25020002000000ff00", want: "$3:$3"},
		// other sheets, through the table of references
		{code: "3a0000" + "00000000", want: "'My Sheet'!$A$1"},
		{code: "3b0400" + "0000010000c001c0", want: "'Data:My Sheet'!A1:B2"},
		{code: "3a0300" + "00000000", want: "#REF!"},
		{code: "3c0000" + "00000000", want: "#REF!"},
		{code: "3a0200" + "00000000", err: errExternal},
		// names, and the functions of add-ins
		{code: "2301000000" + "1e0a00" + "05", want: "Rate*10"},
		{code: "390100" + "01000000" + "44000000c0" + "1e0100" + "4203ff00", want: "COUNTIFS(A1,1)"},
		{code: "390100" + "02000000" + "44000000c0" + "1e0100" + "4203ff00", want: "EDATE(A1,1)"},
		{code: "390200" + "01000000", err: errExternal},
		// the constants of arrays follow the tokens
		{code: "6000000000000000" + "19100000", extra: "010100" + "01" + one + "0201000061" + "040100000000000000" + "100700000000000000", want: `SUM({1,"a";TRUE,#DIV/0!})`},
		{code: "6000000000000000", extra: "0101", err: errFormula},
		// what is not understood
		{code: "03", err: errFormula},
		{code: "18", err: errFormula},
		{code: "0203000400", err: errFormula},
		{code: "1e05", err: errFormula},
		{code: "1e0100" + "1e0200", err: errFormula},
		{code: "44000000c0" + "42017f01", err: errFormula},
	}
	for _, c := range tests {
		got, err := b.formula(unhex(t, c.code), unhex(t, c.extra), cell)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("%s: %q, %v; want %q, %v", c.code, got, err, c.want, c.err)
		}
	}

	// in a name every relative reference is counted from the cell that
	// uses it: written for A1, it wraps around the sheet of today
	got, err := b.formula(unhex(t, "3a0000"+"ffff00c0"), nil, origin{rows: 1 << 20, cols: 1 << 14, name: true})
	if got != "'My Sheet'!A1048576" || err != nil {
		t.Errorf("relative name: %q, %v", got, err)
	}
}
