// Package xmlcanon tells whether two XML parts mean the same thing to Office,
// however each was written.
//
// The canonical form keeps what a reader sees: elements and attributes by
// namespace URI and local name, attributes in sorted order, decoded values
// and text, comments and processing instructions. It drops what a writer
// chooses: prefixes, namespace declarations, quoting, escaping, the XML
// declaration, a byte order mark, and the whitespace between elements, which Office schemas
// never give a meaning (they have no mixed content).
package xmlcanon

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmltok"
)

const (
	xmlNamespace = "http://www.w3.org/XML/1998/namespace"
	mcNamespace  = "http://schemas.openxmlformats.org/markup-compatibility/2006"
)

// Canonical is the canonical form of an XML document, one line per node.
func Canonical(data []byte) ([]byte, error) {
	s := xmltok.New(bytes.TrimPrefix(data, []byte("\uFEFF")))
	var out, text []byte
	var hasChild []bool // per open element
	var err error
	flush := func(beforeElement bool) {
		blank := len(bytes.Trim(text, " \t\n")) == 0
		inContent := len(hasChild) > 0 && (beforeElement || hasChild[len(hasChild)-1])
		if len(text) > 0 && !(blank && (inContent || len(hasChild) == 0)) {
			out = append(out, "text "...)
			out = strconv.AppendQuote(out, string(text))
			out = append(out, '\n')
		}
		text = text[:0]
	}
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		switch tok.Kind {
		case xmltok.StartElement:
			flush(true)
			if len(hasChild) > 0 {
				hasChild[len(hasChild)-1] = true
			}
			hasChild = append(hasChild, false)
			var line []byte
			if line, err = element(s, tok); err != nil {
				return nil, err
			}
			out = append(out, line...)
		case xmltok.EndElement:
			flush(false)
			hasChild = hasChild[:len(hasChild)-1]
			out = append(out, ")\n"...)
		case xmltok.Text:
			if text, err = xmltok.Unescape(text, tok.Data); err != nil {
				return nil, err
			}
		case xmltok.CData:
			text = append(text, tok.Data...)
		case xmltok.Comment:
			flush(true)
			out = append(out, "comment "...)
			out = strconv.AppendQuote(out, string(tok.Data))
			out = append(out, '\n')
		case xmltok.ProcInst:
			flush(true)
			if string(tok.Name) != "xml" {
				out = append(out, "pi "...)
				out = strconv.AppendQuote(out, string(tok.Name)+" "+strings.TrimSpace(string(tok.Data)))
				out = append(out, '\n')
			}
		}
	}
	flush(false)
	return out, s.Err()
}

// element is the line of a start tag: its name, then its attributes sorted.
func element(s *xmltok.Scanner, tok xmltok.Token) ([]byte, error) {
	name, err := qualify(s, tok.Name, true)
	if err != nil {
		return nil, err
	}
	var attrs []string
	for n, v := range xmltok.Attrs(tok.Data) {
		if string(n) == "xmlns" || bytes.HasPrefix(n, []byte("xmlns:")) {
			continue
		}
		an, err := qualify(s, n, false)
		if err != nil {
			return nil, err
		}
		value, err := xmltok.Unescape(nil, normalize(v))
		if err != nil {
			return nil, err
		}
		if listsPrefixes(name, an) {
			value = resolvePrefixes(s, value)
		}
		attrs = append(attrs, an+"="+strconv.Quote(string(value)))
	}
	slices.Sort(attrs)
	line := "(" + name
	for _, a := range attrs {
		line += " " + a
	}
	return []byte(line + "\n"), nil
}

// qualify resolves a prefixed name to {uri}local. Unprefixed attributes have
// no namespace; unprefixed elements take the default one.
func qualify(s *xmltok.Scanner, name []byte, isElement bool) (string, error) {
	prefix, local, found := bytes.Cut(name, []byte(":"))
	if !found {
		if !isElement {
			return string(name), nil
		}
		prefix, local = nil, name
	}
	var uri []byte
	switch string(prefix) {
	case "xml":
		uri = []byte(xmlNamespace)
	default:
		uri = s.Space(prefix)
		if uri == nil && found {
			return "", fmt.Errorf("xmlcanon: undeclared prefix %q", prefix)
		}
	}
	if len(uri) == 0 {
		return string(local), nil
	}
	return "{" + string(uri) + "}" + string(local), nil
}

// listsPrefixes reports the markup compatibility attributes whose value is a
// list of prefixes: renaming a prefix without them changes what Office
// ignores.
func listsPrefixes(element, attr string) bool {
	const mc = "{" + mcNamespace + "}"
	return attr == mc+"Ignorable" || attr == mc+"MustUnderstand" ||
		element == mc+"Choice" && attr == "Requires"
}

// resolvePrefixes keeps a prefix that is not declared, which real files
// have, as "?prefix".
func resolvePrefixes(s *xmltok.Scanner, value []byte) []byte {
	var uris []string
	for _, p := range strings.Fields(string(value)) {
		uri := s.Space([]byte(p))
		if uri == nil {
			uris = append(uris, "?"+p)
			continue
		}
		uris = append(uris, string(uri))
	}
	slices.Sort(uris)
	return []byte(strings.Join(uris, " "))
}

// normalize applies XML attribute value normalization to a raw value: its
// literal line breaks and tabs read as spaces, but not those written as
// character references.
func normalize(raw []byte) []byte {
	if bytes.IndexAny(raw, "\t\n\r") < 0 {
		return raw
	}
	v := bytes.ReplaceAll(raw, []byte("\r\n"), []byte(" "))
	for i, c := range v {
		if c == '\t' || c == '\n' || c == '\r' {
			v[i] = ' '
		}
	}
	return v
}

// Diff is nil when a and b have the same canonical form, else an error
// describing the first difference.
func Diff(a, b []byte) error {
	ca, err := Canonical(a)
	if err != nil {
		return fmt.Errorf("first document: %w", err)
	}
	cb, err := Canonical(b)
	if err != nil {
		return fmt.Errorf("second document: %w", err)
	}
	la := strings.Split(string(ca), "\n")
	lb := strings.Split(string(cb), "\n")
	for i := range max(len(la), len(lb)) {
		x, y := line(la, i), line(lb, i)
		if x != y {
			return fmt.Errorf("xmlcanon: node %d differs:\n- %s\n+ %s", i+1, x, y)
		}
	}
	return nil
}

func line(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<end>"
}
