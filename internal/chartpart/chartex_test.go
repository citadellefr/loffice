package chartpart_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/internal/chartpart"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	chartExRel  = "http://schemas.microsoft.com/office/2014/relationships/chartEx"
	chartExType = "application/vnd.ms-office.chartex+xml"
	chartExNS   = `xmlns:cx="http://schemas.microsoft.com/office/drawing/2014/chartex"`
	mcNS        = "http://schemas.openxmlformats.org/markup-compatibility/2006"
	relNS       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
)

var extended = map[string]string{
	"waterfall": "waterfall", "histogram": "histogram", "boxwhisker": "boxWhisker", "funnel": "funnel",
	"treemap": "treemap", "sunburst": "sunburst", "map": "regionMap",
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("../../chart/testdata/" + name + ".xml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func template(t *testing.T, name string) *opc.Package {
	t.Helper()
	data, err := os.ReadFile("../../testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func relate(t *testing.T, p *opc.Package, source, target string) {
	t.Helper()
	rels, err := p.Relationships(source)
	if err != nil {
		t.Fatal(err)
	}
	rels = append(rels, opc.Relationship{ID: "rIdEx", Type: chartExRel, Target: target})
	if err := p.Set(opc.RelsName(source), opc.MarshalRelationships(rels)); err != nil {
		t.Fatal(err)
	}
}

func pptxWith(t *testing.T, kind string) []byte {
	t.Helper()
	p := template(t, "pptx/transitional.pptx")
	if err := p.Add("ppt/charts/chartEx1.xml", chartExType, fixture(t, kind)); err != nil {
		t.Fatal(err)
	}
	relate(t, p, "ppt/slides/slide1.xml", "../charts/chartEx1.xml")
	slide, _ := p.Read("ppt/slides/slide1.xml")
	frame := `<mc:AlternateContent xmlns:mc="` + mcNS + `"><mc:Choice xmlns:cx1="http://schemas.microsoft.com/office/drawing/2015/9/8/chartex" Requires="cx1">` +
		`<p:graphicFrame><p:nvGraphicFramePr><p:cNvPr id="5" name="Graphique 1"/><p:cNvGraphicFramePr/><p:nvPr/></p:nvGraphicFramePr>` +
		`<p:xfrm><a:off x="100" y="200"/><a:ext cx="3000" cy="2000"/></p:xfrm><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/drawing/2014/chartex">` +
		`<cx:chart ` + chartExNS + ` r:id="rIdEx"/></a:graphicData></a:graphic></p:graphicFrame></mc:Choice>` +
		`<mc:Fallback><p:pic><p:nvPicPr><p:cNvPr id="5" name="Graphique 1"/><p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:blipFill/>` +
		`<p:spPr><a:xfrm><a:off x="100" y="200"/><a:ext cx="3000" cy="2000"/></a:xfrm></p:spPr></p:pic></mc:Fallback></mc:AlternateContent>`
	slide = []byte(strings.Replace(string(slide), "</p:spTree>", frame+"</p:spTree>", 1))
	if err := p.Set("ppt/slides/slide1.xml", slide); err != nil {
		t.Fatal(err)
	}
	return bytesOf(t, p)
}

func docxWith(t *testing.T, kind string) []byte {
	t.Helper()
	p := template(t, "docx/transitional.docx")
	if err := p.Add("word/charts/chartEx1.xml", chartExType, fixture(t, kind)); err != nil {
		t.Fatal(err)
	}
	relate(t, p, "word/document.xml", "charts/chartEx1.xml")
	doc, _ := p.Read("word/document.xml")
	run := `<w:p><w:r><mc:AlternateContent xmlns:mc="` + mcNS + `" xmlns:cx1="http://schemas.microsoft.com/office/drawing/2015/9/8/chartex" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main">` +
		`<mc:Choice Requires="cx1"><w:drawing><wp:inline><wp:extent cx="5000000" cy="3000000"/><wp:docPr id="1" name="Graphique 1"/>` +
		`<a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/drawing/2014/chartex"><cx:chart ` + chartExNS + ` r:id="rIdEx"/></a:graphicData></a:graphic></wp:inline></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict/></mc:Fallback></mc:AlternateContent></w:r></w:p>`
	doc = []byte(strings.Replace(string(doc), "<w:sectPr>", run+"<w:sectPr>", 1))
	if err := p.Set("word/document.xml", doc); err != nil {
		t.Fatal(err)
	}
	return bytesOf(t, p)
}

func bytesOf(t *testing.T, p *opc.Package) []byte {
	t.Helper()
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestExtendedCharts(t *testing.T) {
	for _, format := range []struct {
		ext   string
		build func(*testing.T, string) []byte
		part  string
	}{{"pptx", pptxWith, "ppt/charts/chartEx1.xml"}, {"docx", docxWith, "word/charts/chartEx1.xml"}} {
		for file, kind := range extended {
			t.Run(format.ext+"/"+file, func(t *testing.T) {
				data := format.build(t, file)
				f, tree, err := open("x."+format.ext, data)
				if err != nil {
					t.Fatal(err)
				}
				ids := nodes(tree)
				if len(ids) != 1 {
					t.Fatalf("chart nodes: %v", ids)
				}
				c := chartOf(t, tree, ids[0])
				if len(c.Plots) != 1 || c.Plots[0].Kind != kind {
					t.Fatalf("plots: %+v", c.Plots)
				}
				if err := edit(f, tree, ids[0], ot.Values{chartpart.ChartKey: withTitle(c, "Autre").JSON()}); !errors.Is(err, chartpart.ErrReadOnly) {
					t.Errorf("the look of an Excel 2016 chart is edited: %v", err)
				}
				saved, err := f.Save(tree)
				if err != nil {
					t.Fatal(err)
				}
				if parts := changedParts(t, data, saved); len(parts) != 0 {
					t.Errorf("saved with changes: %v", parts)
				}
			})
		}
	}
}

func withTitle(c *chart.Chart, text string) *chart.Chart {
	d := *c
	d.Title = &chart.Title{Text: text}
	return &d
}

func TestExtendedFrameMoves(t *testing.T) {
	data := pptxWith(t, "waterfall")
	f, tree, err := open("x.pptx", data)
	if err != nil {
		t.Fatal(err)
	}
	id := nodes(tree)[0]
	if err := edit(f, tree, id, ot.Values{"xfrm": []byte(`{"x":500,"y":600,"w":4000,"h":2500}`), "name": []byte(`"Cascade"`)}); err != nil {
		t.Fatal(err)
	}
	saved, err := f.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if parts := changedParts(t, data, saved); fmt.Sprint(parts) != "[ppt/slides/slide1.xml]" {
		t.Fatalf("changed parts: %v", parts)
	}
	p, _ := opc.Open(saved, opc.Limits{})
	slide, _ := p.Read("ppt/slides/slide1.xml")
	for want, count := range map[string]int{`<a:off x="500" y="600"/>`: 2, `<a:ext cx="4000" cy="2500"/>`: 2, `name="Cascade"`: 2, `<cx:chart`: 1, `<mc:Fallback>`: 1} {
		if got := strings.Count(string(slide), want); got != count {
			t.Errorf("%d of %s, want %d: %s", got, want, count, slide)
		}
	}
}
