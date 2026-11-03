package loffice

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
	"github.com/citadellefr/trame/trametest"
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func capture(t *testing.T) *syncBuffer {
	t.Helper()
	logs := &syncBuffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return logs
}

// inject serves the extension with a format that panics, while the file
// reads.
func inject(t *testing.T, ext string, read trame.Format) {
	t.Helper()
	formats[ext] = read
	t.Cleanup(func() { delete(formats, ext) })
}

func TestPanicOfOpenIsRefusedAndOtherRoomsServe(t *testing.T) {
	logs := capture(t)
	inject(t, ".boom", func(string, []byte) (*ot.Tree, trame.File, error) { panic("decoder") })
	store := trametest.NewStore()
	store.Data["a.boom"] = []byte("x")
	store.Data["notes.txt"] = []byte("hello")
	h := NewHub(store, fastOptions())

	c := trametest.Connect(t, func(c *trametest.Conn) error { return h.Serve(context.Background(), c, "a.boom", Peer{ID: "1"}) })
	if err := <-c.Done; err == nil || !strings.Contains(err.Error(), "decoder") {
		t.Fatalf("a file whose decoder panics opened: %v", err)
	}
	if code, _ := c.Conn.CloseFrame(); code != CloseLoadFailed {
		t.Fatalf("close code %d", code)
	}
	if out := logs.String(); !strings.Contains(out, "stack=") {
		t.Fatalf("the stack is not logged: %s", out)
	}

	b, _, _ := join(t, h, "notes.txt", Peer{ID: "2"})
	b.Send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"r":5},{"i":"!"}]}]}`)
	b.Expect("ack")
	b.Leave()
}

func TestPanicOfCheckAndEncodeIsAnError(t *testing.T) {
	logs := capture(t)
	data, err := os.ReadFile("opc/testdata/blank.docx")
	if err != nil {
		t.Fatal(err)
	}
	tree, _, err := open("a.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	// files without the document they wrap panic in every call
	files := map[string]trame.File{
		"document":     document{},
		"presentation": presentation{},
		"workbook":     &workbook{},
	}
	for name, f := range files {
		if _, err := f.Encode(tree); err == nil {
			t.Errorf("%s: Encode: no error", name)
		}
	}
	if err := files["workbook"].Check(tree, ot.Edit{{Op: ot.OpSet, ID: "body"}}, Peer{}); err == nil {
		t.Error("workbook: Check: no error")
	}
	if n := strings.Count(logs.String(), "stack="); n != len(files)+1 {
		t.Fatalf("%d stacks logged: %.300s", n, logs.String())
	}
}

func TestPanicOfEncodeLeavesOtherRoomsServed(t *testing.T) {
	capture(t)
	inject(t, ".boom", func(key string, data []byte) (*ot.Tree, trame.File, error) {
		tree, _, err := trame.Text(key, data)
		return tree, &workbook{}, err
	})
	store := trametest.NewStore()
	store.Data["a.boom"] = []byte("x")
	store.Data["notes.txt"] = []byte("hello")
	h := NewHub(store, fastOptions())

	a, _, _ := join(t, h, "a.boom", Peer{ID: "1"})
	a.Send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"y"}]}]}`)
	a.Expect("nack")
	a.Leave()

	b, _, _ := join(t, h, "notes.txt", Peer{ID: "2"})
	b.Send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"r":5},{"i":"!"}]}]}`)
	b.Expect("ack")
	<-store.Saves
	if got := store.File("notes.txt"); got != "hello!" {
		t.Fatalf("saved %q", got)
	}
	b.Leave()
}

func TestPanicOfFollowStartsTheCalculationOver(t *testing.T) {
	logs := capture(t)
	data, err := os.ReadFile("testdata/xlsx/blank.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	tree, f, err := openWorkbook("book.xlsx", data)
	if err != nil {
		t.Fatal(err)
	}
	w := f.(*workbook)
	follow := func(edit string) ot.Edit {
		t.Helper()
		var e ot.Edit
		if err := json.Unmarshal([]byte(edit), &e); err != nil {
			t.Fatal(err)
		}
		if err := tree.Apply(e); err != nil {
			t.Fatal(err)
		}
		return w.Follow(tree, e, nil, Peer{})
	}
	cell := func(row int) string { return string(tree.Node("S1").Grid.Cell(row, 1)) }

	follow(`[{"o":"cel","id":"S1","c":[[1,1,{"v":2}],[2,1,{"f":"A1*3"}]]}]`)
	if cell(2) != `{"f":"A1*3","v":6}` {
		t.Fatalf("A2: %s", cell(2))
	}

	good := w.calc
	w.calc = nil
	if out := follow(`[{"o":"cel","id":"S1","c":[[1,1,{"v":5}]]}]`); out != nil || !w.stale {
		t.Fatalf("a failed calculation: out %v, stale %v", out, w.stale)
	}
	if cell(2) != `{"f":"A1*3","v":6}` {
		t.Fatalf("A2 after the failure: %s", cell(2))
	}
	if !strings.Contains(logs.String(), "stack=") {
		t.Fatalf("the stack is not logged: %s", logs.String())
	}

	w.calc = good
	out := follow(`[{"o":"cel","id":"S1","c":[[3,1,{"v":1}]]}]`)
	if w.stale || cell(2) != `{"f":"A1*3","v":15}` {
		t.Fatalf("stale %v, A2 %s", w.stale, cell(2))
	}
	if raw, _ := json.Marshal(out); !strings.Contains(string(raw), `"v":15`) {
		t.Fatalf("the values are not sent: %s", raw)
	}
}

// templates are the extensions served beside the documents they derive from:
// a file to take the package from, and the content type of the main part
// as the document has it, then as the template does.
var templates = map[string]struct{ file, document, template string }{
	".dotm": {"opc/testdata/blank.docx", "wordprocessingml.document.main+xml", "ms-word.template.macroEnabledTemplate.main+xml"},
	".xltm": {"testdata/xlsx/blank.xlsx", "spreadsheetml.sheet.main+xml", "ms-excel.template.macroEnabled.main+xml"},
	".potx": {"opc/testdata/blank.pptx", "presentationml.presentation.main+xml", "presentationml.template.main+xml"},
	".potm": {"opc/testdata/blank.pptx", "presentationml.presentation.main+xml", "ms-powerpoint.template.macroEnabled.main+xml"},
}

// retype rewrites the content type of the main part of a package.
func retype(t *testing.T, data []byte, from, to string) []byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, f := range r.File {
		body := unzip(t, f)
		if f.Name == "[Content_Types].xml" {
			if !bytes.Contains(body, []byte(from)) {
				t.Fatalf("no %q in the content types", from)
			}
			body = bytes.Replace(body, []byte(from), []byte(to), 1)
		}
		part, err := w.Create(f.Name)
		if err != nil {
			t.Fatal(err)
		}
		part.Write(body)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func unzip(t *testing.T, f *zip.File) []byte {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	body, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func parts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			out[f.Name] = string(unzip(t, f))
		}
	}
	return out
}

func roundTrip(t *testing.T, name string, data []byte) {
	t.Helper()
	tree, f, err := open(name, data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	saved, err := f.Encode(tree)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	before, after := parts(t, data), parts(t, saved)
	if len(before) != len(after) {
		t.Errorf("%s: %d parts, then %d", name, len(before), len(after))
	}
	for part, body := range before {
		if after[part] != body {
			t.Errorf("%s: %s changed", name, part)
		}
	}
}

func TestTemplatesAreServedAndKeptAsTheyAre(t *testing.T) {
	for ext, c := range templates {
		data, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip(t, "model"+ext, retype(t, data, c.document, c.template))
	}
}

func TestCorpusTemplatesAreKeptAsTheyAre(t *testing.T) {
	if testing.Short() {
		t.Skip("the corpus is not read in -short mode")
	}
	var files []string
	for ext := range templates {
		found, _ := filepath.Glob("corpus/files/*/*" + ext)
		files = append(files, found...)
	}
	if len(files) == 0 {
		t.Skip("no corpus: go run ./tools/corpus")
	}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip(t, filepath.Base(name), data)
	}
}
