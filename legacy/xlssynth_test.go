package legacy

import (
	"encoding/binary"
	"math"
	"slices"
	"testing"
	"unicode/utf16"

	"github.com/citadellefr/loffice/xlsx"
	"github.com/citadellefr/trame/ot"
)

// The workbooks of these tests are made of their bytes, so that they run
// without the corpus.

func rc(id uint16, parts ...[]byte) []byte {
	var data []byte
	for _, p := range parts {
		data = append(data, p...)
	}
	return append(binary.LittleEndian.AppendUint16(binary.LittleEndian.AppendUint16(nil, id), uint16(len(data))), data...)
}

func words(v ...int) []byte {
	var b []byte
	for _, n := range v {
		b = binary.LittleEndian.AppendUint16(b, uint16(n))
	}
	return b
}

func dword(n uint32) []byte { return binary.LittleEndian.AppendUint32(nil, n) }

// short and long are texts of one byte a character, after their length in
// a byte or in two, and their flag.
func short(s string) []byte { return concat([]byte{byte(len(s)), 0}, []byte(s)) }

func long(s string) []byte { return concat(words(len(s)), []byte{0}, []byte(s)) }

// bounded is the workbook of one worksheet whose records are these.
func bounded(sheet ...[]byte) []byte {
	globals := func(offset uint32) []byte {
		return concat(
			rc(recBOF, words(biff8, bofGlobals, 0x0DBB, 0x07CC), dword(0), dword(6)),
			rc(recBoundSheet, dword(offset), []byte{0, 0}, short("S")),
			rc(recEOF))
	}
	stream := globals(0)
	stream = globals(uint32(len(stream)))
	stream = append(stream, rc(recBOF, words(biff8, 0x0010, 0x0DBB, 0x07CC), dword(0), dword(6))...)
	stream = append(stream, concat(sheet...)...)
	stream = append(stream, rc(recEOF)...)
	return compoundOf(stream)
}

func concat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

// compoundOf holds a stream named Workbook in a Compound File Binary
// container of 512-byte sectors: the table, the directory, the stream, that
// is padded to the size from which a stream is not kept in the mini stream.
func compoundOf(stream []byte) []byte {
	const sector = 512
	stream = append(stream, make([]byte, max(4096-len(stream), 0))...)
	n := (len(stream) + sector - 1) / sector
	stream = append(stream, make([]byte, n*sector-len(stream))...)

	header := make([]byte, sector)
	copy(header, cfbMagic)
	binary.LittleEndian.PutUint16(header[24:], 0x3E)
	binary.LittleEndian.PutUint16(header[26:], 3)
	binary.LittleEndian.PutUint16(header[28:], 0xFFFE)
	binary.LittleEndian.PutUint16(header[30:], 9)
	binary.LittleEndian.PutUint16(header[32:], 6)
	binary.LittleEndian.PutUint32(header[44:], 1)
	binary.LittleEndian.PutUint32(header[48:], 1)
	binary.LittleEndian.PutUint32(header[56:], 4096)
	binary.LittleEndian.PutUint32(header[60:], endOfChain)
	binary.LittleEndian.PutUint32(header[68:], endOfChain)
	for i := range 109 {
		binary.LittleEndian.PutUint32(header[76+4*i:], noEntry)
	}
	binary.LittleEndian.PutUint32(header[76:], 0)

	fat := make([]byte, sector)
	for i := range sector / 4 {
		binary.LittleEndian.PutUint32(fat[4*i:], noEntry)
	}
	binary.LittleEndian.PutUint32(fat, 0xFFFFFFFD)
	binary.LittleEndian.PutUint32(fat[4:], endOfChain)
	for i := range n {
		next := uint32(3 + i)
		if i == n-1 {
			next = endOfChain
		}
		binary.LittleEndian.PutUint32(fat[4*(2+i):], next)
	}

	dir := make([]byte, sector)
	entry := func(i int, name string, kind byte, child uint32, start uint32, size uint64) {
		e := dir[128*i : 128*(i+1)]
		units := utf16.Encode([]rune(name))
		for j, u := range units {
			binary.LittleEndian.PutUint16(e[2*j:], u)
		}
		if name != "" {
			binary.LittleEndian.PutUint16(e[64:], uint16(2*len(units)+2))
		}
		e[66], e[67] = kind, 1
		binary.LittleEndian.PutUint32(e[68:], noEntry)
		binary.LittleEndian.PutUint32(e[72:], noEntry)
		binary.LittleEndian.PutUint32(e[76:], child)
		binary.LittleEndian.PutUint32(e[116:], start)
		binary.LittleEndian.PutUint64(e[120:], size)
	}
	entry(0, "Root Entry", entryRoot, 1, endOfChain, 0)
	entry(1, "Workbook", entryStream, noEntry, 2, uint64(len(stream)))
	entry(2, "", 0, noEntry, 0, 0)
	entry(3, "", 0, noEntry, 0, 0)
	return concat(header, fat, dir, stream)
}

