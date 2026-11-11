package pptx

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/drawingml"
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
