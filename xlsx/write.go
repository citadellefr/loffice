package xlsx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const (
	xmlHeader = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\r\n"
	typeRels  = "application/vnd.openxmlformats-package.relationships+xml"
)

// workbookOrder is the order of the children of a workbook.
var workbookOrder = []string{"fileVersion", "fileSharing", "workbookPr", "workbookProtection", "bookViews", "sheets",
	"functionGroups", "externalReferences", "definedNames", "calcPr", "oleSize", "customWorkbookViews", "pivotCaches",
	"smartTagPr", "smartTagTypes", "webPublishing", "fileRecoveryPr", "webPublishObjects", "extLst"}

// Save writes the workbook as the tree has it. Only what changed is
// rewritten: a sheet, when its cells or settings changed; the shared
// strings and the styles, when a rewritten sheet adds to them; the
// workbook, when sheets came, went, moved or were renamed.
func (d *Document) Save(tree *ot.Tree) ([]byte, error) {
	return d.save(tree, false)
}

// save is Save, every sheet rewritten when force is set.
func (d *Document) save(tree *ot.Tree, force bool) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	pkg, err := opc.Open(d.original, Limits)
	if err != nil {
		return nil, err
	}
	w := &writer{d: d, pkg: pkg, tree: tree, force: force, rels: slices.Clone(d.bookRels), stringRefs: map[*sheetPart]int{}, moves: d.movesOf(tree)}
	if err := w.save(); err != nil {
		return nil, err
	}
	return pkg.Bytes()
}

type writer struct {
	d     *Document
	pkg   *opc.Package
	tree  *ot.Tree
	force bool
	// rels are the relationships of the workbook as written.
	rels        []opc.Relationship
	relsChanged bool
	styles      *styleWriter
	strings     *stringWriter
	// stringRefs counts the cells of each sheet rewritten that point to
	// shared strings.
	stringRefs map[*sheetPart]int
	// moves are the rows and columns inserted and removed since the
	// workbook was read, which every sheet is rewritten after.
	moves []move
}

// sheet is a sheet of the workbook as written.
type sheet struct {
	node *ot.Node
	part *sheetPart
}

func (w *writer) save() error {
	var sheets []sheet
	used := map[*sheetPart]bool{}
	nextID := 1
	for _, p := range w.d.sheets {
		nextID = max(nextID, p.sheetID+1)
	}
	var err error
	if w.styles, err = newStyleWriter(w); err != nil {
		return err
	}
	w.strings = &stringWriter{base: w.d.sst, index: map[string]int{}}
	rewritten := false
	for _, n := range w.tree.Children("book") {
		p := w.d.sheets[n.ID]
		switch {
		case n.Type == "kept" && p != nil && p.kept:
		case n.Type != "sheet":
			continue
		case p == nil || p.kept:
			p = w.newSheet(nextID)
			nextID++
			fallthrough
		case w.force || len(w.moves) > 0 || !w.same(n.ID):
			if err := w.sheet(n, p); err != nil {
				return err
			}
			rewritten = true
		}
		used[p] = true
		sheets = append(sheets, sheet{n, p})
	}
	if len(sheets) == 0 {
		return fmt.Errorf("xlsx: a workbook needs a sheet")
	}
	if len(w.moves) > 0 {
		if err := w.moveParts(sheets); err != nil {
			return err
		}
	}
	removed := false
	for _, p := range w.d.sheets {
		if !used[p] {
			w.remove(p)
			removed = true
		}
	}
	if w.strings.changed() {
		if err := w.sharedStrings(sheets); err != nil {
			return err
		}
	}
	if err := w.styles.write(); err != nil {
		return err
	}
	if rewritten || removed {
		w.dropCalcChain()
	}
	if rewritten || removed || !w.sameBook() {
		if err := w.workbook(sheets, rewritten); err != nil {
			return err
		}
	}
	if w.relsChanged {
		return w.put(opc.RelsName(w.d.bookName), typeRels, opc.MarshalRelationships(w.rels))
	}
	return nil
}

// same tells whether a sheet is as it was read.
func (w *writer) same(id string) bool {
	a, b := w.d.loaded.Node(id), w.tree.Node(id)
	if a == nil || b == nil || a.Type != b.Type || !sameValues(a.Attrs, b.Attrs) {
		return false
	}
	if (a.Grid == nil) != (b.Grid == nil) {
		return false
	}
	return a.Grid == nil || slices.EqualFunc(a.Grid.Cells(), b.Grid.Cells(), func(x, y ot.Cell) bool {
		return x.Row == y.Row && x.Col == y.Col && bytes.Equal(x.Fields, y.Fields)
	})
}

