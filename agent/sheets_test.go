package agent

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/trame/ot"
)

const book = "book.xlsx"

// buildBook is a workbook of two sheets: Données holds 2, 3 and their sum,
// a percentage, a text with a bar, an error; Cache is hidden.
func buildBook() ([]byte, error) {
	const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	const main = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"`
	const rel = `http://schemas.openxmlformats.org/officeDocument/2006/relationships`
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", head + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
			`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
			`<Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`},
		{"_rels/.rels", head + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + rel + `/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", head + `<workbook ` + main + ` xmlns:r="` + rel + `"><sheets>` +
			`<sheet name="Données" sheetId="1" r:id="rId1"/><sheet name="Cache" sheetId="2" state="hidden" r:id="rId2"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", head + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="` + rel + `/worksheet" Target="worksheets/sheet1.xml"/>` +
			`<Relationship Id="rId2" Type="` + rel + `/worksheet" Target="worksheets/sheet2.xml"/>` +
			`<Relationship Id="rId3" Type="` + rel + `/styles" Target="styles.xml"/></Relationships>`},
		{"xl/styles.xml", head + `<styleSheet ` + main + `><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts>` +
			`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
			`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
			`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
			`<cellXfs count="2"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/><xf numFmtId="9" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/></cellXfs></styleSheet>`},
		{"xl/worksheets/sheet1.xml", head + `<worksheet ` + main + `><sheetData>` +
			`<row r="1"><c r="A1"><v>2</v></c><c r="B1" s="1"><v>0.5</v></c><c r="C1" t="e"><f>1/0</f><v>#DIV/0!</v></c></row>` +
			`<row r="2"><c r="A2"><v>3</v></c><c r="B2" t="inlineStr"><is><t>a|b</t></is></c></row>` +
			`<row r="3"><c r="A3"><f>SUM(A1:A2)</f><v>5</v></c></row>` +
			`<row r="4"><c r="A4"><v>40</v></c></row></sheetData></worksheet>`},
		{"xl/worksheets/sheet2.xml", head + `<worksheet ` + main + `><sheetData/></worksheet>`},
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, p := range parts {
		w, err := z.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.data)); err != nil {
			return nil, err
		}
	}
	err := z.Close()
	return b.Bytes(), err
}

func newBook(t *testing.T) (*room, *Agent) {
	t.Helper()
	data, err := buildBook()
	if err != nil {
		t.Fatal(err)
	}
	r := newRoomOf(t, book, data, loffice.Options{})
	return r, r.join("Héraut", Options{})
}

// cell is the cell ref of a sheet, as Range gives it.
func cell(t *testing.T, a *Agent, sheet, ref string) Cell {
	t.Helper()
	_, r, err := a.Range(sheet, ref)
	if err != nil || len(r.Rows) != 1 || len(r.Rows[0]) != 1 {
		t.Fatalf("range %s: %+v %v", ref, r, err)
	}
	return r.Rows[0][0]
}

// waitCell waits for the calculation of the hub to make a cell of Données
// what it is.
func waitCell(t *testing.T, a *Agent, want Cell) {
	t.Helper()
	for range 200 {
		if cell(t, a, "Données", want.Ref) == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s is %+v, want %+v", want.Ref, cell(t, a, "Données", want.Ref), want)
}

func apply(t *testing.T, a *Agent, edits ...Edit) Record {
	t.Helper()
	rec, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: edits})
	if err != nil {
		t.Fatal(err)
	}
	return rec
}

func TestSheets(t *testing.T) {
	_, a := newBook(t)
	v, sheets, err := a.Sheets()
	if err != nil || v != a.Version() {
		t.Fatal(v, err)
	}
	if len(sheets) != 2 || sheets[0].Name != "Données" || sheets[0].Used != "A1:C4" || sheets[0].Hidden ||
		sheets[1].Name != "Cache" || sheets[1].Used != "" || !sheets[1].Hidden || sheets[0].ID == "" {
		t.Fatalf("%+v", sheets)
	}
}

