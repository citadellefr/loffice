package opc

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/url"
	"path"
	"strings"
)

const relsNamespace = "http://schemas.openxmlformats.org/package/2006/relationships"

// Relationship links a source part to a target: another part of the package,
// or an external resource that must never be fetched on the user's behalf
// without asking.
type Relationship struct {
	ID       string `xml:"Id,attr"`
	Type     string `xml:"Type,attr"`
	Target   string `xml:"Target,attr"`
	External bool   `xml:"-"`
}

// RelsName is the part holding the relationships of source; "" is the
// package itself.
func RelsName(source string) string {
	dir, file := path.Split(source)
	return dir + "_rels/" + file + ".rels"
}

// MarshalRelationships writes a relationships part.
func MarshalRelationships(list []Relationship) []byte {
	var buf bytes.Buffer
	buf.WriteString(declaration)
	buf.WriteString(`<Relationships xmlns="` + relsNamespace + `">`)
	for _, r := range list {
		fmt.Fprintf(&buf, `<Relationship Id="%s" Type="%s" Target="%s"`, escape(r.ID), escape(r.Type), escape(r.Target))
		if r.External {
			buf.WriteString(` TargetMode="External"`)
		}
		buf.WriteString(`/>`)
	}
	buf.WriteString(`</Relationships>`)
	return buf.Bytes()
}

// Relationships of source, none when it has no relationships part.
func (p *Package) Relationships(source string) ([]Relationship, error) {
	name := RelsName(source)
	if !p.Has(name) {
		return nil, nil
	}
	data, err := p.Read(name)
	if err != nil {
		return nil, err
	}
	var doc struct {
		XMLName xml.Name `xml:"Relationships"`
		Rels    []struct {
			Relationship
			TargetMode string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}
	if err := checkProlog(name, data); err != nil {
		return nil, err
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalid, name, err)
	}
	if doc.XMLName.Space != relsNamespace {
		return nil, fmt.Errorf("%w: %s: namespace %q", ErrInvalid, name, doc.XMLName.Space)
	}
	rels := make([]Relationship, len(doc.Rels))
	for i, r := range doc.Rels {
		rels[i] = r.Relationship
		rels[i].External = r.TargetMode == "External"
	}
	return rels, nil
}

// Resolve turns an internal relationship target, relative to its source part
// or absolute, into a part name. A target climbing above the package root
// stops at the root, like a URI path.
func Resolve(source, target string) (string, error) {
	u, err := url.Parse(target)
	if target == "" || err != nil || u.Scheme != "" || u.Host != "" {
		return "", fmt.Errorf("%w: relationship target %q", ErrInvalid, target)
	}
	name := u.Path
	if !strings.HasPrefix(name, "/") {
		name = path.Join("/", path.Dir("/"+source), name)
	}
	name = strings.TrimPrefix(path.Clean(name), "/")
	if !validName(name) {
		return "", fmt.Errorf("%w: relationship target %q", ErrInvalid, target)
	}
	return name, nil
}
