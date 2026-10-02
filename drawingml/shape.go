package drawingml

import (
	"bytes"
	"encoding/json"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/ot"
)

var spPrOrder = []string{
	"xfrm", "custGeom", "prstGeom", "noFill", "solidFill", "gradFill", "blipFill", "pattFill", "grpFill",
	"ln", "effectLst", "effectDag", "scene3d", "sp3d", "extLst",
}

// ShapeProps reads the spPr of a shape into the attributes of its node:
// "xfrm", "geom", "fill" and "line", those it has. media names the picture
// a relationship id points to.
func ShapeProps(spPr *xmldom.Element, media func(rid string) string) ot.Values {
	v := ot.Values{}
	if spPr == nil {
		return v
	}
	put(v, "xfrm", ReadXfrm(child(spPr, "xfrm")))
	put(v, "geom", GeometryIn(spPr))
	put(v, "fill", FillIn(spPr, media))
	put(v, "line", ReadLine(child(spPr, "ln")))
	return v
}

// put sets v[key] to x as JSON, unless x is nil.
func put[T any](v ot.Values, key string, x *T) {
	if x != nil {
		data, _ := json.Marshal(x)
		v[key] = data
	}
}

// SetShapeProps patches an spPr read as old so that it holds the attributes
// of values. A picture fill needs embed to give the relationship id of its
// media.
func SetShapeProps(spPr *xmldom.Element, old, values ot.Values, embed func(media string) string) {
	changed := func(key string) bool { return !bytes.Equal(old[key], values[key]) }
	if changed("xfrm") {
		var x Xfrm
		switch e := child(spPr, "xfrm"); {
		case json.Unmarshal(values["xfrm"], &x) != nil:
			if e != nil {
				spPr.Remove(e)
			}
		case e == nil:
			e = newA("xfrm")
			SetXfrm(e, &x)
			spPr.Insert(e, spPrOrder)
		default:
			SetXfrm(e, &x)
		}
	}
	if changed("geom") {
		SetGeometry(spPr, decode[Geometry](values["geom"]), spPrOrder)
	}
	if changed("fill") {
		SetFill(spPr, decode[Fill](values["fill"]), embed, spPrOrder)
	}
	if changed("line") {
		l := decode[Line](values["line"])
		e := child(spPr, "ln")
		switch {
		case l == nil:
			if e != nil {
				spPr.Remove(e)
			}
		case e == nil:
			e = newA("ln")
			SetLine(e, nil, l)
			spPr.Insert(e, spPrOrder)
		default:
			SetLine(e, ReadLine(e), l)
		}
	}
}

// decode is the JSON value data as a T, nil when it is not one.
func decode[T any](data json.RawMessage) *T {
	if data == nil {
		return nil
	}
	var t T
	if json.Unmarshal(data, &t) != nil {
		return nil
	}
	return &t
}
