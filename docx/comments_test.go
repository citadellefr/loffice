package docx

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

// withComments is a document of that body with a comment and its answer,
// resolved.
func withComments(t *testing.T, body string) []byte {
	t.Helper()
	pkg, err := opc.Open(build(t, body), Limits)
	if err != nil {
		t.Fatal(err)
	}
	rels, _ := pkg.Read("word/_rels/document.xml.rels")
	rels = bytes.Replace(rels, []byte("</Relationships>"), []byte(`<Relationship Id="rId8" Type="`+relComments+`" Target="comments.xml"/>`+
		`<Relationship Id="rId9" Type="`+relCommentsExtended+`" Target="commentsExtended.xml"/></Relationships>`), 1)
	const ns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"`
	for _, err := range []error{
		pkg.Set("word/_rels/document.xml.rels", rels),
		pkg.Add("word/comments.xml", typeComments, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+
			`<w:comments `+ns+`><w:comment w:id="0" w:author="Alice" w:date="2026-09-01T10:00:00Z" w:initials="A"><w:p w14:paraId="00000001"><w:r><w:annotationRef/></w:r><w:r><w:t>Pourquoi ?</w:t></w:r></w:p></w:comment>`+
			`<w:comment w:id="3" w:author="Bob" w:date="2026-09-02T10:00:00Z" w:initials="B"><w:p w14:paraId="00000002"><w:r><w:t>Parce que.</w:t></w:r></w:p></w:comment></w:comments>`)),
		pkg.Add("word/commentsExtended.xml", typeCommentsEx, []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\n"+
			`<w15:commentsEx xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"><w15:commentEx w15:paraId="00000001" w15:done="1"/><w15:commentEx w15:paraId="00000002" w15:paraIdParent="00000001" w15:done="0"/></w15:commentsEx>`)),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	data, err := pkg.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return data
}

const commented = `<w:p><w:r><w:t xml:space="preserve">Un </w:t></w:r><w:commentRangeStart w:id="0"/><w:r><w:t>mot</w:t></w:r><w:commentRangeEnd w:id="0"/>` +
	`<w:r><w:commentReference w:id="0"/></w:r><w:commentRangeStart w:id="7"/><w:r><w:t>.</w:t></w:r></w:p>`

func TestReadComments(t *testing.T) {
	_, tree, err := Open(withComments(t, commented))
	if err != nil {
		t.Fatal(err)
	}
	c := tree.Node("cm0")
	if c == nil || c.Type != "comment" || str(c.Attrs, "author") != "Alice" || str(c.Attrs, "initials") != "A" || !done(c) {
		t.Fatalf("comment %+v", c)
	}
	if text := tree.Children("cm0")[0].Text.Delta(); text[len(text)-2].Insert != "Pourquoi ?" {
		t.Errorf("comment text %+v", text)
	}
	if answer := tree.Node("cm3"); answer == nil || str(answer.Attrs, "parent") != "cm0" || done(answer) {
		t.Errorf("answer %+v", answer)
	}
	var anchors []string
	for _, o := range firstText(t, tree).Text.Delta() {
		for _, k := range []string{"cs", "ce", "comment"} {
			if o.Attrs[k] != "" {
				anchors = append(anchors, k+":"+o.Attrs[k])
			}
		}
	}
	if got := strings.Join(anchors, " "); got != "cs:cm0 ce:cm0 comment:cm0" {
		t.Errorf("anchors %s", got)
	}
}

func TestEditComments(t *testing.T) {
	data := withComments(t, commented)
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	text := tree.Children("cm3")[0]
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpTxt, ID: text.ID, Text: ot.Delta{{Retain: 10}, {Insert: " Voilà."}}},
		{Op: ot.OpSet, ID: "cm3", Attrs: ot.Values{"done": json.RawMessage("true")}},
	})
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: "cm3", Attrs: ot.Values{"author": json.RawMessage(`"Eve"`)}}}, ""); err == nil {
		t.Error("an author changed")
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	comments := partOf(t, saved, "word/comments.xml")
	if !strings.Contains(comments, `<w:t>Parce que. Voilà.</w:t>`) || !strings.Contains(comments, `<w:annotationRef/>`) {
		t.Errorf("comments\n%s", comments)
	}
	if ex := partOf(t, saved, "word/commentsExtended.xml"); !strings.Contains(ex, `<w15:commentEx w15:paraId="00000002" w15:paraIdParent="00000001" w15:done="1"/>`) {
		t.Errorf("extended\n%s", ex)
	}
	if partOf(t, saved, "word/document.xml") != partOf(t, data, "word/document.xml") {
		t.Error("the document was rewritten")
	}
}

