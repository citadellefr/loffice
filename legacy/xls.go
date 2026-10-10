// Package legacy converts the documents of the Office of before 2007 into
// those of today, which the other packages read. Nothing of them is
// edited: a conversion is made once, and tells what it left out.
package legacy

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

var (
	ErrInvalid   = errors.New("legacy: not a document of Office 97 to 2003")
	ErrEncrypted = errors.New("legacy: the document is protected by a password")
	ErrTooOld    = errors.New("legacy: the document was written before Office 97")
	ErrEmpty     = errors.New("legacy: the workbook has no worksheet")
	ErrTooLarge  = errors.New("legacy: the workbook holds too many cells")
)

// MaxCells bounds the cells of a workbook: a file names millions of empty
// ones in little room. What they hold is bounded by xlsx.MaxCellData.
var MaxCells = 4 << 20

// Loss is something a document held that its conversion leaves out.
type Loss string

const (
	Charts      Loss = "charts"
	Drawings    Loss = "drawings" // pictures, shapes, form controls
	Comments    Loss = "comments"
	Links       Loss = "links"
	Conditional Loss = "conditional"
	Validation  Loss = "validation"
	Pivots      Loss = "pivots"
	Macros      Loss = "macros"
	Filters     Loss = "filters"    // what the filters of a sheet show
	Formulas    Loss = "formulas"   // formulas not understood, left as their values
	External    Loss = "external"   // formulas that read other workbooks, left as their values
	RichText    Loss = "rich"       // fonts of parts of the text of a cell
	Print       Loss = "print"      // headers, footers, page breaks
	Protection  Loss = "protection" // of the sheets and of the workbook
)

// blank is an empty workbook, which a converted one is written into.
//
//go:embed blank.xlsx
var blank []byte

// blankSheet is the sheet of the empty workbook.
const blankSheet = "S1"

// Workbook converts an Excel workbook of 97 to 2003 (.xls) into an .xlsx:
// its sheets, their cells with their values, formulas and formats, the
// sizes of rows and columns, merged cells, frozen panes, filtered areas
// and the names defined. lost is what it held that the .xlsx does not.
func Workbook(data []byte) (out []byte, lost []Loss, err error) {
	// before Excel 5 a workbook was its records alone, without a container
	if len(data) > 4 && data[0] == 0x09 && data[1] <= 0x08 && data[3] == 0 {
		return nil, nil, ErrTooOld
	}
	c, err := openCompound(data)
	if err != nil {
		return nil, nil, err
	}
	stream, ok, err := c.stream("Workbook")
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		if _, old, _ := c.stream("Book"); old {
			return nil, nil, ErrTooOld
		}
		return nil, nil, ErrInvalid
	}
	b := &book{
		formats: map[int]string{},
		lost:    map[Loss]bool{},
		xfIDs:   map[int]string{},
		styles:  map[string]string{},
		filters: map[int]string{},
	}
	for i := range b.palette {
		b.palette[i] = xlsx.IndexedColor(i + 8)
	}
	if c.find(0, "_VBA_PROJECT_CUR") != 0 {
		b.lose(Macros)
	}
	if err := b.globals(stream); err != nil {
		return nil, nil, err
	}
	edit, err := b.edit(stream)
	if err != nil {
		return nil, nil, err
	}
	doc, tree, err := xlsx.Open(blank)
	if err != nil {
		return nil, nil, err
	}
	if err := tree.Apply(edit); err != nil {
		return nil, nil, fmt.Errorf("legacy: %w", err)
	}
	if out, err = doc.Save(tree); err != nil {
		return nil, nil, err
	}
	return out, slices.Sorted(maps.Keys(b.lost)), nil
}

// book is a workbook as its first substream tells it.
type book struct {
	date1904 bool
	active   int
	fonts    [][]byte
	formats  map[int]string
	xfs      [][]byte
	// palette are the colors 8 to 63.
	palette  [56]string
	sheets   []boundSheet
	strings  []string
	supbooks []supbook
	externs  []extern
	names    []name
	lost     map[Loss]bool

	// xfIDs are the xf nodes made for the formats of the file, by index,
	// and styles those nodes by what they hold; nodes creates them.
	xfIDs  map[int]string
	styles map[string]string
	nodes  ot.Edit
	// filters are the filtered areas, by sheet.
	filters map[int]string
	// cells counts the cells read, and size what they hold.
	cells, size int
}

