package drawingml

import (
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Xfrm places a shape, in EMU; a group also maps its children's coordinates
// (cx, cy, cw, ch) onto its own box.
type Xfrm struct {
	X     int64  `json:"x"`
	Y     int64  `json:"y"`
	W     int64  `json:"w"`
	H     int64  `json:"h"`
	Rot   int64  `json:"rot,omitempty"`
	FlipH bool   `json:"flipH,omitempty"`
	FlipV bool   `json:"flipV,omitempty"`
	CX    *int64 `json:"cx,omitempty"`
	CY    *int64 `json:"cy,omitempty"`
	CW    *int64 `json:"cw,omitempty"`
	CH    *int64 `json:"ch,omitempty"`
}

// ReadXfrm reads an a:xfrm or p:xfrm, nil when it does not place anything.
func ReadXfrm(e *xmldom.Element) *Xfrm {
	off, ext := child(e, "off"), child(e, "ext")
	if off == nil || ext == nil {
		return nil
	}
	x := &Xfrm{FlipH: flag(e.Get("flipH")), FlipV: flag(e.Get("flipV"))}
	x.X, _ = number(off.Get("x"))
	x.Y, _ = number(off.Get("y"))
	x.W, _ = number(ext.Get("cx"))
	x.H, _ = number(ext.Get("cy"))
	x.Rot, _ = number(e.Get("rot"))
	if o, x2 := child(e, "chOff"), child(e, "chExt"); o != nil && x2 != nil {
		x.CX, x.CY, x.CW, x.CH = new(int64), new(int64), new(int64), new(int64)
		*x.CX, _ = number(o.Get("x"))
		*x.CY, _ = number(o.Get("y"))
		*x.CW, _ = number(x2.Get("cx"))
		*x.CH, _ = number(x2.Get("cy"))
	}
	return x
}

const maxCoordinate = 51206400 * 8

// SetXfrm patches e, an xfrm element, to place the shape as x does.
func SetXfrm(e *xmldom.Element, x *Xfrm) {
	coordinate := func(v int64) string { return strconv.FormatInt(min(max(v, -maxCoordinate), maxCoordinate), 10) }
	extent := func(v int64) string { return strconv.FormatInt(min(max(v, 0), maxCoordinate), 10) }
	pair := func(local, a, b, va, vb string) {
		c := child(e, local)
		if c == nil {
			c = newA(local)
			e.Insert(c, []string{"off", "ext", "chOff", "chExt"})
		}
		c.Set(a, va)
		c.Set(b, vb)
	}
	if x.Rot%21600000 != 0 {
		e.Set("rot", strconv.FormatInt(x.Rot%21600000, 10))
	} else {
		e.Unset("rot")
	}
	for _, f := range []struct {
		attr string
		on   bool
	}{{"flipH", x.FlipH}, {"flipV", x.FlipV}} {
		if f.on {
			e.Set(f.attr, "1")
		} else {
			e.Unset(f.attr)
		}
	}
	pair("off", "x", "y", coordinate(x.X), coordinate(x.Y))
	pair("ext", "cx", "cy", extent(x.W), extent(x.H))
	if x.CX != nil && x.CY != nil && x.CW != nil && x.CH != nil {
		pair("chOff", "x", "y", coordinate(*x.CX), coordinate(*x.CY))
		pair("chExt", "cx", "cy", extent(*x.CW), extent(*x.CH))
	}
}
