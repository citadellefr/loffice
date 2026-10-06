package pptx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

const (
	relComments       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments"
	relCommentAuthors = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/commentAuthors"
	typeComments      = "application/vnd.openxmlformats-officedocument.presentationml.comments+xml"
	typeAuthors       = "application/vnd.openxmlformats-officedocument.presentationml.commentAuthors+xml"
	p15NS             = "http://schemas.microsoft.com/office/powerpoint/2012/main"
	threadingURI      = "{C676402C-5697-4E1C-873F-D02D1690AC5C}"
	commentTime       = "2006-01-02T15:04:05.000"
)

// A comment is a "comment" node under its slide: "author", "initials",
// "date" (RFC 3339), "x" and "y" where it sits on the slide in the units
// of PowerPoint's comments (1/576 inch), "parent" the comment it answers.
// Its text is the node's, without formatting. The modern comments of
// Microsoft 365 are not read: their parts are left as they are.

// cmAuthor is an entry of the authors of the comments.
type cmAuthor struct {
	id, lastIdx, clrIdx int
	name, initials      string
}

// cmRef is the author and the number PowerPoint gives a comment.
type cmRef struct{ author, idx int }

func (r *reader) commentAuthors(rels *partrel.Rels) {
	name := rels.OfType(r.d.presName, relCommentAuthors)
	if name == "" || !r.d.pkg.Has(name) {
		return
	}
	root, _, _, err := r.part(name)
	if err != nil {
		return
	}
	r.d.authorsPart = name
	for _, a := range elements(root, pNS, "cmAuthor") {
		id, err := strconv.Atoi(a.Get("id"))
		if err != nil {
			continue
		}
		last, _ := strconv.Atoi(a.Get("lastIdx"))
		clr, _ := strconv.Atoi(a.Get("clrIdx"))
		r.d.authors = append(r.d.authors, cmAuthor{id: id, lastIdx: last, clrIdx: clr, name: a.Get("name"), initials: a.Get("initials")})
	}
}

func (d *Document) authorByID(id int) *cmAuthor {
	for i := range d.authors {
		if d.authors[i].id == id {
			return &d.authors[i]
		}
	}
	return nil
}

func commentID(slide string, ref cmRef) string {
	return slide + "-c" + strconv.Itoa(ref.author) + "." + strconv.Itoa(ref.idx)
}

// readComments adds the comments of a slide, parents first.
func (r *reader) readComments(slide string, part *slidePart, rels *partrel.Rels) {
	part.comments = rels.OfType(part.name, relComments)
	if part.comments == "" || !r.d.pkg.Has(part.comments) {
		part.comments = ""
		return
	}
	root, _, _, err := r.part(part.comments)
	if err != nil || root.Space != pNS || root.Local != "cmLst" {
		part.comments = ""
		return
	}
	list := elements(root, pNS, "cm")
	keys := ot.Keys(len(list))
	for i, c := range list {
		a, err1 := strconv.Atoi(c.Get("authorId"))
		idx, err2 := strconv.Atoi(c.Get("idx"))
		author := r.d.authorByID(a)
		id := commentID(slide, cmRef{a, idx})
		if err1 != nil || err2 != nil || author == nil || r.ids[id] {
			continue
		}
		attrs := ot.Values{}
		putString(attrs, "author", author.name)
		putString(attrs, "initials", author.initials)
		if t, err := time.Parse(commentTime, c.Get("dt")); err == nil {
			putString(attrs, "date", t.UTC().Format(time.RFC3339))
		} else if t, err := time.Parse(time.RFC3339, c.Get("dt")); err == nil {
			putString(attrs, "date", t.UTC().Format(time.RFC3339))
		}
		if pos := c.Child(pNS, "pos"); pos != nil {
			attrs["x"], attrs["y"] = json.RawMessage(coordinate(pos.Get("x"))), json.RawMessage(coordinate(pos.Get("y")))
		}
		if pa, pi, ok := parentOf(c); ok {
			putString(attrs, "parent", commentID(slide, cmRef{pa, pi}))
		}
		r.d.cmRefs[id] = cmRef{a, idx}
		text := strings.TrimSpace(c.Child(pNS, "text").Text())
		flow := ot.Delta{{Insert: text + "\n"}}
		r.add(id, "comment", slide, "zzz"+keys[i], attrs, flow)
	}
}

