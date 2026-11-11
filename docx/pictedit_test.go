package docx

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	anchoredPicture = `<w:p><w:r><w:drawing><wp:anchor xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" distT="0" distB="0" behindDoc="0" layoutInCell="1"><wp:simplePos x="0" y="0"/>` +
		`<wp:positionH relativeFrom="column"><wp:posOffset>100</wp:posOffset></wp:positionH><wp:positionV relativeFrom="paragraph"><wp:align>top</wp:align></wp:positionV>` +
		`<wp:extent cx="914400" cy="457200"/><wp:wrapSquare wrapText="bothSides" distL="114300"/><wp:docPr id="1" name="Picture 1" descr="A cat"/>` +
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:blipFill><a:blip r:embed="rId4"/><a:srcRect l="1000"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill>` +
		`<pic:spPr bwMode="auto"><a:xfrm><a:off x="0" y="0"/><a:ext cx="914400" cy="457200"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr>` +
		`</pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r></w:p>`
	inlinePicture = `<w:p><w:r><w:drawing><wp:inline xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="Picture 1" descr="A cat"/>` +
		`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:blipFill><a:blip r:embed="rId4"/></pic:blipFill><pic:spPr><a:xfrm><a:ext cx="914400" cy="457200"/></a:xfrm></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`
)

// pictureEdit is the edit that gives the first object of the body the
// picture change makes of the one it has.
func pictureEdit(t *testing.T, tree *ot.Tree, change func(p *Picture)) ot.Edit {
	t.Helper()
	n := firstText(t, tree)
	var p Picture
	if err := json.Unmarshal([]byte(n.Text.Delta()[0].Attrs["img"]), &p); err != nil {
		t.Fatal(err)
	}
	change(&p)
	data, _ := json.Marshal(p)
	return ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{{Retain: 1, Attrs: ot.Attrs{"img": string(data)}}}}}
}

func TestPictureResizeAndMove(t *testing.T) {
	d, tree := open(t, anchoredPicture)
	apply(t, d, tree, pictureEdit(t, tree, func(p *Picture) {
		p.W, p.H = 1828800, 914400
		p.Descr = "A dog"
		p.Crop = map[string]int{"l": 2000, "b": 500}
		p.Float.X, p.Float.RelX = 250000, "page"
		p.Float.AlignY, p.Float.RelY = "bottom", "margin"
		p.Float.Wrap, p.Float.WrapText = "topAndBottom", ""
		p.Float.Behind = true
	}))
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	for _, want := range []string{
		`<wp:extent cx="1828800" cy="914400"/>`,
		`<a:ext cx="1828800" cy="914400"/>`,
		`descr="A dog"`,
		`<a:srcRect l="2000" b="500"/>`,
		`<wp:positionH relativeFrom="page"><wp:posOffset>250000</wp:posOffset></wp:positionH>`,
		`<wp:positionV relativeFrom="margin"><wp:align>bottom</wp:align></wp:positionV>`,
		`<wp:wrapTopAndBottom/>`,
		`behindDoc="1"`,
		`<pic:spPr bwMode="auto">`, `<a:prstGeom prst="rect">`, `<a:off x="0" y="0"/>`, `<a:stretch>`, `layoutInCell="1"`, `name="Picture 1"`,
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("missing %s in\n%s", want, doc)
		}
	}
	if strings.Contains(doc, "wrapSquare") {
		t.Errorf("the old wrapping stayed:\n%s", doc)
	}
	_, again, err := Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	var p Picture
	json.Unmarshal([]byte(firstText(t, again).Text.Delta()[0].Attrs["img"]), &p)
	if p.W != 1828800 || p.Float.X != 250000 || p.Float.Wrap != "topAndBottom" || p.Crop["l"] != 2000 {
		t.Errorf("read back %+v %+v", p, p.Float)
	}
}

func TestPictureWrapSides(t *testing.T) {
	d, tree := open(t, anchoredPicture)
	apply(t, d, tree, pictureEdit(t, tree, func(p *Picture) { p.Float.WrapText = "largest" }))
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if doc := partOf(t, saved, "word/document.xml"); !strings.Contains(doc, `<wp:wrapSquare wrapText="largest" distL="114300"/>`) {
		t.Errorf("document\n%s", doc)
	}
}

func TestPictureCropRemoved(t *testing.T) {
	d, tree := open(t, anchoredPicture)
	apply(t, d, tree, pictureEdit(t, tree, func(p *Picture) { p.Crop = nil }))
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if doc := partOf(t, saved, "word/document.xml"); strings.Contains(doc, "srcRect") || !strings.Contains(doc, "<a:stretch>") {
		t.Errorf("document\n%s", doc)
	}
}

func TestPictureUnchanged(t *testing.T) {
	d, tree := open(t, anchoredPicture)
	apply(t, d, tree, pictureEdit(t, tree, func(p *Picture) {}))
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if doc := partOf(t, saved, "word/document.xml"); !strings.Contains(doc, `<wp:positionH relativeFrom="column"><wp:posOffset>100</wp:posOffset></wp:positionH>`) {
		t.Errorf("document\n%s", doc)
	}
}

// An edit of a picture is written, or refused: never dropped.
func TestPictureEditRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		body   string
		change func(p *Picture)
	}{
		"other media":      {anchoredPicture, func(p *Picture) { p.Media = "other" }},
		"text":             {anchoredPicture, func(p *Picture) { p.Text = ot.Delta{{Insert: "x\n"}} }},
		"fill":             {anchoredPicture, func(p *Picture) { p.Fill = "FF0000" }},
		"no size":          {anchoredPicture, func(p *Picture) { p.W = 0 }},
		"huge size":        {anchoredPicture, func(p *Picture) { p.H = 1 << 40 }},
		"crop side":        {anchoredPicture, func(p *Picture) { p.Crop = map[string]int{"x": 1} }},
		"crop range":       {anchoredPicture, func(p *Picture) { p.Crop = map[string]int{"l": 1 << 20} }},
		"reference":        {anchoredPicture, func(p *Picture) { p.Float.RelX = "somewhere" }},
		"alignment":        {anchoredPicture, func(p *Picture) { p.Float.AlignY = "middle" }},
		"offset":           {anchoredPicture, func(p *Picture) { p.Float.Y = 1 << 40 }},
		"tight wrapping":   {anchoredPicture, func(p *Picture) { p.Float.Wrap = "tight" }},
		"sides":            {anchoredPicture, func(p *Picture) { p.Float.WrapText = "up" }},
		"sides of no wrap": {anchoredPicture, func(p *Picture) { p.Float.Wrap, p.Float.WrapText = "none", "left" }},
		"floating inline":  {inlinePicture, func(p *Picture) { p.Float = &Float{RelX: "page", RelY: "page", Wrap: "square"} }},
		"inline floating":  {anchoredPicture, func(p *Picture) { p.Float = nil }},
	} {
		t.Run(name, func(t *testing.T) {
			d, tree := open(t, tc.body)
			if err := d.Check(tree, pictureEdit(t, tree, tc.change), ""); err != ErrReadOnly {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestPictureEditRefusedOutside(t *testing.T) {
	d, tree := open(t, anchoredPicture)
	n := firstText(t, tree)
	for name, delta := range map[string]ot.Delta{
		"removed": {{Retain: 1, Attrs: ot.Attrs{"img": ""}}},
		"new":     {{Insert: Object, Attrs: ot.Attrs{"img": `{"media":"x","w":10,"h":10,"crop":{"l":5}}`}}},
		"chart":   {{Insert: Object, Attrs: ot.Attrs{"img": `{"w":10,"h":10,"chartNode":"chart-1"}`}}},
	} {
		if err := d.Check(tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: delta}}, ""); err != ErrReadOnly {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestPictureInsertedResized(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>A</w:t></w:r></w:p>`)
	name, err := d.AddPicture([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDRnew"))
	if err != nil {
		t.Fatal(err)
	}
	n := firstText(t, tree)
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Insert: Object, Attrs: ot.Attrs{"img": `{"media":"` + name + `","w":914400,"h":457200}`}},
	}}})
	apply(t, d, tree, ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{
		{Retain: 1, Attrs: ot.Attrs{"img": `{"media":"` + name + `","w":100,"h":50}`}},
	}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if doc := partOf(t, saved, "word/document.xml"); !strings.Contains(doc, `<wp:extent cx="100" cy="50"/>`) {
		t.Errorf("document\n%s", doc)
	}
}

