package xlsx

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	relPerson        = "http://schemas.microsoft.com/office/2017/10/relationships/person"
	relThreaded      = "http://schemas.microsoft.com/office/2017/10/relationships/threadedComment"
	typeComments     = "application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml"
	typeVML          = "application/vnd.openxmlformats-officedocument.vmlDrawing"
	typeThreaded     = "application/vnd.ms-excel.threadedcomments+xml"
	typePersons      = "application/vnd.ms-excel.person+xml"
	threadedNS       = "http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments"
	threadedTime     = "2006-01-02T15:04:05.00"
	personsName      = "xl/persons/person.xml"
	maxThread        = 1000
	maxAuthor        = 255
	threadedPreamble = "[Threaded comment]\n\nYour version of Excel allows you to read this threaded comment; however, any edits to it " +
		"will get removed if the file is opened in a newer version of Excel. Learn more: https://go.microsoft.com/fwlink/?linkid=870924\n\n"
)

// A cell holds a note, the comment Excel draws as a red triangle, in its
// "note" field, or a threaded comment, Excel 365's, in its "thread" field,
// never both. Notes follow the rows and columns inserted or removed with
// their cells.

// Note is the "note" of a cell: its author, its text without formatting,
// and Shape, which of the notes of the workbook as read it is, so that the
// size and place of its box are kept. A note added has no Shape.
type Note struct {
	Author string `json:"author"`
	Shape  *int   `json:"shape,omitempty"`
	Text   string `json:"text"`
}

// ThreadItem is a comment of the "thread" of a cell, the first one
// starting the thread, the others answering it. Done, on the first one
// only, tells that the thread is resolved.
type ThreadItem struct {
	ID     string `json:"id"`
	Author string `json:"author"`
	Date   string `json:"date"`
	Text   string `json:"text"`
	Done   bool   `json:"done,omitempty"`
}

// commentCell is the comment of a cell.
type commentCell struct {
	row, col     int
	note, thread json.RawMessage
}

// person is an author of threaded comments.
type person struct{ name, id string }

// sheetComments is what a worksheet had of comments, as read: the parts it
// kept them in, the XML Excel wrote for each note and thread, and where they
// were.
type sheetComments struct {
	comments, threaded, vml string
	cells                   []commentCell
	notes                   []origNote
	items                   map[string]*xmldom.Element
}

// origNote is a note as the workbook had it: the text it holds, with
// its formatting, and the shape that draws it.
type origNote struct {
	author, text string
	rich         *xmldom.Element
	shape        string
	row, col     int
}

func (c commentCell) same(o commentCell) bool {
	return c.row == o.row && c.col == o.col && bytes.Equal(c.note, o.note) && bytes.Equal(c.thread, o.thread)
}

var (
	vmlShape = regexp.MustCompile(`(?s)<v:shape\s.*?</v:shape>`)
	vmlID    = regexp.MustCompile(`_x0000_s(\d+)`)
	vmlRow   = regexp.MustCompile(`<x:Row>\s*(\d+)\s*</x:Row>`)
	vmlCol   = regexp.MustCompile(`<x:Column>\s*(\d+)\s*</x:Column>`)
	vmlSpot  = regexp.MustCompile(`(?s)<x:Anchor>\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*</x:Anchor>`)
)

func isNoteShape(shape string) bool {
	return strings.Contains(shape, `ObjectType="Note"`) || strings.Contains(shape, `ObjectType='Note'`)
}

// readPersons reads the authors of threaded comments.
func (r *reader) persons() {
	d := r.d
	d.personsName = target(d.bookName, d.bookRels, relPerson)
	if d.personsName == "" || !d.pkg.Has(d.personsName) {
		d.personsName = ""
		return
	}
	doc, err := r.part(d.personsName)
	if err != nil {
		return
	}
	d.personsDoc = doc
	for _, p := range doc.Root.Elements() {
		if p.Local == "person" && p.Get("id") != "" {
			d.persons = append(d.persons, person{p.Get("displayName"), p.Get("id")})
		}
	}
}

