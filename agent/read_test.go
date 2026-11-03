package agent

import (
	"errors"
	"testing"

	"github.com/citadellefr/loffice"
)

func TestRead(t *testing.T) {
	a := newRoom(t, loffice.Options{}).join("Héraut", Options{})
	golden(t, "sample.md", read(t, a, Markdown))
	golden(t, "sample.critic.md", read(t, a, Critic))
}

func TestOutlineSearchRevisions(t *testing.T) {
	a := newRoom(t, loffice.Options{}).join("Héraut", Options{})
	_, headings, err := a.Outline()
	if err != nil || len(headings) != 2 || headings[0] != (Heading{"p1", 1, "Rapport"}) || headings[1] != (Heading{"p3", 2, "Risques"}) {
		t.Fatalf("outline %v %v", headings, err)
	}
	_, matches, err := a.Search("budget", false)
	if err != nil || len(matches) != 2 || matches[0].Block != "p2" || matches[0].Offset != 32 || matches[1].Block != "p5" || matches[1].Offset != 3 ||
		matches[1].Excerpt != "Le budget reste à valider." {
		t.Fatalf("search %+v %v", matches, err)
	}
	_, matches, err = a.Search(`L[a-z]+`, true)
	if err != nil || len(matches) != 4 || matches[3].Text != "Lot" {
		t.Fatalf("regex %+v %v", matches, err)
	}
	if _, _, err := a.Search("(", true); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	_, revs, err := a.Revisions()
	if err != nil || len(revs) != 2 || revs[0].ID != "p4/1" || revs[0].Kind != "del" || revs[0].Author != "Alice" || revs[0].Text != "pourrait " ||
		revs[1].ID != "p4/2" || revs[1].Kind != "ins" || revs[1].Text != "va " || revs[1].Date.IsZero() {
		t.Fatalf("revisions %+v %v", revs, err)
	}
}

func TestUnsupported(t *testing.T) {
	a := &Agent{kind: ".xlsx"}
	if _, _, err := a.Read(Markdown); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1"}); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
