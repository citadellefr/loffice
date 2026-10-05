package pptx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/drawingml"
	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	typeSlide = "application/vnd.openxmlformats-officedocument.presentationml.slide+xml"
	xmlHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\r\n"
)

// Save writes the presentation as the tree has it. Only the parts whose
// content changed are rewritten: a slide, when anything on it changed; its
// notes; the presentation, when slides came, went or moved.
func (d *Document) Save(tree *ot.Tree) ([]byte, error) {
	pkg, err := opc.Open(d.original, Limits)
	if err != nil {
		return nil, err
	}
	w := &writer{d: d, pkg: pkg, tree: tree, fresh: map[*xmldom.Element]bool{}, authors: slices.Clone(d.authors)}
	if err := w.save(); err != nil {
		return nil, err
	}
	return pkg.Bytes()
}

type writer struct {
	d    *Document
	pkg  *opc.Package
	tree *ot.Tree
	// fresh are the ids of the shapes created since the document was
	// read, which may need new ones.
	fresh map[*xmldom.Element]bool
	// authors are those of the comments, as the writer has them.
	authors        []cmAuthor
	authorsChanged bool
	// newAuthors is the part of the authors, when the writer adds it.
	newAuthors string
	// force rewrites every slide, for tests.
	force bool
}

func (w *writer) rels(source string, rels *partrel.Rels) *partrel.Writer {
	return partrel.NewWriter(source, rels, w.d.names.Lookup)
}

// slide is a slide as the presentation lists it.
type slide struct {
	node *ot.Node
	part string
	id   int64
}

