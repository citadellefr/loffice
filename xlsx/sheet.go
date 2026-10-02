package xlsx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/internal/xmltok"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/ot"
)

// Tail are the fields of the columns from From on, up to To.
type Tail struct {
	From int `json:"from"`
	To   int `json:"to"`
	lineFields
}

// frozen are the rows and columns a sheet keeps in view.
type frozen struct {
	Rows int `json:"r,omitempty"`
	Cols int `json:"c,omitempty"`
}

func (r *reader) sheet(s *xmldom.Element, key string) (string, error) {
	d := r.d
	sheetID := atoi(s.Get("sheetId"))
	id := "S" + strconv.Itoa(sheetID)
	if sheetID <= 0 || d.sheets[id] != nil {
		return "", fmt.Errorf("xlsx: sheet %q has no valid id", s.Get("name"))
	}
	rid := relID(s, d.book.Root)
	var rel opc.Relationship
	for _, x := range d.bookRels {
		if x.ID == rid && !x.External {
			rel = x
		}
	}
	name, err := opc.Resolve(d.bookName, rel.Target)
	part := &sheetPart{name: name, sheetID: sheetID, rid: rid}
	d.sheets[id] = part

	attrs := ot.Values{"name": mustJSON(sheetName(s.Get("name"), len(d.sheets)-1, r.names))}
	if state := s.Get("state"); state == "hidden" || state == "veryHidden" {
		attrs["state"] = mustJSON(state)
	}
	if rel.ID == "" || err != nil || rel.Type != relWorksheet || !d.pkg.Has(name) {
		part.kept = true
		if rel.Type == relChartsheet && err == nil {
			if c := d.sheetChart(name); c != nil {
				attrs["chart"] = c.JSON()
			}
		}
		r.add(ot.Change{Op: ot.OpNew, ID: id, Type: "kept", Parent: "book", Key: key, Attrs: attrs})
		return id, nil
	}

	data, err := d.pkg.Read(name)
	if err != nil {
		return "", err
	}
	r.tables(id, name)
	cells, rest, err := r.sheetData(data, part)
	if err != nil {
		return "", fmt.Errorf("xlsx: %s: %w", name, err)
	}
	doc, err := xmldom.Parse(rest)
	if err != nil {
		return "", fmt.Errorf("xlsx: %s: %w", name, err)
	}
	part.doc = doc
	root := doc.Root

	if view := root.Child(mainNS, "sheetViews"); view != nil {
		if v := view.Child(mainNS, "sheetView"); v != nil {
			if g, ok := v.Attr("showGridLines"); ok && !truthy(g) {
				attrs["grid"] = mustJSON(false)
			}
			if truthy(v.Get("rightToLeft")) {
				attrs["rtl"] = mustJSON(true)
			}
			if z := atoi(v.Get("zoomScale")); z > 0 && z != 100 {
				attrs["zoom"] = mustJSON(z)
			}
			if p := v.Child(mainNS, "pane"); p != nil && strings.HasPrefix(p.Get("state"), "frozen") {
				f := frozen{Rows: int(parseFloat(p.Get("ySplit"))), Cols: int(parseFloat(p.Get("xSplit")))}
				if f != (frozen{}) {
					attrs["frozen"] = mustJSON(f)
				}
			}
		}
	}
	if pr := root.Child(mainNS, "sheetPr"); pr != nil {
		if c := pr.Child(mainNS, "tabColor"); c != nil {
			attrs["tab"] = mustJSON(d.styles.color(c))
		}
	}
	if l := validationLists(root); len(l) > 0 {
		attrs[listsKey] = mustJSON(l)
	}
	if c := d.conditionals(root); len(c) > 0 {
		attrs[conditionalKey] = mustJSON(c)
	}
	if f := autoFilter(root); f != nil {
		attrs[filterKey] = mustJSON(f)
	}
	if c := d.sheetCharts(name, root); len(c) > 0 {
		attrs[chartsKey] = mustJSON(c)
	}
	if f := root.Child(mainNS, "sheetFormatPr"); f != nil {
		if w := parseFloat(f.Get("defaultColWidth")); w > 0 {
			attrs["dw"] = mustJSON(w)
		} else if w := parseFloat(f.Get("baseColWidth")); w > 0 {
			attrs["dw"] = mustJSON(w + 5.0/7)
		}
		if h := parseFloat(f.Get("defaultRowHeight")); h > 0 {
			attrs["dh"] = mustJSON(h)
		}
	}

	columns := map[int]json.RawMessage{}
	for _, c := range elements(root.Child(mainNS, "cols"), "col") {
		lo, hi := atoi(c.Get("min")), atoi(c.Get("max"))
		if lo < 1 || hi < lo {
			continue
		}
		f := lineFields{
			BF: truthy(c.Get("bestFit")), CL: truthy(c.Get("collapsed")), CW: truthy(c.Get("customWidth")),
			W: min(max(parseFloat(c.Get("width")), 0), maxWidth), Hide: truthy(c.Get("hidden")), OL: outlineLevel(c.Get("outlineLevel")),
			S: d.styleID(atoi(c.Get("style"))),
		}
		if f == (lineFields{}) {
			continue
		}
		hi = min(hi, formula.MaxCols)
		if hi > maxExpanded {
			attrs["tail"] = mustJSON(Tail{From: max(lo, maxExpanded+1), To: hi, lineFields: f})
			hi = maxExpanded
		}
		raw := mustJSON(f)
		for col := lo; col <= hi; col++ {
			columns[col] = raw
		}
	}
	head := make([]ot.Cell, 0, len(columns)+len(cells))
	for _, col := range slices.Sorted(maps.Keys(columns)) {
		head = append(head, ot.Cell{Row: 0, Col: col, Fields: columns[col]})
	}
	cells = append(head, cells...)

	merges := map[[2]int][2]int{}
	for _, m := range elements(root.Child(mainNS, "mergeCells"), "mergeCell") {
		a, ok := formula.ParseArea(m.Get("ref"))
		if ok && !a.Cols && !a.Rows && (a.R1 != a.R2 || a.C1 != a.C2) {
			merges[[2]int{a.R1, a.C1}] = [2]int{a.R2 - a.R1 + 1, a.C2 - a.C1 + 1}
		}
	}
	if len(merges) > 0 {
		cells = mergeInto(cells, merges)
	}
	if cells == nil {
		cells = []ot.Cell{}
	}
	r.add(ot.Change{Op: ot.OpNew, ID: id, Type: "sheet", Parent: "book", Key: key, Attrs: attrs, Cells: cells})
	return id, nil
}

