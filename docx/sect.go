package docx

import (
	"encoding/json"
	"maps"
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/ot"
)

// Section is the layout of the pages of a section, in twentieths of a
// point. Headers and footers name their nodes by kind: "default", "first",
// "even".
type Section struct {
	W        int               `json:"w"`
	H        int               `json:"h"`
	Orient   string            `json:"orient,omitempty"`
	Top      int               `json:"top"`
	Right    int               `json:"right"`
	Bottom   int               `json:"bottom"`
	Left     int               `json:"left"`
	Header   int               `json:"header"`
	Footer   int               `json:"footer"`
	Gutter   int               `json:"gutter,omitempty"`
	Cols     int               `json:"cols,omitempty"`
	ColSpace int               `json:"colSpace,omitempty"`
	Type     string            `json:"type,omitempty"`
	TitlePg  bool              `json:"titlePg,omitempty"`
	Hdr      map[string]string `json:"hdr,omitempty"`
	Ftr      map[string]string `json:"ftr,omitempty"`
	PgStart  *int              `json:"pgStart,omitempty"`
	PgFmt    string            `json:"pgFmt,omitempty"`
	// LinePitch is the pitch of the line grid of East Asian documents.
	LinePitch int `json:"linePitch,omitempty"`
}

var sectPrOrder = []string{
	"headerReference", "footerReference", "footnotePr", "endnotePr", "type", "pgSz", "pgMar", "paperSrc",
	"pgBorders", "lnNumType", "pgNumType", "cols", "formProt", "vAlign", "noEndnote", "titlePg",
	"textDirection", "bidi", "rtlGutter", "docGrid", "printerSettings", "sectPrChange",
}

func number(e *xmldom.Element, local string, deflt int) int {
	if n, err := strconv.Atoi(attr(e, local)); err == nil {
		return n
	}
	if f, err := strconv.ParseFloat(attr(e, local), 64); err == nil {
		return int(f)
	}
	return deflt
}

// readSection reads a sectPr; headers names the header and footer nodes by
// the name of their relationship.
func readSection(s *xmldom.Element, headers map[string]string) Section {
	sz, mar := child(s, "pgSz"), child(s, "pgMar")
	sec := Section{
		W:      number(sz, "w", 12240),
		H:      number(sz, "h", 15840),
		Orient: attr(sz, "orient"),
		Top:    number(mar, "top", 1440),
		Right:  number(mar, "right", 1800),
		Bottom: number(mar, "bottom", 1440),
		Left:   number(mar, "left", 1800),
		Header: number(mar, "header", 720),
		Footer: number(mar, "footer", 720),
		Gutter: number(mar, "gutter", 0),
		Type:   attr(child(s, "type"), "val"),
		PgFmt:  attr(child(s, "pgNumType"), "fmt"),
	}
	cols := child(s, "cols")
	sec.Cols = number(cols, "num", 1)
	sec.ColSpace = number(cols, "space", 720)
	sec.TitlePg = on(child(s, "titlePg")) == "1"
	if n, err := strconv.Atoi(attr(child(s, "pgNumType"), "start")); err == nil {
		sec.PgStart = &n
	}
	if g := child(s, "docGrid"); attr(g, "type") == "lines" || attr(g, "type") == "linesAndChars" || attr(g, "type") == "snapToChars" {
		sec.LinePitch = number(g, "linePitch", 0)
	}
	for _, ref := range s.Elements() {
		if ref.Space != NS || ref.Local != "headerReference" && ref.Local != "footerReference" {
			continue
		}
		id := headers[ref.Get("r:id")]
		if id == "" {
			continue
		}
		kind := attr(ref, "type")
		if kind == "" {
			kind = "default"
		}
		m := &sec.Hdr
		if ref.Local == "footerReference" {
			m = &sec.Ftr
		}
		if *m == nil {
			*m = map[string]string{}
		}
		(*m)[kind] = id
	}
	return sec
}

// sect reads the sectPr of a paragraph: its section as JSON, and the XML.
func (r *reader) sect(s *xmldom.Element) (string, string) {
	data, _ := json.Marshal(readSection(s, r.headers))
	return string(data), r.d.raw(s)
}

