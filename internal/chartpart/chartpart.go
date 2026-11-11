// Package chartpart edits the charts of Word documents and presentations:
// a chart part, written as a patch, and the workbook embedded with it,
// which the data of the chart live in. A node holding a chart has it in
// its "chart" attribute and, when its data can be edited, their table
// (xlsx.ChartTable) in "chartData": a client changes the look of the chart
// in the first and the data in the second, and Follow gives the chart the
// references and values cached that go with them.
package chartpart

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/citadellefr/loffice/chart"
	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

const (
	ChartKey = "chart"
	DataKey  = "chartData"

	relPackage = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/package"
)

var (
	// ErrReadOnly is an edit of the data of a chart without a workbook
	// embedded that its series read.
	ErrReadOnly = errors.New("loffice: this chart has no embedded workbook its series read: its data are read-only")
	// ErrInvalid is an edit of a chart that does not fit it.
	ErrInvalid = errors.New("loffice: this chart cannot take this edit")
)

// Workbooks embedded are small: these bound what a file can make Open do.
const (
	maxBooks    = 256
	maxBookSize = 32 << 20
)

// Part is a chart part as the file has it.
type Part struct {
	name  string
	chart *chart.Chart
	read  json.RawMessage
	// book is the workbook embedded, nil when the data are read-only;
	// bookName its part and table its table, as JSON.
	book     *xlsx.ChartBook
	bookName string
	table    json.RawMessage
}

// Chart is the chart as the file has it.
func (p *Part) Chart() *chart.Chart { return p.chart }

// A Reader reads the chart parts of a package, each once.
type Reader struct {
	pkg    *opc.Package
	limits opc.Limits
	parts  map[string]*Part
	books  int
}

func NewReader(pkg *opc.Package, limits opc.Limits) *Reader {
	return &Reader{pkg: pkg, limits: limits, parts: map[string]*Part{}}
}

// Read reads a chart part and the workbook embedded with it, nil when it
// is no chart.
func (r *Reader) Read(name string) *Part {
	if p, ok := r.parts[name]; ok {
		return p
	}
	r.parts[name] = r.read(name)
	return r.parts[name]
}

func (r *Reader) read(name string) *Part {
	data, err := r.pkg.Read(name)
	if err != nil {
		return nil
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil
	}
	c, err := chart.ReadElement(doc.Root)
	if err != nil {
		return nil
	}
	p := &Part{name: name, chart: c, read: c.JSON()}
	if target := embedded(r.pkg, name, doc.Root); target != "" && r.books < maxBooks {
		r.books++
		r.open(p, target)
	}
	return p
}

// open gives p the workbook of a part, when its series read one.
func (r *Reader) open(p *Part, target string) {
	data, err := r.pkg.Read(target)
	if err != nil || len(data) > maxBookSize {
		return
	}
	book, err := xlsx.OpenChartBook(data, p.chart, xlsx.Limits{
		MaxParts:     r.limits.MaxParts,
		MaxPartSize:  bounded(r.limits.MaxPartSize),
		MaxTotalSize: bounded(r.limits.MaxTotalSize),
	})
	if err != nil {
		return
	}
	if table, err := json.Marshal(book.Table()); err == nil {
		p.book, p.bookName, p.table = book, target, table
	}
}

func bounded(limit int64) int64 {
	if limit > 0 {
		return min(limit, maxBookSize)
	}
	return maxBookSize
}

// embedded is the part of the workbook a chart embeds, "" when it links
// to one or has none.
func embedded(pkg *opc.Package, name string, space *xmldom.Element) string {
	ext := space.Child(chart.NS, "externalData")
	if ext == nil {
		return ""
	}
	var id string
	for _, a := range ext.Attrs {
		if prefix, local, ok := strings.Cut(a.Name, ":"); ok && local == "id" && space.Spaces()[prefix] == partrel.NS {
			id = a.Value
		}
	}
	rels, err := pkg.Relationships(name)
	if err != nil {
		return ""
	}
	for _, r := range rels {
		if r.ID == id && r.Type == relPackage && !r.External {
			if target, err := opc.Resolve(name, r.Target); err == nil && pkg.Has(target) {
				return target
			}
		}
	}
	return ""
}

// Attrs are the attributes of a node holding the chart, as read.
func (p *Part) Attrs(attrs ot.Values) {
	attrs[ChartKey] = p.read
	if p.book != nil {
		attrs[DataKey] = p.table
	}
}

