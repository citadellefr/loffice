package xlsx

import (
	"github.com/citadellefr/loffice/internal/xmldom"
)

// A Run is a part of the text of a cell, in a font of its own: the fields
// of Font that the workbook tells for it, none for the font of the cell.
type Run struct {
	Text string
	Font *Font
}

// RichText is the "rich" field for a cell whose text is made of runs, which
// the document trusts from then on. Call it before the document is used.
func (d *Document) RichText(runs []Run) string {
	si := xmldom.New(mainNS, "si", "xmlns", mainNS)
	for _, run := range runs {
		r := xmldom.New(mainNS, "r")
		if f := run.Font; f != nil {
			r.Append(runProperties(f))
		}
		t := xmldom.New(mainNS, "t", "xml:space", "preserve")
		t.Append(xmldom.Raw(escapeText(encodeEscapes(run.Text))))
		r.Append(t)
		si.Append(r)
	}
	rich := string(si.Bytes())
	d.trust(rich)
	return rich
}

func runProperties(f *Font) *xmldom.Element {
	p := xmldom.New(mainNS, "rPr")
	add := func(local string, attrs ...string) {
		p.Append(xmldom.New(mainNS, local, attrs...))
	}
	if f.Name != "" {
		add("rFont", "val", f.Name)
	}
	if f.Bold {
		add("b")
	}
	if f.Italic {
		add("i")
	}
	if f.Strike {
		add("strike")
	}
	if f.Color != nil {
		p.Append(colorElement(p, "color", f.Color))
	}
	if f.Size > 0 {
		add("sz", "val", formatFloat(f.Size))
	}
	switch f.Underline {
	case "":
	case "single":
		add("u")
	default:
		add("u", "val", f.Underline)
	}
	if f.VertAlign != "" {
		add("vertAlign", "val", f.VertAlign)
	}
	return p
}
