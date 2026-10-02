package drawingml

import (
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Fill is how an area is painted: exactly one of its kinds.
type Fill struct {
	None  bool      `json:"none,omitempty"`
	Solid *Color    `json:"solid,omitempty"`
	Group bool      `json:"grp,omitempty"`
	Grad  *Gradient `json:"grad,omitempty"`
	Patt  *Pattern  `json:"patt,omitempty"`
	Blip  *Blip     `json:"blip,omitempty"`
}

type Gradient struct {
	Stops []Stop `json:"stops"`
	// Lin is the angle of a linear gradient, in 60000ths of a degree.
	Lin    *int64 `json:"lin,omitempty"`
	Scaled bool   `json:"scaled,omitempty"`
	// Path is "circle", "rect" or "shape" for a gradient from a focus,
	// Focus the rectangle it grows from, in 1000ths of a percent.
	Path  string  `json:"path,omitempty"`
	Focus []int64 `json:"focus,omitempty"`
}

// Stop is a color at a position along a gradient, in 1000ths of a percent,
// written [pos, color].
type Stop struct {
	Pos   int64
	Color Color
}

func (s Stop) MarshalJSON() ([]byte, error) {
	return marshalPair(s.Pos, s.Color)
}

func (s *Stop) UnmarshalJSON(data []byte) error {
	return unmarshalPair(data, &s.Pos, &s.Color)
}

type Pattern struct {
	Preset string `json:"prst"`
	FG     *Color `json:"fg,omitempty"`
	BG     *Color `json:"bg,omitempty"`
}

// Blip is a picture, by the media name the document gives it.
type Blip struct {
	Media string `json:"media"`
	// Rect crops the picture, in 1000ths of a percent from each side.
	Rect []int64 `json:"rect,omitempty"`
	Tile bool    `json:"tile,omitempty"`
	// Alpha is the opacity, in 1000ths of a percent, when not opaque.
	Alpha *int64 `json:"alpha,omitempty"`
}

var fillKinds = []string{"noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill"}

// FillIn is the fill among the children of e, nil when there is none or it
// is not understood. media names the picture a relationship id points to.
func FillIn(e *xmldom.Element, media func(rid string) string) *Fill {
	for _, c := range children(e) {
		switch c.Local {
		case "noFill":
			return &Fill{None: true}
		case "grpFill":
			return &Fill{Group: true}
		case "solidFill":
			if color := ColorIn(c); color != nil {
				return &Fill{Solid: color}
			}
			return nil
		case "gradFill":
			if g := readGradient(c); g != nil {
				return &Fill{Grad: g}
			}
			return nil
		case "pattFill":
			return &Fill{Patt: &Pattern{Preset: c.Get("prst"), FG: ColorIn(child(c, "fgClr")), BG: ColorIn(child(c, "bgClr"))}}
		case "blipFill":
			if b := ReadBlip(c, media); b != nil {
				return &Fill{Blip: b}
			}
			return nil
		}
	}
	return nil
}

func readGradient(e *xmldom.Element) *Gradient {
	g := &Gradient{}
	for _, gs := range children(child(e, "gsLst")) {
		pos, ok := number(gs.Get("pos"))
		color := ColorIn(gs)
		if !ok || color == nil {
			return nil
		}
		g.Stops = append(g.Stops, Stop{pos, *color})
	}
	if len(g.Stops) == 0 {
		return nil
	}
	if lin := child(e, "lin"); lin != nil {
		if a, ok := number(lin.Get("ang")); ok {
			g.Lin = &a
		} else {
			g.Lin = new(int64)
		}
		g.Scaled = flag(lin.Get("scaled"))
	}
	if path := child(e, "path"); path != nil {
		g.Path = path.Get("path")
		g.Focus = rect(child(path, "fillToRect"))
	}
	return g
}

// ReadBlip reads a blipFill.
func ReadBlip(e *xmldom.Element, media func(rid string) string) *Blip {
	blip := child(e, "blip")
	if blip == nil || media == nil {
		return nil
	}
	b := &Blip{Media: media(blip.Get("r:embed"))}
	if b.Media == "" {
		return nil
	}
	b.Rect = rect(child(e, "srcRect"))
	b.Tile = child(e, "tile") != nil
	if a := child(blip, "alphaModFix"); a != nil {
		if amt, ok := number(a.Get("amt")); ok {
			b.Alpha = &amt
		}
	}
	return b
}

// rect reads the l, t, r and b of a relative rectangle, nil when all are 0.
func rect(e *xmldom.Element) []int64 {
	if e == nil {
		return nil
	}
	out := make([]int64, 4)
	zero := true
	for i, n := range []string{"l", "t", "r", "b"} {
		out[i], _ = number(e.Get(n))
		zero = zero && out[i] == 0
	}
	if zero {
		return nil
	}
	return out
}

// Element is the fill as XML; a picture fill needs embed to give the
// relationship id of its media, and is nil without one.
func (f *Fill) Element(embed func(media string) string) *xmldom.Element {
	switch {
	case f.None:
		return newA("noFill")
	case f.Group:
		return newA("grpFill")
	case f.Solid != nil:
		e := newA("solidFill")
		e.Append(f.Solid.Element())
		return e
	case f.Grad != nil && len(f.Grad.Stops) > 0:
		return f.Grad.element()
	case f.Patt != nil:
		prst := f.Patt.Preset
		if !patterns[prst] {
			prst = "pct50"
		}
		e := newA("pattFill", "prst", prst)
		for _, c := range []struct {
			name  string
			color *Color
		}{{"fgClr", f.Patt.FG}, {"bgClr", f.Patt.BG}} {
			if c.color != nil {
				ce := newA(c.name)
				ce.Append(c.color.Element())
				e.Append(ce)
			}
		}
		return e
	case f.Blip != nil && embed != nil:
		rid := embed(f.Blip.Media)
		if rid == "" {
			return nil
		}
		e := newA("blipFill")
		blip := xmldom.New(NS, "a:blip", "xmlns:r", RelNS, "r:embed", rid)
		if f.Blip.Alpha != nil {
			blip.Append(newA("alphaModFix", "amt", strconv.FormatInt(*f.Blip.Alpha, 10)))
		}
		e.Append(blip)
		if len(f.Blip.Rect) == 4 {
			e.Append(relativeRect("srcRect", f.Blip.Rect))
		}
		if f.Blip.Tile {
			e.Append(newA("tile"))
		} else {
			stretch := newA("stretch")
			stretch.Append(newA("fillRect"))
			e.Append(stretch)
		}
		return e
	}
	return nil
}

func (g *Gradient) element() *xmldom.Element {
	e := newA("gradFill", "rotWithShape", "1")
	list := newA("gsLst")
	for _, s := range g.Stops {
		gs := newA("gs", "pos", strconv.FormatInt(min(max(s.Pos, 0), 100000), 10))
		gs.Append(s.Color.Element())
		list.Append(gs)
	}
	e.Append(list)
	switch {
	case g.Path == "circle" || g.Path == "rect" || g.Path == "shape":
		path := newA("path", "path", g.Path)
		if len(g.Focus) == 4 {
			path.Append(relativeRect("fillToRect", g.Focus))
		}
		e.Append(path)
	case g.Lin != nil:
		lin := newA("lin", "ang", strconv.FormatInt(*g.Lin, 10), "scaled", "0")
		if g.Scaled {
			lin.Set("scaled", "1")
		}
		e.Append(lin)
	}
	return e
}

func relativeRect(local string, r []int64) *xmldom.Element {
	e := newA(local)
	for i, n := range []string{"l", "t", "r", "b"} {
		if r[i] != 0 {
			e.Set(n, strconv.FormatInt(r[i], 10))
		}
	}
	return e
}

var patterns = map[string]bool{}

func init() {
	for _, p := range []string{
		"pct5", "pct10", "pct20", "pct25", "pct30", "pct40", "pct50", "pct60", "pct70", "pct75", "pct80", "pct90",
		"horz", "vert", "ltHorz", "ltVert", "dkHorz", "dkVert", "narHorz", "narVert", "dashHorz", "dashVert",
		"cross", "dnDiag", "upDiag", "ltDnDiag", "ltUpDiag", "dkDnDiag", "dkUpDiag", "wdDnDiag", "wdUpDiag",
		"dashDnDiag", "dashUpDiag", "diagCross", "smCheck", "lgCheck", "smGrid", "lgGrid", "dotGrid",
		"smConfetti", "lgConfetti", "horzBrick", "diagBrick", "solidDmnd", "openDmnd", "dotDmnd", "plaid",
		"sphere", "weave", "divot", "shingle", "wave", "trellis", "zigZag",
	} {
		patterns[p] = true
	}
}

// SetFill makes f the fill of e, whose children follow order; nil removes
// it.
func SetFill(e *xmldom.Element, f *Fill, embed func(string) string, order []string) {
	var c *xmldom.Element
	if f != nil {
		c = f.Element(embed)
	}
	setChild(e, fillKinds, c, order)
}
