package xlsx

import (
	"encoding/json"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

func TestPrintRead(t *testing.T) {
	for name, want := range map[string]string{
		"poi/49156.xlsx": `{"scale":47,"margins":{"l":0.17,"r":0.17,"t":0.31,"b":0.26,"head":0.18,"foot":0.16},"rows":[52],` +
			`"foot":"\u0026L\u0026A\u0026CPage \u0026P of \u0026N"}`,
	} {
		_, tree := openCorpus(t, name)
		if got := string(firstSheet(tree).Attrs[printKey]); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}

// Breaks follow the rows they come before, and leave with them.
func TestPrintMoved(t *testing.T) {
	d, tree := openCorpus(t, "poi/49156.xlsx")
	sheet := firstSheet(tree).ID
	c := NewCalc(tree, formula.Options{})
	breaks := func(tr *ot.Tree) []int {
		var p Print
		_ = json.Unmarshal(tr.Node(sheet).Attrs[printKey], &p)
		return p.Rows
	}
	e := ot.Edit{{Op: ot.OpIns, ID: sheet, Dim: ot.DimRows, At: 3, N: 2}}
	if err := moveRows(t, d, tree, e); err != nil {
		t.Fatal(err)
	}
	c.Follow(e, nil)
	if got := breaks(tree); len(got) != 1 || got[0] != 54 {
		t.Fatalf("after rows inserted: %v", got)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	if got := breaks(again); len(got) != 1 || got[0] != 54 {
		t.Fatalf("read back: %v", got)
	}
}

func TestPrintChecked(t *testing.T) {
	for raw, ok := range map[string]bool{
		`{"paper":9,"land":true,"scale":80}`: true,
		`{"fit":[1,0]}`:                      true,
		`{"scale":5}`:                        false,
		`{"rows":[1]}`:                       false,
		`{"rows":[9,4]}`:                     false,
		`{"cols":[16385]}`:                   false,
		`{"margins":{"l":-1}}`:               false,
		`{"fit":[1,-2]}`:                     false,
		`[]`:                                 false,
	} {
		if got := checkPrint(json.RawMessage(raw)); got != ok {
			t.Errorf("%s: got %v", raw, got)
		}
	}
}
