package agent

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/trame/ot"
)

const deckKey = "deck.pptx"

// deck is testdata/deck.pptx: testdata/pptx/transitional.pptx with a shape
// on the master, a layout of placeholders "Titre et contenu", a second
// slide, notes and a table on the first.
func deck(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/deck.pptx")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// joinDeck is a room of the deck with Alice and the agent in it.
func joinDeck(t *testing.T, opt loffice.Options) (*room, *human, *Agent) {
	t.Helper()
	r := newRoomOf(t, deckKey, deck(t), opt)
	alice := r.human("Alice")
	a := r.join("Héraut", Options{})
	alice.next("join")
	return r, alice, a
}

func deckApply(t *testing.T, a *Agent, edits ...Edit) Record {
	t.Helper()
	rec, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: edits})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func slideOf(t *testing.T, a *Agent, id string) Slide {
	t.Helper()
	_, slide, err := a.Slide(id)
	if err != nil {
		t.Fatal(err)
	}
	return slide
}

func shapeOf(t *testing.T, a *Agent, slide, id string) Shape {
	t.Helper()
	for _, sh := range slideOf(t, a, slide).Shapes {
		if sh.ID == id {
			return sh
		}
	}
	t.Fatalf("no shape %s on %s", id, slide)
	return Shape{}
}

func slideIDs(t *testing.T, a *Agent) []string {
	t.Helper()
	_, slides, _, err := a.Slides()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, s := range slides {
		ids = append(ids, s.ID)
	}
	return ids
}

// typeIn is Alice typing text at the start of a shape.
func typeIn(alice *human, a *Agent, id, text string) {
	alice.t.Helper()
	f := alice.edit(ot.Edit{{Op: ot.OpTxt, ID: id, Text: ot.Delta{{Insert: text}}}})
	if f.T != "ack" {
		alice.t.Fatalf("Alice typing: %+v", f)
	}
	settle(alice.t, a, f.V)
}

// savedDeck is the file once the room saved the last version Alice saw.
func savedDeck(r *room, alice *human) []byte {
	for alice.next("saved").V < alice.v {
	}
	return []byte(r.store.File(deckKey))
}

func TestSlidesRead(t *testing.T) {
	a := newRoomOf(t, deckKey, deck(t), loffice.Options{}).join("Héraut", Options{})
	version, slides, layouts, err := a.Slides()
	if err != nil || version != a.Version() {
		t.Fatalf("slides %q %v", version, err)
	}
	if want := []SlideInfo{{"s256", 1, "Vide", "Strict", false}, {"s257", 2, "Titre et contenu", "Suite", false}}; !reflect.DeepEqual(slides, want) {
		t.Errorf("slides %+v", slides)
	}
	if want := []Layout{{"L1", "Vide"}, {"L2", "Titre et contenu"}}; !reflect.DeepEqual(layouts, want) {
		t.Errorf("layouts %+v", layouts)
	}
	_, slide, err := a.Slide("s256")
	if err != nil {
		t.Fatal(err)
	}
	want := Slide{
		SlideInfo: slides[0],
		Shapes: []Shape{
			{ID: "s256-2", Type: "sp", Name: "Titre 1", Placeholder: "title", Text: "Strict"},
			{ID: "s256-3", Type: "sp", Name: "Corps", Text: "Alpha\nBeta"},
			{ID: "s256-4", Type: "frame", Name: "Tableau", Box: &Box{66, 236, 472, 63}, Text: "Lot | Date\nÉtude | Mars"},
		},
		Notes: "Dire bonjour",
	}
	if !reflect.DeepEqual(slide, want) {
		t.Errorf("slide %+v", slide)
	}
	if _, _, err := a.Slide("nope"); !errors.Is(err, ErrGone) {
		t.Errorf("unknown slide: %v", err)
	}
	if _, _, err := a.Slide("s256-3"); !errors.Is(err, ErrGone) {
		t.Errorf("a shape is not a slide: %v", err)
	}
	if _, _, _, err := (&Agent{kind: ".docx"}).Slides(); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Word: %v", err)
	}
}

