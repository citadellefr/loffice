package xlsx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/charset"
	"github.com/citadellefr/trame/ot"
)

func ansi(s string) string {
	b, _ := charset.Encode(s)
	return string(b)
}

// Files saved unedited are the same, whatever the way they were written.
func TestCSVRoundTrip(t *testing.T) {
	for _, file := range []string{
		"",
		"a",
		ansi("Nom;Prénom;Montant;Date;Part\r\nDurand;Élise;1 234,50;28/09/2026;12,5 %\r\nMartin;;-3;1/2/26;0%\r\n"),
		"\ufeffcode;ville\n01000;Bourg-en-Bresse\n007;\"Paris; 1er\"\n",
		"sep=,\nname,amount\n\"Smith, J.\",1234.5\n\"He said \"\"hi\"\"\",1e3\n",
		"a\tb\tc\n1\t2\t3\n",
		"\"a\";\"b\";\"\"\n\"1\";\"x\";\"\"\n",
		"titre\na;b;c\n1;2\n",
		"a;b\nligne 1\nligne 2\"x;\"multi\nligne\";y\n\n\n",
		"=SOMME(A1:A2);VRAI;#DIV/0!;#VALEUR!;faux\n1,50;+3;1,5e3;12:30;25/12/2024 14:05\n",
		"sans fin de ligne;2",
		"0,1;0,10;1,0;.5;5.;1.234,5",
		"a,b,,\nc\r\nd\re,\"\",f\n,,\n",
		"\"\",,x\n\"\",,y\n",
		"a;\"b\nc",
	} {
		c, tree, err := OpenCSV([]byte(file), "Données.csv")
		if err != nil {
			t.Fatalf("%q: %v", file, err)
		}
		out, err := c.Save(tree)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != file {
			t.Errorf("%q\nsaved %q", file, out)
		}
	}
}

func csvCells(t *testing.T, file string) (*CSV, *ot.Tree, map[string]string) {
	t.Helper()
	c, tree, err := OpenCSV([]byte(file), "ventes.csv")
	if err != nil {
		t.Fatal(err)
	}
	cells := map[string]string{}
	for _, cell := range tree.Node(csvSheet).Grid.Cells() {
		cells[formula.CellName(cell.Row, cell.Col)] = string(cell.Fields)
	}
	return c, tree, cells
}

func TestCSVValues(t *testing.T) {
	_, tree, cells := csvCells(t, "1 234,5;12,5 %;28/09/2026;01000;=1+1;VRAI;#N/A;3,50 €;1.5\n")
	for at, want := range map[string]string{
		"A1": `{"s":"x0","src":"1 234,5","v":1234.5}`,
		"B1": `{"s":"x1","src":"12,5 %","v":0.125}`,
		"C1": `{"s":"x2","v":46293}`,
		"D1": `{"v":"01000"}`,
		"E1": `{"v":"=1+1"}`,
		"F1": `{"v":true}`,
		"G1": `{"e":"#N/A"}`,
		"H1": `{"s":"x3","v":3.5}`,
		"I1": `{"v":"1.5"}`,
	} {
		if cells[at] != want {
			t.Errorf("%s = %s, want %s", at, cells[at], want)
		}
	}
	if name := str(tree.Node(csvSheet), "name"); name != "ventes" {
		t.Errorf("sheet named %q", name)
	}
	if string(tree.Node("book").Attrs["csv"]) != "true" {
		t.Error("the book does not say it is a CSV file")
	}
}

// Edited cells are written from their values, in their formats; the
// others keep their text.
func TestCSVEdited(t *testing.T) {
	file := ansi("Article;Prix;Date\r\nCafé;1 234,50;28/09/2026\r\nThé;2,5;01/10/2026\r\n")
	c, tree, _ := csvCells(t, file)
	calc := NewCalc(tree, formula.Options{})
	e := ot.Edit{{Op: ot.OpCel, ID: csvSheet, Cells: []ot.Cell{
		cell(2, 2, `{"v":1500.25}`),
		cell(3, 1, `{"v":"Thé; vert"}`),
		cell(4, 2, `{"f":"SUM(B2:B3)"}`),
		cell(4, 4, `{"v":"émoji 😀"}`),
	}}}
	if err := c.Check(tree, e); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	calc.Follow(e, nil)
	out, err := c.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	want := "\ufeffArticle;Prix;Date;\r\nCafé;1\u202f500;28/09/2026;\r\n\"Thé; vert\";2,5;01/10/2026;\r\n;1502,75;;émoji 😀\r\n"
	if string(out) != want {
		t.Fatalf("saved %q\nwant  %q", out, want)
	}
	// without the emoji, the file stays in Windows-1252
	e = ot.Edit{{Op: ot.OpCel, ID: csvSheet, Cells: []ot.Cell{cell(4, 4, `{"v":null}`)}}}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	out, _ = c.Save(tree)
	if want := ansi("Article;Prix;Date\r\nCafé;1\u00a0500;28/09/2026\r\n\"Thé; vert\";2,5;01/10/2026\r\n;1502,75;\r\n"); string(out) != want {
		t.Fatalf("saved %q\nwant  %q", out, want)
	}
}

