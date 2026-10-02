package xlsx

import (
	"encoding/json"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/ot"
)

func TestChartsFollow(t *testing.T) {
	_, tree := openCorpus(t, "poi/WithTwoCharts.xlsx")
	var id string
	var charts []Placed
	for _, n := range tree.Children("book") {
		if n.Attrs[chartsKey] != nil {
			id = n.ID
			_ = json.Unmarshal(n.Attrs[chartsKey], &charts)
			break
		}
	}
	if len(charts) != 1 {
		t.Fatalf("charts: %+v", charts)
	}
	p := charts[0]
	if *p.From != (Corner{Col: 0, Row: 0, DY: 19049}) || *p.To != (Corner{Col: 8, Row: 15, DX: 247650, DY: 66674}) {
		t.Errorf("anchor: %+v %+v", p.From, p.To)
	}
	if got := p.Chart.Plots[0].Series[1].Val.Ref; got != "Sheet1!$B$1:$B$6" {
		t.Errorf("reference: %s", got)
	}
	data := ""
	for _, n := range tree.Children("book") {
		if str(n, "name") == "Sheet1" {
			data = n.ID
		}
	}
	c := NewCalc(tree, formula.Options{})
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: data, Dim: ot.DimRows, At: 1, N: 2}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpIns, ID: id, Dim: ot.DimRows, At: 3, N: 4}})
	edit(t, tree, c, ot.Edit{{Op: ot.OpRem, ID: id, Dim: ot.DimCols, At: 1, N: 2}})
	_ = json.Unmarshal(tree.Node(id).Attrs[chartsKey], &charts)
	p = charts[0]
	if *p.From != (Corner{Col: 0, Row: 0, DY: 19049}) || *p.To != (Corner{Col: 6, Row: 19, DX: 247650, DY: 66674}) {
		t.Errorf("anchor moved: %+v %+v", p.From, p.To)
	}
	if got := p.Chart.Plots[0].Series[1].Val.Ref; got != "Sheet1!$B$3:$B$8" {
		t.Errorf("reference moved: %s", got)
	}
}

func TestChartSheet(t *testing.T) {
	_, tree := openCorpus(t, "poi/WithChartSheet.xlsx")
	found := false
	for _, n := range tree.Children("book") {
		if n.Type == "kept" && n.Attrs["chart"] != nil {
			found = true
		}
	}
	if !found {
		t.Error("the chart sheet has no chart")
	}
}
