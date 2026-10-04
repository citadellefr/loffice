package xlsx

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

const typeStyles = "application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"

var (
	styleSheetOrder = []string{"numFmts", "fonts", "fills", "borders", "cellStyleXfs", "cellXfs", "cellStyles", "dxfs",
		"tableStyles", "colors", "extLst"}
	fontOrder = []string{"b", "i", "strike", "condense", "extend", "outline", "shadow", "u", "vertAlign", "sz", "color",
		"name", "family", "charset", "scheme"}
	xfOrder = []string{"alignment", "protection", "extLst"}
)

// styleWriter gives each xf node its index among the cell formats of the
// workbook, adding those made in the editor.
type styleWriter struct {
	d   *Document
	w   *writer
	ids map[string]int
	// what follows is set once a format is added
	doc                                  *xmldom.Document
	root                                 *xmldom.Element
	fonts, fills, borders, cellXfs, fmts *xmldom.Element
	formats                              map[string]int
}

func newStyleWriter(w *writer) (*styleWriter, error) {
	s := &styleWriter{d: w.d, w: w, ids: map[string]int{}}
	var created []*ot.Node
	for _, n := range w.tree.Children("") {
		if n.Type != "xf" {
			continue
		}
		if w.d.loaded.Node(n.ID) != nil {
			if i, err := strconv.Atoi(strings.TrimPrefix(n.ID, "x")); err == nil {
				s.ids[n.ID] = i
				continue
			}
		}
		created = append(created, n)
	}
	if len(created) == 0 {
		return s, nil
	}
	if err := s.open(); err != nil {
		return nil, err
	}
	for _, n := range created {
		s.ids[n.ID] = s.add(n)
	}
	return s, nil
}

// index is the index of the cell format an xf node stands for.
func (s *styleWriter) index(id string) (int, bool) {
	i, ok := s.ids[id]
	return i, ok
}