func (d *Document) personName(id string) string {
	for _, p := range d.persons {
		if p.id == id {
			return p.name
		}
	}
	return ""
}

// comments adds to the cells of a sheet its notes and threaded comments.
func (r *reader) comments(id string, root *xmldom.Element, part *sheetPart, cells []ot.Cell) []ot.Cell {
	cm := &sheetComments{items: map[string]*xmldom.Element{}}
	part.cm = cm
	rels, _ := r.d.pkg.Relationships(part.name)
	find := func(typ string) string {
		for _, x := range rels {
			if x.Type == typ && !x.External {
				if name, err := opc.Resolve(part.name, x.Target); err == nil && r.d.pkg.Has(name) {
					return name
				}
			}
		}
		return ""
	}
	if ld := root.Child(mainNS, "legacyDrawing"); ld != nil {
		for _, x := range rels {
			if x.ID == relID(ld, root) && x.Type == relVML && !x.External {
				if name, err := opc.Resolve(part.name, x.Target); err == nil && r.d.pkg.Has(name) {
					cm.vml = name
				}
			}
		}
	}
	cm.comments, cm.threaded = find(relComments), find(relThreaded)

	threads := r.threads(cm)
	shapes := map[[2]int]string{}
	if cm.vml != "" {
		data, _ := r.d.pkg.Read(cm.vml)
		for _, s := range vmlShape.FindAllString(string(data), -1) {
			if row, col, ok := shapeCell(s); ok && isNoteShape(s) {
				shapes[[2]int{row, col}] = s
			}
		}
	}
	notes := map[[2]int]Note{}
	if cm.comments != "" {
		doc, err := r.part(cm.comments)
		if err == nil && doc.Root.Space == mainNS && doc.Root.Local == "comments" {
			authors := elements(doc.Root.Child(mainNS, "authors"), "author")
			for _, c := range elements(doc.Root.Child(mainNS, "commentList"), "comment") {
				row, col, ok := formula.ParseCell(c.Get("ref"))
				text := c.Child(mainNS, "text")
				if !ok || text == nil {
					continue
				}
				item, err := readItem(text.Bytes())
				if err != nil {
					continue
				}
				n := Note{Text: item.text}
				if a := atoi(c.Get("authorId")); a < len(authors) {
					n.Author = authors[a].Text()
				}
				at := [2]int{row, col}
				if _, dup := notes[at]; dup || threads[at] != nil {
					continue
				}
				notes[at] = n
				cm.notes = append(cm.notes, origNote{author: n.Author, text: n.Text, rich: text, shape: shapes[[2]int{row - 1, col - 1}], row: row, col: col})
			}
		}
	}

	slices.SortFunc(cm.notes, func(a, b origNote) int {
		if a.row != b.row {
			return a.row - b.row
		}
		return a.col - b.col
	})
	for i, o := range cm.notes {
		at := [2]int{o.row, o.col}
		n := notes[at]
		n.Shape = &i
		notes[at] = n
	}

	found := map[[2]int]*commentCell{}
	for at, n := range notes {
		found[at] = &commentCell{row: at[0], col: at[1], note: mustJSON(n)}
	}
	for at, t := range threads {
		found[at] = &commentCell{row: at[0], col: at[1], thread: mustJSON(t)}
	}
	for _, c := range found {
		cm.cells = append(cm.cells, *c)
	}
	slices.SortFunc(cm.cells, func(a, b commentCell) int {
		if a.row != b.row {
			return a.row - b.row
		}
		return a.col - b.col
	})
	return withComments(cells, cm.cells)
}

// shapeCell is the cell, counted from 0, a shape of notes is that of.
func shapeCell(shape string) (row, col int, ok bool) {
	r, c := vmlRow.FindStringSubmatch(shape), vmlCol.FindStringSubmatch(shape)
	if r == nil || c == nil {
		return 0, 0, false
	}
	return atoi(r[1]), atoi(c[1]), true
}

