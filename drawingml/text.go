package drawingml

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// Characters of a flow that stand for something other than text.
const (
	// LineBreak is an a:br, a new line within the paragraph.
	LineBreak = "\v"
	// Object is an element of a paragraph the model does not know, kept
	// whole in the attribute "o".
	Object = "￼"
)

// Flow reads the paragraphs of a text body, a txBody or a txPr, into a
// flow; link names the target of a hyperlink's relationship.
func Flow(body *xmldom.Element, link func(rid string) string) ot.Delta {
	var flow ot.Delta
	for _, p := range children(body) {
		if p.Local != "p" {
			continue
		}
		mark := ot.Attrs{}
		for _, c := range p.Elements() {
			if c.Space != NS {
				flow = flow.Push(ot.Op{Insert: Object, Attrs: ot.Attrs{"o": string(c.Bytes())}})
				continue
			}
			switch c.Local {
			case "pPr":
				mark = ot.Attrs(readProps(c, paraProps, nil))
				mark["p"] = string(c.Bytes())
			case "r":
				flow = flow.Push(ot.Op{Insert: text(child(c, "t")), Attrs: runAttrs(child(c, "rPr"), link)})
			case "br":
				flow = flow.Push(ot.Op{Insert: LineBreak, Attrs: runAttrs(child(c, "rPr"), link)})
			case "fld":
				s := text(child(c, "t"))
				if s == "" {
					flow = flow.Push(ot.Op{Insert: Object, Attrs: ot.Attrs{"o": string(c.Bytes())}})
					continue
				}
				attrs := runAttrs(child(c, "rPr"), link)
				if attrs == nil {
					attrs = ot.Attrs{}
				}
				bare := xmldom.New(c.Space, c.Name)
				bare.Attrs = c.Attrs
				for _, fc := range c.Elements() {
					if fc.Local != "rPr" && fc.Local != "t" {
						bare.Append(fc)
					}
				}
				attrs["fld"] = string(bare.Bytes())
				flow = flow.Push(ot.Op{Insert: s, Attrs: attrs})
			case "endParaRPr":
				for k, v := range runAttrs(c, link) {
					mark[k] = v
				}
			default:
				flow = flow.Push(ot.Op{Insert: Object, Attrs: ot.Attrs{"o": string(c.Bytes())}})
			}
		}
		flow = flow.Push(ot.Op{Insert: "\n", Attrs: mark})
	}
	if flow == nil {
		flow = ot.Delta{{Insert: "\n"}}
	}
	return flow
}

func runAttrs(rPr *xmldom.Element, link func(string) string) ot.Attrs {
	if rPr == nil {
		return nil
	}
	a := ot.Attrs(RunProps(rPr, link))
	a["r"] = string(rPr.Bytes())
	return a
}

// text is what an a:t holds, with the characters a flow gives a meaning
// to made spaces.
func text(t *xmldom.Element) string {
	if t == nil {
		return ""
	}
	s := t.Text()
	if strings.ContainsAny(s, "\n\r\v￼") {
		s = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' || r == '\v' || r == 0xFFFC {
				return ' '
			}
			return r
		}, s)
	}
	return s
}

// Raw checks the XML a flow carries: the attributes "r", "p", "fld" and "o"
// are written into the document as they are, and only XML it was read from
// may be. It returns the element, nil if raw is not trusted.
type Raw func(raw string) *xmldom.Element

// Links names the relationship of the link to an address a client asks
// for, "" when it is not one a document may hold.
type Links func(target string) string