// The kinds of sheets told apart: the others hold macros.
const (
	sheetWorksheet = 0
	sheetChart     = 2
)

type boundSheet struct {
	offset uint32
	state  byte
	kind   byte
	name   string
}

// id is the node of the sheet at index i.
func sheetID(i int) string {
	return "L" + strconv.Itoa(i+1)
}

// The kinds of workbooks a formula reaches: this one, an add-in for its
// functions, another file.
const (
	supInternal = iota
	supAddIn
	supExternal
)

type supbook struct {
	kind  int
	names []string
}

// extern is an entry of the table the formulas name sheets by.
type extern struct {
	book, first, last uint16
}

type name struct {
	name   string
	hidden bool
	// sheet is the index of the sheet it is local to, counted from 1.
	sheet int
	// filter is set for the area a sheet filters.
	filter bool
	// code is its formula, nil for a function or a macro, and extra the
	// constants of its arrays.
	code, extra []byte
}

// builtinNames are the names Excel defines itself, by their code.
var builtinNames = [...]string{"Consolidate_Area", "Auto_Open", "Auto_Close", "Extract", "Database", "Criteria",
	"Print_Area", "Print_Titles", "Recorder", "Data_Form", "Auto_Activate", "Auto_Deactivate", "Sheet_Title",
	"_FilterDatabase"}

const builtinFilter = 13

func (b *book) lose(l Loss) {
	b.lost[l] = true
}

func (b *book) globals(stream []byte) error {
	r := records{stream}
	bof, ok := r.next()
	if !ok || len(bof.data) < 4 {
		return ErrInvalid
	}
	if bof.id != recBOF || le.Uint16(bof.data) != biff8 {
		return ErrTooOld
	}
	if le.Uint16(bof.data[2:]) != bofGlobals {
		return ErrInvalid
	}
	for {
		rec, ok := r.next()
		if !ok || rec.id == recEOF {
			return nil
		}
		d := rec.data
		switch rec.id {
		case recFilePass:
			return ErrEncrypted
		case recDate1904:
			b.date1904 = len(d) >= 2 && le.Uint16(d) == 1
		case recProtect:
			if len(d) >= 2 && le.Uint16(d) == 1 {
				b.lose(Protection)
			}
		case recWindow1:
			if len(d) >= 12 {
				b.active = int(le.Uint16(d[10:]))
			}
		case recFont:
			b.fonts = append(b.fonts, d)
		case recFormat:
			if len(d) > 2 {
				if code, size, _ := longText(d[2:]); size > 0 {
					b.formats[int(le.Uint16(d))] = code
				}
			}
		case recXF:
			b.xfs = append(b.xfs, d)
		case recPalette:
			for i := 0; i < len(b.palette) && 2+4*i+3 <= len(d); i++ {
				c := d[2+4*i:]
				b.palette[i] = fmt.Sprintf("%02X%02X%02X", c[0], c[1], c[2])
			}
		case recBoundSheet:
			if len(d) < 6 {
				return ErrInvalid
			}
			name, _ := shortText(d[6:])
			b.sheets = append(b.sheets, boundSheet{offset: le.Uint32(d), state: d[4] & 3, kind: d[5], name: name})
		case recSST:
			chunks := [][]byte{d}
			for r.peek() == recContinue {
				next, _ := r.next()
				chunks = append(chunks, next.data)
			}
			var rich bool
			if b.strings, rich = sharedStrings(chunks); rich {
				b.lose(RichText)
			}
		case recSupBook:
			sup := supbook{kind: supExternal}
			if len(d) == 4 {
				switch le.Uint16(d[2:]) {
				case 0x0401:
					sup.kind = supInternal
				case 0x3A01:
					sup.kind = supAddIn
				}
			}
			b.supbooks = append(b.supbooks, sup)
		case recExternName:
			if len(b.supbooks) > 0 && len(d) > 6 {
				name, _ := shortText(d[6:])
				sup := &b.supbooks[len(b.supbooks)-1]
				sup.names = append(sup.names, name)
			}
		case recExternSheet:
			for d = d[min(2, len(d)):]; len(d) >= 6; d = d[6:] {
				b.externs = append(b.externs, extern{le.Uint16(d), le.Uint16(d[2:]), le.Uint16(d[4:])})
			}
		case recName:
			b.names = append(b.names, readName(d))
		}
	}
}

