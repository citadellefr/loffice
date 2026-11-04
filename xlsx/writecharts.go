package xlsx

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	typeDrawing = "application/vnd.openxmlformats-officedocument.drawing+xml"
	relChart    = relBase + "chart"
)

// chartParts are the relationships of a chart to parts only it uses,
// which go with it.
var chartParts = map[string]bool{
	relBase + "chartUserShapes": true,
	relBase + "package":         true,
	"http://schemas.microsoft.com/office/2011/relationships/chartStyle":      true,
	"http://schemas.microsoft.com/office/2011/relationships/chartColorStyle": true,
}

// placedCharts are the charts of a sheet node.
func placedCharts(n *ot.Node) []Placed {
	var list []Placed
	if n != nil && n.Type == "sheet" {
		_ = json.Unmarshal(n.Attrs[chartsKey], &list)
	}
	return list
}

// charts writes the charts of the sheets as the tree has them: those
// removed go, those inserted come, and those kept are moved, resized and
// edited, their values those of the cells they read.
func (w *writer) charts(sheets []sheet) error {
	var now, then *chartCells
	for _, s := range sheets {
		if s.part.kept {
			continue
		}
		had, want := placedCharts(w.d.loaded.Node(s.node.ID)), placedCharts(s.node)
		if len(had) == 0 && len(want) == 0 {
			continue
		}
		if now == nil {
			now, then = newChartCells(w.tree), newChartCells(w.d.loaded)
		}
		if err := w.sheetCharts(s, had, want, now, then); err != nil {
			return err
		}
	}
	return nil
}

func (w *writer) sheetCharts(s sheet, had, want []Placed, now, then *chartCells) error {
	data, err := w.pkg.Read(s.part.name)
	if err != nil {
		return err
	}
	sheetDoc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	ref := sheetDoc.Root.Child(mainNS, "drawing")
	if ref == nil {
		return nil
	}
	own, err := w.pkg.Relationships(s.part.name)
	if err != nil {
		return err
	}
	name := relPart(own, s.part.name, relID(ref, sheetDoc.Root))
	if name == "" || !w.pkg.Has(name) {
		return nil
	}
	if data, err = w.pkg.Read(name); err != nil {
		return err
	}
	drawing, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	rels, err := w.pkg.Relationships(name)
	if err != nil {
		return err
	}
	rw := &sheetRels{list: rels}

	anchors := map[string]*xmldom.Element{}
	for _, a := range drawing.Root.Elements() {
		if c := find(a, chartNS, "chart"); a.Space == drawingNS && c != nil {
			anchors[relPart(rels, name, relID(c, drawing.Root))] = a
		}
	}
	kept := map[string]bool{}
	for _, p := range want {
		kept[p.ID] = true
	}
	changed := false
	for part, a := range anchors {
		if !kept[part] {
			drawing.Root.Remove(a)
			if err := w.dropChart(rw, name, part); err != nil {
				return err
			}
			changed = true
		}
	}
	before := map[string]*chart.Chart{}
	for _, p := range had {
		before[p.ID] = p.Chart
	}
	sheetName := str(s.node, "name")
	for _, p := range want {
		if p.Chart == nil {
			continue
		}
		refresh(p.Chart, before[p.ID], sheetName, now, then)
		if p.ID == "" {
			if err := w.newChart(drawing.Root, rw, name, p); err != nil {
				return err
			}
			changed = true
			continue
		}
		a := anchors[p.ID]
		if a == nil {
			continue
		}
		changed = place(a, p) || changed
		if err := w.patchChart(p.ID, p.Chart); err != nil {
			return err
		}
	}
	if changed {
		if err := w.pkg.Set(name, drawing.Bytes()); err != nil {
			return err
		}
	}
	return w.putPartRels(name, rw)
}

// relPart is the part a relationship of source points to, "" if none.
func relPart(rels []opc.Relationship, source, id string) string {
	for _, r := range rels {
		if r.ID == id && !r.External {
			if name, err := opc.Resolve(source, r.Target); err == nil {
				return name
			}
		}
	}
	return ""
}

func (w *writer) patchChart(name string, want *chart.Chart) error {
	data, err := w.pkg.Read(name)
	if err != nil {
		return nil
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil
	}
	if !chart.Patch(doc.Root, want) {
		return nil
	}
	return w.pkg.Set(name, doc.Bytes())
}

// dropChart removes a chart of a drawing, and the parts only it uses.
func (w *writer) dropChart(rw *sheetRels, drawing, name string) error {
	rels, err := w.pkg.Relationships(name)
	if err != nil {
		return err
	}
	for _, r := range rels {
		if chartParts[r.Type] {
			if part := relPart(rels, name, r.ID); part != "" && w.pkg.Has(part) {
				if err := w.pkg.Remove(part); err != nil {
					return err
				}
				if w.pkg.Has(opc.RelsName(part)) {
					_ = w.pkg.Remove(opc.RelsName(part))
				}
			}
		}
	}
	w.dropPart(rw, drawing, name, relChart)
	if rels := opc.RelsName(name); w.pkg.Has(rels) {
		return w.pkg.Remove(rels)
	}
	return nil
}