func coordinate(s string) string {
	if n, err := strconv.Atoi(s); err == nil {
		return strconv.Itoa(n)
	}
	return "0"
}

// parentOf is the comment a comment answers, in the threading PowerPoint
// 2013 adds.
func parentOf(c *xmldom.Element) (author, idx int, ok bool) {
	if ext := c.Child(pNS, "extLst"); ext != nil {
		for _, e := range ext.Elements() {
			for _, info := range e.Elements() {
				if info.Space != p15NS || info.Local != "threadingInfo" {
					continue
				}
				if p := info.Child(p15NS, "parentCm"); p != nil {
					a, err1 := strconv.Atoi(p.Get("authorId"))
					i, err2 := strconv.Atoi(p.Get("idx"))
					return a, i, err1 == nil && err2 == nil
				}
			}
		}
	}
	return 0, 0, false
}

// newComment tells whether a comment added belongs to the slide, is
// signed by author and dated, and answers a comment of the slide if any.
func newComment(c ot.Change, author, slide string, typeOf, slideOf func(string) string) bool {
	if c.Parent != slide || c.Text == nil || !plain(c.Text) {
		return false
	}
	for k, v := range c.Attrs {
		switch k {
		case "author", "initials", "date", "parent":
			var s string
			if json.Unmarshal(v, &s) != nil {
				return false
			}
		case "x", "y":
			var n int
			if json.Unmarshal(v, &n) != nil {
				return false
			}
		default:
			return false
		}
	}
	n := &ot.Node{Attrs: c.Attrs}
	initials, parent := str(n, "initials"), str(n, "parent")
	if _, err := time.Parse(time.RFC3339, str(n, "date")); err != nil {
		return false
	}
	return str(n, "author") == author && utf8.RuneCountInString(initials) <= 9 &&
		strings.IndexFunc(initials, unicode.IsControl) < 0 &&
		(parent == "" || typeOf(parent) == "comment" && slideOf(parent) == slide)
}

// plain tells whether a text carries no formatting.
func plain(d ot.Delta) bool {
	for _, o := range d {
		if len(o.Attrs) > 0 {
			return false
		}
	}
	return true
}

