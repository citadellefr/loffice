package loffice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
	"github.com/citadellefr/trame/trametest"
)

func join(t *testing.T, h *Hub, key string, info Peer) (*trametest.Client, trametest.Frame, trametest.Frame) {
	t.Helper()
	return trametest.Join(t, func(c *trametest.Conn) error { return h.Serve(context.Background(), c, key, info) })
}

// open reads a file with the default limits.
func open(key string, data []byte) (*ot.Tree, trame.File, error) {
	return Limits{}.withDefaults().open(key, data)
}

func fastOptions() Options {
	return Options{SaveDelay: 20 * time.Millisecond, SaveMaxDelay: 100 * time.Millisecond}
}

func TestUnsupportedFilesAreRefused(t *testing.T) {
	h := NewHub(trametest.NewStore(), Options{}, Limits{})
	c := trametest.Connect(t, func(c *trametest.Conn) error { return h.Serve(context.Background(), c, "a.odt", Peer{ID: "1"}) })
	if err := <-c.Done; err == nil {
		t.Fatal("an .odt file opened")
	}
	if code, _ := c.Conn.CloseFrame(); code != CloseLoadFailed {
		t.Fatalf("close code %d", code)
	}
}

func TestFilesOverTheLimitAreRefused(t *testing.T) {
	store := trametest.NewStore()
	store.Data["big.txt"] = []byte("0123456789")
	store.Data["small.txt"] = []byte("0123")
	h := NewHub(store, fastOptions(), Limits{MaxFileSize: 5})

	c := trametest.Connect(t, func(c *trametest.Conn) error { return h.Serve(context.Background(), c, "big.txt", Peer{ID: "1"}) })
	if err := <-c.Done; !errors.Is(err, ErrTooLarge) || !strings.Contains(err.Error(), "10 bytes, the limit is 5") {
		t.Fatalf("a file over the limit opened: %v", err)
	}
	if code, _ := c.Conn.CloseFrame(); code != CloseLoadFailed {
		t.Fatalf("close code %d", code)
	}
	join(t, h, "small.txt", Peer{ID: "2"})
}

func TestDefaultFileSize(t *testing.T) {
	big := make([]byte, 33<<20)
	if _, _, err := open("big.txt", big); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("a file of 33 MiB opened: %v", err)
	}
}
