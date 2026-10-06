package pptx

import (
	"encoding/json"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func commentAdded(id, slide, author, parent, text string) ot.Change {
	attrs := ot.Values{
		"author":   json.RawMessage(`"` + author + `"`),
		"initials": json.RawMessage(`"` + author[:1] + `"`),
		"date":     json.RawMessage(`"2026-10-05T10:30:00Z"`),
		"x":        json.RawMessage(`10`),
		"y":        json.RawMessage(`20`),
	}
	if parent != "" {
		attrs["parent"] = json.RawMessage(`"` + parent + `"`)
	}
	return ot.Change{Op: ot.OpNew, ID: id, Type: "comment", Parent: slide, Key: "zzz" + id, Attrs: attrs, Text: ot.Delta{{Insert: text + "\n"}}}
}

// applyBy applies an edit a peer makes.
func applyBy(t *testing.T, d *Document, tree *ot.Tree, e ot.Edit, author string) {
	t.Helper()
	if err := d.Check(tree, e, author); err != nil {
		t.Fatalf("%v refused: %v", e, err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
}

func commentsOf(tree *ot.Tree, slide string) map[string]*ot.Node {
	out := map[string]*ot.Node{}
	for _, n := range commentNodes(tree, slide) {
		out[n.ID] = n
	}
	return out
}

func TestComments(t *testing.T) {
	d, tree := openCorpus(t, "poi/aptia.pptx")
	slide := slidesOf(tree)[0].ID

	applyBy(t, d, tree, ot.Edit{commentAdded("a", slide, "Alice", "", "Première\nligne")}, "Alice")
	applyBy(t, d, tree, ot.Edit{commentAdded("b", slide, "Bob", "a", "Réponse <b> & co")}, "Bob")
	again := saveCorpus(t, d, tree, "comments/two.pptx")
	got := commentsOf(again, slide)
	if len(got) != 2 {
		t.Fatalf("%d comments after a save", len(got))
	}
	var first, reply *ot.Node
	for _, n := range got {
		if str(n, "author") == "Alice" {
			first = n
		} else {
			reply = n
		}
	}
	if first == nil || reply == nil {
		t.Fatalf("authors lost: %v", got)
	}
	if commentText(first) != "Première\nligne" || commentText(reply) != "Réponse <b> & co" {
		t.Errorf("texts %q %q", commentText(first), commentText(reply))
	}
	if str(reply, "parent") != first.ID {
		t.Errorf("reply answers %q, not %q", str(reply, "parent"), first.ID)
	}
	if str(first, "date") != "2026-10-05T10:30:00Z" || string(first.Attrs["x"]) != "10" || string(first.Attrs["y"]) != "20" {
		t.Errorf("attributes %v", first.Attrs)
	}

	// another round on what was read: a comment added, one edited, one deleted
	d2, tree2 := reopen(t, d, tree)
	applyBy(t, d2, tree2, ot.Edit{commentAdded("c", slide, "Alice", "", "Troisième")}, "Alice")
	apply(t, d2, tree2, ot.Edit{{Op: ot.OpDel, ID: reply.ID}})
	apply(t, d2, tree2, ot.Edit{{Op: ot.OpTxt, ID: first.ID, Text: ot.Delta{{Insert: "Modifiée "}}}})
	got = commentsOf(saveCorpus(t, d2, tree2, "comments/three.pptx"), slide)
	if len(got) != 2 {
		t.Fatalf("%d comments after a second save", len(got))
	}
	var texts []string
	for _, n := range got {
		texts = append(texts, commentText(n))
	}
	if !(contains(texts, "Modifiée Première\nligne") && contains(texts, "Troisième")) {
		t.Errorf("texts %q", texts)
	}

	// all of them deleted, the part goes
	d3, tree3 := reopen(t, d2, tree2)
	var del ot.Edit
	for id := range got {
		del = append(del, ot.Change{Op: ot.OpDel, ID: id})
	}
	apply(t, d3, tree3, del)
	saved, err := d3.Save(tree3)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(saved, Limits)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Has("ppt/comments/comment1.xml") {
		t.Error("the comments part of a slide without comments was kept")
	}
}

func reopen(t *testing.T, d *Document, tree *ot.Tree) (*Document, *ot.Tree) {
	t.Helper()
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	d, tree, err = Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	return d, tree
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func TestCommentsRefused(t *testing.T) {
	d, tree := openCorpus(t, "poi/aptia.pptx")
	slide := slidesOf(tree)[0].ID
	forged := commentAdded("a", slide, "Mallory", "", "x")
	if d.Check(tree, ot.Edit{forged}, "Alice") == nil {
		t.Error("a comment signed by another was accepted")
	}
	undated := commentAdded("a", slide, "Alice", "", "x")
	undated.Attrs["date"] = json.RawMessage(`"hier"`)
	if d.Check(tree, ot.Edit{undated}, "Alice") == nil {
		t.Error("an undated comment was accepted")
	}
	styled := commentAdded("a", slide, "Alice", "", "x")
	styled.Text = ot.Delta{{Insert: "x\n", Attrs: ot.Attrs{"b": "1"}}}
	if d.Check(tree, ot.Edit{styled}, "Alice") == nil {
		t.Error("a formatted comment was accepted")
	}
	orphan := commentAdded("a", slide, "Alice", "nowhere", "x")
	if d.Check(tree, ot.Edit{orphan}, "Alice") == nil {
		t.Error("an answer to nothing was accepted")
	}
}

func TestEmptyCommentsKept(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/tdf173266.pptx")
	data, err := save(d, tree, true)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := opc.Open(data, Limits)
	if err != nil {
		t.Fatal(err)
	}
	if !pkg.Has("ppt/comments/comment1.xml") {
		t.Error("the comments part of a slide without a comment is lost")
	}
}
