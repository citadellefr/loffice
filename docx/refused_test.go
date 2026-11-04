package docx

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
)

const (
	mathNS  = `xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"`
	onePara = `<w:p><w:r><w:t>x</w:t></w:r></w:p>`
)

// withDocument is transitional.docx with another word/document.xml, whose
// body is the given text and whose prolog comes first.
func withDocument(t *testing.T, prolog, body string) []byte {
	t.Helper()
	data, err := os.ReadFile("../testdata/docx/transitional.docx")
	if err != nil {
		t.Fatal(err)
	}
	parts := rawParts(t, data)
	parts["word/document.xml"] = []byte(`<?xml version="1.0" encoding="UTF-8"?>` + prolog +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`)
	return zipOf(t, parts)
}

func TestDoctypeWithoutEntities(t *testing.T) {
	for _, prolog := range []string{"<!DOCTYPE w:document>", "<!DOCTYPE w:document [<!ELEMENT w:document ANY>]>"} {
		data := withDocument(t, prolog, onePara)
		d, tree, err := Open(data, opc.Limits{})
		if err != nil {
			t.Fatal(prolog, err)
		}
		saved, err := d.Save(tree)
		if err != nil || !bytes.Equal(saved, data) {
			t.Fatal(prolog, "unedited document saved differently", err)
		}
		_, again, err := Open(withDocument(t, "", onePara), opc.Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if err := sameTree(again, tree); err != nil {
			t.Fatal(prolog, err)
		}
	}
}

func TestEntitiesRefused(t *testing.T) {
	for prolog, want := range map[string]string{
		`<!DOCTYPE w:document [<!ENTITY a SYSTEM "http://poi.apache.org/">]>`: "docx: word/document.xml: xmltok: syntax error at offset 105: entity declarations are not allowed",
		`<!DOCTYPE w:document [<!ENTITY a "b">]>`:                             "docx: word/document.xml: xmltok: syntax error at offset 77: entity declarations are not allowed",
		`<!DOCTYPE w:document SYSTEM "a.dtd">`:                                "docx: word/document.xml: xmltok: syntax error at offset 74: external document type definitions are not allowed",
	} {
		_, _, err := Open(withDocument(t, prolog, onePara+"&a;"), opc.Limits{})
		if err == nil || err.Error() != want {
			t.Errorf("%s: %v, want %q", prolog, err, want)
		}
	}
}

func TestMath(t *testing.T) {
	pre := `<m:sPre><m:sPrePr/><m:sub><m:r><m:t>a</m:t></m:r></m:sub><m:sup/><m:e/></m:sPre>`
	body := `<w:p><m:oMath ` + mathNS + `>` + pre + `</m:oMath></w:p>`
	data := withDocument(t, "", body)
	d, tree, err := Open(data, opc.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := d.Save(tree)
	if err != nil || !bytes.Equal(saved, data) {
		t.Fatal("unedited document saved differently", err)
	}

	body = strings.Replace(body, "<m:t>a</m:t>", "<m:t>a</m:sPre>", 1)
	_, _, err = Open(withDocument(t, "", body), opc.Limits{})
	if err == nil || !strings.HasSuffix(err.Error(), "unexpected </m:sPre>") {
		t.Fatal(err)
	}
}

func TestTooDeep(t *testing.T) {
	const levels = 342
	body := strings.Repeat("<w:tbl><w:tr><w:tc>", levels) + onePara + strings.Repeat("</w:tc></w:tr></w:tbl>", levels)
	_, _, err := Open(withDocument(t, "", body), opc.Limits{})
	if err == nil || err.Error() != "docx: word/document.xml: xmltok: elements nested too deeply: more than 1024 levels" {
		t.Fatal(err)
	}
}
