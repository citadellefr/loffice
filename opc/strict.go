package opc

import (
	"bytes"
	"math"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Strict Open XML names its namespaces and relationship types under
// purl.oclc.org. The parts of a Strict package are read as Transitional,
// the only form callers know, and those written into it go back to Strict.

const strictOfficeDocument = "http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument"

// strictNames pairs Strict names with their Transitional ones. A name
// ending with a slash is a prefix, that of relationship types.
var strictNames = [][2]string{
	{"http://purl.oclc.org/ooxml/officeDocument/relationships/extendedProperties", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties"},
	{"http://purl.oclc.org/ooxml/officeDocument/relationships/customProperties", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/custom-properties"},
	{"http://purl.oclc.org/ooxml/officeDocument/relationships/", "http://schemas.openxmlformats.org/officeDocument/2006/relationships/"},
	{"http://purl.oclc.org/ooxml/officeDocument/relationships", "http://schemas.openxmlformats.org/officeDocument/2006/relationships"},
	{"http://purl.oclc.org/ooxml/officeDocument/extendedProperties", "http://schemas.openxmlformats.org/officeDocument/2006/extended-properties"},
	{"http://purl.oclc.org/ooxml/officeDocument/customProperties", "http://schemas.openxmlformats.org/officeDocument/2006/custom-properties"},
	{"http://purl.oclc.org/ooxml/officeDocument/docPropsVTypes", "http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"},
	{"http://purl.oclc.org/ooxml/officeDocument/sharedTypes", "http://schemas.openxmlformats.org/officeDocument/2006/sharedTypes"},
	{"http://purl.oclc.org/ooxml/officeDocument/bibliography", "http://schemas.openxmlformats.org/officeDocument/2006/bibliography"},
	{"http://purl.oclc.org/ooxml/officeDocument/math", "http://schemas.openxmlformats.org/officeDocument/2006/math"},
	{"http://purl.oclc.org/ooxml/wordprocessingml/main", "http://schemas.openxmlformats.org/wordprocessingml/2006/main"},
	{"http://purl.oclc.org/ooxml/spreadsheetml/main", "http://schemas.openxmlformats.org/spreadsheetml/2006/main"},
	{"http://purl.oclc.org/ooxml/presentationml/main", "http://schemas.openxmlformats.org/presentationml/2006/main"},
	{"http://purl.oclc.org/ooxml/schemaLibrary/main", "http://schemas.openxmlformats.org/schemaLibrary/2006/main"},
	{"http://purl.oclc.org/ooxml/drawingml/main", "http://schemas.openxmlformats.org/drawingml/2006/main"},
	{"http://purl.oclc.org/ooxml/drawingml/chart", "http://schemas.openxmlformats.org/drawingml/2006/chart"},
	{"http://purl.oclc.org/ooxml/drawingml/chartDrawing", "http://schemas.openxmlformats.org/drawingml/2006/chartDrawing"},
	{"http://purl.oclc.org/ooxml/drawingml/diagram", "http://schemas.openxmlformats.org/drawingml/2006/diagram"},
	{"http://purl.oclc.org/ooxml/drawingml/picture", "http://schemas.openxmlformats.org/drawingml/2006/picture"},
	{"http://purl.oclc.org/ooxml/drawingml/spreadsheetDrawing", "http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing"},
	{"http://purl.oclc.org/ooxml/drawingml/wordprocessingDrawing", "http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"},
	{"http://purl.oclc.org/ooxml/drawingml/lockedCanvas", "http://schemas.openxmlformats.org/drawingml/2006/lockedCanvas"},
	{"http://purl.oclc.org/ooxml/drawingml/compatibility", "http://schemas.openxmlformats.org/drawingml/2006/compatibility"},
}

var toTransitional, toStrict = strictReplacers()

// strictReplacers replace the names where they are attribute values, quoted,
// which is where namespaces and relationship types are written.
func strictReplacers() (*strings.Replacer, *strings.Replacer) {
	var strict, transitional []string
	for _, n := range strictNames {
		for _, q := range []string{`"`, `'`} {
			s, t := q+n[0], q+n[1]
			if !strings.HasSuffix(s, "/") {
				s, t = s+q, t+q
			}
			strict = append(strict, s, t)
			transitional = append(transitional, t, s)
		}
	}
	return strings.NewReplacer(strict...), strings.NewReplacer(transitional...)
}

// isStrict tells whether the package relationships point to a Strict main
// part.
func (p *Package) isStrict() bool {
	rels, err := p.Relationships("")
	if err != nil {
		return false
	}
	for _, r := range rels {
		if r.Type == strictOfficeDocument {
			return true
		}
	}
	return false
}

// Strict tells whether the package is Strict Open XML.
func (p *Package) Strict() bool {
	return p.strict
}

// translate turns an XML part of a Strict package to Transitional, or
// back to Strict.
func (p *Package) translate(contentType string, data []byte, r *strings.Replacer) []byte {
	if !p.strict || !strings.HasSuffix(contentType, "xml") {
		return data
	}
	if r == toStrict {
		return []byte(toStrict.Replace(string(percents(data, true))))
	}
	return percents([]byte(toTransitional.Replace(string(data))), false)
}

const drawingML = "http://schemas.openxmlformats.org/drawingml/2006/main"

// percentages are the DrawingML attributes, by element, that Strict writes
// as percentages and Transitional in thousandths of a percent.
var percentages = map[string][]string{
	"alpha": {"val"}, "alphaOff": {"val"}, "alphaMod": {"val"}, "hueMod": {"val"},
	"sat": {"val"}, "satOff": {"val"}, "satMod": {"val"}, "lum": {"val", "bright", "contrast"},
	"lumOff": {"val"}, "lumMod": {"val"}, "shade": {"val"}, "tint": {"val", "amt"},
	"red": {"val"}, "redOff": {"val"}, "redMod": {"val"}, "green": {"val"}, "greenOff": {"val"},
	"greenMod": {"val"}, "blue": {"val"}, "blueOff": {"val"}, "blueMod": {"val"},
	"spcPct": {"val"}, "buSzPct": {"val"}, "normAutofit": {"fontScale", "lnSpcReduction"},
	"scrgbClr": {"r", "g", "b"}, "hslClr": {"sat", "lum"}, "hsl": {"sat", "lum"},
	"alphaModFix": {"amt"}, "alphaBiLevel": {"thresh"}, "biLevel": {"thresh"}, "alphaReplace": {"a"},
	"gs": {"pos"}, "fillToRect": {"l", "t", "r", "b"}, "tileRect": {"l", "t", "r", "b"},
	"srcRect": {"l", "t", "r", "b"}, "fillRect": {"l", "t", "r", "b"}, "tile": {"sx", "sy"},
	"outerShdw": {"sx", "sy"}, "reflection": {"stA", "stPos", "endA", "endPos", "sx", "sy"},
	"relOff": {"tx", "ty"},
}

// percents writes the percentages of the DrawingML elements of a part as
// Strict does, or as Transitional does.
func percents(data []byte, strict bool) []byte {
	if !bytes.Contains(data, []byte(drawingML)) {
		return data
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return data
	}
	if !convertPercents(doc.Root, strict) {
		return data
	}
	return doc.Bytes()
}

func convertPercents(e *xmldom.Element, strict bool) bool {
	changed := false
	if e.Space == drawingML {
		for _, name := range percentages[e.Local] {
			v, ok := e.Attr(name)
			if !ok {
				continue
			}
			if p, ok := percent(v, strict); ok {
				e.Set(name, p)
				changed = true
			}
		}
	}
	for _, c := range e.Elements() {
		if convertPercents(c, strict) {
			changed = true
		}
	}
	return changed
}

// percent is a value in thousandths of a percent as a percentage, or the
// other way round.
func percent(v string, strict bool) (string, bool) {
	if strict {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return "", false
		}
		return strconv.FormatFloat(float64(n)/1000, 'f', -1, 64) + "%", true
	}
	f, err := strconv.ParseFloat(strings.TrimSuffix(v, "%"), 64)
	if !strings.HasSuffix(v, "%") || err != nil || !(math.Abs(f) < 1e12) {
		return "", false
	}
	return strconv.FormatInt(int64(math.Round(f*1000)), 10), true
}
