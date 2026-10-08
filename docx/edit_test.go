package docx

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

// build is a Word document of that body, with a header, a picture and
// footnotes.
func build(t *testing.T, body string) []byte {
	t.Helper()
	const ns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
		`xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"`
	parts := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Default Extension="png" ContentType="image/png"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/><Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/><Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/><Override PartName="/word/footnotes.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footnotes+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/_rels/document.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.com/" TargetMode="External"/><Relationship Id="rId4" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/image1.png"/><Relationship Id="rId6" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes" Target="footnotes.xml"/></Relationships>`,
		"word/footnotes.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:footnotes ` + ns + `><w:footnote w:type="separator" w:id="-1"><w:p><w:r><w:separator/></w:r></w:p></w:footnote><w:footnote w:id="1"><w:p><w:r><w:footnoteRef/></w:r><w:r><w:t xml:space="preserve"> Source.</w:t></w:r></w:p></w:footnote></w:footnotes>`,
		"word/styles.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles ` + ns + `><w:docDefaults><w:rPrDefault><w:rPr><w:sz w:val="22"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="160" w:line="259" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/></w:style><w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:pPr><w:keepNext/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style></w:styles>`,
		"word/header1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:hdr ` + ns + `><w:p><w:r><w:t>Header</w:t></w:r></w:p></w:hdr>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document ` + ns + `><w:body>` + body + `<w:sectPr><w:headerReference w:type="default" r:id="rId2"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1417" w:right="1417" w:bottom="1417" w:left="1417" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`,
		"word/media/image1.png": "\x89PNG\r\n\x1a\n",
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "word/_rels/document.xml.rels", "word/styles.xml", "word/header1.xml", "word/footnotes.xml", "word/document.xml", "word/media/image1.png"} {
		f, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte(parts[name]))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func open(t *testing.T, body string) (*Document, *ot.Tree) {
	t.Helper()
	d, tree, err := Open(build(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return d, tree
}

// part is a part of a saved document.
func partOf(t *testing.T, data []byte, name string) string {
	t.Helper()
	pkg, err := opc.Open(data, Limits)
	if err != nil {
		t.Fatal(err)
	}
	b, err := pkg.Read(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func apply(t *testing.T, d *Document, tree *ot.Tree, e ot.Edit) {
	t.Helper()
	if err := d.Check(tree, e, ""); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
}

// text is the first flow of the body.
func firstText(t *testing.T, tree *ot.Tree) *ot.Node {
	for _, n := range tree.Children("body") {
		if n.Type == "text" {
			return n
		}
	}
	t.Fatal("no text")
	return nil
}

func TestReadFlow(t *testing.T) {
	_, tree := open(t, `<w:p w14:paraId="1A2B3C4D"><w:pPr><w:pStyle w:val="Heading1"/><w:jc w:val="center"/><w:ind w:left="720" w:hanging="360"/></w:pPr>`+
		`<w:r><w:rPr><w:b/><w:sz w:val="28"/></w:rPr><w:t xml:space="preserve">Bold </w:t></w:r>`+
		`<w:proofErr w:type="spellStart"/><w:r><w:t>a</w:t><w:tab/><w:t>b</w:t><w:br/><w:br w:type="page"/></w:r>`+
		`<w:hyperlink r:id="rId3"><w:r><w:t>link</w:t></w:r></w:hyperlink><w:bookmarkStart w:id="0" w:name="here"/><w:bookmarkEnd w:id="0"/>`+
		`<w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> PAGE </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>1</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r>`+
		`</w:p><w:p/>`)
	flow := firstText(t, tree).Text.Delta()
	var s strings.Builder
	for _, o := range flow {
		s.WriteString(o.Insert)
	}
	if got, want := s.String(), "Bold a\tb\v"+Object+"link"+Object+Object+Object+Object+Object+"1"+Object+"\n\n"; got != want {
		t.Fatalf("flow %q, want %q", got, want)
	}
	if a := flow[0].Attrs; a["b"] != "1" || a["sz"] != "28" || a["r"] == "" {
		t.Errorf("bold run %v", a)
	}
	find := func(pred func(ot.Op) bool) ot.Op {
		for _, o := range flow {
			if pred(o) {
				return o
			}
		}
		t.Fatal("not found")
		return ot.Op{}
	}
	if o := find(func(o ot.Op) bool { return o.Insert == "link" }); o.Attrs["link"] != "https://example.com/" || o.Attrs["wrap"] == "" {
		t.Errorf("link %v", o.Attrs)
	}
	if o := find(func(o ot.Op) bool { return o.Attrs["br"] != "" }); o.Attrs["br"] != "page" || o.Attrs["o"] == "" {
		t.Errorf("page break %v", o.Attrs)
	}
	if o := find(func(o ot.Op) bool { return o.Attrs["instr"] != "" }); o.Attrs["instr"] != " PAGE " {
		t.Errorf("field %v", o.Attrs)
	}
	if o := find(func(o ot.Op) bool { return o.Attrs["bm"] != "" }); o.Attrs["bm"] != "here" || o.Attrs["po"] == "" {
		t.Errorf("bookmark %v", o.Attrs)
	}
	mark := find(func(o ot.Op) bool { return strings.HasPrefix(o.Insert, "\n") }).Attrs
	if mark["pstyle"] != "Heading1" || mark["jc"] != "center" || mark["ind.left"] != "720" || mark["ind.first"] != "-360" || mark["pa"] == "" {
		t.Errorf("mark %v", mark)
	}

	var styles map[string]Style
	if err := json.Unmarshal(tree.Node("doc").Attrs["styles"], &styles); err != nil {
		t.Fatal(err)
	}
	if h := styles["Heading1"]; h.BasedOn != "Normal" || h.R["b"] != "1" || h.P["keepNext"] != "1" || h.P["outline"] != "0" {
		t.Errorf("heading style %+v", h)
	}
	var sect Section
	if err := json.Unmarshal(tree.Node("doc").Attrs["sect"], &sect); err != nil {
		t.Fatal(err)
	}
	if sect.W != 11906 || sect.Left != 1417 || sect.Hdr["default"] == "" || tree.Node(sect.Hdr["default"]).Type != "hdr" {
		t.Errorf("section %+v", sect)
	}
}

func TestUntouchedIsSame(t *testing.T) {
	data := build(t, `<w:p><w:r><w:t>Hello</w:t></w:r></w:p>`)
	d, tree, err := Open(data)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, data) {
		samePackage(t, "built", data, saved)
	}
}

func TestEditText(t *testing.T) {
	d, tree := open(t, `<w:p w14:paraId="1A2B3C4D"><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:i/></w:rPr><w:t>Hello world</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Retain: 5, Attrs: ot.Attrs{"b": "1"}},
		{Insert: "\n", Attrs: n.Text.Delta()[1].Attrs},
		{Retain: 1},
		{Insert: "big ", Attrs: ot.Attrs{"sz": "48", "color": "FF0000"}},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	for _, want := range []string{
		`<w:r><w:rPr><w:b/><w:bCs/><w:i/></w:rPr><w:t>Hello</w:t></w:r></w:p>`,
		`<w:r><w:rPr><w:color w:val="FF0000"/><w:sz w:val="48"/><w:szCs w:val="48"/></w:rPr><w:t xml:space="preserve">big </w:t></w:r>`,
		`<w:t>world</w:t>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
	if strings.Count(doc, "w14:paraId") != 1 || strings.Count(doc, `<w:jc w:val="center"/>`) != 2 {
		t.Errorf("paragraphs split wrong:\n%s", doc)
	}
	if partOf(t, saved, "word/header1.xml") != partOf(t, build(t, ""), "word/header1.xml") {
		t.Error("the header was rewritten")
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	if got := firstText(t, again).Text.Delta()[0]; got.Insert != "Hello" || got.Attrs["b"] != "1" || got.Attrs["i"] != "1" {
		t.Errorf("read back %v", got)
	}
}

func TestEditParagraph(t *testing.T) {
	d, tree := open(t, `<w:p><w:pPr><w:pStyle w:val="Heading1"/><w:spacing w:before="240" w:beforeLines="100"/></w:pPr><w:r><w:t>Title</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Retain: 5},
		{Retain: 1, Attrs: ot.Attrs{"pstyle": "", "sp.before": "120", "ind.first": "-283", "num": "3", "lvl": "1", "tabs": "left:720 right:9000:dot"}},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	want := `<w:pPr><w:numPr><w:ilvl w:val="1"/><w:numId w:val="3"/></w:numPr><w:tabs><w:tab w:val="left" w:pos="720"/><w:tab w:val="right" w:leader="dot" w:pos="9000"/></w:tabs><w:spacing w:before="120"/><w:ind w:hanging="283"/></w:pPr>`
	if doc := partOf(t, saved, "word/document.xml"); !strings.Contains(doc, want) {
		t.Errorf("want %s in\n%s", want, doc)
	}
}

