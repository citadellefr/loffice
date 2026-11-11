package docx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func TestBehalfInSettings(t *testing.T) {
	const settings = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="` + NS + `"><w:zoom w:percent="100"/><w:compat/><w:docVars><w:docVar w:name="mine" w:val="x"/></w:docVars><w:rsids/></w:settings>`
	rels := `<Relationship Id="rId9" Type="` + relSettings + `" Target="settings.xml"/>`
	d, tree, err := Open(buildWith(t, `<w:p><w:ins w:id="1" w:author="Héraut" w:date="2026-11-03T08:00:00Z"><w:r><w:t>Texte</w:t></w:r></w:ins></w:p>`, rels, map[string]string{"word/settings.xml": settings}), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	const date = "2026-11-03T09:00:00+01:00"
	id, attrs, ok := BehalfNode("Héraut", date, "u1")
	if !ok {
		t.Fatal("no node")
	}
	e := ot.Edit{{Op: ot.OpNew, ID: id, Type: BehalfType, Parent: "doc", Key: "~", Attrs: attrs}}
	if err := d.Check(tree, e, "Héraut"); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	xml := partOf(t, saved, "word/settings.xml")
	i, j, k := strings.Index(xml, "<w:compat/>"), strings.Index(xml, "<w:docVars>"), strings.Index(xml, "<w:rsids/>")
	if !(0 < i && i < j && j < k) || !strings.Contains(xml, `w:name="mine"`) || strings.Count(xml, "loffice.behalf.") != 1 {
		t.Fatalf("settings.xml\n%s", xml)
	}

	d, again, err := Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := BehalfOf(again, "Héraut", "2026-11-03T08:00:00Z"); got != "u1" {
		t.Errorf("read back: %q", got)
	}
	if got := BehalfOf(again, "Héraut", "2026-11-03T08:00:01Z"); got != "" {
		t.Errorf("another date: %q", got)
	}
	if got := BehalfOf(again, "Carol", "2026-11-03T08:00:00Z"); got != "" {
		t.Errorf("another author: %q", got)
	}
	if err := again.Apply(ot.Edit{{Op: ot.OpDel, ID: id}}); err != nil {
		t.Fatal(err)
	}
	saved, err = d.Save(again)
	if err != nil {
		t.Fatal(err)
	}
	if xml := partOf(t, saved, "word/settings.xml"); strings.Contains(xml, "loffice.behalf.") || !strings.Contains(xml, `w:name="mine"`) {
		t.Fatalf("settings.xml\n%s", xml)
	}
}

func TestBehalfOfComparesAuthorAndDate(t *testing.T) {
	_, tree := open(t, `<w:p><w:r><w:t>Texte</w:t></w:r></w:p>`)
	const date = "2026-11-03T08:00:00Z"
	id, attrs, _ := BehalfNode("Héraut", date, "u1")
	other := ot.Values{"author": quote("Carol"), "date": attrs["date"], "user": quote("u2")}
	if err := tree.Apply(ot.Edit{{Op: ot.OpNew, ID: id, Type: BehalfType, Parent: "doc", Key: "~", Attrs: other}}); err != nil {
		t.Fatal(err)
	}
	if got := BehalfOf(tree, "Héraut", date); got != "" {
		t.Errorf("the node of another author with the same id: %q", got)
	}
	if err := tree.Apply(ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{"author": quote("Héraut"), "date": quote("2026-11-03T08:00:01Z")}}}); err != nil {
		t.Fatal(err)
	}
	if got := BehalfOf(tree, "Héraut", date); got != "" {
		t.Errorf("the node of another date with the same id: %q", got)
	}
	if err := tree.Apply(ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{"date": attrs["date"]}}}); err != nil {
		t.Fatal(err)
	}
	if got := BehalfOf(tree, "Héraut", date); got != "u2" {
		t.Errorf("the node of the author and date: %q", got)
	}
}

func TestBehalfVariablesOfGoneRevisionsAreRemoved(t *testing.T) {
	const first, second = "2026-11-03T08:00:00Z", "2026-11-03T08:05:00Z"
	var vars strings.Builder
	vars.WriteString(`<w:docVar w:name="mine" w:val="x"/>`)
	for i, date := range []string{first, second} {
		id, attrs, _ := BehalfNode("Héraut", date, "u"+string(rune('1'+i)))
		val, _ := json.Marshal(attrs)
		vars.WriteString(`<w:docVar w:name="` + behalfPrefix + id + `" w:val="` + strings.ReplaceAll(string(val), `"`, "&quot;") + `"/>`)
	}
	vars.WriteString(`<w:docVar w:name="other.tool" w:val="y"/>`)
	settings := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="` + NS + `"><w:docVars>` + vars.String() + `</w:docVars></w:settings>`
	rels := `<Relationship Id="rId9" Type="` + relSettings + `" Target="settings.xml"/>`
	body := `<w:p><w:ins w:id="1" w:author="Héraut" w:date="` + first + `"><w:r><w:t>Un</w:t></w:r></w:ins>` +
		`<w:ins w:id="2" w:author="Héraut" w:date="` + second + `"><w:r><w:t>Deux</w:t></w:r></w:ins></w:p>`
	d, tree, err := Open(buildWith(t, body, rels, map[string]string{"word/settings.xml": settings}), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if got := partOf(t, saved, "word/settings.xml"); strings.Count(got, behalfPrefix) != 2 {
		t.Fatalf("both lots have revisions:\n%s", got)
	}

	id := firstText(t, tree).ID
	cleared := ot.Attrs{"ins": "", "insd": ""}
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: id, Text: ot.Delta{{Retain: 2, Attrs: cleared}}}})
	saved, err = d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	got := partOf(t, saved, "word/settings.xml")
	i, _, _ := BehalfNode("Héraut", first, "u1")
	j, _, _ := BehalfNode("Héraut", second, "u2")
	if strings.Contains(got, i) || !strings.Contains(got, j) || strings.Count(got, behalfPrefix) != 1 {
		t.Fatalf("the first lot is accepted:\n%s", got)
	}
	for _, v := range []string{`<w:docVar w:name="mine" w:val="x"/>`, `<w:docVar w:name="other.tool" w:val="y"/>`} {
		if !strings.Contains(got, v) {
			t.Errorf("%s lost:\n%s", v, got)
		}
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: id, Text: ot.Delta{{Retain: 2}, {Delete: 4}}}})
	saved, err = d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if got := partOf(t, saved, "word/settings.xml"); strings.Contains(got, behalfPrefix) || !strings.Contains(got, `<w:docVars><w:docVar w:name="mine" w:val="x"/><w:docVar w:name="other.tool" w:val="y"/></w:docVars>`) {
		t.Fatalf("the second lot is rejected:\n%s", got)
	}
}