// synthetic is a workbook of these records once converted and read again.
func synthetic(t *testing.T, sheet ...[]byte) (*ot.Tree, []Loss) {
	t.Helper()
	book, lost, err := Workbook(bounded(sheet...), Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_, tree, err := xlsx.Open(book, xlsx.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	return tree, lost
}

func num(row, col int, v float64) []byte {
	return rc(recNumber, words(row, col, 0), binary.LittleEndian.AppendUint64(nil, math.Float64bits(v)))
}

func TestSyntheticCells(t *testing.T) {
	tree, lost := synthetic(t, num(0, 0, 42))
	if len(lost) != 0 {
		t.Errorf("lost %v", lost)
	}
	has(t, "A1", fields(t, sheetNamed(t, tree, "S"), "A1"), `"v":42`)
}

// cfRule is a rule of the cells A1:A3: a formula that holds, a red area, and
// the number format block that comes before it, with the word that tells
// whether it is a format written out.
func cfRule(flags uint32, user int, format []byte) []byte {
	formula := []byte{ptgBool, 1}
	return concat(
		rc(recCondFmt, words(1, 1, 0, 2, 0, 0, 1, 0, 2, 0, 0)),
		rc(recCF, []byte{cfRuleFormula, 0}, words(len(formula), 0), dword(flags|cfArea), words(user), format,
			words(1<<10, 0x0A), formula))
}

func TestSyntheticConditionalFormat(t *testing.T) {
	kg := long(`0.00 "kg"`)
	for name, test := range map[string]struct {
		rule []byte
		want string
	}{
		"built-in": {cfRule(cfNumber, 0, []byte{0, 9}), `"fmt":"0%","fill":{"pattern":"solid","fg":{"rgb":"FF0000"}}`},
		"custom":   {cfRule(cfNumber, cfUserFormat, concat(words(len(kg)+2), kg)), `"fmt":"0.00 \"kg\"","fill"`},
		"kept":     {cfRule(cfNumber|cfNoFormat, 0, []byte{0, 9}), `"style":{"fill"`},
		"general":  {cfRule(cfNumber, 0, []byte{0, 0}), `"style":{"fill"`},
		"none":     {cfRule(0, 0, nil), `"style":{"fill"`},
	} {
		t.Run(name, func(t *testing.T) {
			tree, lost := synthetic(t, num(0, 0, 1), test.rule)
			kept(t, lost, Conditional)
			has(t, "cf", string(sheetNamed(t, tree, "S").Attrs["cf"]), `"ref":"A1:A3","type":"formula"`, `"f":["TRUE"]`, test.want)
		})
	}
}

func dv(flags uint32, f1 []byte, ref [4]int) []byte {
	return rc(recValidation, dword(flags), long("\x00"), long("\x00"), long("\x00"), long("\x00"),
		words(len(f1), 0), f1, words(0, 0), words(1), words(ref[0], ref[1], ref[2], ref[3]))
}

func listOf(items string) []byte {
	return concat([]byte{ptgStr}, short(items))
}

// A list written out in a validation has the values Excel and LibreOffice
// show: cut at the nulls, without the spaces that begin them.
func TestSyntheticValidationList(t *testing.T) {
	const flags = dvList | dvBlank | 0x03
	tests := []struct{ items, want string }{
		{"a\x00 b \x00c", `"f1":"\"a,b ,c\""`},
		{"  x\x00\x00y", `"f1":"\"x,,y\""`},
		{"Dairy\x00Produce", `"f1":"\"Dairy,Produce\""`},
	}
	for _, test := range tests {
		tree, lost := synthetic(t, num(0, 0, 1), rc(recValidations, words(0, 0, 0, 0), dword(0), dword(1)),
			dv(flags, listOf(test.items), [4]int{0, 0, 0, 0}))
		kept(t, lost, Validation)
		has(t, "valid", string(sheetNamed(t, tree, "S").Attrs["valid"]), `"ref":"A1","type":"list"`, test.want)
	}
	_, lost := synthetic(t, num(0, 0, 1), dv(flags, listOf("a,b\x00c"), [4]int{0, 0, 0, 0}))
	if !slices.Contains(lost, Validation) {
		t.Errorf("a value with a comma is not told lost: %v", lost)
	}
}

func hlink(area [4]int, flags uint32, parts ...[]byte) []byte {
	return rc(recHyperlink, words(area[0], area[1], area[2], area[3]), make([]byte, 16), dword(2), dword(flags), concat(parts...))
}

func TestSyntheticLink(t *testing.T) {
	url := utf16le("http://example.com/a\x00")
	tree, lost := synthetic(t, num(0, 0, 1),
		hlink([4]int{0, 0, 0, 0}, linkMoniker|linkDisplay, dword(3), utf16le("go\x00"), urlMoniker, dword(uint32(len(url))), url),
		rc(recLinkTip, words(0x0800), words(0, 0, 0, 0), utf16le("tip\x00")),
		hlink([4]int{1, 1, 0, 0}, linkLocation, dword(5), utf16le("S!B2\x00")))
	kept(t, lost, Links)
	sheet := sheetNamed(t, tree, "S")
	has(t, "A1", fields(t, sheet, "A1"), `"link":{"url":"http://example.com/a"`)
	has(t, "A2", fields(t, sheet, "A2"), `"link":{"loc":"S!B2"}`)
}

func utf16le(s string) []byte {
	var b []byte
	for _, u := range utf16.Encode([]rune(s)) {
		b = binary.LittleEndian.AppendUint16(b, u)
	}
	return b
}

func TestSyntheticNote(t *testing.T) {
	const text = "Check this"
	tree, lost := synthetic(t, num(0, 0, 1),
		rc(recObj, words(0x15, 0x12, objNote, 1, 0x4011), make([]byte, 12), words(0, 0)),
		rc(recTxo, words(0x212, 0, 0, 0, 0, len(text), 16, 0, 0)),
		rc(recContinue, []byte{0}, []byte(text)),
		rc(recContinue, make([]byte, 16)),
		rc(recNote, words(0, 0, 0, 1), long("Jane")))
	kept(t, lost, Comments)
	has(t, "A1", fields(t, sheetNamed(t, tree, "S"), "A1"), `"author":"Jane"`, text)
}
