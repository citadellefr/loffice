// Package pptx reads PowerPoint presentations into a tree of nodes (package
// ot) and writes the tree back into the presentation, rewriting only the
// parts that changed.
//
// The tree has a "deck" node, whose children are the slides in order, and
// the masters with their layouts, which are read only. A slide holds its
// shapes, and its notes if it has any:
//
//	deck                         size, notesSize, lst (default text style),
//	                             tblStyles and tblStyleDef (tablestyle.go)
//	master "M1"                  theme, clrMap, bg, title body other (text styles)
//	  layout "L1"                name, type, bg, clrMapOvr, showMasterSp
//	    shapes…                  the placeholders slides inherit from
//	slide "s256"                 layout, name, hidden, bg, clrMapOvr, showMasterSp,
//	                             transition (transition.go)
//	  sp "s256-2"                a shape; its text is the node's
//	  pic cxn frame              a picture, a connector, a table or chart
//	  grp                        a group, with its shapes
//	  alt other                  what is only kept: alternate content, ink
//	  notes "s256-notes"         the text of the notes
//
// Shapes have the attributes drawingml reads from their spPr (xfrm, geom,
// fill, line), their name, descr, hidden, their placeholder (ph), style,
// body and lst; pictures a blip. Every node written back from its XML
// carries it in "xml", its text left out: what the model does not cover is
// kept, even in a copy.
package pptx

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/maphash"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	pNS   = "http://schemas.openxmlformats.org/presentationml/2006/main"
	aNS   = drawingml.NS
	relNS = drawingml.RelNS
	mcNS  = "http://schemas.openxmlformats.org/markup-compatibility/2006"

	relOfficeDocument = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
)

var ErrNotPresentation = errors.New("pptx: not a presentation")

// Limits of the packages Open accepts.
var Limits = opc.Limits{}

// Document is a presentation open for editing.
type Document struct {
	original []byte
	mu       sync.Mutex // guards pkg once open
	pkg      *opc.Package
	// names are the relationships elements point to, by name.
	names *partrel.Names
	// trusted are the hashes of the XML the nodes carry: only that may
	// be written back. The seed is secret, so that no client can forge
	// XML with the hash of some it was sent.
	trusted map[uint64]bool
	seed    maphash.Seed
	loaded  *ot.Tree

	presName string
	presRels *partrel.Rels
	slides   map[string]*slidePart // by node id
	layouts  map[string]string     // part name of each layout node
	// layoutNodes are the layout node ids by part name.
	layoutNodes map[string]string
}

type slidePart struct {
	name  string
	rels  *partrel.Rels
	notes string
	// sldID is the id of the slide in the presentation.
	sldID int64
}

// Open reads a presentation.
func Open(data []byte) (*Document, *ot.Tree, error) {
	pkg, err := opc.Open(data, Limits)
	if err != nil {
		return nil, nil, err
	}
	d := &Document{
		original:    data,
		pkg:         pkg,
		names:       partrel.NewNames(pkg),
		trusted:     map[uint64]bool{},
		seed:        maphash.MakeSeed(),
		slides:      map[string]*slidePart{},
		layouts:     map[string]string{},
		layoutNodes: map[string]string{},
	}
	root, err := partrel.Read(pkg, "")
	if err != nil {
		return nil, nil, err
	}
	d.presName = root.OfType("", relOfficeDocument)
	if d.presName == "" {
		return nil, nil, ErrNotPresentation
	}
	r := &reader{d: d, ids: map[string]bool{}, tableStyles: map[string]bool{}}
	if err := r.presentation(); err != nil {
		return nil, nil, err
	}
	tree, err := ot.NewTree(r.nodes)
	if err != nil {
		return nil, nil, fmt.Errorf("pptx: %w", err)
	}
	d.loaded = tree.Clone()
	return d, tree, nil
}

// reader builds the nodes of the tree, parents first.
type reader struct {
	d     *Document
	nodes ot.Edit
	ids   map[string]bool
	// tableStyles are the styles the tables name.
	tableStyles map[string]bool
}

