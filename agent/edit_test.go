package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
)

// gate runs before once the hub reads the agent's first edit, and before it
// handles it.
type gate struct {
	hub    Server
	before func()
}

func (g *gate) Serve(ctx context.Context, conn trame.Conn, key string, info trame.Peer) error {
	return g.hub.Serve(ctx, &gatedConn{Conn: conn, before: g.before}, key, info)
}

type gatedConn struct {
	trame.Conn
	once   sync.Once
	before func()
}

func (c *gatedConn) ReadMessage() (int, []byte, error) {
	kind, data, err := c.Conn.ReadMessage()
	if err == nil && bytes.HasPrefix(data, []byte(`{"t":"op"`)) {
		c.once.Do(c.before)
	}
	return kind, data, err
}

func TestConcurrentEdit(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	var a *Agent
	g := &gate{hub: r.hub, before: func() {
		if f := alice.edit(typeAt(a, "p2", "Note : ", nil)); f.T != "ack" {
			t.Errorf("Alice's edit: %+v", f)
		}
	}}
	ctx := t.Context()
	a, err := Join(ctx, g, key, trame.Peer{ID: "heraut", Name: "Héraut"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Leave()
	if _, err := a.Apply(ctx, Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"}}}); err != nil {
		t.Fatal(err)
	}
	if f := alice.next("op"); !strings.Contains(string(f.D), "Héraut") {
		t.Fatalf("Alice got %s", f.D)
	}
	want := "[p2] Note : Le projet est **à l'heure** et *sous budget*."
	if got := read(t, a, Markdown); !strings.Contains(got, want+"\n") {
		t.Fatalf("agent has\n%s", got)
	}
	fresh := r.join("Bob", Options{})
	if got, want := read(t, fresh, Critic), read(t, a, Critic); got != want {
		t.Fatalf("the hub has\n%s\nthe agent\n%s", got, want)
	}
}

func TestTrackedEdits(t *testing.T) {
	r := newRoom(t, loffice.Options{SaveDelay: 10 * time.Millisecond, SaveMaxDelay: 50 * time.Millisecond})
	a := r.join("Héraut", Options{})
	b := Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"},
		{Op: "insert", Block: "p3", Text: "Un risque **nouveau**.\n- premier point\n- second point"},
		{Op: "delete", Block: "p11"},
		{Op: "format", Block: "p5", Find: "reste", Format: map[string]string{"b": "1"}},
		{Op: "comment", Block: "p4", Find: "en retard", Text: "Depuis quand ?"},
		{Op: "reply", Comment: "cm0", Text: "Dix mille euros."},
	}}
	p, err := a.Preview(b)
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "preview.critic.md", p.Markup)
	if _, err := a.Apply(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	golden(t, "tracked.critic.md", read(t, a, Critic))
	golden(t, "tracked.md", read(t, a, Markdown))

	_, revs, _ := a.Revisions()
	var mine []string
	for _, rev := range revs {
		if rev.Author == "Héraut" {
			if rev.OnBehalfOf != "u1" {
				t.Errorf("revision %+v", rev)
			}
			mine = append(mine, rev.ID+" "+rev.Kind)
		}
	}
	if got := strings.Join(mine, ", "); got != "p2/1 del, p2/2 ins, p14/1 ins, p15/1 ins, p16/1 ins, p5/1 del, p5/2 ins, p11/1 del" {
		t.Errorf("revisions of the agent: %s", got)
	}
	if doc := saved(t, r); strings.Count(doc, `w:author="Héraut"`) < 6 {
		t.Errorf("saved\n%s", doc)
	}

	bob := r.join("Bob", Options{})
	_, revs, _ = bob.Revisions()
	review := Batch{OnBehalfOf: "Bob"}
	for i := len(revs) - 1; i >= 0; i-- {
		op := "accept"
		if strings.HasPrefix(revs[i].Text, "reste") || revs[i].Text == "premier point\n" {
			op = "reject"
		}
		review.Edits = append(review.Edits, Edit{Op: op, Revision: revs[i].ID})
	}
	if _, err := bob.Apply(t.Context(), review); err != nil {
		t.Fatal(err)
	}
	bob.mu.Lock()
	v := bob.s.version
	bob.mu.Unlock()
	settle(t, a, v)
	golden(t, "reviewed.critic.md", read(t, a, Critic))
}

