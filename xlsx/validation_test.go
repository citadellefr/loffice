package xlsx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/trame/ot"
)

func TestValidationRead(t *testing.T) {
	for _, x := range []struct {
		file string
		want Validation
	}{
		{"libreoffice/data_validation_test.xlsx", Validation{Ref: "A1", Type: "list", F1: "Sheet2!$A$2:$A$4", Blank: true, Err: "stop"}},
		{"poi/dataValidationTableRange.xlsx", Validation{Ref: "B5", Type: "list", F1: "states", Blank: true, Err: "stop"}},
	} {
		data, err := os.ReadFile("../corpus/files/" + x.file)
		if err != nil {
			t.Skip("no corpus: go run ./tools/corpus")
		}
		_, tree, err := Open(data, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		var all []Validation
		for _, n := range tree.Children("book") {
			var v []Validation
			_ = json.Unmarshal(n.Attrs[validKey], &v)
			all = append(all, v...)
		}
		found := false
		for _, v := range all {
			found = found || v == x.want
		}
		if !found {
			t.Errorf("%s: no %+v in %+v", x.file, x.want, all)
		}
	}
}

func TestValidationFollow(t *testing.T) {
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{
			"name": json.RawMessage(`"Feuil1"`),
			validKey: json.RawMessage(`[{"ref":"B2:B5 D3","type":"list","f1":"$A$1:$A$3"},{"ref":"C1","type":"list","f1":"Choix!$A$1:$A$2"},` +
				`{"ref":"F4","type":"list","f1":"\"a,b\""},{"ref":"G1:G2","type":"decimal","f1":"Choix!$A$1","f2":"Choix!$A$2"}]`),
		}, Cells: []ot.Cell{}},
		{Op: ot.OpNew, ID: "S2", Type: "sheet", Parent: "book", Key: "L", Attrs: ot.Values{"name": json.RawMessage(`"Choix"`)}, Cells: []ot.Cell{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	valid := func() string { return string(tree.Node("S1").Attrs[validKey]) }

	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 2}})
	want := `[{"ref":"B4:B7 D5","type":"list","f1":"$A$1:$A$5"},{"ref":"C1","type":"list","f1":"Choix!$A$1:$A$2"},` +
		`{"ref":"F6","type":"list","f1":"\"a,b\""},{"ref":"G1:G4","type":"decimal","f1":"Choix!$A$1","f2":"Choix!$A$2"}]`
	if got := valid(); got != want {
		t.Fatalf("rows inserted: got %s\nwant %s", got, want)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S2", Dim: ot.DimRows, At: 1, N: 1}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{"name": json.RawMessage(`"Valeurs"`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimCols, At: 4, N: 3}})
	want = `[{"ref":"B4:B7","type":"list","f1":"$A$1:$A$5"},{"ref":"C1","type":"list","f1":"Valeurs!$A$2:$A$3"},` +
		`{"ref":"D1:D4","type":"decimal","f1":"Valeurs!$A$2","f2":"Valeurs!$A$3"}]`
	if got := valid(); got != want {
		t.Fatalf("columns removed: got %s\nwant %s", got, want)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 1, N: 10}})
	if got := valid(); got != "" {
		t.Fatalf("all removed: got %s", got)
	}
}

// What an edit sets while rows were inserted above moves with them.
func TestRulesRebased(t *testing.T) {
	tree, c := calcTree(t)
	ins := ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 1, N: 2}}
	edit(t, tree, c, ins)
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{
		validKey:       json.RawMessage(`[{"ref":"B1:B3","type":"custom","f1":"B1>Data!A1"}]`),
		conditionalKey: json.RawMessage(`[{"ref":"A1:A5","type":"formula","pri":1,"f":["A1>B1"],"style":{"font":{"b":true}}}]`),
	}}}, ins)
	if got, want := string(tree.Node("S1").Attrs[validKey]), `[{"ref":"B3:B5","type":"custom","f1":"B3\u003eData!A1"}]`; got != want {
		t.Errorf("validation: got %s, want %s", got, want)
	}
	if got, want := string(tree.Node("S1").Attrs[conditionalKey]), `[{"ref":"A3:A7","type":"formula","pri":1,"f":["A3\u003eB3"],"style":{"font":{"b":true}}}]`; got != want {
		t.Errorf("conditional format: got %s, want %s", got, want)
	}
}

// validation is one validation, whose fields the later ones of the text
// override.
func validation(fields string) json.RawMessage {
	return json.RawMessage(`[{"ref":"A1:A3","type":"whole","f1":"1","f2":"9",` + fields + `}]`)
}

func TestValidationChecked(t *testing.T) {
	long := func(n int) string { return strings.Repeat("é", n) }
	for _, ok := range []string{
		`"x":1`, `"op":"equal","f2":""`, `"type":"list","f1":"\"a,b\"","f2":""`, `"type":"custom","f1":"A1>0","f2":""`,
		`"type":"","f1":"","f2":"","prompt":"Saisir"`, `"err":"warning","errTitle":"` + long(32) + `","errText":"` + long(255) + `"`,
		`"type":"list","f1":"\"` + long(255) + `\"","f2":""`, `"ref":"B2:B5 D3 F:F 3:4"`,
	} {
		if !checkValidations(validation(ok)) {
			t.Errorf("%s refused", ok)
		}
	}
	for _, bad := range []string{
		`"ref":""`, `"ref":"nope"`, `"ref":"A1 B"`, `"type":"foo"`, `"op":"foo"`, `"op":"between"`, `"op":"equal"`,
		`"type":"list","f1":"A1","f2":"2"`, `"type":"list","f1":"A1","op":"equal"`,
		`"type":"custom","f1":"A1","op":"equal"`, `"f1":""`, `"f1":"SUM(("`, `"f2":""`, `"f2":"SUM(("`,
		`"type":"","f1":"1","f2":"","prompt":"x"`, `"type":"","f1":"","f2":""`, `"type":"","f1":"","f2":"","op":"equal","prompt":"x"`,
		`"err":"foo"`, `"err":"stop","errTitle":"` + long(33) + `"`, `"err":"stop","errText":"` + long(256) + `"`,
		`"promptTitle":"` + long(33) + `"`, `"prompt":"` + long(256) + `"`, `"errTitle":"x"`, `"errText":"x"`,
		`"type":"list","f1":"\"` + long(256) + `\"","f2":""`, `"f1":"` + strings.Repeat("1+", maxFormula) + `1"`,
	} {
		if checkValidations(validation(bad)) {
			t.Errorf("%s taken", bad)
		}
	}
	many := "[" + strings.Repeat(`{"ref":"A1","type":"custom","f1":"A1"},`, MaxRules) + `{"ref":"A1","type":"custom","f1":"A1"}]`
	if checkValidations(json.RawMessage(many)) {
		t.Error("too many validations taken")
	}
	if checkValidations(json.RawMessage(`{"ref":"A1"}`)) {
		t.Error("an object taken")
	}
}
