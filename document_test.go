package loffice

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/docx"
	"github.com/citadellefr/trame/ot"
	"github.com/citadellefr/trame/trametest"
)

func TestDocumentsAreEditedAndSaved(t *testing.T) {
	data, err := os.ReadFile("corpus/files/python-docx/having-images.docx")
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	store := trametest.NewStore()
	store.Data["letter.docx"] = data
	h := NewHub(store, fastOptions())

	c, _, doc := join(t, h, "letter.docx", Peer{ID: "1"})
	var nodes ot.Edit
	if err := json.Unmarshal(doc.D, &nodes); err != nil {
		t.Fatal(err)
	}
	var text, media string
	for _, n := range nodes {
		if n.Type == "text" && n.Parent == "body" && text == "" {
			text = n.ID
		}
		for _, o := range n.Text {
			var p docx.Picture
			if json.Unmarshal([]byte(o.Attrs["img"]), &p) == nil && p.Media != "" && media == "" {
				media = p.Media
			}
		}
	}

	c.Send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"` + text + `","x":[{"i":"Bref ","a":{"b":"1"}}]}]}`)
	c.Expect("ack")
	c.Send(`{"t":"op","n":2,"v":1,"d":[{"o":"set","id":"doc","a":{"styles":{}}}]}`)
	if f := c.Expect("nack"); f.Error != docx.ErrReadOnly.Error() {
		t.Fatalf("nack = %+v", f)
	}
	<-store.Saves
	c.Expect("saved")

	_, tree, err := docx.Open([]byte(store.File("letter.docx")))
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Node(text).Text.Delta()[0]; !strings.HasPrefix(got.Insert, "Bref ") || got.Attrs["b"] != "1" {
		t.Fatalf("saved text %+v", got)
	}
	added, err := h.AddPicture(context.Background(), "letter.docx", []byte("\x89PNG\r\n\x1a\nnew picture"))
	if err != nil || added == "" {
		t.Fatalf("picture added: %q %v", added, err)
	}
	if _, err := h.AddPicture(context.Background(), "letter.docx", []byte("text")); !errors.Is(err, ErrPicture) {
		t.Fatalf("text added as a picture: %v", err)
	}
	picture, typ, err := h.Media(context.Background(), "letter.docx", media)
	if err != nil || len(picture) == 0 || !strings.HasPrefix(typ, "image/") {
		t.Fatalf("media %q: %d bytes, %q, %v", media, len(picture), typ, err)
	}
	c.Leave()
}
