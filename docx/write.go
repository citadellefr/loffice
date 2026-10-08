package docx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	xmlHeader  = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\r\n"
	typeHeader = "application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"
	typeFooter = "application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"
	typeRels   = "application/vnd.openxmlformats-package.relationships+xml"
)

// Save writes the document as the tree has it. Only the parts whose
// content changed are rewritten: the main part when the body or the last
// section did, a header or footer when its own blocks did.
func (d *Document) Save(tree *ot.Tree) ([]byte, error) {
	pkg, err := opc.Open(d.original, Limits)
	if err != nil {
		return nil, err
	}
	w := &writer{d: d, pkg: pkg, tree: tree, headerRef: func(string) string { return "" }}
	if err := w.save(); err != nil {
		return nil, err
	}
	return pkg.Bytes()
}

type writer struct {
	d    *Document
	pkg  *opc.Package
	tree *ot.Tree
	// parts are the headers and footers by node id, those created while
	// writing included.
	parts map[string]string
	// headerRef is the relationship id of the part of a header or footer
	// node, from the main part.
	headerRef func(id string) string
	// rels are those of the part being written, drawings the last id of
	// its drawings.
	rels     *partrel.Writer
	drawings int
	// styles are the builtin styles paragraphs took, lists the builtin
	// lists they started by the numbers given them, in numbering.
	styles    []string
	lists     map[string]string
	numbering *xmldom.Document
	// commentID is the w:id of each comment node, anchored the anchors
	// written; mainRels are the relationships of the parts created that the
	// main part must have; anchorsGone tells that comments were deleted,
	// whose anchors the body may still have.
	commentID   map[string]string
	anchored    map[string]bool
	mainRels    []partrel.Rel
	anchorsGone bool
	// revs are the revisions wraps read stand for, made the elements of
	// revisions made for clients, whose ids the part gives.
	revs map[string][2]string
	made map[*xmldom.Element]bool
	// force rewrites every part, for tests.
	force bool
}

func (w *writer) save() error {
	if err := w.addPictures(); err != nil {
		return err
	}
	w.commentIDs()
	w.parts = map[string]string{}
	for id, p := range w.d.parts {
		w.parts[id] = p.name
	}
	var created []*ot.Node
	for _, n := range w.tree.Children("doc") {
		if n.Type != "hdr" && n.Type != "ftr" {
			continue
		}
		p := w.d.parts[n.ID]
		if p == nil {
			created = append(created, n)
			kind := map[string]string{"hdr": "header", "ftr": "footer"}[n.Type]
			w.parts[n.ID] = w.freeName("word/" + kind + "%d.xml")
			continue
		}
		if !w.same(n.ID) {
			if err := w.header(n, p.name, p.rels); err != nil {
				return err
			}
		}
	}
	for _, n := range created {
		if err := w.header(n, w.parts[n.ID], nil); err != nil {
			return err
		}
	}
	for kind, p := range w.d.notes {
		if err := w.notes(kind, p); err != nil {
			return err
		}
	}
	if err := w.comments(); err != nil {
		return err
	}
	if len(created) > 0 || len(w.mainRels) > 0 || w.anchorsGone || !w.same("body") || !w.sameAttrs("doc") {
		if err := w.document(); err != nil {
			return err
		}
	}
	if err := w.settings(); err != nil {
		return err
	}
	return w.definitions()
}

func (w *writer) freeName(pattern string) string {
	for n := 1; ; n++ {
		name := fmt.Sprintf(pattern, n)
		if !w.pkg.Has(name) && !slices.Contains(slices.Collect(maps.Values(w.parts)), name) {
			return name
		}
	}
}

func (w *writer) sameAttrs(id string) bool {
	a, b := w.d.loaded.Node(id), w.tree.Node(id)
	return !w.force && a != nil && b != nil && maps.EqualFunc(a.Attrs, b.Attrs, func(x, y json.RawMessage) bool { return bytes.Equal(x, y) })
}