func TestAddComment(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>Un mot.</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	edit := func(author string) ot.Edit {
		return ot.Edit{
			{Op: ot.OpNew, ID: "k1", Type: "comment", Parent: "doc", Key: "z", Attrs: ot.Values{
				"author": json.RawMessage(`"` + author + `"`), "initials": json.RawMessage(`"A"`), "date": json.RawMessage(`"2026-09-29T08:00:00Z"`)}},
			{Op: ot.OpNew, ID: "k2", Type: "text", Parent: "k1", Key: "V", Text: ot.Delta{{Insert: "À revoir\n"}}},
			{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{{Retain: 3}, {Insert: Object, Attrs: ot.Attrs{"cs": "k1"}}, {Retain: 3},
				{Insert: Object, Attrs: ot.Attrs{"ce": "k1"}}, {Insert: Object, Attrs: ot.Attrs{"comment": "k1"}},
				{Retain: 1}, {Insert: Object, Attrs: ot.Attrs{"comment": "k1"}}}},
		}
	}
	if err := d.Check(tree, edit("Bob"), "Alice"); err == nil {
		t.Error("a comment signed by another")
	}
	if err := d.Check(tree, edit("Alice"), "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(edit("Alice")); err != nil {
		t.Fatal(err)
	}
	answer := ot.Edit{
		{Op: ot.OpNew, ID: "k3", Type: "comment", Parent: "doc", Key: "zz", Attrs: ot.Values{
			"author": json.RawMessage(`"Bob"`), "date": json.RawMessage(`"2026-09-29T09:00:00Z"`), "parent": json.RawMessage(`"k1"`)}},
		{Op: ot.OpNew, ID: "k4", Type: "text", Parent: "k3", Key: "V", Text: ot.Delta{{Insert: "D'accord\n"}}},
	}
	if err := d.Check(tree, answer, "Bob"); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(answer); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	if strings.Count(doc, "<w:commentReference ") != 1 || !strings.Contains(doc, `<w:t xml:space="preserve">Un </w:t></w:r><w:commentRangeStart w:id="0"/><w:r><w:t>mot</w:t></w:r><w:commentRangeEnd w:id="0"/><w:r><w:commentReference w:id="0"/><w:t>.</w:t></w:r>`) {
		t.Errorf("document\n%s", doc)
	}
	comments := partOf(t, saved, "word/comments.xml")
	if !strings.Contains(comments, `<w:comment w:id="0" w:author="Alice" w:date="2026-09-29T08:00:00Z" w:initials="A"><w:p w14:paraId=`) ||
		!strings.Contains(comments, `<w:comment w:id="1" w:author="Bob" w:date="2026-09-29T09:00:00Z"><w:p w14:paraId=`) {
		t.Errorf("comments\n%s", comments)
	}
	if !strings.Contains(partOf(t, saved, "word/commentsExtended.xml"), `w15:paraIdParent=`) {
		t.Error("the answer was not written")
	}
	rels := partOf(t, saved, "word/_rels/document.xml.rels")
	if !strings.Contains(rels, relComments) || !strings.Contains(rels, relCommentsExtended) {
		t.Errorf("relationships\n%s", rels)
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	if c := again.Node("cm1"); c == nil || str(c.Attrs, "parent") != "cm0" || str(again.Node("cm0").Attrs, "author") != "Alice" {
		t.Errorf("read back %+v", c)
	}
}

func TestDeleteComment(t *testing.T) {
	d, tree, err := Open(withComments(t, commented))
	if err != nil {
		t.Fatal(err)
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpDel, ID: "cm0"}, {Op: ot.OpDel, ID: "cm3"}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	if strings.Contains(doc, `w:id="0"`) || !strings.Contains(doc, `<w:commentRangeStart w:id="7"/>`) {
		t.Errorf("document\n%s", doc)
	}
	if comments := partOf(t, saved, "word/comments.xml"); strings.Contains(comments, "<w:comment ") {
		t.Errorf("comments\n%s", comments)
	}
	if ex := partOf(t, saved, "word/commentsExtended.xml"); strings.Contains(ex, "commentEx ") {
		t.Errorf("extended\n%s", ex)
	}
}
