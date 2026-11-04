package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"
)

// protocols are the versions of the protocol the server speaks, the latest
// first.
var protocols = []string{"2025-06-18", "2025-03-26"}

const (
	maxBody = 4 << 20

	codeParse          = -32700
	codeInvalidRequest = -32600
	codeNoMethod       = -32601
	codeInvalidParams  = -32602
	codeInternal       = -32603
	codeRefused        = -32001
)

const instructions = "Read and edit Word documents with other people in them. Every call names the user you work for in on_behalf_of. " +
	"Your edits are tracked changes in your name that people accept or reject: read the document with doc_read, " +
	"edit with the block ids it gives, and preview a batch with dry_run before applying it. " +
	"If a call fails, its message says what to do; nothing is written then."

const internalError = "internal error: nothing was written that you can rely on; read the document again and retry"

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func fail(id json.RawMessage, code int, format string, args ...any) *response {
	if id == nil {
		id = json.RawMessage("null")
	}
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{code, fmt.Sprintf(format, args...)}}
}

// result is what a tool call answers: text, and whether it went wrong.
type result struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func text(s string, isError bool) *result {
	return &result{Content: []content{{"text", s}}, IsError: isError}
}

// isRequest tells whether the message wants an answer.
func (m *message) isRequest() bool { return m.Method != "" && len(m.ID) > 0 }

// validID tells whether an id is a string or a number, as the protocol has
// it.
func validID(id json.RawMessage) bool {
	id = bytes.TrimSpace(id)
	return len(id) > 0 && (id[0] == '"' || id[0] == '-' || id[0] >= '0' && id[0] <= '9')
}

// handle answers one message. The response is nil for a message that gets
// none, and status is the HTTP status that goes with it. r is the HTTP
// request, nil on a stream.
func (s *Server) handle(ctx context.Context, r *http.Request, body []byte) (resp *response, status int) {
	var m message
	if err := json.Unmarshal(body, &m); err != nil {
		return fail(nil, codeParse, "the message is not JSON"), http.StatusBadRequest
	}
	if m.JSONRPC != "2.0" || (m.Method == "" && len(m.Result) == 0 && len(m.Error) == 0) || len(m.ID) > 0 && !validID(m.ID) {
		return fail(nil, codeInvalidRequest, "the message is not a JSON-RPC 2.0 request, notification or response; batches are not supported"), http.StatusBadRequest
	}
	access := Access{Request: r, Method: m.Method}
	var run func(ctx context.Context) *response
	if m.Method == "tools/call" {
		access, run = s.prepare(&m, access)
	}
	if err := s.authorize(ctx, access); err != nil {
		status := http.StatusForbidden
		if errors.Is(err, ErrUnauthenticated) {
			status = http.StatusUnauthorized
		}
		if m.isRequest() && run != nil && status == http.StatusForbidden {
			return &response{JSONRPC: "2.0", ID: m.ID, Result: text(err.Error(), true)}, http.StatusOK
		}
		return fail(m.ID, codeRefused, "%s", err), status
	}
	if !m.isRequest() {
		return nil, http.StatusAccepted
	}
	if run != nil {
		return run(ctx), http.StatusOK
	}
	switch m.Method {
	case "initialize":
		return s.initialize(&m), http.StatusOK
	case "ping":
		return &response{JSONRPC: "2.0", ID: m.ID, Result: struct{}{}}, http.StatusOK
	case "tools/list":
		return s.list(&m), http.StatusOK
	}
	return fail(m.ID, codeNoMethod, "unknown method %s", m.Method), http.StatusOK
}

func (s *Server) initialize(m *message) *response {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(m.Params, &p)
	version := protocols[0]
	if slices.Contains(protocols, p.ProtocolVersion) {
		version = p.ProtocolVersion
	}
	return &response{JSONRPC: "2.0", ID: m.ID, Result: map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "loffice", "title": "loffice", "version": "1"},
		"instructions":    instructions,
	}}
}

func (s *Server) list(m *message) *response {
	tools := make([]map[string]any, len(s.tools))
	for i, t := range s.tools {
		tools[i] = map[string]any{"name": t.name, "description": t.description, "inputSchema": t.schema()}
	}
	return &response{JSONRPC: "2.0", ID: m.ID, Result: map[string]any{"tools": tools}}
}

