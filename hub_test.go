package loffice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/citadellefr/loffice/ot"
)

type fakeConn struct {
	in        chan []byte
	out       chan []byte
	closed    chan struct{}
	closeOnce sync.Once

	mu          sync.Mutex
	closeCode   int
	closeReason string
}

func newFakeConn() *fakeConn {
	return &fakeConn{
		in:     make(chan []byte, 64),
		out:    make(chan []byte, 1024),
		closed: make(chan struct{}),
	}
}

var errConnClosed = errors.New("closed")

func (c *fakeConn) ReadMessage() (int, []byte, error) {
	select {
	case m := <-c.in:
		return textMessage, m, nil
	case <-c.closed:
		return 0, nil, errConnClosed
	}
}

func (c *fakeConn) WriteMessage(_ int, data []byte) error {
	select {
	case <-c.closed:
		return errConnClosed
	case c.out <- data:
		return nil
	}
}

func (c *fakeConn) WriteControl(kind int, data []byte, _ time.Time) error {
	if kind == closeMessage && len(data) >= 2 {
		c.mu.Lock()
		c.closeCode, c.closeReason = int(data[0])<<8|int(data[1]), string(data[2:])
		c.mu.Unlock()
	}
	return nil
}

func (c *fakeConn) SetReadLimit(int64)                        {}
func (c *fakeConn) SetReadDeadline(time.Time) error           { return nil }
func (c *fakeConn) SetWriteDeadline(time.Time) error          { return nil }
func (c *fakeConn) SetPongHandler(func(appData string) error) {}

func (c *fakeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *fakeConn) closeFrame() (int, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCode, c.closeReason
}

type memStore struct {
	mu    sync.Mutex
	data  map[string][]byte
	saves chan string
	fail  error
}

func newMemStore() *memStore {
	return &memStore{data: map[string][]byte{}, saves: make(chan string, 64)}
}

func (s *memStore) Load(_ context.Context, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail != nil {
		return nil, s.fail
	}
	return s.data[key], nil
}

func (s *memStore) Save(_ context.Context, key string, data []byte) error {
	s.mu.Lock()
	err := s.fail
	if err == nil {
		s.data[key] = data
	}
	s.mu.Unlock()
	s.saves <- key
	return err
}

func (s *memStore) setFail(err error) {
	s.mu.Lock()
	s.fail = err
	s.mu.Unlock()
}

func (s *memStore) file(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.data[key])
}

type frame struct {
	T     string          `json:"t"`
	SID   uint32          `json:"sid"`
	Name  string          `json:"name"`
	N     uint64          `json:"n"`
	V     uint64          `json:"v"`
	Ack   uint64          `json:"ack"`
	Epoch string          `json:"epoch"`
	Saved uint64          `json:"saved"`
	Error string          `json:"error"`
	D     json.RawMessage `json:"d"`
	Peers []peerView      `json:"peers"`
	Peer  peerView        `json:"peer"`
}

type client struct {
	t    *testing.T
	conn *fakeConn
	done chan error
}

func connect(t *testing.T, h *Hub, ctx context.Context, key string, info Peer) *client {
	t.Helper()
	c := &client{t: t, conn: newFakeConn(), done: make(chan error, 1)}
	go func() { c.done <- h.Serve(ctx, c.conn, key, info) }()
	t.Cleanup(func() { c.conn.Close() })
	return c
}

// join connects and syncs from scratch, returning the hello and doc frames.
func join(t *testing.T, h *Hub, key string, info Peer) (*client, frame, frame) {
	t.Helper()
	c := connect(t, h, context.Background(), key, info)
	hello := c.expect("hello")
	c.send(`{"t":"sync"}`)
	return c, hello, c.expect("doc")
}

func (c *client) send(s string) { c.conn.in <- []byte(s) }

func (c *client) next() frame {
	c.t.Helper()
	select {
	case raw := <-c.conn.out:
		var f frame
		if err := json.Unmarshal(raw, &f); err != nil {
			c.t.Fatalf("invalid frame %s: %v", raw, err)
		}
		return f
	case <-time.After(2 * time.Second):
		c.t.Fatal("no frame")
		return frame{}
	}
}

