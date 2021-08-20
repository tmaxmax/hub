package hub

import (
	"context"
	"sync"
)

type Conn struct {
	messages        chan interface{}
	messageBuffer   int
	hubs            map[*Hub]struct{}
	deferredAttachs map[*Hub]struct{}
	receiving       bool
	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.RWMutex

	CloseOnNoAttachments bool
}

func NewConn(messageBuffer int) *Conn {
	return &Conn{
		messages:        make(chan interface{}, messageBuffer),
		messageBuffer:   messageBuffer,
		hubs:            map[*Hub]struct{}{},
		deferredAttachs: map[*Hub]struct{}{},
	}
}

type ClosedErr struct {
	what string
	err  error
}

func (c *ClosedErr) Error() string {
	return c.what + " closed: " + c.err.Error()
}

func (c *Conn) unsafeAttachTo(h *Hub, _ ...interface{}) {
	h.attach <- c
	c.hubs[h] = struct{}{}
}

func (c *Conn) AttachTo(h *Hub, topic ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.receiving {
		c.unsafeAttachTo(h, topic...)
	} else {
		c.deferredAttachs[h] = struct{}{}
	}
}

func (c *Conn) unsafeDetachFrom(h *Hub, _ ...interface{}) {
	if _, attachedTo := c.hubs[h]; attachedTo {
		h.detach <- c
		delete(c.hubs, h)

		if c.CloseOnNoAttachments && len(c.hubs) == 0 {
			c.cancel()
		}
	}
}

func (c *Conn) DetachFrom(h *Hub, topics ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.receiving {
		c.unsafeDetachFrom(h, topics...)
	} else {
		delete(c.deferredAttachs, h)
	}
}

func (c *Conn) Send(ctx context.Context, message interface{}) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	select {
	case <-c.ctx.Done():
		return &ClosedErr{what: "conn", err: c.ctx.Err()}
	case c.messages <- message:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Conn) unsafeInit(ctx context.Context) bool {
	if c.receiving {
		return false
	}

	c.ctx, c.cancel = context.WithCancel(ctx)
	c.receiving = true

	for h := range c.deferredAttachs {
		c.unsafeAttachTo(h)
	}
	c.deferredAttachs = map[*Hub]struct{}{}

	go func() {
		<-c.ctx.Done()

		c.mu.Lock()
		defer c.mu.Unlock()

		for h := range c.hubs {
			c.unsafeDetachFrom(h)
		}
		c.hubs = map[*Hub]struct{}{}

		close(c.messages)
		c.messages = make(chan interface{})
		c.receiving = false
	}()

	return true
}

func (c *Conn) Receive(ctx context.Context) <-chan interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.unsafeInit(ctx) {
		return c.messages
	}
	return nil
}

func (c *Conn) context() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.ctx
}
