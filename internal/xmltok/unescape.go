package xmltok

import (
	"bytes"
	"fmt"
	"strconv"
	"unicode/utf8"
)

// Unescape appends raw text to dst with its entity and character references
// decoded and its line breaks normalized to "\n", as an XML parser must. The
// content of CDATA sections is kept as it is, apart from its line breaks.
func Unescape(dst, raw []byte) ([]byte, error) {
	if bytes.IndexAny(raw, "&\r<") < 0 {
		return append(dst, raw...), nil
	}
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; c {
		case '<':
			rest := raw[i:]
			if !bytes.HasPrefix(rest, []byte("<![CDATA[")) {
				dst = append(dst, c)
				continue
			}
			n := bytes.Index(rest, []byte("]]>"))
			if n < 0 {
				return dst, fmt.Errorf("%w: unterminated CDATA section", ErrSyntax)
			}
			dst = lineBreaks(dst, rest[9:n])
			i += n + 2
		case '\r':
			dst = append(dst, '\n')
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
		case '&':
			end := bytes.IndexByte(raw[i:], ';')
			if end < 0 {
				return dst, fmt.Errorf("%w: unterminated reference", ErrSyntax)
			}
			ref := raw[i+1 : i+end]
			r, ok := reference(ref)
			if !ok {
				return dst, fmt.Errorf("%w: unknown reference &%s;", ErrSyntax, ref)
			}
			dst = utf8.AppendRune(dst, r)
			i += end
		default:
			dst = append(dst, c)
		}
	}
	return dst, nil
}

func lineBreaks(dst, raw []byte) []byte {
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\r' {
			dst = append(dst, raw[i])
			continue
		}
		dst = append(dst, '\n')
		if i+1 < len(raw) && raw[i+1] == '\n' {
			i++
		}
	}
	return dst
}

func reference(ref []byte) (rune, bool) {
	switch string(ref) {
	case "amp":
		return '&', true
	case "lt":
		return '<', true
	case "gt":
		return '>', true
	case "quot":
		return '"', true
	case "apos":
		return '\'', true
	}
	if len(ref) < 2 || ref[0] != '#' {
		return 0, false
	}
	var n uint64
	var err error
	if ref[1] == 'x' {
		n, err = strconv.ParseUint(string(ref[2:]), 16, 32)
	} else {
		n, err = strconv.ParseUint(string(ref[1:]), 10, 32)
	}
	r := rune(n)
	switch {
	case err != nil:
		return 0, false
	case r >= 0xD800 && r <= 0xDFFF:
		// a lone surrogate, which other readers also replace
		return utf8.RuneError, true
	}
	return r, isChar(r)
}

// isChar is the Char production of XML 1.0.
func isChar(r rune) bool {
	return r == 0x09 || r == 0x0A || r == 0x0D ||
		r >= 0x20 && r <= 0xD7FF ||
		r >= 0xE000 && r <= 0xFFFD ||
		r >= 0x10000 && r <= 0x10FFFF
}