func (c *client) expect(kind string) frame {
	c.t.Helper()
	f := c.next()
	if f.T != kind {
		c.t.Fatalf("got %q frame, want %q: %+v", f.T, kind, f)
	}
	return f
}

func (c *client) quiet() {
	c.t.Helper()
	select {
	case raw := <-c.conn.out:
		c.t.Fatalf("unexpected frame %s", raw)
	case <-time.After(50 * time.Millisecond):
	}
}

func (c *client) leave() {
	c.t.Helper()
	c.conn.Close()
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		c.t.Fatal("Serve did not return")
	}
}

func fastOptions() Options {
	return Options{SaveDelay: 20 * time.Millisecond, SaveMaxDelay: 100 * time.Millisecond}
}

func TestEditsAreRebasedAndSaved(t *testing.T) {
	store := newMemStore()
	store.data["a.txt"] = []byte("one\r\ntwo")
	h := NewHub(store, fastOptions())

	alice, hello, doc := join(t, h, "a.txt", Peer{ID: "1", Name: "Alice", Client: "ca"})
	if hello.SID != 1 || hello.Name != "Alice" || len(hello.Peers) != 0 || hello.Epoch == "" {
		t.Fatalf("hello = %+v", hello)
	}
	if string(doc.D) != `[{"o":"new","id":"body","t":"text","k":"V","x":[{"i":"one\ntwo\n"}]}]` || doc.V != 0 {
		t.Fatalf("doc = %+v", doc)
	}
	bob, hello, _ := join(t, h, "a.txt", Peer{ID: "2", Name: "Bob"})
	if len(hello.Peers) != 1 || hello.Peers[0].Name != "Alice" {
		t.Fatalf("hello = %+v", hello)
	}
	if f := alice.expect("join"); f.Peer.Name != "Bob" || f.Peer.SID != 2 {
		t.Fatalf("join = %+v", f)
	}

	// both typed on revision 0: Bob's edit comes second and is rebased
	alice.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"X"}]}]}`)
	if f := alice.expect("ack"); f.N != 1 || f.V != 1 {
		t.Fatalf("ack = %+v", f)
	}
	bob.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"r":3},{"i":"Y"}]}]}`)
	if f := bob.expect("op"); f.SID != 1 || f.V != 1 || string(f.D) != `[{"o":"txt","id":"body","x":[{"i":"X"}]}]` {
		t.Fatalf("op = %+v", f)
	}
	if f := bob.expect("ack"); f.V != 2 {
		t.Fatalf("ack = %+v", f)
	}
	if f := alice.expect("op"); f.SID != 2 || f.V != 2 || string(f.D) != `[{"o":"txt","id":"body","x":[{"r":4},{"i":"Y"}]}]` {
		t.Fatalf("op = %+v", f)
	}

	<-store.saves
	if got := store.file("a.txt"); got != "XoneY\r\ntwo" {
		t.Fatalf("saved %q", got)
	}
	if f := alice.expect("saved"); f.V != 2 {
		t.Fatalf("saved = %+v", f)
	}
	bob.expect("saved")

	bob.leave()
	alice.expect("leave")
}

