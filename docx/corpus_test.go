package docx

import (
	"bytes"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/opc"
	"github.com/citadellefr/loffice/ot"
)

func documents(t *testing.T) []string {
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
		case ".docx", ".docm", ".dotx":
			out = append(out, f)
		}
	}
	return out
}

// save is Save, every part rewritten when force is set.
func save(d *Document, tree *ot.Tree, force bool) ([]byte, error) {
	pkg, err := opc.Open(d.original, Limits)
	if err != nil {
		return nil, err
	}
	w := &writer{d: d, pkg: pkg, tree: tree, force: force, headerRef: func(string) string { return "" }}
	if err := w.save(); err != nil {
		return nil, err
	}
	return pkg.Bytes()
}

func TestCorpusOpenAndSave(t *testing.T) {
	var opened, refused, nodes int
	reasons := map[string]int{}
	for _, f := range documents(t) {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		d, tree, err := Open(data)
		if err != nil {
			refused++
			reason := err.Error()
			if errors.Is(err, opc.ErrCompound) || errors.Is(err, opc.ErrInvalid) {
				reason = "package"
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
		if !bytes.Equal(saved, data) {
			samePackage(t, f, data, saved)
		}
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

// Every part rewritten from the tree reads back as the same tree. When
// LOFFICE_OUT is set, the packages are written there for the validator.
func TestCorpusRewrite(t *testing.T) {
	out := os.Getenv("LOFFICE_OUT")
	var files int
	for _, f := range documents(t) {
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
		_, again, err := Open(saved)
		if err != nil {
			t.Fatalf("%s: rewritten package does not open: %v", f, err)
		}
		if err := sameTree(tree, again); err != nil {
			t.Fatalf("%s: %v", f, err)
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
	t.Logf("%d documents rewritten", files)
}

// sameTree tells how two trees differ, nil if they do not.
func sameTree(a, b *ot.Tree) error {
	x, y := a.Edit(), b.Edit()
	for i := range min(len(x), len(y)) {
		p, _ := json.Marshal(x[i])
		q, _ := json.Marshal(y[i])
		if !bytes.Equal(p, q) {
			at := 0
			for at < min(len(p), len(q)) && p[at] == q[at] {
				at++
			}
			from := max(at-300, 0)
			return errors.New("node differs:\n" + string(p[from:min(at+300, len(p))]) + "\n" + string(q[from:min(at+300, len(q))]))
		}
	}
	if len(x) != len(y) {
		return errors.New("not as many nodes")
	}
	return nil
}

// Every document edited as the editor would, saved and read back. When
// LOFFICE_EDITED is set, the packages are written there for the validator.
func TestCorpusEdit(t *testing.T) {
	out := os.Getenv("LOFFICE_EDITED")
	var files int
	for _, f := range documents(t) {
		data, _ := os.ReadFile(f)
		d, tree, err := Open(data)
		if err != nil {
			continue
		}
		e := edits(tree)
		if len(e) == 0 {
			continue
		}
		picture, err := d.AddPicture(onePixel)
		if err != nil {
			t.Fatal(err)
		}
		e = append(e, ot.Change{Op: ot.OpTxt, ID: e[0].ID, Text: ot.Delta{
			{Retain: 5},
			{Insert: Object, Attrs: ot.Attrs{"img": `{"media":"` + picture + `","w":127000,"h":127000}`}},
		}})
		if err := d.Check(tree, e, "Bref"); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if err := tree.Apply(e); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		files++
		_, again, err := Open(saved)
		if err != nil {
			t.Fatalf("%s: edited package does not open: %v", f, err)
		}
		if got := firstFlow(again); got == nil || !strings.HasPrefix(strings.TrimLeft(concat(got.Text.Delta()), Object), "Bref") {
			t.Fatalf("%s: the edit was lost", f)
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
	t.Logf("%d documents edited", files)
}

// onePixel is a PNG picture of one pixel.
var onePixel = func() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewGray(image.Rect(0, 0, 1, 1)))
	return b.Bytes()
}()

func firstFlow(tree *ot.Tree) *ot.Node {
	for _, n := range tree.Children("body") {
		if n.Type == "text" {
			return n
		}
	}
	return nil
}

// edits types in the first flow of the body, bolds and splits a paragraph
// there, comments it, centers the tables and widens the left margin; it
// resolves the first comment of the document, answers it, and deletes the
// last.
func edits(tree *ot.Tree) ot.Edit {
	n := firstFlow(tree)
	if n == nil {
		return nil
	}
	var e ot.Edit
	flow := n.Text.Delta()
	var marks []ot.Attrs
	for _, o := range flow {
		for range strings.Count(o.Insert, "\n") {
			marks = append(marks, o.Attrs)
		}
	}
	mark := ot.Attrs{}
	for k, v := range marks[0] {
		switch k {
		case "sect", "sx", "ins", "insd", "del", "deld":
		default:
			mark[k] = v
		}
	}
	delta := ot.Delta{
		{Insert: Object, Attrs: ot.Attrs{"cs": "bref-c"}},
		{Insert: "Bref ", Attrs: ot.Attrs{"b": "1", "color": "C00000", "link": "https://citadelle.fr/"}},
		{Insert: Object, Attrs: ot.Attrs{"ce": "bref-c"}},
		{Insert: Object, Attrs: ot.Attrs{"comment": "bref-c"}},
		{Insert: "1", Attrs: ot.Attrs{"field": "PAGE"}},
		{Insert: Object, Attrs: ot.Attrs{"br": "page"}},
		{Insert: "\n", Attrs: ot.Attrs{"pstyle": "Title"}},
		{Insert: "•", Attrs: ot.Attrs{"ins": "Bref", "insd": "2026-09-29T10:00:00Z"}},
		{Insert: "\n", Attrs: ot.Attrs{"num": "bullet", "lvl": "0", "ins": "Bref", "insd": "2026-09-29T10:00:00Z"}},
	}
	if first := len([]rune(strings.SplitN(concat(flow), "\n", 2)[0])); first > 6 {
		delta = append(delta,
			ot.Op{Retain: 3, Attrs: ot.Attrs{"i": "1", "u": "single"}},
			ot.Op{Retain: 2, Attrs: ot.Attrs{"del": "Bref", "deld": "2026-09-29T10:00:00Z"}},
			ot.Op{Insert: "\n", Attrs: mark})
	}
	e = append(e, ot.Change{Op: ot.OpTxt, ID: n.ID, Text: delta})
	e = append(e, ot.Change{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"track": json.RawMessage("true")}})
	comment := func(id, parent string) {
		attrs := ot.Values{"author": json.RawMessage(`"Bref"`), "date": json.RawMessage(`"2026-09-29T10:00:00Z"`)}
		if parent != "" {
			attrs["parent"], _ = json.Marshal(parent)
		}
		e = append(e,
			ot.Change{Op: ot.OpNew, ID: id, Type: "comment", Parent: "doc", Key: ot.KeyBetween(lastKey(tree, "doc"), ""), Attrs: attrs},
			ot.Change{Op: ot.OpNew, ID: id + "-t", Type: "text", Parent: id, Key: "V", Text: ot.Delta{{Insert: "Commentaire\n"}}})
	}
	comment("bref-c", "")
	var comments []*ot.Node
	for _, c := range tree.Children("doc") {
		if c.Type == "comment" {
			comments = append(comments, c)
		}
	}
	if len(comments) > 0 {
		e = append(e, ot.Change{Op: ot.OpSet, ID: comments[0].ID, Attrs: ot.Values{"done": json.RawMessage("true")}})
		comment("bref-r", comments[0].ID)
	}
	if len(comments) > 1 {
		e = append(e, ot.Change{Op: ot.OpDel, ID: comments[len(comments)-1].ID})
	}
	for _, t := range tree.Children("body") {
		if t.Type == "tbl" {
			e = append(e, ot.Change{Op: ot.OpSet, ID: t.ID, Attrs: ot.Values{"jc": json.RawMessage(`"center"`)}})
		}
	}
	for _, note := range tree.Children("doc") {
		if note.Type != "note" {
			continue
		}
		for _, t := range tree.Children(note.ID) {
			if t.Type == "text" {
				e = append(e, ot.Change{Op: ot.OpTxt, ID: t.ID, Text: ot.Delta{{Insert: "Note ", Attrs: ot.Attrs{"i": "1"}}}})
				break
			}
		}
		break
	}
	if s := tree.Node("doc").Attrs["sect"]; s != nil {
		var sect Section
		json.Unmarshal(s, &sect)
		sect.Left += 20
		data, _ := json.Marshal(sect)
		e = append(e, ot.Change{Op: ot.OpSet, ID: "doc", Attrs: ot.Values{"sect": data}})
	}
	return e
}

func lastKey(tree *ot.Tree, parent string) string {
	kids := tree.Children(parent)
	return kids[len(kids)-1].Key
}

func concat(flow ot.Delta) string {
	var b strings.Builder
	for _, o := range flow {
		b.WriteString(o.Insert)
	}
	return b.String()
}
