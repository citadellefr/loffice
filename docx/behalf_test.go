package docx

import (
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func TestBehalfInSettings(t *testing.T) {
	const settings = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="` + NS + `"><w:zoom w:percent="100"/><w:compat/><w:docVars><w:docVar w:name="mine" w:val="x"/></w:docVars><w:rsids/></w:settings>`
	rels := `<Relationship Id="rId9" Type="` + relSettings + `" Target="settings.xml"/>`
	d, tree, err := Open(buildWith(t, `<w:p><w:r><w:t>Texte</w:t></w:r></w:p>`, rels, map[string]string{"word/settings.xml": settings}), opc.Limits{})
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
