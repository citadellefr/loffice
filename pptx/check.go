package pptx

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/citadellefr/loffice/internal/chartpart"
	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/trame/ot"
)

var (
	ErrReadOnly  = errors.New("pptx: this part of the presentation cannot be edited")
	ErrAnimation = errors.New("pptx: invalid animation")
)

// shapeTypes are the nodes a slide or a group can hold.
var shapeTypes = map[string]bool{"sp": true, "pic": true, "cxn": true, "grp": true, "frame": true, "alt": true, "other": true}

// commentEditable are the attributes of a comment that may change: where
// it sits.
var commentEditable = map[string]bool{"x": true, "y": true}

// editable are the attributes a change may set, by node type.
var editable = map[string]map[string]bool{
	"slide": {"name": true, "hidden": true, "bg": true, "layout": true, "transition": true, "animations": true},
	"sp":    {"name": true, "descr": true, "hidden": true, "xfrm": true, "geom": true, "fill": true, "line": true, "body": true},
	"pic":   {"name": true, "descr": true, "hidden": true, "xfrm": true, "geom": true, "fill": true, "line": true, "blip": true},
	"cxn":   {"name": true, "descr": true, "hidden": true, "xfrm": true, "geom": true, "line": true},
	"grp":   {"name": true, "descr": true, "hidden": true, "xfrm": true, "fill": true},
	"frame": {"name": true, "descr": true, "hidden": true, "xfrm": true, "tbl": true, "grid": true, chartpart.ChartKey: true, chartpart.DataKey: true},
	"tr":    {"h": true},
	"tc": {"gridSpan": true, "rowSpan": true, "hMerge": true, "vMerge": true, "fill": true, "mar": true, "anchor": true, "vert": true,
		"lnL": true, "lnR": true, "lnT": true, "lnB": true, "lnTlToBr": true, "lnBlToTr": true},
}

// tableTypes are the rows of a table and their cells, by the type of node
// they are under.
var tableTypes = map[string]string{"tr": "frame", "tc": "tr"}

// Check tells whether an edit only changes what can be edited: the slides,
// their shapes and the text of their notes, not the masters and layouts.
// The comments it adds are signed by author.
func (d *Document) Check(tree *ot.Tree, e ot.Edit, author string) error {
	created := map[string]string{}
	parents := map[string]string{}
	slideOf := func(id string) string {
		if p, ok := parents[id]; ok {
			return p
		}
		if n := tree.Node(id); n != nil {
			return n.Parent
		}
		return ""
	}
	typeOf := func(id string) string {
		if t, ok := created[id]; ok {
			return t
		}
		if n := tree.Node(id); n != nil {
			return n.Type
		}
		return ""
	}
	deleted := map[string]bool{}
	// slideOfShape is the slide a shape is under, "" if it is not a shape.
	slideOfShape := func(id string) string {
		if t := typeOf(id); !shapeTypes[t] || t == "alt" || t == "other" || deleted[id] {
			return ""
		}
		for n := id; n != ""; {
			if typeOf(n) == "slide" {
				return n
			}
			if p, ok := parents[n]; ok {
				n = p
			} else if node := tree.Node(n); node != nil {
				n = node.Parent
			} else {
				break
			}
		}
		return ""
	}
	for _, c := range e {
		switch c.Op {
		case ot.OpNew:
			parent := typeOf(c.Parent)
			ok := c.Type == "slide" && c.Parent == "deck" ||
				c.Type == "comment" && parent == "slide" && newComment(c, author, c.Parent, typeOf, slideOf) ||
				shapeTypes[c.Type] && (parent == "slide" || parent == "grp") ||
				tableTypes[c.Type] != "" && tableTypes[c.Type] == parent
			if !ok {
				return ErrReadOnly
			}
			created[c.ID] = c.Type
			parents[c.ID] = c.Parent
		case ot.OpDel:
			deleted[c.ID] = true
			if t := typeOf(c.ID); t != "" && t != "slide" && t != "comment" && !shapeTypes[t] && tableTypes[t] == "" {
				return ErrReadOnly
			}
		case ot.OpSet:
			t := typeOf(c.ID)
			if t == "" {
				continue
			}
			if t != "slide" && t != "comment" && !shapeTypes[t] && tableTypes[t] == "" {
				return ErrReadOnly
			}
			for k := range c.Attrs {
				if !editable[t][k] && !(t == "comment" && commentEditable[k]) {
					return ErrReadOnly
				}
			}
			if err := chartpart.Check(tree, c.ID, c.Attrs, d.charts); err != nil {
				return err
			}
			if raw, ok := c.Attrs["animations"]; ok {
				if err := d.checkAnimations(raw, c.ID, slideOfShape); err != nil {
					return err
				}
			}
			if !d.inSlide(tree, created, c.ID) {
				return ErrReadOnly
			}
		case ot.OpTxt:
			if t := typeOf(c.ID); t != "" && t != "sp" && t != "tc" && t != "notes" && t != "comment" {
				return ErrReadOnly
			}
		}
		if c.Op != ot.OpNew && c.Op != ot.OpSet && !d.inSlide(tree, created, c.ID) {
			return ErrReadOnly
		}
	}
	return nil
}

