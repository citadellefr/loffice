package loffice

import (
	"testing"

	"github.com/citadellefr/loffice/ot"
)

func TestTextFiles(t *testing.T) {
	for _, c := range []struct {
		file, flow, saved string
	}{
		{"", "\n", ""},
		{"one\ntwo\n", "one\ntwo\n\n", "one\ntwo\n"},
		{"\xEF\xBB\xBFa\r\nb", "a\nb\n", "\xEF\xBB\xBFa\r\nb"},
		{"a\r\nb\nc", "a\nb\nc\n", "a\r\nb\r\nc"},
		{"old\rmac", "old\rmac\n", "old\rmac"},
		{"caf\xE9 \x80", "café €\n", "café €"},
	} {
		doc, f, err := openText("", []byte(c.file))
		if err != nil {
			t.Fatal(err)
		}
		if got := doc.Node(textBody).Text.Delta(); len(got) != 1 || got[0].Insert != c.flow {
			t.Errorf("%q read as %v, want %q", c.file, got, c.flow)
		}
		if got, _ := f.encode(doc); string(got) != c.saved {
			t.Errorf("%q written back as %q, want %q", c.file, got, c.saved)
		}
	}
}

func TestTextFileKeepsParagraphsApart(t *testing.T) {
	doc, f, err := openText("", []byte("a\nb"))
	if err != nil {
		t.Fatal(err)
	}
	d := ot.Delta{{Retain: 1}, {Insert: "\n\n"}, {Retain: 2, Attrs: ot.Attrs{"b": "1"}}}
	if err := doc.Apply(ot.Edit{{Op: ot.OpTxt, ID: textBody, Text: d}}); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.encode(doc); string(got) != "a\n\n\nb" {
		t.Fatalf("saved %q", got)
	}
}

func TestTextFileOnlyEditsItsText(t *testing.T) {
	_, f, err := openText("", []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range []ot.Edit{
		{{Op: ot.OpNew, ID: "x", Type: "text"}},
		{{Op: ot.OpTxt, ID: "x", Text: ot.Delta{{Insert: "a"}}}},
		{{Op: ot.OpSet, ID: textBody, Key: "V"}},
		{{Op: ot.OpDel, ID: textBody}},
	} {
		if f.check(nil, e, Peer{}) == nil {
			t.Errorf("%v passes", e)
		}
	}
}