func TestReconnectCatchesUp(t *testing.T) {
	h := NewHub(newMemStore(), Options{SaveDelay: time.Hour, SaveMaxDelay: time.Hour})
	keeper, hello, _ := join(t, h, "a.txt", Peer{ID: "0"})
	epoch := hello.Epoch

	first, _, _ := join(t, h, "a.txt", Peer{ID: "1", Client: "c1"})
	keeper.expect("join")
	keeper.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"k"}]}]}`)
	keeper.expect("ack")
	first.expect("op")
	first.send(`{"t":"op","n":1,"v":1,"d":[{"o":"txt","id":"body","x":[{"i":"a"}]}]}`)
	first.expect("ack")
	first.leave()
	keeper.expect("op")
	keeper.expect("leave")
	keeper.send(`{"t":"op","n":2,"v":2,"d":[{"o":"txt","id":"body","x":[{"i":"b"}]}]}`)
	keeper.expect("ack")

	// back from revision 1: its own edit is acknowledged, the other replayed
	again := connect(t, h, context.Background(), "a.txt", Peer{ID: "1", Client: "c1"})
	if f := again.expect("hello"); f.V != 3 || f.Epoch != epoch {
		t.Fatalf("hello = %+v", f)
	}
	again.send(`{"t":"sync","epoch":"` + epoch + `","v":1}`)
	if f := again.expect("ack"); f.N != 1 || f.V != 2 {
		t.Fatalf("ack = %+v", f)
	}
	if f := again.expect("op"); f.V != 3 || string(f.D) != `[{"o":"txt","id":"body","x":[{"i":"b"}]}]` {
		t.Fatalf("op = %+v", f)
	}
	if f := again.expect("ready"); f.V != 3 {
		t.Fatalf("ready = %+v", f)
	}
	again.send(`{"t":"op","n":1,"v":1,"d":[{"o":"txt","id":"body","x":[{"i":"z"}]}]}`)
	again.send(`{"t":"op","n":2,"v":3,"d":[{"o":"txt","id":"body","x":[{"r":3},{"i":"c"}]}]}`)
	if f := again.expect("ack"); f.N != 2 || f.V != 4 {
		t.Fatalf("ack = %+v", f)
	}

	// from another stay in memory, or too far back: the whole document
	for _, sync := range []string{`{"t":"sync","epoch":"other","v":1}`, `{"t":"sync","epoch":"` + epoch + `","v":9}`} {
		c := connect(t, h, context.Background(), "a.txt", Peer{ID: "1", Client: "c1"})
		c.expect("hello")
		c.send(sync)
		if f := c.expect("doc"); f.V != 4 || f.Ack != 2 || string(f.D) != `[{"o":"new","id":"body","t":"text","k":"V","x":[{"i":"bakc\n"}]}]` {
			t.Fatalf("doc = %+v", f)
		}
		c.leave()
	}
}

func TestRefusedOperations(t *testing.T) {
	store := newMemStore()
	store.data["a.txt"] = []byte("abc")
	h := NewHub(store, Options{MaxLength: 8, History: 2})

	reader, _, _ := join(t, h, "a.txt", Peer{ID: "1", ReadOnly: true})
	reader.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"x"}]}]}`)
	if f := reader.expect("nack"); f.N != 1 || f.Error != errReadOnly.Error() {
		t.Fatalf("nack = %+v", f)
	}

	writer, _, _ := join(t, h, "a.txt", Peer{ID: "2"})
	reader.expect("join")
	for _, c := range []struct{ op, err string }{
		{`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"x","r":1}]}]}`, errMalformed.Error()},
		{`{"t":"op","n":2,"v":0,"d":[{"o":"txt","id":"body","x":[{"d":-1}]}]}`, errMalformed.Error()},
		{`{"t":"op","n":3,"v":0,"d":{"i":"x"}}`, errMalformed.Error()},
		{`{"t":"op","n":4,"v":0,"d":[{"o":"txt","id":"body","x":[{"r":4},{"i":"x"}]}]}`, ot.ErrNoMark.Error()},
		{`{"t":"op","n":5,"v":0,"d":[{"o":"txt","id":"body","x":[{"r":9}]}]}`, ot.ErrLength.Error()},
		{`{"t":"op","n":6,"v":1,"d":[{"o":"txt","id":"body","x":[{"i":"x"}]}]}`, errStale.Error()},
		{`{"t":"op","n":7,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"12345"}]}]}`, errTooLong.Error()},
	} {
		writer.send(c.op)
		if f := writer.expect("nack"); f.Error != c.err {
			t.Errorf("%s: nack %q, want %q", c.op, f.Error, c.err)
		}
	}
	reader.quiet()

	for n := range 3 {
		writer.send(`{"t":"op","n":` + strconv.Itoa(8+n) + `,"v":` + strconv.Itoa(n) + `,"d":[{"o":"txt","id":"body","x":[{"d":1}]}]}`)
		writer.expect("ack")
	}
	writer.send(`{"t":"op","n":11,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"x"}]}]}`)
	if f := writer.expect("nack"); f.Error != errStale.Error() {
		t.Fatalf("an edit older than the history: %+v", f)
	}
}