// readName reads a defined name. Those that stand for functions and
// macros keep their name alone, for the formulas that call them.
func readName(d []byte) name {
	var n name
	if len(d) < 15 {
		return n
	}
	flags, length, size := le.Uint16(d), int(d[3]), int(le.Uint16(d[4:]))
	s, used, _ := text(d[14:], length)
	if used == 0 {
		return n
	}
	n.name, n.hidden, n.sheet = s, flags&0x01 != 0, int(le.Uint16(d[8:]))
	if flags&0x20 != 0 && len(s) == 1 && int(s[0]) < len(builtinNames) {
		n.name, n.filter = "_xlnm."+builtinNames[s[0]], s[0] == builtinFilter
	}
	if rest := d[14+used:]; flags&0x0E == 0 && size > 0 && size <= len(rest) {
		n.code, n.extra = rest[:size], rest[size:]
	}
	return n
}

// edit makes the workbook out of the empty one.
func (b *book) edit(stream []byte) (ot.Edit, error) {
	attrs := ot.Values{}
	if b.date1904 {
		attrs["date1904"] = json.RawMessage("true")
	}
	if names := b.definedNames(); len(names) > 0 {
		attrs["names"] = mustJSON(names)
	}
	var sheets ot.Edit
	keys := ot.Keys(len(b.sheets))
	for i, s := range b.sheets {
		switch s.kind {
		case sheetWorksheet:
			change, err := b.sheet(stream, i, keys[i])
			if err != nil {
				return nil, err
			}
			sheets = append(sheets, change)
			if i == b.active {
				attrs["active"] = mustJSON(change.ID)
			}
		case sheetChart:
			b.lose(Charts)
		default:
			b.lose(Macros)
		}
	}
	if len(sheets) == 0 {
		return nil, ErrEmpty
	}
	edit := ot.Edit{{Op: ot.OpDel, ID: blankSheet}}
	edit = append(edit, b.nodes...)
	edit = append(edit, sheets...)
	if len(attrs) > 0 {
		edit = append(edit, ot.Change{Op: ot.OpSet, ID: "book", Attrs: attrs})
	}
	return edit, nil
}

// definedNames are the names whose formula is understood, each once. The
// area a sheet filters is kept aside, for its sheet.
func (b *book) definedNames() []xlsx.Name {
	var out []xlsx.Name
	seen := map[[2]string]bool{}
	for _, n := range b.names {
		if n.code == nil {
			continue
		}
		local := ""
		if n.sheet > 0 {
			if n.sheet > len(b.sheets) || b.sheets[n.sheet-1].kind != sheetWorksheet {
				continue
			}
			local = sheetID(n.sheet - 1)
		}
		ref, err := b.formula(n.code, n.extra, origin{rows: formula.MaxRows, cols: formula.MaxCols, name: true})
		if err != nil {
			b.loseFormula(err)
			continue
		}
		key := [2]string{strings.ToLower(n.name), local}
		if seen[key] {
			continue
		}
		seen[key] = true
		if n.filter && n.sheet > 0 {
			b.filters[n.sheet-1] = ref
		}
		out = append(out, xlsx.Name{Name: n.name, Ref: ref, Sheet: local, Hidden: n.hidden})
	}
	return out
}

func (b *book) loseFormula(err error) {
	if errors.Is(err, errExternal) {
		b.lose(External)
	} else {
		b.lose(Formulas)
	}
}

func mustJSON(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}