func TestEditSection(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	var sect Section
	json.Unmarshal(tree.Node("doc").Attrs["sect"], &sect)
	sect.W, sect.H, sect.Orient = sect.H, sect.W, "landscape"
	sect.Left, sect.Cols = 720, 2
	data, _ := json.Marshal(sect)
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"sect": data}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	for _, want := range []string{
		`<w:headerReference w:type="default" r:id="rId2"/>`,
		`<w:pgSz w:w="16838" w:h="11906" w:orient="landscape"/>`,
		`w:left="720"`,
		`<w:cols w:num="2" w:space="720"/>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
}

func TestNewHeader(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	var sect Section
	json.Unmarshal(tree.Node("doc").Attrs["sect"], &sect)
	sect.Ftr = map[string]string{"default": "foot"}
	data, _ := json.Marshal(sect)
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpNew, ID: "foot", Type: "ftr", Parent: "doc", Key: "z"},
		{Op: ot.OpNew, ID: "foot-1", Type: "text", Parent: "foot", Key: "V", Text: ot.Delta{{Insert: "Page\n"}}},
		{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"sect": data}},
	})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if footer := partOf(t, saved, "word/footer1.xml"); !strings.Contains(footer, `<w:ftr xmlns:w=`) || !strings.Contains(footer, "<w:t>Page</w:t>") {
		t.Errorf("footer\n%s", footer)
	}
	if rels := partOf(t, saved, "word/_rels/document.xml.rels"); !strings.Contains(rels, `relationships/footer" Target="footer1.xml"`) {
		t.Errorf("rels\n%s", rels)
	}
	if doc := partOf(t, saved, "word/document.xml"); !strings.Contains(doc, `<w:footerReference w:type="default" r:id="rId7"/>`) {
		t.Errorf("document\n%s", doc)
	}
	if types := partOf(t, saved, "[Content_Types].xml"); !strings.Contains(types, `/word/footer1.xml`) {
		t.Errorf("content types\n%s", types)
	}
	if _, again, err := Open(saved); err != nil || again.Node("f1") == nil {
		t.Fatalf("read back: %v", err)
	}
}

