// Package partrel gives the relationships of the parts of an Office
// document names that hold across parts: "@" and a hash, which does not
// depend on the part an element is in. An element can so move from a part
// to another, and its pictures and links follow; the name of a picture
// stands for its content, which the host serves it by.
package partrel

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"iter"
	"path"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
)

const (
	NS    = "http://schemas.openxmlformats.org/officeDocument/2006/relationships"
	Image = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/image"
)

// Rel is a relationship an element of the document points to.
type Rel struct {
	Type     string
	Target   string // a part name, or a URL when external
	External bool
}

// Rels are the relationships of a part, as read.
type Rels struct {
	List []opc.Relationship
	byID map[string]opc.Relationship
}

// Read reads the relationships of a part, "" for the package's.
func Read(pkg *opc.Package, part string) (*Rels, error) {
	list, err := pkg.Relationships(part)
	if err != nil {
		return nil, err
	}
	return New(list), nil
}

func New(list []opc.Relationship) *Rels {
	r := &Rels{List: list, byID: map[string]opc.Relationship{}}
	for _, x := range list {
		r.byID[x.ID] = x
	}
	return r
}

// Target is the part a relationship of source points to, "" when it points
// outside the package or nowhere.
func (r *Rels) Target(source, id string) string {
	x, ok := r.byID[id]
	if !ok || x.External {
		return ""
	}
	name, err := opc.Resolve(source, x.Target)
	if err != nil {
		return ""
	}
	return name
}

// OfType is the part the first relationship of that type points to.
func (r *Rels) OfType(source, typ string) string {
	for _, x := range r.List {
		if x.Type == typ && !x.External {
			if name, err := opc.Resolve(source, x.Target); err == nil {
				return name
			}
		}
	}
	return ""
}

// Names are the names given to the relationships of a document.
type Names struct {
	pkg    *opc.Package
	byName map[string]Rel
	names  map[Rel]string
}

func NewNames(pkg *opc.Package) *Names {
	return &Names{pkg: pkg, byName: map[string]Rel{}, names: map[Rel]string{}}
}

// Lookup is the relationship a name stands for.
func (n *Names) Lookup(name string) (Rel, bool) {
	r, ok := n.byName[name]
	return r, ok
}

// All are the relationships named so far.
func (n *Names) All() iter.Seq2[string, Rel] {
	return func(yield func(string, Rel) bool) {
		for k, v := range n.byName {
			if !yield(k, v) {
				return
			}
		}
	}
}

// Name gives the relationship id of source its document-wide name, "" when
// there is no such relationship.
func (n *Names) Name(source string, rels *Rels, id string) string {
	x, ok := rels.byID[id]
	if !ok {
		return ""
	}
	r := Rel{Type: x.Type, Target: x.Target, External: x.External}
	if !x.External {
		name, err := opc.Resolve(source, x.Target)
		if err != nil {
			return ""
		}
		r.Target = name
	}
	s, ok := n.names[r]
	if !ok {
		s = nameOf(n.pkg, r)
		n.names[r] = s
	}
	if s != "" {
		if _, known := n.byName[s]; !known {
			n.byName[s] = r
		}
	}
	return s
}

// NameAll replaces, in e and its descendants, the relationship ids of
// source by their names.
func (n *Names) NameAll(e *xmldom.Element, source string, rels *Rels, spaces map[string]string) {
	Walk(e, spaces, func(el *xmldom.Element, attr, value string) {
		if s := n.Name(source, rels, value); s != "" {
			el.Set(attr, s)
		}
	})
}

// Picture names a picture that is not yet in the package, added under
// that target: the name its content gives it, which it keeps once written.
func (n *Names) Picture(target string, data []byte) string {
	s := pictureName(crc32.ChecksumIEEE(data), uint64(len(data)))
	if _, known := n.byName[s]; !known {
		n.byName[s] = Rel{Type: Image, Target: target}
	}
	return s
}

func pictureName(crc uint32, size uint64) string {
	sum := sha256.Sum256(binary.BigEndian.AppendUint64(binary.BigEndian.AppendUint32(nil, crc), size))
	return "@" + base62(sum[:])
}

