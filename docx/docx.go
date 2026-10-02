// Package docx reads Word documents into a tree of nodes (package ot) and
// writes the tree back into the document, rewriting only the parts that
// changed.
//
//	doc "doc"            styles, defaults, numbering, fonts, colors, settings;
//	                     sect and sx, the section of the last blocks
//	  body "body"        the blocks of the document, in order:
//	    text             paragraphs following one another, as a flow
//	    tbl              a table: tblPr, grid; its rows
//	      tr             a row: trPr; its cells
//	        tc           a cell: tcPr; its blocks
//	    sdt              a content control around blocks
//	    other            what is only kept: a bookmark between blocks…
//	  hdr ftr            a header or footer, with its blocks
//
// A flow holds the text of paragraphs, "\n" ending each: its attributes
// are those of the paragraph and of its mark (see Props), with "p" the pPr
// it was read from, "pa" the paragraph element and, for the last paragraph
// of a section, "sect" and "sx" its sectPr. A character is text, "\t" a
// tab, "\v" a line break, or Object for any other element of a run, kept
// whole in "o", or of the paragraph, in "po". The hyperlinks, fields,
// revisions and content controls around runs ride along in "wrap", a JSON
// list of the elements without their content, outermost first.
//
// Nodes read from XML carry it in "xml", their content left out: what the
// model does not cover is kept, even in a copy.
package docx

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"strconv"
	"strings"
	"sync"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/ot"
)

const (
	NS    = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	relNS = partrel.NS
	mcNS  = "http://schemas.openxmlformats.org/markup-compatibility/2006"

	relOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	relStyles         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"
	relNumbering      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/numbering"
	relSettings       = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings"
	relTheme          = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme"
	relHeader         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/header"
	relFooter         = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer"
	relHyperlink      = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink"
)

// relNotes are the relationships of the parts of footnotes and endnotes.
var relNotes = map[string]string{
	"footnote": "http://schemas.openxmlformats.org/officeDocument/2006/relationships/footnotes",
	"endnote":  "http://schemas.openxmlformats.org/officeDocument/2006/relationships/endnotes",
}

// Characters of a flow that stand for something other than text.
const (
	LineBreak = "\v"
	Tab       = "\t"
	// Object is an element the model does not know, kept whole.
	Object = "￼"
)

var ErrNotDocument = errors.New("docx: not a Word document")

// Limits of the packages Open accepts.
var Limits = opc.Limits{}

// Document is a Word document open for editing.
type Document struct {
	original []byte
	mu       sync.Mutex // guards pkg once open
	pkg      *opc.Package
	names    *partrel.Names
	// trusted are the hashes of the XML the nodes carry: only that may
	// be written back. The seed is secret, so that no client can forge
	// XML with the hash of some it was sent.
	trusted map[uint64]bool
	seed    maphash.Seed
	loaded  *ot.Tree

	main     string
	mainRels *partrel.Rels
	// spaces are the prefixes the main part declares.
	spaces map[string]string
	// parts are the headers and footers, by node id; notes the parts of
	// footnotes and endnotes, by kind.
	parts map[string]*part
	notes map[string]*part
	// comments is the part of the comments, commentMax the highest w:id
	// it gives one.
	comments   *part
	commentMax int
	// pending are the pictures clients added, by the part they go in.
	pending map[string]*pending
	// fileStyles are the ids of the styles styles.xml defines.
	fileStyles map[string]bool
}

type part struct {
	name string
	rels *partrel.Rels
}

