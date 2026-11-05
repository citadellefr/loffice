package agent

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
)

func apply(t *testing.T, a *Agent, user string, edits ...Edit) {
	t.Helper()
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: user, Edits: edits}); err != nil {
		t.Fatal(err)
	}
}

// behalfOfRevisions lists the revisions of the agent as "user kind text".
func behalfOfRevisions(t *testing.T, a *Agent, block string) string {
	t.Helper()
	_, revs, err := a.Revisions()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range revs {
		if r.Author == a.peer.Name && r.Block == block {
			out = append(out, r.OnBehalfOf+" "+r.Kind+" "+strings.TrimSpace(r.Text))
		}
	}
	return strings.Join(out, ", ")
}

func TestDeleteWhatTheAgentInsertedForAnotherUser(t *testing.T) {
	a := newRoom(t, loffice.Options{}).join("Héraut", Options{})
	apply(t, a, "u2", Edit{Op: "insert", Block: "p5", Text: "Brouillon de u2, à relire."})
	apply(t, a, "u1", Edit{Op: "insert", Block: "p5", Text: "Brouillon de u1, à relire."})
	_, matches, _ := a.Search("Brouillon de u2", false)
	_, others, _ := a.Search("Brouillon de u1", false)
	u2, u1 := matches[0].Block, others[0].Block

	apply(t, a, "u1", Edit{Op: "replace", Block: u2, Find: "Brouillon", Text: "Texte"}, Edit{Op: "replace", Block: u1, Find: "Brouillon", Text: "Texte"})
	if got := behalfOfRevisions(t, a, u2); got != "u2 ins Brouillon, u1 del Brouillon, u1 ins Texte, u2 ins de u2, à relire." {
		t.Errorf("what u2 had inserted, replaced for u1: %s", got)
	}
	if got := behalfOfRevisions(t, a, u1); got != "u1 ins Texte, u1 ins de u1, à relire." {
		t.Errorf("what u1 had inserted, replaced for u1: %s", got)
	}
	if text := read(t, a, Markdown); strings.Contains(text, "Brouillon de u1") {
		t.Errorf("u1's own text is still there:\n%s", text)
	}
}