func TestSlideEdits(t *testing.T) {
	r, alice, a := joinDeck(t, loffice.Options{SaveDelay: 10 * time.Millisecond, SaveMaxDelay: 50 * time.Millisecond})
	do := func(edits ...Edit) {
		t.Helper()
		deckApply(t, a, edits...)
		if f := alice.next("op"); f.SID == 0 {
			t.Fatalf("frame %+v", f)
		}
	}

	do(Edit{Op: "text", Block: "s256-3", Text: "Un\nDeux\nTrois"})
	if got := shapeOf(t, a, "s256", "s256-3").Text; got != "Un\nDeux\nTrois" {
		t.Errorf("text %q", got)
	}
	a.mu.Lock()
	flow := a.s.tree.Node("s256-3").Text.Delta()
	a.mu.Unlock()
	for _, o := range flow {
		if o.Insert != "\n" && (o.Attrs["b"] != "1" || o.Attrs["r"] == "") {
			t.Errorf("run %+v lost the style of the first", o)
		}
	}

	do(Edit{Op: "shape", Block: "s257", Text: "Pied\nde page", Box: &Box{72, 400, 200, 30}})
	slide := slideOf(t, a, "s257")
	if len(slide.Shapes) != 2 || slide.Shapes[1].Type != "sp" || slide.Shapes[1].Text != "Pied\nde page" || *slide.Shapes[1].Box != (Box{72, 400, 200, 30}) {
		t.Errorf("shapes %+v", slide.Shapes)
	}

	do(Edit{Op: "slide", Block: "s256", Layout: "titre et contenu"})
	ids := slideIDs(t, a)
	if len(ids) != 3 || ids[0] != "s256" || ids[2] != "s257" {
		t.Fatalf("slides %v", ids)
	}
	added := slideOf(t, a, ids[1])
	if added.Layout != "Titre et contenu" || len(added.Shapes) != 2 || added.Shapes[0].Placeholder != "title" || added.Shapes[1].Placeholder != "obj" ||
		added.Shapes[0].Name != "Titre 1" {
		t.Errorf("new slide %+v", added)
	}
	do(Edit{Op: "slide", Layout: "L1"})
	ids = slideIDs(t, a)
	if last := slideOf(t, a, ids[len(ids)-1]); len(ids) != 4 || last.Layout != "Vide" || len(last.Shapes) != 0 {
		t.Errorf("last slide %+v of %v", last, ids)
	}

	do(Edit{Op: "notes", Block: "s256", Text: "Nouvelles notes"})
	if got := slideOf(t, a, "s256").Notes; got != "Nouvelles notes" {
		t.Errorf("notes %q", got)
	}
	do(Edit{Op: "comment", Block: "s257", Text: "À revoir"})
	if got := slideOf(t, a, "s257").Comments; len(got) != 1 || got[0].Author != "Héraut" || got[0].Text != "À revoir" {
		t.Errorf("comments %+v", got)
	}
	do(Edit{Op: "delete", Block: "s256-4"}, Edit{Op: "delete", Block: "s257"})
	if got := slideIDs(t, a); len(got) != 3 || slices.Contains(got, "s257") {
		t.Errorf("slides %v", got)
	}
	if _, _, err := a.Slide("s257"); !errors.Is(err, ErrGone) {
		t.Errorf("deleted slide: %v", err)
	}

	b := newRoomOf(t, deckKey, savedDeck(r, alice), loffice.Options{}).join("Bob", Options{})
	_, slides, _, err := b.Slides()
	if err != nil || len(slides) != 3 || slides[1].Layout != "Titre et contenu" || slides[2].Layout != "Vide" {
		t.Fatalf("saved slides %+v %v", slides, err)
	}
	saved := slideOf(t, b, "s256")
	if len(saved.Shapes) != 2 || saved.Shapes[1].Text != "Un\nDeux\nTrois" || saved.Notes != "Nouvelles notes" {
		t.Errorf("saved slide %+v", saved)
	}
}

