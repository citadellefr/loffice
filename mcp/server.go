// Package mcp serves the agent package to an AI model as an MCP server
// (Model Context Protocol, 2025-06-18): the tools to read a document of a
// hub, search it, edit it as revisions, comment it and take back what the AI
// did.
//
// A host mounts a Server as an http.Handler beside its hub, which the server
// shares, and calls it over the network; the same server speaks the protocol
// on standard input and output for a local client. The server owns no
// authority: the host's Authorize is asked on every request who calls, which
// user the AI acts for and which document it touches, and what it refuses
// does not happen. Nothing is global: a host that does not mount the server
// exposes nothing.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/citadellefr/loffice/agent"
)

// ErrUnauthenticated is what Authorize returns for a caller that is not an
// authorized service: the request is answered 401 whatever it asked.
var ErrUnauthenticated = errors.New("mcp: the caller is not an authorized service")

// Access is a request the host decides on.
type Access struct {
	// Request is the HTTP request, whose credentials identify the calling
	// service; nil on standard input.
	Request *http.Request
	// Method is the JSON-RPC method: "initialize", "tools/list",
	// "tools/call"…
	Method string
	// Tool is the tool called.
	Tool string
	// User is the id the AI acts on behalf of, empty for the requests that
	// do not name one, and for a call that forgot to: the caller alone is
	// then to be checked, and the call is refused after.
	User string
	// Doc is the key of the document, under which the hub serves it. Empty
	// for a request that touches none.
	Doc string
	// Write is true when the call changes the document.
	Write bool
}

// Document is a document a user may open.
type Document struct {
	Doc   string `json:"doc"`
	Title string `json:"title,omitempty"`
}

// Config is what the host gives a Server.
type Config struct {
	// Hub is the hub the documents are served by: the host's own.
	Hub agent.Server
	// Name is the AI's name: the author of its revisions and comments in
	// Word, which people see. It stays the same.
	Name string
	// ID is the AI's id among the participants of a room; Name when empty.
	ID string
	// Authorize is called on every request, before anything is read or
	// written, and refuses with an error. The AI has no right of its own:
	// the caller must be a service the host authorized, and the user named
	// by User must be allowed to read the document (Write false) or edit it
	// (Write true), and to have the AI do so for them: their consent, for
	// instance. An error that is not ErrUnauthenticated refuses a call with
	// its message, which the model reads, and any other request with a 403.
	Authorize func(ctx context.Context, a Access) error
	// List gives the documents User may open, for doc_list; the tool does
	// not exist without it.
	List func(ctx context.Context, a Access) ([]Document, error)
	// AllowedOrigins are the origins of browsers allowed to call the
	// handler; a request with another Origin is refused. A service sends
	// none.
	AllowedOrigins []string
	// Timeout bounds a call, and so what a slow hub or model may take;
	// two minutes when zero.
	Timeout time.Duration
	// Idle is how long the AI stays in the room of a document nobody asks
	// it anything about; ten minutes when zero. The versions it gave are
	// void after.
	Idle time.Duration
}

// Server is an MCP server over a hub. It is an http.Handler for the
// streamable HTTP transport, and Serve speaks the protocol over a stream.
type Server struct {
	cfg   Config
	tools []*tool

	// keepalive spaces what a long call sends to keep its connection up.
	keepalive time.Duration

	mu     sync.Mutex
	closed bool
	rooms  map[string]*room
	// journals are what the AI did in the rooms it left, which its next
	// visit needs to know who it worked for.
	journals map[string][]agent.Record
}

// New makes a server. Close it to take the AI out of the rooms.
func New(cfg Config) (*Server, error) {
	switch {
	case cfg.Hub == nil:
		return nil, errors.New("mcp: no hub")
	case strings.TrimSpace(cfg.Name) == "":
		return nil, errors.New("mcp: the AI needs a name")
	case cfg.Authorize == nil:
		return nil, errors.New("mcp: no Authorize: the host decides who may do what")
	}
	if cfg.ID == "" {
		cfg.ID = cfg.Name
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 2 * time.Minute
	}
	if cfg.Idle <= 0 {
		cfg.Idle = 10 * time.Minute
	}
	s := &Server{cfg: cfg, keepalive: 15 * time.Second, rooms: map[string]*room{}, journals: map[string][]agent.Record{}}
	for _, t := range newTools() {
		if t.name != "doc_list" || cfg.List != nil {
			s.tools = append(s.tools, t)
		}
	}
	return s, nil
}

// Close takes the AI out of every room.
func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	rooms := s.rooms
	s.rooms = map[string]*room{}
	for _, r := range rooms {
		if r.idle != nil {
			r.idle.Stop()
		}
	}
	s.mu.Unlock()
	for key, r := range rooms {
		r.leave(s, key)
	}
}

// authorize asks the host.
func (s *Server) authorize(ctx context.Context, a Access) error {
	if err := s.cfg.Authorize(ctx, a); err != nil {
		if errors.Is(err, ErrUnauthenticated) {
			return err
		}
		return &refusal{err}
	}
	return nil
}

// refusal is what the host refused.
type refusal struct{ error }

func (r *refusal) Unwrap() error { return r.error }

func (r *refusal) Error() string {
	return fmt.Sprintf("not allowed: %s", r.error)
}