// SetFlow replaces the paragraphs of a text body by those of a flow.
func SetFlow(body *xmldom.Element, flow ot.Delta, raw Raw, links Links) {
	var kept []xmldom.Node
	for _, c := range body.Content {
		if e, ok := c.(*xmldom.Element); ok && e.Space == NS && e.Local == "p" {
			continue
		}
		kept = append(kept, c)
	}
	body.Content = kept

	p := newA("p")
	var run *xmldom.Element
	var runKey string
	var runText strings.Builder
	endRun := func() {
		if run != nil {
			t := newA("t")
			t.Append(xmldom.EscapeText(runText.String()))
			run.Append(t)
			p.Append(run)
		}
		run, runKey = nil, ""
		runText.Reset()
	}
	for _, o := range flow {
		s := o.Insert
		for s != "" {
			i := strings.IndexAny(s, "\n\v￼")
			if i != 0 {
				chunk := s
				if i > 0 {
					chunk = s[:i]
				}
				key := attrsKey(o.Attrs)
				if run == nil || key != runKey {
					endRun()
					run = newRun(o.Attrs, raw, links)
					runKey = key
				}
				runText.WriteString(chunk)
				s = s[len(chunk):]
				continue
			}
			endRun()
			r, size := utf8.DecodeRuneInString(s)
			s = s[size:]
			switch r {
			case '\n':
				finishParagraph(p, o.Attrs, raw)
				body.Append(p)
				p = newA("p")
			case '\v':
				br := newA("br")
				if rPr := runElement(o.Attrs, "rPr", raw, nil); rPr != nil {
					br.Append(rPr)
				}
				p.Append(br)
			default:
				if e := raw(o.Attrs["o"]); e != nil {
					p.Append(e)
				}
			}
		}
	}
	endRun()
}

// attrsKey tells runs apart: two chunks of text with the same key belong
// to the same run.
func attrsKey(a ot.Attrs) string {
	var b strings.Builder
	for _, k := range sortedKeys(a) {
		b.WriteString(k)
		b.WriteByte(0)
		b.WriteString(a[k])
		b.WriteByte(0)
	}
	return b.String()
}

// newRun starts an a:r, or an a:fld when the text is a field's.
func newRun(a ot.Attrs, raw Raw, links Links) *xmldom.Element {
	run := newA("r")
	if f := a["fld"]; f != "" {
		if fld := raw(f); fld != nil && fld.Local == "fld" {
			run = fld
			rPr := runElement(a, "rPr", raw, nil)
			if rPr != nil {
				run.Content = append([]xmldom.Node{rPr}, run.Content...)
			}
			return run
		}
	}
	if rPr := runElement(a, "rPr", raw, links); rPr != nil {
		run.Append(rPr)
	}
	return run
}

// runElement is the rPr or endParaRPr of attributes: the one they were read
// from with their keys written over it, or a new one; nil when there is
// nothing to say. With links, it points where the "link" of a client asks.
func runElement(a ot.Attrs, local string, raw Raw, links Links) *xmldom.Element {
	props := Props{}
	for k, v := range a {
		if !paraKeys[k] && k != "r" && k != "o" && k != "fld" && k != "link" {
			props[k] = v
		}
	}
	e := raw(a["r"])
	var old Props
	if e != nil {
		old = RunProps(e, nil)
		delete(old, "link")
		if e.Local != local {
			e.Rename(local)
		}
	} else {
		if len(props) == 0 && (links == nil || a["link"] == "") {
			return nil
		}
		e = newA(local)
	}
	SetRunProps(e, old, props)
	if links != nil {
		setLink(e, a["link"], links)
	}
	return e
}

// setLink makes the hyperlink of a run lead to the address of its "link",
// when it is not where the run was read leading.
func setLink(rPr *xmldom.Element, link string, links Links) {
	var l struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(link), &l) != nil || l.URL == "" {
		return
	}
	name := links(l.URL)
	if name == "" {
		return
	}
	h := child(rPr, "hlinkClick")
	if h == nil {
		h = newA("hlinkClick")
		setChild(rPr, []string{"hlinkClick"}, h, rPrOrder)
	}
	if h.Get("r:id") != name {
		h.Set("r:id", name)
		h.Unset("action")
	}
}

// finishParagraph gives a paragraph its pPr and endParaRPr from the
// attributes of its mark.
func finishParagraph(p *xmldom.Element, mark ot.Attrs, raw Raw) {
	props := Props{}
	for k, v := range mark {
		if paraKeys[k] && k != "p" {
			props[k] = v
		}
	}
	pPr := raw(mark["p"])
	var old Props
	if pPr != nil && pPr.Local == "pPr" {
		old = readProps(pPr, paraProps, nil)
	} else if len(props) > 0 {
		pPr = newA("pPr")
	} else {
		pPr = nil
	}
	if pPr != nil {
		SetParaProps(pPr, old, props)
		p.Content = append([]xmldom.Node{pPr}, p.Content...)
	}
	if end := runElement(mark, "endParaRPr", raw, nil); end != nil {
		p.Append(end)
	}
}
