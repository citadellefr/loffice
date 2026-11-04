package xlsx

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// cellFields are the fields of a cell, in the order of their names.
type cellFields struct {
	CA   bool            `json:"ca,omitempty"`
	CM   int             `json:"cm,omitempty"`
	E    string          `json:"e,omitempty"`
	F    string          `json:"f,omitempty"`
	FA   string          `json:"fa,omitempty"`
	FX   string          `json:"fx,omitempty"`
	Link json.RawMessage `json:"link,omitempty"`
	M    *[2]int         `json:"m,omitempty"`
	Rich string          `json:"rich,omitempty"`
	S    string          `json:"s,omitempty"`
	V    json.RawMessage `json:"v,omitempty"`
	VM   int             `json:"vm,omitempty"`
}

// lineFields are the fields of a whole row or column.
type lineFields struct {
	BF   bool    `json:"bf,omitempty"`
	CH   bool    `json:"ch,omitempty"`
	CL   bool    `json:"cl,omitempty"`
	CW   bool    `json:"cw,omitempty"`
	H    float64 `json:"h,omitempty"`
	Hide bool    `json:"hide,omitempty"`
	OL   int     `json:"ol,omitempty"`
	S    string  `json:"s,omitempty"`
	W    float64 `json:"w,omitempty"`
}

// marshal writes the fields as encoding/json does, without its cost: a
// workbook has millions of cells.
func (f *cellFields) marshal() json.RawMessage {
	b := make([]byte, 0, 32)
	b = append(b, '{')
	field := func(name string) {
		if len(b) > 1 {
			b = append(b, ',')
		}
		b = append(b, '"')
		b = append(b, name...)
		b = append(b, '"', ':')
	}
	if f.CA {
		field("ca")
		b = append(b, "true"...)
	}
	if f.CM != 0 {
		field("cm")
		b = strconv.AppendInt(b, int64(f.CM), 10)
	}
	if f.E != "" {
		field("e")
		b = appendString(b, f.E)
	}
	if f.F != "" {
		field("f")
		b = appendString(b, f.F)
	}
	if f.FA != "" {
		field("fa")
		b = appendString(b, f.FA)
	}
	if f.FX != "" {
		field("fx")
		b = appendString(b, f.FX)
	}
	if len(f.Link) > 0 {
		field("link")
		b = append(b, f.Link...)
	}
	if f.M != nil {
		field("m")
		b = append(b, '[')
		b = strconv.AppendInt(b, int64(f.M[0]), 10)
		b = append(b, ',')
		b = strconv.AppendInt(b, int64(f.M[1]), 10)
		b = append(b, ']')
	}
	if f.Rich != "" {
		field("rich")
		b = appendString(b, f.Rich)
	}
	if f.S != "" {
		field("s")
		b = appendString(b, f.S)
	}
	if len(f.V) > 0 {
		field("v")
		b = append(b, f.V...)
	}
	if f.VM != 0 {
		field("vm")
		b = strconv.AppendInt(b, int64(f.VM), 10)
	}
	return append(b, '}')
}

const hex = "0123456789abcdef"

// appendString writes s as a JSON string, escaped as encoding/json
// escapes it.
func appendString(b []byte, s string) []byte {
	b = append(b, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 0x20 && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			b = append(b, s[start:i]...)
			switch c {
			case '"', '\\':
				b = append(b, '\\', c)
			case '\n':
				b = append(b, '\\', 'n')
			case '\r':
				b = append(b, '\\', 'r')
			case '\t':
				b = append(b, '\\', 't')
			case '\b':
				b = append(b, '\\', 'b')
			case '\f':
				b = append(b, '\\', 'f')
			default:
				b = append(b, '\\', 'u', '0', '0', hex[c>>4], hex[c&0xF])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 || r == '\u2028' || r == '\u2029' {
			b = append(b, s[start:i]...)
			switch r {
			case utf8.RuneError:
				b = append(b, `\ufffd`...)
			default:
				b = append(b, '\\', 'u', '2', '0', '2', hex[r&0xF])
			}
			i += size
			start = i
			continue
		}
		i += size
	}
	b = append(b, s[start:]...)
	return append(b, '"')
}
