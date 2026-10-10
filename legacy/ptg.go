package legacy

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/formula"
)

var (
	errFormula  = errors.New("legacy: formula not understood")
	errExternal = errors.New("legacy: formula reads another workbook")
)

// The tokens of a formula. Those from ptgArray on come in three classes,
// told by two more bits, that read the same.
const (
	ptgExp      = 0x01
	ptgAdd      = 0x03
	ptgRange    = 0x11
	ptgUplus    = 0x12
	ptgUminus   = 0x13
	ptgPercent  = 0x14
	ptgParen    = 0x15
	ptgMissArg  = 0x16
	ptgStr      = 0x17
	ptgAttr     = 0x19
	ptgErr      = 0x1C
	ptgBool     = 0x1D
	ptgInt      = 0x1E
	ptgNum      = 0x1F
	ptgArray    = 0x20
	ptgFunc     = 0x21
	ptgFuncVar  = 0x22
	ptgName     = 0x23
	ptgRef      = 0x24
	ptgArea     = 0x25
	ptgMemArea  = 0x26
	ptgMemErr   = 0x27
	ptgMemNoMem = 0x28
	ptgMemFunc  = 0x29
	ptgRefErr   = 0x2A
	ptgAreaErr  = 0x2B
	ptgRefN     = 0x2C
	ptgAreaN    = 0x2D
	ptgNameX    = 0x39
	ptgRef3d    = 0x3A
	ptgArea3d   = 0x3B
	ptgRefErr3d = 0x3C
	ptgAreaErr3 = 0x3D

	attrChoose = 0x04
	attrSum    = 0x10

	userFunction = 255

	biffRows = 1 << 16
	biffCols = 1 << 8
)

// operators are the binary operators, from ptgAdd to ptgRange.
var operators = [...]string{"+", "-", "*", "/", "^", "&", "<", "<=", "=", ">=", ">", "<>", " ", ",", ":"}

// origin is the cell the relative references of a formula are counted
// from, on a sheet of rows by cols that they wrap around: the cell of a
// shared formula or, for a name, A1 of the sheets of today. In a name the
// references to other sheets are relative too.
type origin struct {
	row, col   int
	rows, cols int
	name       bool
}

// cell is the cell a token points to, counted from 1, and whether its row
// and column are absolute.
func (o origin) cell(rw, col uint16, relative bool) (r, c int, absR, absC bool) {
	relR, relC := col&0x8000 != 0, col&0x4000 != 0
	r, c = int(rw), int(col&0xFF)
	if relative && relR {
		r = ((o.row+int(int16(rw)))%o.rows + o.rows) % o.rows
	}
	if relative && relC {
		c = ((o.col+int(int8(col)))%o.cols + o.cols) % o.cols
	}
	return r + 1, c + 1, !relR, !relC
}

func (o origin) ref(b []byte, relative bool) formula.Area {
	r, c, absR, absC := o.cell(le.Uint16(b), le.Uint16(b[2:]), relative)
	return formula.Area{R1: r, C1: c, R2: r, C2: c, AbsR1: absR, AbsC1: absC, AbsR2: absR, AbsC2: absC, Cell: true}
}

// area reads an area; one that spans the sheet of the file spans the
// sheet of today.
func (o origin) area(b []byte, relative bool) formula.Area {
	var a formula.Area
	a.R1, a.C1, a.AbsR1, a.AbsC1 = o.cell(le.Uint16(b), le.Uint16(b[4:]), relative)
	a.R2, a.C2, a.AbsR2, a.AbsC2 = o.cell(le.Uint16(b[2:]), le.Uint16(b[6:]), relative)
	switch rows, cols := a.R1 == 1 && a.R2 == biffRows, a.C1 == 1 && a.C2 == biffCols; {
	case rows && cols:
		a.Rows, a.R2 = true, formula.MaxRows
	case rows:
		a.Cols = true
	case cols:
		a.Rows = true
	}
	return a
}

// sheetRef is the reference to the sheets an index of the table of
// references names: none for the workbook itself, an invalid one when
// they were deleted.
func (b *book) sheetRef(index uint16) (formula.Ref, error) {
	var ref formula.Ref
	if int(index) >= len(b.externs) {
		return ref, errFormula
	}
	x := b.externs[index]
	if int(x.book) >= len(b.supbooks) {
		return ref, errFormula
	}
	switch b.supbooks[x.book].kind {
	case supExternal:
		return ref, errExternal
	case supAddIn:
		return ref, errFormula
	}
	switch {
	case x.first == 0xFFFE:
	case int(x.first) >= len(b.sheets) || int(x.last) >= len(b.sheets):
		ref.Invalid = true
	default:
		ref.Sheet = b.sheets[x.first].name
		if x.last != x.first {
			ref.LastSheet = b.sheets[x.last].name
		}
	}
	return ref, nil
}

