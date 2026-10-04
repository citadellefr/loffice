package xlsx

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"runtime"
	"strconv"
	"testing"

	"github.com/citadellefr/trame/ot"
)

// BenchmarkOpenLarge opens a sheet of 100 000 rows and 20 columns, half
// numbers and half texts, and reports the memory the tree holds against
// the size of the XML.
func BenchmarkOpenLarge(b *testing.B) {
	data := largeWorkbook(b, 100000, 20)
	xml := unzipped(b, data)
	var tree *ot.Tree
	for b.Loop() {
		var err error
		if _, tree, err = Open(data); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	tree = nil
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, tree, _ = Open(data)
	runtime.GC()
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(tree)
	b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/float64(xml), "×xml")
}

// largeWorkbook is the blank workbook with rows × cols cells, as L'Office
// writes it.
func largeWorkbook(b *testing.B, rows, cols int) []byte {
	blank, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		b.Fatal(err)
	}
	d, tree, err := Open(blank)
	if err != nil {
		b.Fatal(err)
	}
	var sheet string
	for _, n := range tree.Children("book") {
		if n.Type == "sheet" {
			sheet = n.ID
			break
		}
	}
	cells := make([]ot.Cell, 0, rows*cols)
	for r := 1; r <= rows; r++ {
		for c := 1; c <= cols; c++ {
			v := strconv.Itoa(r*c) + ".25"
			if c%2 == 0 {
				v = `"Ligne ` + strconv.Itoa(r) + `"`
			}
			cells = append(cells, cell(r, c, `{"v":`+v+`}`))
		}
	}
	if err := tree.Apply(ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: cells}}); err != nil {
		b.Fatal(err)
	}
	data, err := d.Save(tree)
	if err != nil {
		b.Fatal(err)
	}
	return data
}

// unzipped is the size of the parts of a package once decompressed.
func unzipped(b *testing.B, data []byte) int64 {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		b.Fatal(err)
	}
	var n int64
	for _, f := range z.File {
		r, _ := f.Open()
		m, _ := io.Copy(io.Discard, r)
		r.Close()
		n += m
	}
	return n
}

// BenchmarkOpenCSV opens a CSV file of 100 000 lines of 20 fields, half
// numbers and half texts.
func BenchmarkOpenCSV(b *testing.B) {
	var data []byte
	for r := 1; r <= 100000; r++ {
		for c := 1; c <= 20; c++ {
			if c > 1 {
				data = append(data, ';')
			}
			if c%2 == 0 {
				data = append(data, "Ligne "+strconv.Itoa(r)...)
			} else {
				data = append(data, strconv.Itoa(r*c)+",25"...)
			}
		}
		data = append(data, "\r\n"...)
	}
	for b.Loop() {
		if _, _, err := OpenCSV(data, "large.csv"); err != nil {
			b.Fatal(err)
		}
	}
}
