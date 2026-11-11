package chart

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmldom"
)

func readExtendedFile(t *testing.T, name string) *Chart {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name + ".xml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Read(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !c.Extended() || len(c.Plots) == 0 {
		t.Fatalf("%s: not extended: %+v", name, c.Plots)
	}
	return c
}

func valuesOf(d *Data) string {
	out, _ := json.Marshal(d.Num)
	return string(out)
}

func TestReadWaterfall(t *testing.T) {
	c := readExtendedFile(t, "waterfall")
	p := c.Plots[0]
	if p.Kind != "waterfall" || *p.Gap != 50 || len(p.Series) != 1 {
		t.Fatalf("plot: %+v", p)
	}
	s := p.Series[0]
	if c.Title.Text != "Trésorerie" || c.Legend.Pos != "t" || s.Name.Str[0] != "Montant" || s.Name.Ref != "Feuille1!$B$1" {
		t.Errorf("title, legend, name: %+v %+v %+v", c.Title, c.Legend, s.Name)
	}
	if s.Cat.Ref != "Feuille1!$A$2:$A$7" || s.Cat.Str[3] != "Solde" || valuesOf(s.Val) != "[100,50,-30,120,40,160]" || s.Val.Format != "General" {
		t.Errorf("data: %+v %+v", s.Cat, s.Val)
	}
	if got, _ := json.Marshal(s.Options.Subtotals); string(got) != "[0,3,5]" || s.Options.NoConnectors {
		t.Errorf("options: %+v", s.Options)
	}
	if string(s.Shape["fill"]) != `{"solid":{"rgb":"4472C4"}}` || len(s.Points) != 1 || s.Points[0].Idx != 2 || s.Points[0].Shape["fill"] == nil {
		t.Errorf("shapes: %+v %+v", s.Shape, s.Points)
	}
	if s.Labels == nil || !s.Labels.Val || s.Labels.Cat || s.Labels.Pos != "outEnd" {
		t.Errorf("labels: %+v", s.Labels)
	}
	if len(c.Axes) != 2 || c.Axes[0].Kind != "cat" || c.Axes[0].Pos != "b" || c.Axes[1].Kind != "val" || c.Axes[1].Grid == nil || c.Axes[1].Labels != "nextTo" {
		t.Errorf("axes: %+v", c.Axes)
	}
}

func TestReadHistogram(t *testing.T) {
	c := readExtendedFile(t, "histogram")
	p := c.Plots[0]
	s := p.Series[0]
	b := s.Options.Bins
	if p.Kind != "histogram" || *p.Gap != 0 || s.Cat != nil || len(s.Val.Num) != 12 {
		t.Errorf("plot: %+v", p)
	}
	if b == nil || *b.Size != 3 || b.Closed != "r" || b.Count != nil || s.Options.Aggregate {
		t.Errorf("bins: %+v", b)
	}
}

func TestReadPareto(t *testing.T) {
	c := readExtendedFile(t, "pareto")
	if len(c.Plots) != 1 || c.Plots[0].Kind != "pareto" || len(c.Plots[0].Series) != 2 {
		t.Fatalf("plots: %+v", c.Plots)
	}
	bars, line := c.Plots[0].Series[0], c.Plots[0].Series[1]
	if !bars.Options.Aggregate || bars.Options.Line || !line.Options.Line || line.Val != nil || bars.Index != 0 || line.Index != 1 {
		t.Errorf("series: %+v %+v", bars.Options, line.Options)
	}
	if len(c.Axes) != 3 || *c.Axes[2].Max != 1 || c.Axes[2].Format != "0%" {
		t.Errorf("axes: %+v", c.Axes)
	}
}

func TestReadBoxWhisker(t *testing.T) {
	c := readExtendedFile(t, "boxwhisker")
	p := c.Plots[0]
	if p.Kind != "boxWhisker" || len(p.Series) != 2 || p.Gap != nil {
		t.Fatalf("plot: %+v", p)
	}
	a, b := p.Series[0], p.Series[1]
	if *a.Options.Box != (Box{MeanMarker: true, Outliers: true, Quartile: "exclusive"}) {
		t.Errorf("first box: %+v", a.Options.Box)
	}
	if *b.Options.Box != (Box{MeanLine: true, Inner: true, Outliers: true, Quartile: "inclusive"}) {
		t.Errorf("second box: %+v", b.Options.Box)
	}
	if a.Cat == nil || b.Cat != nil || valuesOf(b.Val) != "[10,12,13,15,16,18,19,20]" || b.Name.Str[0] != "Équipe B" {
		t.Errorf("data: %+v %+v", a.Cat, b)
	}
}

func TestReadFunnel(t *testing.T) {
	c := readExtendedFile(t, "funnel")
	p := c.Plots[0]
	s := p.Series[0]
	if p.Kind != "funnel" || *p.Gap != 6 || s.Cat.Str[0] != "Visites" || valuesOf(s.Val) != "[1000,600,300,120]" || s.Labels.Pos != "inEnd" {
		t.Errorf("funnel: %+v %+v", p, s)
	}
}

func TestReadHierarchies(t *testing.T) {
	for kind, parents := range map[string]string{"treemap": "overlapping", "sunburst": ""} {
		c := readExtendedFile(t, kind)
		p := c.Plots[0]
		s := p.Series[0]
		if p.Kind != kind || p.Gap != nil || s.Options.Parents != parents {
			t.Errorf("%s: %+v %+v", kind, p, s.Options)
		}
		if len(s.Cat.Levels) != 2 || s.Cat.Levels[0][0] != "Pommes" || s.Cat.Levels[1][0] != "Fruits" || s.Cat.Str[2] != "Carottes" || s.Cat.Ref != "Feuille1!$A$2:$B$7" {
			t.Errorf("%s levels: %+v", kind, s.Cat)
		}
		if valuesOf(s.Val) != "[30,20,25,10,5,10]" || s.Labels == nil || !s.Labels.Cat || s.Labels.Val {
			t.Errorf("%s values: %+v %+v", kind, s.Val, s.Labels)
		}
	}
}

func TestReadMap(t *testing.T) {
	c := readExtendedFile(t, "map")
	p := c.Plots[0]
	if p.Kind != "regionMap" || p.Series[0].Cat.Str[1] != "Italie" || p.Series[0].Val != nil {
		t.Errorf("map: %+v", p)
	}
}

func TestExtendedNotPatched(t *testing.T) {
	data, err := os.ReadFile("testdata/waterfall.xml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := ReadElement(doc.Root)
	c.Title.Text = "Autre"
	c.Plots[0].Kind = "bar"
	if Patch(doc.Root, c) || string(doc.Bytes()) != string(mustParse(t, data).Bytes()) {
		t.Error("an extended chart was patched")
	}
}

func mustParse(t *testing.T, data []byte) *xmldom.Document {
	t.Helper()
	doc, err := xmldom.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestHistogramBinsAsAttribute(t *testing.T) {
	data, err := os.ReadFile("testdata/histogram.xml")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Read([]byte(strings.Replace(string(data), "<cx:binSize>3</cx:binSize>", `<cx:binSize val="3"/>`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if b := c.Plots[0].Series[0].Options.Bins; b == nil || b.Size == nil || *b.Size != 3 {
		t.Errorf("bins: %+v", b)
	}
}
