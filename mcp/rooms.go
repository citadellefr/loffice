package mcp

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/citadellefr/loffice/agent"
	"github.com/citadellefr/trame"
)

// maxJournal is how many batches of a document the server remembers for
// the AI's next visit.
const maxJournal = 1000

// errClosing is what a call gets once the server is closed.
var errClosing = errors.New("mcp: the server is closing")

// room is the AI in the room of a document: one agent for every call about
// that document, since the ids of blocks and the versions it gives hold for
// that agent only.
type room struct {
	ready chan struct{}
	agent *agent.Agent
	err   error
	users int
	idle  *time.Timer
	// leaving is set once the agent is on its way out; gone is closed when
	// it has gone and its journal is kept.
	leaving bool
	gone    chan struct{}
	once    sync.Once
}

// acquire gives the AI in the room of the document, joined if it was not,
// and the release to call once done with it.
func (s *Server) acquire(ctx context.Context, key string) (*agent.Agent, func(), error) {
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, nil, errClosing
		}
		r := s.rooms[key]
		if r != nil && r.leaving {
			s.mu.Unlock()
			select {
			case <-r.gone:
				continue
			case <-ctx.Done():
				return nil, nil, context.Cause(ctx)
			}
		}
		if r == nil {
			r = &room{ready: make(chan struct{}), gone: make(chan struct{}), users: 1}
			s.rooms[key] = r
			journal := slices.Clone(s.journals[key])
			s.mu.Unlock()
			peer := trame.Peer{ID: s.cfg.ID, Name: s.cfg.Name}
			a, err := agent.Join(ctx, s.cfg.Hub, key, peer, agent.Options{Journal: journal})
			s.mu.Lock()
			r.agent, r.err = a, err
			if err != nil && s.rooms[key] == r {
				delete(s.rooms, key)
			}
			close(r.ready)
			s.mu.Unlock()
			if err != nil {
				return nil, nil, err
			}
			return a, s.releaser(key, r), nil
		}
		r.users++
		if r.idle != nil {
			r.idle.Stop()
			r.idle = nil
		}
		s.mu.Unlock()
		release := s.releaser(key, r)
		select {
		case <-r.ready:
		case <-ctx.Done():
			release()
			return nil, nil, context.Cause(ctx)
		}
		switch {
		case r.err != nil && (errors.Is(r.err, context.Canceled) || errors.Is(r.err, context.DeadlineExceeded)) && ctx.Err() == nil:
			release()
		case r.err != nil:
			release()
			return nil, nil, r.err
		case isDone(r.agent):
			s.drop(key, r)
			release()
		default:
			return r.agent, release, nil
		}
	}
}

func isDone(a *agent.Agent) bool {
	select {
	case <-a.Done():
		return true
	default:
		return false
	}
}

// releaser gives the release of a room: the AI leaves it once idle.
func (s *Server) releaser(key string, r *room) func() {
	var done bool
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if done {
			return
		}
		done = true
		if r.users--; r.users == 0 && !s.closed {
			r.idle = time.AfterFunc(s.cfg.Idle, func() { s.expire(key, r) })
		}
	}
}

// expire takes the AI out of a room nobody has used for a while.
func (s *Server) expire(key string, r *room) {
	s.mu.Lock()
	if r.users > 0 || s.rooms[key] != r {
		s.mu.Unlock()
		return
	}
	r.leaving = true
	s.mu.Unlock()
	r.leave(s, key)
}

// drop forgets a room whose agent the hub disconnected.
func (s *Server) drop(key string, r *room) {
	s.mu.Lock()
	if s.rooms[key] != r {
		s.mu.Unlock()
		return
	}
	r.leaving = true
	s.mu.Unlock()
	r.leave(s, key)
}

// leave takes the agent out and keeps its journal, once.
func (r *room) leave(s *Server, key string) {
	r.once.Do(func() {
		<-r.ready
		var journal []agent.Record
		if r.agent != nil {
			r.agent.Leave()
			journal = r.agent.Journal()
			if len(journal) > maxJournal {
				journal = journal[len(journal)-maxJournal:]
			}
		}
		s.mu.Lock()
		if r.agent != nil {
			s.journals[key] = journal
		}
		if s.rooms[key] == r {
			delete(s.rooms, key)
		}
		s.mu.Unlock()
		close(r.gone)
	})
	<-r.gone
}
