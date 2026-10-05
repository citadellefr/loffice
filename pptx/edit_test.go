package pptx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

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

func slidesOf(tree *ot.Tree) []*ot.Node {
	var out []*ot.Node
	for _, n := range tree.Children("deck") {
		if n.Type == "slide" {
			out = append(out, n)
		}
	}
	return out
}

func apply(t *testing.T, d *Document, tree *ot.Tree, e ot.Edit) {
	t.Helper()
	if err := d.Check(tree, e, "Test"); err != nil {
		t.Fatalf("%v refused: %v", e, err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatalf("%v: %v", e, err)
	}
}

func text(n *ot.Node) string {
	var b strings.Builder
	for _, o := range n.Text.Delta() {
		b.WriteString(o.Insert)
	}
	return b.String()
}

// firstText is the first shape of a slide with text.
func firstText(tree *ot.Tree, slide string) *ot.Node {
	for _, n := range tree.Children(slide) {
		if n.Type == "sp" && n.Text.Len() > 1 {
			return n
		}
	}
	return nil
}

func TestEdits(t *testing.T) {
	d, tree := openCorpus(t, "poi/aptia.pptx")
	slides := slidesOf(tree)
	if len(slides) != 9 {
		t.Fatalf("%d slides", len(slides))
	}

	title := firstText(tree, slides[0].ID)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: title.ID, Text: ot.Delta{{Insert: "Bref: "}}}})
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: title.ID, Attrs: ot.Values{"xfrm": json.RawMessage(`{"x":100,"y":200,"w":3000000,"h":1000000}`)}}})

	var withNotes, withPicture *ot.Node
	for _, s := range slides {
		for _, c := range tree.Children(s.ID) {
			if c.Type == "notes" && withNotes == nil {
				withNotes = s
			}
			if c.Type == "pic" && withPicture == nil {
				withPicture = s
			}
		}
	}
	if withNotes == nil || withPicture == nil {
		t.Fatal("no slide with notes or a picture")
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: withNotes.ID + "-notes", Text: ot.Delta{{Insert: "Notes de Bref.\n"}}}})

	// a copy of the slide with a picture, first in the deck, and a new shape
	copyEdit := ot.Edit{{Op: ot.OpNew, ID: "copy", Type: "slide", Parent: "deck", Key: ot.KeyBetween("", slides[0].Key), Attrs: withPicture.Attrs}}
	for i, c := range tree.Children(withPicture.ID) {
		if c.Type == "notes" {
			continue
		}
		change := ot.Change{Op: ot.OpNew, ID: "copy-" + string(rune('a'+i)), Type: c.Type, Parent: "copy", Key: c.Key, Attrs: c.Attrs}
		if c.Text != nil {
			change.Text = c.Text.Delta()
		}
		copyEdit = append(copyEdit, change)
	}
	copyEdit = append(copyEdit, ot.Change{Op: ot.OpNew, ID: "star", Type: "sp", Parent: "copy", Key: "zz", Attrs: ot.Values{
		"name": json.RawMessage(`"Étoile"`),
		"xfrm": json.RawMessage(`{"x":914400,"y":914400,"w":1828800,"h":1828800}`),
		"geom": json.RawMessage(`{"prst":"star5"}`),
		"fill": json.RawMessage(`{"solid":{"scheme":"accent2"}}`),
	}, Text: ot.Delta{{Insert: "Nouveau", Attrs: ot.Attrs{"b": "1", "sz": "2400"}}, {Insert: "\n", Attrs: ot.Attrs{"algn": "ctr"}}}})
	apply(t, d, tree, copyEdit)

	// a new slide from a layout, with its placeholders, and a shape with the
	// theme's style
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpNew, ID: "fresh", Type: "slide", Parent: "deck", Key: ot.KeyBetween(slides[8].Key, ""), Attrs: ot.Values{"layout": json.RawMessage(`"L2"`)}},
		{Op: ot.OpNew, ID: "fresh-title", Type: "sp", Parent: "fresh", Key: "V", Attrs: ot.Values{"ph": json.RawMessage(`{"type":"title"}`), "name": json.RawMessage(`"Titre 1"`)}, Text: ot.Delta{{Insert: "Titre neuf\n"}}},
		{Op: ot.OpNew, ID: "fresh-body", Type: "sp", Parent: "fresh", Key: "k", Attrs: ot.Values{"ph": json.RawMessage(`{"idx":"1"}`), "name": json.RawMessage(`"Espace réservé du contenu 2"`)}, Text: ot.Delta{{Insert: "Un\nDeux\n"}}},
		{Op: ot.OpNew, ID: "fresh-shape", Type: "sp", Parent: "fresh", Key: "z", Attrs: ot.Values{
			"xfrm":  json.RawMessage(`{"x":914400,"y":914400,"w":914400,"h":914400}`),
			"geom":  json.RawMessage(`{"prst":"ellipse"}`),
			"style": json.RawMessage(`{"ln":{"idx":"2","color":{"scheme":"accent1","mods":[["shade",50000]]}},"fill":{"idx":"1","color":{"scheme":"accent1"}},"effect":{"idx":"0","color":{"scheme":"accent1"}},"font":{"idx":"minor","color":{"scheme":"lt1"}}}`),
		}, Text: ot.Delta{{Insert: "\n"}}},
	})

	// the last slide deleted, the second moved to the end
	apply(t, d, tree, ot.Edit{{Op: ot.OpDel, ID: slides[8].ID}})
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: slides[1].ID, Key: ot.KeyBetween(slides[7].Key, "")}})

	for _, e := range []ot.Edit{
		{{Op: ot.OpSet, ID: "L1", Attrs: ot.Values{"name": json.RawMessage(`"x"`)}}},
		{{Op: ot.OpDel, ID: "M1"}},
		{{Op: ot.OpNew, ID: "x", Type: "sp", Parent: "L1", Key: "V"}},
		{{Op: ot.OpSet, ID: title.ID, Attrs: ot.Values{"xml": json.RawMessage(`"<p:sp/>"`)}}},
		{{Op: ot.OpTxt, ID: slides[0].ID, Text: ot.Delta{{Insert: "x"}}}},
	} {
		if d.Check(tree, e, "Test") == nil {
			t.Errorf("%v allowed", e)
		}
	}

	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("LOFFICE_EDITED"); out != "" {
		if err := os.MkdirAll(out+"/poi", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out+"/poi/aptia.pptx", saved, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d2, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	got := slidesOf(again)
	if len(got) != 10 {
		t.Fatalf("%d slides after saving", len(got))
	}
	fresh := got[9]
	if str(fresh, "layout") != "L2" {
		t.Fatalf("new slide on layout %q", str(fresh, "layout"))
	}
	var titled, styled bool
	for _, c := range again.Children(fresh.ID) {
		if string(c.Attrs["ph"]) == `{"type":"title"}` && text(c) == "Titre neuf\n" {
			titled = true
		}
		if c.Attrs["style"] != nil && string(c.Attrs["geom"]) == `{"prst":"ellipse"}` {
			styled = true
		}
	}
	if !titled || !styled {
		t.Fatalf("new slide shapes: %v", again.Children(fresh.ID))
	}
	got = got[:9]
	want := []string{"copy", slides[0].ID}
	for _, s := range slides[2:8] {
		want = append(want, s.ID)
	}
	want = append(want, slides[1].ID)
	// the copy got a slide id of its own
	if got[0].ID == "copy" || !strings.HasPrefix(got[0].ID, "s") {
		t.Fatalf("copy read back as %s", got[0].ID)
	}
	for i, s := range got[1:] {
		if s.ID != want[i+1] {
			t.Fatalf("slide %d is %s, want %s", i+1, s.ID, want[i+1])
		}
	}

	if tt := firstText(again, slides[0].ID); !strings.HasPrefix(text(tt), "Bref: ") || string(tt.Attrs["xfrm"]) != `{"x":100,"y":200,"w":3000000,"h":1000000}` {
		t.Fatalf("title %q at %s", text(tt), tt.Attrs["xfrm"])
	}
	if n := again.Node(withNotes.ID + "-notes"); n == nil || !strings.HasPrefix(text(n), "Notes de Bref.") {
		t.Fatal("notes lost")
	}

	var copied []*ot.Node
	copied = again.Children(got[0].ID)
	if len(copied) != len(tree.Children("copy")) {
		t.Fatalf("copy has %d shapes, want %d", len(copied), len(tree.Children("copy")))
	}
	var picture, star *ot.Node
	for _, c := range copied {
		switch {
		case c.Type == "pic":
			picture = c
		case str(c, "name") == "Étoile":
			star = c
		}
	}
	if picture == nil || star == nil {
		t.Fatal("picture or new shape missing from the copy")
	}
	var blip struct{ Media string }
	_ = json.Unmarshal(picture.Attrs["blip"], &blip)
	if _, _, err := d2.Media(blip.Media); err != nil {
		t.Fatalf("picture of the copy: %v", err)
	}
	if text(star) != "Nouveau\n" || string(star.Attrs["geom"]) != `{"prst":"star5"}` {
		t.Fatalf("new shape %q %s", text(star), star.Attrs["geom"])
	}
	if flow := star.Text.Delta(); flow[0].Attrs["b"] != "1" || flow[1].Attrs["algn"] != "ctr" {
		t.Fatalf("new shape formatting %v", flow)
	}

	pkg, _ := opc.Open(saved, Limits)
	if pkg.Has(d.slides[slides[8].ID].name) {
		t.Fatal("the deleted slide's part is still there")
	}
	rels, _ := pkg.Relationships(d.presName)
	for _, r := range rels {
		if name, err := opc.Resolve(d.presName, r.Target); err == nil && !r.External && !pkg.Has(name) {
			t.Fatalf("the presentation points to a missing %s", name)
		}
	}
}

// BenchmarkOpen reads the largest presentations of the corpus: in bytes,
// and in slides.
func BenchmarkOpen(b *testing.B) {
	for _, name := range []string{"KEY02.pptx", "2411-Performance_Up.pptx"} {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile("../corpus/files/poi/" + name)
			if err != nil {
				b.Skip("no corpus: corpus/fetch.sh")
			}
			for b.Loop() {
				if _, _, err := Open(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
