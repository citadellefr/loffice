package xlsx

import (
	"encoding/json"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

const typeTable = "application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"

// tableOrder is the order of the children of a table.
var tableOrder = []string{"autoFilter", "sortState", "tableColumns", "tableStyleInfo", "extLst"}

// tableColumnOrder is the order of the children of a table column.
var tableColumnOrder = []string{"calculatedColumnFormula", "totalsRowFormula", "xmlColumnPr", "extLst"}

// tables writes the tables of a sheet from its node: the parts of those
// read are rewritten, new ones are made, those no longer there go, and
// the worksheet lists them all in its tableParts.
func (w *writer) tables(root *xmldom.Element, n *ot.Node, p *sheetPart) error {
	var tables []Table
	_ = json.Unmarshal(n.Attrs[tablesKey], &tables)
	read := map[int]tablePart{}
	if w.d.sheets[n.ID] == p {
		for id, t := range w.d.tableParts {
			if t.sheet == n.ID {
				read[id] = t
			}
		}
	}
	if len(tables) == 0 && len(read) == 0 {
		return nil
	}
	rels, _ := w.pkg.Relationships(p.name)
	var rids []string
	done := map[int]bool{}
	for _, t := range tables {
		if part, ok := read[t.ID]; ok && !done[t.ID] {
			done[t.ID] = true
			if err := w.rewriteTable(part.name, t); err != nil {
				return err
			}
			w.tableNames[part.name] = true
			rids = append(rids, part.rid)
			continue
		}
		name := w.freeName("xl/tables/table%d.xml")
		if err := w.newTable(name, t); err != nil {
			return err
		}
		w.tableNames[name] = true
		rid := freeRel(rels)
		rels = append(rels, opc.Relationship{ID: rid, Type: relTable, Target: relative(p.name, name)})
		rids = append(rids, rid)
	}
	for id, part := range read {
		if !done[id] {
			w.dropTable(part.name)
			rels = slices.DeleteFunc(rels, func(x opc.Relationship) bool { return x.ID == part.rid })
		}
	}
	for _, x := range rels {
		if x.Type == relTable && !slices.Contains(rids, x.ID) {
			rids = append(rids, x.ID)
		}
	}
	if err := w.putRels(p, &sheetRels{list: rels, changed: true}); err != nil {
		return err
	}
	tableParts(root, rids)
	return nil
}

// tableParts gives a worksheet the list of its tables.
func tableParts(root *xmldom.Element, rids []string) {
	if e := root.Child(mainNS, "tableParts"); e != nil {
		root.Remove(e)
	}
	if len(rids) == 0 {
		return
	}
	rel, ok := prefixFor(root, relNS)
	if !ok {
		rel = freePrefix(root, nil)
		root.Set("xmlns:"+rel, relNS)
	}
	prefix := prefixOf(root)
	list := xmldom.New(mainNS, prefix+"tableParts", "count", strconv.Itoa(len(rids)))
	for _, id := range rids {
		list.Append(xmldom.New(mainNS, prefix+"tablePart", rel+":id", id))
	}
	root.Insert(list, worksheetOrder)
}

// dropTables removes the tables of a sheet that no longer is.
func (w *writer) dropTables(sheet string) {
	for _, part := range w.d.tableParts {
		if part.sheet == sheet {
			w.dropTable(part.name)
		}
	}
}

func (w *writer) dropTable(name string) {
	for _, n := range []string{name, opc.RelsName(name)} {
		if w.pkg.Has(n) {
			_ = w.pkg.Remove(n)
		}
	}
}

// rewriteTable writes a table read into its part, which keeps what the
// tree does not hold.
func (w *writer) rewriteTable(name string, t Table) error {
	data, err := w.pkg.Read(name)
	if err != nil {
		return err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	writeTable(doc.Root, t, t.ID)
	return w.pkg.Set(name, doc.Bytes())
}

// newTable makes the part of a table the workbook did not have.
func (w *writer) newTable(name string, t Table) error {
	shown := ""
	if !t.Totals {
		shown = ` totalsRowShown="0"`
	}
	doc, err := xmldom.Parse([]byte(xmlHeader + `<table xmlns="` + mainNS + `"` + shown + `/>`))
	if err != nil {
		return err
	}
	writeTable(doc.Root, t, w.tableID(t.ID))
	return w.pkg.Add(name, typeTable, doc.Bytes())
}

// tableID is the id of a new table: the one it asks for if no table of the
// workbook has it, else the one after the greatest.
func (w *writer) tableID(want int) int {
	if w.tableIDs == nil {
		w.scanTables()
	}
	if want <= 0 || w.tableIDs[want] {
		w.lastTableID++
		want = w.lastTableID
	}
	w.tableIDs[want] = true
	return want
}

// scanTables finds the ids tables have: those read, those of the tables
// the tree does not hold, and those the tree gives.
func (w *writer) scanTables() {
	w.tableIDs = map[int]bool{}
	read := map[string]bool{}
	for id, part := range w.d.tableParts {
		w.tableIDs[id] = true
		read[part.name] = true
	}
	for _, name := range w.pkg.Names() {
		if w.pkg.ContentType(name) != typeTable || read[name] {
			continue
		}
		if data, err := w.pkg.Read(name); err == nil {
			if doc, err := xmldom.Parse(data); err == nil {
				w.tableIDs[atoi(doc.Root.Get("id"))] = true
			}
		}
	}
	for id := range w.tableIDs {
		w.lastTableID = max(w.lastTableID, id)
	}
	for _, n := range w.tree.Children("book") {
		var tables []Table
		_ = json.Unmarshal(n.Attrs[tablesKey], &tables)
		for _, t := range tables {
			w.lastTableID = max(w.lastTableID, t.ID)
		}
	}
}

// writeTable gives a table element the table: what the tree holds is set,
// the rest is left as it is.
func writeTable(root *xmldom.Element, t Table, id int) {
	prefix := prefixOf(root)
	root.Set("id", strconv.Itoa(id))
	root.Set("name", encodeEscapes(t.Name))
	root.Set("displayName", encodeEscapes(t.Name))
	root.Set("ref", t.area().String())
	if t.NoHeader {
		root.Set("headerRowCount", "0")
	} else {
		root.Unset("headerRowCount")
	}
	setAttr(root, "totalsRowCount", boolAttr(t.Totals))

	before := columnIDs(root)
	ids := writeColumns(root, t.Columns)
	filter := root.Child(mainNS, "autoFilter")
	switch {
	case !t.Filter:
		if filter != nil {
			root.Remove(filter)
		}
	default:
		if filter == nil {
			filter = xmldom.New(mainNS, prefix+"autoFilter")
			root.Insert(filter, tableOrder)
		}
		a := t.area()
		if t.Totals {
			a.R2--
		}
		filter.Set("ref", a.String())
		followColumns(filter, before, ids)
	}

	style := child(root, "tableStyleInfo", tableOrder)
	setAttr(style, "name", t.Style)
	for _, f := range []struct {
		name string
		on   bool
	}{{"showFirstColumn", t.First}, {"showLastColumn", t.Last}, {"showRowStripes", t.Rows}, {"showColumnStripes", t.Cols}} {
		style.Set(f.name, map[bool]string{true: "1", false: "0"}[f.on])
	}
}

// columnIDs are the ids of the columns of a table element, in order.
func columnIDs(root *xmldom.Element) []int {
	var ids []int
	for _, c := range elements(root.Child(mainNS, "tableColumns"), "tableColumn") {
		ids = append(ids, atoi(c.Get("id")))
	}
	return ids
}

// writeColumns gives a table element its columns, and returns their ids.
// A column is the one of the element that has its id, which keeps what the
// tree does not hold; the others are new, with an id no column has.
func writeColumns(root *xmldom.Element, cols []TableColumn) []int {
	list := child(root, "tableColumns", tableOrder)
	prefix := prefixOf(list)
	old := map[int]*xmldom.Element{}
	last := 0
	for _, e := range elements(list, "tableColumn") {
		id := atoi(e.Get("id"))
		old[id] = e
		last = max(last, id)
	}
	for _, c := range cols {
		last = max(last, c.ID)
	}
	ids := make([]int, len(cols))
	used := map[int]bool{}
	list.Content = nil
	for i, c := range cols {
		id := c.ID
		if id <= 0 || used[id] {
			last++
			id = last
		}
		used[id] = true
		ids[i] = id
		e := old[id]
		if e == nil {
			e = xmldom.New(mainNS, prefix+"tableColumn")
		}
		e.Set("id", strconv.Itoa(id))
		e.Set("name", encodeEscapes(c.Name))
		setAttr(e, "totalsRowFunction", c.Total)
		setAttr(e, "totalsRowLabel", encodeEscapes(c.Label))
		columnFormula(e, "calculatedColumnFormula", c.F)
		columnFormula(e, "totalsRowFormula", c.TotalF)
		list.Append(e)
	}
	list.Set("count", strconv.Itoa(len(cols)))
	return ids
}

// columnFormula gives a table column the formula it has in the tree, in
// the form files store it unless it is the one the column already holds.
func columnFormula(e *xmldom.Element, local, f string) {
	cur := e.Child(mainNS, local)
	switch {
	case f == "":
		if cur != nil {
			e.Remove(cur)
		}
		return
	case cur == nil:
		cur = xmldom.New(mainNS, prefixOf(e)+local)
		e.Insert(cur, tableColumnOrder)
	case cur.Text() == f:
		return
	}
	cur.Content = []xmldom.Node{xmldom.EscapeText(formula.Stored(f))}
}

// followColumns points the filters of a table to the columns they filter,
// counted again: before are the ids of the columns the filters counted,
// after those of the columns now. A filter of a column that went goes too.
func followColumns(filter *xmldom.Element, before, after []int) {
	for _, c := range elements(filter, "filterColumn") {
		col := atoi(c.Get("colId"))
		at := -1
		if col >= 0 && col < len(before) {
			at = slices.Index(after, before[col])
		}
		if at < 0 {
			filter.Remove(c)
			continue
		}
		c.Set("colId", strconv.Itoa(at))
	}
}

// freeRel is a relationship id no relationship has.
func freeRel(rels []opc.Relationship) string {
	id := ""
	for n := 1; id == "" || slices.ContainsFunc(rels, func(x opc.Relationship) bool { return x.ID == id }); n++ {
		id = "rId" + strconv.Itoa(n)
	}
	return id
}

// relative is how the part from points to the part to.
func relative(from, to string) string {
	var dir []string
	if d := path.Dir(from); d != "." {
		dir = strings.Split(d, "/")
	}
	parts := strings.Split(to, "/")
	i := 0
	for i < len(dir) && i < len(parts)-1 && dir[i] == parts[i] {
		i++
	}
	return strings.Repeat("../", len(dir)-i) + strings.Join(parts[i:], "/")
}