func TestEditTable(t *testing.T) {
	d, tree := open(t, `<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr><w:tblGrid><w:gridCol w:w="4000"/></w:tblGrid>`+
		`<w:tr><w:tc><w:tcPr><w:tcW w:w="4000" w:type="dxa"/></w:tcPr><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p/>`)
	var tbl, tr, tc *ot.Node
	for _, n := range tree.Children("body") {
		if n.Type == "tbl" {
			tbl = n
		}
	}
	tr = tree.Children(tbl.ID)[0]
	tc = tree.Children(tr.ID)[0]
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpSet, ID: tbl.ID, Attrs: ot.Values{"jc": json.RawMessage(`"center"`), "grid": json.RawMessage(`[3000,3000]`)}},
		{Op: ot.OpSet, ID: tc.ID, Attrs: ot.Values{"shd": json.RawMessage(`"FFFF00"`), "w": json.RawMessage(`{"w":3000,"type":"dxa"}`)}},
		{Op: ot.OpNew, ID: "c2", Type: "tc", Parent: tr.ID, Key: ot.KeyBetween(tc.Key, "")},
		{Op: ot.OpNew, ID: "c2-t", Type: "text", Parent: "c2", Key: "V", Text: ot.Delta{{Insert: "B1\n"}}},
		{Op: ot.OpNew, ID: "r2", Type: "tr", Parent: tbl.ID, Key: ot.KeyBetween(tr.Key, "")},
		{Op: ot.OpNew, ID: "r2c1", Type: "tc", Parent: "r2", Key: "V"},
	})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	for _, want := range []string{
		`<w:tblW w:w="0" w:type="auto"/><w:jc w:val="center"/></w:tblPr><w:tblGrid><w:gridCol w:w="3000"/><w:gridCol w:w="3000"/></w:tblGrid>`,
		`<w:tcPr><w:tcW w:w="3000" w:type="dxa"/><w:shd w:val="clear" w:color="auto" w:fill="FFFF00"/></w:tcPr>`,
		`<w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc></w:tr><w:tr><w:tc><w:p/></w:tc></w:tr></w:tbl>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
}

func TestCheck(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	for _, e := range []ot.Edit{
		{{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"styles": json.RawMessage(`{}`)}}},
		{{Op: ot.OpDel, ID: "body"}},
		{{Op: ot.OpNew, ID: "x", Type: "tr", Parent: "body", Key: "z"}},
		{{Op: ot.OpNew, ID: "x", Type: "text", Parent: n.ID, Key: "z", Text: ot.Delta{{Insert: "\n"}}}},
		{{Op: ot.OpNew, ID: "x", Type: "tbl", Parent: "body", Key: "z", Text: ot.Delta{{Insert: "\n"}}}},
		{{Op: ot.OpTxt, ID: "body", Text: ot.Delta{{Insert: "x"}}}},
	} {
		if err := d.Check(tree, e, ""); err == nil {
			t.Errorf("%v allowed", e)
		}
	}
	for _, e := range []ot.Edit{
		{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{{Insert: "x"}}}},
		{{Op: ot.OpNew, ID: "x", Type: "text", Parent: "body", Key: "z", Text: ot.Delta{{Insert: "\n"}}}},
		{{Op: ot.OpSet, ID: n.ID, Key: "a"}},
		{{Op: ot.OpDel, ID: n.ID}},
	} {
		if err := d.Check(tree, e, ""); err != nil {
			t.Errorf("%v refused: %v", e, err)
		}
	}
}

// XML a client makes up is not written: only what the document read.
func TestForgedXML(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{
		{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
			{Insert: Object, Attrs: ot.Attrs{"o": `<w:t>forged</w:t>`}},
			{Insert: "B", Attrs: ot.Attrs{"r": `<w:rPr><w:vanish/></w:rPr>`, "wrap": `["<w:hyperlink r:id=\"rId9\"/>"]`}},
			{Retain: 1, Attrs: ot.Attrs{"p": `<w:pPr><w:pStyle w:val="Forged"/></w:pPr>`}},
		}},
		{Op: ot.OpNew, ID: "o", Type: "other", Parent: "body", Key: "z", Attrs: ot.Values{"xml": json.RawMessage(`"<w:p><w:r><w:t>forged</w:t></w:r></w:p>"`)}},
	})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	if strings.Contains(doc, "forged") || strings.Contains(doc, "Forged") || strings.Contains(doc, "vanish") || strings.Contains(doc, "hyperlink") {
		t.Errorf("forged XML written:\n%s", doc)
	}
}

func TestMedia(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:drawing><wp:inline xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="Picture 1" descr="A cat"/>`+
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:blipFill><a:blip r:embed="rId4"/></pic:blipFill></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`)
	var p Picture
	if err := json.Unmarshal([]byte(firstText(t, tree).Text.Delta()[0].Attrs["img"]), &p); err != nil {
		t.Fatal(err)
	}
	if p.W != 914400 || p.H != 457200 || p.Descr != "A cat" || p.Media == "" {
		t.Fatalf("picture %+v", p)
	}
	data, typ, err := d.Media(p.Media)
	if err != nil || typ != "image/png" || !bytes.HasPrefix(data, []byte("\x89PNG")) {
		t.Fatalf("media %q %v", typ, err)
	}
	if _, _, err := d.Media("nope"); err == nil {
		t.Error("unknown media served")
	}
}

// What a client asks for by keys, without XML: breaks, fields and links
// the writer makes.
func TestClientObjects(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Insert: Object, Attrs: ot.Attrs{"br": "page"}},
		{Insert: "1", Attrs: ot.Attrs{"field": "PAGE", "b": "1"}},
		{Insert: "site", Attrs: ot.Attrs{"link": "https://example.org/a?b=1"}},
		{Insert: "ici", Attrs: ot.Attrs{"link": "#_Toc1"}},
		{Insert: "x", Attrs: ot.Attrs{"field": `INCLUDETEXT "c:\\secret"`}},
		{Insert: "y", Attrs: ot.Attrs{"link": "javascript:alert(1)"}},
		{Insert: "@Devis", Attrs: ot.Attrs{"link": "urn:citadelle:todo?id=7"}},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	for _, want := range []string{
		`<w:r><w:br w:type="page"/></w:r>`,
		`<w:fldSimple w:instr=" PAGE "><w:r><w:rPr><w:b/><w:bCs/></w:rPr><w:t>1</w:t></w:r></w:fldSimple>`,
		`<w:hyperlink r:id="rId7"><w:r><w:t>site</w:t></w:r></w:hyperlink>`,
		`<w:hyperlink w:anchor="_Toc1"><w:r><w:t>ici</w:t></w:r></w:hyperlink>`,
		`<w:r><w:t>xy</w:t></w:r>`,
		`<w:hyperlink r:id="rId8"><w:r><w:t>@Devis</w:t></w:r></w:hyperlink>`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
	if rels := partOf(t, saved, "word/_rels/document.xml.rels"); !strings.Contains(rels, `Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="https://example.org/a?b=1" TargetMode="External"`) {
		t.Errorf("rels\n%s", rels)
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	flow := firstText(t, again).Text.Delta()
	if flow[0].Attrs["br"] != "page" || flow[1].Attrs["field"] != "PAGE" || flow[2].Attrs["link"] != "https://example.org/a?b=1" {
		t.Errorf("read back %v", flow)
	}
}

func TestBuiltinStylesAndLists(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	var styles map[string]Style
	json.Unmarshal(tree.Node("doc").Attrs["styles"], &styles)
	if !styles["Title"].Builtin || styles["Heading1"].Builtin {
		t.Fatalf("builtin styles: Title %+v, Heading1 %+v", styles["Title"], styles["Heading1"])
	}
	var lists map[string]Numbering
	json.Unmarshal(tree.Node("doc").Attrs["numbering"], &lists)
	if lists["bullet"].Levels[0].Fmt != "bullet" || lists["decimal"].Levels[1].Fmt != "lowerLetter" {
		t.Fatalf("builtin lists %+v", lists)
	}
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Insert: "Titre\nUn", Attrs: nil},
		{Insert: "\n", Attrs: ot.Attrs{"num": "bullet", "lvl": "0"}},
		{Insert: "Deux", Attrs: nil},
		{Insert: "\n", Attrs: ot.Attrs{"num": "bullet", "lvl": "1"}},
		{Retain: 1},
		{Retain: 1, Attrs: ot.Attrs{"pstyle": "Title"}},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	if strings.Count(doc, `<w:numId w:val="1"/>`) != 2 || !strings.Contains(doc, `<w:pStyle w:val="Title"/>`) {
		t.Errorf("document\n%s", doc)
	}
	numbering := partOf(t, saved, "word/numbering.xml")
	if !strings.Contains(numbering, `<w:abstractNum w:abstractNumId="0">`) || !strings.Contains(numbering, `<w:num w:numId="1"><w:abstractNumId w:val="0"/></w:num>`) {
		t.Errorf("numbering\n%s", numbering)
	}
	if !strings.Contains(partOf(t, saved, "word/styles.xml"), `w:styleId="Title"`) {
		t.Error("the style was not written")
	}
	if !strings.Contains(partOf(t, saved, "word/_rels/document.xml.rels"), `relationships/numbering" Target="numbering.xml"`) {
		t.Error("no relationship to the numbering")
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	json.Unmarshal(again.Node("doc").Attrs["styles"], &styles)
	if styles["Title"].Builtin {
		t.Error("the style written is still builtin")
	}
}

