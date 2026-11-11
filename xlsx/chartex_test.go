package xlsx

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

var extendedCharts = []string{"waterfall", "histogram", "boxwhisker", "funnel", "treemap", "sunburst", "map"}

func chartexPackage(t *testing.T) ([]byte, map[string][]byte) {
	t.Helper()
	blank, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(blank, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	const cx = `xmlns:cx="http://schemas.microsoft.com/office/drawing/2014/chartex"`
	wsDr := `<xdr:wsDr xmlns:xdr="` + drawingNS + `" xmlns:a="` + aNS + `" xmlns:r="` + relNS + `" xmlns:mc="` + mcNS + `" xmlns:cx1="http://schemas.microsoft.com/office/drawing/2015/9/8/chartex">`
	var rels []opc.Relationship
	parts := map[string][]byte{}
	for i, name := range extendedCharts {
		data, err := os.ReadFile("../chart/testdata/" + name + ".xml")
		if err != nil {
			t.Fatal(err)
		}
		part := fmt.Sprintf("xl/charts/chartEx%d.xml", i+1)
		parts[part] = data
		if err := p.Add(part, typeChartEx, data); err != nil {
			t.Fatal(err)
		}
		id := fmt.Sprintf("rId%d", i+1)
		rels = append(rels, opc.Relationship{ID: id, Type: relChartEx, Target: fmt.Sprintf("../charts/chartEx%d.xml", i+1)})
		pos := func(shift int) string {
			return fmt.Sprintf(`<xdr:from><xdr:col>%d</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>%d</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from>`+
				`<xdr:to><xdr:col>%d</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>%d</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:to>`, 10, shift*20, 18, shift*20+15)
		}
		wsDr += `<mc:AlternateContent><mc:Choice Requires="cx1"><xdr:twoCellAnchor>` + pos(i) +
			`<xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="` + fmt.Sprint(i+2) + `" name="Graphique ` + fmt.Sprint(i+1) + `"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr>` +
			`<xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/drawing/2014/chartex">` +
			`<cx:chart ` + cx + ` r:id="` + id + `"/></a:graphicData></a:graphic></xdr:graphicFrame><xdr:clientData/></xdr:twoCellAnchor></mc:Choice>` +
			`<mc:Fallback><xdr:twoCellAnchor>` + pos(i) + `<xdr:sp macro="" textlink=""><xdr:nvSpPr><xdr:cNvPr id="` + fmt.Sprint(i+2) + `" name="Graphique ` + fmt.Sprint(i+1) + `"/><xdr:cNvSpPr/></xdr:nvSpPr><xdr:spPr/></xdr:sp><xdr:clientData/></xdr:twoCellAnchor></mc:Fallback></mc:AlternateContent>`
	}
	wsDr += `</xdr:wsDr>`
	sheet := `<worksheet xmlns="` + mainNS + `" xmlns:r="` + relNS + `"><sheetData>` +
		`<row r="1"><c r="A1" t="inlineStr"><is><t>Serie</t></is></c><c r="B1" t="inlineStr"><is><t>Valeur</t></is></c></row>` +
		`<row r="2"><c r="A2" t="inlineStr"><is><t>Départ</t></is></c><c r="B2"><v>100</v></c></row>` +
		`</sheetData><drawing r:id="rId1"/></worksheet>`
	if err := p.Set("xl/worksheets/sheet1.xml", []byte(sheet)); err != nil {
		t.Fatal(err)
	}
	for _, a := range []struct {
		name, typ string
		data      []byte
	}{
		{"xl/worksheets/_rels/sheet1.xml.rels", typeRels, opc.MarshalRelationships([]opc.Relationship{{ID: "rId1", Type: relDrawing, Target: "../drawings/drawing1.xml"}})},
		{"xl/drawings/drawing1.xml", typeDrawing, []byte(wsDr)},
		{"xl/drawings/_rels/drawing1.xml.rels", typeRels, opc.MarshalRelationships(rels)},
	} {
		if err := p.Add(a.name, a.typ, []byte(a.data)); err != nil {
			t.Fatal(err)
		}
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return out, parts
}

func sheetPlaced(t *testing.T, tree *ot.Tree) (string, []Placed) {
	t.Helper()
	for _, n := range tree.Children("book") {
		if n.Attrs[chartsKey] != nil {
			var charts []Placed
			if err := json.Unmarshal(n.Attrs[chartsKey], &charts); err != nil {
				t.Fatal(err)
			}
			return n.ID, charts
		}
	}
	t.Fatal("no sheet has charts")
	return "", nil
}

func TestChartexRead(t *testing.T) {
	data, _ := chartexPackage(t)
	_, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, charts := sheetPlaced(t, tree)
	if len(charts) != len(extendedCharts) {
		t.Fatalf("%d charts, want %d", len(charts), len(extendedCharts))
	}
	for i, want := range []string{"waterfall", "histogram", "boxWhisker", "funnel", "treemap", "sunburst", "regionMap"} {
		p := charts[i]
		if len(p.Chart.Plots) != 1 || p.Chart.Plots[0].Kind != want {
			t.Errorf("chart %d: %+v", i, p.Chart.Plots)
			continue
		}
		s := p.Chart.Plots[0].Series[0]
		if want != "regionMap" && (s.Val == nil || s.Val.Ref == "" || len(s.Val.Num) == 0) {
			t.Errorf("%s has no data: %+v", want, s.Val)
		}
		if p.From == nil || p.From.Row != i*20 {
			t.Errorf("%s anchor: %+v", want, p.From)
		}
	}
	if o := charts[0].Chart.Plots[0].Series[0].Options; o == nil || len(o.Subtotals) == 0 {
		t.Errorf("waterfall options: %+v", o)
	}
}

func TestChartexRoundTrip(t *testing.T) {
	data, parts := chartexPackage(t)
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	edit(t, tree, NewCalc(tree, formula.Options{}), ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(5, 5, `{"v":42}`)}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := opc.Open(data, opc.Limits{})
	after, err := opc.Open(saved, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"xl/drawings/drawing1.xml", "xl/drawings/_rels/drawing1.xml.rels"}
	for name := range parts {
		names = append(names, name)
	}
	for _, name := range names {
		a, _ := before.Read(name)
		b, err := after.Read(name)
		if err != nil || string(a) != string(b) {
			t.Errorf("%s changed: %v", name, err)
		}
		if before.ContentType(name) != after.ContentType(name) {
			t.Errorf("%s content type: %q", name, after.ContentType(name))
		}
	}
}

func TestChartexMoveAndRefuse(t *testing.T) {
	data, parts := chartexPackage(t)
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	id, charts := sheetPlaced(t, tree)
	moved := make([]Placed, len(charts))
	copy(moved, charts)
	from, to := *charts[1].From, *charts[1].To
	from.Row += 3
	to.Row += 3
	moved[1].From, moved[1].To = &from, &to
	attrs, _ := json.Marshal(moved)
	e := ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: attrs}}}
	if err := d.Check(tree, e, ""); err != nil {
		t.Fatalf("a frame moves: %v", err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := opc.Open(saved, opc.Limits{})
	for name, want := range parts {
		if got, _ := after.Read(name); string(got) != string(want) {
			t.Errorf("%s changed by a move", name)
		}
	}
	_, reopened, err := Open(saved, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if _, got := sheetPlaced(t, reopened); got[1].From.Row != from.Row || got[1].Chart.Plots[0].Kind != "histogram" {
		t.Errorf("moved chart: %+v", got[1])
	}

	edited := make([]Placed, len(charts))
	copy(edited, charts)
	c := *charts[0].Chart
	c.Title = &chart.Title{Text: "Autre"}
	edited[0].Chart = &c
	attrs, _ = json.Marshal(edited)
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: id, Attrs: ot.Values{chartsKey: attrs}}}, ""); err == nil {
		t.Error("the look of an Excel 2016 chart is edited")
	}
}
