package xmlcanon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/xmltok"
	"github.com/citadellefr/loffice/opc"
)

const w = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

func TestSame(t *testing.T) {
	for _, pair := range [][2]string{
		{`<w:p xmlns:w="` + w + `"/>`, `<x:p xmlns:x="` + w + `"></x:p>`},
		{`<p xmlns="` + w + `"/>`, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" + `<w:p xmlns:w="` + w + `"/>`},
		{`<a b="1" c='2'/>`, `<a c="2"  b = '1' />`},
		{`<a b="&#x41;&amp;">&lt;x&gt;</a>`, `<a b="A&amp;"><![CDATA[<x>]]></a>`},
		{"<a>\n  <b/>\n  <c/>\n</a>", "<a><b/><c/></a>"},
		{`<a b="x` + "\n" + `y"/>`, `<a b="x y"/>`},
		{"<a>x\r\ny</a>", "<a>x\ny</a>"},
		{"\uFEFF<a/>", "<a/>"},
		{
			`<w:d xmlns:w="` + w + `" xmlns:mc="` + mcNamespace + `" xmlns:w14="u14" xmlns:w15="u15" mc:Ignorable="w14 w15"/>`,
			`<d xmlns="` + w + `" xmlns:m="` + mcNamespace + `" xmlns:a="u15" xmlns:b="u14" m:Ignorable="a b"/>`,
		},
		{
			`<a xmlns:mc="` + mcNamespace + `" mc:Ignorable="w14"/>`,
			`<a xmlns:m="` + mcNamespace + `" m:Ignorable="w14"/>`,
		},
		{
			`<mc:Choice xmlns:mc="` + mcNamespace + `" xmlns:wps="u" Requires="wps"/>`,
			`<m:Choice xmlns:m="` + mcNamespace + `" xmlns:s="u" Requires="s"/>`,
		},
	} {
		if err := Diff([]byte(pair[0]), []byte(pair[1])); err != nil {
			t.Errorf("%s\n%s\n%v", pair[0], pair[1], err)
		}
	}
}

func TestDifferent(t *testing.T) {
	for _, pair := range [][2]string{
		{`<w:p xmlns:w="u1"/>`, `<w:p xmlns:w="u2"/>`},
		{`<p/>`, `<p xmlns="u"/>`},
		{`<a b="1"/>`, `<a/>`},
		{`<a b="1"/>`, `<a b="2"/>`},
		{`<a><b/><c/></a>`, `<a><c/><b/></a>`},
		{`<t> </t>`, `<t/>`},
		{`<t>x</t>`, `<t>x </t>`},
		{`<a b="x&#10;y"/>`, `<a b="x y"/>`},
		{`<a><!-- c --></a>`, `<a/>`},
		{
			`<d xmlns:mc="` + mcNamespace + `" xmlns:w14="u14" mc:Ignorable="w14"/>`,
			`<d xmlns:mc="` + mcNamespace + `" xmlns:w14="u14" xmlns:w15="u15" mc:Ignorable="w15"/>`,
		},
	} {
		if err := Diff([]byte(pair[0]), []byte(pair[1])); err == nil {
			t.Errorf("%s and %s read the same", pair[0], pair[1])
		}
	}
}

func TestRefused(t *testing.T) {
	for _, doc := range []string{
		`<w:p/>`,
		`<a>`,
	} {
		if _, err := Canonical([]byte(doc)); err == nil {
			t.Errorf("%s: no error", doc)
		}
	}
}

func TestDiffShowsTheNode(t *testing.T) {
	err := Diff([]byte(`<a><b c="1"/></a>`), []byte(`<a><b c="2"/></a>`))
	if err == nil || !strings.Contains(err.Error(), `- (b c="1"`) || !strings.Contains(err.Error(), `+ (b c="2"`) {
		t.Fatal(err)
	}
}

// TestCorpus canonicalizes every XML part of the corpus that xmltok reads.
func TestCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	paths, _ := filepath.Glob(filepath.Join("..", "..", "corpus", "files", "*", "*.*x"))
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
				part, err := p.Read(name)
				if err != nil {
					continue
				}
				_, err = Canonical(part)
				if err != nil && !errors.Is(err, xmltok.ErrSyntax) && !errors.Is(err, xmltok.ErrTooDeep) {
					t.Errorf("%s: %v", name, err)
				}
			}
		})
	}
}
