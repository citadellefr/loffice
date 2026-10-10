package legacy

import (
	"math"
	"unicode/utf16"
)

// The records of BIFF8 that are read.
const (
	recFormula        = 0x0006
	recEOF            = 0x000A
	recProtect        = 0x0012
	recHeader         = 0x0014
	recFooter         = 0x0015
	recExternSheet    = 0x0017
	recName           = 0x0018
	recVPageBreaks    = 0x001A
	recHPageBreaks    = 0x001B
	recNote           = 0x001C
	recDate1904       = 0x0022
	recExternName     = 0x0023
	recFilePass       = 0x002F
	recFont           = 0x0031
	recContinue       = 0x003C
	recWindow1        = 0x003D
	recPane           = 0x0041
	recDefColWidth    = 0x0055
	recObj            = 0x005D
	recColInfo        = 0x007D
	recBoundSheet     = 0x0085
	recPalette        = 0x0092
	recStandardWidth  = 0x0099
	recAutoFilterInfo = 0x009D
	recAutoFilter     = 0x009E
	recScl            = 0x00A0
	recPivotView      = 0x00B0
	recMulRK          = 0x00BD
	recMulBlank       = 0x00BE
	recRString        = 0x00D6
	recXF             = 0x00E0
	recMergedCells    = 0x00E5
	recSST            = 0x00FC
	recLabelSST       = 0x00FD
	recSupBook        = 0x01AE
	recCondFmt        = 0x01B0
	recHyperlink      = 0x01B8
	recValidation     = 0x01BE
	recBlank          = 0x0201
	recNumber         = 0x0203
	recLabel          = 0x0204
	recBoolErr        = 0x0205
	recString         = 0x0207
	recRow            = 0x0208
	recArray          = 0x0221
	recDefRowHeight   = 0x0225
	recWindow2        = 0x023E
	recRK             = 0x027E
	recFormat         = 0x041E
	recSharedFormula  = 0x04BC
	recBOF            = 0x0809
	recSheetExt       = 0x0862
)

// What a BOF record tells of the substream it opens: the version of the
// format, and for the first that it holds what the workbook shares.
const (
	biff8      = 0x0600
	bofGlobals = 0x0005
)

type record struct {
	id   uint16
	data []byte
}

// records walks the records of a stream.
type records struct {
	data []byte
}

// next is the next record; ok is false at the end of the stream, or where
// it is cut short.
func (r *records) next() (rec record, ok bool) {
	if len(r.data) < 4 {
		return rec, false
	}
	size := int(le.Uint16(r.data[2:]))
	if len(r.data) < 4+size {
		return rec, false
	}
	rec = record{le.Uint16(r.data), r.data[4 : 4+size]}
	r.data = r.data[4+size:]
	return rec, true
}

// peek is the id of the next record, 0 at the end.
func (r *records) peek() uint16 {
	if len(r.data) < 4 {
		return 0
	}
	return le.Uint16(r.data)
}

// The flags of a string.
const (
	strWide = 0x01
	strExt  = 0x04
	strRich = 0x08
)

// text reads a string of n characters that starts at its flags; size is
// what it takes of b, 0 when b is too short, and rich tells whether parts
// of it had a font of their own.
func text(b []byte, n int) (s string, size int, rich bool) {
	if len(b) < 1 {
		return "", 0, false
	}
	flags, pos, after := b[0], 1, 0
	if flags&strRich != 0 {
		if len(b) < pos+2 {
			return "", 0, false
		}
		after += 4 * int(le.Uint16(b[pos:]))
		rich = after > 0
		pos += 2
	}
	if flags&strExt != 0 {
		if len(b) < pos+4 {
			return "", 0, false
		}
		after += int(le.Uint32(b[pos:]))
		pos += 4
	}
	units := make([]uint16, n)
	if flags&strWide != 0 {
		if len(b) < pos+2*n {
			return "", 0, false
		}
		for i := range units {
			units[i] = le.Uint16(b[pos+2*i:])
		}
		pos += 2 * n
	} else {
		if len(b) < pos+n {
			return "", 0, false
		}
		for i := range units {
			units[i] = uint16(b[pos+i])
		}
		pos += n
	}
	if len(b) < pos+after {
		return "", 0, false
	}
	return string(utf16.Decode(units)), pos + after, rich
}

// shortText reads a string whose length is a byte, longText one whose
// length is two.
func shortText(b []byte) (string, int) {
	if len(b) < 1 {
		return "", 0
	}
	s, size, _ := text(b[1:], int(b[0]))
	if size == 0 {
		return "", 0
	}
	return s, size + 1
}

func longText(b []byte) (s string, size int, rich bool) {
	if len(b) < 2 {
		return "", 0, false
	}
	s, size, rich = text(b[2:], int(le.Uint16(b)))
	if size == 0 {
		return "", 0, false
	}
	return s, size + 2, rich
}

// sharedStrings reads the table of strings, which goes on in the records
// that follow it. A string cut by the end of a record starts again in the
// next with flags of its own: its characters may change width there.
func sharedStrings(chunks [][]byte) (out []string, rich bool) {
	if len(chunks[0]) < 8 {
		return nil, false
	}
	count := int(le.Uint32(chunks[0][4:]))
	b, rest := chunks[0][8:], chunks[1:]
	// take moves past n bytes, across records
	take := func(n int) bool {
		for n > len(b) {
			if len(rest) == 0 {
				return false
			}
			n -= len(b)
			b, rest = rest[0], rest[1:]
		}
		b = b[n:]
		return true
	}
	for len(out) < count {
		if len(b) == 0 && len(rest) > 0 {
			b, rest = rest[0], rest[1:]
		}
		if len(b) < 3 {
			return out, rich
		}
		n, flags := int(le.Uint16(b)), b[2]
		b = b[3:]
		after := 0
		if flags&strRich != 0 {
			if len(b) < 2 {
				return out, rich
			}
			after += 4 * int(le.Uint16(b))
			rich = rich || after > 0
			b = b[2:]
		}
		if flags&strExt != 0 {
			if len(b) < 4 {
				return out, rich
			}
			after += int(le.Uint32(b))
			b = b[4:]
		}
		units := make([]uint16, 0, n)
		wide := flags&strWide != 0
		for len(units) < n {
			if len(b) == 0 {
				if len(rest) == 0 || len(rest[0]) == 0 {
					return out, rich
				}
				wide = rest[0][0]&strWide != 0
				b, rest = rest[0][1:], rest[1:]
			}
			if wide {
				if len(b) < 2 {
					return out, rich
				}
				units = append(units, le.Uint16(b))
				b = b[2:]
			} else {
				units = append(units, uint16(b[0]))
				b = b[1:]
			}
		}
		out = append(out, string(utf16.Decode(units)))
		if !take(after) {
			break
		}
	}
	return out, rich
}

// rk is a number packed in four bytes: an integer or the high half of a
// float, possibly a hundred times what it stands for.
func rk(v uint32) float64 {
	var f float64
	if v&2 != 0 {
		f = float64(int32(v) >> 2)
	} else {
		f = math.Float64frombits(uint64(v&^3) << 32)
	}
	if v&1 != 0 {
		f /= 100
	}
	return f
}

// errorNames are the errors of a cell, by their code.
var errorNames = map[byte]string{0x00: "#NULL!", 0x07: "#DIV/0!", 0x0F: "#VALUE!", 0x17: "#REF!", 0x1D: "#NAME?", 0x24: "#NUM!", 0x2A: "#N/A"}

func errorName(code byte) string {
	if name, ok := errorNames[code]; ok {
		return name
	}
	return "#N/A"
}