// place moves and sizes the anchor of a chart as p has it, and tells
// whether it changed.
func place(a *xmldom.Element, p Placed) bool {
	changed := false
	set := func(e *xmldom.Element, value int64) {
		if s := strconv.FormatInt(value, 10); e != nil && strings.TrimSpace(e.Text()) != s {
			e.Content = []xmldom.Node{xmldom.Raw(s)}
			changed = true
		}
	}
	attr := func(e *xmldom.Element, name string, value int64) {
		if s := strconv.FormatInt(value, 10); e != nil && e.Get(name) != s {
			e.Set(name, s)
			changed = true
		}
	}
	for local, c := range map[string]*Corner{"from": p.From, "to": p.To} {
		if e := a.Child(drawingNS, local); e != nil && c != nil {
			set(e.Child(drawingNS, "col"), int64(c.Col))
			set(e.Child(drawingNS, "colOff"), c.DX)
			set(e.Child(drawingNS, "row"), int64(c.Row))
			set(e.Child(drawingNS, "rowOff"), c.DY)
		}
	}
	if pos := a.Child(drawingNS, "pos"); pos != nil {
		attr(pos, "x", p.X)
		attr(pos, "y", p.Y)
	}
	if ext := a.Child(drawingNS, "ext"); ext != nil {
		attr(ext, "cx", p.W)
		attr(ext, "cy", p.H)
	}
	return changed
}

// newChart adds to a drawing the part of a chart inserted, and the
// anchor that places it.
func (w *writer) newChart(root *xmldom.Element, rw *sheetRels, drawing string, p Placed) error {
	name := w.freeName("xl/charts/chart%d.xml")
	if err := w.pkg.Add(name, typeChart, chart.New(p.Chart)); err != nil {
		return err
	}
	rid := rw.add(relChart, "/"+name)
	id := 0
	for _, a := range root.Elements() {
		if c := find(a, drawingNS, "cNvPr"); c != nil {
			id = max(id, atoi(c.Get("id")))
		}
	}
	prefixes := map[string]string{}
	for _, ns := range [][2]string{{drawingNS, "xdr"}, {aNS, "a"}, {chartNS, "c"}, {relNS, "r"}} {
		prefix, ok := prefixFor(root, ns[0])
		if !ok {
			prefix = ns[1]
			root.Set("xmlns:"+prefix, ns[0])
		}
		prefixes[ns[0]] = prefix + ":"
	}
	xdr, a, c, r := prefixes[drawingNS], prefixes[aNS], prefixes[chartNS], prefixes[relNS]
	corner := func(local string, k *Corner) string {
		return fmt.Sprintf(`<%[1]s%[2]s><%[1]scol>%[3]d</%[1]scol><%[1]scolOff>%[4]d</%[1]scolOff><%[1]srow>%[5]d</%[1]srow><%[1]srowOff>%[6]d</%[1]srowOff></%[1]s%[2]s>`,
			xdr, local, k.Col, k.DX, k.Row, k.DY)
	}
	frag := fmt.Sprintf(`<%[1]stwoCellAnchor>%[2]s%[3]s<%[1]sgraphicFrame macro=""><%[1]snvGraphicFramePr><%[1]scNvPr id="%[4]d" name="Graphique %[4]d"/><%[1]scNvGraphicFramePr/></%[1]snvGraphicFramePr>`+
		`<%[1]sxfrm><%[5]soff x="0" y="0"/><%[5]sext cx="0" cy="0"/></%[1]sxfrm><%[5]sgraphic><%[5]sgraphicData uri="%[6]s"><%[7]schart %[8]sid="%[9]s"/></%[5]sgraphicData></%[5]sgraphic></%[1]sgraphicFrame><%[1]sclientData/></%[1]stwoCellAnchor>`,
		xdr, corner("from", p.From), corner("to", p.To), id+1, a, chartNS, c, r, rid)
	e, err := xmldom.ParseFragment([]byte(frag), root.Spaces())
	if err != nil {
		return err
	}
	root.Append(e)
	return nil
}

// chartDrawing gives a sheet with charts and no drawing one to hold them.
func (w *writer) chartDrawing(root *xmldom.Element, n *ot.Node, p *sheetPart) error {
	if len(placedCharts(n)) == 0 || root.Child(mainNS, "drawing") != nil {
		return nil
	}
	rels, err := w.pkg.Relationships(p.name)
	if err != nil {
		return err
	}
	rw := &sheetRels{list: rels}
	name := w.freeName("xl/drawings/drawing%d.xml")
	if err := w.pkg.Add(name, typeDrawing, []byte(xmlHeader+`<xdr:wsDr xmlns:xdr="`+drawingNS+`" xmlns:a="`+aNS+`"/>`)); err != nil {
		return err
	}
	e := xmldom.New(mainNS, prefixOf(root)+"drawing")
	e.Set(relsPrefix(root)+":id", rw.add(relDrawing, "/"+name))
	root.Insert(e, worksheetOrder)
	return w.putRels(p, rw)
}

// relsPrefix is the prefix of relationships in a part, declared on its
// root when it has none.
func relsPrefix(root *xmldom.Element) string {
	if prefix, ok := prefixFor(root, relNS); ok {
		return prefix
	}
	root.Set("xmlns:r", relNS)
	return "r"
}