func TestPictureResizeFollowedByFallback(t *testing.T) {
	const ns = `xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	drawing := func(name string) string {
		return `<w:drawing><wp:inline><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="` + name + `"/>` +
			`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
			`<pic:blipFill><a:blip r:embed="rId4"/></pic:blipFill><pic:spPr><a:xfrm><a:ext cx="914400" cy="457200"/></a:xfrm></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing>`
	}
	d, tree := open(t, `<w:p><w:r><mc:AlternateContent `+ns+`><mc:Choice Requires="am3d">`+drawing("Choice")+`</mc:Choice><mc:Fallback>`+drawing("Fallback")+`</mc:Fallback></mc:AlternateContent></w:r></w:p>`)
	apply(t, d, tree, pictureEdit(t, tree, func(p *Picture) { p.W, p.H = 1828800, 914400 }))
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	if n := strings.Count(doc, `<wp:extent cx="1828800" cy="914400"/>`); n != 2 {
		t.Errorf("%d drawings resized of 2:\n%s", n, doc)
	}
	if n := strings.Count(doc, `<a:ext cx="1828800" cy="914400"/>`); n != 2 {
		t.Errorf("%d pictures resized of 2:\n%s", n, doc)
	}
}

func TestPictureOnPlainText(t *testing.T) {
	d, tree := open(t, `<w:p><w:r><w:t>Bonjour</w:t></w:r></w:p>`)
	n := firstText(t, tree)
	for name, tc := range map[string]struct {
		img  string
		want error
	}{
		"plain":   {`{"media":"x","w":10,"h":10}`, nil},
		"cropped": {`{"media":"x","w":10,"h":10,"crop":{"l":5}}`, ErrReadOnly},
		"floated": {`{"media":"x","w":10,"h":10,"float":{"relX":"page","relY":"page","wrap":"square"}}`, ErrReadOnly},
		"no size": {`{"media":"x"}`, ErrReadOnly},
	} {
		edit := ot.Edit{{Op: ot.OpTxt, ID: n.ID, Text: ot.Delta{{Retain: 3, Attrs: ot.Attrs{"img": tc.img}}}}}
		if err := d.Check(tree, edit, ""); err != tc.want {
			t.Errorf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestPictureResizeFollowedByVMLFallback(t *testing.T) {
	const ns = `xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:v="urn:schemas-microsoft-com:vml"`
	drawing := `<w:drawing><wp:anchor behindDoc="0"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="column"><wp:posOffset>127000</wp:posOffset></wp:positionH><wp:positionV relativeFrom="paragraph"><wp:posOffset>254000</wp:posOffset></wp:positionV>` +
		`<wp:extent cx="914400" cy="457200"/><wp:wrapSquare wrapText="bothSides"/><wp:docPr id="1" name="Chart"/>` +
		`<a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture">` +
		`<pic:blipFill><a:blip r:embed="rId4"/></pic:blipFill><pic:spPr><a:xfrm><a:ext cx="914400" cy="457200"/></a:xfrm></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing>`
	pict := `<w:pict><v:shape id="s1" style="position:absolute;margin-left:10pt;margin-top:20pt;width:72pt;height:36pt;z-index:1" type="#t"><v:imagedata r:id="rId4"/></v:shape></w:pict>`
	d, tree := open(t, `<w:p><w:r><mc:AlternateContent `+ns+`><mc:Choice Requires="am3d">`+drawing+`</mc:Choice><mc:Fallback>`+pict+`</mc:Fallback></mc:AlternateContent></w:r></w:p>`)
	apply(t, d, tree, pictureEdit(t, tree, func(p *Picture) {
		p.W, p.H = 1828800, 914400
		p.Float.X, p.Float.Y = 254000, 381000
	}))
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	doc := partOf(t, saved, "word/document.xml")
	const want = `style="position:absolute;margin-left:20pt;margin-top:30pt;width:144pt;height:72pt;z-index:1"`
	if !strings.Contains(doc, want) {
		t.Errorf("VML fallback not resized and moved:\n%s", doc)
	}
}
