package xlsx

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/ot"
)

// ErrTable refuses rows or columns inserted or removed across a table,
// whose columns and header the workbook would lose.
var ErrTable = errors.New("xlsx: rows or columns cannot be inserted or removed across a table")

const (
	relDrawing     = relBase + "drawing"
	relTable       = relBase + "table"
	relComments    = relBase + "comments"
	relVML         = relBase + "vmlDrawing"
	relPivotTable  = relBase + "pivotTable"
	typeChart      = "application/vnd.openxmlformats-officedocument.drawingml.chart+xml"
	typePivotCache = "application/vnd.openxmlformats-officedocument.spreadsheetml.pivotCacheDefinition+xml"
	drawingNS      = "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"
	chartNS        = "http://schemas.openxmlformats.org/drawingml/2006/chart"
	movesKey       = "moves"
)

// A move is rows or columns inserted into a sheet, or removed when n is
// negative.
type move struct {
	sheet string
	rows  bool
	at, n int
}

// Moved records the rows and columns an edit the hub applied inserted or
// removed, and returns how many the document recorded: the book's
// "moves", which tell Save how many to replay on what the tree leaves as
// XML (conditional formats, validations, links, drawings, tables).
func (d *Document) Moved(e ot.Edit) int {
	d.movesMu.Lock()
	defer d.movesMu.Unlock()
	for _, c := range e {
		if c.Op != ot.OpIns && c.Op != ot.OpRem {
			continue
		}
		n := c.N
		if c.Op == ot.OpRem {
			n = -n
		}
		d.moves = append(d.moves, move{c.ID, c.Dim == ot.DimRows, c.At, n})
	}
	return len(d.moves)
}

// movesOf are the moves the tree has seen.
func (d *Document) movesOf(tree *ot.Tree) []move {
	var n int
	if book := tree.Node("book"); book != nil {
		_ = json.Unmarshal(book.Attrs[movesKey], &n)
	}
	d.movesMu.Lock()
	defer d.movesMu.Unlock()
	return append([]move(nil), d.moves[:min(max(n, 0), len(d.moves))]...)
}

// tablesOf are the areas of the tables of a sheet, as its moves left
// them.
func (d *Document) tablesOf(sheet string) []formula.Area {
	d.movesMu.Lock()
	defer d.movesMu.Unlock()
	var out []formula.Area
	for _, a := range d.tables[sheet] {
		ok := true
		for _, m := range d.moves {
			if m.sheet != sheet || !ok {
				continue
			}
			ref, err := formula.Shift(a.String(), "", true, m.rows, m.at, m.n)
			if a, ok = formula.ParseArea(ref); !ok || err != nil {
				ok = false
			}
		}
		if ok {
			out = append(out, a)
		}
	}
	return out
}

// checkTables refuses a change that would cut through a table: columns
// inserted or removed inside it, rows removed from its header or all of
// its body.
func (d *Document) checkTables(c ot.Change) error {
	n := c.N
	for _, t := range d.tablesOf(c.ID) {
		if c.Dim == ot.DimRows {
			end := c.At + n - 1
			if c.Op == ot.OpRem && end >= t.R1 && c.At <= t.R2 && !(c.At > t.R1 && end < t.R2) {
				return ErrTable
			}
			continue
		}
		if c.Op == ot.OpIns && c.At > t.C1 && c.At <= t.C2 || c.Op == ot.OpRem && c.At+n-1 >= t.C1 && c.At <= t.C2 {
			return ErrTable
		}
	}
	return nil
}

// mover replays moves on what a workbook keeps as XML.
type mover struct {
	moves []move
	// names are the names of the sheets moved, by node id.
	names map[string]string
}

func (w *writer) mover() *mover {
	m := &mover{moves: w.moves, names: map[string]string{}}
	for _, mv := range w.moves {
		if n := w.tree.Node(mv.sheet); n != nil {
			m.names[mv.sheet] = str(n, "name")
		}
	}
	return m
}