func TestEditsBeforeSyncAreIgnored(t *testing.T) {
	h := NewHub(newMemStore(), Options{})
	c := connect(t, h, context.Background(), "a.txt", Peer{ID: "1"})
	c.expect("hello")
	c.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"x"}]}]}`)
	c.send(`{"t":"sync"}`)
	if f := c.expect("doc"); string(f.D) != `[{"o":"new","id":"body","t":"text","k":"V","x":[{"i":"\n"}]}]` {
		t.Fatalf("doc = %+v", f)
	}
	c.quiet()
}

func TestPresenceIsRelayedAndRateLimited(t *testing.T) {
	h := NewHub(newMemStore(), Options{PresenceRate: 3})
	a, _, _ := join(t, h, "a.txt", Peer{ID: "1"})
	b, _, _ := join(t, h, "a.txt", Peer{ID: "2"})
	a.expect("join")

	for range 10 {
		a.send(`{"t":"eph","d":{"s":[1,2]}}`)
	}
	for range 3 {
		if f := b.expect("eph"); f.SID != 1 || string(f.D) != `{"s":[1,2]}` {
			t.Fatalf("eph = %+v", f)
		}
	}
	b.quiet()
	a.quiet()
	b.send(`{"d":{"s":[3,4]},"t":"eph"}`)
	if f := a.expect("eph"); string(f.D) != `{"s":[3,4]}` {
		t.Fatalf("eph = %+v", f)
	}
}

func TestPresenceData(t *testing.T) {
	for _, c := range []struct {
		msg, d string
		ok     bool
	}{
		{`{"t":"eph","d":{"s":[1,2]}}`, `{"s":[1,2]}`, true},
		{`{"t":"eph","d":null}`, `null`, true},
		{`{"t":"eph","d":{"s":[1,2]},"x":1}`, "", false},
		{`{"t":"eph","d":{"s":[1,2}}`, "", false},
		{`{"d":{"s":[1,2]},"t":"eph"}`, "", false},
		{`{"t":"op","n":1}`, "", false},
	} {
		d, ok := presenceData([]byte(c.msg))
		if ok != c.ok || ok && string(d) != c.d {
			t.Errorf("presenceData(%s) = %s, %v", c.msg, d, ok)
		}
	}
}

type compressingConn struct {
	*fakeConn
	sizes chan int
	next  bool
}

func (c *compressingConn) EnableWriteCompression(enable bool) { c.next = enable }

func (c *compressingConn) WriteMessage(kind int, data []byte) error {
	if c.next {
		c.sizes <- len(data)
	}
	return c.fakeConn.WriteMessage(kind, data)
}

func TestOnlyLargeFramesAreCompressed(t *testing.T) {
	store := newMemStore()
	store.data["a.txt"] = []byte(strings.Repeat("x", compressFrom))
	h := NewHub(store, Options{})
	conn := &compressingConn{fakeConn: newFakeConn(), sizes: make(chan int, 8)}
	go func() { _ = h.Serve(context.Background(), conn, "a.txt", Peer{ID: "2"}) }()
	t.Cleanup(func() { conn.Close() })
	<-conn.out
	conn.in <- []byte(`{"t":"sync"}`)
	<-conn.out
	select {
	case size := <-conn.sizes:
		if size < compressFrom {
			t.Fatalf("compressed a %d-byte frame", size)
		}
	default:
		t.Fatal("large frame not compressed")
	}
	if len(conn.sizes) != 0 {
		t.Fatal("small frame compressed")
	}
}

func TestSaveFailureIsReportedAndRetried(t *testing.T) {
	store := newMemStore()
	h := NewHub(store, fastOptions())
	a, _, _ := join(t, h, "a.txt", Peer{ID: "1"})

	store.setFail(errors.New("disk full"))
	a.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"a"}]}]}`)
	a.expect("ack")
	<-store.saves
	if f := a.expect("error"); f.Error != "disk full" {
		t.Fatalf("error = %+v", f)
	}

	late := connect(t, h, context.Background(), "a.txt", Peer{ID: "2"})
	if f := late.expect("hello"); f.Error != "disk full" {
		t.Fatalf("hello = %+v", f)
	}
	a.expect("join")

	store.setFail(nil)
	<-store.saves
	for _, c := range []*client{a, late} {
		if f := c.expect("saved"); f.V != 1 {
			t.Fatalf("saved = %+v", f)
		}
	}
}

