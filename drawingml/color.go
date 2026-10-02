package drawingml

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Color is a DrawingML color: exactly one of its kinds, then the
// transforms applied to it in order.
type Color struct {
	RGB    string  `json:"rgb,omitempty"`
	Scheme string  `json:"scheme,omitempty"`
	Sys    string  `json:"sys,omitempty"`
	Preset string  `json:"prst,omitempty"`
	ScRGB  []int64 `json:"scrgb,omitempty"`
	HSL    []int64 `json:"hsl,omitempty"`
	Mods   []Mod   `json:"mods,omitempty"`
}

// Mod is a color transform, lumMod or alpha, most with a value.
type Mod struct {
	Name  string
	Value *int64
}

func (m Mod) MarshalJSON() ([]byte, error) {
	if m.Value == nil {
		return json.Marshal([]any{m.Name})
	}
	return json.Marshal([]any{m.Name, *m.Value})
}

func (m *Mod) UnmarshalJSON(data []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) < 1 || len(raw) > 2 || json.Unmarshal(raw[0], &m.Name) != nil || m.Name == "" {
		return errors.New("drawingml: bad color transform")
	}
	if len(raw) == 2 {
		m.Value = new(int64)
		return json.Unmarshal(raw[1], m.Value)
	}
	return nil
}

var colorKinds = []string{"scrgbClr", "srgbClr", "hslClr", "sysClr", "schemeClr", "prstClr"}

// ColorIn is the color among the children of e, nil when there is none or
// it is not understood.
func ColorIn(e *xmldom.Element) *Color {
	for _, c := range children(e) {
		for _, k := range colorKinds {
			if c.Local == k {
				return readColor(c)
			}
		}
	}
	return nil
}

func readColor(e *xmldom.Element) *Color {
	c := &Color{}
	ints := func(names ...string) []int64 {
		var out []int64
		for _, n := range names {
			v, ok := number(e.Get(n))
			if !ok {
				return nil
			}
			out = append(out, v)
		}
		return out
	}
	switch e.Local {
	case "srgbClr":
		c.RGB = e.Get("val")
	case "schemeClr":
		c.Scheme = e.Get("val")
	case "sysClr":
		c.Sys, c.RGB = e.Get("val"), e.Get("lastClr")
	case "prstClr":
		c.Preset = e.Get("val")
	case "scrgbClr":
		c.ScRGB = ints("r", "g", "b")
	case "hslClr":
		c.HSL = ints("hue", "sat", "lum")
	}
	if c.RGB == "" && c.Scheme == "" && c.Sys == "" && c.Preset == "" && c.ScRGB == nil && c.HSL == nil {
		return nil
	}
	for _, m := range children(e) {
		mod := Mod{Name: m.Local}
		if v, ok := m.Attr("val"); ok {
			n, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil
			}
			mod.Value = &n
		}
		c.Mods = append(c.Mods, mod)
	}
	if len(e.Elements()) != len(c.Mods) {
		return nil
	}
	return c
}

// Element is the color as XML.
func (c *Color) Element() *xmldom.Element {
	var e *xmldom.Element
	itoa := func(v int64) string { return strconv.FormatInt(v, 10) }
	switch {
	case schemeColors[c.Scheme]:
		e = newA("schemeClr", "val", c.Scheme)
	case name(c.Sys):
		e = newA("sysClr", "val", c.Sys, "lastClr", validRGB(c.RGB))
	case name(c.Preset):
		e = newA("prstClr", "val", c.Preset)
	case len(c.ScRGB) == 3:
		e = newA("scrgbClr", "r", itoa(c.ScRGB[0]), "g", itoa(c.ScRGB[1]), "b", itoa(c.ScRGB[2]))
	case len(c.HSL) == 3:
		e = newA("hslClr", "hue", itoa(c.HSL[0]), "sat", itoa(c.HSL[1]), "lum", itoa(c.HSL[2]))
	default:
		e = newA("srgbClr", "val", validRGB(c.RGB))
	}
	for _, m := range c.Mods {
		valued, known := colorMods[m.Name]
		if !known || valued != (m.Value != nil) {
			continue
		}
		mod := newA(m.Name)
		if m.Value != nil {
			mod.Set("val", itoa(*m.Value))
		}
		e.Append(mod)
	}
	return e
}

var schemeColors = map[string]bool{
	"bg1": true, "tx1": true, "bg2": true, "tx2": true, "dk1": true, "lt1": true, "dk2": true, "lt2": true,
	"accent1": true, "accent2": true, "accent3": true, "accent4": true, "accent5": true, "accent6": true,
	"hlink": true, "folHlink": true, "phClr": true,
}

// colorMods are the color transforms, and whether each takes a value.
var colorMods = map[string]bool{
	"tint": true, "shade": true, "comp": false, "inv": false, "gray": false,
	"alpha": true, "alphaOff": true, "alphaMod": true,
	"hue": true, "hueOff": true, "hueMod": true, "sat": true, "satOff": true, "satMod": true,
	"lum": true, "lumOff": true, "lumMod": true,
	"red": true, "redOff": true, "redMod": true, "green": true, "greenOff": true, "greenMod": true,
	"blue": true, "blueOff": true, "blueMod": true, "gamma": false, "invGamma": false,
}

// validRGB is s if it is six hex digits, black otherwise.
func validRGB(s string) string {
	if len(s) != 6 {
		return "000000"
	}
	for _, c := range s {
		if !('0' <= c && c <= '9' || 'A' <= c && c <= 'F' || 'a' <= c && c <= 'f') {
			return "000000"
		}
	}
	return s
}

// name is a local name a client may send: letters and digits.
func name(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !('a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9') {
			return false
		}
	}
	return true
}
