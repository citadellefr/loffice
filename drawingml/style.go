package drawingml

import "github.com/citadellefr/loffice/internal/xmldom"

// Style points a shape to the theme's line, fill, effect and font styles,
// each with a color.
type Style struct {
	Line   *StyleRef `json:"ln,omitempty"`
	Fill   *StyleRef `json:"fill,omitempty"`
	Effect *StyleRef `json:"effect,omitempty"`
	Font   *StyleRef `json:"font,omitempty"`
}

// StyleRef is an index in the theme's list of styles, or "major" or
// "minor" for a font.
type StyleRef struct {
	Idx   string `json:"idx"`
	Color *Color `json:"color,omitempty"`
}

// ReadStyle reads a style element, nil for none.
func ReadStyle(e *xmldom.Element) *Style {
	if e == nil {
		return nil
	}
	ref := func(local string) *StyleRef {
		c := child(e, local)
		if c == nil {
			return nil
		}
		return &StyleRef{Idx: c.Get("idx"), Color: ColorIn(c)}
	}
	return &Style{Line: ref("lnRef"), Fill: ref("fillRef"), Effect: ref("effectRef"), Font: ref("fontRef")}
}
