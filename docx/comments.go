package docx

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

const (
	relComments         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	relCommentsExtended = "http://schemas.microsoft.com/office/2011/relationships/commentsExtended"
	relCommentsIds      = "http://schemas.microsoft.com/office/2016/09/relationships/commentsIds"
	relCommentsCex      = "http://schemas.microsoft.com/office/2018/08/relationships/commentsExtensible"
	typeComments        = "application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"
	typeCommentsEx      = "application/vnd.openxmlformats-officedocument.wordprocessingml.commentsExtended+xml"
	w14NS               = "http://schemas.microsoft.com/office/word/2010/wordml"
	w15NS               = "http://schemas.microsoft.com/office/word/2012/wordml"
	cidNS               = "http://schemas.microsoft.com/office/word/2016/wordml/cid"
	cexNS               = "http://schemas.microsoft.com/office/word/2018/wordml/cex"
)

// A comment is a "comment" node under "doc", "cm" and its w:id for those
// read, with its blocks: "author", "initials" and "date" as Word wrote
// them, "done" when it was resolved and "parent" the comment it answers.
// The flows mark what it is about with Objects "cs" and "ce", the start
// and end of its range, and "comment" its reference.

// readComments reads the comments part before the body, so that the
// anchors know the comments they belong to.
func (r *reader) readComments(rels *partrel.Rels) error {
	partName := rels.OfType(r.d.main, relComments)
	if partName == "" || !r.d.pkg.Has(partName) {
		return nil
	}
	root, crels, _, err := r.part(partName)
	if err != nil {
		return err
	}
	r.d.comments = &part{name: partName, rels: crels}
	r.comments = map[string]*xmldom.Element{}
	for _, c := range root.Elements() {
		if c.Space != NS || c.Local != "comment" {
			continue
		}
		id := commentNode(c)
		if id == "" {
			continue
		}
		n, _ := strconv.Atoi(id[2:])
		r.d.commentMax = max(r.d.commentMax, n)
		if r.comments[id] != nil {
			continue
		}
		r.comments[id] = c
		r.commentOrder = append(r.commentOrder, id)
	}
	return nil
}

// commentNodes adds the comments read, keyed after last.
func (r *reader) commentNodes(rels *partrel.Rels, last string) error {
	byPara := map[string]string{}
	for _, id := range r.commentOrder {
		if p := paraID(lastParagraph(r.comments[id])); p != "" {
			byPara[p] = id
		}
	}
	done, parents := map[string]bool{}, map[string]string{}
	if name := rels.OfType(r.d.main, relCommentsExtended); name != "" && r.d.pkg.Has(name) {
		root, _, _, err := r.part(name)
		if err != nil {
			return err
		}
		for _, e := range root.Elements() {
			if e.Space != w15NS || e.Local != "commentEx" {
				continue
			}
			id := byPara[nsAttr(e, "paraId")]
			if id == "" {
				continue
			}
			done[id] = truthy(nsAttr(e, "done"))
			if p := byPara[nsAttr(e, "paraIdParent")]; p != "" && p != id {
				parents[id] = p
			}
		}
	}
	for _, id := range r.commentOrder {
		c := r.comments[id]
		attrs := ot.Values{}
		for _, k := range []string{"author", "initials", "date"} {
			if v := attr(c, k); v != "" {
				attrs[k], _ = json.Marshal(v)
			}
		}
		if done[id] {
			attrs["done"] = json.RawMessage("true")
		}
		if p := parents[id]; p != "" {
			attrs["parent"], _ = json.Marshal(p)
		}
		last = ot.KeyBetween(last, "")
		r.addID(id, "comment", "doc", last, attrs, nil)
		r.blocks(c, id)
	}
	return nil
}

// commentAnchor is the node id of the comment an anchor belongs to, ""
// when the document has no such comment.
func (r *reader) commentAnchor(e *xmldom.Element) string {
	id := commentNode(e)
	if r.comments[id] == nil {
		return ""
	}
	return id
}