// prepare reads a tools/call far enough to tell the host what it asks, and
// gives what runs it once the host agrees.
func (s *Server) prepare(m *message, access Access) (Access, func(ctx context.Context) *response) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal(m.Params, &p)
	access.Tool = p.Name
	var t *tool
	for _, c := range s.tools {
		if c.name == p.Name {
			t = c
		}
	}
	if t == nil {
		return access, func(context.Context) *response {
			return fail(m.ID, codeInvalidParams, "unknown tool %q: tools/list gives the tools", p.Name)
		}
	}
	args, decodeErr := t.decode(p.Arguments)
	access.User, access.Doc = args.OnBehalfOf, args.Doc
	access.Write = t.write != nil && t.write(args)
	return access, func(ctx context.Context) *response {
		res := s.invoke(ctx, t, args, decodeErr)
		return &response{JSONRPC: "2.0", ID: m.ID, Result: res}
	}
}

// invoke runs a tool the host allowed. A panic is an error of the tool,
// logged, not the end of the server.
func (s *Server) invoke(ctx context.Context, t *tool, args *params, decodeErr error) (res *result) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("mcp: tool panicked", "tool", t.name, "panic", r, "stack", string(debug.Stack()))
			res = text(internalError, true)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()
	if decodeErr != nil {
		return text(decodeErr.Error(), true)
	}
	c, err := t.check(args)
	if err != nil {
		return text(err.Error(), true)
	}
	out, err := t.run(ctx, s, c)
	if err != nil {
		return text(explain(err), true)
	}
	return text(out, false)
}

// ServeHTTP is the streamable HTTP transport: each message is a POST, and
// the answer to a request the body of the response. The server opens no
// stream of its own, keeps no session, and answers a long call on a stream
// that the client accepts, with a ping now and then to keep it up.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" && !slices.Contains(s.cfg.AllowedOrigins, origin) {
		http.Error(w, "origin not allowed", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST the JSON-RPC messages", http.StatusMethodNotAllowed)
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !slices.Contains(protocols, v) {
		http.Error(w, "unsupported protocol version", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "message too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	done := make(chan outcome, 1)
	go func() {
		resp, status := s.handle(r.Context(), r, body)
		done <- outcome{resp, status}
	}()
	timer := time.NewTimer(s.keepalive)
	defer timer.Stop()
	select {
	case out := <-done:
		out.write(w)
		return
	case <-timer.C:
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		out := <-done
		out.write(w)
		return
	}
	s.stream(w, r, body, done)
}

// outcome is the answer to a message, and the HTTP status that goes with
// it.
type outcome struct {
	resp   *response
	status int
}

func (o outcome) write(w http.ResponseWriter) {
	if o.resp == nil {
		w.WriteHeader(o.status)
		return
	}
	data, err := json.Marshal(o.resp)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(o.status)
	_, _ = w.Write(data)
}

// stream answers on a stream of server-sent events: a comment, and a
// progress notification if the client asked for them, every keepalive until
// the answer.
func (s *Server) stream(w http.ResponseWriter, r *http.Request, body []byte, done <-chan outcome) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		out := <-done
		out.write(w)
		return
	}
	var req struct {
		Params struct {
			Meta struct {
				ProgressToken json.RawMessage `json:"progressToken"`
			} `json:"_meta"`
		} `json:"params"`
	}
	_ = json.Unmarshal(body, &req)
	token := req.Params.Meta.ProgressToken
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	tick := time.NewTicker(s.keepalive)
	defer tick.Stop()
	for n := 1; ; n++ {
		select {
		case out := <-done:
			if out.resp != nil {
				if data, err := json.Marshal(out.resp); err == nil {
					fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
				}
			}
			flusher.Flush()
			return
		case <-tick.C:
			if validID(token) {
				fmt.Fprintf(w, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{\"progressToken\":%s,\"progress\":%d,\"message\":\"still working\"}}\n\n", token, n)
			} else {
				fmt.Fprint(w, ": still working\n\n")
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// Serve speaks the protocol over a stream, a message to a line, as a local
// client starts a server: it returns once in ends, after the calls it is
// answering.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 64<<10), maxBody)
	var mu sync.Mutex
	var wg sync.WaitGroup
	defer wg.Wait()
	for scan.Scan() {
		line := bytes.TrimSpace(bytes.Clone(scan.Bytes()))
		if len(line) == 0 {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, _ := s.handle(ctx, nil, line)
			if resp == nil {
				return
			}
			data, err := json.Marshal(resp)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			_, _ = out.Write(append(data, '\n'))
		}()
	}
	return scan.Err()
}
