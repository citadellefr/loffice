package docx

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// On behalf of: a program that signs revisions with its own name records
// whom it made them for in a "behalf" node of the document, one per author
// and date. Word keeps it as a document variable of settings.xml, which
// survives its saves. The id of the node is made of the author and the
// date, so that the hub refuses a second node of the same two.

// BehalfType is the type of the nodes that tell whom revisions were made for.
const BehalfType = "behalf"

const (
	behalfPrefix = "loffice.behalf."
	maxBehalf    = 256
)

// BehalfNode is the node that says author made the revisions dated date for
// user: its id and attributes, none when one of them is not valid.
func BehalfNode(author, date, user string) (string, ot.Values, bool) {
	t, err := time.Parse(time.RFC3339, date)
	if err != nil || !sane(author) || !sane(user) {
		return "", nil, false
	}
	date = t.UTC().Format(time.RFC3339)
	h := fnv.New32a()
	h.Write([]byte(author))
	id := fmt.Sprintf("behalf-%s-%08x", date, h.Sum32())
	return id, ot.Values{"author": quote(author), "date": quote(date), "user": quote(user)}, true
}

// BehalfOf is the user the revisions of author dated date were made for, ""
// when the document does not say.
func BehalfOf(tree *ot.Tree, author, date string) string {
	id, want, ok := BehalfNode(author, date, "-")
	if !ok {
		return ""
	}
	if n := tree.Node(id); n != nil && n.Type == BehalfType && str(n.Attrs, "author") == str(want, "author") && str(n.Attrs, "date") == str(want, "date") {
		return str(n.Attrs, "user")
	}
	return ""
}

// sane tells whether s is a name or a user fit for a document variable.
func sane(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= maxBehalf && strings.IndexFunc(s, unicode.IsControl) < 0
}

// newBehalf tells whether a node added is the one author's revisions of its
// date are made for.
func newBehalf(c ot.Change, author string) bool {
	id, attrs, ok := BehalfNode(str(c.Attrs, "author"), str(c.Attrs, "date"), str(c.Attrs, "user"))
	if !ok || str(c.Attrs, "author") != author || len(c.Attrs) != len(attrs) || c.ID != id || c.Parent != "doc" || c.Text != nil {
		return false
	}
	for k := range attrs {
		if str(c.Attrs, k) != str(attrs, k) {
			return false
		}
	}
	return true
}

func quote(s string) json.RawMessage {
	b, _ := json.Marshal(s)
	return b
}

// behalf reads the document variables that say whom revisions were made
// for, into nodes of the document keyed after last.
func (r *reader) behalf(settings *xmldom.Element) {
	last := ""
	for _, c := range r.nodes {
		if c.Parent == "doc" {
			last = max(last, c.Key)
		}
	}
	for _, v := range elements(child(settings, "docVars")) {
		name, ok := strings.CutPrefix(attr(v, "name"), behalfPrefix)
		if v.Space != NS || v.Local != "docVar" || !ok {
			continue
		}
		var attrs ot.Values
		if json.Unmarshal([]byte(attr(v, "val")), &attrs) != nil {
			continue
		}
		if id, _, ok := BehalfNode(str(attrs, "author"), str(attrs, "date"), str(attrs, "user")); ok && id == name && len(attrs) == 3 {
			last = ot.KeyBetween(last, "")
			r.addID(id, BehalfType, "doc", last, attrs, nil)
		}
	}
}

// behalves are the nodes that say whom revisions were made for.
func behalves(tree *ot.Tree) []*ot.Node {
	var out []*ot.Node
	for _, n := range tree.Children("doc") {
		if n.Type == BehalfType {
			out = append(out, n)
		}
	}
	return out
}

// liveBehalf are the nodes that say whom revisions were made for, those a
// revision of the saved package still carries the author and date of.
func (w *writer) liveBehalf() ([]*ot.Node, error) {
	nodes := behalves(w.tree)
	if len(nodes) == 0 {
		return nil, nil
	}
	signed := map[string]bool{}
	for _, name := range w.pkg.Names() {
		if !strings.Contains(w.pkg.ContentType(name), "wordprocessingml.") {
			continue
		}
		data, err := w.pkg.Read(name)
		if err != nil {
			return nil, err
		}
		doc, err := xmldom.Parse(data)
		if err != nil {
			return nil, err
		}
		collectSigned(doc.Root, signed)
	}
	return slices.DeleteFunc(nodes, func(n *ot.Node) bool {
		return !signed[str(n.Attrs, "author")+"\x00"+str(n.Attrs, "date")]
	}), nil
}

// collectSigned adds to signed the author and date, as a behalf node has
// them, of the revisions under e.
func collectSigned(e *xmldom.Element, signed map[string]bool) {
	if e.Space == NS && revisions[e.Local] {
		date := attr(e, "date")
		if t, err := time.Parse(time.RFC3339, date); err == nil {
			date = t.UTC().Format(time.RFC3339)
		}
		signed[attr(e, "author")+"\x00"+date] = true
	}
	for _, c := range e.Elements() {
		collectSigned(c, signed)
	}
}

// sameBehalf tells whether nodes say whom revisions were made for as the
// document the tree was read from did.
func (w *writer) sameBehalf(nodes []*ot.Node) bool {
	a := behalves(w.d.loaded)
	if len(a) != len(nodes) {
		return false
	}
	for i := range a {
		if a[i].ID != nodes[i].ID {
			return false
		}
	}
	return true
}

// writeBehalf makes the document variables of settings those of nodes: the
// others of this program go, and those of other programs are kept.
func (w *writer) writeBehalf(root *xmldom.Element, nodes []*ot.Node) {
	vars := child(root, "docVars")
	for _, v := range elements(vars) {
		if strings.HasPrefix(attr(v, "name"), behalfPrefix) {
			vars.Remove(v)
		}
	}
	if len(nodes) > 0 && vars == nil {
		vars = newW(root, "docVars")
		insert(root, vars, settingsOrder)
	}
	for _, n := range nodes {
		val, _ := json.Marshal(n.Attrs)
		vars.Append(newW(vars, "docVar", "name", behalfPrefix+n.ID, "val", string(val)))
	}
	if vars != nil && len(vars.Content) == 0 {
		root.Remove(vars)
	}
}
