package drawingml

import (
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

const body = `<p:txBody xmlns:a="` + NS + `" xmlns:p="urn:p"><a:bodyPr/><a:lstStyle/>` +
	`<a:p><a:pPr lvl="1"/><a:r><a:rPr lang="fr-FR" b="1"/><a:t>Un</a:t></a:r><a:br/><a:r><a:t>deux</a:t></a:r>` +
	`<a:fld id="{1}" type="slidenum"><a:rPr lang="fr-FR"/><a:t>3</a:t></a:fld><a:endParaRPr lang="fr-FR" sz="1800"/></a:p>` +
	`<a:p><m:x xmlns:m="urn:m"/></a:p></p:txBody>`

func TestFlow(t *testing.T) {
	doc, err := xmldom.Parse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	flow := Flow(doc.Root, nil)
	var text strings.Builder
	for _, o := range flow {
		text.WriteString(o.Insert)
	}
	if got := text.String(); got != "Un\vdeux3\n"+Object+"\n" {
		t.Fatalf("text %q", got)
	}
	if flow[0].Attrs["b"] != "1" || flow[2].Attrs["fld"] == "" {
		t.Fatalf("flow %v", flow)
	}
	if mark := flow[3].Attrs; mark["lvl"] != "1" || mark["sz"] != "1800" || mark["p"] == "" {
		t.Fatalf("mark %v", mark)
	}

	// unbold the first word, add a bulleted paragraph at the end
	edited, err := ot.Compose(flow, ot.Delta{
		{Retain: 2, Attrs: ot.Attrs{"b": ""}}, {Retain: 9},
		{Insert: "trois"}, {Insert: "\n", Attrs: ot.Attrs{"bu": "char:•", "algn": "r"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	spaces := map[string]string{"a": NS}
	SetFlow(doc.Root, edited, fragments(spaces), nil)
	want := `<a:p><a:pPr lvl="1"/><a:r><a:rPr lang="fr-FR"/><a:t>Un</a:t></a:r><a:br/><a:r><a:t>deux</a:t></a:r>` +
		`<a:fld id="{1}" type="slidenum"><a:rPr lang="fr-FR"/><a:t>3</a:t></a:fld><a:endParaRPr lang="fr-FR" sz="1800"/></a:p>` +
		`<a:p><m:x xmlns:m="urn:m"/></a:p>` +
		`<a:p><a:pPr algn="r"><a:buChar char="•"/></a:pPr><a:r><a:t>trois</a:t></a:r></a:p>`
	if got := string(doc.Root.Bytes()); !strings.HasSuffix(got, want+"</p:txBody>") {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}
