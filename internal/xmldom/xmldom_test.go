package xmldom

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

const sample = `<?xml version="1.0"?>
<p:sld xmlns:a="urn:a" xmlns:p="urn:p"><!-- c --><p:cSld>
  <a:t xml:space="preserve">a &amp; b<![CDATA[<c>]]></a:t><a:e/><a:x q='1'/>
</p:cSld></p:sld>
`

func TestRoundTrip(t *testing.T) {
	doc, err := Parse([]byte(sample))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(doc.Bytes()); got != sample {
		t.Fatalf("written back as %q", got)
	}
	c := doc.Root.Child("urn:p", "cSld")
	text := c.Child("urn:a", "t")
	if text.Text() != "a & b<c>" || text.Get("xml:space") != "preserve" {
		t.Fatalf("text %q", text.Text())
	}

	c.Child("urn:a", "x").Set("q", `"2"`)
	c.Child("urn:a", "e").Append(New("urn:a", "a:f", "v", "1&2"))
	c.Insert(New("urn:a", "a:first"), []string{"first", "t"})
	c.Remove(text)
	want := strings.Replace(sample, `<a:t xml:space="preserve">a &amp; b<![CDATA[<c>]]></a:t><a:e/><a:x q='1'/>`,
		`<a:first/><a:e><a:f v="1&amp;2"/></a:e><a:x q="&quot;2&quot;"/>`, 1)
	if got := string(doc.Bytes()); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestRefused(t *testing.T) {
	for _, bad := range []string{"", "<!-- only -->", "<a></b>", "<a/><b/>", "<a>"} {
		if _, err := Parse([]byte(bad)); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}

// Every XML part of the corpus is written back byte for byte.
func TestCorpus(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../../corpus/files/*/*")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	parts := 0
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".pptx", ".xlsx", ".pptm", ".xlsm":
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
			if !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".rels") {
				continue
			}
			part, err := p.Read(name)
			if err != nil {
				continue
			}
			doc, err := Parse(part)
			if err != nil {
				continue
			}
			parts++
			if got := doc.Bytes(); !bytes.Equal(got, part) {
				t.Fatalf("%s %s written back differently", f, name)
			}
		}
	}
	t.Logf("%d parts", parts)
}

func TestInsert(t *testing.T) {
	doc, err := Parse([]byte(`<w><a/><mc:x xmlns:mc="m"/><c/><e/></w>`))
	if err != nil {
		t.Fatal(err)
	}
	order := []string{"a", "b", "c", "d", "e"}
	doc.Root.Insert(New("", "d"), order)
	doc.Root.Insert(New("", "b"), order)
	doc.Root.Insert(New("", "z"), order)
	if got, want := string(doc.Bytes()), `<w><a/><b/><mc:x xmlns:mc="m"/><c/><d/><e/><z/></w>`; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}
