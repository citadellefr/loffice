// Package xmldom reads an XML part into a tree of elements that keep the
// exact bytes they were read from, so that a part can be written back with
// only the elements that changed rewritten.
package xmldom

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"github.com/citadellefr/loffice/internal/xmltok"
)

var ErrNoRoot = errors.New("xmldom: no root element")

// Element is an element as read, or built. Content holds its children:
// elements and the raw bytes around them (text, comments, blanks).
type Element struct {
	Name  string // qualified, as written
	Space string // namespace URI
	Local string
	Attrs []Attr
	// Content is nil for an empty element.
	Content []Node
	// start and end are the tags as read, nil for a built element or one
	// whose attributes changed; end is empty for a self-closing element.
	start, end []byte
}

// Attr is an attribute, its value unescaped.
type Attr struct {
	Name  string
	Value string
}

// Node is a child of an element: an *Element, or Raw bytes kept as they are.
type Node interface{ node() }

// Raw is content written back byte for byte: text still escaped, comments,
// processing instructions, CDATA sections.
type Raw []byte

func (*Element) node() {}
func (Raw) node()      {}

// Document is a part: what comes before its root element, the root, and
// what follows.
type Document struct {
	Prolog []byte
	Root   *Element
	Tail   []byte
}

// Parse reads a part.
func Parse(data []byte) (*Document, error) {
	s := xmltok.New(data)
	for {
		tok, ok := s.Next()
		if !ok {
			if s.Err() != nil {
				return nil, s.Err()
			}
			return nil, ErrNoRoot
		}
		if tok.Kind != xmltok.StartElement {
			continue
		}
		root, end, err := parse(s, data, tok)
		if err != nil {
			return nil, err
		}
		for {
			t, ok := s.Next()
			if !ok {
				break
			}
			if t.Kind == xmltok.StartElement {
				return nil, xmltok.ErrSyntax
			}
		}
		if s.Err() != nil {
			return nil, s.Err()
		}
		return &Document{Prolog: data[:tok.Offset], Root: root, Tail: data[end:]}, nil
	}
}

// parse reads the element start opens, and returns where it ends.
func parse(s *xmltok.Scanner, data []byte, start xmltok.Token) (*Element, int, error) {
	e := &Element{Name: string(start.Name)}
	prefix, local := split(e.Name)
	e.Local = local
	e.Space = string(s.Space([]byte(prefix)))
	for name, value := range xmltok.Attrs(start.Data) {
		v, err := xmltok.Unescape(nil, value)
		if err != nil {
			return nil, 0, err
		}
		e.Attrs = append(e.Attrs, Attr{Name: string(name), Value: string(v)})
	}
	e.start = data[start.Offset:start.End]
	if start.SelfClosing {
		e.end = []byte{}
		s.Next()
		return e, start.End, nil
	}
	for {
		tok, ok := s.Next()
		if !ok {
			if s.Err() != nil {
				return nil, 0, s.Err()
			}
			return nil, 0, xmltok.ErrSyntax
		}
		switch tok.Kind {
		case xmltok.StartElement:
			child, _, err := parse(s, data, tok)
			if err != nil {
				return nil, 0, err
			}
			e.Content = append(e.Content, child)
		case xmltok.EndElement:
			e.end = data[tok.Offset:tok.End]
			return e, tok.End, nil
		default:
			e.Content = append(e.Content, Raw(data[tok.Offset:tok.End]))
		}
	}
}

func split(name string) (prefix, local string) {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[:i], name[i+1:]
	}
	return "", name
}

// Bytes writes the document back.
func (d *Document) Bytes() []byte {
	var b bytes.Buffer
	b.Write(d.Prolog)
	d.Root.Write(&b)
	b.Write(d.Tail)
	return b.Bytes()
}

// Write writes the element: its tags as read unless it changed, its
// children each the same way.
func (e *Element) Write(b *bytes.Buffer) {
	if e.start != nil && (len(e.end) > 0 || len(e.Content) == 0) {
		b.Write(e.start)
	} else {
		b.WriteByte('<')
		b.WriteString(e.Name)
		for _, a := range e.Attrs {
			b.WriteByte(' ')
			b.WriteString(a.Name)
			b.WriteString(`="`)
			escape(b, a.Value, true)
			b.WriteByte('"')
		}
		if len(e.Content) == 0 {
			b.WriteString("/>")
			return
		}
		b.WriteByte('>')
	}
	for _, c := range e.Content {
		switch c := c.(type) {
		case *Element:
			c.Write(b)
		case Raw:
			b.Write(c)
		}
	}
	if len(e.end) > 0 {
		b.Write(e.end)
	} else if len(e.Content) > 0 {
		b.WriteString("</")
		b.WriteString(e.Name)
		b.WriteByte('>')
	}
}

// Bytes is the element written back.
func (e *Element) Bytes() []byte {
	var b bytes.Buffer
	e.Write(&b)
	return b.Bytes()
}

