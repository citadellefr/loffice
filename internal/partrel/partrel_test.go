package partrel

import (
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

func TestRelative(t *testing.T) {
	for _, c := range []struct{ source, name, want string }{
		{"word/document.xml", "word/media/image1.png", "media/image1.png"},
		{"ppt/slides/slide1.xml", "ppt/media/image1.png", "../media/image1.png"},
		{"", "word/document.xml", "word/document.xml"},
		{"word/header1.xml", "word/header1.xml", "header1.xml"},
	} {
		if got := Relative(c.source, c.name); got != c.want {
			t.Errorf("Relative(%q, %q) = %q, want %q", c.source, c.name, got, c.want)
		}
	}
}

// A name does not depend on the part the relationship is read from, and
// the writer of another part gives it an id of its own.
func TestNamesAcrossParts(t *testing.T) {
	names := NewNames(nil)
	link := opc.Relationship{ID: "rId7", Type: "http://link", Target: "https://example.com", External: true}
	a := New([]opc.Relationship{link})
	b := New([]opc.Relationship{{ID: "rId1", Type: "other", Target: "x.xml"}, {ID: "rId2", Type: "http://link", Target: "https://example.com", External: true}})
	na, nb := names.Name("word/document.xml", a, "rId7"), names.Name("word/header1.xml", b, "rId2")
	if na == "" || na != nb {
		t.Fatalf("names %q and %q", na, nb)
	}

	e, err := xmldom.ParseFragment([]byte(`<w:hyperlink xmlns:w="w" xmlns:r="`+NS+`" r:id="`+na+`" r:x="@unknown"/>`), nil)
	if err != nil {
		t.Fatal(err)
	}
	w := NewWriter("word/footer1.xml", nil, names.Lookup)
	w.Resolve(e, nil)
	if got := e.Get("r:id"); got != "rId1" || e.Get("r:x") != "" || !w.Changed || len(w.List) != 1 {
		t.Fatalf("resolved to %q, %v", got, w.List)
	}
}
