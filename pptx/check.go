package pptx

import (
	"errors"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/trame/ot"
)

var (
	ErrReadOnly = errors.New("pptx: this part of the presentation cannot be edited")
	ErrNoMedia  = errors.New("pptx: no such picture")
)

// shapeTypes are the nodes a slide or a group can hold.
var shapeTypes = map[string]bool{"sp": true, "pic": true, "cxn": true, "grp": true, "frame": true, "alt": true, "other": true}

// editable are the attributes a change may set, by node type.
var editable = map[string]map[string]bool{
	"slide": {"name": true, "hidden": true, "bg": true, "layout": true, "transition": true},
	"sp":    {"name": true, "descr": true, "hidden": true, "xfrm": true, "geom": true, "fill": true, "line": true, "body": true},
	"pic":   {"name": true, "descr": true, "hidden": true, "xfrm": true, "geom": true, "fill": true, "line": true, "blip": true},
	"cxn":   {"name": true, "descr": true, "hidden": true, "xfrm": true, "geom": true, "line": true},
	"grp":   {"name": true, "descr": true, "hidden": true, "xfrm": true, "fill": true},
	"frame": {"name": true, "descr": true, "hidden": true, "xfrm": true, "tbl": true, "grid": true},
	"tr":    {"h": true},
	"tc": {"gridSpan": true, "rowSpan": true, "hMerge": true, "vMerge": true, "fill": true, "mar": true, "anchor": true, "vert": true,
		"lnL": true, "lnR": true, "lnT": true, "lnB": true, "lnTlToBr": true, "lnBlToTr": true},
}

// tableTypes are the rows of a table and their cells, by the type of node
// they are under.
var tableTypes = map[string]string{"tr": "frame", "tc": "tr"}

// Check tells whether an edit only changes what can be edited: the slides,
// their shapes and the text of their notes, not the masters and layouts.
func (d *Document) Check(tree *ot.Tree, e ot.Edit) error {
	created := map[string]string{}
	typeOf := func(id string) string {
		if t, ok := created[id]; ok {
			return t
		}
		if n := tree.Node(id); n != nil {
			return n.Type
		}
		return ""
	}
	for _, c := range e {
		switch c.Op {
		case ot.OpNew:
			parent := typeOf(c.Parent)
			ok := c.Type == "slide" && c.Parent == "deck" ||
				shapeTypes[c.Type] && (parent == "slide" || parent == "grp") ||
				tableTypes[c.Type] != "" && tableTypes[c.Type] == parent
			if !ok {
				return ErrReadOnly
			}
			created[c.ID] = c.Type
		case ot.OpDel:
			if t := typeOf(c.ID); t != "" && t != "slide" && !shapeTypes[t] && tableTypes[t] == "" {
				return ErrReadOnly
			}
		case ot.OpSet:
			t := typeOf(c.ID)
			if t == "" {
				continue
			}
			if t != "slide" && !shapeTypes[t] && tableTypes[t] == "" {
				return ErrReadOnly
			}
			for k := range c.Attrs {
				if !editable[t][k] {
					return ErrReadOnly
				}
			}
			if !d.inSlide(tree, created, c.ID) {
				return ErrReadOnly
			}
		case ot.OpTxt:
			if t := typeOf(c.ID); t != "" && t != "sp" && t != "tc" && t != "notes" {
				return ErrReadOnly
			}
		}
		if c.Op != ot.OpNew && c.Op != ot.OpSet && !d.inSlide(tree, created, c.ID) {
			return ErrReadOnly
		}
	}
	return nil
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
		return nil, "", ErrNoMedia
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	data, err := d.pkg.Read(r.Target)
	if err != nil {
		return nil, "", errors.Join(ErrNoMedia, err)
	}
	return data, d.pkg.ContentType(r.Target), nil
}
