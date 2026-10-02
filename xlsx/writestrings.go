package xlsx

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmltok"
)

// stringWriter is the table of shared strings as written: the one read,
// followed by the strings the sheets rewritten add to it, so that the
// sheets left alone keep theirs.
type stringWriter struct {
	base  *sharedStrings // nil when the workbook had none
	added []sharedString
	index map[string]int
}

func (t *stringWriter) add(item sharedString) int {
	key := item.text
	if item.rich != "" {
		key = "\x00" + item.rich
	}
	if t.base != nil {
		if i, ok := t.base.index[key]; ok {
			return i
		}
	}
	if i, ok := t.index[key]; ok {
		return i
	}
	i := len(t.added)
	if t.base != nil {
		i += len(t.base.items)
	}
	t.added = append(t.added, item)
	t.index[key] = i
	return i
}

func (t *stringWriter) changed() bool {
	return len(t.added) > 0
}

// sharedStrings writes the table with the strings added.
func (w *writer) sharedStrings(sheets []sheet) error {
	refs := 0
	for _, s := range sheets {
		if n, ok := w.stringRefs[s.part]; ok {
			refs += n
		} else {
			refs += s.part.strings
		}
	}
	unique := len(w.strings.added)
	name := "xl/sharedStrings.xml"
	data := []byte(xmlHeader + `<sst xmlns="` + mainNS + `"/>`)
	if base := w.strings.base; base != nil {
		name = base.name
		unique += len(base.items)
		var err error
		if data, err = w.pkg.Read(name); err != nil {
			return err
		}
	} else {
		if w.pkg.Has(name) {
			name = w.freeName("xl/sharedStrings%d.xml")
		}
		w.addRel(relSharedStrings, w.target(name))
	}

	s := xmltok.New(data)
	var root xmltok.Token
	end := -1
	for {
		tok, ok := s.Next()
		if !ok {
			break
		}
		switch {
		case tok.Kind == xmltok.StartElement && s.Depth() == 1:
			root = tok
		case tok.Kind == xmltok.EndElement && s.Depth() == 0 && !root.SelfClosing:
			end = tok.Offset
		}
	}
	if s.Err() != nil || root.Name == nil {
		return ErrNotWorkbook
	}
	prefix := ""
	if i := bytes.IndexByte(root.Name, ':'); i >= 0 {
		prefix = string(root.Name[:i+1])
	}

	var b bytes.Buffer
	b.Grow(len(data) + 64*len(w.strings.added))
	b.Write(data[:root.Offset])
	b.WriteByte('<')
	b.Write(root.Name)
	seen := map[string]bool{}
	for n, v := range xmltok.Attrs(root.Data) {
		name := string(n)
		switch name {
		case "count":
			v = strconv.AppendInt(nil, int64(refs), 10)
		case "uniqueCount":
			v = strconv.AppendInt(nil, int64(unique), 10)
		}
		seen[name] = true
		b.WriteByte(' ')
		b.Write(n)
		b.WriteString(`="`)
		b.Write(v)
		b.WriteByte('"')
	}
	if !seen["count"] {
		b.WriteString(` count="` + strconv.Itoa(refs) + `"`)
	}
	if !seen["uniqueCount"] {
		b.WriteString(` uniqueCount="` + strconv.Itoa(unique) + `"`)
	}
	b.WriteByte('>')
	if !root.SelfClosing {
		b.Write(data[root.End:end])
	}
	for _, item := range w.strings.added {
		if item.rich != "" {
			b.WriteString(item.rich)
			continue
		}
		b.WriteString("<" + prefix + "si><" + prefix + "t")
		if item.text != strings.TrimSpace(item.text) {
			b.WriteString(` xml:space="preserve"`)
		}
		b.WriteByte('>')
		b.Write(escapeText(encodeEscapes(item.text)))
		b.WriteString("</" + prefix + "t></" + prefix + "si>")
	}
	if root.SelfClosing {
		b.WriteString("</")
		b.Write(root.Name)
		b.WriteByte('>')
		b.Write(data[root.End:])
	} else {
		b.Write(data[end:])
	}
	return w.put(name, typeSharedStrings, b.Bytes())
}
