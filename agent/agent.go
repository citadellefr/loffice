// Package agent lets a program take part in a room of the hub as one more
// participant: an AI that reads a document and edits it while people have
// it open, as WordCraft, Revise or Adeu do. Its edits go through the hub
// like anyone's: checked by the same File.Check, rebased over what others
// do meanwhile, shown live to everyone with its name and cursor.
//
// The agent reads a document as Markdown, or as CriticMarkup to see the
// revisions and comments, each block given an id it keeps while it exists.
// It edits by batches, applied whole or not at all, which it can preview
// first. In Word every change of text is a revision signed by the agent's
// name, which people accept or reject as their own; each batch records the
// user the agent acts for, and UndoMine takes back what it did for one of
// them. A batch prepared on a version whose blocks changed since, as far as
// the agent knows when it sends it, fails without writing anything; what
// others send meanwhile is rebased as for anyone.
//
// Word documents are edited so far; PowerPoint and Excel will take the
// same batches, edited directly rather than as revisions.
package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/citadellefr/trame"
	"github.com/citadellefr/trame/ot"
)

var (
	ErrClosed = errors.New("agent: not in the room")
	// ErrUnsupported is what the agent cannot do yet with this kind of
	// document.
	ErrUnsupported = errors.New("agent: not available for this kind of document")
	// ErrGone is a block, revision or comment that no longer exists: it
	// was deleted, or rewritten as another.
	ErrGone = errors.New("agent: no such block")
	// ErrConflict is a block changed since the version a batch was
	// prepared on.
	ErrConflict = errors.New("agent: changed since the version given")
	// ErrStale is a version another agent gave, or a malformed one.
	ErrStale = errors.New("agent: unknown version, read the document again")
	// ErrNotFound is text a block does not hold.
	ErrNotFound = errors.New("agent: text not found")
	ErrInvalid  = errors.New("agent: invalid edit")
	// ErrDenied is a write the host's Allow refused.
	ErrDenied = errors.New("agent: write denied")
	// ErrRefused is an edit the document refused.
	ErrRefused = errors.New("agent: edit refused")
)

// Server is the hub the agent joins: a loffice.Hub or a trame.Hub.
type Server interface {
	Serve(ctx context.Context, conn trame.Conn, key string, info trame.Peer) error
}

type Options struct {
	// Allow is asked before every write, with the batch: an error refuses
	// it, before anything is sent. The user the agent acts for has not
	// consented, may not edit this document…
	Allow func(ctx context.Context, b Batch) error
	// Now is the clock that dates revisions; time.Now when nil.
	Now func() time.Time
	// Journal is what a previous session of the agent applied to this
	// document, for its dates to go on from the last one.
	Journal []Record
}

// Agent is a participant of a room, until Leave.
type Agent struct {
	peer    trame.Peer
	kind    string
	opt     Options
	conn    *pipe
	session string
	ready   chan struct{}
	done    chan struct{}
	cancel  context.CancelFunc

	// sending lets one batch at a time travel.
	sending sync.Mutex

	mu      sync.Mutex
	s       *state
	n       uint64
	flight  *flight
	err     error
	journal []Record
}

// flight is an edit sent, waiting for the hub to take it.
type flight struct {
	n    uint64
	base uint64
	edit ot.Edit
	// since are the edits of others the hub applied before it.
	since []ot.Edit
	done  chan error
	// applied is the edit as the hub applied it.
	applied ot.Edit
	version uint64
}

// Join connects to the document key of the hub as peer, whose Name signs
// the agent's revisions and comments, and returns once it has the
// document.
func Join(ctx context.Context, hub Server, key string, peer trame.Peer, opt Options) (*Agent, error) {
	if strings.TrimSpace(peer.Name) == "" {
		return nil, fmt.Errorf("%w: the agent needs a name", ErrInvalid)
	}
	if opt.Now == nil {
		opt.Now = time.Now
	}
	a := &Agent{
		peer:    peer,
		kind:    strings.ToLower(path.Ext(key)),
		opt:     opt,
		session: rand.Text()[:10],
		ready:   make(chan struct{}),
		done:    make(chan struct{}),
		journal: append([]Record(nil), opt.Journal...),
	}
	a.conn = newPipe(a.receive)
	serve, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go func() {
		a.end(hub.Serve(serve, a.conn, key, peer))
	}()
	_ = a.conn.send([]byte(`{"t":"sync"}`))
	select {
	case <-a.ready:
		return a, nil
	case <-a.done:
		return nil, a.Err()
	case <-ctx.Done():
		a.Leave()
		return nil, context.Cause(ctx)
	}
}

// Leave disconnects the agent.
func (a *Agent) Leave() {
	a.cancel()
	a.conn.Close()
	<-a.done
}

// Done is closed once the agent is out of the room: it left, or the hub
// disconnected it, as Err tells.
func (a *Agent) Done() <-chan struct{} {
	return a.done
}

func (a *Agent) Err() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.err
}

// Version is the version of the document the agent has: given to Apply, it
// makes the batch fail if a block it touches changed since. It holds for
// this agent only, whose block ids it goes with.
func (a *Agent) Version() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.version()
}

func (a *Agent) version() string {
	return a.versionOf(a.s.version)
}

func (a *Agent) versionOf(v uint64) string {
	return a.session + "-" + strconv.FormatUint(v, 10)
}

