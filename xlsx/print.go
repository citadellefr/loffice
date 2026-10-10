package xlsx

import (
	"encoding/json"
	"slices"
	"strconv"
	"unicode/utf16"

	"github.com/citadellefr/loffice/formula"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const printKey = "print"

// Print is how a sheet goes on paper: its "print". What is printed and the
// rows and columns each page repeats are the names _xlnm.Print_Area and
// _xlnm.Print_Titles of the sheet.
type Print struct {
	// Paper is the size of the paper as Excel counts them: 1 Letter, 5
	// Legal, 8 A3, 9 A4, 11 A5.
	Paper int  `json:"paper,omitempty"`
	Land  bool `json:"land,omitempty"`
	// Scale is a percentage, 100 when it is 0. Fit takes its place: the
	// pages the sheet is shrunk to, wide and tall, 0 for as many as it
	// takes.
	Scale int     `json:"scale,omitempty"`
	Fit   *[2]int `json:"fit,omitempty"`
	// Across numbers the pages of a row before those under them.
	Across bool `json:"across,omitempty"`
	// First is the number of the first page, 1 when it is 0.
	First   int      `json:"first,omitempty"`
	Margins *Margins `json:"margins,omitempty"`
	CenterH bool     `json:"ch,omitempty"`
	CenterV bool     `json:"cv,omitempty"`
	// Grid prints the gridlines, Heads the letters and numbers of the
	// columns and rows.
	Grid  bool `json:"grid,omitempty"`
	Heads bool `json:"heads,omitempty"`
	// Rows and Cols are the rows and columns a page starts at.
	Rows []int `json:"rows,omitempty"`
	Cols []int `json:"cols,omitempty"`
	// Head and Foot are the header and footer in the codes of Excel
	// ("&L", "&C", "&R", "&P"…), Even and
	// FirstPage those of even pages and of the first one when they have
	// their own.
	Head      string     `json:"head,omitempty"`
	Foot      string     `json:"foot,omitempty"`
	Even      *[2]string `json:"even,omitempty"`
	FirstPage *[2]string `json:"firstPage,omitempty"`
}

// Margins are the margins of a page and the distance of its header and
// footer from the edge, in inches.
type Margins struct {
	L    float64 `json:"l"`
	R    float64 `json:"r"`
	T    float64 `json:"t"`
	B    float64 `json:"b"`
	Head float64 `json:"head"`
	Foot float64 `json:"foot"`
}

// The limits of Excel on what a page holds.
const (
	maxBreaks  = 1026
	maxHeader  = 255
	maxMargin  = 49
	maxPaper   = 118
	maxFitPage = 32767
)

func readPrint(root *xmldom.Element) *Print {
	var p Print
	if o := root.Child(mainNS, "printOptions"); o != nil {
		p.CenterH, p.CenterV = truthy(o.Get("horizontalCentered")), truthy(o.Get("verticalCentered"))
		p.Grid, p.Heads = truthy(o.Get("gridLines")), truthy(o.Get("headings"))
	}
	if m := root.Child(mainNS, "pageMargins"); m != nil {
		p.Margins = &Margins{
			L: margin(m.Get("left")), R: margin(m.Get("right")), T: margin(m.Get("top")), B: margin(m.Get("bottom")),
			Head: margin(m.Get("header")), Foot: margin(m.Get("footer")),
		}
	}
	fit := false
	if pr := root.Child(mainNS, "sheetPr"); pr != nil {
		if s := pr.Child(mainNS, "pageSetUpPr"); s != nil {
			fit = truthy(s.Get("fitToPage"))
		}
	}
	if s := root.Child(mainNS, "pageSetup"); s != nil {
		p.Paper = min(max(atoi(s.Get("paperSize")), 0), maxPaper)
		p.Land = s.Get("orientation") == "landscape"
		if scale := atoi(s.Get("scale")); scale >= 10 && scale <= 400 && scale != 100 {
			p.Scale = scale
		}
		p.Across = s.Get("pageOrder") == "overThenDown"
		if truthy(s.Get("useFirstPageNumber")) {
			if first := atoi(s.Get("firstPageNumber")); first > 1 && first <= maxFitPage {
				p.First = first
			}
		}
		if fit {
			p.Fit = &[2]int{fitPages(s, "fitToWidth"), fitPages(s, "fitToHeight")}
		}
	} else if fit {
		p.Fit = &[2]int{1, 1}
	}
	p.Rows = readBreaks(root.Child(mainNS, "rowBreaks"), formula.MaxRows)
	p.Cols = readBreaks(root.Child(mainNS, "colBreaks"), formula.MaxCols)
	if hf := root.Child(mainNS, "headerFooter"); hf != nil {
		text := func(local string) string {
			if e := hf.Child(mainNS, local); e != nil {
				return e.Text()
			}
			return ""
		}
		p.Head, p.Foot = text("oddHeader"), text("oddFooter")
		if truthy(hf.Get("differentOddEven")) {
			p.Even = &[2]string{text("evenHeader"), text("evenFooter")}
		}
		if truthy(hf.Get("differentFirst")) {
			p.FirstPage = &[2]string{text("firstHeader"), text("firstFooter")}
		}
	}
	if !p.valid() || p.empty() {
		return nil
	}
	return &p
}

func margin(s string) float64 {
	return min(max(parseFloat(s), 0), maxMargin)
}

func fitPages(s *xmldom.Element, name string) int {
	v, ok := s.Attr(name)
	if !ok {
		return 1
	}
	return min(max(atoi(v), 0), maxFitPage)
}

// readBreaks are the rows or columns pages start at: a break is written
// with the one before it.
func readBreaks(e *xmldom.Element, last int) []int {
	var out []int
	for _, b := range elements(e, "brk") {
		if at := atoi(b.Get("id")) + 1; at > 1 && at <= last {
			out = append(out, at)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	return out[:min(len(out), maxBreaks)]
}

func (p *Print) empty() bool {
	return string(mustJSON(p)) == "{}"
}

// valid tells whether the settings are some a worksheet can hold.
func (p *Print) valid() bool {
	if p.Paper < 0 || p.Paper > maxPaper || p.Scale != 0 && (p.Scale < 10 || p.Scale > 400) || p.First < 0 || p.First > maxFitPage {
		return false
	}
	if f := p.Fit; f != nil && (f[0] < 0 || f[0] > maxFitPage || f[1] < 0 || f[1] > maxFitPage) {
		return false
	}
	if m := p.Margins; m != nil {
		for _, v := range []float64{m.L, m.R, m.T, m.B, m.Head, m.Foot} {
			if !(v >= 0 && v <= maxMargin) {
				return false
			}
		}
	}
	texts := []string{p.Head, p.Foot}
	if p.Even != nil {
		texts = append(texts, p.Even[:]...)
	}
	if p.FirstPage != nil {
		texts = append(texts, p.FirstPage[:]...)
	}
	for _, t := range texts {
		if len(utf16.Encode([]rune(t))) > maxHeader {
			return false
		}
	}
	return validBreaks(p.Rows, formula.MaxRows) && validBreaks(p.Cols, formula.MaxCols)
}

func validBreaks(at []int, last int) bool {
	for i, b := range at {
		if b < 2 || b > last || i > 0 && b <= at[i-1] {
			return false
		}
	}
	return len(at) <= maxBreaks
}

func checkPrint(raw json.RawMessage) bool {
	var p Print
	return json.Unmarshal(raw, &p) == nil && p.valid()
}

// shifted are the settings once rows or columns are inserted at at, or
// removed when n is negative: the breaks move with what they come before.
func (p *Print) shifted(rows bool, at, n int) *Print {
	q := *p
	move := func(list []int) []int {
		var out []int
		for _, b := range list {
			if b = shiftIndex(b-1, at, n) + 1; b > 1 {
				out = append(out, b)
			}
		}
		slices.Sort(out)
		return slices.Compact(out)
	}
	if rows {
		q.Rows = move(p.Rows)
	} else {
		q.Cols = move(p.Cols)
	}
	if !q.valid() {
		return p
	}
	return &q
}

// writePrint writes how a sheet is printed, leaving what the settings do
// not tell as the worksheet had it: the printer's own, the pictures of
// the header.
func writePrint(root *xmldom.Element, raw json.RawMessage) {
	var p Print
	_ = json.Unmarshal(raw, &p)
	if !p.valid() {
		return
	}

	options := child(root, "printOptions", worksheetOrder)
	setAttr(options, "horizontalCentered", boolAttr(p.CenterH))
	setAttr(options, "verticalCentered", boolAttr(p.CenterV))
	setAttr(options, "gridLines", boolAttr(p.Grid))
	setAttr(options, "headings", boolAttr(p.Heads))
	if len(options.Attrs) == 0 {
		root.Remove(options)
	}

	if m := p.Margins; m != nil {
		e := child(root, "pageMargins", worksheetOrder)
		for _, a := range []struct {
			name string
			v    float64
		}{{"left", m.L}, {"right", m.R}, {"top", m.T}, {"bottom", m.B}, {"header", m.Head}, {"footer", m.Foot}} {
			e.Set(a.name, formatFloat(a.v))
		}
	} else if e := root.Child(mainNS, "pageMargins"); e != nil {
		root.Remove(e)
	}

	number := func(n int) string {
		if n == 0 {
			return ""
		}
		return strconv.Itoa(n)
	}
	setup := child(root, "pageSetup", worksheetOrder)
	setAttr(setup, "paperSize", number(p.Paper))
	setAttr(setup, "scale", number(p.Scale))
	setAttr(setup, "firstPageNumber", number(p.First))
	setAttr(setup, "useFirstPageNumber", boolAttr(p.First != 0))
	setup.Unset("fitToWidth")
	setup.Unset("fitToHeight")
	if f := p.Fit; f != nil {
		if f[0] != 1 {
			setup.Set("fitToWidth", strconv.Itoa(f[0]))
		}
		if f[1] != 1 {
			setup.Set("fitToHeight", strconv.Itoa(f[1]))
		}
	}
	setup.Unset("pageOrder")
	if p.Across {
		setup.Set("pageOrder", "overThenDown")
	}
	setup.Unset("orientation")
	if p.Land {
		setup.Set("orientation", "landscape")
	}
	if len(setup.Attrs) == 0 {
		root.Remove(setup)
	}
	pr := root.Child(mainNS, "sheetPr")
	if p.Fit != nil {
		pr = child(root, "sheetPr", worksheetOrder)
		s := pr.Child(mainNS, "pageSetUpPr")
		if s == nil {
			s = xmldom.New(mainNS, prefixOf(pr)+"pageSetUpPr")
			pr.Insert(s, sheetPrOrder)
		}
		s.Set("fitToPage", "1")
	} else if pr != nil {
		if s := pr.Child(mainNS, "pageSetUpPr"); s != nil {
			s.Unset("fitToPage")
		}
	}

	writeHeaderFooter(root, &p)
	writeBreaks(root, "rowBreaks", p.Rows, formula.MaxCols-1)
	writeBreaks(root, "colBreaks", p.Cols, formula.MaxRows-1)
}

var headerFooterOrder = []string{"oddHeader", "oddFooter", "evenHeader", "evenFooter", "firstHeader", "firstFooter"}

func writeHeaderFooter(root *xmldom.Element, p *Print) {
	even, first := [2]string{}, [2]string{}
	if p.Even != nil {
		even = *p.Even
	}
	if p.FirstPage != nil {
		first = *p.FirstPage
	}
	hf := root.Child(mainNS, "headerFooter")
	if hf == nil {
		if p.Head == "" && p.Foot == "" && p.Even == nil && p.FirstPage == nil {
			return
		}
		hf = child(root, "headerFooter", worksheetOrder)
	}
	setAttr(hf, "differentOddEven", boolAttr(p.Even != nil))
	setAttr(hf, "differentFirst", boolAttr(p.FirstPage != nil))
	for i, text := range []string{p.Head, p.Foot, even[0], even[1], first[0], first[1]} {
		local := headerFooterOrder[i]
		e := hf.Child(mainNS, local)
		switch {
		case text == "" && e != nil:
			hf.Remove(e)
		case text != "":
			if e == nil {
				e = xmldom.New(mainNS, prefixOf(hf)+local)
				hf.Insert(e, headerFooterOrder)
			}
			e.Content = []xmldom.Node{xmldom.EscapeText(text)}
		}
	}
	if len(hf.Attrs) == 0 && len(hf.Elements()) == 0 {
		root.Remove(hf)
	}
}

// writeBreaks writes the breaks of the rows or of the columns, each across
// the whole sheet, up to its line last.
func writeBreaks(root *xmldom.Element, local string, at []int, last int) {
	if e := root.Child(mainNS, local); e != nil {
		root.Remove(e)
	}
	if len(at) == 0 {
		return
	}
	e := child(root, local, worksheetOrder)
	count := strconv.Itoa(len(at))
	e.Set("count", count)
	e.Set("manualBreakCount", count)
	for _, b := range at {
		e.Append(xmldom.New(mainNS, prefixOf(e)+"brk", "id", strconv.Itoa(b-1), "max", strconv.Itoa(last), "man", "1"))
	}
}
