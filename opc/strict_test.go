package opc

import (
	"strings"
	"testing"
)

const (
	strictWord       = "http://purl.oclc.org/ooxml/wordprocessingml/main"
	transitionalWord = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
)

func TestPercents(t *testing.T) {
	p := &Package{strict: true}
	const space = `xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"`
	const strictSpace = `xmlns:a="http://purl.oclc.org/ooxml/drawingml/main"`
	strict := `<a:solidFill ` + strictSpace + `><a:schemeClr val="accent1"><a:lumMod val="12.5%"/><a:tint val="-3%"/></a:schemeClr><a:ext val="50%"/></a:solidFill>`
	transitional := `<a:solidFill ` + space + `><a:schemeClr val="accent1"><a:lumMod val="12500"/><a:tint val="-3000"/></a:schemeClr><a:ext val="50%"/></a:solidFill>`
	if got := string(p.translate("application/xml", []byte(strict), false)); got != transitional {
		t.Errorf("read as\n%s", got)
	}
	if got := string(p.translate("application/xml", []byte(transitional), true)); got != strict {
		t.Errorf("written as\n%s", got)
	}
	other := `<x:lumMod xmlns:x="urn:other" xmlns:a="` + strictDrawingML + `" val="12.5%"/>`
	if got := string(p.translate("application/xml", []byte(other), false)); got != strings.Replace(other, strictDrawingML, drawingML, 1) {
		t.Errorf("other namespace read as %s", got)
	}
}

func TestStrictTranslatesNamesOnly(t *testing.T) {
	quoted := `<w:t>"` + strictWord + `" '` + strictWord + `' "` + transitionalWord + `"</w:t>`
	strict := `<w:p xmlns:w="` + strictWord + `" w:rsid="` + strictWord + `">` + quoted + `</w:p>`
	transitional := `<w:p xmlns:w="` + transitionalWord + `" w:rsid="` + strictWord + `">` + quoted + `</w:p>`
	rels := `<Relationships xmlns="` + relsNamespace + `"><Relationship Id="rId1" Type="http://purl.oclc.org/ooxml/officeDocument/relationships/officeDocument" Target="a.xml"/>` +
		`<Relationship Id="rId2" Type="http://purl.oclc.org/ooxml/officeDocument/relationships/styles" Target="` + strictWord + `"/></Relationships>`
	p := &Package{strict: true}
	if got := string(p.translate("application/xml", []byte(strict), false)); got != transitional {
		t.Errorf("read as\n%s", got)
	}
	if got := string(p.translate("application/xml", []byte(transitional), true)); got != strict {
		t.Errorf("written as\n%s", got)
	}
	got := string(p.translate("application/vnd.openxmlformats-package.relationships+xml", []byte(rels), false))
	if !strings.Contains(got, `Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="`+strictWord+`"`) {
		t.Errorf("relationships read as\n%s", got)
	}
}