// check tells whether a node with the attributes attrs can take those of
// change: the look of its chart, not its data, and the data of a workbook
// embedded.
func (p *Part) check(attrs, change ot.Values) error {
	if raw, ok := change[DataKey]; ok {
		if p.book == nil {
			return ErrReadOnly
		}
		var t xlsx.ChartTable
		if json.Unmarshal(raw, &t) != nil {
			return ErrInvalid
		}
		if err := p.book.Check(&t); err != nil {
			return err
		}
	}
	raw, ok := change[ChartKey]
	if !ok {
		return nil
	}
	var c, old chart.Chart
	if json.Unmarshal(raw, &c) != nil || json.Unmarshal(attrs[ChartKey], &old) != nil {
		return ErrInvalid
	}
	if !reflect.DeepEqual(seriesData(&c), seriesData(&old)) && p.book == nil {
		return ErrReadOnly
	}
	if !reflect.DeepEqual(seriesData(&c), seriesData(&old)) || !xlsx.CheckChart(&c, &old) {
		return ErrInvalid
	}
	return nil
}

// seriesData are the data of the series of a chart, as JSON, by index.
func seriesData(c *chart.Chart) map[int]string {
	out := map[int]string{}
	for _, p := range c.Plots {
		for _, s := range p.Series {
			data, _ := json.Marshal([]*chart.Data{s.Name, s.Cat, s.Val, s.Size})
			out[s.Index] = string(data)
		}
	}
	return out
}

// follow is what a node with the attributes attrs takes after an edit
// changed those of change: its chart placed, with the data of its table,
// and that table calculated.
func (p *Part) follow(attrs, change ot.Values) ot.Values {
	var c chart.Chart
	if json.Unmarshal(attrs[ChartKey], &c) != nil {
		return nil
	}
	before := c.JSON()
	out := ot.Values{}
	if raw, ok := change[DataKey]; ok {
		var t xlsx.ChartTable
		if p.book == nil || json.Unmarshal(raw, &t) != nil {
			return nil
		}
		table, err := p.book.Chart(&t, &c)
		if err != nil {
			return nil
		}
		if !reflect.DeepEqual(table, &t) {
			out[DataKey], _ = json.Marshal(table)
		}
	}
	chart.Normalize(&c)
	if after := c.JSON(); !bytes.Equal(before, after) {
		out[ChartKey] = after
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Check tells whether an edit that gives the node id the attributes of
// change is one of its chart that fits.
func Check(tree *ot.Tree, id string, change ot.Values, charts map[string]*Part) error {
	if !touches(change) {
		return nil
	}
	p, n := charts[id], tree.Node(id)
	if p == nil || n == nil {
		return ErrInvalid
	}
	return p.check(n.Attrs, change)
}

func touches(attrs ot.Values) bool {
	_, c := attrs[ChartKey]
	_, d := attrs[DataKey]
	return c || d
}

// Follow makes in tree the changes that follow an edit of the nodes of
// charts, and returns them.
func Follow(tree *ot.Tree, e ot.Edit, charts map[string]*Part) ot.Edit {
	var out ot.Edit
	for _, c := range e {
		p, n := charts[c.ID], tree.Node(c.ID)
		if c.Op != ot.OpSet || p == nil || n == nil || !touches(c.Attrs) {
			continue
		}
		if attrs := p.follow(n.Attrs, c.Attrs); attrs != nil {
			more := ot.Edit{{Op: ot.OpSet, ID: c.ID, Attrs: attrs}}
			if tree.Apply(more) == nil {
				out = append(out, more...)
			}
		}
	}
	return out
}

// Write writes the charts of the nodes of a tree, what changed only.
func Write(pkg *opc.Package, tree *ot.Tree, charts map[string]*Part) error {
	for _, id := range slices.Sorted(maps.Keys(charts)) {
		if n := tree.Node(id); n != nil {
			if err := charts[id].write(pkg, n.Attrs); err != nil {
				return err
			}
		}
	}
	return nil
}

// write writes the chart and the workbook of a node as its attributes
// have them, what changed only.
func (p *Part) write(pkg *opc.Package, attrs ot.Values) error {
	var t *xlsx.ChartTable
	if p.book != nil {
		t = &xlsx.ChartTable{}
		if err := json.Unmarshal(attrs[DataKey], t); err != nil {
			return err
		}
		if p.book.Same(t) {
			t = nil
		}
	}
	if t == nil && bytes.Equal(attrs[ChartKey], p.read) {
		return nil
	}
	var c chart.Chart
	if err := json.Unmarshal(attrs[ChartKey], &c); err != nil {
		return err
	}
	if t != nil {
		if _, err := p.book.Chart(t, &c); err != nil {
			return err
		}
		data, err := p.book.Save(t)
		if err != nil {
			return err
		}
		if err := pkg.Set(p.bookName, data); err != nil {
			return err
		}
	}
	data, err := pkg.Read(p.name)
	if err != nil {
		return err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return err
	}
	if !chart.Patch(doc.Root, &c) {
		return nil
	}
	return pkg.Set(p.name, doc.Bytes())
}
