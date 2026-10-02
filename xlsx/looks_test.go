package xlsx

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/ot"
)

func looksTree(t *testing.T) (*ot.Tree, *Calc) {
	t.Helper()
	rules := `[
		{"ref":"A1:A5","type":"formula","pri":1,"f":["A1>3"],"style":{"fill":{"pattern":"solid","fg":{"rgb":"FFC7CE"}}}},
		{"ref":"A1:A5","type":"colorScale","pri":2,"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"},{"rgb":"63BE7B"}]},
		{"ref":"B1:B3","type":"duplicateValues","pri":3,"style":{"font":{"b":true}}},
		{"ref":"A1:A5","type":"iconSet","pri":4,"icons":"3Arrows","stops":[{"t":"percent","v":"0"},{"t":"percent","v":"33"},{"t":"percent","v":"67"}]},
		{"ref":"A1:A5","type":"top10","pri":5,"rank":1,"style":{"font":{"i":true}}}
	]`
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{
			"name": json.RawMessage(`"Feuil1"`), conditionalKey: json.RawMessage(rules),
		}, Cells: []ot.Cell{
			cell(1, 1, `{"v":1}`), cell(2, 1, `{"v":2}`), cell(3, 1, `{"v":3}`), cell(4, 1, `{"v":4}`), cell(5, 1, `{"v":5}`),
			cell(1, 2, `{"v":"x"}`), cell(2, 2, `{"v":"X"}`), cell(3, 2, `{"v":"y"}`),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree, NewCalc(tree, formula.Options{})
}

func lookAt(tree *ot.Tree, row, col int) string {
	return string(tree.Node(looksID("S1")).Grid.Cell(row, col))
}

func TestLooks(t *testing.T) {
	tree, c := looksTree(t)
	for _, x := range []struct {
		row, col int
		want     string
	}{
		{1, 1, `{"i":[3,0],"s":[1,0]}`},
		{3, 1, `{"i":[3,1],"s":[1,0.5]}`},
		{4, 1, `{"i":[3,2],"r":[0],"s":[1,0.75]}`},
		{5, 1, `{"i":[3,2],"r":[0,4],"s":[1,1]}`},
		{1, 2, `{"r":[2]}`},
		{2, 2, `{"r":[2]}`},
		{3, 2, ``},
	} {
		if got := lookAt(tree, x.row, x.col); got != x.want {
			t.Errorf("%s: got %s, want %s", formula.CellName(x.row, x.col), got, x.want)
		}
	}

	out := edit(t, tree, c, ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(1, 1, `{"v":10}`), cell(2, 2, `{"v":"z"}`)}}})
	if !strings.Contains(out, `"id":"S1.cf"`) {
		t.Fatalf("no looks in %s", out)
	}
	for _, x := range []struct {
		row, col int
		want     string
	}{
		{1, 1, `{"i":[3,2],"r":[0,4],"s":[1,1]}`},
		{5, 1, `{"i":[3,1],"r":[0],"s":[1,0.375]}`},
		{1, 2, ``},
	} {
		if got := lookAt(tree, x.row, x.col); got != x.want {
			t.Errorf("edited, %s: got %s, want %s", formula.CellName(x.row, x.col), got, x.want)
		}
	}

	// the looks and the rules move with the rows
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 1, N: 1}})
	if got := lookAt(tree, 2, 1); got != `{"i":[3,2],"r":[0,4],"s":[1,1]}` {
		t.Errorf("moved: got %s", got)
	}
	if got := lookAt(tree, 1, 1); got != `` {
		t.Errorf("row inserted: got %s", got)
	}
	var rules []Conditional
	_ = json.Unmarshal(tree.Node("S1").Attrs[conditionalKey], &rules)
	if rules[0].Ref != "A2:A6" || rules[0].F[0] != "A2>3" {
		t.Errorf("rule moved: %+v", rules[0])
	}
}

func TestLooksStop(t *testing.T) {
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{
			"name": json.RawMessage(`"Feuil1"`), conditionalKey: json.RawMessage(`[
				{"ref":"A1:A2","type":"formula","pri":1,"stop":true,"f":["A1=1"],"style":{"font":{"b":true}}},
				{"ref":"A1:A2","type":"formula","pri":2,"f":["TRUE"],"style":{"font":{"i":true}}}
			]`),
		}, Cells: []ot.Cell{cell(1, 1, `{"v":1}`), cell(2, 1, `{"v":2}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	NewCalc(tree, formula.Options{})
	if a1, a2 := lookAt(tree, 1, 1), lookAt(tree, 2, 1); a1 != `{"r":[0]}` || a2 != `{"r":[1]}` {
		t.Errorf("got %s and %s", a1, a2)
	}
}

// The conditional formats of the corpus are read and calculated.
func TestCorpusLooks(t *testing.T) {
	rules, looks := 0, 0
	for _, name := range workbooks(t) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		_, tree, err := Open(data)
		if err != nil {
			continue
		}
		NewCalc(tree, formula.Options{})
		for _, n := range tree.Children("book") {
			var r []Conditional
			_ = json.Unmarshal(n.Attrs[conditionalKey], &r)
			rules += len(r)
			if l := tree.Node(looksID(n.ID)); l != nil {
				looks += l.Grid.Len()
			}
		}
	}
	t.Logf("%d conditional formats, %d cells with a look", rules, looks)
	if rules == 0 || looks == 0 {
		t.Fatal("no conditional formats calculated")
	}
}

// BenchmarkFollowLooks follows an edit of a sheet of 10 000 numbers under
// a formula rule and a color scale.
func BenchmarkFollowLooks(b *testing.B) {
	var cells []ot.Cell
	for i := 1; i <= 10000; i++ {
		cells = append(cells, cell(i, 1, `{"v":`+strconv.Itoa(i)+`}`))
	}
	tree, err := ot.NewTree(ot.Edit{
		{Op: ot.OpNew, ID: "book", Type: "book", Key: "V"},
		{Op: ot.OpNew, ID: "S1", Type: "sheet", Parent: "book", Key: "K", Attrs: ot.Values{
			"name": json.RawMessage(`"Feuil1"`), conditionalKey: json.RawMessage(`[
				{"ref":"A1:A10000","type":"formula","pri":1,"f":["A1>$B$1"],"style":{"font":{"b":true}}},
				{"ref":"A1:A10000","type":"colorScale","pri":2,"stops":[{"t":"min"},{"t":"max"}],"colors":[{"rgb":"F8696B"},{"rgb":"63BE7B"}]}
			]`),
		}, Cells: cells},
	})
	if err != nil {
		b.Fatal(err)
	}
	c := NewCalc(tree, formula.Options{})
	i := 0
	for b.Loop() {
		i++
		e := ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(5000, 1, `{"v":`+strconv.Itoa(5000+4000*(i%2))+`}`)}}}
		if err := tree.Apply(e); err != nil {
			b.Fatal(err)
		}
		if out := c.Follow(e, nil); len(out) != 1 {
			b.Fatalf("got %d changes", len(out))
		}
	}
}
