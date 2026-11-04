package xlsx

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	topID   = "{9FE0EFFC-F649-7D41-B909-522703F4B7ED}"
	replyID = "{11111111-2222-4333-8444-555555555555}"
)

const vmlNote = `<v:shape id="_x0000_s1030" type="#_x0000_t202" style='position:absolute;margin-left:330pt;margin-top:2pt;width:104pt;height:69pt;z-index:1;visibility:hidden' fillcolor="#ffffe1" o:insetmode="auto">
  <x:ClientData ObjectType="Note">
   <x:Anchor>
    3, 15, 0, 2, 5, 13, 4, 14</x:Anchor>
   <x:Row>2</x:Row>
   <x:Column>1</x:Column>
  </x:ClientData>
 </v:shape>`

const vmlButton = `<v:shape id="_x0000_s1040" type="#_x0000_t201"><x:ClientData ObjectType="Button"><x:Row>5</x:Row><x:Column>0</x:Column></x:ClientData></v:shape>`

func commentsPackage(t *testing.T, forms bool) []byte {
	t.Helper()
	blank, err := os.ReadFile("../testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	p, err := opc.Open(blank, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	vml := `<xml xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel">` +
		`<o:shapelayout v:ext="edit"><o:idmap v:ext="edit" data="1"/></o:shapelayout>` + vmlNote
	if forms {
		vml += vmlButton
	}
	vml += `</xml>`
	comments := `<comments xmlns="` + mainNS + `"><authors><author>tc=` + topID + `</author><author>Bob</author></authors><commentList>` +
		`<comment ref="C2" authorId="0" shapeId="0"><text><t xml:space="preserve">[Threaded comment]

Comment:
    testing
</t></text></comment>` +
		`<comment ref="B3" authorId="1" shapeId="0"><text><r><rPr><b/><sz val="10"/></rPr><t>Bob:</t></r><r><t xml:space="preserve">
Sample note</t></r></text></comment></commentList></comments>`
	threaded := `<ThreadedComments xmlns="` + threadedNS + `" xmlns:x="` + mainNS + `">` +
		`<threadedComment ref="C2" dT="2020-09-21T17:32:33.59" personId="{7305E7A7-E410-8C4C-A5C1-67489F9B7D76}" id="` + topID + `"><text>testing</text></threadedComment>` +
		`<threadedComment ref="C2" dT="2020-09-22T08:00:00.00" personId="{7305E7A7-E410-8C4C-A5C1-67489F9B7D76}" id="` + replyID + `" parentId="` + topID + `"><text>and an answer</text></threadedComment>` +
		`</ThreadedComments>`
	persons := `<personList xmlns="` + threadedNS + `" xmlns:x="` + mainNS + `"><person displayName="Pankaj" id="{7305E7A7-E410-8C4C-A5C1-67489F9B7D76}" userId="pankaj" providerId="AD"/></personList>`
	sheet := `<worksheet xmlns="` + mainNS + `" xmlns:r="` + relNS + `"><dimension ref="A1:C3"/><sheetData>` +
		`<row r="1"><c r="A1"><v>1</v></c></row><row r="2"><c r="C2"/></row><row r="3"><c r="B3"/></row></sheetData><legacyDrawing r:id="rId1"/></worksheet>`
	sheetRels := string(opc.MarshalRelationships([]opc.Relationship{
		{ID: "rId1", Type: relVML, Target: "../drawings/vmlDrawing1.vml"},
		{ID: "rId2", Type: relComments, Target: "../comments1.xml"},
		{ID: "rId3", Type: relThreaded, Target: "../threadedComments/threadedComment1.xml"},
	}))
	bookRels, _ := p.Relationships("xl/workbook.xml")
	bookRels = append(bookRels, opc.Relationship{ID: "rId9", Type: relPerson, Target: "persons/person.xml"})
	if err := p.Set("xl/worksheets/sheet1.xml", []byte(sheet)); err != nil {
		t.Fatal(err)
	}
	if err := p.Set("xl/_rels/workbook.xml.rels", opc.MarshalRelationships(bookRels)); err != nil {
		t.Fatal(err)
	}
	for _, a := range []struct{ name, typ, data string }{
		{"xl/worksheets/_rels/sheet1.xml.rels", typeRels, sheetRels},
		{"xl/drawings/vmlDrawing1.vml", typeVML, vml},
		{"xl/comments1.xml", typeComments, comments},
		{"xl/threadedComments/threadedComment1.xml", typeThreaded, threaded},
		{"xl/persons/person.xml", typePersons, persons},
	} {
		if err := p.Add(a.name, a.typ, []byte(a.data)); err != nil {
			t.Fatal(err)
		}
	}
	out, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func cellAt(t *testing.T, tree *ot.Tree, row, col int) map[string]json.RawMessage {
	t.Helper()
	var f map[string]json.RawMessage
	for _, n := range tree.Children("book") {
		if n.Type == "sheet" {
			_ = json.Unmarshal(n.Grid.Cell(row, col), &f)
			break
		}
	}
	return f
}

func threadAt(t *testing.T, tree *ot.Tree, row, col int) []ThreadItem {
	t.Helper()
	var items []ThreadItem
	if err := json.Unmarshal(cellAt(t, tree, row, col)["thread"], &items); err != nil {
		t.Fatalf("no thread at %d,%d: %v", row, col, err)
	}
	return items
}

func noteAt(t *testing.T, tree *ot.Tree, row, col int) Note {
	t.Helper()
	var n Note
	if err := json.Unmarshal(cellAt(t, tree, row, col)["note"], &n); err != nil {
		t.Fatalf("no note at %d,%d: %v", row, col, err)
	}
	return n
}

func partOf(t *testing.T, data []byte, name string) string {
	t.Helper()
	p, err := opc.Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Read(name)
	if err != nil {
		return ""
	}
	return string(b)
}

func setCell(row, col int, fields string) ot.Edit {
	return ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(row, col, fields)}}}
}

func TestCommentsRead(t *testing.T) {
	data := commentsPackage(t, false)
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	items := threadAt(t, tree, 2, 3)
	want := []ThreadItem{
		{ID: topID, Author: "Pankaj", Date: "2020-09-21T17:32:33Z", Text: "testing"},
		{ID: replyID, Author: "Pankaj", Date: "2020-09-22T08:00:00Z", Text: "and an answer"},
	}
	if len(items) != 2 || items[0] != want[0] || items[1] != want[1] {
		t.Fatalf("thread is %+v", items)
	}
	if f := cellAt(t, tree, 2, 3); f["note"] != nil {
		t.Fatalf("the shadow note of the thread is a note: %s", f["note"])
	}
	n := noteAt(t, tree, 3, 2)
	if n.Author != "Bob" || n.Text != "Bob:\nSample note" || n.Shape == nil || *n.Shape != 0 {
		t.Fatalf("note is %+v", n)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, data) {
		t.Fatal("a workbook whose comments were not edited is written back differently")
	}
}

func TestCommentsEdit(t *testing.T) {
	data := commentsPackage(t, true)
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	const alice = "Alice"
	check := func(e ot.Edit, author string) error { return d.Check(tree, e, author) }
	reply := `{"id":"{AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE}","author":"Alice","date":"2026-11-03T10:00:00Z","text":"I agree"}`
	top := `{"id":"` + topID + `","author":"Pankaj","date":"2020-09-21T17:32:33Z","text":"testing"}`
	old := `{"id":"` + replyID + `","author":"Pankaj","date":"2020-09-22T08:00:00Z","text":"and an answer"}`

	for name, e := range map[string]ot.Edit{
		"a thread signed by another": setCell(2, 3, `{"thread":[`+top+`,`+old+`,`+strings.Replace(reply, "Alice", "Eve", 1)+`]}`),
		"the text of another":        setCell(2, 3, `{"thread":[`+strings.Replace(top, "testing", "changed", 1)+`,`+old+`]}`),
		"a note signed by another":   setCell(5, 5, `{"note":{"author":"Eve","text":"hi"}}`),
		"a note and a thread":        setCell(3, 2, `{"thread":[`+reply+`]}`),
		"an author changed":          setCell(3, 2, `{"note":{"author":"Alice","shape":0,"text":"hi"}}`),
		"a shape of its own":         setCell(5, 5, `{"note":{"author":"Alice","shape":0,"text":"hi"}}`),
		"a bad id":                   setCell(5, 5, `{"thread":[{"id":"1","author":"Alice","date":"2026-11-03T10:00:00Z","text":"hi"}]}`),
		"a bad date":                 setCell(5, 5, `{"thread":[{"id":"{AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE}","author":"Alice","date":"yesterday","text":"hi"}]}`),
		"an empty comment":           setCell(5, 5, `{"thread":[{"id":"{AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE}","author":"Alice","date":"2026-11-03T10:00:00Z","text":""}]}`),
		"a resolved reply":           setCell(2, 3, `{"thread":[`+top+`,`+strings.Replace(old, `"text"`, `"done":true,"text"`, 1)+`]}`),
		"a note on a row":            setCell(5, 0, `{"note":{"author":"Alice","text":"hi"}}`),
	} {
		if err := check(e, alice); err == nil {
			t.Errorf("%s is not refused", name)
		}
	}

	edits := []ot.Edit{
		setCell(2, 3, `{"thread":[`+top+`,`+old+`,`+reply+`]}`),
		setCell(3, 2, `{"note":{"author":"Bob","shape":0,"text":"Bob:\nChanged"}}`),
		setCell(5, 5, `{"note":{"author":"Alice","text":"A <new> note & more"}}`),
		setCell(6, 1, `{"thread":[{"id":"{AAAAAAAA-BBBB-4CCC-8DDD-000000000001}","author":"Alice","date":"2026-11-03T10:00:00Z","text":"Is this right?"}]}`),
	}
	for _, e := range edits {
		if err := check(e, alice); err != nil {
			t.Fatalf("%v refused: %v", e, err)
		}
		if err := tree.Apply(e); err != nil {
			t.Fatal(err)
		}
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, back, err := Open(saved, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := threadAt(t, back, 2, 3); len(got) != 3 || got[2].Text != "I agree" || got[2].Author != "Alice" {
		t.Fatalf("thread is %+v", got)
	}
	if n := noteAt(t, back, 3, 2); n.Text != "Bob:\nChanged" || n.Shape == nil {
		t.Fatalf("note is %+v", n)
	}
	if n := noteAt(t, back, 5, 5); n.Text != "A <new> note & more" || n.Author != "Alice" {
		t.Fatalf("note is %+v", n)
	}
	if got := threadAt(t, back, 6, 1); len(got) != 1 || got[0].Text != "Is this right?" {
		t.Fatalf("thread is %+v", got)
	}

	persons := partOf(t, saved, "xl/persons/person.xml")
	if !strings.Contains(persons, `displayName="Alice"`) || !strings.Contains(persons, `displayName="Pankaj"`) {
		t.Errorf("persons are %s", persons)
	}
	threaded := partOf(t, saved, "xl/threadedComments/threadedComment1.xml")
	for _, want := range []string{`ref="C2"`, `parentId="` + topID + `"`, `ref="A6"`} {
		if !strings.Contains(threaded, want) {
			t.Errorf("no %s in %s", want, threaded)
		}
	}
	comments := partOf(t, saved, "xl/comments1.xml")
	for _, want := range []string{`<author>tc=` + topID + `</author>`, `<author>Alice</author>`, `A &lt;new&gt; note &amp; more`, `Reply:`} {
		if !strings.Contains(comments, want) {
			t.Errorf("no %s in %s", want, comments)
		}
	}
	vml := partOf(t, saved, "xl/drawings/vmlDrawing1.vml")
	for _, want := range []string{`ObjectType="Button"`, `_x0000_s1030`, `<x:Row>2</x:Row>`, `<x:Row>4</x:Row>`, `<x:Column>4</x:Column>`} {
		if !strings.Contains(vml, want) {
			t.Errorf("no %s in %s", want, vml)
		}
	}
	if n := strings.Count(vml, `ObjectType="Note"`); n != 4 {
		t.Errorf("%d note shapes in %s", n, vml)
	}
}

func TestCommentsRemove(t *testing.T) {
	for _, forms := range []bool{false, true} {
		data := commentsPackage(t, forms)
		d, tree, err := Open(data, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		e := ot.Edit{{Op: ot.OpCel, ID: "S1", Cells: []ot.Cell{cell(2, 3, `{"thread":null}`), cell(3, 2, `{"note":null}`)}}}
		if err := d.Check(tree, e, "Alice"); err != nil {
			t.Fatal(err)
		}
		if err := tree.Apply(e); err != nil {
			t.Fatal(err)
		}
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatal(err)
		}
		_, back, err := Open(saved, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if f := cellAt(t, back, 3, 2); f["note"] != nil {
			t.Errorf("note is back: %s", f["note"])
		}
		sheet := partOf(t, saved, "xl/worksheets/sheet1.xml")
		if got := strings.Contains(sheet, "legacyDrawing"); got != forms {
			t.Errorf("forms %v: legacyDrawing is %v", forms, got)
		}
		for _, name := range []string{"xl/comments1.xml", "xl/threadedComments/threadedComment1.xml"} {
			if partOf(t, saved, name) != "" {
				t.Errorf("%s is still there", name)
			}
		}
		vml := partOf(t, saved, "xl/drawings/vmlDrawing1.vml")
		if forms && (!strings.Contains(vml, "Button") || strings.Contains(vml, "Note")) {
			t.Errorf("vml is %s", vml)
		}
		if !forms && vml != "" {
			t.Errorf("vml is still there")
		}
		rels := partOf(t, saved, "xl/worksheets/_rels/sheet1.xml.rels")
		if strings.Contains(rels, "comments") || strings.Contains(rels, "threaded") || !forms && strings.Contains(rels, "vml") {
			t.Errorf("rels are %s", rels)
		}
	}
}

// Notes move with their cells, those added as those read, and with the
// shapes of the drawing.
func TestCommentsMove(t *testing.T) {
	data := commentsPackage(t, true)
	d, tree, err := Open(data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	add := setCell(5, 5, `{"note":{"author":"Alice","text":"new"}}`)
	if err := d.Check(tree, add, "Alice"); err != nil {
		t.Fatal(err)
	}
	if err := tree.Apply(add); err != nil {
		t.Fatal(err)
	}
	for _, e := range []ot.Edit{
		{{Op: ot.OpIns, ID: "S1", Dim: ot.DimRows, At: 2, N: 3}},
		{{Op: ot.OpRem, ID: "S1", Dim: ot.DimCols, At: 1, N: 1}},
	} {
		if err := moveRows(t, d, tree, e); err != nil {
			t.Fatal(err)
		}
	}
	// the thread was at C2, the note at B3, the new one at E5, the button at A6
	if got := threadAt(t, tree, 5, 2); len(got) != 2 {
		t.Fatalf("thread is %+v", got)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, back, err := Open(saved, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if got := threadAt(t, back, 5, 2); len(got) != 2 || got[0].ID != topID {
		t.Fatalf("thread is %+v", got)
	}
	if n := noteAt(t, back, 8, 4); n.Text != "new" {
		t.Fatalf("note is %+v", n)
	}
	if n := noteAt(t, back, 6, 1); n.Text != "Bob:\nSample note" {
		t.Fatalf("note is %+v", n)
	}
	vml := partOf(t, saved, "xl/drawings/vmlDrawing1.vml")
	for _, want := range []string{`<x:Row>4</x:Row>`, `<x:Row>5</x:Row>`, `<x:Row>7</x:Row>`, `<x:Row>8</x:Row>`, `<x:Column>3</x:Column>`} {
		if !strings.Contains(vml, want) {
			t.Errorf("no %s in %s", want, vml)
		}
	}
}

func signed(t *testing.T, d *Document, tree *ot.Tree, e ot.Edit) {
	t.Helper()
	if err := d.Check(tree, e, "Auteur"); err != nil {
		t.Fatalf("%v refused: %v", e, err)
	}
	if err := tree.Apply(e); err != nil {
		t.Fatal(err)
	}
}

// Every workbook of the corpus takes a reply in each of its threads, its
// notes changed and a note and a thread added. When LOFFICE_COMMENTED is
// set, the packages are written there for the validator.
func TestCorpusComments(t *testing.T) {
	out := os.Getenv("LOFFICE_COMMENTED")
	commented, withComments := 0, 0
	for _, f := range workbooks(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data, Limits{})
		if err != nil {
			continue
		}
		sheet := ""
		count := map[string]int{}
		for _, n := range tree.Children("book") {
			if n.Type != "sheet" {
				continue
			}
			if sheet == "" {
				sheet = n.ID
			}
			for _, c := range commentsOf(n) {
				count[n.ID]++
				withComments++
				var edit string
				if c.thread != nil {
					var items []ThreadItem
					_ = json.Unmarshal(c.thread, &items)
					items = append(items, ThreadItem{ID: newGUID(), Author: "Auteur", Date: "2026-11-03T10:00:00Z", Text: "Réponse <&>"})
					edit = `{"thread":` + string(mustJSON(items)) + `}`
				} else {
					var note Note
					_ = json.Unmarshal(c.note, &note)
					note.Text += " (modifiée)"
					edit = `{"note":` + string(mustJSON(note)) + `}`
				}
				signed(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: n.ID, Cells: []ot.Cell{cell(c.row, c.col, edit)}}})
			}
		}
		if sheet == "" {
			continue
		}
		signed(t, d, tree, ot.Edit{{Op: ot.OpCel, ID: sheet, Cells: []ot.Cell{
			cell(900, 20, `{"note":{"author":"Auteur","text":"Nouvelle note"}}`),
			cell(901, 20, `{"thread":[{"id":"`+newGUID()+`","author":"Auteur","date":"2026-11-03T10:00:00Z","text":"Nouveau fil"}]}`),
		}}})
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		commented++
		_, again, err := Open(saved, Limits{})
		if err != nil {
			t.Fatalf("%s: commented workbook does not open: %v", f, err)
		}
		for _, n := range again.Children("book") {
			if n.Type != "sheet" {
				continue
			}
			want := count[n.ID]
			if n.ID == sheet {
				want += 2
			}
			if got := len(commentsOf(n)); got != want {
				t.Fatalf("%s: %d comments read back in %s, want %d", f, got, n.ID, want)
			}
		}
		if n := noteAt(t, again, 900, 20); n.Text != "Nouvelle note" || n.Author != "Auteur" {
			t.Fatalf("%s: note read back as %+v", f, n)
		}
		if got := threadAt(t, again, 901, 20); len(got) != 1 || got[0].Text != "Nouveau fil" {
			t.Fatalf("%s: thread read back as %+v", f, got)
		}
		if out != "" {
			rel, _ := filepath.Rel(filepath.Join("..", "corpus", "files"), f)
			dst := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, saved, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("%d workbooks commented, %d comments edited", commented, withComments)
}
