package agent

import (
	"sync"
	"time"
)

const closeMessage = 8

// pipe is the agent's connection to the hub, in memory. What the hub writes
// is handed to deliver at once, on the hub's goroutine: the agent keeps up
// with the room however long its model thinks, and nothing on it expires.
type pipe struct {
	in      chan []byte
	deliver func([]byte)

	closed chan struct{}
	once   sync.Once

	mu     sync.Mutex
	code   int
	reason string
}

func newPipe(deliver func([]byte)) *pipe {
	return &pipe{in: make(chan []byte, 16), deliver: deliver, closed: make(chan struct{})}
}

func (p *pipe) ReadMessage() (int, []byte, error) {
	select {
	case m := <-p.in:
		return 1, m, nil
	case <-p.closed:
		return 0, nil, ErrClosed
	}
}

func (p *pipe) WriteMessage(_ int, data []byte) error {
	select {
	case <-p.closed:
		return ErrClosed
	default:
	}
	p.deliver(data)
	return nil
}

func (p *pipe) WriteControl(kind int, data []byte, _ time.Time) error {
	if kind == closeMessage && len(data) >= 2 {
		p.mu.Lock()
		p.code, p.reason = int(data[0])<<8|int(data[1]), string(data[2:])
		p.mu.Unlock()
	}
	return nil
}

func (p *pipe) SetReadLimit(int64)                        {}
func (p *pipe) SetReadDeadline(time.Time) error           { return nil }
func (p *pipe) SetWriteDeadline(time.Time) error          { return nil }
func (p *pipe) SetPongHandler(func(appData string) error) {}

func (p *pipe) Close() error {
	p.once.Do(func() { close(p.closed) })
	return nil
}

// send hands a frame to the hub.
func (p *pipe) send(frame []byte) error {
	select {
	case p.in <- frame:
		return nil
	case <-p.closed:
		return ErrClosed
	}
}

// closeFrame is the code and reason the hub closed the connection with.
func (p *pipe) closeFrame() (int, string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.code, p.reason
}