// saved is the body of the document the hub saves next.
func saved(t *testing.T, r *room) string {
	t.Helper()
	select {
	case <-r.store.Saves:
	case <-time.After(2 * time.Second):
		t.Fatal("not saved")
	}
	data := []byte(r.store.File(key))
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := z.Open("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	doc, _ := io.ReadAll(f)
	return string(doc)
}

func TestBatchIsAtomic(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	a := r.join("Héraut", Options{})
	alice.next("join")
	before, v := read(t, a, Critic), a.Version()
	b := Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"},
		{Op: "replace", Block: "p4", Find: "absent", Text: "présent"},
	}}
	if _, err := a.Preview(b); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := a.Apply(t.Context(), b); !errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "edit 2") {
		t.Fatal(err)
	}
	if _, err := a.Apply(t.Context(), Batch{Edits: b.Edits[:1]}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a batch for nobody: %v", err)
	}
	b.Edits = b.Edits[:1]
	p, err := a.Preview(b)
	if err != nil || p.Version != v || p.Markup != "[p2] Le projet est {--**en avance**--}{++**à l'heure**++} et *sous budget*.\n" {
		t.Fatalf("preview %+v %v", p, err)
	}
	alice.quiet()
	if read(t, a, Critic) != before || a.Version() != v {
		t.Fatal("the preview changed the document")
	}
	rec, err := a.Apply(t.Context(), b)
	if err != nil || rec.OnBehalfOf != "u1" || rec.Version == v || rec.Version != a.Version() || len(rec.Edits) != 1 {
		t.Fatalf("record %+v %v", rec, err)
	}
	if j := a.Journal(); len(j) != 1 || j[0].OnBehalfOf != "u1" {
		t.Fatalf("journal %+v", j)
	}
	if f := alice.next("op"); bytes.Count(f.D, []byte(`"o":"txt"`)) != 1 {
		t.Fatalf("not one edit: %s", f.D)
	}
}

func TestStaleVersion(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	a := r.join("Héraut", Options{})
	alice.next("join")
	v := a.Version()
	f := alice.edit(typeAt(a, "p4", "Hélas, ", nil))
	settle(t, a, f.V)
	_, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{
		{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"},
		{Op: "delete", Block: "p4"},
	}})
	if !errors.Is(err, ErrConflict) || !strings.Contains(err.Error(), "p4") {
		t.Fatal(err)
	}
	alice.quiet()
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"}}}); err != nil {
		t.Fatalf("a block that did not change: %v", err)
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: "autre-3", Edits: []Edit{{Op: "delete", Block: "p2"}}}); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
}

func TestUndoMine(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	a := r.join("Héraut", Options{Now: func() time.Time { return now }})
	alice.next("join")
	for _, b := range []Batch{
		{OnBehalfOf: "u1", Edits: []Edit{{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"}, {Op: "insert", Block: "p5", Text: "Pour u1."}}},
		{OnBehalfOf: "u2", Edits: []Edit{{Op: "replace", Block: "p2", Find: "sous budget", Text: "dans le budget"}, {Op: "delete", Block: "p11"}}},
		{OnBehalfOf: "u1", Edits: []Edit{{Op: "format", Block: "p5", Find: "budget", Format: map[string]string{"i": "1"}}}},
	} {
		if _, err := a.Apply(t.Context(), b); err != nil {
			t.Fatal(err)
		}
	}
	f := alice.edit(typeAt(a, "p7", "Le ", ot.Attrs{"ins": "Alice", "insd": "2026-11-03T08:00:00Z"}))
	if f.T != "ack" {
		t.Fatalf("Alice's revision: %+v", f)
	}
	settle(t, a, f.V)
	if _, err := a.UndoMine(t.Context(), "u1"); err != nil {
		t.Fatal(err)
	}
	golden(t, "undo.critic.md", read(t, a, Critic))
	_, revs, _ := a.Revisions()
	var left []string
	for _, rev := range revs {
		left = append(left, rev.Author+"/"+rev.OnBehalfOf+" "+rev.Kind+" "+rev.Text)
	}
	if got := strings.Join(left, ", "); got != "Héraut/u2 del sous budget, Héraut/u2 ins dans le budget, Alice/ del pourrait , Alice/ ins va , Alice/ ins Le , Héraut/u2 del Fin du rapport.\n" {
		t.Fatalf("left %s", got)
	}
}

func TestAllow(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	var asked []string
	a := r.join("Héraut", Options{Allow: func(_ context.Context, b Batch) error {
		asked = append(asked, b.OnBehalfOf)
		if b.OnBehalfOf != "u1" {
			return errors.New("no consent")
		}
		return nil
	}})
	alice.next("join")
	b := Batch{OnBehalfOf: "u2", Edits: []Edit{{Op: "delete", Block: "p2"}}}
	if _, err := a.Apply(t.Context(), b); !errors.Is(err, ErrDenied) || !strings.Contains(err.Error(), "no consent") {
		t.Fatal(err)
	}
	alice.quiet()
	b.OnBehalfOf = "u1"
	if _, err := a.Apply(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	alice.next("op")
	if strings.Join(asked, ",") != "u2,u1" {
		t.Fatal(asked)
	}
}

func TestSlowModel(t *testing.T) {
	r := newRoom(t, loffice.Options{History: 10})
	alice := r.human("Alice")
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	clock := func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		return now
	}
	a := r.join("Héraut", Options{Now: clock})
	alice.next("join")
	v := a.Version()
	if _, err := a.Preview(Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"}}}); err != nil {
		t.Fatal(err)
	}
	for i := range 600 {
		if f := alice.edit(typeAt(a, "p11", "x", nil)); f.T != "ack" {
			t.Fatalf("edit %d: %+v", i, f)
		}
		settle(t, a, alice.v)
	}
	mu.Lock()
	now = now.Add(90 * time.Second)
	mu.Unlock()
	select {
	case <-a.Done():
		t.Fatal(a.Err())
	default:
	}
	rec, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"}}})
	if err != nil || !rec.Date.Equal(now) {
		t.Fatalf("%+v %v", rec, err)
	}
	if !strings.Contains(read(t, a, Markdown), "[p11] "+strings.Repeat("x", 600)+"Fin du rapport.") {
		t.Fatal("the agent missed edits")
	}
}

func TestGoneBlock(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	a := r.join("Héraut", Options{})
	alice.next("join")
	v := a.Version()
	a.mu.Lock()
	n, i, start, _ := a.s.paragraph("p4")
	size := flowLen(n.Text.Paragraphs()[i])
	a.mu.Unlock()
	var b builder
	b.to(start)
	b.remove(size)
	f := alice.edit(b.edit(n.ID))
	settle(t, a, f.V)
	for _, version := range []string{v, ""} {
		_, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: version, Edits: []Edit{
			{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"},
			{Op: "replace", Block: "p4", Find: "retard", Text: "avance"},
		}})
		if !errors.Is(err, ErrGone) || !strings.Contains(err.Error(), "edit 2: agent: no such block: p4 (deleted or rewritten") {
			t.Fatal(err)
		}
	}
	alice.quiet()
	if strings.Contains(read(t, a, Critic), "l'heure") {
		t.Fatal("partly applied")
	}

	if err := a.Focus("p5"); err != nil {
		t.Fatal(err)
	}
	var eph struct {
		S Cursor `json:"s"`
	}
	if err := json.Unmarshal(alice.next("eph").D, &eph); err != nil || eph.S.Node != n.ID || eph.S.Extent <= eph.S.Base {
		t.Fatalf("cursor %+v %v", eph, err)
	}
	if err := a.Focus("p4"); !errors.Is(err, ErrGone) {
		t.Fatal(err)
	}
}

func TestDirectEdits(t *testing.T) {
	a := newRoom(t, loffice.Options{}).join("Héraut", Options{})
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "delete", Block: "t6"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a table deleted as a revision: %v", err)
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Direct: true, Edits: []Edit{
		{Op: "insert", Text: "# Résumé"},
		{Op: "insert", Block: "t6", Text: "Après le tableau."},
		{Op: "delete", Block: "t6"},
		{Op: "delete", Block: "p3"},
		{Op: "replace", Block: "p5", Text: "Budget validé."},
		{Op: "format", Block: "p11", Find: "rapport", Format: map[string]string{"u": "single"}},
	}}); err != nil {
		t.Fatal(err)
	}
	golden(t, "direct.md", read(t, a, Markdown))
	if _, revs, _ := a.Revisions(); len(revs) != 2 {
		t.Fatalf("revisions %+v", revs)
	}
}

