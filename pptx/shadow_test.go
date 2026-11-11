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

const effectLst = `<a:effectLst><a:outerShdw blurRad="50800" dist="38100" dir="2700000" algn="tl" rotWithShape="0"><a:srgbClr val="000000"><a:alpha val="40000"/></a:srgbClr></a:outerShdw></a:effectLst>`

func withShadow(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/pptx/transitional.pptx")
	if err != nil {
		t.Fatal(err)
	}
	parts := rawParts(t, data)
	slide := string(parts["ppt/slides/slide1.xml"])
	if !strings.Contains(slide, "</a:solidFill></p:spPr>") {
		t.Fatal("fixture changed")
	}
	parts["ppt/slides/slide1.xml"] = []byte(strings.Replace(slide, "</a:solidFill></p:spPr>", "</a:solidFill>"+effectLst+"</p:spPr>", 1))
	return zipOf(t, parts)
}

func TestShadow(t *testing.T) {
	d, tree, err := Open(withShadow(t), opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	shapes := tree.Children(slidesOf(tree)[0].ID)
	var title *ot.Node
	for _, n := range shapes {
		if n.Type == "sp" {
			title = n
		}
	}
	var s drawingml.Shadow
	if err := json.Unmarshal(title.Attrs["shadow"], &s); err != nil {
		t.Fatalf("shadow %s: %v", title.Attrs["shadow"], err)
	}
	if s.Blur != 50800 || s.Dist != 38100 || s.Dir != 2700000 || s.Color == nil || s.Color.RGB != "000000" || len(s.Color.Mods) != 1 || s.Color.Mods[0].Name != "alpha" || *s.Color.Mods[0].Value != 40000 {
		t.Errorf("shadow %+v", s)
	}

	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: title.ID, Attrs: ot.Values{"name": json.RawMessage(`"Ombre"`)}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if slide := string(rawParts(t, saved)["ppt/slides/slide1.xml"]); !strings.Contains(slide, effectLst) {
		t.Errorf("effectLst lost: %s", slide)
	}
}

func TestShadowRoundTrip(t *testing.T) {
	data := withShadow(t)
	d, tree, err := Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	samePackage(t, "shadow", data, saved)
}