func TestFootnotes(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>Texte</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r></w:p>`)
	note := tree.Node("fn1")
	if note == nil || note.Type != "note" || string(note.Attrs["kind"]) != `"footnote"` {
		t.Fatalf("note %+v", note)
	}
	if tree.Node("fn-1") != nil {
		t.Fatal("the separator was read as a note")
	}
	text := tree.Children("fn1")[0]
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: text.ID, Text: ot.Delta{{Retain: 1}, {Insert: " Voir"}}}})
	if err := d.Check(tree, ot.Edit{{Op: ot.OpDel, ID: "fn1"}}, ""); err == nil {
		t.Error("a note deleted")
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	notes := partOf(t, saved, "word/footnotes.xml")
	if !strings.Contains(notes, `<w:t xml:space="preserve"> Voir Source.</w:t>`) || !strings.Contains(notes, `<w:separator/>`) {
		t.Errorf("footnotes\n%s", notes)
	}
	if partOf(t, saved, "word/document.xml") != partOf(t, build(t, `<w:p><w:r><w:t>Texte</w:t></w:r><w:r><w:footnoteReference w:id="1"/></w:r></w:p>`), "word/document.xml") {
		t.Error("the document was rewritten")
	}
}

func TestAddPicture(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRnew")
	name, err := d.AddPicture(png)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := d.AddPicture(png); again != name {
		t.Errorf("the same picture named %q then %q", name, again)
	}
	if existing, _ := d.AddPicture([]byte("\x89PNG\r\n\x1a\n")); existing == name {
		t.Error("a picture of the document named like a new one")
	}
	if _, err := d.AddPicture([]byte("<svg/>")); err == nil {
		t.Error("an SVG file added")
	}
	if data, typ, err := d.Media(name); err != nil || typ != "image/png" || string(data) != string(png) {
		t.Fatalf("media before saving: %q %v", typ, err)
	}
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Insert: Object, Attrs: ot.Attrs{"img": `{"media":"` + name + `","w":914400,"h":457200}`}},
		{Insert: Object, Attrs: ot.Attrs{"img": `{"media":"nothing","w":914400,"h":457200}`}},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	if strings.Count(doc, "<w:drawing>") != 1 || !strings.Contains(doc, `<wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="Image 1"/>`) || !strings.Contains(doc, `<a:blip r:embed="rId7"/>`) {
		t.Errorf("document\n%s", doc)
	}
	if partOf(t, saved, "word/media/bref1.png") != string(png) {
		t.Error("the picture was not written")
	}
	if !strings.Contains(partOf(t, saved, "word/_rels/document.xml.rels"), `Id="rId7" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/bref1.png"`) {
		t.Error("no relationship to the picture")
	}
	_, again, err := Open(saved)
	if err != nil {
		t.Fatal(err)
	}
	var p Picture
	if json.Unmarshal([]byte(firstText(t, again).Text.Delta()[0].Attrs["img"]), &p); p.Media != name || p.W != 914400 {
		t.Errorf("read back %+v", p)
	}
}