// commentNode is the node id of the comment whose w:id e carries.
func commentNode(e *xmldom.Element) string {
	n, err := strconv.Atoi(attr(e, "id"))
	if err != nil || n < 0 {
		return ""
	}
	return "cm" + strconv.Itoa(n)
}

func lastParagraph(e *xmldom.Element) *xmldom.Element {
	var last *xmldom.Element
	for _, c := range e.Elements() {
		if c.Space == NS && c.Local == "p" {
			last = c
		}
	}
	return last
}

// nsAttr is an attribute of any prefix: the parts of comments use each a
// namespace of their own, under the prefix Word gives it.
func nsAttr(e *xmldom.Element, local string) string {
	if e == nil {
		return ""
	}
	for _, a := range e.Attrs {
		if _, l, ok := strings.Cut(a.Name, ":"); ok && l == local && !strings.HasPrefix(a.Name, "xmlns:") {
			return a.Value
		}
	}
	return ""
}

func paraID(p *xmldom.Element) string {
	if p == nil {
		return ""
	}
	return p.Get(prefixOf(p, w14NS, "w14") + ":paraId")
}

// prefixOf is the prefix e or the part declares for space, def when none
// does.
func prefixOf(e *xmldom.Element, space, def string) string {
	for p, s := range e.Spaces() {
		if s == space && p != "" {
			return p
		}
	}
	return def
}

// commentIDs gives each comment of the tree its w:id: its own for those
// read, the next free ones for the others.
func (w *writer) commentIDs() {
	w.commentID, w.anchored = map[string]string{}, map[string]bool{}
	next := w.d.commentMax + 1
	for _, n := range w.tree.Children("doc") {
		if n.Type != "comment" {
			continue
		}
		if old := w.d.loaded.Node(n.ID); old != nil && old.Type == "comment" {
			w.commentID[n.ID] = strings.TrimPrefix(n.ID, "cm")
			continue
		}
		w.commentID[n.ID] = strconv.Itoa(next)
		next++
	}
}

// comments writes the comments part when a comment changed, was added or
// deleted, and what Word keeps of them beside.
func (w *writer) comments() error {
	var nodes []*ot.Node
	changed := false
	for _, n := range w.tree.Children("doc") {
		if n.Type == "comment" {
			nodes = append(nodes, n)
			changed = changed || !w.same(n.ID)
		}
	}
	for _, n := range w.d.loaded.Children("doc") {
		if n.Type == "comment" && w.tree.Node(n.ID) == nil {
			changed, w.anchorsGone = true, true
		}
	}
	if !changed {
		return nil
	}
	var doc *xmldom.Document
	var rels *partrel.Rels
	spaces := standardSpaces()
	partName := ""
	var read map[string]int
	if p := w.d.comments; p != nil {
		var err error
		if doc, spaces, err = w.open(p.name, p.rels); err != nil {
			return err
		}
		partName, rels = p.name, p.rels
		read = revisionIDs(doc.Root)
		w.drawings = lastDrawing(doc.Root)
	} else {
		partName = "word/comments.xml"
		if w.pkg.Has(partName) {
			partName = w.freeName("word/comments%d.xml")
		}
		root := xmldom.New(NS, "w:comments", "xmlns:w", NS, "xmlns:r", relNS, "xmlns:mc", mcNS)
		doc = &xmldom.Document{Prolog: []byte(xmlHeader), Root: root}
		w.mainRels = append(w.mainRels, partrel.Rel{Type: relComments, Target: partName})
	}
	w.rels = partrel.NewWriter(partName, rels, w.d.names.Lookup)
	written := map[string]*xmldom.Element{}
	var kept []xmldom.Node
	for _, c := range doc.Root.Content {
		e, ok := c.(*xmldom.Element)
		if !ok || e.Space != NS || e.Local != "comment" {
			kept = append(kept, c)
			continue
		}
		id := commentNode(e)
		if old := w.d.loaded.Node(id); old != nil && old.Type == "comment" && written[id] == nil {
			if w.tree.Node(id) == nil {
				continue
			}
			written[id] = e
			if !w.same(id) {
				w.comment(id, e)
			}
		}
		kept = append(kept, e)
	}
	doc.Root.Content = kept
	for _, n := range nodes {
		if written[n.ID] != nil {
			continue
		}
		e := newW(doc.Root, "comment", "id", w.commentID[n.ID], "author", str(n.Attrs, "author"))
		for _, k := range []string{"date", "initials"} {
			if s := str(n.Attrs, k); s != "" {
				setAttr(e, k, s)
			}
		}
		w.comment(n.ID, e)
		doc.Root.Append(e)
		written[n.ID] = e
	}
	if err := w.commentsExtended(nodes, written, doc.Root); err != nil {
		return err
	}
	if err := w.put(doc, partName, typeComments, rels, spaces, w.rels, read); err != nil {
		return err
	}
	return w.pruneCommentIds(doc.Root)
}