// Open reads a Word document.
func Open(data []byte) (*Document, *ot.Tree, error) {
	pkg, err := opc.Open(data, Limits)
	if err != nil {
		return nil, nil, err
	}
	d := &Document{
		original:   data,
		pkg:        pkg,
		names:      partrel.NewNames(pkg),
		trusted:    map[uint64]bool{},
		seed:       maphash.MakeSeed(),
		parts:      map[string]*part{},
		notes:      map[string]*part{},
		pending:    map[string]*pending{},
		commentMax: -1,
	}
	root, err := partrel.Read(pkg, "")
	if err != nil {
		return nil, nil, err
	}
	d.main = root.OfType("", relOfficeDocument)
	if d.main == "" {
		return nil, nil, ErrNotDocument
	}
	r := &reader{d: d, headers: map[string]string{}}
	if err := r.document(); err != nil {
		return nil, nil, err
	}
	tree, err := ot.NewTree(r.nodes)
	if err != nil {
		return nil, nil, fmt.Errorf("docx: %w", err)
	}
	d.loaded = tree.Clone()
	return d, tree, nil
}

// reader builds the nodes of the tree, parents first.
type reader struct {
	d     *Document
	nodes ot.Edit
	next  int
	// headers are the node ids of the headers and footers, by the name
	// of their relationship.
	headers map[string]string
	// comments are the elements of the comments, by node id, in the order
	// of commentOrder.
	comments     map[string]*xmldom.Element
	commentOrder []string
}

func (r *reader) add(typ, parent, key string, attrs ot.Values, text ot.Delta) string {
	r.next++
	id := strconv.Itoa(r.next)
	r.addID(id, typ, parent, key, attrs, text)
	return id
}

func (r *reader) addID(id, typ, parent, key string, attrs ot.Values, text ot.Delta) {
	if len(attrs) == 0 {
		attrs = nil
	}
	r.nodes = append(r.nodes, ot.Change{Op: ot.OpNew, ID: id, Type: typ, Parent: parent, Key: key, Attrs: attrs, Text: text})
}

// part reads an XML part with its relationships named, and the prefixes
// its root declares.
func (r *reader) part(name string) (*xmldom.Element, *partrel.Rels, map[string]string, error) {
	data, err := r.d.pkg.Read(name)
	if err != nil {
		return nil, nil, nil, err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("docx: %s: %w", name, err)
	}
	rels, err := partrel.Read(r.d.pkg, name)
	if err != nil {
		return nil, nil, nil, err
	}
	spaces := standardSpaces()
	for k, v := range doc.Root.Spaces() {
		spaces[k] = v
	}
	r.d.names.NameAll(doc.Root, name, rels, spaces)
	return doc.Root, rels, spaces, nil
}

// standardSpaces are the prefixes Word gives the namespaces it writes: XML
// carried in a node may land in a part that declares none.
func standardSpaces() map[string]string {
	return map[string]string{
		"w":    NS,
		"r":    relNS,
		"mc":   mcNS,
		"wp":   "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing",
		"a":    "http://schemas.openxmlformats.org/drawingml/2006/main",
		"pic":  "http://schemas.openxmlformats.org/drawingml/2006/picture",
		"v":    "urn:schemas-microsoft-com:vml",
		"o":    "urn:schemas-microsoft-com:office:office",
		"m":    "http://schemas.openxmlformats.org/officeDocument/2006/math",
		"w10":  "urn:schemas-microsoft-com:office:word",
		"w14":  "http://schemas.microsoft.com/office/word/2010/wordml",
		"w15":  "http://schemas.microsoft.com/office/word/2012/wordml",
		"wps":  "http://schemas.microsoft.com/office/word/2010/wordprocessingShape",
		"wpg":  "http://schemas.microsoft.com/office/word/2010/wordprocessingGroup",
		"wp14": "http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing",
	}
}

// raw keeps XML a node carries as a string, and trusts it.
func (d *Document) raw(e *xmldom.Element) string {
	s := string(e.Bytes())
	d.trusted[maphash.String(d.seed, s)] = true
	return s
}

func (d *Document) rawValue(e *xmldom.Element) json.RawMessage {
	data, _ := json.Marshal(d.raw(e))
	return data
}

// fragment is the element XML a node carries stands for, nil unless the
// document read it.
func (d *Document) fragment(s string) *xmldom.Element {
	if s == "" || !d.trusted[maphash.String(d.seed, s)] {
		return nil
	}
	e, err := xmldom.ParseFragment([]byte(s), d.spaces)
	if err != nil {
		return nil
	}
	return e
}

