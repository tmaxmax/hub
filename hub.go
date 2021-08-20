package hub

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/tmaxmax/go-unbounded-chan"
)

type Hub struct {
	broadcast   chan interface{}
	attach      chan conn
	detach      chan conn
	connections map[conn]chan<- interface{}
	running     bool
	ctx         context.Context
	mu          sync.RWMutex
	wg          sync.WaitGroup

	Log             *log.Logger
	SendTimeout     time.Duration
	MaxRetries      int
	BackoffUnit     time.Duration
	Backoff         BackoffFunction
	BackoffJitter   float64
	BroadcastBuffer int
}

type conn interface {
	DetachFrom(*Hub, ...interface{})
	Send(context.Context, interface{}) error
	context() context.Context
}

func NewHub(broadcastBuffer int) *Hub {
	return &Hub{
		broadcast:   make(chan interface{}, broadcastBuffer),
		attach:      make(chan conn),
		detach:      make(chan conn),
		connections: make(map[conn]chan<- interface{}),
		MaxRetries:  1,
	}
}

func (h *Hub) newContext() (context.Context, context.CancelFunc) {
	if h.SendTimeout != 0 {
		return context.WithTimeout(h.ctx, h.SendTimeout)
	}

	return h.ctx, nil
}

func (h *Hub) trySend(c conn, m interface{}) bool {
	ctx, cancel := h.newContext()
	if cancel != nil {
		defer cancel()
	}

	if err := c.Send(ctx, m); err != nil {
		if _, ok := err.(*ClosedErr); !ok {
			return false
		}
		c.DetachFrom(h)
	}

	return true
}

func (h *Hub) attachConn(c conn) {
	in, out := unbounded.NewChan()
	h.connections[c] = in

	h.wg.Add(1)
	go func() {
		defer h.wg.Done()

		for m := range out {
			var i int
			for ; i < h.MaxRetries; i++ {
				if i > 0 {
					select {
					case <-h.ctx.Done():
						return
					case <-c.context().Done():
						return
					case <-time.After(h.Backoff(h.BackoffUnit, h.BackoffJitter, i)):
						break
					}
				}
				if h.trySend(c, m) {
					break
				}
			}
			if i == h.MaxRetries {
				c.DetachFrom(h)
				return
			}
		}
	}()
}

func (h *Hub) detachConn(c conn) {
	close(h.connections[c])
	delete(h.connections, c)
}

func (h *Hub) teardown() {
	h.mu.Lock()
	defer h.mu.Unlock()

	go func() {
		for c := range h.detach {
			close(h.connections[c])
		}
		h.connections = map[conn]chan<- interface{}{}
	}()

	for c := range h.connections {
		c.DetachFrom(h)
	}
	close(h.detach)

	h.wg.Wait()
	h.running = false
}

func (h *Hub) Start(ctx context.Context) {
	h.mu.Lock()
	if h.running {
		return
	}
	h.ctx = ctx
	h.mu.Unlock()

	for {
		select {
		case c := <-h.attach:
			h.attachConn(c)
		case c := <-h.detach:
			h.detachConn(c)
		case m := <-h.broadcast:
			for _, ch := range h.connections {
				ch <- m
			}
		case <-h.ctx.Done():
			h.teardown()

			return
		}
	}
}

func (h *Hub) Broadcast(ctx context.Context, message interface{}, _ interface{}) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	select {
	case h.broadcast <- message:
		return nil
	case <-h.ctx.Done():
		return fmt.Errorf("hub done: %w", h.ctx.Err())
	case <-ctx.Done():
		return ctx.Err()
	}
}