// nameX is the name a formula reaches through the table of references:
// one of the workbook, or a function of an add-in.
func (b *book) nameX(index, name uint16) (string, error) {
	if int(index) >= len(b.externs) || int(b.externs[index].book) >= len(b.supbooks) || name == 0 {
		return "", errFormula
	}
	switch sup := b.supbooks[b.externs[index].book]; {
	case sup.kind == supExternal:
		return "", errExternal
	case sup.kind == supAddIn && int(name) <= len(sup.names):
		return sup.names[name-1], nil
	case sup.kind == supInternal && int(name) <= len(b.names):
		return b.names[name-1].name, nil
	}
	return "", errFormula
}

// formula writes the tokens of a formula as text, without "=". extra is
// what follows the tokens: the constants of its arrays.
func (b *book) formula(code, extra []byte, o origin) (string, error) {
	var stack []string
	push := func(s string) { stack = append(stack, s) }
	pop := func(n int) ([]string, bool) {
		if n > len(stack) {
			return nil, false
		}
		args := stack[len(stack)-n:]
		stack = stack[:len(stack)-n]
		return args, true
	}
	call := func(name string, n int) bool {
		args, ok := pop(n)
		if ok && name != "" {
			push(name + "(" + strings.Join(args, ",") + ")")
		}
		return ok && name != ""
	}
	for len(code) > 0 {
		ptg := code[0]
		if ptg >= 0x40 {
			ptg = ptg&0x1F | 0x20
		}
		code = code[1:]
		size, ok := ptgSizes[ptg]
		if !ok || len(code) < size {
			return "", errFormula
		}
		arg := code[:size]
		code = code[size:]
		switch {
		case ptg >= ptgAdd && ptg <= ptgRange:
			args, ok := pop(2)
			if !ok {
				return "", errFormula
			}
			push(args[0] + operators[ptg-ptgAdd] + args[1])
		case ptg == ptgUplus || ptg == ptgUminus || ptg == ptgPercent || ptg == ptgParen:
			args, ok := pop(1)
			if !ok {
				return "", errFormula
			}
			switch ptg {
			case ptgUplus:
				push("+" + args[0])
			case ptgUminus:
				push("-" + args[0])
			case ptgPercent:
				push(args[0] + "%")
			case ptgParen:
				push("(" + args[0] + ")")
			}
		case ptg == ptgMissArg:
			push("")
		case ptg == ptgStr:
			s, n, _ := text(code, int(arg[0]))
			if n == 0 {
				return "", errFormula
			}
			code = code[n:]
			push(quoted(s))
		case ptg == ptgAttr:
			switch {
			case arg[0]&attrChoose != 0:
				jumps := 2 * (int(le.Uint16(arg[1:])) + 1)
				if len(code) < jumps {
					return "", errFormula
				}
				code = code[jumps:]
			case arg[0]&attrSum != 0:
				if !call("SUM", 1) {
					return "", errFormula
				}
			}
		case ptg == ptgErr:
			push(errorName(arg[0]))
		case ptg == ptgBool:
			push(boolean(arg[0]))
		case ptg == ptgInt:
			push(strconv.Itoa(int(le.Uint16(arg))))
		case ptg == ptgNum:
			push(number(math.Float64frombits(le.Uint64(arg))))
		case ptg == ptgArray:
			s, n := arrayConstant(extra)
			if n == 0 {
				return "", errFormula
			}
			extra = extra[n:]
			push(s)
		case ptg == ptgFunc:
			f := functions[le.Uint16(arg)]
			if f.args < 0 || !call(f.name, f.args) {
				return "", errFormula
			}
		case ptg == ptgFuncVar:
			n, index := int(arg[0]&0x7F), le.Uint16(arg[1:])
			name := functions[index].name
			if index == userFunction && n >= 1 && n <= len(stack) {
				// the function of an add-in is named by its first argument
				name = stack[len(stack)-n]
				if short := strings.TrimPrefix(name, futurePrefix); builtinSince2007[short] {
					name = short
				}
				stack = append(stack[:len(stack)-n], stack[len(stack)-n+1:]...)
				n--
			}
			if !call(name, n) {
				return "", errFormula
			}
		case ptg == ptgName:
			i := int(le.Uint16(arg))
			if i == 0 || i > len(b.names) || b.names[i-1].name == "" {
				return "", errFormula
			}
			push(b.names[i-1].name)
		case ptg == ptgNameX:
			name, err := b.nameX(le.Uint16(arg), le.Uint16(arg[2:]))
			if err != nil {
				return "", err
			}
			push(name)
		case ptg == ptgRef || ptg == ptgRefN:
			push(o.ref(arg, ptg == ptgRefN).String())
		case ptg == ptgArea || ptg == ptgAreaN:
			push(o.area(arg, ptg == ptgAreaN).String())
		case ptg == ptgRefErr || ptg == ptgAreaErr:
			push("#REF!")
		case ptg == ptgRef3d || ptg == ptgArea3d || ptg == ptgRefErr3d || ptg == ptgAreaErr3:
			ref, err := b.sheetRef(le.Uint16(arg))
			if err != nil {
				return "", err
			}
			switch ptg {
			case ptgRef3d:
				ref.Area = o.ref(arg[2:], o.name)
			case ptgArea3d:
				ref.Area = o.area(arg[2:], o.name)
			default:
				ref.Invalid = true
			}
			if ref.Invalid {
				ref.Sheet, ref.LastSheet = "", ""
			}
			push(ref.String())
		case ptg == ptgMemArea:
			// the areas it caches follow the tokens
			if len(extra) < 2 || len(extra) < 2+8*int(le.Uint16(extra)) {
				return "", errFormula
			}
			extra = extra[2+8*int(le.Uint16(extra)):]
		case ptg == ptgMemErr || ptg == ptgMemNoMem || ptg == ptgMemFunc:
		default:
			return "", errFormula
		}
	}
	if len(stack) != 1 || stack[0] == "" || len(stack[0]) > maxFormula {
		return "", errFormula
	}
	if _, err := formula.Tokens(stack[0]); err != nil {
		return "", errFormula
	}
	return stack[0], nil
}