// shell is a copy of e without its content but for the children keep
// tells to keep.
func shell(e *xmldom.Element, keep func(c *xmldom.Element) bool) *xmldom.Element {
	c := e.Clone()
	var kept []xmldom.Node
	for _, n := range c.Content {
		if el, ok := n.(*xmldom.Element); ok && keep != nil && keep(el) {
			kept = append(kept, el)
		}
	}
	c.Content = kept
	return c
}

// properties tells the children that describe an element from those it
// holds.
func properties(c *xmldom.Element) bool {
	return strings.HasSuffix(c.Local, "Pr") || c.Local == "fldData" || c.Local == "sdtEndPr"
}

func (r *reader) document() error {
	root, rels, spaces, err := r.part(r.d.main)
	if err != nil {
		return err
	}
	if root.Space != NS || root.Local != "document" {
		return ErrNotDocument
	}
	body := child(root, "body")
	if body == nil {
		return ErrNotDocument
	}
	r.d.mainRels, r.d.spaces = rels, spaces

	attrs := ot.Values{}
	r.styles(attrs, rels)
	keys := ot.Keys(2)
	r.addID("doc", "doc", "", keys[0], attrs, nil)

	var headers []*xmldom.Element
	for _, x := range rels.List {
		if x.External || x.Type != relHeader && x.Type != relFooter {
			continue
		}
		name := rels.Target(r.d.main, x.ID)
		if name == "" || !r.d.pkg.Has(name) {
			continue
		}
		e := xmldom.New("", "ref", "id", r.d.names.Name(r.d.main, rels, x.ID), "type", x.Type, "part", name)
		headers = append(headers, e)
	}
	hkeys := ot.Keys(1 + len(headers))
	count := map[string]int{}
	for i, h := range headers {
		typ, prefix := "hdr", "h"
		if h.Get("type") == relFooter {
			typ, prefix = "ftr", "f"
		}
		count[typ]++
		id := prefix + strconv.Itoa(count[typ])
		r.headers[h.Get("id")] = id
		if err := r.header(id, typ, h.Get("part"), hkeys[1+i]); err != nil {
			return err
		}
	}

	if err := r.readComments(rels); err != nil {
		return err
	}
	r.addID("body", "body", "doc", keys[1], nil, nil)
	r.blocks(body, "body")
	if s := child(body, "sectPr"); s != nil {
		r.section(r.nodes[0].Attrs, s)
	}
	last := keys[1]
	for _, h := range hkeys {
		last = max(last, h)
	}
	for _, kind := range []string{"footnote", "endnote"} {
		name := rels.OfType(r.d.main, relNotes[kind])
		if name == "" || !r.d.pkg.Has(name) {
			continue
		}
		var err error
		if last, err = r.notes(kind, name, last); err != nil {
			return err
		}
	}
	return r.commentNodes(rels, last)
}

// notes reads the footnotes or endnotes of a part, each into a "note" node
// keyed after [last]; the separators Word draws them under are only kept.
func (r *reader) notes(kind, partName, last string) (string, error) {
	root, rels, _, err := r.part(partName)
	if err != nil {
		return last, err
	}
	r.d.notes[kind] = &part{name: partName, rels: rels}
	seen := map[string]bool{}
	for _, n := range root.Elements() {
		if n.Space != NS || n.Local != kind || attr(n, "type") != "" && attr(n, "type") != "normal" {
			continue
		}
		id := kind[:1] + "n" + attr(n, "id")
		if !name(id) || seen[id] {
			continue
		}
		seen[id] = true
		last = ot.KeyBetween(last, "")
		r.addID(id, "note", "doc", last, ot.Values{"kind": json.RawMessage(`"` + kind + `"`)}, nil)
		r.blocks(n, id)
	}
	return last, nil
}

