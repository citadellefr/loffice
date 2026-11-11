package xlsx

import (
	"fmt"
	"testing"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/trame/ot"
)

// BenchmarkChartRefresh refreshes charts of long series the way a Save does.
func BenchmarkChartRefresh(b *testing.B) {
	const rows, series, charts = 5000, 4, 4
	t := &testing.T{}
	d, tree, calc, id := chartBook(t)
	var cells []ot.Cell
	for r := 1; r <= rows; r++ {
		cells = append(cells, cell(r, 1, fmt.Sprintf(`{"v":"T%d"}`, r)))
		for c := 2; c <= series+1; c++ {
			cells = append(cells, cell(r, c, fmt.Sprintf(`{"v":%d.5}`, r*c)))
		}
	}
	chartFollowed(t, d, tree, calc, ot.Edit{{Op: ot.OpCel, ID: id, Cells: cells}})
	build := func() *chart.Chart {
		p := chart.Plot{Kind: "bar", Dir: "col", Grouping: "clustered"}
		for i := range series {
			col := string(rune('B' + i))
			p.Series = append(p.Series, chart.Series{
				Index: i, Order: i,
				Name: &chart.Data{Ref: fmt.Sprintf("Feuille1!$%s$1", col), Count: 1},
				Cat:  &chart.Data{Ref: fmt.Sprintf("Feuille1!$A$2:$A$%d", rows)},
				Val:  &chart.Data{Ref: fmt.Sprintf("Feuille1!$%s$2:$%s$%d", col, col, rows)},
			})
		}
		return &chart.Chart{Plots: []chart.Plot{p}}
	}
	b.ReportAllocs()
	for b.Loop() {
		now, then := newChartCells(tree), newChartCells(d.loaded)
		for range charts {
			want, had := build(), build()
			refresh(had, nil, "Feuille1", now, then)
			refresh(want, had, "Feuille1", now, then)
		}
	}
}
