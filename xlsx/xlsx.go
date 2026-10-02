// Package xlsx reads Excel workbooks into a tree of nodes (package ot) and
// writes the tree back into the workbook, rewriting only the parts that
// changed.
//
// The tree has a "book" node, whose children are the sheets in order, and
// one "xf" node per cell format at its root:
//
//	book "book"     theme, date1904, active, names
//	  sheet "S1"    name, state, frozen, grid, zoom, tab, dw, dh, tail;
//	                its cells in a grid (see below)
//	  kept "S4"     a chart, macro or dialog sheet, kept: name, state, and
//	                the chart a chart sheet shows (package chart)
//	xf "x0"         a Style
//
// A cell has the fields: v its value (number, string or boolean), e its
// error ("#DIV/0!"), f its formula without "=", s its format (an xf node
// id, none for the default), m the rows and columns it is merged over
// ([2,3]), fa the area of the array formula it holds, ca when it is always
// recalculated, cm its metadata index (dynamic arrays), rich its rich text
// and fx a data table formula, both XML the workbook carried. Column 0
// holds the fields of rows: h their height in points, ch when it was set
// by hand, hide, s, ol their outline level, cl when collapsed; row 0 those
// of columns: w their width in characters, cw, bf, hide, s, ol, cl.
// Columns past the first maxExpanded that share their fields are the
// sheet's "tail".
package xlsx

import (
	"errors"
	"fmt"
	"hash/maphash"
	"strconv"
	"sync"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/ot"
)

const (
	mainNS = "http://schemas.openxmlformats.org/spreadsheetml/2006/main"
	relNS  = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	aNS    = "http://schemas.openxmlformats.org/drawingml/2006/main"

	relBase           = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"
	relOfficeDocument = relBase + "officeDocument"
	relWorksheet      = relBase + "worksheet"
	relChartsheet     = relBase + "chartsheet"
	relSharedStrings  = relBase + "sharedStrings"
	relStyles         = relBase + "styles"
	relTheme          = relBase + "theme"
	relCalcChain      = relBase + "calcChain"

	typeWorksheet     = "application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"
	typeSharedStrings = "application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"
)

// maxExpanded is the last column whose fields are kept column by column.
const maxExpanded = 2048

var (
	ErrNotWorkbook = errors.New("xlsx: not a workbook")
	ErrStrict      = errors.New("xlsx: Strict Open XML workbooks are not supported")
)

const (
	strictNS             = "http://purl.oclc.org/ooxml/spreadsheetml/main"
	strictOfficeDocument = "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"
)

// Limits of the packages Open accepts.
var Limits = opc.Limits{}

// MaxCellData bounds the fields of all the cells of a workbook, once
// shared strings are copied into them: a long string shared by many cells
// would otherwise take the memory of a large workbook.
var MaxCellData = 256 << 20

// Document is a workbook open for editing.
type Document struct {
	mu       sync.Mutex // guards pkg once open
	original []byte
	pkg      *opc.Package
	bookName string
	book     *xmldom.Document
	bookRels []opc.Relationship
	styles   *styles
	sst      *sharedStrings
	date1904 bool
	sheets   map[string]*sheetPart // by node id
	// order are the sheet node ids as the workbook lists them.
	order  []string
	loaded *ot.Tree
	// moves are the rows and columns inserted and removed since the
	// workbook was read, and tables the areas of its tables as read, by
	// sheet.
	movesMu sync.Mutex
	moves   []move
	tables  map[string][]formula.Area
	// cellData is the size of the fields of the cells read.
	cellData int
	// trusted are the hashes of the XML the cells carry: only that may be
	// written back. The seed is secret, so that no client can forge XML
	// with the hash of some it was sent.
	trusted map[uint64]bool
	seed    maphash.Seed
	// csv is set for the workbook of a CSV file, whose cells may keep the
	// text they were read from.
	csv bool
}

type sheetPart struct {
	name    string
	sheetID int
	rid     string
	// kept is a sheet other than a worksheet, written back as it was.
	kept bool
	// doc is the worksheet with its sheetData left empty.
	doc *xmldom.Document
	// strings counts its cells that point to shared strings.
	strings int
}

// Open reads a workbook.
func Open(data []byte) (*Document, *ot.Tree, error) {
	pkg, err := opc.Open(data, Limits)
	if err != nil {
		return nil, nil, err
	}
	d := &Document{original: data, pkg: pkg, sheets: map[string]*sheetPart{}, tables: map[string][]formula.Area{}, trusted: map[uint64]bool{}, seed: maphash.MakeSeed()}
	root, err := pkg.Relationships("")
	if err != nil {
		return nil, nil, err
	}
	d.bookName = target("", root, relOfficeDocument)
	if d.bookName == "" {
		if target("", root, strictOfficeDocument) != "" {
			return nil, nil, ErrStrict
		}
		return nil, nil, ErrNotWorkbook
	}
	r := &reader{d: d, names: map[string]bool{}}
	if err := r.workbook(); err != nil {
		return nil, nil, err
	}
	tree, err := ot.NewTree(r.nodes)
	if err != nil {
		return nil, nil, fmt.Errorf("xlsx: %w", err)
	}
	d.loaded = tree.Clone()
	return d, tree, nil
}

// target is the first part source points to with a relationship of type
// typ, "" when there is none.
func target(source string, rels []opc.Relationship, typ string) string {
	for _, x := range rels {
		if x.Type == typ && !x.External {
			if name, err := opc.Resolve(source, x.Target); err == nil {
				return name
			}
		}
	}
	return ""
}

func (d *Document) trust(s string) {
	d.trusted[maphash.String(d.seed, s)] = true
}

func (d *Document) isTrusted(s string) bool {
	return d.trusted[maphash.String(d.seed, s)]
}

