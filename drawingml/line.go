package drawingml

import (
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Line is the outline of a shape. Absent fields are inherited.
type Line struct {
	W    *int64 `json:"w,omitempty"`
	Fill *Fill  `json:"fill,omitempty"`
	// Dash is a preset dash, "solid" or "dash" or "sysDot"…
	Dash string `json:"dash,omitempty"`
	Cap  string `json:"cap,omitempty"`
	Cmpd string `json:"cmpd,omitempty"`
	// Join is "round", "bevel" or "miter".
	Join string `json:"join,omitempty"`
	Head *End   `json:"head,omitempty"`
	Tail *End   `json:"tail,omitempty"`
}

// End is an arrowhead.
type End struct {
	Type string `json:"type,omitempty"`
	W    string `json:"w,omitempty"`
	Len  string `json:"len,omitempty"`
}

var lineOrder = []string{"noFill", "solidFill", "gradFill", "pattFill", "prstDash", "custDash", "round", "bevel", "miter", "headEnd", "tailEnd", "extLst"}

// ReadLine reads an a:ln.
func ReadLine(e *xmldom.Element) *Line {
	if e == nil {
		return nil
	}
	l := &Line{Cap: e.Get("cap"), Cmpd: e.Get("cmpd")}
	if w, ok := number(e.Get("w")); ok {
		l.W = &w
	}
	l.Fill = FillIn(e, nil)
	if d := child(e, "prstDash"); d != nil {
		l.Dash = d.Get("val")
	}
	for _, j := range []string{"round", "bevel", "miter"} {
		if child(e, j) != nil {
			l.Join = j
		}
	}
	l.Head = readEnd(child(e, "headEnd"))
	l.Tail = readEnd(child(e, "tailEnd"))
	return l
}

func readEnd(e *xmldom.Element) *End {
	if e == nil {
		return nil
	}
	return &End{Type: e.Get("type"), W: e.Get("w"), Len: e.Get("len")}
}

// SetLine patches the a:ln e, read as old, to be l: what did not change
// stays as it was.
func SetLine(e *xmldom.Element, old, l *Line) {
	if old == nil {
		old = &Line{}
	}
	if !same(old.W, l.W) {
		setNumber(e, "w", l.W, 0, 20116800)
	}
	if old.Cap != l.Cap {
		setEnum(e, "cap", l.Cap, "rnd", "sq", "flat")
	}
	if old.Cmpd != l.Cmpd {
		setEnum(e, "cmpd", l.Cmpd, "sng", "dbl", "thickThin", "thinThick", "tri")
	}
	if !same(old.Fill, l.Fill) {
		SetFill(e, l.Fill, nil, lineOrder)
	}
	if old.Dash != l.Dash {
		var d *xmldom.Element
		if dashes[l.Dash] {
			d = newA("prstDash", "val", l.Dash)
		}
		setChild(e, []string{"prstDash", "custDash"}, d, lineOrder)
	}
	if old.Join != l.Join {
		var j *xmldom.Element
		switch l.Join {
		case "round", "bevel":
			j = newA(l.Join)
		case "miter":
			j = newA("miter", "lim", "800000")
		}
		setChild(e, []string{"round", "bevel", "miter"}, j, lineOrder)
	}
	for _, end := range []struct {
		local    string
		old, new *End
	}{{"headEnd", old.Head, l.Head}, {"tailEnd", old.Tail, l.Tail}} {
		if same(end.old, end.new) {
			continue
		}
		var c *xmldom.Element
		if end.new != nil {
			c = newA(end.local)
			setEnum(c, "type", end.new.Type, "none", "triangle", "stealth", "diamond", "oval", "arrow")
			setEnum(c, "w", end.new.W, "sm", "med", "lg")
			setEnum(c, "len", end.new.Len, "sm", "med", "lg")
		}
		setChild(e, []string{end.local}, c, lineOrder)
	}
}

var dashes = map[string]bool{
	"solid": true, "dot": true, "dash": true, "lgDash": true, "dashDot": true, "lgDashDot": true,
	"lgDashDotDot": true, "sysDash": true, "sysDot": true, "sysDashDot": true, "sysDashDotDot": true,
}

// setEnum sets the attribute to v if it is one of the values allowed,
// removes it otherwise.
func setEnum(e *xmldom.Element, attr, v string, allowed ...string) {
	for _, a := range allowed {
		if a == v {
			e.Set(attr, v)
			return
		}
	}
	e.Unset(attr)
}

// setNumber sets the attribute to v bounded to [lo, hi], or removes it.
func setNumber(e *xmldom.Element, attr string, v *int64, lo, hi int64) {
	if v == nil {
		e.Unset(attr)
		return
	}
	e.Set(attr, strconv.FormatInt(min(max(*v, lo), hi), 10))
}