// threads reads the threaded comments of a sheet, by cell.
func (r *reader) threads(cm *sheetComments) map[[2]int][]ThreadItem {
	out := map[[2]int][]ThreadItem{}
	if cm.threaded == "" {
		return out
	}
	doc, err := r.part(cm.threaded)
	if err != nil || doc.Root.Space != threadedNS || doc.Root.Local != "ThreadedComments" {
		return out
	}
	for _, e := range doc.Root.Elements() {
		if e.Space != threadedNS || e.Local != "threadedComment" {
			continue
		}
		row, col, ok := formula.ParseCell(e.Get("ref"))
		text := e.Child(threadedNS, "text")
		if !ok || e.Get("id") == "" || text == nil {
			continue
		}
		item := ThreadItem{ID: e.Get("id"), Author: r.d.personName(e.Get("personId")), Text: decodeEscapes(strings.TrimSuffix(text.Text(), "\n")), Done: truthy(e.Get("done"))}
		if t, err := parseThreadedTime(e.Get("dT")); err == nil {
			item.Date = t.UTC().Format(time.RFC3339)
		}
		cm.items[item.ID] = e
		at := [2]int{row, col}
		if e.Get("parentId") == "" && len(out[at]) > 0 {
			out[at] = slices.Insert(out[at], 0, item)
			continue
		}
		out[at] = append(out[at], item)
	}
	for at, items := range out {
		for i := range items {
			items[i].Done = i == 0 && items[i].Done
		}
		out[at] = items[:min(len(items), maxThread)]
	}
	return out
}

