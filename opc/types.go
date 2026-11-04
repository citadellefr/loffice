package opc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"path"
	"slices"
	"strings"
)

const typesNamespace = "http://schemas.openxmlformats.org/package/2006/content-types"

// declaration is the XML declaration Office writes on every part.
const declaration = "<?xml version=\"1.0\" encoding=\"UTF-8\" standalone=\"yes\"?>\r\n"

// contentTypes is [Content_Types].xml, in its original order so that a
// rewrite changes no more than it must.
type contentTypes struct {
	XMLName xml.Name   `xml:"Types"`
	Rules   []typeRule `xml:",any"`
}

// typeRule is a Default when it has an Extension, else an Override.
type typeRule struct {
	XMLName     xml.Name
	Extension   string `xml:"Extension,attr,omitempty"`
	PartName    string `xml:"PartName,attr,omitempty"`
	ContentType string `xml:"ContentType,attr"`
}

func parseContentTypes(data []byte) (*contentTypes, error) {
	var t contentTypes
	if err := checkProlog(contentTypesName, data); err != nil {
		return nil, err
	}
	if err := xml.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, contentTypesName, err)
	}
	if t.XMLName.Space != typesNamespace {
		return nil, fmt.Errorf("%w: %s: namespace %q", ErrInvalid, contentTypesName, t.XMLName.Space)
	}
	t.Rules = slices.DeleteFunc(t.Rules, func(r typeRule) bool {
		return r.XMLName.Local != "Default" && r.XMLName.Local != "Override"
	})
	return &t, nil
}

// lookup applies the rules: an override for the part, else the default of its
// extension.
func (t *contentTypes) lookup(name string) string {
	if i := t.find(name); i >= 0 {
		return t.Rules[i].ContentType
	}
	ext := strings.TrimPrefix(path.Ext(name), ".")
	for _, r := range t.Rules {
		if r.PartName == "" && strings.EqualFold(r.Extension, ext) {
			return r.ContentType
		}
	}
	return ""
}

// find is the index of the override for the part, -1 if there is none.
func (t *contentTypes) find(name string) int {
	return slices.IndexFunc(t.Rules, func(r typeRule) bool {
		return r.PartName != "" && strings.EqualFold(r.PartName, "/"+name)
	})
}

func (t *contentTypes) override(name, contentType string) {
	if i := t.find(name); i >= 0 {
		t.Rules[i].ContentType = contentType
		return
	}
	t.Rules = append(t.Rules, typeRule{PartName: "/" + name, ContentType: contentType})
}

func (t *contentTypes) removeOverride(name string) bool {
	i := t.find(name)
	if i >= 0 {
		t.Rules = slices.Delete(t.Rules, i, i+1)
	}
	return i >= 0
}

func (t *contentTypes) marshal() []byte {
	var buf bytes.Buffer
	buf.WriteString(declaration)
	buf.WriteString(`<Types xmlns="` + typesNamespace + `">`)
	for _, r := range t.Rules {
		if r.PartName == "" {
			fmt.Fprintf(&buf, `<Default Extension="%s" ContentType="%s"/>`, escape(r.Extension), escape(r.ContentType))
		} else {
			fmt.Fprintf(&buf, `<Override PartName="%s" ContentType="%s"/>`, escape(r.PartName), escape(r.ContentType))
		}
	}
	buf.WriteString(`</Types>`)
	return buf.Bytes()
}

func escape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}
