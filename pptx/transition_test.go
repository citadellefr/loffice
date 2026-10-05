package pptx

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/citadellefr/trame/ot"
)

// saveCorpus saves an edited presentation of the corpus and opens it
// again; with LOFFICE_EDITED, it is also written there for tools/validate.
func saveCorpus(t *testing.T, d *Document, tree *ot.Tree, name string) *ot.Tree {
	t.Helper()
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("LOFFICE_EDITED"); out != "" {
		path := filepath.Join(out, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, saved, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	return again
}

func TestTransitions(t *testing.T) {
	for name, want := range map[string]string{
		"libreoffice/tdf150770.pptx": `{"effect":"fade","dur":700}`,
		"libreoffice/tdf156808.pptx": `{"effect":"flythrough","dir":"out","dur":800}`,
		"libreoffice/tdf142915.pptx": `{"dur":10,"noClick":true,"after":1000}`,
	} {
		_, tree := openCorpus(t, name)
		if got := string(slidesOf(tree)[0].Attrs["transition"]); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}

	const name = "libreoffice/tdf150770.pptx"
	d, tree := openCorpus(t, name)
	slides := slidesOf(tree)
	set := []string{
		`{"effect":"push","dir":"u","dur":1000}`,
		`{"effect":"split","dir":"in","orient":"vert","dur":2000}`,
		``,
		`{"dur":500,"noClick":true,"after":3000}`,
	}
	var e ot.Edit
	for i, v := range set {
		var value json.RawMessage
		if v != "" {
			value = json.RawMessage(v)
		}
		e = append(e, ot.Change{Op: ot.OpSet, ID: slides[i].ID, Attrs: ot.Values{"transition": value}})
	}
	apply(t, d, tree, e)
	again := saveCorpus(t, d, tree, name)
	for i, v := range set {
		if got := string(slidesOf(again)[i].Attrs["transition"]); got != v {
			t.Errorf("slide %d: %s, want %s", i+1, got, v)
		}
	}

	// PowerPoint 2010's effect kept, its duration changed
	const fly = "libreoffice/tdf156808.pptx"
	d, tree = openCorpus(t, fly)
	slide := slidesOf(tree)[0]
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"transition": json.RawMessage(`{"effect":"flythrough","dir":"out","dur":1200}`)}}})
	again = saveCorpus(t, d, tree, fly)
	if got := string(slidesOf(again)[0].Attrs["transition"]); got != `{"effect":"flythrough","dir":"out","dur":1200}` {
		t.Errorf("flythrough: %s", got)
	}
}