func TestBehalfSurvivesSessions(t *testing.T) {
	r := newRoom(t, loffice.Options{SaveDelay: 10 * time.Millisecond, SaveMaxDelay: 50 * time.Millisecond})
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	opt := Options{Now: func() time.Time { return now }}
	first := r.join("Héraut", opt)
	second, err := Join(t.Context(), r.hub, key, trame.Peer{ID: "heraut2", Name: "Héraut"}, opt)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Leave()

	apply(t, first, "u1", Edit{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"})
	settle(t, second, first.s.version)
	apply(t, second, "u2", Edit{Op: "replace", Block: "p2", Find: "sous budget", Text: "dans le budget"})
	want := "u1 del en avance, u1 ins à l'heure, u2 del sous budget, u2 ins dans le budget"
	if got := behalfOfRevisions(t, second, "p2"); got != want {
		t.Fatalf("revisions of both users: %s", got)
	}
	if first.Journal()[0].Date.Equal(second.Journal()[0].Date) {
		t.Fatalf("both batches are dated %s", first.Journal()[0].Date)
	}

	var data []byte
	for range 200 {
		data = []byte(r.store.File(key))
		if settings := part(t, data, "word/settings.xml"); strings.Count(settings, "loffice.behalf.") == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if settings := part(t, data, "word/settings.xml"); !strings.Contains(settings, "<w:docVars>") || strings.Count(settings, "loffice.behalf.") != 2 {
		t.Fatalf("settings.xml\n%s", settings)
	}

	reopened := newRoom(t, loffice.Options{})
	reopened.store.Data[key] = data
	third := reopened.join("Héraut", Options{})
	if got := behalfOfRevisions(t, third, "p2"); got != want {
		t.Fatalf("revisions after reopening, without the journal: %s", got)
	}
	apply(t, third, "u1", Edit{Op: "replace", Block: "p2", Find: "dans le budget", Text: "sous budget"})
	if rec, err := third.UndoMine(t.Context(), "u1"); err != nil || len(rec.Edits) != 4 {
		t.Fatalf("undo for u1: %+v %v", rec, err)
	}
	if got := behalfOfRevisions(t, third, "p2"); got != "u2 del sous budget, u2 ins dans le budget" {
		t.Fatalf("after undoing u1: %s", got)
	}
}

// part is a part of a package, empty when it has none.
func part(t *testing.T, data []byte, name string) string {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := z.Open(name)
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	return string(b)
}

func TestBehalfNodesAreChecked(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	a := r.join("Héraut", Options{})
	heraut, alice := r.human("Héraut"), r.human("Alice")
	apply(t, a, "u1", Edit{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"})
	date := a.Journal()[0].Date.Format(time.RFC3339)

	node := func(author, date, user string) ot.Edit {
		id, attrs, ok := docx.BehalfNode(author, date, user)
		if !ok {
			t.Fatalf("no node for %s %s %s", author, date, user)
		}
		return ot.Edit{{Op: ot.OpNew, ID: id, Type: docx.BehalfType, Parent: "doc", Key: "~", Attrs: attrs}}
	}
	if f := heraut.edit(node("Héraut", date, "u2")); f.T != "nack" {
		t.Errorf("a second user for the same author and date: %+v", f)
	}
	if f := alice.edit(node("Héraut", "2026-11-03T10:00:00Z", "u2")); f.T != "nack" {
		t.Errorf("a node signed by another author: %+v", f)
	}
	if f := heraut.edit(node("Héraut", "2026-11-03T10:00:00Z", "u2")); f.T != "ack" {
		t.Errorf("a node of a new date: %+v", f)
	}
	id, _, _ := docx.BehalfNode("Héraut", date, "u1")
	if f := alice.edit(ot.Edit{{Op: ot.OpDel, ID: id}}); f.T != "nack" {
		t.Errorf("a node deleted: %+v", f)
	}
	if f := alice.edit(ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{"user": []byte(`"u3"`)}}}); f.T != "nack" {
		t.Errorf("a node changed: %+v", f)
	}
}

// rewritten is a Word document saved by the agent, a part of which is
// changed by f, as another program saving it would.
func rewritten(t *testing.T, data []byte, name string, f func(string) string) []byte {
	t.Helper()
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, file := range z.File {
		r, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r)
		r.Close()
		if file.Name == name {
			b = []byte(f(string(b)))
		}
		o, err := w.Create(file.Name)
		if err != nil {
			t.Fatal(err)
		}
		o.Write(b)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// savedAfterInsert is the document the agent inserted a paragraph in for
// u1 at 09:00:00, as the store has it.
func savedAfterInsert(t *testing.T) []byte {
	t.Helper()
	r := newRoom(t, loffice.Options{SaveDelay: 10 * time.Millisecond, SaveMaxDelay: 50 * time.Millisecond})
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	a := r.join("Héraut", Options{Now: func() time.Time { return now }})
	apply(t, a, "u1", Edit{Op: "insert", Block: "p5", Text: "Brouillon à relire."})
	for range 200 {
		data := []byte(r.store.File(key))
		if strings.Contains(part(t, data, "word/settings.xml"), "loffice.behalf.") {
			return data
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the document is not saved")
	return nil
}

func TestDeleteWhatTheAgentInsertedWithoutBehalf(t *testing.T) {
	saved := savedAfterInsert(t)
	for name, data := range map[string][]byte{
		"no node": rewritten(t, saved, "word/settings.xml", func(s string) string {
			return strings.ReplaceAll(s, "loffice.behalf.", "other.tool.")
		}),
		"date rewritten": rewritten(t, saved, "word/document.xml", func(s string) string {
			return strings.ReplaceAll(s, "2026-11-03T09:00:00Z", "2026-11-03T09:00:01Z")
		}),
	} {
		r := newRoom(t, loffice.Options{})
		r.store.Data[key] = data
		a := r.join("Héraut", Options{})
		_, matches, _ := a.Search("Brouillon", false)
		block := matches[0].Block
		if got := behalfOfRevisions(t, a, block); got != " ins Brouillon à relire." {
			t.Fatalf("%s: revisions of the document: %s", name, got)
		}
		apply(t, a, "u1", Edit{Op: "replace", Block: block, Find: "Brouillon", Text: "Texte"})
		if got := behalfOfRevisions(t, a, block); got != " ins Brouillon, u1 del Brouillon, u1 ins Texte,  ins à relire." {
			t.Errorf("%s: after u1 replaced it: %s", name, got)
		}
	}
}
