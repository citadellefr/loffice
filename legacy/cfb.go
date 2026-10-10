package legacy

import (
	"bytes"
	"encoding/binary"
	"strings"
	"unicode/utf16"
)

var le = binary.LittleEndian

var cfbMagic = []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}

const (
	// lastSector is the last number a sector can have: those above mark
	// the end of a chain, a free sector, a sector of the table.
	lastSector = 0xFFFFFFFA
	endOfChain = 0xFFFFFFFE
	noEntry    = 0xFFFFFFFF
	miniSector = 64

	entryStorage = 1
	entryStream  = 2
	entryRoot    = 5
)

// compound is a Compound File Binary container, the file system in a file
// that the Office of before 2007 keeps its documents in: streams in
// storages, cut into sectors that a table chains.
type compound struct {
	data   []byte
	sector int
	fat    []uint32
	// streams shorter than cutoff are cut into mini sectors of the mini
	// stream, chained by the mini table.
	cutoff  uint64
	miniFAT []uint32
	mini    []byte
	entries []entry
}

type entry struct {
	name               string
	kind               byte
	left, right, child uint32
	start              uint32
	size               uint64
}

func openCompound(data []byte) (*compound, error) {
	if len(data) < 512 || !bytes.HasPrefix(data, cfbMagic) {
		return nil, ErrInvalid
	}
	shift := le.Uint16(data[30:])
	if shift != 9 && shift != 12 {
		return nil, ErrInvalid
	}
	c := &compound{data: data, sector: 1 << shift, cutoff: uint64(le.Uint32(data[56:]))}
	perSector := c.sector / 4
	maxSectors := len(data)/c.sector + 1

	// the sectors of the table are listed in the header, then in sectors
	// chained from it
	var tables []uint32
	for i := range 109 {
		tables = append(tables, le.Uint32(data[76+4*i:]))
	}
	for next, n := le.Uint32(data[68:]), 0; next <= lastSector; n++ {
		s := c.at(next)
		if s == nil || n > maxSectors {
			return nil, ErrInvalid
		}
		for i := range perSector - 1 {
			tables = append(tables, le.Uint32(s[4*i:]))
		}
		next = le.Uint32(s[4*(perSector-1):])
	}
	for _, t := range tables {
		if t > lastSector {
			continue
		}
		s := c.at(t)
		if s == nil || len(c.fat) > maxSectors {
			return nil, ErrInvalid
		}
		for i := range perSector {
			c.fat = append(c.fat, le.Uint32(s[4*i:]))
		}
	}

	dir, err := c.chain(le.Uint32(data[48:]))
	if err != nil {
		return nil, err
	}
	for ; len(dir) >= 128; dir = dir[128:] {
		e := entry{
			kind:  dir[66],
			left:  le.Uint32(dir[68:]),
			right: le.Uint32(dir[72:]),
			child: le.Uint32(dir[76:]),
			start: le.Uint32(dir[116:]),
			size:  le.Uint64(dir[120:]),
		}
		if shift == 9 {
			e.size &= 0xFFFFFFFF
		}
		if n := int(le.Uint16(dir[64:])); n >= 2 && n <= 64 {
			name := make([]uint16, n/2-1)
			for i := range name {
				name[i] = le.Uint16(dir[2*i:])
			}
			e.name = string(utf16.Decode(name))
		}
		c.entries = append(c.entries, e)
	}
	if len(c.entries) == 0 || c.entries[0].kind != entryRoot {
		return nil, ErrInvalid
	}

	if first := le.Uint32(data[60:]); first <= lastSector {
		table, err := c.chain(first)
		if err != nil {
			return nil, err
		}
		for ; len(table) >= 4; table = table[4:] {
			c.miniFAT = append(c.miniFAT, le.Uint32(table))
		}
		root := c.entries[0]
		if c.mini, err = c.read(root.start, root.size); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// at is a sector, nil when the file ends before it. The last one may be
// cut short: files are.
func (c *compound) at(sector uint32) []byte {
	off := (int64(sector) + 1) * int64(c.sector)
	if off >= int64(len(c.data)) {
		return nil
	}
	if end := off + int64(c.sector); end <= int64(len(c.data)) {
		return c.data[off:end]
	}
	return append(make([]byte, 0, c.sector), c.data[off:]...)[:c.sector]
}

// chain are the sectors chained from start, to the end of the chain.
func (c *compound) chain(start uint32) ([]byte, error) {
	var out []byte
	for s, n := start, 0; s != endOfChain; s, n = c.fat[s], n+1 {
		sector := c.at(s)
		if sector == nil || int64(s) >= int64(len(c.fat)) || n > len(c.fat) {
			return nil, ErrInvalid
		}
		out = append(out, sector...)
	}
	return out, nil
}

// read are the size bytes chained from start.
func (c *compound) read(start uint32, size uint64) ([]byte, error) {
	if size > uint64(len(c.data)) {
		return nil, ErrInvalid
	}
	out := make([]byte, 0, size)
	for s := start; uint64(len(out)) < size; s = c.fat[s] {
		sector := c.at(s)
		if sector == nil || int64(s) >= int64(len(c.fat)) {
			return nil, ErrInvalid
		}
		out = append(out, sector...)
	}
	return out[:size], nil
}

// find is the entry of that name in a storage, 0 when it has none. Names
// are compared as the format sorts them, without case.
func (c *compound) find(storage int, name string) int {
	todo := []uint32{c.entries[storage].child}
	for seen := 0; len(todo) > 0 && seen <= len(c.entries); seen++ {
		id := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if id == noEntry || int64(id) >= int64(len(c.entries)) {
			continue
		}
		e := c.entries[id]
		if strings.EqualFold(e.name, name) {
			return int(id)
		}
		todo = append(todo, e.left, e.right)
	}
	return 0
}

// stream reads a stream of the root storage; ok is false when there is
// none of that name.
func (c *compound) stream(name string) (data []byte, ok bool, err error) {
	id := c.find(0, name)
	if id == 0 || c.entries[id].kind != entryStream {
		return nil, false, nil
	}
	e := c.entries[id]
	if e.size >= c.cutoff {
		data, err = c.read(e.start, e.size)
		return data, true, err
	}
	if e.size > uint64(len(c.mini)) {
		return nil, true, ErrInvalid
	}
	data = make([]byte, 0, e.size)
	for s := e.start; uint64(len(data)) < e.size; s = c.miniFAT[s] {
		off := int64(s) * miniSector
		if int64(s) >= int64(len(c.miniFAT)) || off >= int64(len(c.mini)) {
			return nil, true, ErrInvalid
		}
		data = append(data, c.mini[off:min(off+miniSector, int64(len(c.mini)))]...)
	}
	return data[:e.size], true, nil
}
