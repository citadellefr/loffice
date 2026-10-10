package legacy

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

// converted is a workbook of Apache POI's test files once converted and
// read again: its tree, and what the conversion left out.
func converted(t *testing.T, name string) (*ot.Tree, []Loss) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../corpus/files/poi", name))
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	book, lost, err := Workbook(data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	_, tree, err := xlsx.Open(book)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return tree, lost
}

// sheetNamed is the sheet of that name.
func sheetNamed(t *testing.T, tree *ot.Tree, name string) *ot.Node {
	t.Helper()
	for _, n := range tree.Children("book") {
		if string(n.Attrs["name"]) == `"`+name+`"` {
			return n
		}
	}
	t.Fatalf("no sheet %q", name)
	return nil
}

// fields are the fields of a cell, named as in a formula.
func fields(t *testing.T, sheet *ot.Node, cell string) string {
	t.Helper()
	row, col, ok := formula.ParseCell(cell)
	if !ok {
		t.Fatalf("%q is not a cell", cell)
	}
	for _, c := range sheet.Grid.Cells() {
		if c.Row == row && c.Col == col {
			var f map[string]json.RawMessage
			if err := json.Unmarshal(c.Fields, &f); err != nil {
				t.Fatal(err)
			}
			delete(f, "s")
			out, _ := json.Marshal(f)
			return string(out)
		}
	}
	return ""
}

func TestWorkbookCells(t *testing.T) {
	tree, lost := converted(t, "SampleSS.xls")
	if len(lost) != 0 {
		t.Errorf("lost %v", lost)
	}
	var names []string
	for _, n := range tree.Children("book") {
		names = append(names, string(n.Attrs["name"]))
	}
	if got := strings.Join(names, " "); got != `"First Sheet" "Sheet Number 2" "Sheet3"` {
		t.Errorf("sheets %s", got)
	}
	if got := fields(t, sheetNamed(t, tree, "Sheet Number 2"), "D7"); got != `{"f":"SUM(A7:C7)","v":13}` {
		t.Errorf("D7 is %s", got)
	}
}

func TestWorkbookFormulas(t *testing.T) {
	// shared by the cells of an area, written from the cell of each
	tree, _ := converted(t, "SharedFormulaTest.xls")
	sheet := sheetNamed(t, tree, "0")
	for cell, want := range map[string]string{"B3": `{"f":"A$1*2","v":2}`, "DY4": `{"f":"DZ4*2","v":8}`} {
		if got := fields(t, sheet, cell); got != want {
			t.Errorf("%s is %s, want %s", cell, got, want)
		}
	}
	// an array formula belongs to its first cell
	tree, _ = converted(t, "RowFunctionTestCaseData.xls")
	sheet = sheetNamed(t, tree, "Tests")
	for cell, want := range map[string]string{"B4": `{"f":"ROW(1:4)","fa":"B4:B7","v":1}`, "B5": `{"v":2}`} {
		if got := fields(t, sheet, cell); got != want {
			t.Errorf("%s is %s, want %s", cell, got, want)
		}
	}
	// a data table is left as its values, and told
	if _, lost := converted(t, "44958.xls"); !slices.Contains(lost, Formulas) {
		t.Errorf("lost %v", lost)
	}
}

func TestWorkbookSettings(t *testing.T) {
	tree, _ := converted(t, "51670.xls")
	sheet := sheetNamed(t, tree, "SBNReport")
	if string(sheet.Attrs["frozen"]) != `{"r":7}` || string(sheet.Attrs["grid"]) != "false" {
		t.Errorf("sheet %v", sheet.Attrs)
	}
	if want := `[{"name":"_xlnm.Print_Titles","ref":"SBNReport!$1:$7","sheet":"` + sheet.ID + `"}]`; string(tree.Node("book").Attrs["names"]) != want {
		t.Errorf("names %s", tree.Node("book").Attrs["names"])
	}

	tree, _ = converted(t, "templateExcelWithAutofilter.xls")
	if got := string(sheetNamed(t, tree, "Initial sheet").Attrs["filter"]); got != `{"ref":"A1:D1"}` {
		t.Errorf("filter %s", got)
	}

	tree, _ = converted(t, "1904DateWindowing.xls")
	if string(tree.Node("book").Attrs["date1904"]) != "true" {
		t.Errorf("book %v", tree.Node("book").Attrs)
	}
}

func TestWorkbookRefused(t *testing.T) {
	for name, want := range map[string]error{
		"password.xls":     ErrEncrypted,
		"testEXCEL_95.xls": ErrTooOld,
		"testEXCEL_4.xls":  ErrTooOld,
		"61300.xls":        ErrEmpty,
		"SampleSS.xlsx":    ErrInvalid,
	} {
		data, err := os.ReadFile(filepath.Join("../corpus/files/poi", name))
		if err != nil {
			t.Skip("no corpus: corpus/fetch.sh")
		}
		if _, _, err := Workbook(data); !errors.Is(err, want) {
			t.Errorf("%s: %v, want %v", name, err, want)
		}
	}
}

// A workbook with more cells than MaxCells is refused before it takes
// the memory they would.
func TestWorkbookTooLarge(t *testing.T) {
	data, err := os.ReadFile("../corpus/files/poi/SampleSS.xls")
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	defer func(n int) { MaxCells = n }(MaxCells)
	MaxCells = 10
	if _, _, err := Workbook(data); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("%v", err)
	}
}

// Whatever the file, a conversion ends, and what it gives opens.
func FuzzWorkbook(f *testing.F) {
	files, _ := filepath.Glob("../corpus/files/poi/*.xls")
	for _, name := range files {
		if data, err := os.ReadFile(name); err == nil && len(data) < 20<<10 {
			f.Add(data)
		}
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		book, _, err := Workbook(data)
		if err != nil {
			return
		}
		if _, _, err := xlsx.Open(book); err != nil {
			t.Fatalf("the converted workbook does not open: %v", err)
		}
	})
}

func corpus(t *testing.T, ext string) []string {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../corpus/files/*/*")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	var out []string
	for _, f := range files {
		if strings.ToLower(filepath.Ext(f)) == ext {
			out = append(out, f)
		}
	}
	return out
}

// Every workbook of the corpus converts into one that opens, or is refused
// for what it is. When LOFFICE_OUT is set, the workbooks are written there
// for the validator, each beside the empty one it was made from.
func TestCorpusWorkbooks(t *testing.T) {
	out := os.Getenv("LOFFICE_OUT")
	converted, cells := 0, 0
	refused, lost := map[string]int{}, map[Loss]int{}
	for _, f := range corpus(t, ".xls") {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		book, losses, err := Workbook(data)
		if err != nil {
			if !errors.Is(err, ErrInvalid) && !errors.Is(err, ErrEncrypted) && !errors.Is(err, ErrTooOld) && !errors.Is(err, ErrEmpty) {
				t.Errorf("%s: %v", f, err)
			}
			refused[err.Error()]++
			continue
		}
		_, tree, err := xlsx.Open(book)
		if err != nil {
			t.Fatalf("%s: the converted workbook does not open: %v", f, err)
		}
		converted++
		for _, n := range tree.Children("book") {
			if n.Grid != nil {
				cells += n.Grid.Len()
			}
		}
		for _, l := range losses {
			lost[l]++
		}
		if out != "" {
			name := filepath.Base(f) + ".xlsx"
			for dir, data := range map[string][]byte{"blank": blank, "converted": book} {
				if err := os.MkdirAll(filepath.Join(out, dir), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(out, dir, name), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	t.Logf("%d converted, %d cells; lost: %v; refused: %v", converted, cells, lost, refused)
}
