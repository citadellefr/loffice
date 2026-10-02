package xmltok

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

func tokens(t *testing.T, doc string) []string {
	t.Helper()
	s := New([]byte(doc))
	var out []string
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		if doc[tok.Offset:tok.End] == "" && tok.Kind != EndElement {
			t.Errorf("empty token %+v", tok)
		}
		switch tok.Kind {
		case StartElement:
			out = append(out, "<"+string(tok.Name)+strings.TrimSpace(string(tok.Data))+">")
		case EndElement:
			out = append(out, "</"+string(tok.Name)+">")
		case Text:
			out = append(out, "T:"+string(tok.Data))
		case CData:
			out = append(out, "C:"+string(tok.Data))
		case Comment:
			out = append(out, "!:"+string(tok.Data))
		case ProcInst:
			out = append(out, "?"+string(tok.Name))
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTokens(t *testing.T) {
	got := tokens(t, "<?xml version=\"1.0\"?>\r\n<w:p a='1'  b = \"2\"><w:r/>x &amp; y<!-- c --><![CDATA[<z>]]></w:p >")
	want := []string{
		"?xml", "T:\r\n", `<w:pa='1'  b = "2">`, "<w:r>", "</w:r>",
		"T:x &amp; y", "!: c ", "C:<z>", "</w:p>",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

func TestSyntaxErrors(t *testing.T) {
	for _, doc := range []string{
		`<!DOCTYPE a [<!ENTITY x "y">]><a/>`,
		`<a></b>`,
		`<a>`,
		`</a>`,
		`<a"b"/>`,
		`<a b=1/>`,
		`<a b="1/>`,
		`<a b="<"/>`,
		`<a/ >`,
		`<>`,
		`<a><!-- </a>`,
		`<a><![CDATA[ </a>`,
		`<?xml`,
	} {
		s := New([]byte(doc))
		for {
			if _, ok := s.Next(); !ok {
				break
			}
		}
		if !errors.Is(s.Err(), ErrSyntax) {
			t.Errorf("%q: %v, want a syntax error", doc, s.Err())
		}
	}
}

func TestTooDeep(t *testing.T) {
	s := New([]byte(strings.Repeat("<a>", MaxDepth+1)))
	for {
		if _, ok := s.Next(); !ok {
			break
		}
	}
	if !errors.Is(s.Err(), ErrTooDeep) {
		t.Errorf("%v, want ErrTooDeep", s.Err())
	}
}

func TestAttr(t *testing.T) {
	s := New([]byte(`<c r="A1" t = 's' s="&quot;"/>`))
	tok, _ := s.Next()
	for name, want := range map[string]string{"r": "A1", "t": "s", "s": "&quot;"} {
		if v, ok := tok.Attr(name); !ok || string(v) != want {
			t.Errorf("%s = %q, %v", name, v, ok)
		}
	}
	if _, ok := tok.Attr("x"); ok {
		t.Error("found a missing attribute")
	}
}

func TestSpace(t *testing.T) {
	s := New([]byte(`<a xmlns="d" xmlns:w="u1"><w:b xmlns:w="u2"/><c/></a>`))
	want := []struct{ prefix, uri string }{{"w", "u1"}, {"w", "u2"}, {"w", "u1"}}
	for _, w := range want {
		for {
			tok, ok := s.Next()
			if !ok {
				t.Fatal(s.Err())
			}
			if tok.Kind == StartElement {
				break
			}
		}
		if got := s.Space([]byte(w.prefix)); string(got) != w.uri {
			t.Errorf("%s = %q, want %q", w.prefix, got, w.uri)
		}
		if got := s.Space(nil); string(got) != "d" {
			t.Errorf("default = %q", got)
		}
	}
}

func TestSkipElement(t *testing.T) {
	doc := `<body><p><x:ext a="1"><y/>text</x:ext><r/></p><q/></body>`
	s := New([]byte(doc))
	var skipped []string
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		if tok.Kind == StartElement && (string(tok.Name) == "x:ext" || string(tok.Name) == "q") {
			raw, err := s.SkipElement(tok)
			if err != nil {
				t.Fatal(err)
			}
			skipped = append(skipped, string(raw))
		}
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []string{`<x:ext a="1"><y/>text</x:ext>`, `<q/>`}; !slices.Equal(skipped, want) {
		t.Fatalf("%q", skipped)
	}
}

func TestUnescape(t *testing.T) {
	for raw, want := range map[string]string{
		"plain":                   "plain",
		"a &amp; b &lt;&gt;":      "a & b <>",
		"&quot;&apos;":            `"'`,
		"&#233;&#xE9;&#x1F600;":   "éé😀",
		"one\r\ntwo\rthree\nfour": "one\ntwo\nthree\nfour",
		"&#xD800;":                "\uFFFD",
	} {
		got, err := Unescape(nil, []byte(raw))
		if err != nil || string(got) != want {
			t.Errorf("Unescape(%q) = %q, %v", raw, got, err)
		}
	}
	for _, raw := range []string{"&nbsp;", "&amp", "&#0;", "&#xFFFE;", "&#;", "&#x;", "&#-1;"} {
		if got, err := Unescape(nil, []byte(raw)); err == nil {
			t.Errorf("Unescape(%q) = %q, want an error", raw, got)
		}
	}
}

// events describes a document as encoding/xml sees it: resolved element
// names, decoded attribute values and text, comments and instructions left
// out. ok is false when the parser refuses the document.
func stdEvents(data []byte) (events []string, ok bool) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	flush := func() {
		if text.Len() > 0 {
			events = append(events, "T:"+text.String())
			text.Reset()
		}
	}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			flush()
			return events, true
		}
		if err != nil {
			return nil, false
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			flush()
			e := "S:" + tok.Name.Space + "|" + tok.Name.Local
			for _, a := range tok.Attr {
				e += " " + a.Name.Local + "=" + a.Value
			}
			events = append(events, e)
		case xml.EndElement:
			flush()
			events = append(events, "E")
		case xml.CharData:
			text.Write(tok)
		}
	}
}

func ourEvents(data []byte) (events []string, err error) {
	s := New(data)
	var text []byte
	flush := func() {
		if len(text) > 0 {
			events = append(events, "T:"+string(text))
			text = text[:0]
		}
	}
	for {
		tok, more := s.Next()
		if !more {
			flush()
			return events, s.Err()
		}
		switch tok.Kind {
		case StartElement:
			flush()
			prefix, local := splitName(tok.Name)
			e := "S:" + resolve(s, prefix) + "|" + local
			for name, value := range Attrs(tok.Data) {
				_, attrLocal := splitName(name)
				var v []byte
				if v, err = Unescape(nil, value); err != nil {
					break
				}
				e += " " + attrLocal + "=" + string(v)
			}
			events = append(events, e)
		case EndElement:
			flush()
			events = append(events, "E")
		case Text:
			text, err = Unescape(text, tok.Data)
		case CData:
			text = append(text, tok.Data...)
		}
		if err != nil {
			return nil, err
		}
	}
}

// splitName splits a qualified name the way encoding/xml does.
func splitName(name []byte) (prefix, local string) {
	i := bytes.IndexByte(name, ':')
	if i < 1 || i > len(name)-2 {
		return "", string(name)
	}
	return string(name[:i]), string(name[i+1:])
}

func resolve(s *Scanner, prefix string) string {
	if prefix == "xml" {
		return "http://www.w3.org/XML/1998/namespace"
	}
	if uri := s.Space([]byte(prefix)); uri != nil || prefix == "" {
		return string(uri)
	}
	return prefix
}

func compare(t *testing.T, name string, data []byte) {
	std, stdOK := stdEvents(data)
	ours, err := ourEvents(data)
	switch {
	case stdOK && err != nil && !errors.Is(err, ErrTooDeep) && !bytes.Contains(data, []byte("<!")):
		t.Errorf("%s: refused a document encoding/xml reads: %v", name, err)
	case stdOK && err == nil && !slices.Equal(std, ours):
		i := 0
		for i < min(len(std), len(ours)) && std[i] == ours[i] {
			i++
		}
		t.Errorf("%s: event %d differs:\nencoding/xml %q\nxmltok       %q", name, i, at(std, i), at(ours, i))
	}
}

func at(events []string, i int) string {
	if i < len(events) {
		return events[i]
	}
	return "<end>"
}

func TestSameAsEncodingXML(t *testing.T) {
	for i, doc := range []string{
		`<a xmlns="d" xmlns:w="u"><w:b w:c="1" d="&amp;">x<![CDATA[y]]>z</w:b></a>`,
		`<a xml:space="preserve"> t </a>`,
		"<a>\r\n</a>",
	} {
		compare(t, fmt.Sprint(i), []byte(doc))
	}
}

// TestCorpusSameAsEncodingXML reads every XML part of the corpus with both
// parsers.
func TestCorpusSameAsEncodingXML(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	root := filepath.Join("..", "..", "corpus", "files")
	paths, _ := filepath.Glob(filepath.Join(root, "*", "*.*x"))
	if len(paths) == 0 {
		t.Skip("no corpus: run corpus/fetch.sh")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			data, _ := os.ReadFile(path)
			p, err := opc.Open(data, opc.Limits{})
			if err != nil {
				return
			}
			for _, name := range p.Names() {
				if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
					continue
				}
				if part, err := p.Read(name); err == nil {
					compare(t, name, part)
				}
			}
		})
	}
}