func TestLastParagraphOfFlow(t *testing.T) {
	a := newRoom(t, loffice.Options{}).join("Héraut", Options{})
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Direct: true, Edits: []Edit{{Op: "delete", Block: "p5"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "insert", Block: "p4", Text: "Ajout."}}}); err != nil {
		t.Fatal(err)
	}
	_, revs, _ := a.Revisions()
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "reject", Revision: revs[len(revs)-1].ID}}}); err != nil {
		t.Fatal(err)
	}
	if got := read(t, a, Markdown); !strings.Contains(got, "[p4] Le fournisseur va livrer en retard.\n[t6]") {
		t.Fatalf("ids moved:\n%s", got)
	}
}

func TestSessions(t *testing.T) {
	r := newRoom(t, loffice.Options{})
	alice := r.human("Alice")
	if _, err := Join(t.Context(), r.hub, key, trame.Peer{ID: "x"}, Options{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an agent without a name: %v", err)
	}
	peer := trame.Peer{ID: "heraut", Name: "Héraut", Client: "c1"}
	first, err := Join(t.Context(), r.hub, key, peer, Options{})
	if err != nil {
		t.Fatal(err)
	}
	b := Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "replace", Block: "p2", Find: "en avance", Text: "à l'heure"}}}
	if _, err := first.Apply(t.Context(), b); err != nil {
		t.Fatal(err)
	}
	v := first.Version()
	first.Leave()
	alice.next("op")
	second, err := Join(t.Context(), r.hub, key, peer, Options{Journal: first.Journal()})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Leave()
	b.Edits[0] = Edit{Op: "replace", Block: "p4", Find: "retard", Text: "avance"}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if _, err := second.Apply(ctx, b); err != nil {
		t.Fatal(err)
	}
	b.Version = v
	if _, err := second.Apply(ctx, b); !errors.Is(err, ErrStale) {
		t.Fatalf("a version of another agent: %v", err)
	}
	_, revs, _ := second.Revisions()
	if revs[0].Author != "Héraut" || revs[0].OnBehalfOf != "u1" {
		t.Fatalf("the journal of the first session: %+v", revs[0])
	}
}
