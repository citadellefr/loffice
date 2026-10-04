package docx

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/citadellefr/trame/ot"
)

const revised = `<w:p><w:pPr><w:rPr><w:ins w:id="5" w:author="Alice" w:date="2026-09-01T10:00:00Z"/></w:rPr></w:pPr>` +
	`<w:r><w:t xml:space="preserve">Un </w:t></w:r><w:ins w:id="1" w:author="Alice" w:date="2026-09-01T10:00:00Z"><w:r><w:t>ajout</w:t></w:r></w:ins>` +
	`<w:del w:id="2" w:author="Bob" w:date="2026-09-02T10:00:00Z"><w:r><w:delText xml:space="preserve"> retiré</w:delText></w:r></w:del></w:p>` +
	`<w:p><w:r><w:t>Fin</w:t></w:r></w:p>`

func TestReadRevisions(t *testing.T) {
	_, tree := open(t, revised)
	flow := firstText(t, tree).Text.Delta()
	want := []struct{ text, key, author, date string }{
		{"ajout", "ins", "Alice", "2026-09-01T10:00:00Z"},
		{" retiré", "del", "Bob", "2026-09-02T10:00:00Z"},
		{"\n", "ins", "Alice", "2026-09-01T10:00:00Z"},
	}
	for i, w := range want {
		o := flow[i+1]
		if o.Insert != w.text || o.Attrs[w.key] != w.author || o.Attrs[revisionDates[w.key]] != w.date {
			t.Errorf("op %d: %q %v", i+1, o.Insert, o.Attrs)
		}
	}
	if tracking(tree) {
		t.Error("changes tracked")
	}
}

func TestAcceptAndReject(t *testing.T) {
	d, tree := open(t, revised)
	id := firstText(t, tree).ID
	cleared := func(key string) ot.Attrs { return ot.Attrs{key: "", revisionDates[key]: ""} }
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: id, Text: ot.Delta{
		{Retain: 3},
		{Retain: 5, Attrs: cleared("ins")},
		{Retain: 7, Attrs: cleared("del")},
		{Retain: 1, Attrs: cleared("ins")},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	xml := partOf(t, saved, "word/document.xml")
	if strings.Contains(xml, "w:ins") || strings.Contains(xml, "w:del") {
		t.Errorf("revisions left: %s", xml)
	}
	if !strings.Contains(xml, "<w:pPr></w:pPr><w:r><w:t>Un ajout retiré</w:t></w:r>") {
		t.Errorf("text lost: %s", xml)
	}
}

func TestTrackedChanges(t *testing.T) {
	d, tree := open(t, revised)
	id := firstText(t, tree).ID
	const date = "2026-09-29T08:30:00Z"
	mine := func(key string) ot.Attrs { return ot.Attrs{key: "Carol", revisionDates[key]: date} }
	e := ot.Edit{
		{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"track": json.RawMessage("true")}},
		{Op: ot.OpTxt, ID: id, Text: ot.Delta{
			{Retain: 3},
			{Retain: 5, Attrs: mine("del")},
			{Retain: 8},
			{Insert: "neuf\n", Attrs: mine("ins")},
			{Retain: 3, Attrs: mine("del")},
		}},
	}
	if err := d.Check(tree, e, "Carol"); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	xml := partOf(t, saved, "word/document.xml")
	for _, s := range []string{
		`<w:ins w:id="1" w:author="Alice" w:date="2026-09-01T10:00:00Z"><w:del w:author="Carol" w:date="2026-09-29T08:30:00Z" w:id="`,
		`<w:delText>ajout</w:delText>`,
		`<w:rPr><w:ins w:author="Carol" w:date="2026-09-29T08:30:00Z" w:id="`,
		`<w:ins w:author="Carol" w:date="2026-09-29T08:30:00Z" w:id="`,
		`<w:t>neuf</w:t>`,
		`<w:delText>Fin</w:delText>`,
	} {
		if !strings.Contains(xml, s) {
			t.Errorf("no %s in %s", s, xml)
		}
	}
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`<w:(?:ins|del) [^>]*w:id="(\d+)"`).FindAllStringSubmatch(xml, -1) {
		if ids[m[1]] {
			t.Errorf("id %s twice", m[1])
		}
		ids[m[1]] = true
	}
	if len(ids) != 7 {
		t.Errorf("%d revisions", len(ids))
	}
	if !strings.Contains(partOf(t, saved, "word/settings.xml"), "<w:trackRevisions/>") {
		t.Error("tracking not written")
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	if !tracking(again) {
		t.Error("tracking not read back")
	}
	if a, b := revisionsOf(firstText(t, tree).Text.Delta()), revisionsOf(firstText(t, again).Text.Delta()); a != b {
		t.Errorf("read back\n%s\nnot\n%s", b, a)
	}
}

// revisionsOf tells, character by character, who inserted and deleted
// the text of a flow, and when.
func revisionsOf(flow ot.Delta) string {
	var b strings.Builder
	for _, o := range flow {
		for _, c := range o.Insert {
			b.WriteString(string(c) + " " + o.Attrs["ins"] + o.Attrs["insd"] + " " + o.Attrs["del"] + o.Attrs["deld"] + "\n")
		}
	}
	return b.String()
}

func TestRevisionsSigned(t *testing.T) {
	d, tree := open(t, revised)
	id := firstText(t, tree).ID
	wrap := firstText(t, tree).Text.Delta()[1].Attrs["wrap"]
	for name, delta := range map[string]ot.Delta{
		"forged insertion": {{Insert: "x", Attrs: ot.Attrs{"ins": "Mallory"}}},
		"forged deletion":  {{Retain: 2, Attrs: ot.Attrs{"del": "Alice"}}},
		"date alone":       {{Retain: 2, Attrs: ot.Attrs{"deld": "2026-09-29T08:30:00Z"}}},
		"bad date":         {{Insert: "x", Attrs: ot.Attrs{"ins": "Carol", "insd": "hier"}}},
		"forged wrap":      {{Insert: "x", Attrs: ot.Attrs{"ins": "Bob", "wrap": wrap}}},
	} {
		if d.Check(tree, ot.Edit{{Op: ot.OpTxt, ID: id, Text: delta}}, "Carol") == nil {
			t.Errorf("%s accepted", name)
		}
	}
	copied := ot.Delta{{Insert: "x", Attrs: ot.Attrs{"ins": "Alice", "insd": "2026-09-01T10:00:00Z", "wrap": wrap}}}
	if err := d.Check(tree, ot.Edit{{Op: ot.OpTxt, ID: id, Text: copied}}, "Carol"); err != nil {
		t.Errorf("copied revision refused: %v", err)
	}
	if d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"track": json.RawMessage(`"oui"`)}}}, "Carol") == nil {
		t.Error("tracking set to a string")
	}
}