func TestLastLeaveSavesAndUnloads(t *testing.T) {
	store := newMemStore()
	h := NewHub(store, Options{SaveDelay: time.Hour, SaveMaxDelay: time.Hour})
	a, _, _ := join(t, h, "a.txt", Peer{ID: "1"})
	a.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"a"}]}]}`)
	a.expect("ack")
	a.leave()

	if got := store.file("a.txt"); got != "a" {
		t.Fatalf("saved %q", got)
	}
	h.mu.Lock()
	loaded := len(h.rooms)
	h.mu.Unlock()
	if loaded != 0 {
		t.Fatalf("%d documents still loaded", loaded)
	}
}

func TestLoadFailureClosesConnection(t *testing.T) {
	store := newMemStore()
	store.setFail(errors.New("file not found"))
	h := NewHub(store, Options{})
	c := connect(t, h, context.Background(), "a.txt", Peer{ID: "1"})
	if err := <-c.done; err == nil || err.Error() != "file not found" {
		t.Fatalf("Serve = %v", err)
	}
	if code, reason := c.conn.closeFrame(); code != CloseLoadFailed || reason != "file not found" {
		t.Fatalf("close %d %q", code, reason)
	}

	store.setFail(nil)
	again := connect(t, h, context.Background(), "a.txt", Peer{ID: "1"})
	again.expect("hello")

	odd := connect(t, h, context.Background(), "a.odt", Peer{ID: "1"})
	if err := <-odd.done; err == nil {
		t.Fatal("an .odt file opened")
	}
}

func TestCancelledContextRevokes(t *testing.T) {
	h := NewHub(newMemStore(), Options{})
	ctx, cancel := context.WithCancelCause(context.Background())
	c := connect(t, h, ctx, "a.txt", Peer{ID: "1"})
	c.expect("hello")
	cancel(errors.New("share removed"))
	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return")
	}
	if code, reason := c.conn.closeFrame(); code != CloseRevoked || reason != "share removed" {
		t.Fatalf("close %d %q", code, reason)
	}
}

func TestCloseSavesEverything(t *testing.T) {
	store := newMemStore()
	h := NewHub(store, Options{SaveDelay: time.Hour, SaveMaxDelay: time.Hour})
	a, _, _ := join(t, h, "a.txt", Peer{ID: "1"})
	a.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"a"}]}]}`)
	a.expect("ack")

	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := store.file("a.txt"); got != "a" {
		t.Fatalf("saved %q", got)
	}
	if code, _ := a.conn.closeFrame(); code != CloseShutdown {
		t.Fatalf("close code %d", code)
	}
	late := connect(t, h, context.Background(), "a.txt", Peer{ID: "2"})
	if err := <-late.done; !errors.Is(err, ErrClosed) {
		t.Fatalf("Serve after Close = %v", err)
	}
}

func TestClosePayloadTruncatesOnRuneBoundary(t *testing.T) {
	p := closePayload(CloseRevoked, strings.Repeat("é", 100))
	if len(p) > 125 || !strings.HasPrefix(string(p[2:]), "é") || !utf8.ValidString(string(p[2:])) {
		t.Fatalf("payload %q", p)
	}
}

func TestGoneDocumentDisconnectsAndUnloads(t *testing.T) {
	store := newMemStore()
	h := NewHub(store, fastOptions())
	a, _, _ := join(t, h, "a.txt", Peer{ID: "1"})

	store.setFail(fmt.Errorf("file deleted: %w", ErrGone))
	a.send(`{"t":"op","n":1,"v":0,"d":[{"o":"txt","id":"body","x":[{"i":"a"}]}]}`)
	a.expect("ack")
	select {
	case <-a.done:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return")
	}
	if code, reason := a.conn.closeFrame(); code != CloseRevoked || !strings.HasPrefix(reason, "file deleted") {
		t.Fatalf("close %d %q", code, reason)
	}
	h.mu.Lock()
	loaded := len(h.rooms)
	h.mu.Unlock()
	if loaded != 0 {
		t.Fatalf("%d documents still loaded", loaded)
	}
}