func TestSlideBatchAtomicAndPreview(t *testing.T) {
	_, alice, a := joinDeck(t, loffice.Options{})
	before := a.Version()
	_, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "text", Block: "s256-3", Text: "Gamma"},
		{Op: "text", Block: "s256-9", Text: "Delta"},
	}})
	if !errors.Is(err, ErrGone) || !strings.Contains(err.Error(), "edit 2") {
		t.Fatalf("second edit: %v", err)
	}
	if a.Version() != before || shapeOf(t, a, "s256", "s256-3").Text != "Alpha\nBeta" {
		t.Error("a failed batch changed the deck")
	}
	alice.quiet()

	p, err := a.Preview(Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "text", Block: "s256-3", Text: "Gamma"},
		{Op: "slide", Layout: "L2"},
		{Op: "delete", Block: "s257"},
	}})
	if err != nil || p.Version != before {
		t.Fatalf("preview %+v %v", p, err)
	}
	for _, want := range []string{`sp "Corps" (s256-3): {--Alpha` + "\nBeta" + `--}{++Gamma++}`, "{++slide ", `{--slide s257--}`} {
		if !strings.Contains(p.Markup, want) {
			t.Errorf("preview misses %q:\n%s", want, p.Markup)
		}
	}
	if strings.Contains(p.Markup, "Titre 1") {
		t.Errorf("preview lists the shapes of a new or deleted slide:\n%s", p.Markup)
	}
	if a.Version() != before || len(slideIDs(t, a)) != 2 || shapeOf(t, a, "s256", "s256-3").Text != "Alpha\nBeta" {
		t.Error("a preview changed the deck")
	}
	alice.quiet()
}

func TestSlideConflict(t *testing.T) {
	_, alice, a := joinDeck(t, loffice.Options{})
	v := a.Version()
	typeIn(alice, a, "s256-3", "Bonjour ")

	apply := func(e Edit) error {
		_, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{e}})
		return err
	}
	for _, e := range []Edit{
		{Op: "text", Block: "s256-3", Text: "Gamma"},
		{Op: "delete", Block: "s256-3"},
		{Op: "delete", Block: "s256"},
	} {
		if err := apply(e); !errors.Is(err, ErrConflict) {
			t.Errorf("%s %s: %v", e.Op, e.Block, err)
		}
	}
	if got := shapeOf(t, a, "s256", "s256-3").Text; got != "Bonjour Alpha\nBeta" {
		t.Errorf("text %q", got)
	}
	for _, e := range []Edit{
		{Op: "text", Block: "s256-2", Text: "Autre titre"},
		{Op: "shape", Block: "s256", Text: "Ajout", Box: &Box{0, 0, 10, 10}},
		{Op: "comment", Block: "s256", Text: "Vu"},
	} {
		if err := apply(e); err != nil {
			t.Errorf("%s on what did not change: %v", e.Op, err)
		}
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: "other-1", Edits: []Edit{{Op: "delete", Block: "s257"}}}); !errors.Is(err, ErrStale) {
		t.Errorf("version of another session: %v", err)
	}
}