func TestRange(t *testing.T) {
	_, a := newBook(t)
	_, r, err := a.Range("données", "")
	if err != nil || r.Sheet != "Données" || r.Area != "A1:C4" || len(r.Rows) != 4 || len(r.Rows[0]) != 3 {
		t.Fatalf("%+v %v", r, err)
	}
	got, _ := json.Marshal(r.Rows[:3])
	want := `[[{"ref":"A1","value":2},{"ref":"B1","value":0.5,"format":"0%"},{"ref":"C1","error":"#DIV/0!","formula":"=1/0"}],` +
		`[{"ref":"A2","value":3},{"ref":"B2","value":"a|b"},{"ref":"C2"}],` +
		`[{"ref":"A3","value":5,"formula":"=SUM(A1:A2)"},{"ref":"B3"},{"ref":"C3"}]]`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	for area, want := range map[string]string{"B2": "B2", "B2:C3": "B2:C3", "A:A": "A1:A4", "3:4": "A3:C4", "$a$1:b1": "A1:B1", "E1:F2": "E1:F2"} {
		if _, r, err := a.Range("Données", area); err != nil || r.Area != want {
			t.Errorf("%s: %q %v", area, r.Area, err)
		}
	}
	if _, r, err := a.Range("Cache", ""); err != nil || len(r.Rows) != 0 || r.Markdown() != "" {
		t.Errorf("empty sheet: %+v %v", r, err)
	}
}

func TestRangeMarkdown(t *testing.T) {
	_, a := newBook(t)
	_, r, err := a.Range("Données", "A2:C3")
	if err != nil {
		t.Fatal(err)
	}
	want := "| | A | B | C |\n|---|---|---|---|\n| 2 | 3 | a\\|b |  |\n| 3 | =SUM(A1:A2) → 5 |  |  |\n"
	if got := r.Markdown(); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	_, r, _ = a.Range("Données", "C1")
	if got := r.Markdown(); !strings.Contains(got, "=1/0 → #DIV/0!") {
		t.Fatal(got)
	}
}

func TestRangeInvalid(t *testing.T) {
	_, a := newBook(t)
	for _, area := range []string{"A1:", "x", "A1:B2:C3", "A1:Z1000", "A:XFD"} {
		if _, _, err := a.Range("Données", area); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", area, err)
		}
	}
	if _, _, err := a.Range("Données", "A1:XFD1000"); !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "read less") {
		t.Error(err)
	}
	if _, _, err := a.Range("Nowhere", ""); !errors.Is(err, ErrGone) || !strings.Contains(err.Error(), `"Données"`) {
		t.Error(err)
	}
	if _, _, err := a.Range("", ""); !errors.Is(err, ErrInvalid) {
		t.Error(err)
	}
}

func TestCells(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	apply(t, a,
		Edit{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0, "hello"}, {4.0, "=A1*2", true}}},
		Edit{Op: "cells", Sheet: "Données", Range: "C1", Values: [][]any{{nil}}},
	)
	f := alice.next("op")
	if !strings.Contains(string(f.D), `"cel"`) {
		t.Fatalf("%s", f.D)
	}
	waitCell(t, a, Cell{Ref: "B2", Value: 20.0, Formula: "=A1*2"})
	for ref, want := range map[string]Cell{
		"A1": {Ref: "A1", Value: 10.0},
		"B1": {Ref: "B1", Value: "hello", Format: "0%"},
		"A2": {Ref: "A2", Value: 4.0},
		"C2": {Ref: "C2", Value: true},
		"B2": {Ref: "B2", Value: 20.0, Formula: "=A1*2"},
		"C1": {Ref: "C1"},
	} {
		if got := cell(t, a, "Données", ref); got != want {
			t.Errorf("%s: %+v, want %+v", ref, got, want)
		}
	}
	waitCell(t, a, Cell{Ref: "A3", Value: 14.0, Formula: "=SUM(A1:A2)"})
}

// sheetID is the id of the sheet Données.
func sheetID(t *testing.T, a *Agent) string {
	t.Helper()
	_, sheets, err := a.Sheets()
	if err != nil {
		t.Fatal(err)
	}
	return sheets[0].ID
}

// typeCell is an edit of a person setting the fields of a cell.
func typeCell(sheet string, row, col int, fields string) ot.Edit {
	return ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{{Row: row, Col: col, Fields: json.RawMessage(fields)}}}}
}