// comment writes the blocks of a comment into its element.
func (w *writer) comment(id string, e *xmldom.Element) {
	e.Content = nil
	w.blocks(id, e, w.marks(id))
	if len(e.Content) == 0 {
		e.Append(newW(e, "p"))
	}
}

// commentsExtended writes whether each comment is done and which it
// answers, keyed by the id of its last paragraph, given one when it needs
// it; the entries of comments deleted go.
func (w *writer) commentsExtended(nodes []*ot.Node, written map[string]*xmldom.Element, root *xmldom.Element) error {
	parents := map[string]bool{}
	for _, n := range nodes {
		if p := str(n.Attrs, "parent"); p != "" && written[p] != nil {
			parents[p] = true
		}
	}
	used := map[string]bool{}
	for _, e := range root.Elements() {
		for _, p := range e.Elements() {
			if id := paraID(p); id != "" {
				used[id] = true
			}
		}
	}
	para := map[string]string{}
	needed := false
	for _, n := range nodes {
		p := lastParagraph(written[n.ID])
		id := paraID(p)
		if id == "" && p != nil && (done(n) || str(n.Attrs, "parent") != "" || parents[n.ID]) {
			id = freeParaID(used)
			p.Set(prefixOf(root, w14NS, "w14")+":paraId", id)
			ignorable(root, prefixOf(root, w14NS, "w14"))
		}
		para[n.ID] = id
		needed = needed || done(n) || str(n.Attrs, "parent") != ""
	}

	partName := w.d.mainRelsOf(relCommentsExtended)
	var doc *xmldom.Document
	var spaces map[string]string
	var rels *partrel.Rels
	if partName != "" {
		var err error
		rels, err = partrel.Read(w.pkg, partName)
		if err != nil {
			return err
		}
		if doc, spaces, err = w.open(partName, rels); err != nil {
			return err
		}
	} else {
		if !needed {
			return nil
		}
		partName = "word/commentsExtended.xml"
		spaces = standardSpaces()
		doc = &xmldom.Document{Prolog: []byte(xmlHeader), Root: xmldom.New(w15NS, "w15:commentsEx",
			"xmlns:mc", mcNS, "xmlns:w15", w15NS, "mc:Ignorable", "w15")}
		w.mainRels = append(w.mainRels, partrel.Rel{Type: relCommentsExtended, Target: partName})
	}
	prefix := prefixOf(doc.Root, w15NS, "w15")
	entries := map[string]*xmldom.Element{}
	var kept []xmldom.Node
	for _, c := range doc.Root.Content {
		if e, ok := c.(*xmldom.Element); ok && e.Space == w15NS && e.Local == "commentEx" {
			id := nsAttr(e, "paraId")
			if !used[id] {
				continue
			}
			entries[id] = e
		}
		kept = append(kept, c)
	}
	doc.Root.Content = kept
	for _, n := range nodes {
		id := para[n.ID]
		if id == "" {
			continue
		}
		e := entries[id]
		if e == nil {
			if !done(n) && str(n.Attrs, "parent") == "" && !parents[n.ID] {
				continue
			}
			e = xmldom.New(w15NS, prefix+":commentEx", prefix+":paraId", id)
			doc.Root.Append(e)
		}
		if p := para[str(n.Attrs, "parent")]; p != "" {
			e.Set(prefix+":paraIdParent", p)
		} else {
			e.Unset(prefix + ":paraIdParent")
		}
		if done(n) {
			e.Set(prefix+":done", "1")
		} else if nsAttr(e, "done") != "" {
			e.Set(prefix+":done", "0")
		}
	}
	return w.put(doc, partName, typeCommentsEx, rels, spaces, nil, nil)
}