// A function Excel gained after 2003 is written as one of an add-in whose
// name starts with futurePrefix. An .xlsx names those of 2007 without it.
const futurePrefix = "_xlfn."

var builtinSince2007 = map[string]bool{"AVERAGEIF": true, "AVERAGEIFS": true, "COUNTIFS": true, "SUMIFS": true,
	"IFERROR": true, "CUBEKPIMEMBER": true, "CUBEMEMBER": true, "CUBEMEMBERPROPERTY": true,
	"CUBERANKEDMEMBER": true, "CUBESET": true, "CUBESETCOUNT": true, "CUBEVALUE": true}

// maxFormula is the longest formula Excel takes.
const maxFormula = 8192

// ptgSizes are the bytes that follow each token read.
var ptgSizes = map[byte]int{
	ptgAdd: 0, 0x04: 0, 0x05: 0, 0x06: 0, 0x07: 0, 0x08: 0, 0x09: 0, 0x0A: 0, 0x0B: 0, 0x0C: 0, 0x0D: 0, 0x0E: 0,
	0x0F: 0, 0x10: 0, ptgRange: 0, ptgUplus: 0, ptgUminus: 0, ptgPercent: 0, ptgParen: 0, ptgMissArg: 0,
	ptgStr: 1, ptgAttr: 3, ptgErr: 1, ptgBool: 1, ptgInt: 2, ptgNum: 8,
	ptgArray: 7, ptgFunc: 2, ptgFuncVar: 3, ptgName: 4, ptgRef: 4, ptgArea: 8,
	ptgMemArea: 6, ptgMemErr: 6, ptgMemNoMem: 6, ptgMemFunc: 2, ptgRefErr: 4, ptgAreaErr: 8,
	ptgRefN: 4, ptgAreaN: 8, ptgNameX: 6, ptgRef3d: 6, ptgArea3d: 10, ptgRefErr3d: 6, ptgAreaErr3: 10,
}

func quoted(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func boolean(b byte) string {
	if b != 0 {
		return "TRUE"
	}
	return "FALSE"
}

func number(f float64) string {
	return strconv.FormatFloat(f, 'G', -1, 64)
}

// arrayConstant reads the constant of an array, written {1,2;3,4}; size
// is what it takes of b, 0 when b is too short.
func arrayConstant(b []byte) (s string, size int) {
	if len(b) < 3 {
		return "", 0
	}
	cols, rows := int(b[0])+1, int(le.Uint16(b[1:]))+1
	pos := 3
	var out strings.Builder
	out.WriteByte('{')
	for i := range rows * cols {
		switch {
		case i == 0:
		case i%cols == 0:
			out.WriteByte(';')
		default:
			out.WriteByte(',')
		}
		if len(b) < pos+1 {
			return "", 0
		}
		if b[pos] == 0x02 {
			text, n, _ := longText(b[pos+1:])
			if n == 0 {
				return "", 0
			}
			out.WriteString(quoted(text))
			pos += 1 + n
			continue
		}
		if len(b) < pos+9 {
			return "", 0
		}
		switch b[pos] {
		case 0x00:
		case 0x01:
			out.WriteString(number(math.Float64frombits(le.Uint64(b[pos+1:]))))
		case 0x04:
			out.WriteString(boolean(b[pos+1]))
		case 0x10:
			out.WriteString(errorName(b[pos+1]))
		default:
			return "", 0
		}
		pos += 9
	}
	out.WriteByte('}')
	return out.String(), pos
}
