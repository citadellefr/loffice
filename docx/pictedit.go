package docx

import (
	"encoding/json"
	"hash/maphash"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// maxEMU bounds the sizes and offsets of a drawing a client sets.
const maxEMU = 1 << 31

var (
	relativeH = []string{"margin", "page", "column", "character", "leftMargin", "rightMargin", "insideMargin", "outsideMargin"}
	relativeV = []string{"margin", "page", "paragraph", "line", "topMargin", "bottomMargin", "insideMargin", "outsideMargin"}
	alignH    = []string{"left", "right", "center", "inside", "outside"}
	alignV    = []string{"top", "bottom", "center", "inside", "outside"}
	wrapSides = []string{"bothSides", "left", "right", "largest"}
	// anchorOrder is the sequence of the children of a wp:anchor.
	anchorOrder = []string{"simplePos", "positionH", "positionV", "extent", "effectExtent",
		"wrapNone", "wrapSquare", "wrapTight", "wrapThrough", "wrapTopAndBottom", "docPr"}
)

// edited is the element o stands for, with the changes img makes to the
// picture it was read as: its size, crop, description and, for an anchored
// drawing, its position and wrapping. The element is the one read, and the
// error ErrReadOnly, when img changes anything else or what the drawing
// cannot hold; nil when o is not XML the document read.
func (d *Document) edited(o, img string) (*xmldom.Element, error) {
	e := d.fragment(o)
	read := d.images[maphash.String(d.seed, o)]
	if e == nil || img == "" && read == "" || img == read {
		return e, nil
	}
	var from, to Picture
	if json.Unmarshal([]byte(read), &from) != nil || json.Unmarshal([]byte(img), &to) != nil || !to.valid() || !from.sameBut(&to) {
		return e, ErrReadOnly
	}
	edited := e.Clone()
	found := false
	var walk func(c *xmldom.Element) error
	walk = func(c *xmldom.Element) error {
		if c.Space == NS && c.Local == "pict" {
			restyle(c, &from, &to)
			return nil
		}
		if c.Space == wpNS && (c.Local == "inline" || c.Local == "anchor") {
			found = true
			return place(c, &from, &to)
		}
		for _, k := range c.Elements() {
			if err := walk(k); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(edited); err != nil || !found {
		return e, ErrReadOnly
	}
	return edited, nil
}

// plain tells whether img is a picture a client may insert as a new
// drawing: a picture of a size, with its description.
func plain(img string) bool {
	var p Picture
	return json.Unmarshal([]byte(img), &p) == nil &&
		reflect.DeepEqual(p, Picture{Media: p.Media, W: p.W, H: p.H, Descr: p.Descr}) && p.valid()
}

// valid tells whether the size, crop and position of the picture are
// values a drawing can hold.
func (p *Picture) valid() bool {
	if p.W <= 0 || p.H <= 0 || p.W > maxEMU || p.H > maxEMU {
		return false
	}
	for side, v := range p.Crop {
		if !slices.Contains([]string{"l", "t", "r", "b"}, side) || v < -100000 || v > 100000 {
			return false
		}
	}
	f := p.Float
	if f == nil {
		return true
	}
	return slices.Contains(relativeH, f.RelX) && slices.Contains(relativeV, f.RelY) &&
		(f.AlignX == "" || slices.Contains(alignH, f.AlignX)) && (f.AlignY == "" || slices.Contains(alignV, f.AlignY)) &&
		f.X >= -maxEMU && f.X <= maxEMU && f.Y >= -maxEMU && f.Y <= maxEMU &&
		slices.Contains([]string{"none", "square", "tight", "through", "topAndBottom"}, f.Wrap) &&
		(f.WrapText == "" || slices.Contains(wrapSides, f.WrapText))
}

// sameBut tells whether the pictures differ by no more than what edited
// writes.
func (p *Picture) sameBut(q *Picture) bool {
	if (p.Float == nil) != (q.Float == nil) {
		return false
	}
	strip := func(p Picture) string {
		p.W, p.H, p.Crop, p.Descr = 0, 0, nil, ""
		if p.Float != nil {
			p.Float = &Float{}
		}
		data, _ := json.Marshal(p)
		return string(data)
	}
	return strip(*p) == strip(*q)
}

// place writes what a picture changed into a wp:inline or wp:anchor.
func place(c *xmldom.Element, from, to *Picture) error {
	if from.W != to.W || from.H != to.H {
		ext := c.Child(wpNS, "extent")
		if ext == nil {
			return ErrReadOnly
		}
		cx, cy := strconv.FormatInt(to.W, 10), strconv.FormatInt(to.H, 10)
		ext.Set("cx", cx)
		ext.Set("cy", cy)
		for _, k := range c.Elements() {
			if g := find(k, aNS, "graphicData"); g != nil {
				resize(g, cx, cy)
			}
		}
	}
	if from.Descr != to.Descr {
		doc := c.Child(wpNS, "docPr")
		if doc == nil {
			return ErrReadOnly
		}
		if to.Descr == "" {
			doc.Unset("descr")
		} else {
			doc.Set("descr", to.Descr)
		}
	}
	if !maps.Equal(from.Crop, to.Crop) {
		if err := crop(c, to.Crop); err != nil {
			return err
		}
	}
	if from.Float == nil || *from.Float == *to.Float {
		return nil
	}
	if c.Local != "anchor" {
		return ErrReadOnly
	}
	return float(c, from.Float, to.Float)
}

// restyle gives the VML shapes of a w:pict, the fallback of a drawing, the
// size of the picture in points and, when they are absolute, its offsets.
func restyle(pict *xmldom.Element, from, to *Picture) {
	set := map[string]string{}
	if from.W != to.W || from.H != to.H {
		set["width"], set["height"] = points(to.W), points(to.H)
	}
	if from.Float != nil && to.Float != nil {
		if from.Float.X != to.Float.X && to.Float.AlignX == "" {
			set["left"], set["margin-left"] = points(to.Float.X), points(to.Float.X)
		}
		if from.Float.Y != to.Float.Y && to.Float.AlignY == "" {
			set["top"], set["margin-top"] = points(to.Float.Y), points(to.Float.Y)
		}
	}
	if len(set) == 0 {
		return
	}
	var walk func(e *xmldom.Element)
	walk = func(e *xmldom.Element) {
		if style := e.Get("style"); e.Space == vmlNS && style != "" {
			e.Set("style", styled(style, set))
		}
		for _, c := range e.Elements() {
			walk(c)
		}
	}
	walk(pict)
}

// styled is a CSS style of VML with the values of set in place of its own;
// the offsets only where the style has them and is absolute.
func styled(style string, set map[string]string) string {
	decls := strings.Split(style, ";")
	absolute := slices.ContainsFunc(decls, func(d string) bool { return strings.ReplaceAll(d, " ", "") == "position:absolute" })
	for i, d := range decls {
		key, _, ok := strings.Cut(d, ":")
		key = strings.TrimSpace(key)
		v, found := set[key]
		if ok && found && (absolute || key == "width" || key == "height") {
			decls[i] = key + ":" + v
		}
	}
	return strings.Join(decls, ";")
}

// points is a length of EMU in points, as a VML style gives it.
func points(emu int64) string {
	return strconv.FormatFloat(float64(emu)/12700, 'f', -1, 64) + "pt"
}

// resize gives the shape a graphicData holds its size: the xfrm of its
// properties.
func resize(data *xmldom.Element, cx, cy string) {
	for _, shape := range data.Elements() {
		for _, props := range shape.Elements() {
			if props.Local != "spPr" && props.Local != "grpSpPr" {
				continue
			}
			if xfrm := props.Child(aNS, "xfrm"); xfrm != nil {
				if ext := xfrm.Child(aNS, "ext"); ext != nil {
					ext.Set("cx", cx)
					ext.Set("cy", cy)
				}
			}
		}
	}
}

// crop sets the part of the picture cut off each side.
func crop(c *xmldom.Element, sides map[string]int) error {
	fill := find(c, picNS, "blipFill")
	if fill == nil || fill.Child(aNS, "blip") == nil {
		return ErrReadOnly
	}
	src := fill.Child(aNS, "srcRect")
	if len(sides) == 0 {
		if src != nil {
			fill.Remove(src)
		}
		return nil
	}
	if src == nil {
		src = xmldom.New(aNS, "a:srcRect")
		fill.Insert(src, []string{"blip", "srcRect", "tile", "stretch"})
	}
	for _, side := range []string{"l", "t", "r", "b"} {
		if v := sides[side]; v != 0 {
			src.Set(side, strconv.Itoa(v))
		} else {
			src.Unset(side)
		}
	}
	return nil
}

// float sets the position and wrapping of an anchor.
func float(a *xmldom.Element, from, to *Float) error {
	name := func(local string) string {
		if p, _, ok := strings.Cut(a.Name, ":"); ok {
			return p + ":" + local
		}
		return local
	}
	if from.Behind != to.Behind {
		a.Set("behindDoc", map[bool]string{true: "1", false: "0"}[to.Behind])
	}
	for _, axis := range []struct {
		local, rel, align string
		offset            int64
		changed           bool
	}{
		{"positionH", to.RelX, to.AlignX, to.X, from.RelX != to.RelX || from.AlignX != to.AlignX || from.X != to.X},
		{"positionV", to.RelY, to.AlignY, to.Y, from.RelY != to.RelY || from.AlignY != to.AlignY || from.Y != to.Y},
	} {
		if !axis.changed {
			continue
		}
		pos := a.Child(wpNS, axis.local)
		if pos == nil {
			return ErrReadOnly
		}
		pos.Set("relativeFrom", axis.rel)
		pos.Content = nil
		if axis.align != "" {
			al := xmldom.New(wpNS, name("align"))
			al.Append(xmldom.EscapeText(axis.align))
			pos.Append(al)
		} else {
			off := xmldom.New(wpNS, name("posOffset"))
			off.Append(xmldom.EscapeText(strconv.FormatInt(axis.offset, 10)))
			pos.Append(off)
		}
	}
	if from.Wrap == to.Wrap && from.WrapText == to.WrapText {
		return nil
	}
	var wrap *xmldom.Element
	for _, c := range a.Elements() {
		if c.Space == wpNS && strings.HasPrefix(c.Local, "wrap") {
			wrap = c
		}
	}
	if from.Wrap != to.Wrap {
		local := "wrap" + strings.ToUpper(to.Wrap[:1]) + to.Wrap[1:]
		switch to.Wrap {
		case "square":
			next := xmldom.New(wpNS, name(local), "wrapText", "bothSides")
			replace(a, wrap, next)
			wrap = next
		case "topAndBottom", "none":
			if to.WrapText != "" {
				return ErrReadOnly
			}
			replace(a, wrap, xmldom.New(wpNS, name(local)))
			return nil
		default:
			return ErrReadOnly
		}
	}
	if to.Wrap != "square" && to.Wrap != "tight" && to.Wrap != "through" || to.WrapText == "" {
		return nil
	}
	wrap.Set("wrapText", to.WrapText)
	return nil
}

func replace(a, old, next *xmldom.Element) {
	if old != nil {
		a.Remove(old)
	}
	a.Insert(next, anchorOrder)
}

// checkPictures tells whether the pictures a change of a flow sets, over
// old, are written: the changes of those the document read are, or else
// the change is refused.
func (d *Document) checkPictures(old, delta ot.Delta) error {
	pos := 0
	for _, op := range delta {
		img, has := op.Attrs["img"]
		switch {
		case op.Insert != "" && has:
			if err := d.checkPicture(op.Attrs["o"], img); err != nil {
				return err
			}
		case op.Retain > 0:
			if has {
				if err := d.checkRetained(old, pos, pos+op.Retain, op.Attrs); err != nil {
					return err
				}
			}
			pos += op.Retain
		default:
			pos += op.Delete
		}
	}
	return nil
}

// checkRetained checks the objects of old between start and end that
// attrs gives a picture.
func (d *Document) checkRetained(old ot.Delta, start, end int, attrs ot.Attrs) error {
	off := 0
	for _, o := range old {
		n := o.Len()
		if off < end && off+n > start {
			read, ok := attrs["o"]
			if !ok {
				read = o.Attrs["o"]
			}
			if err := d.checkPicture(read, attrs["img"]); err != nil {
				return err
			}
		}
		off += n
	}
	return nil
}

// checkPicture tells whether the picture img, of an object that is the
// XML o, is written.
func (d *Document) checkPicture(o, img string) error {
	if o == "" {
		if img != "" && !plain(img) {
			return ErrReadOnly
		}
		return nil
	}
	_, err := d.edited(o, img)
	return err
}
