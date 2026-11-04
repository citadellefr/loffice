// Package xmltok is a fast XML tokenizer for the parts of Office documents.
//
// It allocates nothing per token: names, attributes and text are slices of
// the input, undecoded until the caller asks. Each token knows its position,
// so an element the caller does not understand can be kept as the exact bytes
// it was read from. Document type declarations that declare entities or point
// to an external definition are refused, which rules out entity expansion and
// external entity attacks.
package xmltok

import (
	"bytes"
	"errors"
	"fmt"
	"iter"
)

var (
	ErrSyntax  = errors.New("xmltok: syntax error")
	ErrTooDeep = errors.New("xmltok: elements nested too deeply")
)

type Kind uint8

const (
	StartElement Kind = iota + 1
	// EndElement follows every StartElement, including a self-closing one,
	// for which it is empty.
	EndElement
	// Text is character data with its entities not yet decoded: see Unescape.
	Text
	// CData is the literal content of a CDATA section.
	CData
	Comment
	ProcInst
	// Doctype is a document type declaration, which declares no entity.
	Doctype
)

type Token struct {
	Kind Kind
	// Name is the qualified name of an element, or the target of a
	// processing instruction.
	Name []byte
	// Data is the content of Text, CData and Comment tokens, and the
	// attributes of a StartElement.
	Data []byte
	// Offset and End delimit the token in the input.
	Offset, End int
	SelfClosing bool
}

// MaxDepth bounds element nesting.
const MaxDepth = 1024

type Scanner struct {
	data    []byte
	pos     int
	stack   [][]byte
	pending bool // a self-closing element still owes its EndElement
	rooted  bool // the root element was opened
	err     error
	spaces  []binding
}

type binding struct {
	prefix, uri []byte
	depth       int
}

func New(data []byte) *Scanner {
	return &Scanner{data: data}
}

// Err is the error that stopped the scanner, nil at the end of the input.
func (s *Scanner) Err() error { return s.err }

// Depth is the number of elements open around the scanner's position.
func (s *Scanner) Depth() int { return len(s.stack) }

func (s *Scanner) fail(format string, args ...any) (Token, bool) {
	s.err = fmt.Errorf("%w at offset %d: %s", ErrSyntax, s.pos, fmt.Sprintf(format, args...))
	return Token{}, false
}

// Next returns the next token, false at the end of the input or on error.
func (s *Scanner) Next() (Token, bool) {
	if s.err != nil {
		return Token{}, false
	}
	if s.pending {
		s.pending = false
		name := s.pop()
		return Token{Kind: EndElement, Name: name, Offset: s.pos, End: s.pos}, true
	}
	if s.pos >= len(s.data) {
		if len(s.stack) > 0 {
			return s.fail("element <%s> not closed", s.stack[len(s.stack)-1])
		}
		return Token{}, false
	}
	start := s.pos
	if s.data[start] != '<' {
		end := len(s.data)
		if i := bytes.IndexByte(s.data[start:], '<'); i >= 0 {
			end = start + i
		}
		s.pos = end
		return Token{Kind: Text, Data: s.data[start:end], Offset: start, End: end}, true
	}
	if start+1 == len(s.data) {
		return s.fail("unterminated tag")
	}
	switch s.data[start+1] {
	case '/':
		return s.endElement(start)
	case '?':
		return s.procInst(start)
	case '!':
		rest := s.data[start:]
		switch {
		case bytes.HasPrefix(rest, []byte("<!--")):
			return s.delimited(start, Comment, 4, "-->")
		case bytes.HasPrefix(rest, []byte("<![CDATA[")):
			return s.delimited(start, CData, 9, "]]>")
		}
		if bytes.HasPrefix(rest, []byte("<!DOCTYPE")) {
			return s.doctype(start)
		}
		return s.fail("malformed declaration")
	}
	return s.startElement(start)
}