// commentText is the text of a comment node, without its last paragraph
// mark.
func commentText(n *ot.Node) string {
	if n.Text == nil {
		return ""
	}
	var b strings.Builder
	for _, o := range n.Text.Delta() {
		b.WriteString(o.Insert)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// commentNodes are the comments of a slide in the tree.
func commentNodes(tree *ot.Tree, slide string) []*ot.Node {
	var out []*ot.Node
	for _, n := range tree.Children(slide) {
		if n.Type == "comment" {
			out = append(out, n)
		}
	}
	return out
}

// commentsSame tells whether the comments of a slide are as they were read.
func (w *writer) commentsSame(slide string) bool {
	if w.force {
		return false
	}
	a, b := commentNodes(w.d.loaded, slide), commentNodes(w.tree, slide)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || !w.same(a[i].ID) {
			return false
		}
	}
	return true
}

// authorOf is the entry of the authors a comment is signed by, added when
// the presentation does not have it.
func (w *writer) authorOf(name, initials string) *cmAuthor {
	for i := range w.authors {
		if w.authors[i].name == name && w.authors[i].initials == initials {
			return &w.authors[i]
		}
	}
	id := 0
	for _, a := range w.authors {
		id = max(id, a.id+1)
	}
	w.authors = append(w.authors, cmAuthor{id: id, clrIdx: len(w.authors), name: name, initials: initials})
	w.authorsChanged = true
	return &w.authors[len(w.authors)-1]
}

// comments writes the comments part of a slide when its comments changed.
func (w *writer) comments(n *ot.Node, p *slidePart, rw *partrel.Writer) error {
	if w.commentsSame(n.ID) {
		return nil
	}
	nodes := commentNodes(w.tree, n.ID)
	existing := ""
	if p != nil {
		existing = p.comments
	}
	if len(nodes) == 0 {
		if existing != "" && len(commentNodes(w.d.loaded, n.ID)) > 0 {
			rw.Drop(relComments, partrel.Rel{Type: relComments})
			w.remove(existing)
		}
		return nil
	}

	refs := map[string]cmRef{}
	for _, c := range nodes {
		if ref, ok := w.d.cmRefs[c.ID]; ok && w.d.loaded.Node(c.ID) != nil {
			refs[c.ID] = ref
			continue
		}
		a := w.authorOf(str(c, "author"), str(c, "initials"))
		a.lastIdx++
		w.authorsChanged = true
		refs[c.ID] = cmRef{a.id, a.lastIdx}
	}

	root := xmldom.New(pNS, "p:cmLst", "xmlns:a", aNS, "xmlns:r", relNS, "xmlns:p", pNS)
	for _, c := range nodes {
		root.Append(w.comment(c, refs))
	}
	name := existing
	if name == "" {
		name = w.freeName("ppt/comments/comment%d.xml")
	}
	rw.Ensure(partrel.Rel{Type: relComments, Target: name})
	return w.put(name, typeComments, append([]byte(xmlHeader), root.Bytes()...))
}

func (w *writer) comment(n *ot.Node, refs map[string]cmRef) *xmldom.Element {
	ref := refs[n.ID]
	e := xmldom.New(pNS, "p:cm", "authorId", strconv.Itoa(ref.author))
	date, err := time.Parse(time.RFC3339, str(n, "date"))
	if err != nil {
		date = time.Unix(0, 0)
	}
	e.Set("dt", date.UTC().Format(commentTime))
	e.Set("idx", strconv.Itoa(ref.idx))
	e.Append(xmldom.New(pNS, "p:pos", "x", intAttr(n, "x"), "y", intAttr(n, "y")))
	text := xmldom.New(pNS, "p:text")
	text.Append(xmldom.EscapeText(commentText(n)))
	e.Append(text)
	if parent, ok := refs[str(n, "parent")]; ok {
		info := xmldom.New(p15NS, "p15:threadingInfo", "xmlns:p15", p15NS, "timeZoneBias", "0")
		info.Append(xmldom.New(p15NS, "p15:parentCm", "authorId", strconv.Itoa(parent.author), "idx", strconv.Itoa(parent.idx)))
		ext := xmldom.New(pNS, "p:ext", "uri", threadingURI)
		ext.Append(info)
		list := xmldom.New(pNS, "p:extLst")
		list.Append(ext)
		e.Append(list)
	}
	return e
}

func intAttr(n *ot.Node, key string) string {
	var v int
	if json.Unmarshal(n.Attrs[key], &v) != nil {
		return "0"
	}
	return strconv.Itoa(v)
}

// writeAuthors writes the entries of the authors of the comments, when they
// changed.
func (w *writer) writeAuthors() error {
	if !w.authorsChanged {
		return nil
	}
	root := xmldom.New(pNS, "p:cmAuthorLst", "xmlns:a", aNS, "xmlns:r", relNS, "xmlns:p", pNS)
	for _, a := range w.authors {
		root.Append(xmldom.New(pNS, "p:cmAuthor", "id", strconv.Itoa(a.id), "name", a.name, "initials", a.initials,
			"lastIdx", strconv.Itoa(a.lastIdx), "clrIdx", strconv.Itoa(a.clrIdx)))
	}
	name := w.d.authorsPart
	if name == "" {
		name = "ppt/commentAuthors.xml"
		if w.pkg.Has(name) {
			return fmt.Errorf("pptx: %s exists without a relationship", name)
		}
		w.newAuthors = name
	}
	return w.put(name, typeAuthors, append([]byte(xmlHeader), root.Bytes()...))
}
