package pptx

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/internal/partrel"
	"github.com/citadellefr/loffice/internal/xmlcanon"
	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/trame/ot"
)

func presentations(t *testing.T) []string {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	files, _ := filepath.Glob("../corpus/files/*/*")
	if len(files) == 0 {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	var out []string
	for _, f := range files {
		switch strings.ToLower(filepath.Ext(f)) {
		case ".pptx", ".pptm", ".potx", ".ppsx":
			out = append(out, f)
		}
	}
	return out
}

// save is Save, every slide rewritten when force is set.
func save(d *Document, tree *ot.Tree, force bool) ([]byte, error) {
	pkg, err := opc.Open(d.original, Limits)
	if err != nil {
		return nil, err
	}
	w := &writer{d: d, pkg: pkg, tree: tree, fresh: map[*xmldom.Element]bool{}, force: force}
	if err := w.save(); err != nil {
		return nil, err
	}
	return pkg.Bytes()
}

func TestCorpusOpenAndSave(t *testing.T) {
	var opened, refused, nodes int
	reasons := map[string]int{}
	for _, f := range presentations(t) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d, tree, err := Open(data)
		if err != nil {
			refused++
			var reason string
			switch {
			case errors.Is(err, opc.ErrCompound), errors.Is(err, opc.ErrInvalid):
				reason = "package"
			default:
				reason = err.Error()
			}
			reasons[reason]++
			continue
		}
		opened++
		nodes += len(tree.Edit())
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		samePackage(t, f, data, saved)
	}
	t.Logf("%d opened, %d nodes; %d refused: %v", opened, nodes, refused, reasons)
}

// samePackage checks that two packages hold the same parts, byte for byte.
func samePackage(t *testing.T, name string, a, b []byte) {
	t.Helper()
	pa, err := opc.Open(a, Limits)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := opc.Open(b, Limits)
	if err != nil {
		t.Fatalf("%s: saved package does not open: %v", name, err)
	}
	if strings.Join(pa.Names(), " ") != strings.Join(pb.Names(), " ") {
		t.Fatalf("%s: parts differ", name)
	}
	for _, n := range pa.Names() {
		x, _ := pa.Read(n)
		y, _ := pb.Read(n)
		if !bytes.Equal(x, y) {
			t.Fatalf("%s: %s changed", name, n)
		}
	}
}

// Every slide rewritten from the tree means what it meant, but for runs of
// the same formatting side by side, which become one. When LOFFICE_OUT is
// set, the packages are written there for the validator.
func TestCorpusRewrite(t *testing.T) {
	out := os.Getenv("LOFFICE_OUT")
	var files, parts int
	for _, f := range presentations(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data)
		if err != nil {
			continue
		}
		saved, err := save(d, tree, true)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		files++
		before, _ := opc.Open(data, Limits)
		after, err := opc.Open(saved, Limits)
		if err != nil {
			t.Fatalf("%s: rewritten package does not open: %v", f, err)
		}
		for _, p := range d.slides {
			for _, name := range []string{p.name, p.notes} {
				if name == "" {
					continue
				}
				parts++
				x := targets(t, before, name)
				y := targets(t, after, name)
				if y == nil {
					t.Fatalf("%s: %s lost", f, name)
				}
				if err := sameMeaning(x, y); err != nil {
					t.Fatalf("%s %s: %v", f, name, err)
				}
			}
		}
		if _, _, err := Open(saved); err != nil {
			t.Fatalf("%s: rewritten package does not open: %v", f, err)
		}
		if out != "" {
			rel, _ := filepath.Rel(filepath.Join("..", "corpus", "files"), f)
			dst := filepath.Join(out, rel)
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dst, saved, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Logf("%d presentations, %d slides and notes rewritten", files, parts)
}

// targets is a part with its relationship ids replaced by what they point
// to, nil when it is missing.
func targets(t *testing.T, pkg *opc.Package, name string) []byte {
	data, err := pkg.Read(name)
	if err != nil {
		return nil
	}
	doc, err := xmldom.Parse(data)
	if err != nil {
		return data
	}
	rels, err := partrel.Read(pkg, name)
	if err != nil {
		t.Fatal(err)
	}
	spaces := standardSpaces()
	for k, v := range doc.Root.Spaces() {
		spaces[k] = v
	}
	partrel.NewNames(pkg).NameAll(doc.Root, name, rels, spaces)
	return doc.Bytes()
}

func sameMeaning(a, b []byte) error {
	da, err := xmldom.Parse(a)
	if err != nil {
		return nil
	}
	db, err := xmldom.Parse(b)
	if err != nil {
		return err
	}
	normalize(da.Root)
	normalize(db.Root)
	return xmlcanon.Diff(da.Bytes(), db.Bytes())
}

// normalize merges the runs of a paragraph that have the same properties
// and drops empty ones, as a flow does, and drops comments, which a shape
// does not keep.
func normalize(e *xmldom.Element) {
	kept := e.Content[:0]
	for _, c := range e.Content {
		if r, ok := c.(xmldom.Raw); ok && bytes.HasPrefix(r, []byte("<!--")) {
			continue
		}
		kept = append(kept, c)
	}
	e.Content = kept
	for _, c := range e.Elements() {
		normalize(c)
	}
	if e.Space != aNS || e.Local != "p" {
		return
	}
	var out []xmldom.Node
	var last *xmldom.Element
	var lastText string
	var lastProps []byte
	flush := func() {
		if last != nil {
			t := xmldom.New(aNS, "a:t")
			t.Append(xmldom.EscapeText(lastText))
			last.Append(t)
		}
		last = nil
	}
	for _, c := range e.Content {
		el, ok := c.(*xmldom.Element)
		if !ok {
			if len(bytes.TrimSpace(c.(xmldom.Raw))) > 0 {
				flush()
				out = append(out, c)
			}
			continue
		}
		if el.Space != aNS || el.Local != "r" {
			flush()
			out = append(out, el)
			continue
		}
		var s string
		if t := el.Child(aNS, "t"); t != nil {
			s = strings.Map(func(r rune) rune {
				if r == '\n' || r == '\r' || r == '\v' || r == 0xFFFC {
					return ' '
				}
				return r
			}, t.Text())
		}
		if s == "" {
			continue
		}
		var props []byte
		rPr := el.Child(aNS, "rPr")
		if rPr != nil {
			props = rPr.Bytes()
		}
		if last != nil && bytes.Equal(props, lastProps) {
			lastText += s
			continue
		}
		flush()
		last = xmldom.New(aNS, "a:r")
		if rPr != nil {
			last.Append(rPr)
		}
		lastText, lastProps = s, props
		out = append(out, last)
	}
	flush()
	e.Content = out
}