func (w *writer) save() error {
	var slides []slide
	used := map[int64]bool{}
	for _, n := range w.tree.Children("deck") {
		if n.Type != "slide" {
			continue
		}
		s := slide{node: n}
		if p := w.d.slides[n.ID]; p != nil && !used[p.sldID] {
			s.part, s.id = p.name, p.sldID
			used[s.id] = true
		}
		slides = append(slides, s)
	}
	next := int64(256)
	for id := range used {
		next = max(next, id+1)
	}
	for i := range slides {
		s := &slides[i]
		p := w.d.slides[s.node.ID]
		switch {
		case s.part == "":
			s.id = next
			next++
			s.part = w.freeName("ppt/slides/slide%d.xml")
			if err := w.slide(s.node, s.part, nil, true); err != nil {
				return err
			}
		case !w.same(s.node.ID):
			if err := w.slide(s.node, s.part, p.rels, false); err != nil {
				return err
			}
		}
		if p != nil && p.notes != "" && !w.same(s.node.ID+"-notes") {
			if err := w.notes(s.node.ID+"-notes", p.notes); err != nil {
				return err
			}
		}
	}
	for id, p := range w.d.slides {
		if w.tree.Node(id) == nil || !slices.ContainsFunc(slides, func(s slide) bool { return s.part == p.name }) {
			w.remove(p.name)
			if p.notes != "" {
				w.remove(p.notes)
			}
		}
	}
	if err := w.writeAuthors(); err != nil {
		return err
	}
	if w.listed(slides) && w.newAuthors == "" {
		return nil
	}
	return w.presentation(slides)
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
	if a.Type != b.Type || a.Key != b.Key || !maps.EqualFunc(a.Attrs, b.Attrs, func(x, y json.RawMessage) bool { return bytes.Equal(x, y) }) {
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

// listed tells whether the presentation lists these slides already.
func (w *writer) listed(slides []slide) bool {
	if w.force {
		return false
	}
	var before []string
	for _, n := range w.d.loaded.Children("deck") {
		if n.Type == "slide" {
			before = append(before, w.d.slides[n.ID].name)
		}
	}
	if len(before) != len(slides) {
		return false
	}
	for i, s := range slides {
		if before[i] != s.part {
			return false
		}
	}
	return true
}

func (w *writer) freeName(pattern string) string {
	for n := 1; ; n++ {
		if name := fmt.Sprintf(pattern, n); !w.pkg.Has(name) {
			return name
		}
	}
}

func (w *writer) remove(part string) {
	_ = w.pkg.Remove(part)
	if rels := opc.RelsName(part); w.pkg.Has(rels) {
		_ = w.pkg.Remove(rels)
	}
}

// put writes a part, adding it when it is new.
func (w *writer) put(name, contentType string, data []byte) error {
	if w.pkg.Has(name) {
		return w.pkg.Set(name, data)
	}
	return w.pkg.Add(name, contentType, data)
}

func (w *writer) putRels(rw *partrel.Writer, isNew bool) error {
	if !rw.Changed && !isNew {
		return nil
	}
	return w.put(opc.RelsName(rw.Source), "application/vnd.openxmlformats-package.relationships+xml", opc.MarshalRelationships(rw.List))
}

// str is a string attribute of a node, "" if it has none.
func str(n *ot.Node, key string) string {
	var s string
	if v, ok := n.Attrs[key]; ok {
		_ = json.Unmarshal(v, &s)
	}
	return s
}

func (w *writer) slide(n *ot.Node, name string, rels *partrel.Rels, isNew bool) error {
	root := w.d.fragment(str(n, "xml"))
	if root == nil || root.Space != pNS || root.Local != "sld" {
		root = newSlide()
	}
	declare(root)
	cSld := root.Child(pNS, "cSld")
	old := ot.Values{}
	putString(old, "name", cSld.Get("name"))
	if v, ok := root.Attr("show"); ok && (v == "0" || v == "false") {
		old["hidden"] = json.RawMessage("true")
	}
	putJSON(old, "bg", background(cSld, w.d.media))
	putJSON(old, "transition", readTransition(root))
	if changed(old, n.Attrs, "name") {
		if s := str(n, "name"); s != "" {
			cSld.Set("name", s)
		} else {
			cSld.Unset("name")
		}
	}
	if changed(old, n.Attrs, "hidden") {
		if string(n.Attrs["hidden"]) == "true" {
			root.Set("show", "0")
		} else {
			root.Unset("show")
		}
	}
	if changed(old, n.Attrs, "bg") {
		setBackground(cSld, n.Attrs["bg"], w.embed)
	}
	if changed(old, n.Attrs, "transition") {
		setTransition(root, n.Attrs["transition"])
	}

	spTree := cSld.Child(pNS, "spTree")
	for _, c := range w.tree.Children(n.ID) {
		if e := w.shape(c); e != nil {
			spTree.Append(e)
		}
	}
	w.uniqueIDs(spTree)
	dropStaleTiming(root, spTree)

	rw := w.rels(name, rels)
	if layout := w.d.layouts[str(n, "layout")]; layout != "" {
		r := partrel.Rel{Type: relSlideLayout, Target: layout}
		rw.Drop(relSlideLayout, r)
		rw.Ensure(r)
	}
	if err := w.comments(n, w.d.slides[n.ID], rw); err != nil {
		return err
	}
	rw.Resolve(root, standardSpaces())
	if err := w.put(name, typeSlide, append([]byte(xmlHeader), root.Bytes()...)); err != nil {
		return err
	}
	return w.putRels(rw, isNew)
}

// changed tells whether a key differs between two sets of attributes.
func changed(a, b ot.Values, key string) bool {
	return !bytes.Equal(a[key], b[key])
}

func newSlide() *xmldom.Element {
	e, _ := xmldom.ParseFragment([]byte(`<p:sld xmlns:a="`+aNS+`" xmlns:r="`+relNS+`" xmlns:p="`+pNS+`">`+
		`<p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr>`+
		`<p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>`+
		`</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`), nil)
	return e
}

// declare makes the root of a part declare the prefixes XML carried from
// elsewhere may use.
func declare(root *xmldom.Element) {
	have := root.Spaces()
	for _, prefix := range []string{"a", "r", "p"} {
		if _, ok := have[prefix]; !ok {
			root.Set("xmlns:"+prefix, standardSpaces()[prefix])
		}
	}
}

func setBackground(cSld *xmldom.Element, value json.RawMessage, embed func(string) string) {
	if old := cSld.Child(pNS, "bg"); old != nil {
		cSld.Remove(old)
	}
	var bg Background
	if value == nil || json.Unmarshal(value, &bg) != nil {
		return
	}
	e := xmldom.New(pNS, "p:bg")
	switch {
	case bg.Fill != nil:
		fill := bg.Fill.Element(embed)
		if fill == nil {
			return
		}
		pr := xmldom.New(pNS, "p:bgPr")
		pr.Append(fill)
		pr.Append(xmldom.New(aNS, "a:effectLst"))
		e.Append(pr)
	case bg.Ref > 0:
		ref := xmldom.New(pNS, "p:bgRef", "idx", strconv.FormatInt(min(bg.Ref, 1003), 10))
		color := bg.Color
		if color == nil {
			color = &drawingml.Color{Scheme: "bg1"}
		}
		ref.Append(color.Element())
		e.Append(ref)
	default:
		return
	}
	cSld.Content = append([]xmldom.Node{e}, cSld.Content...)
}

// shape is the element of a shape node, nil when it cannot be written.
func (w *writer) shape(n *ot.Node) *xmldom.Element {
	e := w.element(n)
	if e != nil && w.d.loaded.Node(n.ID) == nil {
		if c := cNvPr(e); c != nil {
			w.fresh[c] = true
		}
	}
	return e
}

func (w *writer) element(n *ot.Node) *xmldom.Element {
	base := w.d.fragment(str(n, "xml"))
	switch n.Type {
	case "alt", "other":
		return base
	case "frame":
		if base == nil && str(n, "frame") == "table" {
			base = newTableFrame()
		}
		if base != nil {
			if x := base.Child(pNS, "xfrm"); x != nil && changed(ot.Values{"xfrm": mustJSON(drawingml.ReadXfrm(x))}, n.Attrs, "xfrm") {
				var xfrm drawingml.Xfrm
				if json.Unmarshal(n.Attrs["xfrm"], &xfrm) == nil {
					drawingml.SetXfrm(x, &xfrm)
				}
			}
			w.setNames(base, nvAttrs(base), n.Attrs)
			w.table(base, n)
		}
		return base
	case "sp", "pic", "cxn", "grp":
	default:
		return nil
	}
	if base == nil || base.Space != pNS {
		base = skeleton(n.Type)
		setPlaceholder(base, n.Attrs["ph"])
		setStyle(base, n.Attrs["style"])
	}
	old := nvAttrs(base)
	w.setNames(base, old, n.Attrs)
	switch n.Type {
	case "grp":
		pr := base.Child(pNS, "grpSpPr")
		drawingml.SetShapeProps(pr, drawingml.ShapeProps(pr, w.d.media), n.Attrs, w.embed)
		for _, c := range w.tree.Children(n.ID) {
			if e := w.shape(c); e != nil {
				base.Append(e)
			}
		}
		return base
	case "pic":
		w.setBlip(base, n)
	}
	pr := base.Child(pNS, "spPr")
	drawingml.SetShapeProps(pr, drawingml.ShapeProps(pr, w.d.media), n.Attrs, w.embed)
	if n.Type == "sp" && n.Text != nil {
		w.setText(base, n)
	}
	return base
}

func mustJSON(x any) json.RawMessage {
	data, _ := json.Marshal(x)
	if string(data) == "null" {
		return nil
	}
	return data
}

// embed is the name of a picture, known to the document.
func (w *writer) embed(media string) string {
	if r, ok := w.d.names.Lookup("@" + media); ok && r.Type == partrel.Image {
		return "@" + media
	}
	return ""
}

// setNames patches the name, description and visibility of a shape.
func (w *writer) setNames(e *xmldom.Element, old, values ot.Values) {
	c := cNvPr(e)
	if c == nil {
		return
	}
	for _, key := range []string{"name", "descr"} {
		if !changed(old, values, key) {
			continue
		}
		var s string
		_ = json.Unmarshal(values[key], &s)
		if s != "" || key == "name" {
			c.Set(key, s)
		} else {
			c.Unset(key)
		}
	}
	if changed(old, values, "hidden") {
		if string(values["hidden"]) == "true" {
			c.Set("hidden", "1")
		} else {
			c.Unset("hidden")
		}
	}
}

func (w *writer) setBlip(e *xmldom.Element, n *ot.Node) {
	fill := e.Child(pNS, "blipFill")
	if fill == nil {
		return
	}
	var b drawingml.Blip
	if json.Unmarshal(n.Attrs["blip"], &b) != nil || !changed(ot.Values{"blip": mustJSON(drawingml.ReadBlip(fill, w.d.media))}, n.Attrs, "blip") {
		return
	}
	name := w.embed(b.Media)
	if name == "" {
		return
	}
	blip := fill.Child(aNS, "blip")
	if blip == nil {
		blip = xmldom.New(aNS, "a:blip")
		fill.Content = append([]xmldom.Node{blip}, fill.Content...)
	}
	blip.Set("r:embed", name)
}

// setText writes the text of a shape and the properties of its body.
func (w *writer) setText(e *xmldom.Element, n *ot.Node) {
	body := e.Child(pNS, "txBody")
	flow := n.Text.Delta()
	if body == nil {
		if len(flow) == 1 && flow[0].Insert == "\n" && flow[0].Attrs == nil && n.Attrs["body"] == nil {
			return
		}
		body = xmldom.New(pNS, "p:txBody")
		body.Append(xmldom.New(aNS, "a:bodyPr"))
		body.Append(xmldom.New(aNS, "a:lstStyle"))
		e.Append(body)
	}
	if bodyPr := body.Child(aNS, "bodyPr"); bodyPr != nil {
		old := drawingml.BodyProps(bodyPr)
		var props drawingml.Props
		_ = json.Unmarshal(n.Attrs["body"], &props)
		drawingml.SetBodyProps(bodyPr, old, props)
	}
	drawingml.SetFlow(body, flow, w.d.fragment)
}

// setPlaceholder makes a new shape the placeholder ph describes, as a
// slide made from a layout has them.
func setPlaceholder(e *xmldom.Element, value json.RawMessage) {
	var ph map[string]string
	if value == nil || json.Unmarshal(value, &ph) != nil {
		return
	}
	var nvPr *xmldom.Element
	for _, c := range e.Elements() {
		if strings.HasPrefix(c.Local, "nv") {
			nvPr = c.Child(pNS, "nvPr")
			if locks := c.Child(pNS, "cNvSpPr"); locks != nil && e.Local == "sp" {
				locks.Append(xmldom.New(aNS, "a:spLocks", "noGrp", "1"))
			}
		}
	}
	if nvPr == nil {
		return
	}
	el := xmldom.New(pNS, "p:ph")
	for _, a := range []struct {
		name   string
		values []string
	}{
		{"type", []string{"title", "body", "ctrTitle", "subTitle", "dt", "sldNum", "ftr", "hdr", "obj", "chart", "tbl", "clipArt", "dgm", "media", "sldImg", "pic"}},
		{"orient", []string{"horz", "vert"}},
		{"sz", []string{"full", "half", "quarter"}},
	} {
		if slices.Contains(a.values, ph[a.name]) {
			el.Set(a.name, ph[a.name])
		}
	}
	if validNumber(ph["idx"]) {
		el.Set("idx", ph["idx"])
	}
	nvPr.Content = append([]xmldom.Node{el}, nvPr.Content...)
}

// setStyle gives a new shape the references to the theme style describes.
func setStyle(e *xmldom.Element, value json.RawMessage) {
	var st Style
	if value == nil || json.Unmarshal(value, &st) != nil || e.Local == "grp" || e.Local == "pic" {
		return
	}
	style := xmldom.New(pNS, "p:style")
	for _, r := range []struct {
		local string
		ref   *StyleRef
	}{{"lnRef", st.Line}, {"fillRef", st.Fill}, {"effectRef", st.Effect}, {"fontRef", st.Font}} {
		idx := "0"
		if r.local == "fontRef" {
			idx = "minor"
		}
		if r.ref != nil {
			if r.local == "fontRef" && slices.Contains([]string{"major", "minor", "none"}, r.ref.Idx) || r.local != "fontRef" && validIndex(r.ref.Idx) {
				idx = r.ref.Idx
			}
		}
		ref := xmldom.New(aNS, "a:"+r.local, "idx", idx)
		if r.ref != nil && r.ref.Color != nil {
			ref.Append(r.ref.Color.Element())
		}
		style.Append(ref)
	}
	var at int
	for i, c := range e.Content {
		if c, ok := c.(*xmldom.Element); ok && c.Local == "spPr" {
			at = i + 1
		}
	}
	e.Content = append(e.Content[:at:at], append([]xmldom.Node{style}, e.Content[at:]...)...)
}

func validIndex(s string) bool {
	n, err := strconv.ParseUint(s, 10, 32)
	return err == nil && n <= 1003
}

// skeleton is a new shape of that type, before its attributes.
func skeleton(typ string) *xmldom.Element {
	var s string
	switch typ {
	case "sp":
		s = `<p:sp><p:nvSpPr><p:cNvPr id="0" name=""/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr/></p:sp>`
	case "pic":
		s = `<p:pic><p:nvPicPr><p:cNvPr id="0" name=""/><p:cNvPicPr><a:picLocks noChangeAspect="1"/></p:cNvPicPr><p:nvPr/></p:nvPicPr>` +
			`<p:blipFill><a:blip/><a:stretch><a:fillRect/></a:stretch></p:blipFill><p:spPr/></p:pic>`
	case "cxn":
		s = `<p:cxnSp><p:nvCxnSpPr><p:cNvPr id="0" name=""/><p:cNvCxnSpPr/><p:nvPr/></p:nvCxnSpPr><p:spPr/></p:cxnSp>`
	case "grp":
		s = `<p:grpSp><p:nvGrpSpPr><p:cNvPr id="0" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr/></p:grpSp>`
	}
	e, _ := xmldom.ParseFragment([]byte(s), standardSpaces())
	return e
}

// uniqueIDs gives the shapes created since the document was read ids of
// their own: a copy comes with the id of its original. Those read keep
// theirs, even shared.
func (w *writer) uniqueIDs(spTree *xmldom.Element) {
	var all []*xmldom.Element
	walkElements(spTree, func(e *xmldom.Element) bool {
		if e.Space == pNS && e.Local == "cNvPr" {
			all = append(all, e)
		}
		// the branches of alternate content are the same shape
		return e.Space != mcNS || e.Local != "Fallback"
	})
	taken := map[uint64]bool{}
	next := uint64(1)
	for _, c := range all {
		if n, err := strconv.ParseUint(c.Get("id"), 10, 32); err == nil {
			next = max(next, n+1)
			if !w.fresh[c] {
				taken[n] = true
			}
		}
	}
	for _, c := range all {
		if !w.fresh[c] {
			continue
		}
		n, err := strconv.ParseUint(c.Get("id"), 10, 32)
		if err != nil || n == 0 || taken[n] {
			n = next
			next++
			c.Set("id", strconv.FormatUint(n, 10))
		}
		taken[n] = true
	}
}

// dropStaleTiming removes the animations of a slide when one of them names
// a shape that is no longer there, which Office would repair.
func dropStaleTiming(root, spTree *xmldom.Element) {
	timing := root.Child(pNS, "timing")
	if timing == nil {
		return
	}
	ids := map[string]bool{}
	walkElements(spTree, func(e *xmldom.Element) bool {
		if e.Space == pNS && e.Local == "cNvPr" {
			ids[e.Get("id")] = true
		}
		return true
	})
	stale := false
	walkElements(timing, func(e *xmldom.Element) bool {
		if v, ok := e.Attr("spid"); ok && !ids[v] {
			stale = true
		}
		return !stale
	})
	if stale {
		root.Remove(timing)
	}
}

func (w *writer) notes(id, name string) error {
	n := w.tree.Node(id)
	if n == nil || n.Text == nil {
		return nil
	}
	data, err := w.pkg.Read(name)
	if err != nil {
		return err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	rels, err := partrel.Read(w.pkg, name)
	if err != nil {
		return err
	}
	spaces := standardSpaces()
	maps.Copy(spaces, doc.Root.Spaces())
	w.d.names.NameAll(doc.Root, name, rels, spaces)
	body := notesBody(doc.Root)
	if body == nil {
		return nil
	}
	drawingml.SetFlow(body, n.Text.Delta(), w.d.fragment)
	rw := w.rels(name, rels)
	rw.Resolve(doc.Root, spaces)
	if err := w.pkg.Set(name, doc.Bytes()); err != nil {
		return err
	}
	return w.putRels(rw, false)
}

// presentation lists the slides, in their order, in the presentation and
// its sections, and forgets those deleted.
func (w *writer) presentation(slides []slide) error {
	data, err := w.pkg.Read(w.d.presName)
	if err != nil {
		return err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	pres := doc.Root
	spaces := standardSpaces()
	maps.Copy(spaces, pres.Spaces())
	rw := w.rels(w.d.presName, w.d.presRels)
	kept := map[string]bool{}
	for _, s := range slides {
		kept[s.part] = true
	}
	var list []opc.Relationship
	gone := map[string]bool{}
	for _, x := range rw.List {
		if x.Type == relSlide {
			if name, err := opc.Resolve(w.d.presName, x.Target); err == nil && !kept[name] {
				gone[x.ID] = true
				continue
			}
		}
		list = append(list, x)
	}
	rw.List, rw.Changed = list, len(gone) > 0

	ids := pres.Child(pNS, "sldIdLst")
	if ids == nil {
		ids = xmldom.New(pNS, "p:sldIdLst")
		pres.Insert(ids, []string{"sldMasterIdLst", "notesMasterIdLst", "handoutMasterIdLst", "sldIdLst"})
	}
	ids.Content = nil
	for _, s := range slides {
		rid := rw.Ensure(partrel.Rel{Type: relSlide, Target: s.part})
		ids.Append(xmldom.New(pNS, "p:sldId", "id", strconv.FormatInt(s.id, 10), "r:id", rid))
	}
	if len(slides) == 0 {
		pres.Remove(ids)
	}
	if w.newAuthors != "" {
		rw.Ensure(partrel.Rel{Type: relCommentAuthors, Target: w.newAuthors})
	}
	forgetCustomShows(pres, gone)
	w.sections(pres, slides)
	if err := w.pkg.Set(w.d.presName, doc.Bytes()); err != nil {
		return err
	}
	return w.putRels(rw, false)
}

// forgetCustomShows drops the deleted slides from the custom shows.
func forgetCustomShows(pres *xmldom.Element, gone map[string]bool) {
	shows := pres.Child(pNS, "custShowLst")
	if shows == nil {
		return
	}
	for _, show := range shows.Elements() {
		if list := show.Child(pNS, "sldLst"); list != nil {
			for _, s := range list.Elements() {
				if gone[s.Get("r:id")] {
					list.Remove(s)
				}
			}
		}
	}
}

// sections keep the slides they held, in their new order: a new slide
// joins the section of the one before it.
func (w *writer) sections(pres *xmldom.Element, slides []slide) {
	var list *xmldom.Element
	walkElements(pres, func(e *xmldom.Element) bool {
		if e.Local == "sectionLst" {
			list = e
		}
		return list == nil
	})
	if list == nil {
		return
	}
	var sections []*xmldom.Element
	of := map[int64]int{}
	for i, s := range list.Elements() {
		if s.Local != "section" {
			continue
		}
		sections = append(sections, s)
		for _, ids := range s.Elements() {
			for _, id := range ids.Elements() {
				if n, err := strconv.ParseInt(id.Get("id"), 10, 64); err == nil {
					of[n] = len(sections) - 1
				}
			}
		}
		_ = i
	}
	if len(sections) == 0 {
		return
	}
	lists := make([]*xmldom.Element, len(sections))
	for i, s := range sections {
		for _, c := range s.Elements() {
			if c.Local == "sldIdLst" {
				lists[i] = c
				c.Content = nil
			}
		}
		if lists[i] == nil {
			prefix, _, _ := strings.Cut(s.Name, ":")
			lists[i] = xmldom.New(s.Space, prefix+":sldIdLst")
			s.Append(lists[i])
		}
	}
	at := 0
	for _, s := range slides {
		if i, ok := of[s.id]; ok {
			at = i
		}
		prefix, _, _ := strings.Cut(lists[at].Name, ":")
		lists[at].Append(xmldom.New(lists[at].Space, prefix+":sldId", "id", strconv.FormatInt(s.id, 10)))
	}
}
