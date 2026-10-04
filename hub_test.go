package loffice

import (
	"context"
	"testing"
	"time"

	"github.com/citadellefr/trame/trametest"
)

func join(t *testing.T, h *Hub, key string, info Peer) (*trametest.Client, trametest.Frame, trametest.Frame) {
	t.Helper()
	return trametest.Join(t, func(c *trametest.Conn) error { return h.Serve(context.Background(), c, key, info) })
}

func fastOptions() Options {
	return Options{SaveDelay: 20 * time.Millisecond, SaveMaxDelay: 100 * time.Millisecond}
}

func TestUnsupportedFilesAreRefused(t *testing.T) {
	h := NewHub(trametest.NewStore(), Options{})
	c := trametest.Connect(t, func(c *trametest.Conn) error { return h.Serve(context.Background(), c, "a.odt", Peer{ID: "1"}) })
	if err := <-c.Done; err == nil {
		t.Fatal("an .odt file opened")
	}
	if code, _ := c.Conn.CloseFrame(); code != CloseLoadFailed {
		t.Fatalf("close code %d", code)
	}
}
