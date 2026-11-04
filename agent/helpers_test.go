package agent

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
	"github.com/citadellefr/trame/trametest"
)

var update = flag.Bool("update", false, "rewrite the files of testdata")

const ns = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml" ` +
	`xmlns:w15="http://schemas.microsoft.com/office/word/2012/wordml"`

const sampleBody = `<w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Rapport</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t xml:space="preserve">Le projet est </w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>en avance</w:t></w:r>` +
	`<w:r><w:t xml:space="preserve"> et </w:t></w:r><w:r><w:rPr><w:i/></w:rPr><w:t>sous budget</w:t></w:r><w:r><w:t>.</w:t></w:r></w:p>` +
	`<w:p><w:pPr><w:pStyle w:val="Heading2"/></w:pPr><w:r><w:t>Risques</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t xml:space="preserve">Le fournisseur </w:t></w:r>` +
	`<w:del w:id="1" w:author="Alice" w:date="2026-10-01T09:00:00Z"><w:r><w:delText xml:space="preserve">pourrait </w:delText></w:r></w:del>` +
	`<w:ins w:id="2" w:author="Alice" w:date="2026-10-01T09:00:00Z"><w:r><w:t xml:space="preserve">va </w:t></w:r></w:ins>` +
	`<w:r><w:t>livrer en retard.</w:t></w:r></w:p>` +
	`<w:p><w:r><w:t xml:space="preserve">Le </w:t></w:r><w:commentRangeStart w:id="0"/><w:r><w:t>budget</w:t></w:r><w:commentRangeEnd w:id="0"/>` +
	`<w:r><w:commentReference w:id="0"/></w:r><w:r><w:t xml:space="preserve"> reste à valider.</w:t></w:r></w:p>` +
	`<w:tbl><w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr><w:tblGrid><w:gridCol w:w="4000"/><w:gridCol w:w="4000"/></w:tblGrid>` +
	`<w:tr><w:tc><w:p><w:r><w:t>Lot</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Date</w:t></w:r></w:p></w:tc></w:tr>` +
	`<w:tr><w:tc><w:p><w:r><w:t>Étude</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Mars</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
	`<w:p><w:r><w:t>Fin du rapport.</w:t></w:r></w:p>`

// buildSample is the Word document of testdata/sample.docx: headings, bold
// and italic, a revision of Alice, a comment of Alice Bob answers, a table.
func buildSample() ([]byte, error) {
	const head = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	parts := []struct{ name, data string }{
		{"[Content_Types].xml", head + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>` +
			`<Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>` +
			`<Override PartName="/word/comments.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.comments+xml"/>` +
			`<Override PartName="/word/commentsExtended.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.commentsExtended+xml"/></Types>`},
		{"_rels/.rels", head + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`},
		{"word/_rels/document.xml.rels", head + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>` +
			`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/comments" Target="comments.xml"/>` +
			`<Relationship Id="rId3" Type="http://schemas.microsoft.com/office/2011/relationships/commentsExtended" Target="commentsExtended.xml"/></Relationships>`},
		{"word/styles.xml", head + `<w:styles ` + ns + `><w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/></w:style>` +
			`<w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:pPr><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:b/><w:sz w:val="32"/></w:rPr></w:style>` +
			`<w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:pPr><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:b/><w:sz w:val="28"/></w:rPr></w:style></w:styles>`},
		{"word/comments.xml", head + `<w:comments ` + ns + `>` +
			`<w:comment w:id="0" w:author="Alice" w:date="2026-10-01T10:00:00Z" w:initials="A"><w:p w14:paraId="00000001"><w:r><w:annotationRef/></w:r><w:r><w:t>Quel montant ?</w:t></w:r></w:p></w:comment>` +
			`<w:comment w:id="3" w:author="Bob" w:date="2026-10-02T10:00:00Z" w:initials="B"><w:p w14:paraId="00000002"><w:r><w:t>À confirmer.</w:t></w:r></w:p></w:comment></w:comments>`},
		{"word/commentsExtended.xml", head + `<w15:commentsEx ` + ns + `><w15:commentEx w15:paraId="00000001" w15:done="0"/>` +
			`<w15:commentEx w15:paraId="00000002" w15:paraIdParent="00000001" w15:done="0"/></w15:commentsEx>`},
		{"word/document.xml", head + `<w:document ` + ns + `><w:body>` + sampleBody +
			`<w:sectPr><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="1417" w:right="1417" w:bottom="1417" w:left="1417" w:header="708" w:footer="708" w:gutter="0"/></w:sectPr></w:body></w:document>`},
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, p := range parts {
		w, err := z.CreateHeader(&zip.FileHeader{Name: p.name, Method: zip.Deflate, Modified: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.data)); err != nil {
			return nil, err
		}
	}
	err := z.Close()
	return b.Bytes(), err
}

