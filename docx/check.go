package docx

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/trame/ot"
)

var (
	ErrReadOnly = errors.New("docx: this part of the document cannot be edited")
	ErrNoMedia  = errors.New("docx: no such picture")
)

// holders are the nodes blocks go in.
var holders = map[string]bool{"body": true, "tc": true, "sdt": true, "hdr": true, "ftr": true, "note": true, "comment": true}

var blockTypes = map[string]bool{"text": true, "tbl": true, "sdt": true, "other": true}

// editable are the attributes a change may set, by node type.
var editable = func() map[string]map[string]bool {
	out := map[string]map[string]bool{"doc": {"sect": true, "track": true}}
	for typ, list := range map[string][]tprop{"tbl": tblProps, "tr": trProps, "tc": tcProps} {
		out[typ] = map[string]bool{}
		for _, p := range list {
			if p.write != nil {
				out[typ][p.key] = true
			}
		}
	}
	out["tbl"]["grid"] = true
	out["comment"] = map[string]bool{"done": true}
	return out
}()

// Check tells whether an edit only changes what can be edited: the blocks
// of the body, headers and footers, the sections, not the styles; the
// comments it adds are signed by author.
func (d *Document) Check(tree *ot.Tree, e ot.Edit, author string) error {
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
			ok := blockTypes[c.Type] && holders[parent] ||
				c.Type == "tr" && parent == "tbl" ||
				c.Type == "tc" && parent == "tr" ||
				c.Type == "other" && (parent == "tbl" || parent == "tr") ||
				(c.Type == "hdr" || c.Type == "ftr") && c.Parent == "doc" ||
				c.Type == "comment" && c.Parent == "doc" && newComment(c.Attrs, author, typeOf)
			if !ok || (c.Type == "text") != (c.Text != nil) || !d.signed(c.Text, author) {
				return ErrReadOnly
			}
			created[c.ID] = c.Type
		case ot.OpDel:
			switch typeOf(c.ID) {
			case "doc", "body", "hdr", "ftr", "note":
				return ErrReadOnly
			}
		case ot.OpSet:
			t := typeOf(c.ID)
			if t == "" {
				continue
			}
			if c.Key != "" && !blockTypes[t] && t != "tr" && t != "tc" {
				return ErrReadOnly
			}
			for k, v := range c.Attrs {
				if !editable[t][k] || (t == "comment" || k == "track") && !boolean(v) {
					return ErrReadOnly
				}
			}
		case ot.OpTxt:
			if t := typeOf(c.ID); t != "" && t != "text" || !d.signed(c.Text, author) {
				return ErrReadOnly
			}
		default:
			return ErrReadOnly
		}
	}
	return nil
}

// newComment tells whether a comment added is signed by author, dated and
// answers a comment if any.
func newComment(attrs ot.Values, author string, typeOf func(string) string) bool {
	for k, v := range attrs {
		switch k {
		case "author", "date", "initials", "parent":
			var s string
			if json.Unmarshal(v, &s) != nil {
				return false
			}
		case "done":
			if !boolean(v) {
				return false
			}
		default:
			return false
		}
	}
	initials, parent := str(attrs, "initials"), str(attrs, "parent")
	if _, err := time.Parse(time.RFC3339, str(attrs, "date")); err != nil {
		return false
	}
	return str(attrs, "author") == author && utf8.RuneCountInString(initials) <= 9 &&
		strings.IndexFunc(initials, unicode.IsControl) < 0 && (parent == "" || typeOf(parent) == "comment")
}

// boolean is a boolean attribute, or none.
func boolean(v json.RawMessage) bool {
	return v == nil || string(v) == "true" || string(v) == "false"
}

// Media is a picture of the document, by the name its drawings give it,
// and its content type.
func (d *Document) Media(name string) ([]byte, string, error) {
	r, ok := d.names.Lookup("@" + name)
	if !ok || r.Type != partrel.Image || r.External {
		return nil, "", ErrNoMedia
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if p := d.pending[r.Target]; p != nil {
		return p.data, p.contentType, nil
	}
	data, err := d.pkg.Read(r.Target)
	if err != nil {
		return nil, "", errors.Join(ErrNoMedia, err)
	}
	return data, d.pkg.ContentType(r.Target), nil
}