// same tells whether the node and all under it are as they were read.
func (w *writer) same(id string) bool {
	if w.force {
		return false
	}
	a, b := w.d.loaded.Node(id), w.tree.Node(id)
	if a == nil || b == nil {
		return a == b
	}
	if a.Type != b.Type || a.Key != b.Key || !w.sameAttrs(id) {
		return false
	}
	if (a.Text == nil) != (b.Text == nil) || a.Text != nil && !reflect.DeepEqual(a.Text.Delta(), b.Text.Delta()) {
		return false
	}
	ka, kb := w.d.loaded.Children(id), w.tree.Children(id)
	if len(ka) != len(kb) {
		return false
	}
	for i := range ka {
		if ka[i].ID != kb[i].ID || !w.same(ka[i].ID) {
			return false
		}
	}
	return true
}

// open reads a part to rewrite, its relationships named.
func (w *writer) open(name string, rels *partrel.Rels) (*xmldom.Document, map[string]string, error) {
	data, err := w.pkg.Read(name)
	if err != nil {
		return nil, nil, err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil, nil, err
	}
	spaces := standardSpaces()
	maps.Copy(spaces, doc.Root.Spaces())
	w.d.names.NameAll(doc.Root, name, rels, spaces)
	return doc, spaces, nil
}

// put writes a part and its relationships, resolving the names its
// elements carry; read counts the ids of its tracked changes as read.
func (w *writer) put(doc *xmldom.Document, name, contentType string, rels *partrel.Rels, spaces map[string]string, rw *partrel.Writer, read map[string]int) error {
	if rw == nil {
		rw = partrel.NewWriter(name, rels, w.d.names.Lookup)
	}
	rw.Resolve(doc.Root, spaces)
	declare(doc.Root)
	uniqueRevisions(doc.Root, read, w.made)
	data := doc.Bytes()
	if w.pkg.Has(name) {
		if err := w.pkg.Set(name, data); err != nil {
			return err
		}
	} else if err := w.pkg.Add(name, contentType, data); err != nil {
		return err
	}
	if !rw.Changed && (rels != nil || len(rw.List) == 0) {
		return nil
	}
	return w.putRels(rw)
}

// putRels writes the relationships of a part.
func (w *writer) putRels(rw *partrel.Writer) error {
	name := opc.RelsName(rw.Source)
	out := opc.MarshalRelationships(rw.List)
	if w.pkg.Has(name) {
		return w.pkg.Set(name, out)
	}
	return w.pkg.Add(name, typeRels, out)
}

// revisions are the elements of tracked changes, whose ids Word wants
// unique.
var revisions = map[string]bool{
	"ins": true, "del": true, "moveFrom": true, "moveTo": true, "pPrChange": true, "rPrChange": true,
	"sectPrChange": true, "tblPrChange": true, "tblGridChange": true, "trPrChange": true, "tcPrChange": true,
	"cellIns": true, "cellDel": true, "cellMerge": true, "numberingChange": true,
}

// revisionIDs counts the ids of the tracked changes of a part.
func revisionIDs(root *xmldom.Element) map[string]int {
	out := map[string]int{}
	var walk func(e *xmldom.Element)
	walk = func(e *xmldom.Element) {
		if e.Space == NS && revisions[e.Local] {
			if id := attr(e, "id"); id != "" {
				out[id]++
			}
		}
		for _, c := range e.Elements() {
			walk(c)
		}
	}
	walk(root)
	return out
}

// uniqueRevisions gives new ids to the tracked changes that share one more
// often than the part as read did: a paragraph split in two copies those
// of its mark; and ids to those made.
func uniqueRevisions(root *xmldom.Element, read map[string]int, made map[*xmldom.Element]bool) {
	next := 0
	for id := range revisionIDs(root) {
		if n, err := strconv.Atoi(id); err == nil {
			next = max(next, n+1)
		}
	}
	seen := map[string]int{}
	var walk func(e *xmldom.Element)
	walk = func(e *xmldom.Element) {
		if made[e] {
			setAttr(e, "id", strconv.Itoa(next))
			next++
		} else if e.Space == NS && revisions[e.Local] {
			if id := attr(e, "id"); id != "" {
				seen[id]++
				if seen[id] > max(read[id], 1) {
					setAttr(e, "id", strconv.Itoa(next))
					next++
				}
			}
		}
		for _, c := range e.Elements() {
			walk(c)
		}
	}
	walk(root)
}

