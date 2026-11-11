package pptx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func framesOf(tree *ot.Tree, kind string) []*ot.Node {
	var out []*ot.Node
	var walk func(id string)
	walk = func(id string) {
		for _, n := range tree.Children(id) {
			if n.Type == "frame" && string(n.Attrs["frame"]) == `"`+kind+`"` {
				out = append(out, n)
			}
			walk(n.ID)
		}
	}
	walk("deck")
	return out
}

func TestDiagram(t *testing.T) {
	d, tree := openCorpus(t, "libreoffice/tdf132302_SmartArt_rightArrow.pptx")
	frames := framesOf(tree, "diagram")
	if len(frames) != 1 {
		t.Fatalf("%d diagrams", len(frames))
	}
	var dg drawingml.Diagram
	if err := json.Unmarshal(frames[0].Attrs["diagram"], &dg); err != nil {
		t.Fatal(err)
	}
	if len(dg.Shapes) == 0 || dg.Shapes[0].Geom == nil || dg.Shapes[0].Geom.Preset != "rightArrow" || dg.Shapes[0].Xfrm.W == 0 {
		t.Errorf("shapes %+v", dg.Shapes)
	}
	data, err := os.ReadFile("../corpus/files/libreoffice/tdf132302_SmartArt_rightArrow.pptx")
	if err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	samePackage(t, "tdf132302", data, saved)
}

func TestDiagramWithoutDrawing(t *testing.T) {
	_, tree := openCorpus(t, "libreoffice/smartart-org-chart.pptx")
	frames := framesOf(tree, "diagram")
	if len(frames) == 0 {
		t.Fatal("no diagram")
	}
	for _, f := range frames {
		if len(f.Attrs["diagram"]) > 0 {
			t.Errorf("%s: diagram %s", f.ID, f.Attrs["diagram"])
		}
	}
}

func TestPreviews(t *testing.T) {
	for _, name := range []string{"python-pptx/shp-access-ole-object.pptx", "libreoffice/ole.pptx"} {
		d, tree := openCorpus(t, name)
		frames := framesOf(tree, "ole")
		if len(frames) == 0 {
			t.Fatalf("%s: no object", name)
		}
		var blip drawingml.Blip
		if err := json.Unmarshal(frames[0].Attrs["blip"], &blip); err != nil || blip.Media == "" {
			t.Fatalf("%s: preview %s %v", name, frames[0].Attrs["blip"], err)
		}
		if data, _, err := d.Media(blip.Media); err != nil || len(data) == 0 {
			t.Errorf("%s: media %v", name, err)
		}
		data, err := os.ReadFile("../corpus/files/" + name)
		if err != nil {
			t.Fatal(err)
		}
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatal(err)
		}
		samePackage(t, name, data, saved)
	}
}

func TestFallbackPicture(t *testing.T) {
	_, tree := openCorpus(t, "libreoffice/forum-mso-de-138303.pptx")
	frames := framesOf(tree, "chart")
	if len(frames) != 1 {
		t.Fatalf("%d charts", len(frames))
	}
	var xml string
	_ = json.Unmarshal(frames[0].Attrs["xml"], &xml)
	if !strings.Contains(xml, "<mc:Fallback") || !strings.Contains(xml, "<p:pic>") {
		t.Errorf("fallback of %s: %s", frames[0].ID, xml)
	}
}

func TestExtendedFrameMoves(t *testing.T) {
	const nv = `<p:nvPr/></p:nvGraphicFramePr>`
	slide := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n" +
		`<p:sld ` + fuzzNS + ` xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006"><p:cSld><p:spTree>` + group +
		`<mc:AlternateContent><mc:Choice Requires="cx1"><p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="4" name="Chart"/><p:cNvGraphicFramePr/>` + nv +
		`<p:xfrm><a:off x="100" y="200"/><a:ext cx="3000" cy="2000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/drawing/2014/chartex"><cx:chart xmlns:cx="http://schemas.microsoft.com/office/drawing/2014/chartex" r:id="rId20"/></a:graphicData></a:graphic></p:graphicFrame></mc:Choice>` +
		`<mc:Fallback><p:sp><p:nvSpPr><p:cNvPr id="5" name="Chart"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:sp>` +
		`<p:pic><p:nvPicPr><p:cNvPr id="6" name="Chart"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill><a:blip r:embed="rId8"/></p:blipFill><p:spPr><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></p:spPr></p:pic></mc:Fallback></mc:AlternateContent>` +
		`</p:spTree></p:cSld></p:sld>`
	pkg := fuzzSkeleton(t)
	if err := pkg.Set("ppt/slides/slide1.xml", []byte(slide)); err != nil {
		t.Fatal(err)
	}
	d, tree, err := Open(stored(t, pkg), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	frames := framesOf(tree, "chart")
	if len(frames) != 1 {
		t.Fatalf("%d charts", len(frames))
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: frames[0].ID, Attrs: ot.Values{"xfrm": json.RawMessage(`{"x":500,"y":600,"w":7000,"h":4000}`)}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	out, err := opc.Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	xml, _ := out.Read("ppt/slides/slide1.xml")
	got := string(xml)
	const xfrm = `<a:off x="500" y="600"/><a:ext cx="7000" cy="4000"/>`
	if n := strings.Count(got, xfrm); n != 3 {
		t.Errorf("%d frames moved of 3:\n%s", n, got)
	}
	if n := strings.Count(got, `<p:spPr><a:xfrm>`+xfrm+`</a:xfrm><a:prstGeom`); n != 2 {
		t.Errorf("%d fallback xfrm before the geometry of 2:\n%s", n, got)
	}
}
