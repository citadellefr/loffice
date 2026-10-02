package loffice

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/citadellefr/loffice/ot"
	"github.com/citadellefr/loffice/pptx"
)

func TestPresentationsAreEditedAndSaved(t *testing.T) {
	data, err := os.ReadFile("corpus/files/poi/aptia.pptx")
	if err != nil {
		t.Skip("no corpus: corpus/fetch.sh")
	}
	store := newMemStore()
	store.data["deck.pptx"] = data
	h := NewHub(store, fastOptions())

	c, _, doc := join(t, h, "deck.pptx", Peer{ID: "1"})
	var nodes ot.Edit
	if err := json.Unmarshal(doc.D, &nodes); err != nil {
		t.Fatal(err)
	}
	var shape, media string
	for _, n := range nodes {
		if n.Type == "sp" && shape == "" && strings.HasPrefix(n.Parent, "s") && len(n.Text) > 0 {
			shape = n.ID
		}
		if n.Type == "pic" && media == "" {
			var blip struct{ Media string }
			_ = json.Unmarshal(n.Attrs["blip"], &blip)
			media = blip.Media
		}
	}

	c.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"` + shape + `","x":[{"i":"Bref "}]}]}`)
	c.expect("ack")
	c.send(`{"t":"op","n":2,"v":1,"d":[{"o":"set","id":"L1","a":{"name":"x"}}]}`)
	if f := c.expect("nack"); f.Error != pptx.ErrReadOnly.Error() {
		t.Fatalf("nack = %+v", f)
	}
	<-store.saves
	c.expect("saved")

	_, tree, err := pptx.Open([]byte(store.file("deck.pptx")))
	if err != nil {
		t.Fatal(err)
	}
	if got := tree.Node(shape).Text.Delta()[0].Insert; !strings.HasPrefix(got, "Bref ") {
		t.Fatalf("saved text %q", got)
	}

	picture, typ, err := h.Media(context.Background(), "deck.pptx", media)
	if err != nil || len(picture) == 0 || !strings.HasPrefix(typ, "image/") {
		t.Fatalf("media: %d bytes, %q, %v", len(picture), typ, err)
	}
	if _, _, err := h.Media(context.Background(), "deck.pptx", "nothing"); !errors.Is(err, ErrNoMedia) {
		t.Fatalf("unknown media: %v", err)
	}
	c.leave()
}
