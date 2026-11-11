package drawingml

import "github.com/citadellefr/loffice/internal/xmldom"

// Shadow is the outer shadow of a shape: Blur and Dist in EMU, Dir in
// 60000ths of a degree clockwise from the right.
type Shadow struct {
	Color *Color `json:"color"`
	Blur  int64  `json:"blur,omitempty"`
	Dist  int64  `json:"dist,omitempty"`
	Dir   int64  `json:"dir,omitempty"`
}

// ReadShadow reads the a:outerShdw of an a:effectLst, nil when it has none.
func ReadShadow(effects *xmldom.Element) *Shadow {
	e := child(effects, "outerShdw")
	if e == nil {
		return nil
	}
	c := ColorIn(e)
	if c == nil {
		return nil
	}
	s := &Shadow{Color: c}
	s.Blur, _ = number(e.Get("blurRad"))
	s.Dist, _ = number(e.Get("dist"))
	s.Dir, _ = number(e.Get("dir"))
	return s
}
