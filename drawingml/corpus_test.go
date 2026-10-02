package drawingml

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmlcanon"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

// presentationParts are the parts of the corpus's presentations holding
// text bodies, read.
func presentationParts(t *testing.T, visit func(file, name string, doc *xmldom.Document, data []byte)) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../corpus/files/*/*")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".pptx", ".pptm", ".potx", ".ppsx":
		default:
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		p, err := opc.Open(data, opc.Limits{})
		if err != nil {
			continue
		}
		for _, name := range p.Names() {
			if !strings.HasPrefix(strings.TrimPrefix(name, "/"), "ppt/") || !strings.HasSuffix(name, ".xml") {
				continue
			}
			part, err := p.Read(name)
			if err != nil {
				continue
			}
			doc, err := xmldom.Parse(part)
			if err != nil || doc.Root.Spaces()["a"] != NS {
				continue
			}
			visit(f, name, doc, part)
		}
	}
}

func walk(e *xmldom.Element, visit func(*xmldom.Element)) {
	visit(e)
	for _, c := range e.Elements() {
		walk(c, visit)
	}
}

func fragments(spaces map[string]string) Raw {
	return func(raw string) *xmldom.Element {
		if raw == "" {
			return nil
		}
		e, err := xmldom.ParseFragment([]byte(raw), spaces)
		if err != nil {
			panic(err)
		}
		return e
	}
}

// Every text body read into a flow and written back means the same, but
// for runs of the same formatting side by side, which become one.
func TestCorpusTextRoundTrip(t *testing.T) {
	bodies := 0
	presentationParts(t, func(file, name string, doc *xmldom.Document, data []byte) {
		spaces := map[string]string{"a": NS, "r": RelNS}
		for k, v := range doc.Root.Spaces() {
			spaces[k] = v
		}
		changed := false
		walk(doc.Root, func(e *xmldom.Element) {
			if e.Local != "txBody" && e.Local != "txPr" && e.Local != "rich" {
				return
			}
			if child(e, "p") == nil {
				return
			}
			flow := Flow(e, nil)
			SetFlow(e, flow, fragments(spaces))
			bodies++
			changed = true
		})
		if !changed {
			return
		}
		original, err := xmldom.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		normalize(original.Root)
		rewritten, err := xmldom.Parse(doc.Bytes())
		if err != nil {
			t.Fatalf("%s %s: rewritten part does not parse: %v", file, name, err)
		}
		normalize(rewritten.Root)
		if err := xmlcanon.Diff(original.Bytes(), rewritten.Bytes()); err != nil {
			t.Fatalf("%s %s: %v", file, name, err)
		}
	})
	t.Logf("%d text bodies", bodies)
}

// normalize merges the runs of a paragraph that have the same properties
// and drops empty ones, as a flow does.
func normalize(e *xmldom.Element) {
	for _, c := range e.Elements() {
		normalize(c)
	}
	if e.Space != NS || e.Local != "p" {
		return
	}
	var out []xmldom.Node
	var last *xmldom.Element
	var lastText string
	var lastProps []byte
	flush := func() {
		if last != nil {
			t := xmldom.New(NS, "a:t")
			t.Append(xmldom.EscapeText(lastText))
			last.Append(t)
		}
		last = nil
	}
	for _, c := range e.Content {
		el, ok := c.(*xmldom.Element)
		if !ok {
			if len(bytes.TrimSpace(c.(xmldom.Raw))) > 0 {
				flush()
				out = append(out, c)
			}
			continue
		}
		if el.Space != NS || el.Local != "r" {
			flush()
			out = append(out, el)
			continue
		}
		s := text(child(el, "t"))
		if s == "" {
			continue
		}
		var props []byte
		rPr := child(el, "rPr")
		if rPr != nil {
			props = rPr.Bytes()
		}
		if last != nil && bytes.Equal(props, lastProps) {
			lastText += s
			continue
		}
		flush()
		last = xmldom.New(NS, "a:r")
		if rPr != nil {
			last.Append(rPr)
		}
		lastText, lastProps = s, props
		out = append(out, last)
	}
	flush()
	e.Content = out
}
