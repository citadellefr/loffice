package docx

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const smartArtRels = `<Relationship Id="rId10" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/diagramData" Target="diagrams/data1.xml"/>` +
	`<Relationship Id="rId11" Type="http://schemas.microsoft.com/office/2007/relationships/diagramDrawing" Target="diagrams/drawing1.xml"/>`

var smartArtParts = map[string]string{
	"word/diagrams/data1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<dgm:dataModel xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dgm:ptLst/><dgm:extLst><a:ext uri="http://schemas.microsoft.com/office/drawing/2008/diagram"><dsp:dataModelExt xmlns:dsp="http://schemas.microsoft.com/office/drawing/2008/diagram" relId="rId11" minVer="http://schemas.openxmlformats.org/drawingml/2006/diagram"/></a:ext></dgm:extLst></dgm:dataModel>`,
	"word/diagrams/drawing1.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<dsp:drawing xmlns:dsp="http://schemas.microsoft.com/office/drawing/2008/diagram" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><dsp:spTree><dsp:nvGrpSpPr><dsp:cNvPr id="0" name=""/><dsp:cNvGrpSpPr/></dsp:nvGrpSpPr><dsp:grpSpPr/>` +
		`<dsp:sp modelId="{1}"><dsp:nvSpPr><dsp:cNvPr id="0" name=""/><dsp:cNvSpPr/></dsp:nvSpPr><dsp:spPr><a:xfrm><a:off x="100" y="200"/><a:ext cx="3000" cy="1000"/></a:xfrm><a:prstGeom prst="roundRect"><a:avLst/></a:prstGeom><a:solidFill><a:schemeClr val="accent1"/></a:solidFill></dsp:spPr>` +
		`<dsp:style><a:lnRef idx="2"><a:schemeClr val="lt1"/></a:lnRef><a:fillRef idx="1"><a:schemeClr val="accent1"/></a:fillRef><a:effectRef idx="0"><a:schemeClr val="accent1"/></a:effectRef><a:fontRef idx="minor"><a:schemeClr val="lt1"/></a:fontRef></dsp:style>` +
		`<dsp:txBody><a:bodyPr lIns="10"/><a:lstStyle/><a:p><a:r><a:rPr lang="fr-FR" sz="2400"/><a:t>Étape</a:t></a:r></a:p></dsp:txBody><dsp:txXfrm><a:off x="150" y="250"/><a:ext cx="2900" cy="900"/></dsp:txXfrm></dsp:sp>` +
		`</dsp:spTree></dsp:drawing>`,
}

const smartArt = `<w:p><w:r><w:drawing><wp:inline xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"><wp:extent cx="3200" cy="1300"/><wp:docPr id="1" name="Diagram 1"/>` +
	`<a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/diagram"><dgm:relIds xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram" r:dm="rId10" r:lo="rId10" r:qs="rId10" r:cs="rId10"/></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`

func pictureOf(t *testing.T, tree *ot.Tree) Picture {
	t.Helper()
	var p Picture
	for _, op := range firstText(t, tree).Text.Delta() {
		if img := op.Attrs["img"]; img != "" {
			if err := json.Unmarshal([]byte(img), &p); err != nil {
				t.Fatal(err)
			}
			return p
		}
	}
	t.Fatal("no picture")
	return p
}

func TestSmartArt(t *testing.T) {
	data := buildWith(t, smartArt, smartArtRels, smartArtParts)
	d, tree, err := Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	p := pictureOf(t, tree)
	if p.Diagram == nil || len(p.Diagram.Shapes) != 1 {
		t.Fatalf("diagram %+v", p.Diagram)
	}
	s := p.Diagram.Shapes[0]
	if s.Xfrm.X != 100 || s.Xfrm.W != 3000 || s.Geom == nil || s.Geom.Preset != "roundRect" || s.Fill == nil || s.Style == nil {
		t.Errorf("shape %+v", s)
	}
	if s.TextXfrm == nil || s.TextXfrm.X != 150 {
		t.Errorf("text box %+v", s.TextXfrm)
	}
	if got := s.Text; len(got) == 0 || got[0].Insert != "Étape" {
		t.Errorf("text %v", got)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := opc.Open(data, opc.Limits{})
	after, err := opc.Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range before.Names() {
		x, _ := before.Read(n)
		y, _ := after.Read(n)
		if !bytes.Equal(x, y) {
			t.Errorf("%s changed by a save without edit", n)
		}
	}
}

func TestSmartArtWithoutDrawing(t *testing.T) {
	parts := map[string]string{"word/diagrams/data1.xml": `<dgm:dataModel xmlns:dgm="http://schemas.openxmlformats.org/drawingml/2006/diagram"/>`}
	_, tree, err := Open(buildWith(t, smartArt, smartArtRels[:strings.Index(smartArtRels, "<Relationship Id=\"rId11\"")], parts), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if p := pictureOf(t, tree); p.Diagram != nil {
		t.Errorf("diagram %+v", p.Diagram)
	}
}

func TestAlternateFallback(t *testing.T) {
	const ns = `xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" xmlns:v="urn:schemas-microsoft-com:vml" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	_, tree := open(t, `<w:p><w:r><mc:AlternateContent `+ns+`><mc:Choice Requires="am3d"><w:drawing><wp:inline><wp:extent cx="914400" cy="457200"/><wp:docPr id="1" name="3D"/><a:graphic><a:graphicData uri="urn:model"/></a:graphic></wp:inline></w:drawing></mc:Choice>`+
		`<mc:Fallback><w:pict><v:shape style="width:72pt;height:36pt"><v:imagedata r:id="rId4"/></v:shape></w:pict></mc:Fallback></mc:AlternateContent></w:r></w:p>`)
	if p := pictureOf(t, tree); p.Media == "" || p.W != 914400 {
		t.Errorf("fallback not read: %+v", p)
	}
}
