package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/citadellefr/loffice"
	"github.com/citadellefr/loffice/agent"
	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/trametest"
)

const (
	docKey  = "rapport.docx"
	deckKey = "deck.pptx"
	bookKey = "book.xlsx"
)

// host is what a host of the server brings: a hub with the sample
// document, and an Authorize that remembers what it was asked.
type host struct {
	t     testing.TB
	hub   *loffice.Hub
	store *trametest.Store
	srv   *Server

	mu      sync.Mutex
	asked   []Access
	refuse  func(Access) error
	callers []string
}

func newHost(t testing.TB) *host {
	t.Helper()
	h := makeHost(t)
	h.t = t
	t.Cleanup(h.srv.Close)
	return h
}

func makeHost(t interface{ Fatal(...any) }) *host {
	store := trametest.NewStore()
	for key, file := range map[string]string{docKey: "../agent/testdata/sample.docx", deckKey: "../agent/testdata/deck.pptx", bookKey: "../testdata/xlsx/transitional.xlsx"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		store.Data[key] = data
	}
	var err error
	h := &host{store: store, hub: loffice.NewHub(store, loffice.Options{}, loffice.Limits{})}
	h.srv, err = New(Config{
		Hub:  h.hub,
		Name: "Héraut",
		Authorize: func(_ context.Context, a Access) error {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.asked = append(h.asked, a)
			if a.Request != nil {
				h.callers = append(h.callers, a.Request.Header.Get("X-Service"))
			}
			if h.refuse != nil {
				return h.refuse(a)
			}
			return nil
		},
		List: func(_ context.Context, a Access) ([]Document, error) {
			return []Document{{docKey, "Rapport"}, {deckKey, "Présentation"}, {bookKey, "Classeur"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// close takes the AI out and flushes the hub.
func (h *host) close() {
	h.srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = h.hub.Close(ctx)
}

func (h *host) accesses() []Access {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]Access(nil), h.asked...)
}

// client is a client of the protocol.
type client interface {
	rpc(method string, params any) rpcResponse
	notify(method string)
}

type rpcResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	status int
}

type toolResult struct {
	Content []struct{ Type, Text string }
	IsError bool
}

func (r rpcResponse) tool(t *testing.T) toolResult {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("protocol error %+v", r.Error)
	}
	var res toolResult
	if err := json.Unmarshal(r.Result, &res); err != nil || len(res.Content) != 1 || res.Content[0].Type != "text" {
		t.Fatalf("not a tool result: %s %v", r.Result, err)
	}
	return res
}

// callTool calls a tool, with the arguments a model gives.
func callTool(t *testing.T, c client, name string, args map[string]any) toolResult {
	t.Helper()
	return c.rpc("tools/call", map[string]any{"name": name, "arguments": args}).tool(t)
}

// ok calls a tool that must work and gives its text.
func ok(t *testing.T, c client, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, c, name, args)
	if res.IsError {
		t.Fatalf("%s: %s", name, res.Content[0].Text)
	}
	return res.Content[0].Text
}

// fails calls a tool that must fail, and gives what it tells.
func fails(t *testing.T, c client, name string, args map[string]any) string {
	t.Helper()
	res := callTool(t, c, name, args)
	if !res.IsError {
		t.Fatalf("%s should fail: %s", name, res.Content[0].Text)
	}
	return res.Content[0].Text
}

func as(user, doc string, args map[string]any) map[string]any {
	out := map[string]any{"on_behalf_of": user, "doc": doc}
	for k, v := range args {
		out[k] = v
	}
	return out
}

// httpClient talks to the handler over HTTP.
type httpClient struct {
	t       *testing.T
	url     string
	headers http.Header
	id      int
}

func newHTTPClient(t *testing.T, s *Server) *httpClient {
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return &httpClient{t: t, url: ts.URL, headers: http.Header{"X-Service": {"heraut"}}}
}

func (c *httpClient) post(body string) (*http.Response, []byte) {
	c.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, c.url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		req.Header[k] = v
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp, data
}

func (c *httpClient) rpc(method string, params any) rpcResponse {
	c.t.Helper()
	c.id++
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	resp, data := c.post(string(body))
	if strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		data = lastEvent(c.t, data)
	}
	out := rpcResponse{status: resp.StatusCode}
	if err := json.Unmarshal(data, &out); err != nil {
		c.t.Fatalf("%d %s", resp.StatusCode, data)
	}
	return out
}

func (c *httpClient) notify(method string) {
	c.t.Helper()
	resp, _ := c.post(`{"jsonrpc":"2.0","method":"` + method + `"}`)
	if resp.StatusCode != http.StatusAccepted {
		c.t.Fatalf("notification: %d", resp.StatusCode)
	}
}

// lastEvent is the last message of a stream of events.
func lastEvent(t *testing.T, stream []byte) []byte {
	t.Helper()
	var last []byte
	for line := range bytes.Lines(stream) {
		if data, ok := bytes.CutPrefix(line, []byte("data: ")); ok {
			last = bytes.TrimSpace(data)
		}
	}
	if last == nil {
		t.Fatalf("no message in %q", stream)
	}
	return last
}

// stdioClient talks to the server over pipes.
type stdioClient struct {
	t    *testing.T
	in   io.WriteCloser
	out  *bufio.Reader
	id   int
	done chan error
}

func newStdioClient(t *testing.T, s *Server) *stdioClient {
	toServer, in := io.Pipe()
	out, fromServer := io.Pipe()
	c := &stdioClient{t: t, in: in, out: bufio.NewReader(out), done: make(chan error, 1)}
	go func() {
		err := s.Serve(context.Background(), toServer, fromServer)
		fromServer.Close()
		c.done <- err
	}()
	t.Cleanup(func() {
		in.Close()
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			t.Error("Serve did not return at the end of its input")
		}
	})
	return c
}

func (c *stdioClient) rpc(method string, params any) rpcResponse {
	c.t.Helper()
	c.id++
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params})
	if _, err := c.in.Write(append(body, '\n')); err != nil {
		c.t.Fatal(err)
	}
	line, err := c.out.ReadBytes('\n')
	if err != nil {
		c.t.Fatal(err)
	}
	var out struct {
		rpcResponse
		ID int `json:"id"`
	}
	if err := json.Unmarshal(line, &out); err != nil || out.ID != c.id {
		c.t.Fatalf("%s %v", line, err)
	}
	return out.rpcResponse
}

func (c *stdioClient) notify(method string) {
	c.t.Helper()
	if _, err := fmt.Fprintf(c.in, `{"jsonrpc":"2.0","method":%q}`+"\n", method); err != nil {
		c.t.Fatal(err)
	}
}

// someone is another participant of the room, who edits the document.
func (h *host) someone(doc, name string) *agent.Agent {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, err := agent.Join(ctx, h.hub, doc, trame.Peer{ID: name, Name: name}, agent.Options{})
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(a.Leave)
	return a
}