func (s *Scanner) delimited(start int, kind Kind, open int, close string) (Token, bool) {
	i := bytes.Index(s.data[start+open:], []byte(close))
	if i < 0 {
		return s.fail("unterminated %q", s.data[start:start+open])
	}
	end := start + open + i + len(close)
	s.pos = end
	return Token{Kind: kind, Data: s.data[start+open : start+open+i], Offset: start, End: end}, true
}

// doctype reads a document type declaration. Entities are what make one
// dangerous: it is refused when it declares any, or when its definition is
// external, which could declare them.
func (s *Scanner) doctype(start int) (Token, bool) {
	if s.rooted {
		return s.fail("document type declaration after the root element")
	}
	var quote byte
	subset := false
	for i := start + len("<!DOCTYPE"); i < len(s.data); i++ {
		switch c := s.data[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[':
			subset = true
		case c == ']':
			subset = false
		case c == '>' && !subset:
			s.pos = i + 1
			decl := s.data[start:s.pos]
			switch {
			case bytes.Contains(decl, []byte("<!ENTITY")):
				return s.fail("entity declarations are not allowed")
			case bytes.Contains(decl, []byte("SYSTEM")) || bytes.Contains(decl, []byte("PUBLIC")):
				return s.fail("external document type definitions are not allowed")
			}
			return Token{Kind: Doctype, Data: decl[len("<!DOCTYPE") : len(decl)-1], Offset: start, End: s.pos}, true
		}
	}
	return s.fail("unterminated document type declaration")
}

func (s *Scanner) procInst(start int) (Token, bool) {
	i := bytes.Index(s.data[start+2:], []byte("?>"))
	if i < 0 {
		return s.fail("unterminated processing instruction")
	}
	body := s.data[start+2 : start+2+i]
	n := nameEnd(body, 0)
	if n == 0 {
		return s.fail("processing instruction without a target")
	}
	s.pos = start + 2 + i + 2
	return Token{Kind: ProcInst, Name: body[:n], Data: body[n:], Offset: start, End: s.pos}, true
}

func (s *Scanner) endElement(start int) (Token, bool) {
	i := start + 2
	n := nameEnd(s.data, i)
	name := s.data[i:n]
	i = skipSpace(s.data, n)
	if len(name) == 0 || i >= len(s.data) || s.data[i] != '>' {
		return s.fail("malformed end tag")
	}
	if len(s.stack) == 0 || !bytes.Equal(s.stack[len(s.stack)-1], name) {
		return s.fail("unexpected </%s>", name)
	}
	s.pop()
	s.pos = i + 1
	return Token{Kind: EndElement, Name: name, Offset: start, End: s.pos}, true
}

func (s *Scanner) startElement(start int) (Token, bool) {
	n := nameEnd(s.data, start+1)
	name := s.data[start+1 : n]
	if len(name) == 0 {
		return s.fail("malformed start tag")
	}
	if len(s.stack) == MaxDepth {
		s.err = fmt.Errorf("%w: more than %d levels", ErrTooDeep, MaxDepth)
		return Token{}, false
	}
	s.rooted = true
	attrStart := n
	i := n
	for {
		i = skipSpace(s.data, i)
		if i >= len(s.data) {
			return s.fail("unterminated <%s>", name)
		}
		switch s.data[i] {
		case '>', '/':
			selfClosing := s.data[i] == '/'
			if selfClosing && (i+1 >= len(s.data) || s.data[i+1] != '>') {
				return s.fail("malformed <%s>", name)
			}
			attrs := s.data[attrStart:i]
			s.pos = i + 1
			if selfClosing {
				s.pos++
			}
			s.stack = append(s.stack, name)
			s.pending = selfClosing
			if bytes.Contains(attrs, []byte("xmlns")) {
				s.bind(attrs)
			}
			return Token{Kind: StartElement, Name: name, Data: attrs, Offset: start, End: s.pos, SelfClosing: selfClosing}, true
		}
		end, ok := attrEnd(s.data, i)
		if !ok {
			return s.fail("malformed attribute in <%s>", name)
		}
		i = end
	}
}

func (s *Scanner) pop() []byte {
	name := s.stack[len(s.stack)-1]
	s.stack = s.stack[:len(s.stack)-1]
	for len(s.spaces) > 0 && s.spaces[len(s.spaces)-1].depth > len(s.stack) {
		s.spaces = s.spaces[:len(s.spaces)-1]
	}
	return name
}

// bind records the namespace declarations of the element just opened.
func (s *Scanner) bind(attrs []byte) {
	for name, value := range Attrs(attrs) {
		switch {
		case bytes.Equal(name, []byte("xmlns")):
			s.spaces = append(s.spaces, binding{uri: value, depth: len(s.stack)})
		case bytes.HasPrefix(name, []byte("xmlns:")):
			s.spaces = append(s.spaces, binding{prefix: name[6:], uri: value, depth: len(s.stack)})
		}
	}
}

// Space is the namespace URI bound to prefix at the scanner's position, the
// default namespace for an empty prefix. Namespace URIs never contain
// entities, so the value is returned as written.
func (s *Scanner) Space(prefix []byte) []byte {
	for i := len(s.spaces) - 1; i >= 0; i-- {
		if bytes.Equal(s.spaces[i].prefix, prefix) {
			return s.spaces[i].uri
		}
	}
	return nil
}

// SkipElement advances past the end of the element start opened, and
// returns its bytes, from its start tag to its end tag included.
func (s *Scanner) SkipElement(start Token) ([]byte, error) {
	depth := len(s.stack) - 1
	for {
		t, ok := s.Next()
		if !ok {
			if s.err == nil {
				s.err = fmt.Errorf("%w: element <%s> not closed", ErrSyntax, start.Name)
			}
			return nil, s.err
		}
		if t.Kind == EndElement && len(s.stack) == depth {
			end := t.End
			if start.SelfClosing {
				end = start.End
			}
			return s.data[start.Offset:end], nil
		}
	}
}

// Attr is the raw value of the attribute with that qualified name.
func (t Token) Attr(name string) ([]byte, bool) {
	for n, v := range Attrs(t.Data) {
		if string(n) == name {
			return v, true
		}
	}
	return nil, false
}

// Attrs iterates over the attributes of a start tag, as qualified names and
// raw values; the scanner has checked their syntax.
func Attrs(data []byte) iter.Seq2[[]byte, []byte] {
	return func(yield func([]byte, []byte) bool) {
		i := 0
		for {
			i = skipSpace(data, i)
			if i >= len(data) {
				return
			}
			n := nameEnd(data, i)
			name := data[i:n]
			j := skipSpace(data, n) + 1
			j = skipSpace(data, j)
			q := data[j]
			k := j + 1 + bytes.IndexByte(data[j+1:], q)
			if !yield(name, data[j+1:k]) {
				return
			}
			i = k + 1
		}
	}
}

// attrEnd checks the attribute at i and returns the offset past its value.
func attrEnd(data []byte, i int) (int, bool) {
	n := nameEnd(data, i)
	if n == i {
		return 0, false
	}
	j := skipSpace(data, n)
	if j >= len(data) || data[j] != '=' {
		return 0, false
	}
	j = skipSpace(data, j+1)
	if j >= len(data) || (data[j] != '"' && data[j] != '\'') {
		return 0, false
	}
	k := bytes.IndexByte(data[j+1:], data[j])
	if k < 0 || bytes.IndexByte(data[j+1:j+1+k], '<') >= 0 {
		return 0, false
	}
	return j + 1 + k + 1, true
}

func nameEnd(data []byte, i int) int {
	for i < len(data) {
		switch c := data[i]; c {
		case ' ', '\t', '\r', '\n', '/', '>', '=', '<', '"', '\'', '?':
			return i
		}
		i++
	}
	return i
}

func skipSpace(data []byte, i int) int {
	for i < len(data) && isSpace(data[i]) {
		i++
	}
	return i
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}