// header reads a header or footer part into a node of its own.
func (r *reader) header(id, typ, name, key string) error {
	root, rels, _, err := r.part(name)
	if err != nil {
		return err
	}
	r.d.parts[id] = &part{name: name, rels: rels}
	r.addID(id, typ, "doc", key, nil, nil)
	r.blocks(root, id)
	return nil
}

// block is a node about to be added: blocks are read before their keys are
// known.
type block struct {
	typ   string
	attrs ot.Values
	flow  ot.Delta
	e     *xmldom.Element
}

// blocks reads the block content of an element: the body, a cell, a
// header, a content control.
func (r *reader) blocks(e *xmldom.Element, parent string) {
	var list []block
	var flow ot.Delta
	endFlow := func() {
		if flow != nil {
			list = append(list, block{typ: "text", flow: flow})
			flow = nil
		}
	}
	for _, c := range e.Elements() {
		if c.Space == NS {
			switch c.Local {
			case "p":
				flow = r.paragraph(c, flow)
				continue
			case "sectPr", "tcPr", "sdtPr", "sdtEndPr", "customXmlPr":
				continue
			case "tbl":
				endFlow()
				list = append(list, block{typ: "tbl", e: c})
				continue
			case "sdt":
				if content := child(c, "sdtContent"); content != nil {
					endFlow()
					list = append(list, block{typ: "sdt", e: c})
					continue
				}
			case "proofErr":
				continue
			}
		}
		endFlow()
		list = append(list, block{typ: "other", attrs: ot.Values{"xml": r.d.rawValue(c)}})
	}
	endFlow()
	keys := ot.Keys(len(list))
	for i, b := range list {
		switch b.typ {
		case "text":
			r.add("text", parent, keys[i], nil, b.flow)
		case "other":
			r.add("other", parent, keys[i], b.attrs, nil)
		case "tbl":
			r.table(b.e, parent, keys[i])
		case "sdt":
			id := r.add("sdt", parent, keys[i], ot.Values{"xml": r.d.rawValue(shell(b.e, properties))}, nil)
			r.blocks(child(b.e, "sdtContent"), id)
		}
	}
}

// paragraph reads a w:p onto the end of a flow.
func (r *reader) paragraph(p *xmldom.Element, flow ot.Delta) ot.Delta {
	mark := ot.Attrs{}
	if len(p.Attrs) > 0 {
		mark["pa"] = r.d.raw(shell(p, nil))
	}
	if pPr := child(p, "pPr"); pPr != nil {
		pPr = pPr.Clone()
		if s := child(pPr, "sectPr"); s != nil {
			pPr.Remove(s)
			sect, raw := r.sect(s)
			mark["sect"], mark["sx"] = sect, raw
		}
		for k, v := range ParaProps(pPr) {
			mark[k] = v
		}
		for k, v := range RunProps(child(pPr, "rPr")) {
			mark[k] = v
		}
		for _, c := range elements(child(pPr, "rPr")) {
			readRevision(mark, c)
		}
		mark["p"] = r.d.raw(pPr)
	}
	flow = r.inline(p, nil, flow)
	return flow.Push(ot.Op{Insert: "\n", Attrs: mark})
}

// inline reads what a paragraph, or an element around runs, holds; wrap
// are the elements around, outermost first.
func (r *reader) inline(e *xmldom.Element, wrap []string, flow ot.Delta) ot.Delta {
	for _, c := range e.Elements() {
		if c.Space != NS {
			flow = r.object(flow, "po", c, wrap)
			continue
		}
		if properties(c) {
			continue
		}
		switch c.Local {
		case "proofErr":
		case "r":
			flow = r.run(c, wrap, flow)
		case "hyperlink", "smartTag", "customXml", "fldSimple", "ins", "del", "moveFrom", "moveTo", "dir", "bdo":
			flow = r.wrapped(c, c, wrap, flow)
		case "sdt":
			if content := child(c, "sdtContent"); content != nil {
				flow = r.wrapped(c, content, wrap, flow)
			} else {
				flow = r.object(flow, "po", c, wrap)
			}
		default:
			flow = r.object(flow, "po", c, wrap)
		}
	}
	return flow
}