// pruneCommentIds drops from the parts of durable ids and dates those of
// the comments deleted: those whose paragraph the comments part lacks.
func (w *writer) pruneCommentIds(comments *xmldom.Element) error {
	used := map[string]bool{}
	for _, e := range comments.Elements() {
		for _, p := range e.Elements() {
			if id := paraID(p); id != "" {
				used[id] = true
			}
		}
	}
	dropped := map[string]bool{}
	if err := w.prune(relCommentsIds, cidNS, "commentId", func(e *xmldom.Element) bool {
		if used[nsAttr(e, "paraId")] {
			return true
		}
		dropped[nsAttr(e, "durableId")] = true
		return false
	}); err != nil {
		return err
	}
	if len(dropped) == 0 {
		return nil
	}
	return w.prune(relCommentsCex, cexNS, "commentExtensible", func(e *xmldom.Element) bool {
		return !dropped[nsAttr(e, "durableId")]
	})
}

// prune rewrites the part the main part relates to by typ without the
// entries keep refuses.
func (w *writer) prune(typ, space, local string, keep func(e *xmldom.Element) bool) error {
	partName := w.d.mainRelsOf(typ)
	if partName == "" {
		return nil
	}
	rels, err := partrel.Read(w.pkg, partName)
	if err != nil {
		return err
	}
	doc, spaces, err := w.open(partName, rels)
	if err != nil {
		return err
	}
	var kept []xmldom.Node
	for _, c := range doc.Root.Content {
		if e, ok := c.(*xmldom.Element); ok && e.Space == space && e.Local == local && !keep(e) {
			continue
		}
		kept = append(kept, c)
	}
	if len(kept) == len(doc.Root.Content) {
		return nil
	}
	doc.Root.Content = kept
	return w.put(doc, partName, "", rels, spaces, nil, nil)
}

// mainRelsOf is the part the main part relates to by typ, "" if none.
func (d *Document) mainRelsOf(typ string) string {
	name := d.mainRels.OfType(d.main, typ)
	if name == "" || !d.pkg.Has(name) {
		return ""
	}
	return name
}

func done(n *ot.Node) bool {
	return string(n.Attrs["done"]) == "true"
}

// freeParaID is a paragraph id Word takes, below 0x80000000, not in used.
func freeParaID(used map[string]bool) string {
	for {
		id := fmt.Sprintf("%08X", rand.Uint32N(0x7FFFFFFE)+1)
		if !used[id] {
			used[id] = true
			return id
		}
	}
}

// ignorable adds prefix to the prefixes the root tells older readers to
// ignore.
func ignorable(root *xmldom.Element, prefix string) {
	mc := prefixOf(root, mcNS, "")
	if mc == "" {
		return
	}
	list := root.Get(mc + ":Ignorable")
	for _, p := range strings.Fields(list) {
		if p == prefix {
			return
		}
	}
	root.Set(mc+":Ignorable", strings.TrimSpace(list+" "+prefix))
}