// declare gives the root the declarations of the standard prefixes its
// elements use and it lacks: XML carried by a node may come from a part
// that declared more.
func declare(root *xmldom.Element) {
	declared := root.Spaces()
	used := map[string]bool{}
	var walk func(e *xmldom.Element)
	walk = func(e *xmldom.Element) {
		if p, _, ok := strings.Cut(e.Name, ":"); ok {
			used[p] = true
		}
		for _, a := range e.Attrs {
			if p, _, ok := strings.Cut(a.Name, ":"); ok && p != "xmlns" && p != "xml" {
				used[p] = true
			}
		}
		for _, c := range e.Elements() {
			walk(c)
		}
	}
	walk(root)
	standard := standardSpaces()
	for _, p := range slices.Sorted(maps.Keys(used)) {
		if _, ok := declared[p]; !ok && standard[p] != "" {
			root.Set("xmlns:"+p, standard[p])
		}
	}
}

func (w *writer) header(n *ot.Node, name string, rels *partrel.Rels) error {
	var doc *xmldom.Document
	spaces := standardSpaces()
	contentType := typeHeader
	if n.Type == "ftr" {
		contentType = typeFooter
	}
	var read map[string]int
	if rels != nil {
		var err error
		if doc, spaces, err = w.open(name, rels); err != nil {
			return err
		}
		read = revisionIDs(doc.Root)
		w.drawings = lastDrawing(doc.Root)
	} else {
		root := xmldom.New(NS, "w:"+n.Type)
		root.Set("xmlns:w", NS)
		root.Set("xmlns:r", relNS)
		doc = &xmldom.Document{Prolog: []byte(xmlHeader), Root: root}
	}
	doc.Root.Content = nil
	w.rels = partrel.NewWriter(name, rels, w.d.names.Lookup)
	w.blocks(n.ID, doc.Root, w.marks(n.ID))
	if rels == nil && len(doc.Root.Content) == 0 {
		doc.Root.Append(newW(doc.Root, "p"))
	}
	return w.put(doc, name, contentType, rels, spaces, w.rels, read)
}