// wrapped reads the runs of content, which e wraps.
func (r *reader) wrapped(e, content *xmldom.Element, wrap []string, flow ot.Delta) ot.Delta {
	before := flow.Change()
	s := shell(e, properties)
	inner := r.inline(content, append(wrap[:len(wrap):len(wrap)], r.d.raw(s)), flow)
	if inner.Change() == before {
		return r.object(flow, "po", e, wrap)
	}
	return inner
}

// object adds an element as an Object, under key "o" for the content of a
// run, "po" for that of a paragraph.
func (r *reader) object(flow ot.Delta, key string, e *xmldom.Element, wrap []string) ot.Delta {
	attrs := r.wrapAttrs(nil, wrap)
	if attrs == nil {
		attrs = ot.Attrs{}
	}
	attrs[key] = r.d.raw(e)
	r.describe(e, attrs)
	return flow.Push(ot.Op{Insert: Object, Attrs: clean(attrs)})
}

// wrapAttrs adds to attrs the elements around a run, and what they mean.
func (r *reader) wrapAttrs(attrs ot.Attrs, wrap []string) ot.Attrs {
	if len(wrap) == 0 {
		return attrs
	}
	if attrs == nil {
		attrs = ot.Attrs{}
	}
	data, _ := json.Marshal(wrap)
	attrs["wrap"] = string(data)
	for _, s := range wrap {
		e := r.d.fragment(s)
		if e == nil || e.Space != NS {
			continue
		}
		switch e.Local {
		case "hyperlink":
			if a := attr(e, "anchor"); a != "" {
				attrs["link"] = "#" + a
			} else if l, ok := r.d.names.Lookup(e.Get("r:id")); ok && l.External {
				attrs["link"] = l.Target
			}
		case "ins", "moveTo", "del", "moveFrom":
			readRevision(attrs, e)
		case "fldSimple":
			attrs["field"] = strings.TrimSpace(attr(e, "instr"))
		}
	}
	return attrs
}

// run reads the content of a w:r, each child a character or text.
func (r *reader) run(e *xmldom.Element, wrap []string, flow ot.Delta) ot.Delta {
	var attrs ot.Attrs
	if rPr := child(e, "rPr"); rPr != nil {
		attrs = ot.Attrs(RunProps(rPr))
		attrs["r"] = r.d.raw(rPr)
	}
	attrs = clean(r.wrapAttrs(attrs, wrap))
	push := func(s string) {
		flow = flow.Push(ot.Op{Insert: s, Attrs: attrs})
	}
	for _, c := range e.Elements() {
		if c.Space == NS {
			switch c.Local {
			case "rPr", "lastRenderedPageBreak":
				continue
			case "t", "delText":
				if s := text(c); s != "" {
					push(s)
				}
				continue
			case "tab":
				push(Tab)
				continue
			case "cr":
				push(LineBreak)
				continue
			case "br":
				if len(c.Attrs) == 0 {
					push(LineBreak)
					continue
				}
			case "noBreakHyphen":
				push("‑")
				continue
			case "softHyphen":
				push("­")
				continue
			}
		}
		with := ot.Attrs{}
		for k, v := range attrs {
			with[k] = v
		}
		with["o"] = r.d.raw(c)
		r.describe(c, with)
		flow = flow.Push(ot.Op{Insert: Object, Attrs: clean(with)})
	}
	return flow
}

// clean drops the attributes without a value, which a flow may not insert.
func clean(attrs ot.Attrs) ot.Attrs {
	for k, v := range attrs {
		if v == "" {
			delete(attrs, k)
		}
	}
	return attrs
}

// text is what a w:t holds, with the characters a flow gives a meaning to
// made spaces.
func text(t *xmldom.Element) string {
	s := t.Text()
	if strings.ContainsAny(s, "\n\r\v￼") {
		s = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == '\v' || r == 0xFFFC {
				return ' '
			}
			return r
		}, s)
	}
	return s
}