func sameValues(a, b ot.Values) bool {
	return maps.EqualFunc(a, b, func(x, y json.RawMessage) bool { return bytes.Equal(x, y) })
}

// sameBook tells whether the workbook lists the same sheets, in the same
// order, under the same names, with the same names defined.
func (w *writer) sameBook() bool {
	if w.force {
		return false
	}
	a, b := w.d.loaded.Children("book"), w.tree.Children("book")
	if len(a) != len(b) || !sameValues(w.d.loaded.Node("book").Attrs, w.tree.Node("book").Attrs) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || str(a[i], "name") != str(b[i], "name") || str(a[i], "state") != str(b[i], "state") {
			return false
		}
	}
	return true
}

// str is a string attribute of a node, "" if it has none.
func str(n *ot.Node, key string) string {
	var s string
	_ = json.Unmarshal(n.Attrs[key], &s)
	return s
}

func (w *writer) newSheet(sheetID int) *sheetPart {
	name := w.freeName("xl/worksheets/sheet%d.xml")
	rid := w.addRel(relWorksheet, w.target(name))
	doc, _ := xmldom.Parse([]byte(xmlHeader + `<worksheet xmlns="` + mainNS + `" xmlns:r="` + relNS + `">` +
		`<dimension ref="A1"/><sheetViews><sheetView workbookViewId="0"/></sheetViews>` +
		`<sheetFormatPr defaultRowHeight="15"/><sheetData/>` +
		`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/></worksheet>`))
	return &sheetPart{name: name, sheetID: sheetID, rid: rid, doc: doc}
}

// target is how the workbook points to a part.
func (w *writer) target(name string) string {
	dir := w.d.bookName[:strings.LastIndexByte(w.d.bookName, '/')+1]
	if strings.HasPrefix(name, dir) {
		return name[len(dir):]
	}
	return "/" + name
}

func (w *writer) freeName(pattern string) string {
	for n := 1; ; n++ {
		if name := fmt.Sprintf(pattern, n); !w.pkg.Has(name) {
			return name
		}
	}
}

// addRel adds a relationship from the workbook, and returns its id.
func (w *writer) addRel(typ, target string) string {
	ids := map[string]bool{}
	for _, x := range w.rels {
		ids[x.ID] = true
	}
	id := ""
	for n := 1; id == "" || ids[id]; n++ {
		id = "rId" + strconv.Itoa(n)
	}
	w.rels = append(w.rels, opc.Relationship{ID: id, Type: typ, Target: target})
	w.relsChanged = true
	return id
}

func (w *writer) removeRel(id string) {
	w.rels = slices.DeleteFunc(w.rels, func(x opc.Relationship) bool {
		if x.ID == id {
			w.relsChanged = true
			return true
		}
		return false
	})
}

// remove takes out a sheet that no longer is.
func (w *writer) remove(p *sheetPart) {
	w.removeRel(p.rid)
	if p.name != "" && w.pkg.Has(p.name) {
		_ = w.pkg.Remove(p.name)
		if rels := opc.RelsName(p.name); w.pkg.Has(rels) {
			_ = w.pkg.Remove(rels)
		}
	}
}

// put writes a part, adding it when it is new.
func (w *writer) put(name, contentType string, data []byte) error {
	if w.pkg.Has(name) {
		return w.pkg.Set(name, data)
	}
	return w.pkg.Add(name, contentType, data)
}

// dropCalcChain removes the order Excel last calculated cells in, which
// cells rewritten may no longer follow: Excel makes it again.
func (w *writer) dropCalcChain() {
	for _, x := range w.rels {
		if x.Type != relCalcChain {
			continue
		}
		if name, err := opc.Resolve(w.d.bookName, x.Target); err == nil && w.pkg.Has(name) {
			_ = w.pkg.Remove(name)
		}
		w.removeRel(x.ID)
		return
	}
}

// prefixOf is the prefix of an element's name, colon included.
func prefixOf(e *xmldom.Element) string {
	if i := strings.IndexByte(e.Name, ':'); i >= 0 {
		return e.Name[:i+1]
	}
	return ""
}

// child is the child of e with that local name, added in the order given
// when it has none.
func child(e *xmldom.Element, local string, order []string) *xmldom.Element {
	if c := e.Child(mainNS, local); c != nil {
		return c
	}
	c := xmldom.New(mainNS, prefixOf(e)+local)
	e.Insert(c, order)
	return c
}

// setAttr sets an attribute, or removes it when value is "".
func setAttr(e *xmldom.Element, name, value string) {
	if value == "" {
		e.Unset(name)
	} else {
		e.Set(name, value)
	}
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func boolAttr(b bool) string {
	if b {
		return "1"
	}
	return ""
}
