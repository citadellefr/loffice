package legacy

import (
	"math"
	"slices"
	"unicode/utf16"

	"github.com/citadellefr/loffice/xlsx"
)

// The records that tell how a sheet is printed.
const (
	recLeftMargin   = 0x0026
	recRightMargin  = 0x0027
	recTopMargin    = 0x0028
	recBottomMargin = 0x0029
	recPrintHeaders = 0x002A
	recPrintGrid    = 0x002B
	recWsBool       = 0x0081
	recHCenter      = 0x0083
	recVCenter      = 0x0084
	recSetup        = 0x00A1
	recHeaderFooter = 0x089C
)

// What a page of Excel 97 holds by default, in inches.
var defaultMargins = xlsx.Margins{L: 0.75, R: 0.75, T: 1, B: 1, Head: 0.5, Foot: 0.5}

// The limits of Excel on a header or footer, in UTF-16 units, on the
// number of page breaks, on a margin in inches and on the paper.
const (
	maxHeader = 255
	maxBreaks = 1026
	maxMargin = 49
	maxPaper  = 118
)

// printing is how a sheet is printed, as its records tell it.
type printing struct {
	xlsx.Print
	margins xlsx.Margins
	// set tells that a record gave the margins.
	set bool
	// fit tells that the sheet is shrunk to pages, from its sheet properties.
	fit bool
	// pages are the pages it is shrunk to.
	pages [2]int
}

func (p *printing) margin(id uint16, d []byte) {
	if len(d) < 8 {
		return
	}
	p.set = true
	at := map[uint16]*float64{recLeftMargin: &p.margins.L, recRightMargin: &p.margins.R, recTopMargin: &p.margins.T, recBottomMargin: &p.margins.B}[id]
	*at = math.Float64frombits(le.Uint64(d))
}

// setup reads the page setup: paper, scale, pages, orientation.
func (p *printing) setup(d []byte) {
	if len(d) < 34 {
		return
	}
	p.set = true
	flags := le.Uint16(d[10:])
	p.margins.Head, p.margins.Foot = math.Float64frombits(le.Uint64(d[16:])), math.Float64frombits(le.Uint64(d[24:]))
	p.Across = flags&0x01 != 0
	if flags&0x80 != 0 {
		p.First = int(le.Uint16(d[4:]))
	}
	// the settings of the printer are void in a sheet that was never set up
	if flags&0x04 != 0 {
		return
	}
	p.Paper = int(le.Uint16(d))
	p.Scale = int(le.Uint16(d[2:]))
	p.pages = [2]int{int(le.Uint16(d[6:])), int(le.Uint16(d[8:]))}
	p.Land = flags&0x42 == 0
}

// headerFooter reads a header or a footer: the record is empty when there
// is none.
func headerFooter(d []byte) string {
	text, _, _ := longText(d)
	return text
}

// extra reads the header and footer of the even pages and of the first
// one, which a record of Excel 2000 adds to those of the others.
func (p *printing) extra(d []byte) {
	if len(d) < 38 {
		return
	}
	flags := le.Uint16(d[28:])
	sizes := [4]int{int(le.Uint16(d[30:])), int(le.Uint16(d[32:])), int(le.Uint16(d[34:])), int(le.Uint16(d[36:]))}
	var texts [4]string
	d = d[38:]
	for i, n := range sizes {
		if n > 0 {
			var size int
			if texts[i], size, _ = text(d, n); size == 0 {
				return
			}
			d = d[size:]
		}
	}
	if flags&0x01 != 0 {
		p.Even = &[2]string{texts[0], texts[1]}
	}
	if flags&0x02 != 0 {
		p.FirstPage = &[2]string{texts[2], texts[3]}
	}
}

// breaks reads the rows or columns at which a new page starts.
func breaks(d []byte, last int) []int {
	if len(d) < 2 {
		return nil
	}
	var out []int
	for d = d[2:]; len(d) >= 6; d = d[6:] {
		if at := int(le.Uint16(d)) + 1; at > 1 && at <= last {
			out = append(out, at)
		}
	}
	slices.Sort(out)
	out = slices.Compact(out)
	return out[:min(len(out), maxBreaks)]
}

// result is how the sheet is printed, nil when nothing differs from what
// a sheet takes by itself.
func (p *printing) result() *xlsx.Print {
	out := p.Print
	if p.set {
		m := p.margins
		for _, v := range []*float64{&m.L, &m.R, &m.T, &m.B, &m.Head, &m.Foot} {
			if !(*v >= 0 && *v <= maxMargin) {
				*v = 0
			}
		}
		out.Margins = &m
	}
	if p.fit {
		out.Fit, out.Scale = &p.pages, 0
	}
	if out.Scale < 10 || out.Scale > 400 || out.Scale == 100 {
		out.Scale = 0
	}
	if out.Paper > maxPaper {
		out.Paper = 0
	}
	if out.First < 2 {
		out.First = 0
	}
	if tooLong(out.Head, out.Foot) {
		out.Head, out.Foot = "", ""
	}
	if out.Even != nil && tooLong(out.Even[:]...) {
		out.Even = nil
	}
	if out.FirstPage != nil && tooLong(out.FirstPage[:]...) {
		out.FirstPage = nil
	}
	if string(mustJSON(out)) == "{}" {
		return nil
	}
	return &out
}

func tooLong(texts ...string) bool {
	return slices.ContainsFunc(texts, func(t string) bool { return len(utf16.Encode([]rune(t))) > maxHeader })
}