func parseThreadedTime(s string) (time.Time, error) {
	for _, layout := range []string{threadedTime, "2006-01-02T15:04:05", time.RFC3339, "2006-01-02T15:04:05.999999999"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("xlsx: bad date %q", s)
}

// withComments gives the cells the notes and threads they hold.
func withComments(cells []ot.Cell, found []commentCell) []ot.Cell {
	if len(found) == 0 {
		return cells
	}
	at := map[[2]int]commentCell{}
	for _, c := range found {
		at[[2]int{c.row, c.col}] = c
	}
	for i, c := range cells {
		cc, ok := at[[2]int{c.Row, c.Col}]
		if !ok {
			continue
		}
		var f cellFields
		_ = json.Unmarshal(c.Fields, &f)
		f.Note, f.Thread = cc.note, cc.thread
		cells[i].Fields = f.marshal()
		delete(at, [2]int{c.Row, c.Col})
	}
	for _, cc := range at {
		f := cellFields{Note: cc.note, Thread: cc.thread}
		cells = append(cells, ot.Cell{Row: cc.row, Col: cc.col, Fields: f.marshal()})
	}
	slices.SortFunc(cells, func(a, b ot.Cell) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return cells
}

// checkComment tells whether a note or a thread a client sets is one it can
// set over what the cell holds. A comment added is signed by author, and
// only the comments of author are edited.
func checkComment(key string, v, old json.RawMessage, author string) bool {
	switch key {
	case "note":
		var n, o Note
		if !decode(v, &n) || len(utf16.Encode([]rune(n.Text))) > maxText {
			return false
		}
		if old == nil {
			return n.Shape == nil && n.Author == author && validAuthor(n.Author)
		}
		return decode(old, &o) && n.Author == o.Author && (n.Shape == nil) == (o.Shape == nil) && (n.Shape == nil || *n.Shape == *o.Shape)
	case "thread":
		var items, olds []ThreadItem
		if !decode(v, &items) || len(items) == 0 || len(items) > maxThread {
			return false
		}
		if old != nil && !decode(old, &olds) {
			return false
		}
		return checkThread(items, olds, author)
	}
	return false
}

func checkThread(items, olds []ThreadItem, author string) bool {
	seen := map[string]bool{}
	last := -1
	for i, it := range items {
		if !validGUID(it.ID) || seen[it.ID] || it.Done && i > 0 {
			return false
		}
		seen[it.ID] = true
		at := slices.IndexFunc(olds, func(o ThreadItem) bool { return o.ID == it.ID })
		if at < 0 {
			if it.Author != author || !validAuthor(author) || !validText(it.Text) {
				return false
			}
			if _, err := time.Parse(time.RFC3339, it.Date); err != nil {
				return false
			}
			continue
		}
		o := olds[at]
		if at < last || it.Author != o.Author || it.Date != o.Date || it.Text != o.Text && (it.Author != author || !validText(it.Text)) {
			return false
		}
		last = at
	}
	return true
}

func validText(s string) bool {
	return s != "" && len(utf16.Encode([]rune(s))) <= maxText
}

func validAuthor(s string) bool {
	return s != "" && len(utf16.Encode([]rune(s))) <= maxAuthor && strings.IndexFunc(s, unicode.IsControl) < 0
}

// validGUID tells whether s is a GUID between braces, as Excel writes them.
func validGUID(s string) bool {
	if len(s) != 38 || s[0] != '{' || s[37] != '}' {
		return false
	}
	for i := 1; i < 37; i++ {
		switch c := s[i]; {
		case i == 9 || i == 14 || i == 19 || i == 24:
			if c != '-' {
				return false
			}
		case !(c >= '0' && c <= '9' || c >= 'A' && c <= 'F'):
			return false
		}
	}
	return true
}

func newGUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6], b[8] = b[6]&0x0f|0x40, b[8]&0x3f|0x80
	return fmt.Sprintf("{%X-%X-%X-%X-%X}", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func decode(raw json.RawMessage, v any) bool {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v) == nil && !dec.More()
}

// checkExclusive tells whether a cell holds no note and thread at once, once
// the fields set are set over those it has.
func checkExclusive(set map[string]json.RawMessage, old json.RawMessage) bool {
	var was map[string]json.RawMessage
	_ = json.Unmarshal(old, &was)
	has := func(key string) bool {
		if v, ok := set[key]; ok {
			return !bytes.Equal(v, []byte("null"))
		}
		return was[key] != nil
	}
	return !has("note") || !has("thread")
}

// commentsOf are the notes and threads of the cells of a sheet node, by
// row, then column.
func commentsOf(n *ot.Node) []commentCell {
	var out []commentCell
	if n == nil || n.Grid == nil {
		return out
	}
	n.Grid.Each(1, formula.MaxRows, func(row, col int, raw json.RawMessage) bool {
		if col > 0 && (bytes.Contains(raw, []byte(`"note":`)) || bytes.Contains(raw, []byte(`"thread":`))) {
			var f cellFields
			if json.Unmarshal(raw, &f) == nil && (f.Note != nil || f.Thread != nil) {
				out = append(out, commentCell{row, col, f.Note, f.Thread})
			}
		}
		return true
	})
	return out
}

// comments writes the notes and threads of a sheet into its parts, when
// they are not those it was read with.
func (w *writer) comments(root *xmldom.Element, n *ot.Node, p *sheetPart) error {
	if p.cm == nil {
		p.cm = &sheetComments{items: map[string]*xmldom.Element{}}
	}
	cm := p.cm
	cells := commentsOf(n)
	if !w.force && len(w.moves) == 0 && slices.EqualFunc(cm.cells, cells, commentCell.same) {
		return nil
	}
	rels, err := w.pkg.Relationships(p.name)
	if err != nil {
		return err
	}
	rw := &sheetRels{list: rels}

	var threads []commentCell
	for _, c := range cells {
		if c.thread != nil {
			threads = append(threads, c)
		}
	}

	if len(cells) == 0 {
		w.dropPart(rw, p.name, cm.comments, relComments)
		w.dropPart(rw, p.name, cm.threaded, relThreaded)
		if err := w.writeVML(root, rw, p, n.ID, nil); err != nil {
			return err
		}
		return w.putRels(p, rw)
	}

	ids := w.threadIDs(threads)
	authors := newAuthors()
	var list []*xmldom.Element
	for _, c := range cells {
		var comment *xmldom.Element
		if c.thread != nil {
			comment = w.shadow(c, ids, authors)
		} else {
			comment = w.noteComment(c, cm, authors)
		}
		list = append(list, comment)
	}
	doc := xmldom.New(mainNS, "comments", "xmlns", mainNS)
	as := xmldom.New(mainNS, "authors")
	for _, a := range authors.list {
		e := xmldom.New(mainNS, "author")
		e.Append(xmldom.EscapeText(a))
		as.Append(e)
	}
	doc.Append(as)
	cl := xmldom.New(mainNS, "commentList")
	for _, c := range list {
		cl.Append(c)
	}
	doc.Append(cl)
	if cm.comments == "" {
		cm.comments = w.freeName("xl/comments%d.xml")
		rw.add(relComments, "/"+cm.comments)
	}
	if err := w.put(cm.comments, typeComments, append([]byte(xmlHeader), doc.Bytes()...)); err != nil {
		return err
	}

	if len(threads) == 0 {
		w.dropPart(rw, p.name, cm.threaded, relThreaded)
		cm.threaded = ""
	} else {
		if cm.threaded == "" {
			cm.threaded = w.freeName("xl/threadedComments/threadedComment%d.xml")
			rw.add(relThreaded, "/"+cm.threaded)
		}
		if err := w.writeThreaded(cm, threads, ids); err != nil {
			return err
		}
	}
	if err := w.writeVML(root, rw, p, n.ID, cells); err != nil {
		return err
	}
	return w.putRels(p, rw)
}

// authorList numbers authors in the order they come.
type authorList struct {
	list []string
}

func newAuthors() *authorList { return &authorList{} }

func (a *authorList) id(name string) int {
	if i := slices.Index(a.list, name); i >= 0 {
		return i
	}
	a.list = append(a.list, name)
	return len(a.list) - 1
}

func (w *writer) noteComment(c commentCell, cm *sheetComments, authors *authorList) *xmldom.Element {
	var n Note
	_ = json.Unmarshal(c.note, &n)
	e := xmldom.New(mainNS, "comment", "ref", formula.CellName(c.row, c.col), "authorId", strconv.Itoa(authors.id(n.Author)), "shapeId", "0")
	text := xmldom.New(mainNS, "text")
	if n.Shape != nil && *n.Shape >= 0 && *n.Shape < len(cm.notes) {
		if o := cm.notes[*n.Shape]; o.author == n.Author && o.text == n.Text && o.rich.Name == "text" {
			e.Append(o.rich.Clone())
			return e
		}
	}
	text.Append(plainRun(n.Text))
	e.Append(text)
	return e
}

func plainRun(s string) *xmldom.Element {
	t := xmldom.New(mainNS, "t", "xml:space", "preserve")
	t.Append(xmldom.EscapeText(encodeEscapes(s)))
	return t
}

// shadow is the note Excel keeps for a threaded comment, which versions
// that do not know them show.
func (w *writer) shadow(c commentCell, ids map[string][]string, authors *authorList) *xmldom.Element {
	var items []ThreadItem
	_ = json.Unmarshal(c.thread, &items)
	top := ids[cellKey(c)][0]
	var b strings.Builder
	b.WriteString(threadedPreamble)
	for i, it := range items {
		if i == 0 {
			b.WriteString("Comment:\n    ")
		} else {
			b.WriteString("Reply:\n    ")
		}
		b.WriteString(it.Text)
		b.WriteByte('\n')
	}
	e := xmldom.New(mainNS, "comment", "ref", formula.CellName(c.row, c.col), "authorId", strconv.Itoa(authors.id("tc="+top)), "shapeId", "0")
	text := xmldom.New(mainNS, "text")
	text.Append(plainRun(b.String()))
	e.Append(text)
	return e
}

func cellKey(c commentCell) string {
	return strconv.Itoa(c.row) + "," + strconv.Itoa(c.col)
}

// threadIDs are the ids the threaded comments of a sheet are written with,
// by cell: those they have, unless the workbook already has them.
func (w *writer) threadIDs(threads []commentCell) map[string][]string {
	out := map[string][]string{}
	for _, c := range threads {
		var items []ThreadItem
		_ = json.Unmarshal(c.thread, &items)
		ids := make([]string, len(items))
		for i, it := range items {
			id := it.ID
			for w.usedIDs[id] || !validGUID(id) {
				id = newGUID()
			}
			w.usedIDs[id] = true
			ids[i] = id
		}
		out[cellKey(c)] = ids
	}
	return out
}

func (w *writer) writeThreaded(cm *sheetComments, threads []commentCell, ids map[string][]string) error {
	root := xmldom.New(threadedNS, "ThreadedComments", "xmlns", threadedNS, "xmlns:x", mainNS)
	for _, c := range threads {
		var items []ThreadItem
		_ = json.Unmarshal(c.thread, &items)
		for i, it := range items {
			id := ids[cellKey(c)][i]
			e := cm.items[it.ID]
			if e != nil && ids[cellKey(c)][i] == it.ID {
				e = e.Clone()
				if decodeEscapes(strings.TrimSuffix(e.Child(threadedNS, "text").Text(), "\n")) != it.Text {
					e.Content = nil
					e.Append(w.threadedText(it.Text))
				}
			} else {
				e = xmldom.New(threadedNS, "threadedComment")
				e.Append(w.threadedText(it.Text))
			}
			e.Set("ref", formula.CellName(c.row, c.col))
			if t, err := time.Parse(time.RFC3339, it.Date); err == nil && (e.Get("dT") == "" || e.Get("dT") != t.UTC().Format(threadedTime)) {
				e.Set("dT", t.UTC().Format(threadedTime))
			}
			if w.d.personName(e.Get("personId")) != it.Author || e.Get("personId") == "" {
				e.Set("personId", w.personID(it.Author))
			}
			e.Set("id", id)
			setAttr(e, "done", boolAttr(i == 0 && it.Done))
			if i == 0 {
				e.Unset("parentId")
			} else {
				e.Set("parentId", ids[cellKey(c)][0])
			}
			root.Append(e)
		}
	}
	return w.put(cm.threaded, typeThreaded, append([]byte(xmlHeader), root.Bytes()...))
}

func (w *writer) threadedText(s string) *xmldom.Element {
	t := xmldom.New(threadedNS, "text", "xml:space", "preserve")
	t.Append(xmldom.EscapeText(encodeEscapes(s)))
	return t
}

// personID is the id of the person of that name among those threaded
// comments are signed by, who is added when the workbook does not have it.
func (w *writer) personID(name string) string {
	for _, p := range w.persons {
		if p.name == name {
			return p.id
		}
	}
	id := newGUID()
	w.persons = append(w.persons, person{name, id})
	w.newPersons = append(w.newPersons, person{name, id})
	return id
}

// writePersons adds to the persons part those threaded comments were signed
// by since the workbook was read.
func (w *writer) writePersons() error {
	if len(w.newPersons) == 0 {
		return nil
	}
	root := xmldom.New(threadedNS, "personList", "xmlns", threadedNS, "xmlns:x", mainNS)
	if w.d.personsDoc != nil {
		root = w.d.personsDoc.Root.Clone()
	}
	for _, p := range w.newPersons {
		root.Append(xmldom.New(threadedNS, "person", "displayName", p.name, "id", p.id, "userId", p.name, "providerId", "None"))
	}
	name := w.d.personsName
	if name == "" {
		name = personsName
		if w.pkg.Has(name) {
			return fmt.Errorf("xlsx: %s exists without a relationship", name)
		}
		w.addRel(relPerson, w.target(name))
	}
	return w.put(name, typePersons, append([]byte(xmlHeader), root.Bytes()...))
}

// sheetRels are the relationships of a sheet as written.
type sheetRels struct {
	list    []opc.Relationship
	changed bool
}

func (r *sheetRels) add(typ, target string) string {
	ids := map[string]bool{}
	for _, x := range r.list {
		ids[x.ID] = true
	}
	id := ""
	for n := 1; id == "" || ids[id]; n++ {
		id = "rId" + strconv.Itoa(n)
	}
	r.list = append(r.list, opc.Relationship{ID: id, Type: typ, Target: target})
	r.changed = true
	return id
}

func (r *sheetRels) remove(id string) {
	r.list = slices.DeleteFunc(r.list, func(x opc.Relationship) bool {
		if x.ID == id {
			r.changed = true
			return true
		}
		return false
	})
}

// dropPart removes a part of a sheet and what points to it.
func (w *writer) dropPart(rw *sheetRels, sheet, name, typ string) {
	if name == "" {
		return
	}
	for _, x := range slices.Clone(rw.list) {
		if x.Type == typ && !x.External {
			if target, err := opc.Resolve(sheet, x.Target); err == nil && target == name {
				rw.remove(x.ID)
			}
		}
	}
	if w.pkg.Has(name) {
		_ = w.pkg.Remove(name)
	}
}

func (w *writer) putRels(p *sheetPart, rw *sheetRels) error {
	return w.putPartRels(p.name, rw)
}

// putPartRels writes the relationships of a part, removed when it has none.
func (w *writer) putPartRels(part string, rw *sheetRels) error {
	if !rw.changed {
		return nil
	}
	name := opc.RelsName(part)
	if len(rw.list) == 0 {
		if w.pkg.Has(name) {
			return w.pkg.Remove(name)
		}
		return nil
	}
	return w.put(name, typeRels, opc.MarshalRelationships(rw.list))
}

// writeVML writes the shapes of the notes and threads of a sheet beside those of other
// objects the drawing holds.
func (w *writer) writeVML(root *xmldom.Element, rw *sheetRels, p *sheetPart, own string, cells []commentCell) error {
	cm := p.cm
	var kept string
	if cm.vml != "" && w.pkg.Has(cm.vml) {
		data, err := w.pkg.Read(cm.vml)
		if err != nil {
			return err
		}
		kept = string(data)
	}
	var others []string
	if kept != "" {
		for _, s := range vmlShape.FindAllString(kept, -1) {
			if !isNoteShape(s) {
				others = append(others, s)
			}
		}
	}
	ld := root.Child(mainNS, "legacyDrawing")
	if len(cells) == 0 && len(others) == 0 {
		if cm.vml != "" {
			w.dropPart(rw, p.name, cm.vml, relVML)
			cm.vml = ""
		}
		if ld != nil {
			root.Remove(ld)
		}
		return nil
	}

	next := 1025
	for _, m := range vmlID.FindAllStringSubmatch(kept, -1) {
		next = max(next, atoi(m[1])+1)
	}
	var shapes strings.Builder
	var mover *mover
	if len(w.moves) > 0 {
		mover = w.mover()
	}
	for _, s := range others {
		if mover != nil {
			s = shiftVML(s, mover, own)
		}
		shapes.WriteString(s)
	}
	for _, c := range cells {
		var n Note
		_ = json.Unmarshal(c.note, &n)
		shapes.WriteString(w.noteShape(c, &n, cm, &next))
	}

	var data string
	if kept != "" {
		first := vmlShape.FindStringIndex(kept)
		end := strings.LastIndex(kept, "</xml>")
		switch {
		case first == nil && end >= 0:
			data = kept[:end] + shapes.String() + kept[end:]
		case first != nil && end >= first[1]:
			data = kept[:first[0]] + shapes.String() + kept[end:]
		default:
			kept = ""
		}
	}
	if kept == "" {
		data = vmlHead + shapes.String() + "</xml>"
	}
	if cm.vml == "" {
		cm.vml = w.freeName("xl/drawings/vmlDrawing%d.vml")
	}
	if err := w.put(cm.vml, typeVML, []byte(data)); err != nil {
		return err
	}
	if ld == nil {
		ld = xmldom.New(mainNS, prefixOf(root)+"legacyDrawing")
		ld.Set(relsPrefix(root)+":id", rw.add(relVML, "/"+cm.vml))
		root.Insert(ld, worksheetOrder)
	}
	return nil
}

const vmlHead = `<xml xmlns:v="urn:schemas-microsoft-com:vml" xmlns:o="urn:schemas-microsoft-com:office:office" xmlns:x="urn:schemas-microsoft-com:office:excel">` + "\n" +
	` <o:shapelayout v:ext="edit">` + "\n" + `  <o:idmap v:ext="edit" data="1"/>` + "\n" + ` </o:shapelayout>` +
	`<v:shapetype id="_x0000_t202" coordsize="21600,21600" o:spt="202" path="m,l,21600r21600,l21600,xe">` + "\n" +
	`  <v:stroke joinstyle="miter"/>` + "\n" + `  <v:path gradientshapeok="t" o:connecttype="rect"/>` + "\n" + ` </v:shapetype>`

// noteShape is the shape of a note: the one it had, moved with its cell,
// or a new one, hidden until the pointer is over the cell.
func (w *writer) noteShape(c commentCell, n *Note, cm *sheetComments, next *int) string {
	if n.Shape != nil && *n.Shape >= 0 && *n.Shape < len(cm.notes) && cm.notes[*n.Shape].shape != "" && !w.usedShapes[cm.notes[*n.Shape].shape] {
		o := cm.notes[*n.Shape]
		w.usedShapes[o.shape] = true
		return moveShape(o.shape, o.row-1, o.col-1, c.row-1, c.col-1)
	}
	id := *next
	*next++
	return fmt.Sprintf(`<v:shape id="_x0000_s%d" type="#_x0000_t202" style='position:absolute;margin-left:59.25pt;margin-top:1.5pt;width:108pt;height:59.25pt;z-index:%d;visibility:hidden' fillcolor="#ffffe1" o:insetmode="auto">`+"\n"+
		`  <v:fill color2="#ffffe1"/>`+"\n"+`  <v:shadow on="t" color="black" obscured="t"/>`+"\n"+`  <v:path o:connecttype="none"/>`+"\n"+
		`  <v:textbox style='mso-direction-alt:auto'>`+"\n"+`   <div style='text-align:left'></div>`+"\n"+`  </v:textbox>`+"\n"+
		`  <x:ClientData ObjectType="Note">`+"\n"+`   <x:MoveWithCells/>`+"\n"+`   <x:SizeWithCells/>`+"\n"+
		`   <x:Anchor>%d, 15, %d, 2, %d, 15, %d, 16</x:Anchor>`+"\n"+`   <x:AutoFill>False</x:AutoFill>`+"\n"+
		`   <x:Row>%d</x:Row>`+"\n"+`   <x:Column>%d</x:Column>`+"\n"+`  </x:ClientData>`+"\n"+` </v:shape>`,
		id, id-1024, c.col, max(c.row-2, 0), c.col+2, c.row+2, c.row-1, c.col-1)
}

// moveShape moves a shape of a note from one cell to another, counted from 0.
func moveShape(shape string, fromRow, fromCol, row, col int) string {
	shape = vmlRow.ReplaceAllString(shape, "<x:Row>"+strconv.Itoa(row)+"</x:Row>")
	shape = vmlCol.ReplaceAllString(shape, "<x:Column>"+strconv.Itoa(col)+"</x:Column>")
	return vmlSpot.ReplaceAllStringFunc(shape, func(s string) string {
		g := vmlSpot.FindStringSubmatch(s)
		v := func(i, delta int) string { return strconv.Itoa(max(atoi(g[i])+delta, 0)) }
		return "<x:Anchor>" + strings.Join([]string{v(1, col-fromCol), g[2], v(3, row-fromRow), g[4], v(5, col-fromCol), g[6], v(7, row-fromRow), g[8]}, ", ") + "</x:Anchor>"
	})
}

// shiftVML moves the cells the shapes of a drawing are those of, once the
// rows or columns of a sheet moved.
func shiftVML(data string, m *mover, own string) string {
	return vmlAnchor.ReplaceAllStringFunc(data, func(s string) string {
		g := vmlAnchor.FindStringSubmatch(s)
		i, _ := strconv.Atoi(g[2])
		return "<x:" + g[1] + ">" + strconv.Itoa(m.index(i, own, g[1] == "Row")) + "</x:" + g[1] + ">"
	})
}
