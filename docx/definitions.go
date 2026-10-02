package docx

import (
	"slices"
	"strconv"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmldom"
)

const (
	typeStyles    = "application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"
	typeNumbering = "application/vnd.openxmlformats-officedocument.wordprocessingml.numbering+xml"
)

// useStyle notes a builtin style a paragraph takes, for styles.xml to get
// it.
func (w *writer) useStyle(id string) {
	for _, b := range builtinStyles {
		if b.id == id && !slices.Contains(w.styles, id) {
			w.styles = append(w.styles, id)
		}
	}
}

// list is the number of a builtin list in numbering.xml, which gets it the
// first time a paragraph asks for it.
func (w *writer) list(name string) string {
	if id, ok := w.lists[name]; ok {
		return id
	}
	root := w.numberingRoot()
	abstract, num := -1, 0
	for _, e := range root.Elements() {
		switch {
		case e.Space != NS:
		case e.Local == "abstractNum":
			abstract = max(abstract, number(e, "abstractNumId", 0))
		case e.Local == "num":
			num = max(num, number(e, "numId", 0))
		}
	}
	order := []string{"numPicBullet", "abstractNum", "num", "numIdMacAtCleanup"}
	a := builtin(builtinLists[name])
	setAttr(a, "abstractNumId", strconv.Itoa(abstract+1))
	insert(root, a, order)
	n := newW(root, "num", "numId", strconv.Itoa(num+1))
	n.Append(newW(n, "abstractNumId", "val", strconv.Itoa(abstract+1)))
	insert(root, n, order)
	if w.lists == nil {
		w.lists = map[string]string{}
	}
	w.lists[name] = strconv.Itoa(num + 1)
	return w.lists[name]
}

// numberingRoot is numbering.xml as it is being written, read or made the
// first time.
func (w *writer) numberingRoot() *xmldom.Element {
	if w.numbering != nil {
		return w.numbering.Root
	}
	if name := w.d.mainRels.OfType(w.d.main, relNumbering); name != "" {
		if data, err := w.pkg.Read(name); err == nil {
			if doc, err := xmldom.Parse(data); err == nil && doc.Root.Space == NS {
				w.numbering = doc
				return doc.Root
			}
		}
	}
	root := xmldom.New(NS, "w:numbering")
	root.Set("xmlns:w", NS)
	w.numbering = &xmldom.Document{Prolog: []byte(xmlHeader), Root: root}
	return root
}

// definitions writes the styles and lists paragraphs took that the file
// did not define.
func (w *writer) definitions() error {
	if len(w.styles) > 0 {
		name := w.d.mainRels.OfType(w.d.main, relStyles)
		var doc *xmldom.Document
		if name != "" {
			data, err := w.pkg.Read(name)
			if err != nil {
				return err
			}
			if doc, err = xmldom.Parse(data); err != nil {
				return err
			}
		} else {
			name = "word/styles.xml"
			root := xmldom.New(NS, "w:styles")
			root.Set("xmlns:w", NS)
			doc = &xmldom.Document{Prolog: []byte(xmlHeader), Root: root}
		}
		for _, b := range builtinStyles {
			if slices.Contains(w.styles, b.id) {
				doc.Root.Append(builtin(b.xml))
			}
		}
		declare(doc.Root)
		if err := w.write(name, typeStyles, doc.Bytes(), relStyles); err != nil {
			return err
		}
	}
	if w.numbering != nil {
		name := w.d.mainRels.OfType(w.d.main, relNumbering)
		if name == "" {
			name = "word/numbering.xml"
		}
		declare(w.numbering.Root)
		if err := w.write(name, typeNumbering, w.numbering.Bytes(), relNumbering); err != nil {
			return err
		}
	}
	return nil
}

// write writes a part the main part points to by a relationship of that
// type, adding it and the relationship when they are new.
func (w *writer) write(name, contentType string, data []byte, typ string) error {
	if w.pkg.Has(name) {
		return w.pkg.Set(name, data)
	}
	if err := w.pkg.Add(name, contentType, data); err != nil {
		return err
	}
	rels, err := partrel.Read(w.pkg, w.d.main)
	if err != nil {
		return err
	}
	rw := partrel.NewWriter(w.d.main, rels, w.d.names.Lookup)
	rw.Ensure(partrel.Rel{Type: typ, Target: name})
	return w.putRels(rw)
}