// nameOf is the name of a relationship: made of the checksum and size of
// the content of a picture, of its type and target otherwise.
func nameOf(pkg *opc.Package, r Rel) string {
	if r.Type == Image && !r.External {
		crc, size, ok := pkg.Checksum(r.Target)
		if !ok {
			data, err := pkg.Read(r.Target)
			if err != nil {
				return ""
			}
			crc, size = crc32.ChecksumIEEE(data), uint64(len(data))
		}
		return pictureName(crc, size)
	}
	sum := sha256.Sum256([]byte(r.Type + "\x00" + r.Target + "\x00" + strconv.FormatBool(r.External)))
	return "@" + base62(sum[:])
}

func base62(b []byte) string {
	const digits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	v := binary.BigEndian.Uint64(b)
	out := make([]byte, 11)
	for i := range out {
		out[i] = digits[v%62]
		v /= 62
	}
	return string(out)
}

// Walk calls f for every attribute in the relationships namespace of e and
// its descendants, prefixes resolved from spaces and the declarations met
// on the way.
func Walk(e *xmldom.Element, spaces map[string]string, f func(e *xmldom.Element, attr, value string)) {
	local := spaces
	for _, a := range e.Attrs {
		if a.Name == "xmlns" || strings.HasPrefix(a.Name, "xmlns:") {
			local = e.Spaces()
			for k, v := range spaces {
				if _, set := local[k]; !set {
					local[k] = v
				}
			}
			break
		}
	}
	for _, a := range e.Attrs {
		prefix, _, found := strings.Cut(a.Name, ":")
		if found && local[prefix] == NS {
			f(e, a.Name, a.Value)
		}
	}
	for _, c := range e.Elements() {
		Walk(c, local, f)
	}
}

// Writer gives the relationships of a part being written their ids,
// keeping those it had.
type Writer struct {
	lookup  func(name string) (Rel, bool)
	Source  string
	List    []opc.Relationship
	Changed bool
}

// NewWriter starts from the relationships the part has, if any; lookup
// tells what the names met stand for.
func NewWriter(source string, rels *Rels, lookup func(name string) (Rel, bool)) *Writer {
	w := &Writer{lookup: lookup, Source: source}
	if rels != nil {
		w.List = append([]opc.Relationship(nil), rels.List...)
	}
	return w
}

// ID is the relationship id of a name, added to the part when it had none;
// "" when the name is unknown.
func (w *Writer) ID(name string) string {
	r, ok := w.lookup(name)
	if !ok {
		return ""
	}
	return w.Ensure(r)
}

// Ensure is the id of a relationship to r, added if the part has none.
func (w *Writer) Ensure(r Rel) string {
	for _, x := range w.List {
		if x.Type == r.Type && x.External == r.External && w.same(x, r) {
			return x.ID
		}
	}
	target := r.Target
	if !r.External {
		target = Relative(w.Source, r.Target)
	}
	id := w.free()
	w.List = append(w.List, opc.Relationship{ID: id, Type: r.Type, Target: target, External: r.External})
	w.Changed = true
	return id
}

// Drop removes the relationships of a type whose target is not keep's.
func (w *Writer) Drop(typ string, keep Rel) {
	out := w.List[:0]
	for _, x := range w.List {
		if x.Type == typ && !w.same(x, keep) {
			w.Changed = true
			continue
		}
		out = append(out, x)
	}
	w.List = out
}

func (w *Writer) same(x opc.Relationship, r Rel) bool {
	if r.External {
		return x.Target == r.Target
	}
	name, err := opc.Resolve(w.Source, x.Target)
	return err == nil && name == r.Target
}

func (w *Writer) free() string {
	used := map[string]bool{}
	for _, x := range w.List {
		used[x.ID] = true
	}
	for n := len(w.List) + 1; ; n++ {
		if id := fmt.Sprintf("rId%d", n); !used[id] {
			return id
		}
	}
}

// Resolve replaces the names in e by relationship ids of the part, and
// drops the attributes whose name it does not know.
func (w *Writer) Resolve(e *xmldom.Element, spaces map[string]string) {
	Walk(e, spaces, func(el *xmldom.Element, attr, value string) {
		if !strings.HasPrefix(value, "@") {
			return
		}
		if id := w.ID(value); id != "" {
			el.Set(attr, id)
		} else {
			el.Unset(attr)
		}
	})
}

// Relative is the target of a relationship from source to the part name.
func Relative(source, name string) string {
	from := strings.Split(path.Dir(source), "/")
	to := strings.Split(name, "/")
	if path.Dir(source) == "." {
		from = nil
	}
	i := 0
	for i < len(from) && i < len(to)-1 && from[i] == to[i] {
		i++
	}
	return strings.Repeat("../", len(from)-i) + strings.Join(to[i:], "/")
}