// Attr is the value of the attribute with that qualified name, and whether
// there is one.
func (e *Element) Attr(name string) (string, bool) {
	for _, a := range e.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Get is the value of an attribute, "" if absent.
func (e *Element) Get(name string) string {
	v, _ := e.Attr(name)
	return v
}

// Set gives an attribute a value, added at the end if it is new.
func (e *Element) Set(name, value string) {
	for i, a := range e.Attrs {
		if a.Name == name {
			if a.Value != value {
				e.Attrs[i].Value = value
				e.start, e.end = nil, nil
			}
			return
		}
	}
	e.Attrs = append(e.Attrs, Attr{Name: name, Value: value})
	e.start, e.end = nil, nil
}

// Unset removes an attribute.
func (e *Element) Unset(name string) {
	for i, a := range e.Attrs {
		if a.Name == name {
			e.Attrs = append(e.Attrs[:i:i], e.Attrs[i+1:]...)
			e.start, e.end = nil, nil
			return
		}
	}
}

// Elements are the element children.
func (e *Element) Elements() []*Element {
	var out []*Element
	for _, c := range e.Content {
		if c, ok := c.(*Element); ok {
			out = append(out, c)
		}
	}
	return out
}

// Child is the first child element with that namespace and local name.
func (e *Element) Child(space, local string) *Element {
	for _, c := range e.Content {
		if c, ok := c.(*Element); ok && c.Local == local && c.Space == space {
			return c
		}
	}
	return nil
}

// Text is the text the element holds, entities decoded, markup skipped.
func (e *Element) Text() string {
	var b []byte
	for _, c := range e.Content {
		switch c := c.(type) {
		case *Element:
			b = append(b, c.Text()...)
		case Raw:
			switch {
			case bytes.HasPrefix(c, []byte("<![CDATA[")):
				b = append(b, c[9:len(c)-3]...)
			case len(c) > 0 && c[0] == '<':
			default:
				b, _ = xmltok.Unescape(b, c)
			}
		}
	}
	return string(b)
}

// Clone is a deep copy of the element, which changes to either leave the
// other alone.
func (e *Element) Clone() *Element {
	c := *e
	c.Attrs = slices.Clone(e.Attrs)
	if e.Content != nil {
		c.Content = make([]Node, len(e.Content))
		for i, n := range e.Content {
			if el, ok := n.(*Element); ok {
				n = el.Clone()
			}
			c.Content[i] = n
		}
	}
	return &c
}

// Remove takes the child out; it reports whether it was there.
func (e *Element) Remove(child *Element) bool {
	for i, c := range e.Content {
		if c == Node(child) {
			e.Content = append(e.Content[:i:i], e.Content[i+1:]...)
			return true
		}
	}
	return false
}

// Replace puts new in the place of old, or appends it when old is not a
// child.
func (e *Element) Replace(old, new *Element) {
	for i, c := range e.Content {
		if c == Node(old) {
			e.Content[i] = new
			return
		}
	}
	e.Append(new)
}

// Append adds a child at the end.
func (e *Element) Append(child Node) {
	if len(e.Content) == 0 && len(e.end) == 0 {
		e.start = nil
	}
	e.Content = append(e.Content, child)
}

// Insert adds a child where the sequence a schema imposes puts it: after
// the last child element whose local name comes no later than its own in
// order, or before the first one when none does. A child whose name order
// misses comes last; the children whose names it misses are passed over.
func (e *Element) Insert(child *Element, order []string) {
	rank := func(local string) int {
		for i, o := range order {
			if o == local {
				return i
			}
		}
		return -1
	}
	r := rank(child.Local)
	if r < 0 {
		r = len(order)
	}
	at := -1
	for i, c := range e.Content {
		c, ok := c.(*Element)
		if !ok {
			continue
		}
		k := rank(c.Local)
		if k > r {
			if at < 0 {
				at = i
			}
			break
		}
		if k >= 0 {
			at = i + 1
		}
	}
	if at < 0 {
		e.Append(child)
		return
	}
	e.Content = append(e.Content[:at:at], append([]Node{child}, e.Content[at:]...)...)
}

func escape(b *bytes.Buffer, s string, attr bool) {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			if attr {
				b.WriteString("&quot;")
			} else {
				b.WriteByte(c)
			}
		case '\t', '\n', '\r':
			if attr {
				b.WriteString(map[byte]string{'\t': "&#9;", '\n': "&#10;", '\r': "&#13;"}[c])
			} else {
				b.WriteByte(c)
			}
		default:
			b.WriteByte(c)
		}
	}
}

// EscapeText is s as element content.
func EscapeText(s string) Raw {
	var b bytes.Buffer
	escape(&b, s, false)
	return Raw(b.Bytes())
}

// New builds an element in the namespace space, under the qualified name
// given; attrs alternate names and values.
func New(space, name string, attrs ...string) *Element {
	e := &Element{Name: name, Space: space}
	_, e.Local = split(name)
	for i := 0; i+1 < len(attrs); i += 2 {
		e.Attrs = append(e.Attrs, Attr{Name: attrs[i], Value: attrs[i+1]})
	}
	return e
}

// Rename changes the local name of the element, keeping its prefix.
func (e *Element) Rename(local string) {
	if prefix, _ := split(e.Name); prefix != "" {
		e.Name = prefix + ":" + local
	} else {
		e.Name = local
	}
	e.Local = local
	e.start, e.end = nil, nil
}

// ParseFragment reads an element written out of its part, whose prefixes
// the part declared: spaces maps them to their namespaces, "" the default.
func ParseFragment(data []byte, spaces map[string]string) (*Element, error) {
	doc, err := Parse(data)
	if err != nil {
		return nil, err
	}
	var resolve func(e *Element)
	resolve = func(e *Element) {
		if e.Space == "" {
			prefix, _ := split(e.Name)
			e.Space = spaces[prefix]
		}
		for _, c := range e.Elements() {
			resolve(c)
		}
	}
	resolve(doc.Root)
	return doc.Root, nil
}

// Spaces are the namespace declarations of the element, by prefix.
func (e *Element) Spaces() map[string]string {
	out := map[string]string{}
	for _, a := range e.Attrs {
		switch {
		case a.Name == "xmlns":
			out[""] = a.Value
		case strings.HasPrefix(a.Name, "xmlns:"):
			out[a.Name[6:]] = a.Value
		}
	}
	return out
}
