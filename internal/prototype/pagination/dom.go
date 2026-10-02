package pagination

import (
	"bytes"
	"strconv"

	"github.com/citadellefr/loffice/internal/xmltok"
)

// node is a parsed XML element, named by its local name: the prototype reads
// only WordprocessingML, whose local names are unambiguous enough.
type node struct {
	name  string
	attrs map[string]string
	kids  []*node
	text  string
}

func parseXML(data []byte) (*node, error) {
	s := xmltok.New(data)
	root := &node{}
	stack := []*node{root}
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		top := stack[len(stack)-1]
		switch tok.Kind {
		case xmltok.StartElement:
			n := &node{name: local(tok.Name), attrs: map[string]string{}}
			for name, value := range xmltok.Attrs(tok.Data) {
				v, err := xmltok.Unescape(nil, value)
				if err != nil {
					return nil, err
				}
				n.attrs[local(name)] = string(v)
			}
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xmltok.EndElement:
			stack = stack[:len(stack)-1]
		case xmltok.Text, xmltok.CData:
			v := tok.Data
			if tok.Kind == xmltok.Text {
				var err error
				if v, err = xmltok.Unescape(nil, v); err != nil {
					return nil, err
				}
			}
			top.text += string(v)
		}
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return root, nil
}

func local(name []byte) string {
	if i := bytes.IndexByte(name, ':'); i >= 0 {
		return string(name[i+1:])
	}
	return string(name)
}

func (n *node) child(name string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

// find follows a path of child names.
func (n *node) find(names ...string) *node {
	for _, name := range names {
		n = n.child(name)
	}
	return n
}

func (n *node) attr(name string) string {
	if n == nil {
		return ""
	}
	return n.attrs[name]
}

func (n *node) children() []*node {
	if n == nil {
		return nil
	}
	return n.kids
}

func (n *node) content() string {
	if n == nil {
		return ""
	}
	return n.text
}

// num is a numeric attribute, and whether it is there.
func (n *node) num(name string) (float64, bool) {
	v, err := strconv.ParseFloat(n.attr(name), 64)
	return v, err == nil
}

// on reads an OOXML on/off property: present and not "0", "false" or "off".
func (n *node) on() bool {
	switch n.attr("val") {
	case "0", "false", "off":
		return false
	}
	return true
}