func (r *reader) add(id, typ, parent, key string, attrs ot.Values, text ot.Delta) {
	if len(attrs) == 0 {
		attrs = nil
	}
	r.ids[id] = true
	r.nodes = append(r.nodes, ot.Change{Op: ot.OpNew, ID: id, Type: typ, Parent: parent, Key: key, Attrs: attrs, Text: text})
}

// part reads an XML part, its relationships named, and the prefixes its
// root declares.
func (r *reader) part(name string) (*xmldom.Element, *partrel.Rels, map[string]string, error) {
	data, err := r.d.pkg.Read(name)
	if err != nil {
		return nil, nil, nil, err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("pptx: %s: %w", name, err)
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

// standardSpaces are the prefixes PowerPoint gives the namespaces it
// writes: XML carried in a node may land in a part that declares none.
func standardSpaces() map[string]string {
	return map[string]string{
		"a":   aNS,
		"p":   pNS,
		"r":   relNS,
		"mc":  mcNS,
		"a14": "http://schemas.microsoft.com/office/drawing/2010/main",
		"p14": "http://schemas.microsoft.com/office/powerpoint/2010/main",
		"a16": "http://schemas.microsoft.com/office/drawing/2014/main",
	}
}

// raw keeps XML a node carries as a string, and trusts it.
func (d *Document) raw(e *xmldom.Element) json.RawMessage {
	s := string(e.Bytes())
	d.trust(s)
	data, _ := json.Marshal(s)
	return data
}

func (d *Document) trust(s string) {
	d.trusted[maphash.String(d.seed, s)] = true
}

// fragment is the element XML a node carries stands for, nil unless the
// document read it.
func (d *Document) fragment(s string) *xmldom.Element {
	if s == "" || !d.trusted[maphash.String(d.seed, s)] {
		return nil
	}
	e, err := xmldom.ParseFragment([]byte(s), standardSpaces())
	if err != nil {
		return nil
	}
	return e
}

func (r *reader) presentation() error {
	pres, rels, _, err := r.part(r.d.presName)
	if err != nil {
		return err
	}
	if pres.Space != pNS || pres.Local != "presentation" {
		return ErrNotPresentation
	}
	r.d.presRels = rels
	deck := ot.Values{}
	if sz := pres.Child(pNS, "sldSz"); sz != nil {
		deck["size"] = size(sz)
	}
	if sz := pres.Child(pNS, "notesSz"); sz != nil {
		deck["notesSize"] = size(sz)
	}
	putJSON(deck, "lst", drawingml.ListStyle(pres.Child(pNS, "defaultTextStyle")))
	var lst *xmldom.Element
	if name := rels.OfType(r.d.presName, relTableStyles); name != "" {
		if data, err := r.d.pkg.Read(name); err == nil {
			if doc, err := xmldom.Parse(data); err == nil {
				lst = doc.Root
			}
		}
	}
	styles, def := tableStyles(lst, r.d.media)
	putString(deck, "tblStyleDef", def)

	masters := elements(pres.Child(pNS, "sldMasterIdLst"), pNS, "sldMasterId")
	keys := ot.Keys(1 + len(masters))
	r.add("deck", "deck", "", keys[0], deck, nil)

	layoutCount := 0
	for i, m := range masters {
		name := r.d.targetOf(m)
		if name == "" {
			continue
		}
		if err := r.master(name, "M"+strconv.Itoa(i+1), keys[1+i], &layoutCount); err != nil {
			return err
		}
	}

	slides := elements(pres.Child(pNS, "sldIdLst"), pNS, "sldId")
	keys = ot.Keys(len(slides))
	for i, s := range slides {
		name := r.d.targetOf(s)
		id, err := strconv.ParseInt(s.Get("id"), 10, 64)
		if name == "" || err != nil {
			continue
		}
		if err := r.slide(name, id, keys[i]); err != nil {
			return err
		}
	}
	addBuiltin(styles, append(slices.Sorted(maps.Keys(r.tableStyles)), def, mediumStyle2)...)
	putJSON(deck, "tblStyles", styles)
	return nil
}

// targetOf is the part an element points to by its r:id, once named.
func (d *Document) targetOf(e *xmldom.Element) string {
	if r, ok := d.names.Lookup(e.Get("r:id")); ok && !r.External {
		return r.Target
	}
	return ""
}

func (r *reader) master(name, id, key string, layoutCount *int) error {
	m, rels, spaces, err := r.part(name)
	if err != nil {
		return err
	}
	attrs := ot.Values{}
	if theme := rels.OfType(name, relTheme); theme != "" {
		if t, _, _, err := r.part(theme); err == nil {
			putJSON(attrs, "theme", readTheme(t, r.d.media))
		}
	}
	if clrMap := m.Child(pNS, "clrMap"); clrMap != nil {
		putJSON(attrs, "clrMap", attrMap(clrMap))
	}
	cSld := m.Child(pNS, "cSld")
	putJSON(attrs, "bg", background(cSld, r.d.media))
	if styles := m.Child(pNS, "txStyles"); styles != nil {
		for _, s := range []string{"title", "body", "other"} {
			putJSON(attrs, s, drawingml.ListStyle(styles.Child(pNS, s+"Style")))
		}
	}
	r.add(id, "master", "", key, attrs, nil)
	r.shapes(cSld.Child(pNS, "spTree"), id, id, spaces)

	layouts := elements(m.Child(pNS, "sldLayoutIdLst"), pNS, "sldLayoutId")
	keys := ot.Keys(len(layouts))
	for i, l := range layouts {
		part := r.d.targetOf(l)
		if part == "" {
			continue
		}
		*layoutCount++
		if err := r.layout(part, "L"+strconv.Itoa(*layoutCount), id, keys[i]); err != nil && !errors.Is(err, opc.ErrNotFound) {
			return err
		}
	}
	return nil
}

func (r *reader) layout(name, id, master, key string) error {
	l, _, spaces, err := r.part(name)
	if err != nil {
		return err
	}
	cSld := l.Child(pNS, "cSld")
	attrs := ot.Values{}
	putString(attrs, "name", cSld.Get("name"))
	putString(attrs, "type", l.Get("type"))
	putJSON(attrs, "bg", background(cSld, r.d.media))
	putJSON(attrs, "clrMapOvr", colorMapOverride(l))
	if v, ok := l.Attr("showMasterSp"); ok {
		attrs["showMasterSp"] = json.RawMessage(strconv.FormatBool(v != "0" && v != "false"))
	}
	r.d.layouts[id] = name
	r.d.layoutNodes[name] = id
	r.add(id, "layout", master, key, attrs, nil)
	r.shapes(cSld.Child(pNS, "spTree"), id, id, spaces)
	return nil
}

func (r *reader) slide(name string, sldID int64, key string) error {
	s, rels, spaces, err := r.part(name)
	if err != nil {
		return err
	}
	if s.Space != pNS || s.Local != "sld" {
		return fmt.Errorf("pptx: %s is not a slide", name)
	}
	id := "s" + strconv.FormatInt(sldID, 10)
	part := &slidePart{name: name, rels: rels, sldID: sldID, notes: rels.OfType(name, relNotesSlide)}
	r.d.slides[id] = part

	cSld := s.Child(pNS, "cSld")
	attrs := ot.Values{}
	putString(attrs, "layout", r.d.layoutNodes[rels.OfType(name, relSlideLayout)])
	putString(attrs, "name", cSld.Get("name"))
	if v, ok := s.Attr("show"); ok && (v == "0" || v == "false") {
		attrs["hidden"] = json.RawMessage("true")
	}
	putJSON(attrs, "bg", background(cSld, r.d.media))
	putJSON(attrs, "clrMapOvr", colorMapOverride(s))
	if v, ok := s.Attr("showMasterSp"); ok {
		attrs["showMasterSp"] = json.RawMessage(strconv.FormatBool(v != "0" && v != "false"))
	}
	putJSON(attrs, "transition", readTransition(s))
	spTree := cSld.Child(pNS, "spTree")
	placeholder := len(r.nodes)
	r.add(id, "slide", "deck", key, nil, nil)
	r.shapes(spTree, id, id, spaces)
	for _, c := range spTree.Elements() {
		if isShape(c) {
			spTree.Remove(c)
		}
	}
	attrs["xml"] = r.d.raw(s)
	r.nodes[placeholder].Attrs = attrs

	if part.notes != "" {
		if flow := r.notes(part.notes); flow != nil {
			r.add(id+"-notes", "notes", id, "V", nil, flow)
		}
	}
	return nil
}

// notes is the text of the body placeholder of a notes slide.
func (r *reader) notes(name string) ot.Delta {
	n, _, _, err := r.part(name)
	if err != nil {
		return nil
	}
	body := notesBody(n)
	if body == nil {
		return nil
	}
	return r.flow(body)
}

// notesBody is the text body of the body placeholder of a notes slide.
func notesBody(n *xmldom.Element) *xmldom.Element {
	var found *xmldom.Element
	walkElements(n, func(e *xmldom.Element) bool {
		if found != nil || e.Space != pNS || e.Local != "sp" {
			return found == nil
		}
		if ph := placeholder(e); ph != nil && ph.Get("type") == "body" {
			found = e.Child(pNS, "txBody")
		}
		return false
	})
	return found
}

func walkElements(e *xmldom.Element, visit func(*xmldom.Element) bool) {
	if !visit(e) {
		return
	}
	for _, c := range e.Elements() {
		walkElements(c, visit)
	}
}

// flow reads a text body, trusting the XML its attributes carry.
func (r *reader) flow(body *xmldom.Element) ot.Delta {
	flow := drawingml.Flow(body, r.d.link)
	for _, o := range flow {
		for _, k := range []string{"r", "p", "fld", "o"} {
			if v := o.Attrs[k]; v != "" {
				r.d.trust(v)
			}
		}
	}
	return flow
}

// link is the URL of an external relationship name.
func (d *Document) link(name string) string {
	if r, ok := d.names.Lookup(name); ok && r.External {
		return r.Target
	}
	return ""
}

// media is the picture a name points to, without its "@".
func (d *Document) media(name string) string {
	if r, ok := d.names.Lookup(name); ok && r.Type == partrel.Image && !r.External {
		return strings.TrimPrefix(name, "@")
	}
	return ""
}

func isShape(e *xmldom.Element) bool {
	switch {
	case e.Space == pNS:
		switch e.Local {
		case "sp", "pic", "cxnSp", "grpSp", "graphicFrame", "contentPart":
			return true
		}
	case e.Space == mcNS && e.Local == "AlternateContent":
		return true
	}
	return false
}

// shapes reads the shapes of a spTree or grpSp under parent, their ids
// made from prefix.
func (r *reader) shapes(container *xmldom.Element, parent, prefix string, spaces map[string]string) {
	if container == nil {
		return
	}
	var list []*xmldom.Element
	for _, c := range container.Elements() {
		if isShape(c) {
			list = append(list, c)
		}
	}
	keys := ot.Keys(len(list))
	for i, e := range list {
		r.shape(e, parent, prefix, keys[i], spaces)
	}
}

func (r *reader) shapeID(prefix string, e *xmldom.Element) string {
	id := "x"
	if c := cNvPr(e); c != nil && validNumber(c.Get("id")) {
		id = c.Get("id")
	}
	base := prefix + "-" + id
	id = base
	for n := 2; r.ids[id]; n++ {
		id = base + "~" + strconv.Itoa(n)
	}
	return id
}

func validNumber(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && n > 0
}

func (r *reader) shape(e *xmldom.Element, parent, prefix, key string, spaces map[string]string) {
	id := r.shapeID(prefix, e)
	if e.Space == mcNS {
		r.add(id, "alt", parent, key, ot.Values{"xml": r.d.raw(e)}, nil)
		var fallback *xmldom.Element
		for _, c := range e.Elements() {
			if c.Local == "Fallback" {
				fallback = c
			}
		}
		r.shapes(fallback, id, id, spaces)
		return
	}
	attrs := nvAttrs(e)
	switch e.Local {
	case "sp":
		r.spAttrs(attrs, e)
		body := e.Child(pNS, "txBody")
		text := ot.Delta{{Insert: "\n"}}
		if body != nil {
			putJSON(attrs, "body", nonEmpty(drawingml.BodyProps(body.Child(aNS, "bodyPr"))))
			putJSON(attrs, "lst", drawingml.ListStyle(body.Child(aNS, "lstStyle")))
			text = r.flow(body)
			for _, p := range body.Elements() {
				if p.Space == aNS && p.Local == "p" {
					body.Remove(p)
				}
			}
		}
		attrs["xml"] = r.d.raw(e)
		r.add(id, "sp", parent, key, attrs, text)
	case "pic":
		r.spAttrs(attrs, e)
		putJSON(attrs, "blip", drawingml.ReadBlip(e.Child(pNS, "blipFill"), r.d.media))
		attrs["xml"] = r.d.raw(e)
		r.add(id, "pic", parent, key, attrs, nil)
	case "cxnSp":
		r.spAttrs(attrs, e)
		attrs["xml"] = r.d.raw(e)
		r.add(id, "cxn", parent, key, attrs, nil)
	case "grpSp":
		for k, v := range drawingml.ShapeProps(e.Child(pNS, "grpSpPr"), r.d.media) {
			attrs[k] = v
		}
		r.add(id, "grp", parent, key, nil, nil)
		at := len(r.nodes) - 1
		r.shapes(e, id, prefix, spaces)
		for _, c := range e.Elements() {
			if isShape(c) {
				e.Remove(c)
			}
		}
		attrs["xml"] = r.d.raw(e)
		r.nodes[at].Attrs = attrs
	case "graphicFrame":
		putJSON(attrs, "xfrm", drawingml.ReadXfrm(e.Child(pNS, "xfrm")))
		kind := frameKind(e)
		putString(attrs, "frame", kind)
		if kind == "chart" {
			if c := r.d.chart(e); c != nil {
				attrs["chart"] = c.JSON()
			}
		}
		tbl := tableOf(e)
		if tbl == nil {
			attrs["xml"] = r.d.raw(e)
			r.add(id, "frame", parent, key, attrs, nil)
			return
		}
		at := len(r.nodes)
		r.add(id, "frame", parent, key, nil, nil)
		r.table(tbl, id, attrs)
		attrs["xml"] = r.d.raw(e)
		r.nodes[at].Attrs = attrs
	default:
		r.add(id, "other", parent, key, ot.Values{"xml": r.d.raw(e)}, nil)
	}
}

// spAttrs reads the spPr and style of a shape.
func (r *reader) spAttrs(attrs ot.Values, e *xmldom.Element) {
	for k, v := range drawingml.ShapeProps(e.Child(pNS, "spPr"), r.d.media) {
		attrs[k] = v
	}
	putJSON(attrs, "style", readStyle(e.Child(pNS, "style")))
}

// cNvPr is the non-visual properties of a shape.
func cNvPr(e *xmldom.Element) *xmldom.Element {
	for _, c := range e.Elements() {
		if strings.HasPrefix(c.Local, "nv") {
			return c.Child(pNS, "cNvPr")
		}
	}
	return nil
}

func placeholder(e *xmldom.Element) *xmldom.Element {
	for _, c := range e.Elements() {
		if strings.HasPrefix(c.Local, "nv") {
			if nvPr := c.Child(pNS, "nvPr"); nvPr != nil {
				return nvPr.Child(pNS, "ph")
			}
		}
	}
	return nil
}

// nvAttrs reads the name, description and placeholder of a shape.
func nvAttrs(e *xmldom.Element) ot.Values {
	attrs := ot.Values{}
	if c := cNvPr(e); c != nil {
		putString(attrs, "name", c.Get("name"))
		putString(attrs, "descr", c.Get("descr"))
		if v := c.Get("hidden"); v == "1" || v == "true" {
			attrs["hidden"] = json.RawMessage("true")
		}
	}
	if ph := placeholder(e); ph != nil {
		m := attrMap(ph)
		if m == nil {
			m = map[string]string{}
		}
		attrs["ph"], _ = json.Marshal(m)
	}
	return attrs
}

func frameKind(e *xmldom.Element) string {
	var uri string
	if g := e.Child(aNS, "graphic"); g != nil {
		if gd := g.Child(aNS, "graphicData"); gd != nil {
			uri = gd.Get("uri")
		}
	}
	switch uri {
	case "http://schemas.openxmlformats.org/drawingml/2006/table":
		return "table"
	case "http://schemas.openxmlformats.org/drawingml/2006/chart", "http://schemas.microsoft.com/office/drawing/2014/chartex":
		return "chart"
	case "http://schemas.openxmlformats.org/drawingml/2006/diagram":
		return "diagram"
	case "http://schemas.openxmlformats.org/presentationml/2006/ole":
		return "ole"
	}
	return "other"
}

// chart reads the chart a graphic frame shows, nil when it cannot.
func (d *Document) chart(frame *xmldom.Element) *chart.Chart {
	g := frame.Child(aNS, "graphic")
	if g == nil {
		return nil
	}
	gd := g.Child(aNS, "graphicData")
	if gd == nil {
		return nil
	}
	ref := gd.Child(chart.NS, "chart")
	if ref == nil {
		return nil
	}
	data, err := d.pkg.Read(d.targetOf(ref))
	if err != nil {
		return nil
	}
	c, err := chart.Read(data)
	if err != nil {
		return nil
	}
	return c
}

func size(e *xmldom.Element) json.RawMessage {
	w, _ := strconv.ParseInt(e.Get("cx"), 10, 64)
	h, _ := strconv.ParseInt(e.Get("cy"), 10, 64)
	data, _ := json.Marshal(map[string]int64{"w": w, "h": h})
	return data
}

// elements are the children of e with that name.
func elements(e *xmldom.Element, space, local string) []*xmldom.Element {
	var out []*xmldom.Element
	if e == nil {
		return out
	}
	for _, c := range e.Elements() {
		if c.Space == space && c.Local == local {
			out = append(out, c)
		}
	}
	return out
}

// attrMap is the attributes of e, nil when it has none.
func attrMap(e *xmldom.Element) map[string]string {
	var m map[string]string
	for _, a := range e.Attrs {
		if a.Name == "xmlns" || strings.HasPrefix(a.Name, "xmlns:") {
			continue
		}
		if m == nil {
			m = map[string]string{}
		}
		m[a.Name] = a.Value
	}
	return m
}

func nonEmpty(p drawingml.Props) drawingml.Props {
	if len(p) == 0 {
		return nil
	}
	return p
}

// putJSON sets v[key] to x as JSON, unless x is nil or empty.
func putJSON[T any](v ot.Values, key string, x T) {
	data, err := json.Marshal(x)
	if err != nil || string(data) == "null" || string(data) == "{}" {
		return
	}
	v[key] = data
}

func putString(v ot.Values, key, s string) {
	if s != "" {
		data, _ := json.Marshal(s)
		v[key] = data
	}
}

// Background is how a slide, layout or master is painted: a fill, or a
// reference to one of the theme's background fills with its color.
type Background struct {
	Fill  *drawingml.Fill  `json:"fill,omitempty"`
	Ref   int64            `json:"ref,omitempty"`
	Color *drawingml.Color `json:"color,omitempty"`
}

func background(cSld *xmldom.Element, media func(rid string) string) *Background {
	bg := cSld.Child(pNS, "bg")
	if bg == nil {
		return nil
	}
	if pr := bg.Child(pNS, "bgPr"); pr != nil {
		if f := drawingml.FillIn(pr, media); f != nil {
			return &Background{Fill: f}
		}
	}
	if ref := bg.Child(pNS, "bgRef"); ref != nil {
		idx, _ := strconv.ParseInt(ref.Get("idx"), 10, 64)
		return &Background{Ref: idx, Color: drawingml.ColorIn(ref)}
	}
	return nil
}

func colorMapOverride(e *xmldom.Element) map[string]string {
	o := e.Child(pNS, "clrMapOvr")
	if o == nil {
		return nil
	}
	if m := o.Child(aNS, "overrideClrMapping"); m != nil {
		return attrMap(m)
	}
	return nil
}

// Style points a shape to the theme's line, fill, effect and font styles,
// each with a color.
type Style struct {
	Line   *StyleRef `json:"ln,omitempty"`
	Fill   *StyleRef `json:"fill,omitempty"`
	Effect *StyleRef `json:"effect,omitempty"`
	Font   *StyleRef `json:"font,omitempty"`
}

// StyleRef is an index in the theme's list of styles, or "major" or
// "minor" for a font.
type StyleRef struct {
	Idx   string           `json:"idx"`
	Color *drawingml.Color `json:"color,omitempty"`
}

func readStyle(e *xmldom.Element) *Style {
	if e == nil {
		return nil
	}
	ref := func(local string) *StyleRef {
		c := e.Child(aNS, local)
		if c == nil {
			return nil
		}
		return &StyleRef{Idx: c.Get("idx"), Color: drawingml.ColorIn(c)}
	}
	return &Style{Line: ref("lnRef"), Fill: ref("fillRef"), Effect: ref("effectRef"), Font: ref("fontRef")}
}

// Theme is what a master's theme gives its slides.
type Theme struct {
	Colors  map[string]*drawingml.Color  `json:"colors,omitempty"`
	Fonts   map[string]map[string]string `json:"fonts,omitempty"`
	Fills   []*drawingml.Fill            `json:"fills,omitempty"`
	Lines   []*drawingml.Line            `json:"lines,omitempty"`
	BgFills []*drawingml.Fill            `json:"bgFills,omitempty"`
}

func readTheme(t *xmldom.Element, media func(rid string) string) *Theme {
	els := t.Child(aNS, "themeElements")
	if els == nil {
		return nil
	}
	th := &Theme{}
	if scheme := els.Child(aNS, "clrScheme"); scheme != nil {
		th.Colors = map[string]*drawingml.Color{}
		for _, c := range scheme.Elements() {
			if color := drawingml.ColorIn(c); color != nil {
				th.Colors[c.Local] = color
			}
		}
	}
	if fonts := els.Child(aNS, "fontScheme"); fonts != nil {
		th.Fonts = map[string]map[string]string{}
		for _, kind := range []string{"major", "minor"} {
			f := fonts.Child(aNS, kind+"Font")
			if f == nil {
				continue
			}
			m := map[string]string{}
			for _, c := range f.Elements() {
				switch c.Local {
				case "latin", "ea", "cs":
					m[c.Local] = c.Get("typeface")
				case "font":
					m[c.Get("script")] = c.Get("typeface")
				}
			}
			th.Fonts[kind] = m
		}
	}
	if fmt := els.Child(aNS, "fmtScheme"); fmt != nil {
		if l := fmt.Child(aNS, "fillStyleLst"); l != nil {
			th.Fills = fills(l, media)
		}
		if l := fmt.Child(aNS, "bgFillStyleLst"); l != nil {
			th.BgFills = fills(l, media)
		}
		if l := fmt.Child(aNS, "lnStyleLst"); l != nil {
			for _, ln := range l.Elements() {
				th.Lines = append(th.Lines, drawingml.ReadLine(ln))
			}
		}
	}
	return th
}

// fills are the fills of a list of styles, one for each, nil where one is
// not understood.
func fills(list *xmldom.Element, media func(rid string) string) []*drawingml.Fill {
	var out []*drawingml.Fill
	for _, c := range list.Elements() {
		wrapper := xmldom.New(aNS, "a:w")
		wrapper.Append(c)
		out = append(out, drawingml.FillIn(wrapper, media))
	}
	return out
}
