package legacy

import (
	"bytes"
	"unicode/utf16"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/xlsx"
)

// recLinkTip is the record that tells what is shown over a link.
const recLinkTip = 0x0800

// maxLinkArea is the most cells a link is given to, one at a time.
const maxLinkArea = 1024

// The flags of a link that tell what follows its header, and the class of
// the moniker that tells an address of the web.
const (
	linkMoniker  = 0x01
	linkLocation = 0x08
	linkDisplay  = 0x10
	linkFrame    = 0x80
	linkAsString = 0x100
)

var urlMoniker = []byte{0xE0, 0xC9, 0xEA, 0x79, 0xF9, 0xBA, 0xCE, 0x11, 0x8C, 0x82, 0x00, 0xAA, 0x00, 0x4B, 0xA9, 0x0B}

// areaLink is a link over an area, its rows then its columns from 0.
type areaLink struct {
	area [4]int
	link xlsx.Link
}

// cursor reads what a record holds, and tells when it is too short.
type cursor struct {
	b      []byte
	failed bool
}

func (c *cursor) take(n int) []byte {
	if n < 0 || n > len(c.b) {
		c.failed = true
		c.b = nil
		return nil
	}
	got := c.b[:n]
	c.b = c.b[n:]
	return got
}

func (c *cursor) uint16() int {
	if b := c.take(2); b != nil {
		return int(le.Uint16(b))
	}
	return 0
}

func (c *cursor) uint32() int {
	if b := c.take(4); b != nil {
		return int(le.Uint32(b))
	}
	return 0
}

// wide reads a string of n UTF-16 units that ends where a null does.
func (c *cursor) wide(n int) string {
	return (&cursor{b: c.take(2 * n)}).rest()
}

// rest reads what is left as a string that ends where a null does.
func (c *cursor) rest() string {
	units := make([]uint16, 0, len(c.b)/2)
	for b := c.b; len(b) >= 2 && le.Uint16(b) != 0; b = b[2:] {
		units = append(units, le.Uint16(b))
	}
	return string(utf16.Decode(units))
}

// areaOf reads the cells of a link or of a tip: the first row, the last, the
// first column, the last.
func areaOf(d []byte) ([4]int, bool) {
	if len(d) < 8 {
		return [4]int{}, false
	}
	a := [4]int{int(le.Uint16(d)), int(le.Uint16(d[2:])), int(le.Uint16(d[4:])), int(le.Uint16(d[6:]))}
	return a, a[1] >= a[0] && a[3] >= a[2] && a[3] < biffCols
}

// hyperlink reads a link over an area. Those that lead to a file, or
// whose address a document of today would not take, are left out and told.
func (s *sheet) hyperlink(d []byte) {
	area, ok := areaOf(d)
	if !ok || len(d) < 32 {
		s.b.lose(Links)
		return
	}
	flags := int(le.Uint32(d[28:]))
	c := &cursor{b: d[32:]}
	if flags&linkDisplay != 0 {
		c.wide(c.uint32())
	}
	if flags&linkFrame != 0 {
		c.wide(c.uint32())
	}
	var l xlsx.Link
	if flags&linkMoniker != 0 {
		switch {
		case flags&linkAsString != 0:
			l.URL = c.wide(c.uint32())
		case bytes.Equal(c.take(16), urlMoniker):
			l.URL = (&cursor{b: c.take(c.uint32())}).rest()
		default:
			c.failed = true
		}
	}
	if flags&linkLocation != 0 {
		l.Loc = c.wide(c.uint32())
	}
	if c.failed || !l.Valid() || l.URL != "" && partrel.LinkTarget(l.URL) != l.URL ||
		(area[1]-area[0]+1)*(area[3]-area[2]+1) > maxLinkArea {
		s.b.lose(Links)
		return
	}
	s.links = append(s.links, areaLink{area: area, link: l})
}

// linkTip reads what is shown over a link, given for its area.
func (s *sheet) linkTip(d []byte) {
	if len(d) < 12 {
		return
	}
	if area, ok := areaOf(d[4:]); ok {
		if s.tips == nil {
			s.tips = map[[4]int]string{}
		}
		s.tips[area] = (&cursor{b: d[12:]}).rest()
	}
}

// putLinks gives the cells their links.
func (s *sheet) putLinks() {
	for _, l := range s.links {
		l.link.Tip = s.tips[l.area]
		if !l.link.Valid() {
			l.link.Tip = ""
		}
		for row := l.area[0]; row <= l.area[1]; row++ {
			for col := l.area[2]; col <= l.area[3]; col++ {
				link := l.link
				s.get([2]int{row + 1, col + 1}).Link = &link
			}
		}
	}
}