// tables notes the areas of the tables of a sheet.
func (r *reader) tables(id, name string) {
	rels, err := r.d.pkg.Relationships(name)
	if err != nil {
		return
	}
	for _, x := range rels {
		if x.Type != relTable || x.External {
			continue
		}
		part, err := opc.Resolve(name, x.Target)
		if err != nil {
			continue
		}
		doc, err := r.part(part)
		if err != nil {
			continue
		}
		if a, ok := formula.ParseArea(doc.Root.Get("ref")); ok {
			r.d.tables[id] = append(r.d.tables[id], a)
		}
	}
}

// relID is the relationship a sheet of the workbook points to, whatever
// the prefix of its namespace.
func relID(e, root *xmldom.Element) string {
	for _, a := range e.Attrs {
		prefix, local, ok := strings.Cut(a.Name, ":")
		if !ok || local != "id" {
			continue
		}
		space, declared := e.Spaces()[prefix]
		if !declared {
			space = root.Spaces()[prefix]
		}
		if space == relNS {
			return a.Value
		}
	}
	return ""
}

// mergeInto gives the first cell of each merged area its span.
func mergeInto(cells []ot.Cell, merges map[[2]int][2]int) []ot.Cell {
	for i, c := range cells {
		span, ok := merges[[2]int{c.Row, c.Col}]
		if !ok {
			continue
		}
		var f cellFields
		_ = json.Unmarshal(c.Fields, &f)
		f.M = &span
		cells[i].Fields = f.marshal()
		delete(merges, [2]int{c.Row, c.Col})
	}
	for at, span := range merges {
		cells = append(cells, ot.Cell{Row: at[0], Col: at[1], Fields: (&cellFields{M: &span}).marshal()})
	}
	slices.SortFunc(cells, func(a, b ot.Cell) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	return cells
}

// styleID is the id of the xf node of a cell format, "" for the default
// one and those the workbook does not have.
func (d *Document) styleID(i int) string {
	if i <= 0 || i >= len(d.styles.xfs) {
		return ""
	}
	return xfID(i)
}

// The limits of Excel: rows up to 409.5 points high, columns 255
// characters wide, grouped 7 levels deep.
const (
	maxHeight  = 409.5
	maxWidth   = 255
	maxOutline = 7
)

func outlineLevel(s string) int {
	return min(max(atoi(s), 0), maxOutline)
}

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// shared is the formula of a group of cells, as its first cell holds it.
type shared struct {
	f        string
	row, col int
}

// sheetData reads the cells of a worksheet, and returns the worksheet with
// its sheetData left empty.
func (r *reader) sheetData(data []byte, part *sheetPart) ([]ot.Cell, []byte, error) {
	s := xmltok.New(data)
	var start xmltok.Token
	for {
		tok, ok := s.Next()
		if !ok {
			if s.Err() != nil {
				return nil, nil, s.Err()
			}
			return nil, data, nil
		}
		if tok.Kind == xmltok.StartElement && s.Depth() == 2 && local(tok.Name) == "sheetData" {
			start = tok
			break
		}
	}
	c := &cellReader{d: r.d, part: part, shared: map[string]shared{}}
	c.cells = make([]ot.Cell, 0, bytes.Count(data, []byte("<c "))+bytes.Count(data, []byte("<row ")))
	var end int
	if start.SelfClosing {
		end = start.End
		s.Next()
	} else {
		var err error
		if end, err = c.rows(s); err != nil {
			return nil, nil, err
		}
	}
	c.resolveShared()
	if !c.sorted {
		c.sort()
	}
	var rest bytes.Buffer
	rest.Write(data[:start.Offset])
	rest.WriteByte('<')
	rest.Write(start.Name)
	rest.WriteString("/>")
	rest.Write(data[end:])
	return c.cells, rest.Bytes(), nil
}

type cellReader struct {
	d      *Document
	part   *sheetPart
	cells  []ot.Cell
	shared map[string]shared
	// followers are the cells of shared formulas read before their first.
	followers []follower
	sorted    bool
	lastRow   int
	lastCol   int
	// text and value are buffers for a cell's value, as read and as JSON.
	text, value []byte
}

type follower struct {
	cell int
	si   string
}

// rows reads the rows of sheetData, and returns where it ends.
func (c *cellReader) rows(s *xmltok.Scanner) (int, error) {
	c.sorted = true
	row := 0
	for {
		tok, ok := s.Next()
		if !ok {
			if s.Err() != nil {
				return 0, s.Err()
			}
			return 0, xmltok.ErrSyntax
		}
		switch {
		case tok.Kind == xmltok.EndElement && s.Depth() == 1:
			return tok.End, nil
		case tok.Kind != xmltok.StartElement:
		case local(tok.Name) == "row":
			var err error
			if row, err = c.row(tok, row); err != nil {
				return 0, err
			}
		case local(tok.Name) == "c":
			if err := c.cell(s, tok, row); err != nil {
				return 0, err
			}
		default:
			if _, err := s.SkipElement(tok); err != nil {
				return 0, err
			}
		}
	}
}

func (c *cellReader) row(tok xmltok.Token, last int) (int, error) {
	row := last + 1
	var f lineFields
	styled := false
	for name, value := range xmltok.Attrs(tok.Data) {
		v := string(value)
		switch string(name) {
		case "r":
			if n, err := strconv.Atoi(v); err == nil && n >= 1 && n <= formula.MaxRows {
				row = n
			}
		case "ht":
			f.H = min(max(parseFloat(v), 0), maxHeight)
		case "customHeight":
			f.CH = truthy(v)
		case "hidden":
			f.Hide = truthy(v)
		case "s":
			f.S = c.d.styleID(atoi(v))
		case "customFormat":
			styled = truthy(v)
		case "outlineLevel":
			f.OL = outlineLevel(v)
		case "collapsed":
			f.CL = truthy(v)
		}
	}
	if !styled {
		f.S = ""
	}
	c.lastCol = 0
	if f != (lineFields{}) {
		return row, c.put(row, 0, mustJSON(f))
	}
	return row, nil
}

func (c *cellReader) put(row, col int, fields json.RawMessage) error {
	c.d.cellData += len(fields)
	if c.d.cellData > MaxCellData {
		return fmt.Errorf("xlsx: cells: %w", opc.ErrTooLarge)
	}
	if row < c.lastRow || row == c.lastRow && col <= c.lastCol {
		c.sorted = false
	}
	c.lastRow, c.lastCol = row, col
	c.cells = append(c.cells, ot.Cell{Row: row, Col: col, Fields: fields})
	return nil
}

// cell reads a <c> element, which tok opens.
func (c *cellReader) cell(s *xmltok.Scanner, tok xmltok.Token, row int) error {
	col := c.lastCol + 1
	var f cellFields
	typ := "n"
	for name, value := range xmltok.Attrs(tok.Data) {
		v := string(value)
		switch string(name) {
		case "r":
			if r, cl, ok := formula.ParseCell(v); ok {
				row, col = r, cl
			}
		case "s":
			f.S = c.d.styleID(atoi(v))
		case "t":
			typ = v
		case "cm":
			f.CM = atoi(v)
		case "vm":
			f.VM = atoi(v)
		}
	}
	if row < 1 {
		row = max(c.lastRow, 1)
	}
	var value []byte
	var si string
	depth := s.Depth()
	for {
		t, ok := s.Next()
		if !ok {
			return s.Err()
		}
		if t.Kind == xmltok.EndElement && s.Depth() < depth {
			break
		}
		if t.Kind != xmltok.StartElement {
			continue
		}
		body, err := s.SkipElement(t)
		if err != nil {
			return err
		}
		inner := body[t.End-t.Offset:]
		if t.SelfClosing {
			inner = nil
		} else {
			inner = inner[:bytes.LastIndexByte(inner, '<')]
		}
		switch local(t.Name) {
		case "v":
			if c.text, err = xmltok.Unescape(c.text[:0], inner); err != nil {
				return err
			}
			if value = c.text; len(value) == 0 {
				value = nil
			}
		case "is":
			item, err := readItem(body)
			if err != nil {
				return err
			}
			value, typ = []byte(item.text), "str"
			if item.rich != "" {
				f.Rich = siOf(item.rich, local(t.Name) != string(t.Name))
				c.d.trust(f.Rich)
			}
		case "f":
			text, err := xmltok.Unescape(nil, inner)
			if err != nil {
				return err
			}
			formulaType, _ := t.Attr("t")
			ref, _ := t.Attr("ref")
			ca, _ := t.Attr("ca")
			f.CA = truthy(string(ca)) && (len(text) > 0 || string(formulaType) == "shared")
			switch string(formulaType) {
			case "shared":
				idx, _ := t.Attr("si")
				si = string(idx)
				if len(ref) > 0 || len(text) > 0 {
					c.shared[si] = shared{f: string(text), row: row, col: col}
					f.F = string(text)
				}
			case "array":
				f.F, f.FA = string(text), string(ref)
			case "dataTable":
				f.FX = string(body)
				c.d.trust(f.FX)
			default:
				f.F = string(text)
			}
		}
	}
	switch typ {
	case "s":
		c.part.strings++
		if i, err := strconv.Atoi(string(value)); err == nil && c.d.sst != nil && i >= 0 && i < len(c.d.sst.items) {
			item := c.d.sst.items[i]
			c.value = appendString(c.value[:0], item.text)
			f.V = c.value
			if item.rich != "" {
				f.Rich = item.rich
				c.d.trust(f.Rich)
			}
		}
	case "str", "inlineStr":
		if value != nil || typ == "inlineStr" {
			c.value = appendString(c.value[:0], decodeEscapes(string(value)))
			f.V = c.value
		}
	case "b":
		if len(value) > 0 {
			f.V = mustJSON(string(value) == "1" || string(value) == "true")
		}
	case "e":
		f.E = strings.TrimSpace(string(value))
	case "d":
		if t, err := time.Parse("2006-01-02T15:04:05", strings.TrimSuffix(string(value), "Z")); err == nil {
			f.V = number(serial(t, c.d.date1904))
		}
	default:
		if len(value) > 0 {
			if n, err := strconv.ParseFloat(strings.TrimSpace(string(value)), 64); err == nil && !math.IsInf(n, 0) {
				c.value = strconv.AppendFloat(c.value[:0], n, 'g', -1, 64)
				f.V = c.value
			}
		}
	}
	c.lastCol = col
	if f.F == "" && si != "" {
		c.followers = append(c.followers, follower{cell: len(c.cells), si: si})
	}
	fields := f.marshal()
	if string(fields) == "{}" && si == "" {
		return nil
	}
	return c.put(row, col, fields)
}

// siOf is a rich inline string <is> written as the <si> of the table,
// which declares the namespace of its runs, prefixed or not.
func siOf(is string, prefixed bool) string {
	start := strings.IndexByte(is, '>')
	end := strings.LastIndexByte(is, '<')
	open := `<si xmlns="` + mainNS + `"`
	if prefixed {
		prefix, _, _ := strings.Cut(is[1:], ":")
		open += ` xmlns:` + prefix + `="` + mainNS + `"`
	}
	if start < 0 || end <= start || strings.HasSuffix(is[:start], "/") {
		return open + "/>"
	}
	return open + ">" + is[start+1:end] + "</si>"
}

// resolveShared gives the cells of shared formulas their own formula, the
// first one's moved to them.
func (c *cellReader) resolveShared() {
	for _, fl := range c.followers {
		sh, ok := c.shared[fl.si]
		cell := &c.cells[fl.cell]
		if !ok {
			continue
		}
		f, err := formula.Translate(sh.f, cell.Row-sh.row, cell.Col-sh.col)
		if err != nil {
			continue
		}
		var fields cellFields
		_ = json.Unmarshal(cell.Fields, &fields)
		fields.F = f
		cell.Fields = fields.marshal()
	}
}

// sort orders the cells row by row, the last of two at the same place
// winning, as Excel reads them.
func (c *cellReader) sort() {
	slices.SortStableFunc(c.cells, func(a, b ot.Cell) int {
		if a.Row != b.Row {
			return a.Row - b.Row
		}
		return a.Col - b.Col
	})
	out := c.cells[:0]
	for i, cell := range c.cells {
		if i+1 < len(c.cells) && c.cells[i+1].Row == cell.Row && c.cells[i+1].Col == cell.Col {
			continue
		}
		out = append(out, cell)
	}
	c.cells = out
}

func number(n float64) json.RawMessage {
	return strconv.AppendFloat(nil, n, 'g', -1, 64)
}

// serial is the date as Excel counts days: from 1899-12-30, 1900 wrongly
// taken for a leap year, or from 1904-01-01.
func serial(t time.Time, date1904 bool) float64 {
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	days := t.Sub(epoch).Hours() / 24
	if !date1904 && days < 61 {
		days--
	}
	return days
}
