package xlsx

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/citadellefr/loffice/internal/xmltok"
)

// sharedStrings is the table of strings cells point to. Strings a changed
// sheet adds are appended, so that the sheets left alone keep theirs.
type sharedStrings struct {
	name  string
	items []sharedString
	count int // as read
	// index finds plain strings, and rich ones by their XML.
	index map[string]int
}

type sharedString struct {
	text string
	// rich is the <si> element of a string with formatted runs or
	// phonetic guides, "" for plain text.
	rich string
}

func readSharedStrings(name string, data []byte) (*sharedStrings, error) {
	t := &sharedStrings{name: name, index: map[string]int{}}
	s := xmltok.New(data)
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		if tok.Kind != xmltok.StartElement || local(tok.Name) != "si" {
			continue
		}
		raw, err := s.SkipElement(tok)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %s: %w", name, err)
		}
		item, err := readItem(raw)
		if err != nil {
			return nil, fmt.Errorf("xlsx: %s: %w", name, err)
		}
		t.add(item)
	}
	if s.Err() != nil {
		return nil, fmt.Errorf("xlsx: %s: %w", name, s.Err())
	}
	t.count = len(t.items)
	return t, nil
}

func (t *sharedStrings) add(item sharedString) int {
	key := item.text
	if item.rich != "" {
		key = "\x00" + item.rich
	}
	if i, ok := t.index[key]; ok {
		return i
	}
	t.items = append(t.items, item)
	t.index[key] = len(t.items) - 1
	return len(t.items) - 1
}

// readItem reads a string item, <si> or <is>: its text is that of its
// runs, phonetic guides left out.
func readItem(raw []byte) (sharedString, error) {
	var item sharedString
	var b []byte
	rich := false
	s := xmltok.New(raw)
	skip := 0
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		switch tok.Kind {
		case xmltok.StartElement:
			switch local(tok.Name) {
			case "rPh":
				rich = true
				skip = s.Depth()
			case "r", "phoneticPr":
				rich = true
			case "t":
				if skip > 0 {
					continue
				}
				text, err := s.SkipElement(tok)
				if err != nil {
					return item, err
				}
				if !tok.SelfClosing {
					inner := text[tok.End-tok.Offset : bytes.LastIndexByte(text, '<')]
					if b, err = xmltok.Unescape(b, inner); err != nil {
						return item, err
					}
				}
			}
		case xmltok.EndElement:
			if skip > 0 && s.Depth() < skip {
				skip = 0
			}
		}
	}
	if s.Err() != nil {
		return item, s.Err()
	}
	item.text = decodeEscapes(string(b))
	if rich {
		item.rich = string(raw)
	}
	return item, nil
}

// decodeEscapes turns the _xHHHH_ escapes of characters XML cannot hold
// back into them.
func decodeEscapes(s string) string {
	if !strings.Contains(s, "_x") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if i+7 <= len(s) && s[i] == '_' && s[i+1] == 'x' && s[i+6] == '_' {
			var r rune
			ok := true
			for _, c := range s[i+2 : i+6] {
				switch {
				case c >= '0' && c <= '9':
					r = r*16 + c - '0'
				case c >= 'a' && c <= 'f':
					r = r*16 + c - 'a' + 10
				case c >= 'A' && c <= 'F':
					r = r*16 + c - 'A' + 10
				default:
					ok = false
				}
			}
			if ok {
				b.WriteRune(r)
				i += 6
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// encodeEscapes escapes what XML cannot hold or would read back otherwise
// (a carriage return), and an underscore that would read as an escape.
func encodeEscapes(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '_' && i+7 <= len(s) && s[i+1] == 'x' && s[i+6] == '_':
			b.WriteString("_x005F_")
		case r < 0x20 && r != '\t' && r != '\n', r == 0xFFFE, r == 0xFFFF:
			fmt.Fprintf(&b, "_x%04X_", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// local is a qualified name without its prefix.
func local(name []byte) string {
	if i := bytes.IndexByte(name, ':'); i >= 0 {
		name = name[i+1:]
	}
	return string(name)
}