// formula is a formula of sheet own once the rows and columns moved.
func (m *mover) formula(f, own string) string {
	for _, mv := range m.moves {
		name, ok := m.names[mv.sheet]
		if !ok {
			continue
		}
		if g, err := formula.Shift(f, name, own == mv.sheet, mv.rows, mv.at, mv.n); err == nil {
			f = g
		}
	}
	return f
}

// sqref is a list of areas of sheet own once the rows and columns moved,
// those removed left out.
func (m *mover) sqref(s, own string) string {
	var out []string
	for _, a := range strings.Fields(s) {
		if g := m.formula(a, own); !strings.Contains(g, "#REF!") {
			out = append(out, g)
		}
	}
	return strings.Join(out, " ")
}

// index is a row or column counted from 0, as drawings count them, once
// the rows or columns of sheet own moved: into the first left when its own
// is removed.
func (m *mover) index(i int, own string, rows bool) int {
	for _, mv := range m.moves {
		if mv.sheet == own && mv.rows == rows {
			i = shiftIndex(i, mv.at, mv.n)
		}
	}
	return i
}

var (
	areaAttrs    = map[string]bool{"sqref": true, "ref": true}
	cellAttrs    = map[string]bool{"activeCell": true, "topLeftCell": true}
	formulaTexts = map[string]bool{"formula": true, "formula1": true, "formula2": true, "f": true}
	// lists go when they are left empty
	lists = map[string]bool{"dataValidations": true, "hyperlinks": true, "protectedRanges": true, "ignoredErrors": true,
		"conditionalFormattings": true}
)

// sheet moves what a worksheet keeps outside its cells: the areas of
// conditional formats, validations, links and filters, and the formulas
// they hold. It tells whether e lost the area it cannot be without.
func (m *mover) sheet(e *xmldom.Element, own string) bool {
	for _, c := range e.Elements() {
		if c.Local == "sheetData" || c.Local == "mergeCells" || c.Local == "cols" || c.Local == "dimension" {
			continue
		}
		gone := false
		for _, a := range c.Attrs {
			if !areaAttrs[a.Name] && !cellAttrs[a.Name] {
				continue
			}
			g := m.sqref(a.Value, own)
			switch {
			case g == "" && cellAttrs[a.Name]:
				c.Unset(a.Name)
			case g == "":
				gone = true
			case g != a.Value:
				c.Set(a.Name, g)
			}
		}
		switch {
		case gone:
		case formulaTexts[c.Local] && len(c.Elements()) == 0:
			if t := c.Text(); t != "" {
				if g := m.formula(t, own); g != t {
					c.Content = []xmldom.Node{xmldom.EscapeText(g)}
				}
			}
		case c.Local == "sqref" && len(c.Elements()) == 0:
			if t := c.Text(); t != "" {
				g := m.sqref(t, own)
				if g == "" {
					return true
				}
				if g != t {
					c.Content = []xmldom.Node{xmldom.EscapeText(g)}
				}
			}
		default:
			gone = m.sheet(c, own)
		}
		if gone {
			e.Remove(c)
			continue
		}
		if lists[c.Local] {
			if n := len(c.Elements()); n == 0 {
				e.Remove(c)
			} else if _, ok := c.Attr("count"); ok {
				c.Set("count", strconv.Itoa(n))
			}
		}
	}
	return false
}

var vmlAnchor = regexp.MustCompile(`<x:(Row|Column)>(\d+)</x:(?:Row|Column)>`)

