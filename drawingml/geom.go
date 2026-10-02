package drawingml

import (
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Geometry is the outline of a shape: one of the preset shapes with its
// adjustments, or a custom one in the same terms the presets are defined in.
type Geometry struct {
	Preset string            `json:"prst,omitempty"`
	AV     map[string]string `json:"av,omitempty"`
	Custom *Custom           `json:"cust,omitempty"`
}

// Custom is a custom geometry: guides, each a name and a formula, the text
// rectangle, and paths whose coordinates are numbers or guide names.
type Custom struct {
	AV    []Guide  `json:"av,omitempty"`
	GD    []Guide  `json:"gd,omitempty"`
	Rect  []string `json:"rect,omitempty"`
	Paths []Path   `json:"paths"`
}

// Guide is written [name, formula].
type Guide struct {
	Name, Formula string
}

func (g Guide) MarshalJSON() ([]byte, error) {
	return marshalPair(g.Name, g.Formula)
}

func (g *Guide) UnmarshalJSON(data []byte) error {
	return unmarshalPair(data, &g.Name, &g.Formula)
}

type Path struct {
	W      int64  `json:"w,omitempty"`
	H      int64  `json:"h,omitempty"`
	Fill   string `json:"fill,omitempty"`
	Stroke *bool  `json:"stroke,omitempty"`
	// D are the commands: ["M",x,y] ["L",x,y] ["A",wR,hR,stAng,swAng]
	// ["Q",x1,y1,x,y] ["C",x1,y1,x2,y2,x,y] ["Z"].
	D [][]string `json:"d"`
}

var geometryKinds = []string{"custGeom", "prstGeom"}

// GeometryIn is the geometry among the children of e.
func GeometryIn(e *xmldom.Element) *Geometry {
	if p := child(e, "prstGeom"); p != nil {
		g := &Geometry{Preset: p.Get("prst")}
		for _, gd := range children(child(p, "avLst")) {
			if g.AV == nil {
				g.AV = map[string]string{}
			}
			g.AV[gd.Get("name")] = gd.Get("fmla")
		}
		return g
	}
	c := child(e, "custGeom")
	if c == nil {
		return nil
	}
	cust := &Custom{AV: guides(child(c, "avLst")), GD: guides(child(c, "gdLst")), Paths: []Path{}}
	if r := child(c, "rect"); r != nil {
		cust.Rect = []string{r.Get("l"), r.Get("t"), r.Get("r"), r.Get("b")}
	}
	for _, p := range children(child(c, "pathLst")) {
		path := Path{Fill: p.Get("fill"), D: [][]string{}}
		path.W, _ = number(p.Get("w"))
		path.H, _ = number(p.Get("h"))
		if s, ok := p.Attr("stroke"); ok {
			stroke := flag(s)
			path.Stroke = &stroke
		}
		for _, cmd := range children(p) {
			step := []string{commands[cmd.Local]}
			switch cmd.Local {
			case "arcTo":
				step = append(step, cmd.Get("wR"), cmd.Get("hR"), cmd.Get("stAng"), cmd.Get("swAng"))
			case "close":
			default:
				for _, pt := range children(cmd) {
					step = append(step, pt.Get("x"), pt.Get("y"))
				}
			}
			if step[0] != "" {
				path.D = append(path.D, step)
			}
		}
		cust.Paths = append(cust.Paths, path)
	}
	return &Geometry{Custom: cust}
}

var commands = map[string]string{
	"moveTo": "M", "lnTo": "L", "arcTo": "A", "quadBezTo": "Q", "cubicBezTo": "C", "close": "Z",
}

func guides(e *xmldom.Element) []Guide {
	var out []Guide
	for _, gd := range children(e) {
		out = append(out, Guide{gd.Get("name"), gd.Get("fmla")})
	}
	return out
}

// Element is the geometry as XML, nil when it is not one.
func (g *Geometry) Element() *xmldom.Element {
	if g.Custom == nil {
		if !presets[g.Preset] {
			return nil
		}
		e := newA("prstGeom", "prst", g.Preset)
		av := newA("avLst")
		for _, name := range sortedKeys(g.AV) {
			av.Append(newA("gd", "name", name, "fmla", g.AV[name]))
		}
		e.Append(av)
		return e
	}
	c := g.Custom
	e := newA("custGeom")
	for _, list := range []struct {
		local  string
		guides []Guide
	}{{"avLst", c.AV}, {"gdLst", c.GD}} {
		l := newA(list.local)
		for _, gd := range list.guides {
			l.Append(newA("gd", "name", gd.Name, "fmla", gd.Formula))
		}
		e.Append(l)
	}
	e.Append(newA("ahLst"))
	e.Append(newA("cxnLst"))
	r := []string{"l", "t", "r", "b"}
	if len(c.Rect) == 4 {
		r = c.Rect
	}
	e.Append(newA("rect", "l", r[0], "t", r[1], "r", r[2], "b", r[3]))
	paths := newA("pathLst")
	for _, p := range c.Paths {
		pe := newA("path", "w", strconv.FormatInt(max(p.W, 0), 10), "h", strconv.FormatInt(max(p.H, 0), 10))
		setEnum(pe, "fill", p.Fill, "none", "norm", "lighten", "lightenLess", "darken", "darkenLess")
		if p.Stroke != nil && !*p.Stroke {
			pe.Set("stroke", "0")
		}
		for _, step := range p.D {
			if cmd := command(step); cmd != nil {
				pe.Append(cmd)
			}
		}
		paths.Append(pe)
	}
	e.Append(paths)
	return e
}

func command(step []string) *xmldom.Element {
	if len(step) == 0 {
		return nil
	}
	pts := func(local string, n int) *xmldom.Element {
		if len(step) != 1+2*n {
			return nil
		}
		e := newA(local)
		for i := range n {
			e.Append(newA("pt", "x", step[1+2*i], "y", step[2+2*i]))
		}
		return e
	}
	switch step[0] {
	case "M":
		return pts("moveTo", 1)
	case "L":
		return pts("lnTo", 1)
	case "Q":
		return pts("quadBezTo", 2)
	case "C":
		return pts("cubicBezTo", 3)
	case "A":
		if len(step) == 5 {
			return newA("arcTo", "wR", step[1], "hR", step[2], "stAng", step[3], "swAng", step[4])
		}
	case "Z":
		return newA("close")
	}
	return nil
}

// SetGeometry makes g the geometry of e, whose children follow order.
func SetGeometry(e *xmldom.Element, g *Geometry, order []string) {
	var c *xmldom.Element
	if g != nil {
		c = g.Element()
	}
	setChild(e, geometryKinds, c, order)
}