func TestSlideInvalidEdits(t *testing.T) {
	_, _, a := joinDeck(t, loffice.Options{})
	for _, c := range []struct {
		edit Edit
		want error
	}{
		{Edit{Op: "replace", Block: "s256-3", Text: "x"}, ErrInvalid},
		{Edit{Op: "text", Text: "x"}, ErrInvalid},
		{Edit{Op: "text", Block: "s256", Text: "x"}, ErrInvalid},
		{Edit{Op: "text", Block: "s256-4", Text: "x"}, ErrInvalid},
		{Edit{Op: "text", Block: "s999-1", Text: "x"}, ErrGone},
		{Edit{Op: "shape", Block: "s256", Text: "x"}, ErrInvalid},
		{Edit{Op: "shape", Block: "s256", Text: "x", Box: &Box{1, 1, 0, 5}}, ErrInvalid},
		{Edit{Op: "shape", Block: "s256", Text: "x", Box: &Box{1e308, 1, 5, 5}}, ErrInvalid},
		{Edit{Op: "shape", Block: "s256-3", Text: "x", Box: &Box{1, 1, 5, 5}}, ErrInvalid},
		{Edit{Op: "slide"}, ErrInvalid},
		{Edit{Op: "slide", Layout: "Inconnu"}, ErrInvalid},
		{Edit{Op: "slide", Layout: "M1-2"}, ErrInvalid},
		{Edit{Op: "slide", Layout: "Title Placeholder 1"}, ErrInvalid},
		{Edit{Op: "slide", Block: "s256-3", Layout: "L1"}, ErrInvalid},
		{Edit{Op: "delete", Block: "L1"}, ErrInvalid},
		{Edit{Op: "delete", Block: "s256-4-r1-c1"}, ErrInvalid},
		{Edit{Op: "notes", Block: "s257", Text: "x"}, ErrInvalid},
		{Edit{Op: "notes", Block: "s999"}, ErrGone},
		{Edit{Op: "comment", Block: "s256", Text: "  "}, ErrInvalid},
	} {
		if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{c.edit}}); !errors.Is(err, c.want) {
			t.Errorf("%+v: %v", c.edit, err)
		}
	}
}

func TestSlideUndoMine(t *testing.T) {
	_, alice, a := joinDeck(t, loffice.Options{})
	_, slides, _, _ := a.Slides()
	deckApply(t, a,
		Edit{Op: "text", Block: "s256-3", Text: "Un\nDeux"},
		Edit{Op: "delete", Block: "s256-4"},
		Edit{Op: "shape", Block: "s256", Text: "Ajout", Box: &Box{10, 10, 100, 20}},
		Edit{Op: "slide", Block: "s256", Layout: "L2"})
	deckApply(t, a, Edit{Op: "delete", Block: "s257"})
	typeIn(alice, a, "s256-2", "Bonjour ")

	rec, err := a.UndoMine(t.Context(), "u1")
	if err != nil || len(rec.Edits) != 5 {
		t.Fatalf("undo %+v %v", rec, err)
	}
	if got := slideIDs(t, a); !slices.Equal(got, []string{"s256", "s257"}) {
		t.Errorf("slides %v", got)
	}
	slide := slideOf(t, a, "s256")
	var texts []string
	for _, sh := range slide.Shapes {
		texts = append(texts, sh.ID+"="+sh.Text)
	}
	if got := strings.Join(texts, ","); got != "s256-2=Bonjour Strict,s256-3=Alpha\nBeta,s256-4=Lot | Date\nÉtude | Mars" {
		t.Errorf("shapes of the first slide: %s", got)
	}
	if got := slideOf(t, a, "s257"); got.Title != "Suite" || got.Layout != slides[1].Layout {
		t.Errorf("restored slide %+v", got)
	}
	if rec, err := a.UndoMine(t.Context(), "u1"); err != nil || len(rec.Edits) != 0 {
		t.Errorf("nothing left to undo: %+v %v", rec, err)
	}
}