// moveParts moves what the parts around the sheets written hold: the anchors
// of drawings, the areas of tables, comments and pivot tables, and the
// formulas of charts and pivot caches.
func (w *writer) moveParts(sheets []sheet) error {
	m := w.mover()
	edit := func(name string, change func(root *xmldom.Element) bool) error {
		data, err := w.pkg.Read(name)
		if err != nil {
			return nil
		}
		doc, err := xmldom.Parse(data)
		if err != nil {
			return nil
		}
		if !change(doc.Root) {
			return nil
		}
		return w.pkg.Set(name, doc.Bytes())
	}
	for _, s := range sheets {
		if s.part.kept {
			continue
		}
		rels, err := w.pkg.Relationships(s.part.name)
		if err != nil {
			continue
		}
		own := s.node.ID
		for _, r := range rels {
			if r.External {
				continue
			}
			name, err := opc.Resolve(s.part.name, r.Target)
			if err != nil || !w.pkg.Has(name) {
				continue
			}
			switch r.Type {
			case relDrawing:
				err = edit(name, func(root *xmldom.Element) bool { return m.anchors(root, own) })
			case relTable:
				err = edit(name, func(root *xmldom.Element) bool { return m.areas(root, own) })
			case relComments, relPivotTable:
				err = edit(name, func(root *xmldom.Element) bool { return m.areas(root, own) })
			case relVML:
				data, _ := w.pkg.Read(name)
				moved := vmlAnchor.ReplaceAllStringFunc(string(data), func(s string) string {
					g := vmlAnchor.FindStringSubmatch(s)
					i, _ := strconv.Atoi(g[2])
					return "<x:" + g[1] + ">" + strconv.Itoa(m.index(i, own, g[1] == "Row")) + "</x:" + g[1] + ">"
				})
				if moved != string(data) {
					err = w.pkg.Set(name, []byte(moved))
				}
			}
			if err != nil {
				return err
			}
		}
	}
	// charts and pivot caches may read any sheet
	for _, name := range w.pkg.Names() {
		switch w.pkg.ContentType(name) {
		case typeChart:
			if err := edit(name, func(root *xmldom.Element) bool { return m.formulas(root) }); err != nil {
				return err
			}
		case typePivotCache:
			if err := edit(name, func(root *xmldom.Element) bool { return m.cacheSource(root) }); err != nil {
				return err
			}
		}
	}
	return nil
}

// anchors moves the cells the shapes of a drawing are anchored to.
func (m *mover) anchors(root *xmldom.Element, own string) bool {
	changed := false
	for _, a := range root.Elements() {
		for _, corner := range a.Elements() {
			if corner.Space != drawingNS || corner.Local != "from" && corner.Local != "to" {
				continue
			}
			for _, c := range corner.Elements() {
				if c.Local != "row" && c.Local != "col" {
					continue
				}
				i, err := strconv.Atoi(strings.TrimSpace(c.Text()))
				if err != nil {
					continue
				}
				if j := m.index(i, own, c.Local == "row"); j != i {
					c.Content = []xmldom.Node{xmldom.Raw(strconv.Itoa(j))}
					changed = true
				}
			}
		}
	}
	return changed
}

// areas moves the areas of a part about a sheet: a table and its filter,
// comments, the location of a pivot table.
func (m *mover) areas(e *xmldom.Element, own string) bool {
	changed := false
	for _, name := range []string{"ref", "sqref"} {
		if v, ok := e.Attr(name); ok {
			if g := m.sqref(v, own); g != "" && g != v {
				e.Set(name, g)
				changed = true
			}
		}
	}
	for _, c := range e.Elements() {
		if m.areas(c, own) {
			changed = true
		}
	}
	return changed
}

// formulas moves the references of a chart to the sheets.
func (m *mover) formulas(e *xmldom.Element) bool {
	changed := false
	for _, c := range e.Elements() {
		if c.Space == chartNS && c.Local == "f" && len(c.Elements()) == 0 {
			if t := c.Text(); t != "" {
				if g := m.formula(t, ""); g != t {
					c.Content = []xmldom.Node{xmldom.EscapeText(g)}
					changed = true
				}
			}
			continue
		}
		if m.formulas(c) {
			changed = true
		}
	}
	return changed
}

// cacheSource moves the area a pivot cache reads.
func (m *mover) cacheSource(root *xmldom.Element) bool {
	src := root.Child(mainNS, "cacheSource")
	if src == nil {
		return false
	}
	ws := src.Child(mainNS, "worksheetSource")
	if ws == nil || ws.Get("ref") == "" || ws.Get("sheet") == "" {
		return false
	}
	own := ""
	for id, name := range m.names {
		if strings.EqualFold(name, ws.Get("sheet")) {
			own = id
		}
	}
	if own == "" {
		return false
	}
	g := m.sqref(ws.Get("ref"), own)
	if g == "" || g == ws.Get("ref") {
		return false
	}
	ws.Set("ref", g)
	return true
}