func TestLines(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	apply(t, a, Edit{Op: "insert_rows", Sheet: "Données", Range: "2:3"})
	if f := alice.next("op"); !strings.Contains(string(f.D), `"ins"`) {
		t.Fatalf("%s", f.D)
	}
	waitCell(t, a, Cell{Ref: "A5", Value: 5.0, Formula: "=SUM(A1:A4)"})
	_, got, _ := a.Range("Données", "A:A")
	if got.Area != "A1:A6" || got.Rows[0][0].Value != 2.0 || got.Rows[1][0].Value != nil || got.Rows[3][0].Value != 3.0 ||
		got.Rows[4][0].Formula != "=SUM(A1:A4)" || got.Rows[5][0].Value != 40.0 {
		t.Fatalf("%+v", got.Rows)
	}
	apply(t, a, Edit{Op: "delete_rows", Sheet: "Données", Range: "2:4"})
	waitCell(t, a, Cell{Ref: "A2", Value: 2.0, Formula: "=SUM(A1:A1)"})
	if c := cell(t, a, "Données", "A3"); c.Value != 40.0 {
		t.Fatalf("%+v", c)
	}
	apply(t, a, Edit{Op: "delete_columns", Sheet: "Données", Range: "B:B"})
	if c := cell(t, a, "Données", "B1"); c.Error != "#DIV/0!" {
		t.Fatalf("%+v", c)
	}
	apply(t, a, Edit{Op: "insert_columns", Sheet: "Données", Range: "A:B"})
	if c := cell(t, a, "Données", "C1"); c.Value != 2.0 {
		t.Fatalf("%+v", c)
	}
	if _, sheets, _ := a.Sheets(); sheets[0].Used != "C1:D3" {
		t.Fatalf("%+v", sheets)
	}
}

func TestBatchAtomic(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	v := a.Version()
	_, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0}}},
		{Op: "cells", Sheet: "Nowhere", Range: "A1", Values: [][]any{{1.0}}},
	}})
	if !errors.Is(err, ErrGone) || !strings.Contains(err.Error(), "edit 2") {
		t.Fatal(err)
	}
	_, err = a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0}}},
		{Op: "insert_rows", Sheet: "Données", Range: "2:2"},
	}})
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "batch of their own") {
		t.Fatal(err)
	}
	alice.quiet()
	if a.Version() != v || cell(t, a, "Données", "A1").Value != 2.0 {
		t.Fatal("the batch changed the workbook")
	}
}

func TestPreviewSheets(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	v := a.Version()
	p, err := a.Preview(Batch{OnBehalfOf: "u1", Edits: []Edit{
		{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0, "hi"}, {"=A1*2", nil}}},
	}})
	if err != nil || p.Version != v {
		t.Fatal(p, err)
	}
	want := "Données!A1: 10\nDonnées!B1: \"hi\"\nDonnées!A2: =A1*2\nDonnées!B2: (empty)\n"
	if p.Markup != want {
		t.Fatalf("got\n%s\nwant\n%s", p.Markup, want)
	}
	for op, want := range map[string]string{
		"insert_rows":    "rows 5 to 7 of Données inserted\n",
		"delete_columns": "column C of Données deleted\n",
	} {
		rng := "5:7"
		if op == "delete_columns" {
			rng = "C"
		}
		p, err := a.Preview(Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: op, Sheet: "Données", Range: rng}}})
		if err != nil || p.Markup != want {
			t.Errorf("%s: %q %v", op, p.Markup, err)
		}
	}
	alice.quiet()
	if a.Version() != v || cell(t, a, "Données", "A1").Value != 2.0 {
		t.Fatal("the preview changed the workbook")
	}
}

