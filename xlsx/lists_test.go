package xlsx

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/ot"
)

func TestListsRead(t *testing.T) {
	for _, x := range []struct {
		file string
		want List
	}{
		{"libreoffice/data_validation_test.xlsx", List{Ref: "A1", Src: "Sheet2!$A$2:$A$4", Blank: true, Err: "stop"}},
		{"poi/dataValidationTableRange.xlsx", List{Ref: "B5", Src: "states", Blank: true, Err: "stop"}},
	} {
		data, err := os.ReadFile("../corpus/files/" + x.file)
		if err != nil {
			t.Skip("no corpus: corpus/fetch.sh")
		}
		_, tree, err := Open(data)
		if err != nil {
			t.Fatal(err)
		}
		var lists []List
		for _, n := range tree.Children("book") {
			var l []List
			_ = json.Unmarshal(n.Attrs[listsKey], &l)
			lists = append(lists, l...)
		}
		found := false
		for _, l := range lists {
			found = found || l == x.want
		}
		if !found {
			t.Errorf("%s: no %+v in %+v", x.file, x.want, lists)
		}
	}
}

func TestListsFollow(t *testing.T) {
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{
			"name":   json.RawMessage(`"Feuil1"`),
			listsKey: json.RawMessage(`[{"ref":"B2:B5 D3","src":"$A$1:$A$3"},{"ref":"C1","src":"Choix!$A$1:$A$2"},{"ref":"F4","src":"\"a,b\""}]`),
		}, Cells: []ot.Cell{}},
		{Op: ot.OpNew, ID: "S2", Type: "sheet", Parent: "book", Key: "L", Attrs: ot.Values{"name": json.RawMessage(`"Choix"`)}, Cells: []ot.Cell{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	lists := func() string { return string(tree.Node("S1").Attrs[listsKey]) }

	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 2}})
	want := `[{"ref":"B4:B7 D5","src":"$A$1:$A$5"},{"ref":"C1","src":"Choix!$A$1:$A$2"},{"ref":"F6","src":"\"a,b\""}]`
	if got := lists(); got != want {
		t.Fatalf("rows inserted: got %s\nwant %s", got, want)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S2", Dim: ot.DimRows, At: 1, N: 1}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpSet, ID: "S2", Attrs: ot.Values{"name": json.RawMessage(`"Valeurs"`)}}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimCols, At: 4, N: 3}})
	want = `[{"ref":"B4:B7","src":"$A$1:$A$5"},{"ref":"C1","src":"Valeurs!$A$2:$A$3"}]`
	if got := lists(); got != want {
		t.Fatalf("columns removed: got %s\nwant %s", got, want)
	}
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: "S1", Dim: ot.DimRows, At: 1, N: 10}})
	if got := lists(); got != "" {
		t.Fatalf("all removed: got %s", got)
	}
}