// section gives the node of the document the section of its last blocks.
func (r *reader) section(attrs ot.Values, s *xmldom.Element) {
	sect, raw := r.sect(s)
	attrs["sect"] = json.RawMessage(sect)
	attrs["sx"], _ = json.Marshal(raw)
}

// setSection patches a sectPr for what changed from old to sec; ref is the
// relationship id of the part of a header or footer node, "" when it has
// none.
func setSection(s *xmldom.Element, old, sec Section, ref func(id string) string) {
	if sec.W != old.W || sec.H != old.H || sec.Orient != old.Orient {
		sz := ensure(s, "pgSz", sectPrOrder)
		if sec.W > 0 && sec.W <= 31680 && sec.H > 0 && sec.H <= 31680 {
			setAttr(sz, "w", strconv.Itoa(sec.W))
			setAttr(sz, "h", strconv.Itoa(sec.H))
		}
		if sec.Orient == "landscape" || sec.Orient == "portrait" {
			setAttr(sz, "orient", sec.Orient)
		} else {
			unsetAttr(sz, "orient")
		}
	}
	margins := []struct {
		name     string
		old, new int
	}{
		{"top", old.Top, sec.Top}, {"right", old.Right, sec.Right}, {"bottom", old.Bottom, sec.Bottom},
		{"left", old.Left, sec.Left}, {"header", old.Header, sec.Header}, {"footer", old.Footer, sec.Footer},
		{"gutter", old.Gutter, sec.Gutter},
	}
	for _, m := range margins {
		if m.old == m.new || m.new < -31680 || m.new > 31680 {
			continue
		}
		mar := ensure(s, "pgMar", sectPrOrder)
		for _, m := range margins {
			if _, ok := mar.Attr(qualified(mar, m.name)); !ok {
				setAttr(mar, m.name, strconv.Itoa(m.old))
			}
		}
		v := m.new
		if m.name != "top" && m.name != "bottom" {
			v = max(v, 0)
		}
		setAttr(mar, m.name, strconv.Itoa(v))
	}
	if sec.Cols != old.Cols || sec.ColSpace != old.ColSpace {
		cols := ensure(s, "cols", sectPrOrder)
		cols.Content = nil
		unsetAttr(cols, "equalWidth")
		if sec.Cols >= 1 && sec.Cols <= 45 {
			setAttr(cols, "num", strconv.Itoa(sec.Cols))
		}
		if sec.ColSpace >= 0 && sec.ColSpace <= 31680 {
			setAttr(cols, "space", strconv.Itoa(sec.ColSpace))
		}
	}
	if sec.Type != old.Type {
		remove(s, "type")
		if enum("nextPage", "nextColumn", "continuous", "evenPage", "oddPage")(sec.Type) {
			insert(s, newW(s, "type", "val", sec.Type), sectPrOrder)
		}
	}
	if sec.TitlePg != old.TitlePg {
		remove(s, "titlePg")
		if sec.TitlePg {
			insert(s, newW(s, "titlePg"), sectPrOrder)
		}
	}
	if !samePtr(sec.PgStart, old.PgStart) || sec.PgFmt != old.PgFmt {
		pg := ensure(s, "pgNumType", sectPrOrder)
		unsetAttr(pg, "start")
		if sec.PgStart != nil && *sec.PgStart >= 0 {
			setAttr(pg, "start", strconv.Itoa(*sec.PgStart))
		}
		unsetAttr(pg, "fmt")
		if name(sec.PgFmt) && len(sec.PgFmt) < 40 {
			setAttr(pg, "fmt", sec.PgFmt)
		}
		if len(pg.Attrs) == 0 {
			s.Remove(pg)
		}
	}
	if !maps.Equal(sec.Hdr, old.Hdr) || !maps.Equal(sec.Ftr, old.Ftr) {
		remove(s, "headerReference")
		remove(s, "footerReference")
		for _, kind := range []struct {
			local string
			refs  map[string]string
		}{{"headerReference", sec.Hdr}, {"footerReference", sec.Ftr}} {
			for _, t := range []string{"default", "first", "even"} {
				id := kind.refs[t]
				if id == "" {
					continue
				}
				if rid := ref(id); rid != "" {
					e := newW(s, kind.local, "type", t)
					e.Set("r:id", rid)
					insert(s, e, sectPrOrder)
				}
			}
		}
	}
}

func samePtr(a, b *int) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}