func TestSlideUndoOwnEdits(t *testing.T) {
	_, _, a := joinDeck(t, loffice.Options{})
	before := slideOf(t, a, "s256")
	deckApply(t, a, Edit{Op: "text", Block: "s256-3", Text: "Un"})
	deckApply(t, a, Edit{Op: "text", Block: "s256-3", Text: "Deux"})
	deckApply(t, a, Edit{Op: "shape", Block: "s256", Text: "Ajout", Box: &Box{10, 10, 100, 20}})
	added := slideOf(t, a, "s256").Shapes
	deckApply(t, a, Edit{Op: "delete", Block: added[len(added)-1].ID})
	deckApply(t, a, Edit{Op: "slide", Block: "s256", Layout: "L2"})
	deckApply(t, a, Edit{Op: "delete", Block: slideIDs(t, a)[1]})

	if rec, err := a.UndoMine(t.Context(), "u1"); err != nil || len(rec.Edits) != 6 {
		t.Fatalf("undo %+v %v", rec, err)
	}
	if got := slideIDs(t, a); !slices.Equal(got, []string{"s256", "s257"}) {
		t.Errorf("slides %v", got)
	}
	if got := slideOf(t, a, "s256"); !reflect.DeepEqual(got, before) {
		t.Errorf("got %+v, want %+v", got, before)
	}
}

func TestSlideUndoRefusedWhenChanged(t *testing.T) {
	_, alice, a := joinDeck(t, loffice.Options{})
	deckApply(t, a, Edit{Op: "text", Block: "s256-3", Text: "Gamma"}, Edit{Op: "notes", Block: "s256", Text: "Autres notes"})
	alice.next("op")
	typeIn(alice, a, "s256-3", "Bonjour ")
	_, err := a.UndoMine(t.Context(), "u1")
	if !errors.Is(err, ErrChanged) || !strings.Contains(err.Error(), "s256-3") {
		t.Fatalf("undo: %v", err)
	}
	if got := shapeOf(t, a, "s256", "s256-3").Text; got != "Bonjour Gamma" {
		t.Errorf("text %q", got)
	}
	if got := slideOf(t, a, "s256").Notes; got != "Autres notes" {
		t.Errorf("notes %q", got)
	}
}

func TestSlideUndoDeletedWithNotesAndComments(t *testing.T) {
	_, alice, a := joinDeck(t, loffice.Options{})
	f := alice.edit(ot.Edit{{Op: ot.OpNew, ID: "c1", Type: "comment", Parent: "s256", Key: "m", Text: ot.Delta{{Insert: "Humain\n"}}, Attrs: ot.Values{
		"author": quote("Alice"), "initials": quote("A"), "date": quote("2026-11-03T09:00:00Z"),
	}}})
	if f.T != "ack" {
		t.Fatalf("Alice's comment: %+v", f)
	}
	settle(t, a, f.V)
	if got := slideOf(t, a, "s256").Comments; len(got) != 1 {
		t.Fatalf("comments %+v", got)
	}
	deckApply(t, a, Edit{Op: "comment", Block: "s256", Text: "Mon commentaire"}, Edit{Op: "delete", Block: "s256"})
	rec, err := a.UndoMine(t.Context(), "u1")
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(rec.Lost)
	if want := []string{"the comment of Alice on slide s256", "the notes of slide s256"}; !slices.Equal(rec.Lost, want) {
		t.Errorf("lost %q", rec.Lost)
	}
	slide := slideOf(t, a, "s256")
	if len(slide.Shapes) != 3 || slide.Notes != "" || len(slide.Comments) != 0 {
		t.Errorf("restored slide %+v", slide)
	}
}

func TestSlideRecentMarks(t *testing.T) {
	_, alice, a := joinDeck(t, loffice.Options{})
	deckApply(t, a, Edit{Op: "text", Block: "s256-3", Text: "Gamma"})
	for {
		var eph struct {
			Recent []Mark `json:"recent"`
		}
		if err := json.Unmarshal(alice.next("eph").D, &eph); err != nil {
			t.Fatal(err)
		}
		if len(eph.Recent) == 0 {
			continue
		}
		if len(eph.Recent) != 1 || eph.Recent[0].Node != "s256-3" || eph.Recent[0].Cells != nil || eph.Recent[0].At == 0 {
			t.Errorf("recent %+v", eph.Recent)
		}
		return
	}
}