// notes rewrites the footnotes or endnotes whose nodes changed, in their
// part; the others and the separators stay as they are.
func (w *writer) notes(kind string, p *part) error {
	var changed []*ot.Node
	for _, n := range w.tree.Children("doc") {
		if n.Type == "note" && strings.HasPrefix(n.ID, kind[:1]+"n") && !w.same(n.ID) {
			changed = append(changed, n)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	doc, spaces, err := w.open(p.name, p.rels)
	if err != nil {
		return err
	}
	read := revisionIDs(doc.Root)
	w.drawings = lastDrawing(doc.Root)
	w.rels = partrel.NewWriter(p.name, p.rels, w.d.names.Lookup)
	for _, e := range doc.Root.Elements() {
		if e.Space != NS || e.Local != kind {
			continue
		}
		id := kind[:1] + "n" + attr(e, "id")
		if !slices.ContainsFunc(changed, func(n *ot.Node) bool { return n.ID == id }) {
			continue
		}
		e.Content = nil
		w.blocks(id, e, w.marks(id))
		if len(e.Content) == 0 {
			e.Append(newW(e, "p"))
		}
	}
	return w.put(doc, p.name, "", p.rels, spaces, w.rels, read)
}

// document rewrites the body of the main part and its last section.
func (w *writer) document() error {
	doc, spaces, err := w.open(w.d.main, w.d.mainRels)
	if err != nil {
		return err
	}
	read := revisionIDs(doc.Root)
	w.drawings = lastDrawing(doc.Root)
	body := child(doc.Root, "body")
	rw := partrel.NewWriter(w.d.main, w.d.mainRels, w.d.names.Lookup)
	w.headerRef = func(id string) string {
		name := w.parts[id]
		if name == "" {
			return ""
		}
		typ := relHeader
		if n := w.tree.Node(id); n != nil && n.Type == "ftr" {
			typ = relFooter
		}
		return rw.Ensure(partrel.Rel{Type: typ, Target: name})
	}
	for _, r := range w.mainRels {
		rw.Ensure(r)
	}
	sect := child(body, "sectPr")
	body.Content = nil
	w.rels = rw
	w.blocks("body", body, w.marks("body"))
	root := w.tree.Node("doc")
	if s := w.section(str(root.Attrs, "sx"), root.Attrs["sect"], body); s != nil {
		body.Append(s)
	} else if sect != nil {
		body.Append(sect)
	}
	return w.put(doc, w.d.main, "", w.d.mainRels, spaces, rw, read)
}

// str is a string attribute, "" if absent.
func str(attrs ot.Values, key string) string {
	var s string
	if v, ok := attrs[key]; ok {
		_ = json.Unmarshal(v, &s)
	}
	return s
}

// section is the sectPr of raw with sect written over it, nil when there
// is neither.
func (w *writer) section(raw string, sect json.RawMessage, in *xmldom.Element) *xmldom.Element {
	s := w.d.fragment(raw)
	if s != nil && (s.Space != NS || s.Local != "sectPr") {
		s = nil
	}
	var sec Section
	if len(sect) == 0 || json.Unmarshal(sect, &sec) != nil {
		return s
	}
	if s == nil {
		s = newW(in, "sectPr")
	}
	old := readSection(s, w.nodesOf(s))
	setSection(s, old, sec, w.headerRef)
	return s
}

// nodesOf names the header and footer nodes a sectPr refers to, by the name
// of their relationship.
func (w *writer) nodesOf(s *xmldom.Element) map[string]string {
	out := map[string]string{}
	for _, ref := range s.Elements() {
		name := ref.Get("r:id")
		r, ok := w.d.names.Lookup(name)
		if !ok {
			continue
		}
		for id, part := range w.parts {
			if part == r.Target {
				out[name] = id
			}
		}
	}
	return out
}

// marks counts the paragraph elements as read under a node: a paragraph
// split in two gives its id to one half only.
type marks map[string]int

func (w *writer) marks(id string) marks {
	out := marks{}
	var walk func(id string)
	walk = func(id string) {
		for _, n := range w.d.loaded.Children(id) {
			if n.Text != nil {
				for _, p := range paragraphs(n.Text.Delta()) {
					out[p.mark["pa"]]++
				}
			}
			walk(n.ID)
		}
	}
	walk(id)
	return out
}

// blocks writes the blocks under a node into an element.
func (w *writer) blocks(parent string, into *xmldom.Element, ids marks) {
	for _, n := range w.tree.Children(parent) {
		switch n.Type {
		case "text":
			for _, p := range paragraphs(n.Text.Delta()) {
				into.Append(w.paragraph(p, into, ids))
			}
		case "tbl":
			into.Append(w.table(n, into, ids))
		case "sdt":
			sdt := w.d.fragment(str(n.Attrs, "xml"))
			if sdt == nil || sdt.Space != NS || sdt.Local != "sdt" {
				sdt = newW(into, "sdt")
			}
			remove(sdt, "sdtContent")
			content := newW(sdt, "sdtContent")
			sdt.Append(content)
			w.blocks(n.ID, content, ids)
			into.Append(sdt)
		case "other":
			if e := w.d.fragment(str(n.Attrs, "xml")); e != nil {
				into.Append(e)
			}
		}
	}
}

// paragraph is the text of a paragraph, its mark last.
type paragraph struct {
	ops  []ot.Op
	mark ot.Attrs
}

// paragraphs cuts a flow at its marks.
func paragraphs(flow ot.Delta) []paragraph {
	var out []paragraph
	var cur []ot.Op
	for _, o := range flow {
		s := o.Insert
		for s != "" {
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				cur = append(cur, ot.Op{Insert: s, Attrs: o.Attrs})
				break
			}
			if i > 0 {
				cur = append(cur, ot.Op{Insert: s[:i], Attrs: o.Attrs})
			}
			out = append(out, paragraph{ops: cur, mark: o.Attrs})
			cur = nil
			s = s[i+1:]
		}
	}
	return out
}

func (w *writer) paragraph(para paragraph, in *xmldom.Element, ids marks) *xmldom.Element {
	pa := para.mark["pa"]
	p := w.d.fragment(pa)
	if p == nil || p.Space != NS || p.Local != "p" {
		p = newW(in, "p")
	}
	p.Content = nil
	if ids[pa] > 0 {
		ids[pa]--
	} else {
		p.Unset("w14:paraId")
	}
	if pPr := w.pPr(para.mark, p); pPr != nil {
		p.Append(pPr)
	}
	var items []item
	for _, o := range para.ops {
		items = append(items, item{text: o.Insert, attrs: o.Attrs, wrap: w.revised(wrapsOf(o.Attrs), o.Attrs)})
	}
	w.inline(p, items, 0, false)
	return p
}

var runKeys = func() map[string]bool {
	out := map[string]bool{}
	for _, p := range runProps {
		out[p.key] = true
	}
	return out
}()

// pPr is the pPr of a mark: the one it was read from, its keys written
// over it, and its section.
func (w *writer) pPr(mark ot.Attrs, in *xmldom.Element) *xmldom.Element {
	pPr := w.d.fragment(mark["p"])
	var old, oldRun Props
	read := pPr != nil && pPr.Space == NS && pPr.Local == "pPr"
	if read {
		old, oldRun = ParaProps(pPr), RunProps(child(pPr, "rPr"))
	} else {
		pPr = newW(in, "pPr")
	}
	hadRun := child(pPr, "rPr") != nil
	remove(pPr, "sectPr")
	para, run := Props{}, Props{}
	for k, v := range mark {
		switch {
		case paraKeys[k]:
			para[k] = v
		case runKeys[k]:
			run[k] = v
		}
	}
	// a builtin style goes into the file when a client applies it, not for
	// a paragraph that named a style the file lacks
	if id := para["pstyle"]; id != "" && id != old["pstyle"] && !w.d.fileStyles[id] {
		w.useStyle(id)
	}
	if n := para["num"]; builtinLists[n] != "" {
		para["num"] = w.list(n)
	}
	writeProps(pPr, paraProps, old, para)
	rPr := child(pPr, "rPr")
	if rPr == nil && len(run) > 0 {
		rPr = newW(pPr, "rPr")
		insert(pPr, rPr, pPrOrder)
	}
	if rPr != nil {
		writeProps(rPr, runProps, oldRun, run)
		if !hadRun && len(rPr.Content) == 0 {
			pPr.Remove(rPr)
		}
	}
	w.markRevisions(pPr, mark)
	if s := w.section(mark["sx"], json.RawMessage(mark["sect"]), pPr); s != nil {
		insert(pPr, s, pPrOrder)
	}
	if !read && len(pPr.Content) == 0 {
		return nil
	}
	return pPr
}

// item is text of a paragraph with its attributes, and the elements around
// it.
type item struct {
	text  string
	attrs ot.Attrs
	wrap  []string
}

// wrapsOf are the elements around an item: those it was read in, or those
// a client asks for with "field" and "link" and the writer makes, named
// after a NUL.
func wrapsOf(a ot.Attrs) []string {
	var out []string
	if s := a["wrap"]; s != "" {
		_ = json.Unmarshal([]byte(s), &out)
		return out
	}
	if f := fieldInstr(a["field"]); f != "" {
		return []string{"\x00field" + f}
	}
	if l := a["link"]; partrel.LinkTarget(l) != "" || anchorOf(l) != "" {
		return []string{"\x00link" + l}
	}
	return nil
}

// fields are the fields a client may put in a document.
var fields = map[string]bool{"PAGE": true, "NUMPAGES": true, "SECTIONPAGES": true, "DATE": true, "TIME": true, "FILENAME": true, "AUTHOR": true, "TITLE": true}

// fieldInstr are the instructions of a field a client asks for, "" unless
// they are one of fields with switches only.
func fieldInstr(s string) string {
	s = strings.TrimSpace(s)
	name, _, _ := strings.Cut(s, " ")
	if !fields[strings.ToUpper(name)] || len(s) > 200 || strings.ContainsAny(s, "<>&\"\x00\n") {
		return ""
	}
	return " " + s + " "
}

// anchorOf is the bookmark a link inside the document goes to.
func anchorOf(s string) string {
	if a, ok := strings.CutPrefix(s, "#"); ok && name(a) && len(a) <= 40 {
		return a
	}
	return ""
}

// shell is the element a wrap stands for, without content.
func (w *writer) shell(wrap string, in *xmldom.Element) *xmldom.Element {
	switch {
	case strings.HasPrefix(wrap, "\x00field"):
		return newW(in, "fldSimple", "instr", strings.TrimPrefix(wrap, "\x00field"))
	case strings.HasPrefix(wrap, "\x00link"):
		target := strings.TrimPrefix(wrap, "\x00link")
		h := newW(in, "hyperlink")
		if a := anchorOf(target); a != "" {
			setAttr(h, "anchor", a)
		} else if w.rels != nil {
			h.Set("r:id", w.rels.Ensure(partrel.Rel{Type: partrel.Hyperlink, Target: partrel.LinkTarget(target), External: true}))
		}
		return h
	}
	if e := w.madeRevision(wrap, in); e != nil {
		return e
	}
	return w.d.fragment(wrap)
}

// described are the keys that describe an Object or its wrappers, which
// do not tell runs apart.
var described = map[string]bool{
	"o": true, "po": true, "wrap": true, "link": true, "ins": true, "del": true, "insd": true, "deld": true, "field": true,
	"img": true, "fld": true, "instr": true, "br": true, "sym": true, "note": true, "comment": true,
	"bm": true, "math": true, "cs": true, "ce": true,
}

// runKey tells runs apart: two items with the same key belong to the same
// run.
func runKey(a ot.Attrs) string {
	var b strings.Builder
	for _, k := range slices.Sorted(maps.Keys(a)) {
		if described[k] {
			continue
		}
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(a[k])
		b.WriteByte(0)
	}
	return b.String()
}

// inline writes items into a paragraph or the element around them; depth
// is the number of elements around them already written, deleted whether
// one of them is a deletion.
func (w *writer) inline(parent *xmldom.Element, items []item, depth int, deleted bool) {
	for i := 0; i < len(items); {
		it := items[i]
		if len(it.wrap) > depth {
			j := i + 1
			for j < len(items) && len(items[j].wrap) > depth && items[j].wrap[depth] == it.wrap[depth] {
				j++
			}
			shell := w.shell(it.wrap[depth], parent)
			if shell == nil {
				w.inline(parent, items[i:j], depth+1, deleted)
			} else {
				target := shell
				if shell.Space == NS && shell.Local == "sdt" {
					target = newW(shell, "sdtContent")
					shell.Append(target)
				}
				del := deleted || shell.Space == NS && (shell.Local == "del" || shell.Local == "moveFrom")
				w.inline(target, items[i:j], depth+1, del)
				parent.Append(shell)
			}
			i = j
			continue
		}
		if paragraphLevel(it.attrs) {
			for range utf8.RuneCountInString(it.text) {
				if e := w.paragraphObject(it.attrs, parent); e != nil {
					parent.Append(e)
				}
			}
			i++
			continue
		}
		key := runKey(it.attrs)
		j := i + 1
		for j < len(items) && len(items[j].wrap) <= depth && !paragraphLevel(items[j].attrs) && runKey(items[j].attrs) == key {
			j++
		}
		if r := w.run(items[i:j], parent, deleted); r != nil {
			parent.Append(r)
		}
		i = j
	}
}

// paragraphLevel tells the items that are elements of the paragraph
// rather than of a run.
func paragraphLevel(a ot.Attrs) bool {
	return a["po"] != "" || a["cs"] != "" || a["ce"] != ""
}

func (w *writer) paragraphObject(a ot.Attrs, in *xmldom.Element) *xmldom.Element {
	switch {
	case a["cs"] != "":
		return w.commentAnchor(a["cs"], "commentRangeStart", a["po"], in)
	case a["ce"] != "":
		return w.commentAnchor(a["ce"], "commentRangeEnd", a["po"], in)
	}
	return w.d.fragment(a["po"])
}

// commentAnchor is the element of a comment's range or reference: the one
// read, or one made for a comment added; none for a comment deleted, or
// whose anchor of that kind was written already.
func (w *writer) commentAnchor(comment, local, read string, in *xmldom.Element) *xmldom.Element {
	id, ok := w.commentID[comment]
	if !ok || w.anchored[local+id] {
		return nil
	}
	w.anchored[local+id] = true
	if e := w.d.fragment(read); e != nil {
		return e
	}
	return newW(in, local, "id", id)
}

// run writes items of the same formatting as a w:r.
func (w *writer) run(items []item, in *xmldom.Element, deleted bool) *xmldom.Element {
	r := newW(in, "r")
	attrs := items[0].attrs
	rPr := w.d.fragment(attrs["r"])
	var old Props
	read := rPr != nil && rPr.Space == NS && rPr.Local == "rPr"
	if read {
		old = RunProps(rPr)
	} else {
		rPr = newW(r, "rPr")
	}
	props := Props{}
	for k, v := range attrs {
		if runKeys[k] {
			props[k] = v
		}
	}
	writeProps(rPr, runProps, old, props)
	if read || len(rPr.Content) > 0 {
		r.Append(rPr)
	}
	var text strings.Builder
	flush := func() {
		if text.Len() == 0 {
			return
		}
		local := "t"
		if deleted {
			local = "delText"
		}
		t := newW(r, local)
		s := text.String()
		if strings.TrimSpace(s) != s || strings.Contains(s, "  ") {
			t.Set("xml:space", "preserve")
		}
		t.Append(xmldom.EscapeText(s))
		r.Append(t)
		text.Reset()
	}
	for _, it := range items {
		if c := it.attrs["comment"]; c != "" {
			flush()
			for range utf8.RuneCountInString(it.text) {
				if e := w.commentAnchor(c, "commentReference", it.attrs["o"], r); e != nil {
					r.Append(e)
				}
			}
			continue
		}
		if o := it.attrs["o"]; o != "" {
			flush()
			for range utf8.RuneCountInString(it.text) {
				if e := w.d.fragment(o); e != nil {
					r.Append(e)
				}
			}
			continue
		}
		if img := it.attrs["img"]; img != "" {
			flush()
			for range utf8.RuneCountInString(it.text) {
				if e := w.drawing(img); e != nil {
					r.Append(e)
				}
			}
			continue
		}
		if br := it.attrs["br"]; br == "page" || br == "column" || br == "textWrapping" {
			flush()
			for range utf8.RuneCountInString(it.text) {
				e := newW(r, "br")
				if br != "textWrapping" {
					setAttr(e, "type", br)
				}
				r.Append(e)
			}
			continue
		}
		for _, c := range it.text {
			var local string
			switch c {
			case '\t':
				local = "tab"
			case '\v':
				local = "br"
			case '‑':
				local = "noBreakHyphen"
			case '­':
				local = "softHyphen"
			case 0xFFFC:
				continue
			default:
				text.WriteRune(c)
				continue
			}
			flush()
			r.Append(newW(r, local))
		}
	}
	flush()
	if len(r.Content) == 0 || len(r.Content) == 1 && r.Content[0] == xmldom.Node(rPr) {
		return nil
	}
	return r
}

func (w *writer) table(n *ot.Node, in *xmldom.Element, ids marks) *xmldom.Element {
	tbl := w.d.fragment(str(n.Attrs, "xml"))
	if tbl == nil || tbl.Space != NS || tbl.Local != "tbl" {
		tbl = newW(in, "tbl")
	}
	tblPr := child(tbl, "tblPr")
	if tblPr == nil {
		tblPr = newW(tbl, "tblPr")
		writeTprops(tblPr, tblProps, ot.Values{}, n.Attrs)
		if len(tblPr.Content) > 0 {
			tbl.Content = append([]xmldom.Node{tblPr}, tbl.Content...)
			if child(tbl, "tblGrid") == nil {
				insert(tbl, newW(tbl, "tblGrid"), []string{"tblPr", "tblGrid"})
			}
		}
	} else {
		old := ot.Values{}
		readTprops(tblPr, tblProps, old)
		writeTprops(tblPr, tblProps, old, n.Attrs)
	}
	var cols []int
	if g := n.Attrs["grid"]; g != nil && json.Unmarshal(g, &cols) == nil && !slices.Equal(cols, grid(child(tbl, "tblGrid"))) {
		g := child(tbl, "tblGrid")
		if g == nil {
			g = newW(tbl, "tblGrid")
			insert(tbl, g, []string{"tblPr", "tblGrid"})
		}
		g.Content = nil
		for _, c := range cols {
			g.Append(newW(g, "gridCol", "w", fmt.Sprint(max(c, 0))))
		}
	}
	for _, c := range w.tree.Children(n.ID) {
		switch c.Type {
		case "tr":
			tbl.Append(w.row(c, tbl, ids))
		case "other":
			if e := w.d.fragment(str(c.Attrs, "xml")); e != nil {
				tbl.Append(e)
			}
		}
	}
	return tbl
}

func (w *writer) row(n *ot.Node, in *xmldom.Element, ids marks) *xmldom.Element {
	tr := w.d.fragment(str(n.Attrs, "xml"))
	if tr == nil || tr.Space != NS || tr.Local != "tr" {
		tr = newW(in, "tr")
	}
	trPr := child(tr, "trPr")
	if trPr == nil {
		trPr = newW(tr, "trPr")
		writeTprops(trPr, trProps, ot.Values{}, n.Attrs)
		if len(trPr.Content) > 0 {
			insert(tr, trPr, []string{"tblPrEx", "trPr"})
		}
	} else {
		old := ot.Values{}
		readTprops(trPr, trProps, old)
		writeTprops(trPr, trProps, old, n.Attrs)
	}
	for _, c := range w.tree.Children(n.ID) {
		switch c.Type {
		case "tc":
			tr.Append(w.cell(c, tr, ids))
		case "other":
			if e := w.d.fragment(str(c.Attrs, "xml")); e != nil {
				tr.Append(e)
			}
		}
	}
	return tr
}

func (w *writer) cell(n *ot.Node, in *xmldom.Element, ids marks) *xmldom.Element {
	tc := w.d.fragment(str(n.Attrs, "xml"))
	if tc == nil || tc.Space != NS || tc.Local != "tc" {
		tc = newW(in, "tc")
	}
	tcPr := child(tc, "tcPr")
	if tcPr == nil {
		tcPr = newW(tc, "tcPr")
		writeTprops(tcPr, tcProps, ot.Values{}, n.Attrs)
		if len(tcPr.Content) > 0 {
			tc.Content = append([]xmldom.Node{tcPr}, tc.Content...)
		}
	} else {
		old := ot.Values{}
		readTprops(tcPr, tcProps, old)
		writeTprops(tcPr, tcProps, old, n.Attrs)
	}
	before := len(tc.Elements())
	w.blocks(n.ID, tc, ids)
	if len(tc.Elements()) == before {
		tc.Append(newW(tc, "p"))
	}
	return tc
}