func TestCSVCheck(t *testing.T) {
	c, tree, _ := csvCells(t, "a;b\n")
	for _, e := range []ot.Edit{
		{{Op: ot.OpNew, ID: "S2", Type: "sheet", Parent: "book", Key: "W", Attrs: ot.Values{"name": json.RawMessage(`"Deux"`)}, Cells: []ot.Cell{}}},
		{{Op: ot.OpDel, ID: csvSheet}},
	} {
		if err := c.Check(tree, e); err != ErrCSVSheets {
			t.Errorf("%v: %v", e, err)
		}
	}
	for _, e := range []ot.Edit{
		{{Op: ot.OpCel, ID: csvSheet, Cells: []ot.Cell{cell(1, 1, `{"src":"x","v":"x"}`)}}},
		{{Op: ot.OpCel, ID: csvSheet, Cells: []ot.Cell{cell(1, 1, `{"src":null}`)}}},
		{{Op: ot.OpIns, ID: csvSheet, Dim: ot.DimRows, At: 1, N: 2}},
	} {
		if err := c.Check(tree, e); err != nil {
			t.Errorf("%v: %v", e, err)
		}
	}
	// a text forged to add columns is not written
	e := ot.Edit{{Op: ot.OpCel, ID: csvSheet, Cells: []ot.Cell{cell(1, 1, `{"src":"a;x;y","v":"a"}`)}}}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
	if out, _ := c.Save(tree); string(out) != "a;b\n" {
		t.Fatalf("saved %q", out)
	}
}

func TestCSVSeparator(t *testing.T) {
	for file, sep := range map[string]byte{"a;b,c\n": ';', "a,b,c;d\n": ',', "a\tb\n": '\t', "\"a;b\",c\n": ',', "a\n": ';'} {
		if got := separator(file); got != sep {
			t.Errorf("%q: %q", file, got)
		}
	}
	if _, _, err := OpenCSV([]byte(strings.Repeat(";", formula.MaxCols)+"\n"), "x.csv"); err != ErrCSVSize {
		t.Errorf("too wide: %v", err)
	}
}

// Every CSV file of the corpus is saved unedited as it was read.
func TestCorpusCSV(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../corpus/files/*/*.csv")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		c, tree, err := OpenCSV(data, filepath.Base(f))
		if err != nil {
			t.Errorf("%s: %v", f, err)
			continue
		}
		if out, err := c.Save(tree); err != nil || !bytes.Equal(out, data) {
			t.Errorf("%s: saved differently (%v)", f, err)
		}
	}
	t.Logf("%d CSV files", len(files))
}

// Lines move with the rows inserted and removed, their line breaks and
// fields with them.
func TestCSVRows(t *testing.T) {
	c, tree, _ := csvCells(t, "a,b,,\nc\r\nd\n")
	for _, e := range []ot.Edit{
		{{Op: ot.OpIns, ID: csvSheet, Dim: ot.DimRows, At: 1, N: 1}},
		{{Op: ot.OpCel, ID: csvSheet, Cells: []ot.Cell{cell(1, 1, `{"v":"titre"}`)}}},
		{{Op: ot.OpRem, ID: csvSheet, Dim: ot.DimRows, At: 4, N: 1}},
	} {
		if err := c.Check(tree, e); err != nil {
			t.Fatal(err)
		}
		if err := tree.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	if out, _ := c.Save(tree); string(out) != "titre\na,b,,\nc\r\n" {
		t.Fatalf("saved %q", out)
	}
}