// since reads a version back, the current one when empty.
func (a *Agent) since(version string) (uint64, error) {
	if version == "" {
		return a.s.version, nil
	}
	session, v, ok := strings.Cut(version, "-")
	n, err := strconv.ParseUint(v, 10, 64)
	if !ok || err != nil || session != a.session || n > a.s.version {
		return 0, fmt.Errorf("%w: %q", ErrStale, version)
	}
	return n, nil
}

func (a *Agent) end(err error) {
	code, reason := a.conn.closeFrame()
	a.conn.Close()
	a.mu.Lock()
	defer a.mu.Unlock()
	switch {
	case a.err != nil:
	case err != nil:
		a.err = fmt.Errorf("%w: %w", ErrClosed, err)
	case code != 0:
		a.err = fmt.Errorf("%w: %s (%d)", ErrClosed, reason, code)
	default:
		a.err = ErrClosed
	}
	if a.flight != nil {
		a.flight.done <- a.err
		a.flight = nil
	}
	close(a.done)
}

// fail leaves the room on an error of the agent's own.
func (a *Agent) fail(err error) {
	if a.err == nil {
		a.err = fmt.Errorf("%w: %w", ErrClosed, err)
	}
	a.conn.Close()
}

type frame struct {
	T     string          `json:"t"`
	N     uint64          `json:"n"`
	V     uint64          `json:"v"`
	Ack   uint64          `json:"ack"`
	Error string          `json:"error"`
	D     json.RawMessage `json:"d"`
}

// receive takes a frame of the hub.
func (a *Agent) receive(data []byte) {
	var f frame
	if json.Unmarshal(data, &f) != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return
	}
	switch f.T {
	case "doc":
		var nodes ot.Edit
		err := json.Unmarshal(f.D, &nodes)
		var tree *ot.Tree
		if err == nil {
			tree, err = ot.NewTree(nodes)
		}
		if err != nil {
			a.fail(err)
			return
		}
		a.s, a.n = newState(tree, f.V), f.Ack
		close(a.ready)
	case "op":
		var e ot.Edit
		if err := json.Unmarshal(f.D, &e); err != nil {
			a.fail(err)
			return
		}
		a.applied(e, f.V)
		if a.flight != nil {
			a.flight.since = append(a.flight.since, e)
		}
	case "ack":
		f2 := a.flight
		if f2 == nil || f2.n != f.N {
			return
		}
		e := f2.edit
		for _, h := range f2.since {
			e = ot.TransformEdit(h, e, true)
		}
		a.applied(e, f.V)
		f2.applied, f2.version = e, f.V
		a.flight = nil
		f2.done <- nil
	case "nack":
		if f2 := a.flight; f2 != nil && f2.n == f.N {
			a.flight = nil
			f2.done <- fmt.Errorf("%w: %s", ErrRefused, f.Error)
		}
	}
}

// applied follows an edit the hub made revision v.
func (a *Agent) applied(e ot.Edit, v uint64) {
	if a.s == nil || v != a.s.version+1 {
		a.fail(fmt.Errorf("revision %d out of order", v))
		return
	}
	if err := a.s.apply(e, v); err != nil {
		a.fail(err)
	}
}

// submit sends an edit made on the agent's document and waits for the hub
// to apply it, or refuse it. Callers hold a.sending and a.mu, which submit
// releases. If ctx ends first, the flight comes back with the error: the hub
// may still apply the edit, and the callers keep a.sending until it has.
func (a *Agent) submit(ctx context.Context, e ot.Edit) (*flight, error) {
	a.n++
	f := &flight{n: a.n, base: a.s.version, edit: e, done: make(chan error, 1)}
	a.flight = f
	a.mu.Unlock()
	d, err := json.Marshal(e)
	if err == nil {
		frame := `{"t":"op","n":` + strconv.FormatUint(f.n, 10) + `,"v":` + strconv.FormatUint(f.base, 10) + `,"d":` + string(d) + `}`
		err = a.conn.send([]byte(frame))
	}
	if err != nil {
		a.mu.Lock()
		if a.flight == f {
			a.flight = nil
		}
		a.mu.Unlock()
		return nil, err
	}
	select {
	case err := <-f.done:
		if err != nil {
			return nil, err
		}
		return f, nil
	case <-ctx.Done():
		return f, context.Cause(ctx)
	}
}

// Cursor is where the agent shows it is: a selection of a flow, from base
// to extent, in UTF-16 code units.
type Cursor struct {
	Node   string `json:"n"`
	Base   int    `json:"b"`
	Extent int    `json:"e"`
}

// show publishes the agent's cursor to the room.
func (a *Agent) show(c Cursor) error {
	d, err := json.Marshal(map[string]Cursor{"s": c})
	if err != nil {
		return err
	}
	return a.conn.send(append(append([]byte(`{"t":"eph","d":`), d...), '}'))
}

// cursorAfter is the end of the last text an edit changes.
func cursorAfter(e ot.Edit) (Cursor, bool) {
	for i := len(e) - 1; i >= 0; i-- {
		c := e[i]
		switch c.Op {
		case ot.OpTxt:
			at, end := 0, -1
			for _, o := range c.Text {
				switch {
				case o.Delete > 0:
					end = at
				case o.Insert != "":
					at += o.Len()
					end = at
				default:
					at += o.Retain
					if o.Attrs != nil {
						end = at
					}
				}
			}
			if end >= 0 {
				return Cursor{c.ID, end, end}, true
			}
		case ot.OpNew:
			if c.Text != nil {
				return Cursor{c.ID, 0, 0}, true
			}
		}
	}
	return Cursor{}, false
}
