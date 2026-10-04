package xlsx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func openCorpus(t *testing.T, name string) (*Document, *ot.Tree) {
	t.Helper()
	data, err := os.ReadFile("../corpus/files/" + name)
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	return d, tree
}

func firstSheet(tree *ot.Tree) *ot.Node {
	for _, n := range tree.Children("book") {
		if n.Type == "sheet" {
			return n
		}
	}
	return nil
}

func TestFilterRead(t *testing.T) {
	for name, want := range map[string]string{
		"libreoffice/tdf99913.xlsx": `{"ref":"A1:A4","cols":[{"col":0,"vals":["1","3"]}]}`,
		"libreoffice/database.xlsx": `{"ref":"A1:D6","cols":[{"col":1,"kept":1}]}`,
	} {
		_, tree := openCorpus(t, name)
		if got := string(firstSheet(tree).Attrs[filterKey]); got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}

// A filter of another kind stays where its column goes, beside the
// columns the editor filters.
func TestFilterKept(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/database.xlsx")
	sheet := firstSheet(tree).ID
	c := NewCalc(tree, formula.Options{})
	e := ot.Edit{{Op: ot.OpIns, ID: sheet, Dim: ot.DimCols, At: 2, N: 1}}
	if err := moveRows(t, d, tree, e); err != nil {
		t.Fatal(err)
	}
	c.Follow(e, nil)
	if got := string(tree.Node(sheet).Attrs[filterKey]); got != `{"ref":"A1:E6","cols":[{"col":2,"kept":1}]}` {
		t.Fatalf("moved: %s", got)
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: sheet, Attrs: ot.Values{
		filterKey: json.RawMessage(`{"ref":"A1:E6","cols":[{"col":0,"vals":["a"]},{"col":2,"kept":1}]}`),
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _ := opc.Open(saved, Limits)
	xml, _ := pkg.Read(d.sheets[sheet].name)
	for _, want := range []string{`<autoFilter ref="A1:E6">`, `<filterColumn colId="0"><filters><filter val="a"/></filters></filterColumn>`,
		`<filterColumn colId="2"><customFilters`} {
		if !strings.Contains(string(xml), want) {
			t.Errorf("no %s in %s", want, xml)
		}
	}

	// cleared
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: sheet, Attrs: ot.Values{filterKey: json.RawMessage(`null`)}}})
	saved, _ = d.Save(tree)
	pkg, _ = opc.Open(saved, Limits)
	xml, _ = pkg.Read(d.sheets[sheet].name)
	if strings.Contains(string(xml), "autoFilter") {
		t.Errorf("filter left in %s", xml)
	}
}

func TestFilterChecked(t *testing.T) {
	for _, bad := range []string{`{"ref":"A:A"}`, `{"ref":"A1:B5","cols":[{"col":2}]}`, `{"ref":"A1:B5","cols":[{"col":0},{"col":0}]}`, `{"ref":"nope"}`} {
		if checkFilter(json.RawMessage(bad)) {
			t.Errorf("%s taken", bad)
		}
	}
}

// A filter set while rows were inserted above it moves with them.
func TestFilterRebased(t *testing.T) {
	tree, c := calcTree(t)
	ins := ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 1, N: 2}}
	edit(t, tree, c, ins)
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S1", Attrs: ot.Values{filterKey: json.RawMessage(`{"ref":"A1:B5"}`)}}}, ins)
	if got := string(tree.Node("S1").Attrs[filterKey]); got != `{"ref":"A3:B7"}` {
		t.Errorf("got %s", got)
	}
}
