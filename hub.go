package hub

type (
	Hub         chan interface{}
	Topic       chan interface{}
	Conn        chan interface{}
	Disconnect  Conn
	RemoveTopic Topic

	refCounter chan interface{}
)

// New creates and starts a Hub instance.
func New() Hub {
	hub := make(Hub)

	go hub.start()

	return hub
}

func (r refCounter) start(done chan struct{}) {
	conns := map[Conn]uint64{}

	for {
		select {
		case cmd := <-r:
			switch v := cmd.(type) {
			case Conn:
				conns[v]++
			case Disconnect:
				c := Conn(v)
				conns[c]--
				if conns[c] == 0 {
					close(c)
					delete(conns, c)
				}
			}
		case <-done:
			if len(conns) == 0 {
				return
			}
		}
	}
}

func (h Hub) start() {
	topics := map[Topic]struct{}{}
	rc := make(refCounter)
	done := make(chan struct{})

	go rc.start(done)

	for cmd := range h {
		switch v := cmd.(type) {
		case Topic:
			if _, ok := topics[v]; ok {
				break
			}
			topics[v] = struct{}{}
			go v.start(rc)
		case RemoveTopic:
			delete(topics, Topic(v))
			close(v)
		default:
			panic("hub: Hub: message not supported")
		}
	}
	for t := range topics {
		close(t)
	}

	close(done)
}

func (t Topic) start(rc refCounter) {
	conns := map[Conn]struct{}{}

	for cmd := range t {
		switch v := cmd.(type) {
		case Conn:
			if _, ok := conns[v]; ok {
				break
			}
			conns[v] = struct{}{}
			rc <- v
		case Disconnect:
			delete(conns, Conn(v))
			rc <- v
		default:
			for c := range conns {
				c <- v
			}
		}
	}
	for c := range conns {
		rc <- Disconnect(c)
	}
}
