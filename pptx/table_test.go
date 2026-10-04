package pptx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/trame/ot"
)

// tableOn is the first table of a slide.
func tableOn(tree *ot.Tree, slide string) *ot.Node {
	for _, n := range tree.Children(slide) {
		if n.Type == "frame" && str(n, "frame") == "table" {
			return n
		}
	}
	return nil
}

func cells(tree *ot.Tree, table *ot.Node) [][]*ot.Node {
	var out [][]*ot.Node
	for _, r := range tree.Children(table.ID) {
		out = append(out, tree.Children(r.ID))
	}
	return out
}

func TestTableEdits(t *testing.T) {
	d, tree := openCorpus(t, "python-pptx/tbl-cell.pptx")
	slide := slidesOf(tree)[0]
	table := tableOn(tree, slide.ID)
	if table == nil {
		t.Fatal("no table")
	}
	if string(table.Attrs["grid"]) != "[1524000,1524000,1524000,1524000]" || !strings.Contains(string(table.Attrs["tbl"]), `"style":"{D7AC3CCA-C797-4891-BE02-D94E43425B78}"`) {
		t.Fatalf("table %s %s", table.Attrs["grid"], table.Attrs["tbl"])
	}
	grid := cells(tree, table)
	if len(grid) != 4 || len(grid[0]) != 4 {
		t.Fatalf("%d rows", len(grid))
	}
	if text(grid[0][0]) != "having custom margins\n" || string(grid[0][0].Attrs["mar"]) != `{"b":457200,"l":182880,"r":365760,"t":274320}` || str(grid[0][2], "anchor") != "ctr" {
		t.Fatalf("first row %q %s %s", text(grid[0][0]), grid[0][0].Attrs["mar"], grid[0][2].Attrs["anchor"])
	}
	var styles map[string]TableStyle
	_ = json.Unmarshal(tree.Node("deck").Attrs["tblStyles"], &styles)
	if styles["{D7AC3CCA-C797-4891-BE02-D94E43425B78}"]["firstRow"].B != "on" || styles[mediumStyle2]["wholeTbl"].Fill == nil {
		t.Fatalf("styles %v", styles)
	}

	apply(t, d, tree, ot.Edit{
		{Op: ot.OpTxt, ID: grid[1][1].ID, Text: ot.Delta{{Insert: "Cellule"}}},
		{Op: ot.OpSet, ID: grid[1][2].ID, Attrs: ot.Values{"fill": json.RawMessage(`{"solid":{"scheme":"accent2"}}`), "lnB": json.RawMessage(`{"w":25400,"fill":{"solid":{"rgb":"FF0000"}}}`)}},
		{Op: ot.OpSet, ID: grid[2][0].ID, Attrs: ot.Values{"gridSpan": json.RawMessage(`2`)}},
		{Op: ot.OpSet, ID: grid[2][1].ID, Attrs: ot.Values{"hMerge": json.RawMessage(`true`)}},
	})
	// a row, and at the same time a column the row does not have
	last := tree.Children(table.ID)[3]
	edit := ot.Edit{
		{Op: ot.OpNew, ID: "row", Type: "tr", Parent: table.ID, Key: ot.KeyBetween(last.Key, ""), Attrs: ot.Values{"h": json.RawMessage(`370840`)}},
	}
	for i := range 4 {
		edit = append(edit, ot.Change{Op: ot.OpNew, ID: "row-" + string(rune('a'+i)), Type: "tc", Parent: "row", Key: ot.Keys(4)[i], Text: ot.Delta{{Insert: "Ligne\n"}}})
	}
	edit = append(edit, ot.Change{Op: ot.OpSet, ID: table.ID, Attrs: ot.Values{"grid": json.RawMessage(`[1524000,1524000,1524000,1524000,1000000]`)}})
	for _, r := range grid {
		edit = append(edit, ot.Change{Op: ot.OpNew, ID: r[0].Parent + "-new", Type: "tc", Parent: r[0].Parent, Key: ot.KeyBetween(r[3].Key, ""), Text: ot.Delta{{Insert: "Colonne\n"}}})
	}
	apply(t, d, tree, edit)
	apply(t, d, tree, ot.Edit{{Op: ot.OpDel, ID: grid[3][0].Parent}})

	// a table made by a client
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpNew, ID: "made", Type: "frame", Parent: slide.ID, Key: "zz", Attrs: ot.Values{
			"frame": json.RawMessage(`"table"`),
			"name":  json.RawMessage(`"Tableau 2"`),
			"xfrm":  json.RawMessage(`{"x":914400,"y":4572000,"w":3048000,"h":741680}`),
			"tbl":   json.RawMessage(`{"style":"` + mediumStyle2 + `","firstRow":true,"bandRow":true}`),
			"grid":  json.RawMessage(`[1524000,1524000]`),
		}},
		{Op: ot.OpNew, ID: "made-r1", Type: "tr", Parent: "made", Key: "V", Attrs: ot.Values{"h": json.RawMessage(`370840`)}},
		{Op: ot.OpNew, ID: "made-r1-c1", Type: "tc", Parent: "made-r1", Key: "V", Text: ot.Delta{{Insert: "Nom\n"}}},
		{Op: ot.OpNew, ID: "made-r1-c2", Type: "tc", Parent: "made-r1", Key: "k", Text: ot.Delta{{Insert: "Valeur\n"}}},
	})

	// a row across the cell merged over two rows and two columns
	merged := tree.Node("s256-2")
	mrows := tree.Children(merged.ID)
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpSet, ID: "s256-2-r1-c1", Attrs: ot.Values{"rowSpan": json.RawMessage(`3`)}},
		{Op: ot.OpSet, ID: "s256-2-r1-c2", Attrs: ot.Values{"rowSpan": json.RawMessage(`3`)}},
		{Op: ot.OpNew, ID: "across", Type: "tr", Parent: merged.ID, Key: ot.KeyBetween(mrows[0].Key, mrows[1].Key), Attrs: ot.Values{"h": json.RawMessage(`370840`)}},
		{Op: ot.OpNew, ID: "across-1", Type: "tc", Parent: "across", Key: "F", Attrs: ot.Values{"vMerge": json.RawMessage(`true`), "gridSpan": json.RawMessage(`2`)}, Text: ot.Delta{{Insert: "\n"}}},
		{Op: ot.OpNew, ID: "across-2", Type: "tc", Parent: "across", Key: "V", Attrs: ot.Values{"vMerge": json.RawMessage(`true`), "hMerge": json.RawMessage(`true`)}, Text: ot.Delta{{Insert: "\n"}}},
		{Op: ot.OpNew, ID: "across-3", Type: "tc", Parent: "across", Key: "k", Text: ot.Delta{{Insert: "Traversée\n"}}},
	})

	for _, e := range []ot.Edit{
		{{Op: ot.OpNew, ID: "x", Type: "tc", Parent: table.ID, Key: "V"}},
		{{Op: ot.OpNew, ID: "x", Type: "tr", Parent: slide.ID, Key: "V"}},
		{{Op: ot.OpSet, ID: grid[0][0].ID, Attrs: ot.Values{"xml": json.RawMessage(`"<a:tc/>"`)}}},
		{{Op: ot.OpTxt, ID: grid[0][0].Parent, Text: ot.Delta{{Insert: "x"}}}},
	} {
		if d.Check(tree, e) == nil {
			t.Errorf("%v allowed", e)
		}
	}

	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("LOFFICE_EDITED"); out != "" {
		if err := os.MkdirAll(out+"/python-pptx", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out+"/python-pptx/tbl-cell.pptx", saved, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	slide = slidesOf(again)[0]
	table = tableOn(again, slide.ID)
	if string(table.Attrs["grid"]) != "[1524000,1524000,1524000,1524000,1000000]" {
		t.Fatalf("grid %s", table.Attrs["grid"])
	}
	grid = cells(again, table)
	if len(grid) != 4 {
		t.Fatalf("%d rows", len(grid))
	}
	for i, r := range grid {
		if len(r) != 5 {
			t.Fatalf("row %d has %d cells", i, len(r))
		}
	}
	if text(grid[1][1]) != "Cellule\n" || text(grid[1][4]) != "Colonne\n" || text(grid[3][0]) != "Ligne\n" || text(grid[3][4]) != "\n" {
		t.Fatalf("texts %q %q %q %q", text(grid[1][1]), text(grid[1][4]), text(grid[3][0]), text(grid[3][4]))
	}
	if string(grid[1][2].Attrs["fill"]) != `{"solid":{"scheme":"accent2"}}` || !strings.Contains(string(grid[1][2].Attrs["lnB"]), "FF0000") {
		t.Fatalf("cell %s %s", grid[1][2].Attrs["fill"], grid[1][2].Attrs["lnB"])
	}
	if string(grid[2][0].Attrs["gridSpan"]) != "2" || string(grid[2][1].Attrs["hMerge"]) != "true" {
		t.Fatalf("spans %s %s", grid[2][0].Attrs["gridSpan"], grid[2][1].Attrs["hMerge"])
	}
	if m := cells(again, tableOn(again, "s256")); len(m) != 4 || string(m[0][0].Attrs["rowSpan"]) != "3" || string(m[1][0].Attrs["vMerge"]) != "true" || text(m[1][2]) != "Traversée\n" {
		t.Fatalf("merged table %v", m)
	}
	var made *ot.Node
	for _, n := range again.Children(slide.ID) {
		if str(n, "name") == "Tableau 2" {
			made = n
		}
	}
	if made == nil || len(cells(again, made)) != 1 || text(cells(again, made)[0][1]) != "Valeur\n" || !strings.Contains(string(made.Attrs["tbl"]), `"firstRow":true`) {
		t.Fatal("new table lost")
	}
}