func sample(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("testdata", "sample.docx")
	if *update {
		data, err := buildSample()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// golden compares got with the file of testdata, which -update rewrites.
func golden(t *testing.T, name, got string) {
	t.Helper()
	got = newIDs.ReplaceAllString(got, "[new]")
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s:\n%s\nwant:\n%s", name, got, want)
	}
}

// newIDs are the random ids of the comments the agent adds.
var newIDs = regexp.MustCompile(`\[[A-Z2-7]{26}\]`)

const key = "rapport.docx"

type room struct {
	t     *testing.T
	hub   *loffice.Hub
	store *trametest.Store
	key   string
}

func newRoom(t *testing.T, opt loffice.Options) *room {
	return newRoomOf(t, key, sample(t), opt)
}

// newRoomOf is a room for the document name holding data.
func newRoomOf(t *testing.T, name string, data []byte, opt loffice.Options) *room {
	store := trametest.NewStore()
	store.Data[name] = data
	return &room{t, loffice.NewHub(store, opt, loffice.Limits{}), store, name}
}

func (r *room) join(name string, opt Options) *Agent {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, err := Join(ctx, r.hub, r.key, trame.Peer{ID: name, Name: name}, opt)
	if err != nil {
		r.t.Fatal(err)
	}
	r.t.Cleanup(a.Leave)
	return a
}

// human is a person in the room, through the protocol.
type human struct {
	t *testing.T
	c *trametest.Client
	v uint64
	n uint64
}

func (r *room) human(name string) *human {
	r.t.Helper()
	c, _, doc := trametest.Join(r.t, func(c *trametest.Conn) error {
		return r.hub.Serve(context.Background(), c, r.key, trame.Peer{ID: name, Name: name})
	})
	return &human{t: r.t, c: c, v: doc.V}
}

// next is the next frame of that kind, others' edits followed on the way.
func (h *human) next(kind string) trametest.Frame {
	h.t.Helper()
	for {
		f := h.c.Next()
		if f.T == "op" {
			h.v = f.V
		}
		if f.T == kind {
			return f
		}
	}
}

// edit sends an edit and waits for the hub to take it or refuse it.
func (h *human) edit(e ot.Edit) trametest.Frame {
	h.t.Helper()
	h.send(e)
	for {
		f := h.c.Next()
		switch f.T {
		case "op":
			h.v = f.V
		case "ack":
			h.v = f.V
			return f
		case "nack":
			return f
		}
	}
}

func (h *human) send(e ot.Edit) {
	h.t.Helper()
	d, err := json.Marshal(e)
	if err != nil {
		h.t.Fatal(err)
	}
	h.n++
	f, _ := json.Marshal(map[string]any{"t": "op", "n": h.n, "v": h.v, "d": json.RawMessage(d)})
	h.c.Send(string(f))
}

// quiet fails if an edit arrives soon.
func (h *human) quiet() {
	h.t.Helper()
	deadline := time.After(100 * time.Millisecond)
	for {
		select {
		case raw := <-h.c.Conn.Out:
			var f trametest.Frame
			_ = json.Unmarshal(raw, &f)
			if f.T == "op" {
				h.t.Fatalf("unexpected edit %s", raw)
			}
		case <-deadline:
			return
		}
	}
}

// typeAt is an edit typing text at the start of a paragraph.
func typeAt(a *Agent, block, text string, attrs ot.Attrs) ot.Edit {
	a.mu.Lock()
	defer a.mu.Unlock()
	n, _, start, ok := a.s.paragraph(block)
	if !ok {
		panic("no paragraph " + block)
	}
	var b builder
	b.to(start)
	b.insert(text, attrs)
	return b.edit(n.ID)
}

func read(t *testing.T, a *Agent, view View) string {
	t.Helper()
	_, text, err := a.Read(view)
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// settle waits for the agent to have the version v.
func settle(t *testing.T, a *Agent, v uint64) {
	t.Helper()
	for range 200 {
		a.mu.Lock()
		got := a.s.version
		a.mu.Unlock()
		if got >= v {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("agent never got version %d", v)
}
