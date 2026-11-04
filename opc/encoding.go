package opc

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"unicode/utf16"

	"github.com/citadellefr/loffice/internal/charset"
	"github.com/citadellefr/loffice/internal/xmltok"
)

// utf8XML turns the content of the XML part name into UTF-8, from the encoding its
// declaration names or its byte order mark shows, and makes the declaration
// say so. Office reads these encodings; a UTF-8 part is returned as it is.
func utf8XML(name string, data []byte) ([]byte, error) {
	if bytes.HasPrefix(data, []byte{0xFF, 0xFE}) || bytes.HasPrefix(data, []byte{0xFE, 0xFF}) {
		return fixDeclaration(utf16Text(data)), nil
	}
	if !bytes.HasPrefix(data, []byte("<?xml")) {
		return data, nil
	}
	from, to := encodingValue(data)
	var text string
	switch encoding := string(data[from:to]); strings.ToLower(encoding) {
	case "", "utf-8", "utf8", "us-ascii", "ascii":
		return data, nil
	case "iso-8859-1", "iso_8859-1", "latin1":
		text = latin1(data)
	case "windows-1252", "cp1252":
		text = charset.Decode(data)
	default:
		return nil, fmt.Errorf("%w: %s: encoding %q", ErrInvalid, name, encoding)
	}
	return fixDeclaration(text), nil
}

// encodingValue is where the encoding named by the XML declaration is, an
// empty range if there is none.
func encodingValue[T string | []byte](data T) (from, to int) {
	decl := string(data)
	end := strings.Index(decl, "?>")
	if end < 0 {
		return 0, 0
	}
	decl = decl[:end]
	i := strings.Index(decl, "encoding")
	if i < 0 {
		return 0, 0
	}
	rest := strings.TrimLeft(decl[i+len("encoding"):], " \t\r\n")
	rest, ok := strings.CutPrefix(rest, "=")
	rest = strings.TrimLeft(rest, " \t\r\n")
	if !ok || rest == "" || rest[0] != '"' && rest[0] != '\'' {
		return 0, 0
	}
	n := strings.IndexByte(rest[1:], rest[0])
	if n < 0 {
		return 0, 0
	}
	from = end - len(rest) + 1
	return from, from + n
}

func fixDeclaration(text string) []byte {
	if from, to := encodingValue(text); strings.HasPrefix(text, "<?xml") && to > from {
		text = text[:from] + "UTF-8" + text[to:]
	}
	return []byte(text)
}

func latin1(data []byte) string {
	runes := make([]rune, len(data))
	for i, c := range data {
		runes[i] = rune(c)
	}
	return string(runes)
}

// utf16Text decodes UTF-16 with its byte order mark, a trailing odd byte
// being dropped.
func utf16Text(data []byte) string {
	order := binary.ByteOrder(binary.LittleEndian)
	if data[0] == 0xFE {
		order = binary.BigEndian
	}
	units := make([]uint16, (len(data)-2)/2)
	for i := range units {
		units[i] = order.Uint16(data[2+2*i:])
	}
	return string(utf16.Decode(units))
}

// checkProlog refuses a part whose document type declaration declares
// entities or has an external definition, as the readers of the formats do.
func checkProlog(name string, data []byte) error {
	s := xmltok.New(data)
	for tok, ok := s.Next(); ok && tok.Kind != xmltok.StartElement; tok, ok = s.Next() {
	}
	if err := s.Err(); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalid, name, err)
	}
	return nil
}