type reader struct {
	d     *Document
	nodes ot.Edit
	// names are the names of the sheets read, in lower case.
	names map[string]bool
}

func (r *reader) add(c ot.Change) {
	if len(c.Attrs) == 0 {
		c.Attrs = nil
	}
	r.nodes = append(r.nodes, c)
}

func (r *reader) part(name string) (*xmldom.Document, error) {
	data, err := r.d.pkg.Read(name)
	if err != nil {
		return nil, err
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("xlsx: %s: %w", name, err)
	}
	return doc, nil
}

func (r *reader) workbook() error {
	d := r.d
	book, err := r.part(d.bookName)
	if err != nil {
		return err
	}
	if book.Root.Space == strictNS {
		return ErrStrict
	}
	if book.Root.Space != mainNS || book.Root.Local != "workbook" {
		return ErrNotWorkbook
	}
	d.book = book
	if d.bookRels, err = d.pkg.Relationships(d.bookName); err != nil {
		return err
	}
	if pr := book.Root.Child(mainNS, "workbookPr"); pr != nil {
		d.date1904 = truthy(pr.Get("date1904"))
	}

	if name := target(d.bookName, d.bookRels, relStyles); name != "" {
		if doc, err := r.part(name); err == nil {
			d.styles = readStyles(name, doc)
		}
	}
	if d.styles == nil {
		d.styles = readStyles("", &xmldom.Document{Root: xmldom.New(mainNS, "styleSheet")})
	}
	if name := target(d.bookName, d.bookRels, relSharedStrings); name != "" {
		data, err := d.pkg.Read(name)
		if err != nil {
			return err
		}
		if d.sst, err = readSharedStrings(name, data); err != nil {
			return err
		}
	}

	attrs := ot.Values{}
	if name := target(d.bookName, d.bookRels, relTheme); name != "" {
		if doc, err := r.part(name); err == nil {
			attrs["theme"] = mustJSON(readTheme(doc.Root))
		}
	}
	if d.date1904 {
		attrs["date1904"] = mustJSON(true)
	}
	sheets := elements(book.Root.Child(mainNS, "sheets"), "sheet")
	keys := ot.Keys(len(sheets))
	var ids []string
	bookIndex := len(r.nodes)
	r.add(ot.Change{Op: ot.OpNew, ID: "book", Type: "book", Key: "V", Attrs: attrs})
	for i, s := range sheets {
		id, err := r.sheet(s, keys[i])
		if err != nil {
			return err
		}
		ids = append(ids, id)
	}
	d.order = ids

	if views := elements(book.Root.Child(mainNS, "bookViews"), "workbookView"); len(views) > 0 {
		if active := atoi(views[0].Get("activeTab")); active < len(ids) && ids[active] != "" {
			attrs["active"] = mustJSON(ids[active])
		}
	}
	var names []Name
	for _, n := range elements(book.Root.Child(mainNS, "definedNames"), "definedName") {
		name := Name{Name: n.Get("name"), Ref: n.Text(), Hidden: truthy(n.Get("hidden"))}
		if local, ok := n.Attr("localSheetId"); ok {
			if i := atoi(local); i < len(ids) {
				name.Sheet = ids[i]
			}
		}
		names = append(names, name)
	}
	if names != nil {
		attrs["names"] = mustJSON(names)
	}
	r.nodes[bookIndex].Attrs = attrs
	if len(attrs) == 0 {
		r.nodes[bookIndex].Attrs = nil
	}

	for i := range d.styles.xfs {
		st := d.styles.style(i)
		r.add(ot.Change{Op: ot.OpNew, ID: xfID(i), Type: "xf", Key: "V", Attrs: ot.Values{"style": mustJSON(st)}})
	}
	return nil
}

// Name is a defined name of the workbook: a formula without "=", on Sheet
// alone when it is local to it.
type Name struct {
	Name   string `json:"name"`
	Ref    string `json:"ref"`
	Sheet  string `json:"sheet,omitempty"`
	Hidden bool   `json:"hidden,omitempty"`
}

func xfID(i int) string {
	return "x" + strconv.Itoa(i)
}

// Theme is what cells take from the workbook's theme: its colors in the
// order the "theme" attribute of a color counts them, and its fonts.
type Theme struct {
	Colors []string `json:"colors"`
	Major  string   `json:"major,omitempty"`
	Minor  string   `json:"minor,omitempty"`
}

func readTheme(root *xmldom.Element) Theme {
	var t Theme
	elems := root.Child(aNS, "themeElements")
	if elems == nil {
		return t
	}
	if scheme := elems.Child(aNS, "clrScheme"); scheme != nil {
		// SpreadsheetML counts the light colors first
		for _, name := range []string{"lt1", "dk1", "lt2", "dk2", "accent1", "accent2", "accent3", "accent4", "accent5", "accent6", "hlink", "folHlink"} {
			c := "000000"
			if e := scheme.Child(aNS, name); e != nil {
				if s := e.Child(aNS, "srgbClr"); s != nil {
					c = rgb(s.Get("val"))
				} else if s := e.Child(aNS, "sysClr"); s != nil {
					c = rgb(s.Get("lastClr"))
				}
			}
			t.Colors = append(t.Colors, c)
		}
	}
	if fonts := elems.Child(aNS, "fontScheme"); fonts != nil {
		if f := fonts.Child(aNS, "majorFont"); f != nil {
			if l := f.Child(aNS, "latin"); l != nil {
				t.Major = l.Get("typeface")
			}
		}
		if f := fonts.Child(aNS, "minorFont"); f != nil {
			if l := f.Child(aNS, "latin"); l != nil {
				t.Minor = l.Get("typeface")
			}
		}
	}
	return t
}
