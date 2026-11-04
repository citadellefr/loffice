package xlsx

import (
	"bytes"
	"encoding/json"
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// writeConditionals gives a worksheet the conditional formats of its sheet.
// The rules read from the file keep their element when the model still has
// them, whatever their priority, with the extension that goes with it; the
// others are written, and those the model lost are removed. The rules the
// model does not hold stay where their areas are.
func (w *writer) writeConditionals(root *xmldom.Element, raw json.RawMessage) error {
	var model []Conditional
	_ = json.Unmarshal(raw, &model)
	old := w.d.cfRules(root)
	keys := make([][]byte, len(old))
	for i, r := range old {
		keys[i] = unprioritized(r.Conditional)
	}
	used := make([]bool, len(old))

	type group struct {
		ref   string
		rules []*xmldom.Element
	}
	var groups []*group
	byRef := map[string]*group{}
	put := func(ref string, rule *xmldom.Element) {
		g := byRef[ref]
		if g == nil {
			g = &group{ref: ref}
			byRef[ref] = g
			groups = append(groups, g)
		}
		g.rules = append(g.rules, rule)
	}
	p := prefixOf(root)
	for _, c := range model {
		key := unprioritized(c)
		var rule *xmldom.Element
		for i, k := range keys {
			if !used[i] && bytes.Equal(k, key) {
				used[i], rule = true, old[i].elem
				break
			}
		}
		if rule == nil {
			var err error
			if rule, err = w.cfRule(p, c); err != nil {
				return err
			}
		}
		rule.Set("priority", strconv.Itoa(c.Pri))
		put(c.Ref, rule)
	}

	lost := map[string]bool{}
	for i, r := range old {
		if used[i] {
			continue
		}
		for _, ext := range elements(r.elem.Child(mainNS, "extLst"), "ext") {
			if id := ext.Child(x14NS, "id"); id != nil {
				lost[id.Text()] = true
			}
		}
	}
	read := map[*xmldom.Element]bool{}
	for _, r := range old {
		read[r.elem] = true
	}
	for _, cf := range elements(root, "conditionalFormatting") {
		ref, _, ok := cfArea(cf)
		if !ok {
			continue
		}
		for _, rule := range elements(cf, "cfRule") {
			if !read[rule] {
				put(ref, rule)
			}
		}
		root.Remove(cf)
	}
	for _, g := range groups {
		cf := xmldom.New(mainNS, p+"conditionalFormatting", "sqref", g.ref)
		for _, rule := range g.rules {
			cf.Append(rule)
		}
		root.Insert(cf, worksheetOrder)
	}
	if len(lost) > 0 {
		editExt(root, func(ext *xmldom.Element) {
			all := ext.Child(x14NS, "conditionalFormattings")
			for _, cf := range elementsIn(all, x14NS, "conditionalFormatting") {
				rules := 0
				for _, r := range elementsIn(cf, x14NS, "cfRule") {
					if lost[r.Get("id")] {
						cf.Remove(r)
					} else {
						rules++
					}
				}
				if rules == 0 {
					all.Remove(cf)
				}
			}
			if all != nil && len(all.Elements()) == 0 {
				ext.Remove(all)
			}
		})
	}
	return nil
}

// elementsIn are the children of e in a namespace, with that local name; e
// may be nil.
func elementsIn(e *xmldom.Element, space, local string) []*xmldom.Element {
	var out []*xmldom.Element
	if e != nil {
		for _, c := range e.Elements() {
			if c.Space == space && c.Local == local {
				out = append(out, c)
			}
		}
	}
	return out
}

// unprioritized is the rule as JSON, whatever its priority.
func unprioritized(c Conditional) []byte {
	c.Pri = 0
	return mustJSON(c)
}

// cfRule writes a conditional format as a rule.
func (w *writer) cfRule(p string, c Conditional) (*xmldom.Element, error) {
	e := xmldom.New(mainNS, p+"cfRule")
	if c.Type == "formula" {
		e.Set("type", "expression")
	} else {
		e.Set("type", c.Type)
	}
	if c.Style != nil {
		id, err := w.styles.dxf(c.Style)
		if err != nil {
			return nil, err
		}
		e.Set("dxfId", strconv.Itoa(id))
	}
	setAttr(e, "stopIfTrue", boolAttr(c.Stop))
	switch c.Type {
	case "formula":
		f := xmldom.New(mainNS, p+"formula")
		f.Append(xmldom.EscapeText(c.F[0]))
		e.Append(f)
	case "top10":
		e.Set("rank", strconv.Itoa(c.Rank))
		setAttr(e, "percent", boolAttr(c.Percent))
		setAttr(e, "bottom", boolAttr(c.Bottom))
	case "aboveAverage":
		if c.Below {
			e.Set("aboveAverage", "0")
		}
		setAttr(e, "equalAverage", boolAttr(c.Equal))
		if c.StdDev > 0 {
			e.Set("stdDev", strconv.Itoa(c.StdDev))
		}
	case "colorScale", "dataBar", "iconSet":
		e.Append(scaleElement(p, c))
	}
	return e, nil
}

// scaleElement writes the thresholds and colors of a scale, a bar or a set
// of icons.
func scaleElement(p string, c Conditional) *xmldom.Element {
	e := xmldom.New(mainNS, p+c.Type)
	switch c.Type {
	case "dataBar":
		e.Set("minLength", strconv.Itoa(c.MinLen))
		e.Set("maxLength", strconv.Itoa(c.MaxLen))
	case "iconSet":
		e.Set("iconSet", c.Icons)
		setAttr(e, "reverse", boolAttr(c.Reverse))
	}
	if c.Type != "colorScale" && c.NoValue {
		e.Set("showValue", "0")
	}
	for _, t := range c.Stops {
		v := xmldom.New(mainNS, p+"cfvo", "type", t.Type)
		if t.Type != "min" && t.Type != "max" {
			v.Set("val", t.Val)
		}
		if t.Above {
			v.Set("gte", "0")
		}
		e.Append(v)
	}
	colors := c.Colors
	if c.Type == "dataBar" {
		colors = colors[:1]
	}
	if c.Type != "iconSet" {
		for _, col := range colors {
			e.Append(colorElement(e, "color", col))
		}
	}
	return e
}