func TestEditInvalid(t *testing.T) {
	_, a := newBook(t)
	big := make([][]any, 1)
	for name, e := range map[string]Edit{
		"no sheet":      {Op: "cells", Range: "A1", Values: [][]any{{1.0}}},
		"unknown op":    {Op: "merge", Sheet: "Données", Range: "A1:B2"},
		"no range":      {Op: "cells", Sheet: "Données", Values: [][]any{{1.0}}},
		"column range":  {Op: "cells", Sheet: "Données", Range: "A:B", Values: [][]any{{1.0}}},
		"no values":     {Op: "cells", Sheet: "Données", Range: "A1"},
		"too wide":      {Op: "cells", Sheet: "Données", Range: "A1:B2", Values: [][]any{{1.0, 2.0, 3.0}}},
		"too tall":      {Op: "cells", Sheet: "Données", Range: "A1:B2", Values: [][]any{{1.0}, {2.0}, {3.0}}},
		"past the end":  {Op: "cells", Sheet: "Données", Range: "XFD1", Values: [][]any{{1.0, 2.0}}},
		"bad formula":   {Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{"=SUM(A1:A2"}}},
		"bad number":    {Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{math.NaN()}}},
		"bad type":      {Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{[]any{1.0}}}},
		"rows as cols":  {Op: "insert_columns", Sheet: "Données", Range: "5:7"},
		"cols as rows":  {Op: "delete_rows", Sheet: "Données", Range: "C:D"},
		"cell as rows":  {Op: "insert_rows", Sheet: "Données", Range: "A1"},
		"no lines":      {Op: "insert_rows", Sheet: "Données"},
		"too long text": {Op: "cells", Sheet: "Données", Range: "A1", Values: big},
	} {
		if name == "too long text" {
			big[0] = []any{strings.Repeat("x", 40000)}
			if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{e}}); !errors.Is(err, ErrRefused) {
				t.Errorf("%s: %v", name, err)
			}
			continue
		}
		if _, err := a.Preview(Batch{OnBehalfOf: "u1", Edits: []Edit{e}}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSheetConflict(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	id := sheetID(t, a)
	v := a.Version()
	f := alice.edit(typeCell(id, 1, 1, `{"v":7}`))
	if f.T != "ack" {
		t.Fatalf("%+v", f)
	}
	settle(t, a, f.V)
	for name, e := range map[string]Edit{
		"cells":       {Op: "cells", Sheet: "Données", Range: "A1:A2", Values: [][]any{{10.0}, {11.0}}},
		"insert_rows": {Op: "insert_rows", Sheet: "Données", Range: "9:9"},
	} {
		_, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{e}})
		if !errors.Is(err, ErrConflict) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{{Op: "cells", Sheet: "Données", Range: "A2", Values: [][]any{{11.0}}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Version: v, Edits: []Edit{{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0}}}}}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u1", Edits: []Edit{{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0}}}}}); err != nil {
		t.Fatal(err)
	}
}

func TestUndoSheet(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	id := sheetID(t, a)
	apply(t, a, Edit{Op: "cells", Sheet: "Données", Range: "A1:B1", Values: [][]any{{10.0, "=A1+1"}}})
	apply(t, a, Edit{Op: "delete_rows", Sheet: "Données", Range: "4:4"})
	f := alice.edit(typeCell(id, 2, 1, `{"v":"Alice"}`))
	if f.T != "ack" {
		t.Fatalf("%+v", f)
	}
	settle(t, a, f.V)
	rec, err := a.UndoMine(t.Context(), "u1")
	if err != nil || len(rec.Edits) != 2 {
		t.Fatalf("%+v %v", rec, err)
	}
	waitCell(t, a, Cell{Ref: "A3", Value: 2.0, Formula: "=SUM(A1:A2)"})
	for ref, want := range map[string]Cell{
		"A1": {Ref: "A1", Value: 2.0},
		"B1": {Ref: "B1", Value: 0.5, Format: "0%"},
		"A2": {Ref: "A2", Value: "Alice"},
		"A4": {Ref: "A4", Value: 40.0},
	} {
		if got := cell(t, a, "Données", ref); got != want {
			t.Errorf("%s: %+v, want %+v", ref, got, want)
		}
	}
}

func TestUndoSheetOwnLines(t *testing.T) {
	_, a := newBook(t)
	apply(t, a, Edit{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0}}})
	apply(t, a, Edit{Op: "insert_rows", Sheet: "Données", Range: "2"})
	apply(t, a, Edit{Op: "delete_rows", Sheet: "Données", Range: "2"})
	apply(t, a, Edit{Op: "insert_columns", Sheet: "Données", Range: "A"})
	apply(t, a, Edit{Op: "delete_columns", Sheet: "Données", Range: "A"})
	if rec, err := a.UndoMine(t.Context(), "u1"); err != nil || len(rec.Edits) != 5 {
		t.Fatalf("%+v %v", rec, err)
	}
	waitCell(t, a, Cell{Ref: "A3", Value: 5.0, Formula: "=SUM(A1:A2)"})
	for ref, want := range map[string]Cell{
		"A1": {Ref: "A1", Value: 2.0},
		"A2": {Ref: "A2", Value: 3.0},
		"A4": {Ref: "A4", Value: 40.0},
	} {
		if got := cell(t, a, "Données", ref); got != want {
			t.Errorf("%s: %+v, want %+v", ref, got, want)
		}
	}
}