// open reads the styles again, to change them.
func (s *styleWriter) open() error {
	data := []byte(xmlHeader + `<styleSheet xmlns="` + mainNS + `">` +
		`<fonts count="1"><font><sz val="11"/><name val="Calibri"/><family val="2"/><scheme val="minor"/></font></fonts>` +
		`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs>` +
		`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`)
	if name := s.d.styles.name; name != "" {
		var err error
		if data, err = s.w.pkg.Read(name); err != nil {
			return err
		}
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	s.doc, s.root = doc, doc.Root
	s.fonts = child(s.root, "fonts", styleSheetOrder)
	s.fills = child(s.root, "fills", styleSheetOrder)
	s.borders = child(s.root, "borders", styleSheetOrder)
	s.cellXfs = child(s.root, "cellXfs", styleSheetOrder)
	// what Excel expects first in each list, which a workbook may lack
	defaults := []struct {
		list  *xmldom.Element
		local string
		xml   []string
	}{
		{s.fonts, "font", []string{`<font><sz val="11"/><name val="Calibri"/></font>`}},
		{s.fills, "fill", []string{`<fill><patternFill patternType="none"/></fill>`, `<fill><patternFill patternType="gray125"/></fill>`}},
		{s.borders, "border", []string{`<border><left/><right/><top/><bottom/><diagonal/></border>`}},
		{s.cellXfs, "xf", []string{`<xf numFmtId="0" fontId="0" fillId="0" borderId="0"/>`}},
	}
	for _, d := range defaults {
		if len(elements(d.list, d.local)) > 0 {
			continue
		}
		p := prefixOf(d.list)
		for _, x := range d.xml {
			x = strings.ReplaceAll(strings.ReplaceAll(x, "</", "\x00"), "<", "<"+p)
			x = strings.ReplaceAll(x, "\x00", "</"+p)
			e, err := xmldom.ParseFragment([]byte(x), map[string]string{strings.TrimSuffix(p, ":"): mainNS})
			if err != nil {
				return err
			}
			appendTo(d.list, e)
		}
	}
	s.formats = map[string]int{}
	for id, code := range builtinFormats {
		s.formats[code] = id
	}
	for id, code := range s.d.styles.formats {
		s.formats[code] = id
	}
	return nil
}

// add appends the cell format of an xf node, and returns its index: the
// format it derives from, with what the node changes.
func (s *styleWriter) add(n *ot.Node) int {
	var st Style
	_ = json.Unmarshal(n.Attrs["style"], &st)
	xfs := elements(s.cellXfs, "xf")
	b, ok := s.ids[st.Base]
	if !ok || b >= len(xfs) {
		b = 0
	}
	base := s.d.styles.style(b)
	var xf *xmldom.Element
	if b < len(xfs) {
		xf = xfs[b].Clone()
	} else {
		xf = xmldom.New(mainNS, prefixOf(s.cellXfs)+"xf", "numFmtId", "0", "fontId", "0", "fillId", "0", "borderId", "0", "xfId", "0")
	}
	if atoi(xf.Get("xfId")) >= len(elements(s.root.Child(mainNS, "cellStyleXfs"), "xf")) {
		xf.Unset("xfId")
	}
	if st.Format != base.Format {
		xf.Set("numFmtId", strconv.Itoa(s.format(st.Format)))
		xf.Set("applyNumberFormat", "1")
	}
	if !reflect.DeepEqual(st.Font, base.Font) {
		fonts := elements(s.fonts, "font")
		var font *xmldom.Element
		if i := atoi(xf.Get("fontId")); i < len(fonts) {
			font = fonts[i].Clone()
		} else {
			font = xmldom.New(mainNS, prefixOf(s.fonts)+"font")
		}
		patchFont(font, st.Font)
		xf.Set("fontId", strconv.Itoa(appendTo(s.fonts, font)))
		xf.Set("applyFont", "1")
	}
	if !reflect.DeepEqual(st.Fill, base.Fill) {
		xf.Set("fillId", strconv.Itoa(appendTo(s.fills, fillElement(s.fills, st.Fill))))
		xf.Set("applyFill", "1")
	}
	if !reflect.DeepEqual(st.Border, base.Border) {
		xf.Set("borderId", strconv.Itoa(appendTo(s.borders, borderElement(s.borders, st.Border))))
		xf.Set("applyBorder", "1")
	}
	if !reflect.DeepEqual(st.Align, base.Align) {
		replaceChild(xf, "alignment", alignElement(xf, st.Align))
		xf.Set("applyAlignment", "1")
	}
	if !reflect.DeepEqual(st.Prot, base.Prot) {
		replaceChild(xf, "protection", protectionElement(xf, st.Prot))
		xf.Set("applyProtection", "1")
	}
	return appendTo(s.cellXfs, xf)
}

// format is the id of a number format, added when the workbook has none
// with that code.
func (s *styleWriter) format(code string) int {
	if code == "" {
		return 0
	}
	if id, ok := s.formats[code]; ok {
		return id
	}
	id := 164
	for _, i := range s.formats {
		id = max(id, i+1)
	}
	if s.fmts == nil {
		s.fmts = child(s.root, "numFmts", styleSheetOrder)
	}
	appendTo(s.fmts, xmldom.New(mainNS, prefixOf(s.fmts)+"numFmt", "numFmtId", strconv.Itoa(id), "formatCode", code))
	s.formats[code] = id
	return id
}

// appendTo adds an element to a list of the styles, and returns its index.
func appendTo(list, e *xmldom.Element) int {
	n := len(elements(list, e.Local))
	list.Append(e)
	list.Set("count", strconv.Itoa(n+1))
	return n
}

func replaceChild(e *xmldom.Element, local string, c *xmldom.Element) {
	if old := e.Child(mainNS, local); old != nil {
		e.Remove(old)
	}
	if c != nil {
		e.Insert(c, xfOrder)
	}
}

// patchFont makes a font what the model says, keeping what it does not
// tell: its family, its character set.
func patchFont(e *xmldom.Element, f *Font) {
	if f == nil {
		f = &Font{}
	}
	set := func(local string, on bool, attrs ...string) {
		if old := e.Child(mainNS, local); old != nil {
			e.Remove(old)
		}
		if on {
			e.Insert(xmldom.New(mainNS, prefixOf(e)+local, attrs...), fontOrder)
		}
	}
	set("b", f.Bold)
	set("i", f.Italic)
	set("strike", f.Strike)
	if f.Underline == "single" {
		set("u", true)
	} else {
		set("u", f.Underline != "", "val", f.Underline)
	}
	set("vertAlign", f.VertAlign != "", "val", f.VertAlign)
	set("sz", f.Size > 0, "val", formatFloat(f.Size))
	if old := e.Child(mainNS, "color"); old != nil {
		e.Remove(old)
	}
	if f.Color != nil {
		e.Insert(colorElement(e, "color", f.Color), fontOrder)
	}
	set("name", f.Name != "", "val", f.Name)
	set("scheme", f.Scheme != "", "val", f.Scheme)
	inOrder(e, fontOrder)
}

// inOrder sorts the children of e in the order of a schema, those it
// misses last.
func inOrder(e *xmldom.Element, order []string) {
	rank := func(n xmldom.Node) int {
		if c, ok := n.(*xmldom.Element); ok {
			if i := slices.Index(order, c.Local); i >= 0 {
				return i
			}
		}
		return len(order)
	}
	kids := slices.DeleteFunc(slices.Clone(e.Content), func(n xmldom.Node) bool {
		_, ok := n.(xmldom.Raw)
		return ok
	})
	slices.SortStableFunc(kids, func(a, b xmldom.Node) int { return rank(a) - rank(b) })
	if len(kids) == 0 {
		kids = nil
	}
	e.Content = kids
}

func fillElement(fills *xmldom.Element, f *Fill) *xmldom.Element {
	p := prefixOf(fills)
	e := xmldom.New(mainNS, p+"fill")
	switch {
	case f != nil && len(f.Gradient) > 0:
		g := xmldom.New(mainNS, p+"gradientFill", "degree", "90")
		for i, c := range f.Gradient {
			pos := "0"
			if len(f.Gradient) > 1 {
				pos = formatFloat(float64(i) / float64(len(f.Gradient)-1))
			}
			stop := xmldom.New(mainNS, p+"stop", "position", pos)
			if c == nil {
				c = &Color{Auto: true}
			}
			stop.Append(colorElement(stop, "color", c))
			g.Append(stop)
		}
		e.Append(g)
	case f == nil || f.Pattern == "":
		e.Append(xmldom.New(mainNS, p+"patternFill", "patternType", "none"))
	default:
		pf := xmldom.New(mainNS, p+"patternFill", "patternType", f.Pattern)
		if f.Fg != nil {
			pf.Append(colorElement(pf, "fgColor", f.Fg))
		}
		if f.Bg != nil {
			pf.Append(colorElement(pf, "bgColor", f.Bg))
		}
		e.Append(pf)
	}
	return e
}

func borderElement(borders *xmldom.Element, b *Border) *xmldom.Element {
	p := prefixOf(borders)
	e := xmldom.New(mainNS, p+"border")
	if b == nil {
		b = &Border{}
	}
	setAttr(e, "diagonalUp", boolAttr(b.Up))
	setAttr(e, "diagonalDown", boolAttr(b.Down))
	for _, side := range []struct {
		local string
		edge  *Edge
	}{{"left", b.Left}, {"right", b.Right}, {"top", b.Top}, {"bottom", b.Bottom}, {"diagonal", b.Diagonal}} {
		s := xmldom.New(mainNS, p+side.local)
		if side.edge != nil && side.edge.Style != "" && side.edge.Style != "none" {
			s.Set("style", side.edge.Style)
			if side.edge.Color != nil {
				s.Append(colorElement(s, "color", side.edge.Color))
			}
		}
		e.Append(s)
	}
	return e
}

func alignElement(xf *xmldom.Element, a *Align) *xmldom.Element {
	if a == nil || *a == (Align{}) {
		return nil
	}
	e := xmldom.New(mainNS, prefixOf(xf)+"alignment")
	setAttr(e, "horizontal", a.H)
	setAttr(e, "vertical", a.V)
	setAttr(e, "wrapText", boolAttr(a.Wrap))
	if a.Indent > 0 && a.Indent <= 250 {
		e.Set("indent", strconv.Itoa(a.Indent))
	}
	if a.Rotate > 0 && a.Rotate <= 180 || a.Rotate == 255 {
		e.Set("textRotation", strconv.Itoa(a.Rotate))
	}
	setAttr(e, "shrinkToFit", boolAttr(a.Shrink))
	return e
}

func protectionElement(xf *xmldom.Element, p *Protection) *xmldom.Element {
	if p == nil || *p == (Protection{}) {
		return nil
	}
	e := xmldom.New(mainNS, prefixOf(xf)+"protection")
	if p.Unlocked {
		e.Set("locked", "0")
	}
	setAttr(e, "hidden", boolAttr(p.Hidden))
	return e
}

// write writes the styles, when formats were added.
func (s *styleWriter) write() error {
	if s.root == nil {
		return nil
	}
	name := s.d.styles.name
	if name == "" {
		name = "xl/styles.xml"
		if s.w.pkg.Has(name) {
			name = s.w.freeName("xl/styles%d.xml")
		}
		s.w.addRel(relStyles, s.w.target(name))
	}
	return s.w.put(name, typeStyles, s.doc.Bytes())
}
