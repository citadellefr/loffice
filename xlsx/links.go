package xlsx

import (
	"bytes"
	"encoding/json"
	"unicode"
	"unicode/utf8"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

// maxLinkArea is the most cells a link read over an area is given to, one
// cell at a time; a link over a larger one stays as the workbook has it.
const maxLinkArea = 1024

// maxLinkText is the length Excel allows to the place and the tip of a
// link.
const maxLinkText = 255

// Link is where a cell leads to: an address of the web, of mail or a name
// ("urn:"), a place of the workbook such as "Sheet2!A1", or both, and the
// tip shown over the cell.
type Link struct {
	URL string `json:"url,omitempty"`
	Loc string `json:"loc,omitempty"`
	Tip string `json:"tip,omitempty"`
}

// valid tells whether Excel takes the link and its address is one a
// document may hold.
func (l Link) valid() bool {
	return (l.URL != "" || l.Loc != "") && (l.URL == "" || partrel.LinkTarget(l.URL) != "") && text(l.Loc) && text(l.Tip)
}

// Valid tells whether the link is one a document may hold.
func (l Link) Valid() bool {
	return l.valid()
}

func text(s string) bool {
	if len(s) > 4*maxLinkText || utf8.RuneCountInString(s) > maxLinkText {
		return false
	}
	for _, r := range s {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func parseLink(raw json.RawMessage) (Link, bool) {
	var l Link
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return l, dec.Decode(&l) == nil && l.valid()
}

// links takes out of a worksheet the links of its cells, by cell, that a
// client may edit: those over few cells whose address is written as a
// document writes it. The others stay in the worksheet.
func (d *Document) links(root *xmldom.Element, name string) map[[2]int]Link {
	list := root.Child(mainNS, "hyperlinks")
	if list == nil {
		return nil
	}
	rels, err := partrel.Read(d.pkg, name)
	if err != nil {
		rels = partrel.New(nil)
	}
	found := map[[2]int]Link{}
	for _, h := range elements(list, "hyperlink") {
		a, ok := formula.ParseArea(h.Get("ref"))
		if !ok || a.Rows || a.Cols || (a.R2-a.R1+1)*(a.C2-a.C1+1) > maxLinkArea {
			continue
		}
		l := Link{Loc: h.Get("location"), Tip: h.Get("tooltip")}
		if rid := relID(h, root); rid != "" {
			l.URL = rels.Link(rid)
			if l.URL == "" || partrel.LinkTarget(l.URL) != l.URL {
				continue
			}
		}
		if !l.valid() {
			continue
		}
		for r := a.R1; r <= a.R2; r++ {
			for c := a.C1; c <= a.C2; c++ {
				found[[2]int{r, c}] = l
			}
		}
		list.Remove(h)
	}
	if len(list.Elements()) == 0 {
		root.Remove(list)
	}
	return found
}

// link is a link to write over a cell.
type link struct {
	ref string
	Link
}

func (c *sheetWriter) link(r, col int, f *cellFields) {
	if l, ok := parseLink(f.Link); ok {
		c.links = append(c.links, link{ref: formula.CellName(r, col), Link: l})
	}
}

// writeLinks adds the links of the cells to a worksheet, and to the
// relationships of the sheet the addresses they lead to; those no link
// leads to are dropped.
func (w *writer) writeLinks(root *xmldom.Element, name string, links []link) error {
	rels, _ := partrel.Read(w.pkg, name)
	rw := partrel.NewWriter(name, rels, nil)
	if len(links) > 0 {
		prefix, ok := prefixFor(root, relNS)
		if !ok {
			prefix = freePrefix(root, nil)
			root.Set("xmlns:"+prefix, relNS)
		}
		list := child(root, "hyperlinks", worksheetOrder)
		for _, l := range links {
			h := xmldom.New(mainNS, prefixOf(list)+"hyperlink", "ref", l.ref)
			if l.URL != "" {
				target := partrel.LinkTarget(l.URL)
				h.Set(prefix+":id", rw.Ensure(partrel.Rel{Type: partrel.Hyperlink, Target: target, External: true}))
			}
			setAttr(h, "location", l.Loc)
			setAttr(h, "tooltip", l.Tip)
			list.Append(h)
		}
	}
	used := map[string]bool{}
	partrel.Walk(root, root.Spaces(), func(_ *xmldom.Element, _, id string) { used[id] = true })
	rw.Prune(partrel.Hyperlink, func(id string) bool { return used[id] })
	if !rw.Changed {
		return nil
	}
	part := opc.RelsName(name)
	if len(rw.List) == 0 {
		if w.pkg.Has(part) {
			return w.pkg.Remove(part)
		}
		return nil
	}
	return w.put(part, typeRels, opc.MarshalRelationships(rw.List))
}