func TestUndoSheetHumanLines(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	id := sheetID(t, a)
	apply(t, a, Edit{Op: "cells", Sheet: "Données", Range: "A2", Values: [][]any{{30.0}}})
	apply(t, a, Edit{Op: "insert_rows", Sheet: "Données", Range: "1"})
	waitCell(t, a, Cell{Ref: "A4", Value: 32.0, Formula: "=SUM(A2:A3)"})
	a.mu.Lock()
	seen := a.s.version
	a.mu.Unlock()
	for alice.v < seen {
		alice.next("op")
	}
	f := alice.edit(ot.Edit{{Op: ot.OpIns, ID: id, Dim: ot.DimRows, At: 3, N: 1}})
	if f.T != "ack" {
		t.Fatalf("%+v", f)
	}
	settle(t, a, f.V)
	waitCell(t, a, Cell{Ref: "A5", Value: 32.0, Formula: "=SUM(A2:A4)"})
	if _, err := a.UndoMine(t.Context(), "u1"); err != nil {
		t.Fatal(err)
	}
	waitCell(t, a, Cell{Ref: "A4", Value: 5.0, Formula: "=SUM(A1:A3)"})
	for ref, want := range map[string]Cell{
		"A1": {Ref: "A1", Value: 2.0},
		"A2": {Ref: "A2"},
		"A3": {Ref: "A3", Value: 3.0},
	} {
		if got := cell(t, a, "Données", ref); got != want {
			t.Errorf("%s: %+v, want %+v", ref, got, want)
		}
	}
}

func TestUndoSheetTwoUsers(t *testing.T) {
	_, a := newBook(t)
	apply(t, a, Edit{Op: "cells", Sheet: "Données", Range: "A2", Values: [][]any{{30.0}}})
	if _, err := a.Apply(t.Context(), Batch{OnBehalfOf: "u2", Edits: []Edit{{Op: "insert_rows", Sheet: "Données", Range: "1"}}}); err != nil {
		t.Fatal(err)
	}
	if rec, err := a.UndoMine(t.Context(), "u1"); err != nil || len(rec.Edits) != 1 {
		t.Fatalf("%+v %v", rec, err)
	}
	if c := cell(t, a, "Données", "A3"); c.Value != 3.0 {
		t.Fatalf("%+v", c)
	}
	if rec, err := a.UndoMine(t.Context(), "u2"); err != nil || len(rec.Edits) != 1 {
		t.Fatalf("%+v %v", rec, err)
	}
	if c := cell(t, a, "Données", "A2"); c.Value != 3.0 {
		t.Fatalf("%+v", c)
	}
}

func TestUndoSheetChanged(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	id := sheetID(t, a)
	apply(t, a, Edit{Op: "cells", Sheet: "Données", Range: "A1", Values: [][]any{{10.0}}})
	f := alice.edit(typeCell(id, 1, 1, `{"v":99}`))
	if f.T != "ack" {
		t.Fatalf("%+v", f)
	}
	settle(t, a, f.V)
	if _, err := a.UndoMine(t.Context(), "u1"); !errors.Is(err, ErrChanged) || !strings.Contains(err.Error(), "A1 of sheet") {
		t.Fatal(err)
	}
	if c := cell(t, a, "Données", "A1"); c.Value != 99.0 {
		t.Fatalf("%+v", c)
	}
}

func TestSheetMarks(t *testing.T) {
	r, a := newBook(t)
	alice := r.human("Alice")
	id := sheetID(t, a)
	apply(t, a, Edit{Op: "cells", Sheet: "Données", Range: "B2", Values: [][]any{{1.0, 2.0}, {3.0, 4.0}}})
	var eph struct {
		Recent []Mark `json:"recent"`
	}
	if err := json.Unmarshal(alice.next("eph").D, &eph); err != nil {
		t.Fatal(err)
	}
	if len(eph.Recent) != 1 || eph.Recent[0].Node != id || eph.Recent[0].Cells == nil || *eph.Recent[0].Cells != [4]int{2, 2, 3, 3} {
		t.Fatalf("%+v", eph)
	}
}