// Follow gives the charts and the animations an edit changed what goes with
// them, in tree.
func (d *Document) Follow(tree *ot.Tree, e ot.Edit) ot.Edit {
	return append(chartpart.Follow(tree, e, d.charts), followAnimations(tree, e)...)
}

// inSlide tells whether a node is a slide or under one, or no longer
// exists.
func (d *Document) inSlide(tree *ot.Tree, created map[string]string, id string) bool {
	if _, ok := created[id]; ok {
		return true
	}
	n := tree.Node(id)
	if n == nil {
		return true
	}
	for n != nil {
		if n.Type == "slide" {
			return n.Parent == "deck"
		}
		n = tree.Node(n.Parent)
	}
	return false
}

// Media is a picture of the document, by the name its blips give it, and
// its content type.
func (d *Document) Media(name string) ([]byte, string, error) {
	r, ok := d.names.Lookup("@" + name)
	if !ok || r.Type != partrel.Image || r.External {
		return nil, "", partrel.ErrNoMedia
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := d.pkg.Read(r.Target)
	if err != nil {
		return nil, "", errors.Join(partrel.ErrNoMedia, err)
	}
	return data, d.pkg.ContentType(r.Target), nil
}

// checkAnimations tells whether the effects of a slide are in bounds, on
// shapes of that slide, and ones the writer can write.
func (d *Document) checkAnimations(raw json.RawMessage, slide string, slideOfShape func(string) string) error {
	var list []Effect
	if raw == nil {
		return nil
	}
	if err := json.Unmarshal(raw, &list); err != nil || len(list) > maxEffects {
		return ErrAnimation
	}
	inBounds := func(n int) bool { return n >= 0 && n <= maxTime }
	for _, e := range list {
		if nodeTypes[e.Trigger] == "" || !inBounds(e.Delay) || !inBounds(e.Dur) || e.Preset < 0 || e.Subtype < 0 {
			return ErrAnimation
		}
		if e.Target != "" && slideOfShape(e.Target) != slide {
			return ErrAnimation
		}
		if e.Para != nil && (e.Para.From < 0 || e.Para.To < e.Para.From || e.Para.To > maxPara) {
			return ErrAnimation
		}
		if e.XML != "" {
			if d.fragment(e.XML) == nil {
				return ErrAnimation
			}
			continue
		}
		if _, ok := body(preset{e.Class, e.Preset, e.Subtype}, "", 0); !ok || e.Target == "" {
			return ErrAnimation
		}
	}
	return nil
}

// followAnimations removes from the animations of the slides the effects on
// shapes an edit deleted, and gives the changes it made to tree.
func followAnimations(tree *ot.Tree, e ot.Edit) ot.Edit {
	if !slices.ContainsFunc(e, func(c ot.Change) bool { return c.Op == ot.OpDel }) {
		return nil
	}
	var out ot.Edit
	for _, s := range tree.Children("deck") {
		list := slideAnimations(s)
		kept := without(list, func(e Effect) bool { return e.Target != "" && tree.Node(e.Target) == nil })
		if len(kept) == len(list) {
			continue
		}
		var value json.RawMessage
		if len(kept) > 0 {
			value = mustJSON(kept)
		}
		more := ot.Edit{{Op: ot.OpSet, ID: s.ID, Attrs: ot.Values{"animations": value}}}
		if tree.Apply(more) == nil {
			out = append(out, more...)
		}
	}
	return out
}