func FuzzSameAsEncodingXML(f *testing.F) {
	for _, doc := range []string{
		`<a xmlns="d" xmlns:w="u"><w:b w:c="1" d="&amp;">x<![CDATA[y]]>z</w:b></a>`,
		`<?xml version="1.0"?><a><!-- c --><b/></a>`,
		`<a b='&#233;'>&lt;</a>`,
	} {
		f.Add([]byte(doc))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		compare(t, "input", data)
	})
}

func BenchmarkScan(b *testing.B) {
	var doc bytes.Buffer
	doc.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r := 1; r <= 10_000; r++ {
		fmt.Fprintf(&doc, `<row r="%d">`, r)
		for c := 'A'; c < 'U'; c++ {
			fmt.Fprintf(&doc, `<c r="%c%d" s="2"><f>SUM(A%d:B%d)</f><v>%d.25</v></c>`, c, r, r, r, r)
		}
		doc.WriteString(`</row>`)
	}
	doc.WriteString(`</sheetData></worksheet>`)
	data := doc.Bytes()

	b.Run("tokens", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		for b.Loop() {
			s := New(data)
			for {
				if _, ok := s.Next(); !ok {
					break
				}
			}
		}
	})
	b.Run("cells", func(b *testing.B) {
		b.SetBytes(int64(len(data)))
		var buf []byte
		for b.Loop() {
			s := New(data)
			for {
				tok, ok := s.Next()
				if !ok {
					break
				}
				switch tok.Kind {
				case StartElement:
					tok.Attr("r")
				case Text:
					buf, _ = Unescape(buf[:0], tok.Data)
				}
			}
		}
	})
}
