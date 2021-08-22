package hub

type (
	// The Hub channel is used for registering and unregistering topics.
	// Closing this channel will close all topics and connections.
	//
	//   hub := New()
	//   topic := make(Topic) // you can use a buffered channel too
	//   hub <- topic // the topic now accepts broadcasts
	//   hub <- RemoveTopic(topic) // the topic channel will be closed and all connections will be disconnected from it.
	//   close(hub) // this will close all topics and connections
	//
	// See also the New example.
	Hub chan interface{}
	// The Topic channel is used to broadcast messages to connections.
	// To connect to a Topic, send a Conn to the channel:
	//
	//   topic := make(Topic) // you can use a buffered channel too
	//   // register to a hub
	//   conn := make(Conn)
	//   topic <- conn // the connection will receive all messages broadcasted to the topic
	//   topic <- Disconnect(conn) // the connection will stop receiving messages from this topic
	//
	// See altso the New example.
	Topic chan interface{}
	// The Conn channel is used for receiving messages from the topics it's connected to.
	//
	//   conn := make(Conn) // you can use a buffered channel too
	//   // connect to a registered topic
	//   for message := range conn {
	//     fmt.Println(message)
	//   }
	//
	// See the examples for Topic and New for usage.
	Conn chan interface{}
	// The Disconnect type is a Conn that when sent to a Topic it tells that Topic to stop sending the given Conn
	// messages. See the Topic and New examples for usage.
	Disconnect Conn
	// The RemoveTopic type is a Topic that when sent to a Hub it tells that Hub to close the given Topic. Broadcasting
	// new messages won't work after this. See the Topic and New examples for usage.
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
