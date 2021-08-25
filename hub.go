/*
Package hub provides primitives for working with the publish-subscribe messaging model.
All the types are Go channels, so the usage is identical to theirs.
*/
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
	//   topic <- conn // the Connect will receive all messages broadcasted to the topic
	//   topic <- Disconnect(conn) // the Connect will stop receiving messages from this topic
	//
	// See also the New example.
	Topic chan interface{}
	// The Conn channel is used for receiving messages from the topics it's connected to.
	//
	//   conn := make(Conn) // you can use a buffered channel too
	//   // connect to a registered topic
	//   for message := range conn {
	//     fmt.Println(message)
	//   }
	//
	// Sending a Conn to a topic directly can be seen as a shorthand for
	//
	//   topic <- Connect{Conn: conn}
	//
	// Also see the package examples for usage.
	Conn chan interface{}
	// The Connect struct is a message that signals a topic to start sending messages to a connection.
	// If MessageCount is provided, the given topic will disconnect the connection after it has sent
	// that number of messages to it.
	Connect struct {
		// The number of messages the topic should send to the Conn.
		MessageCount int
		// The connection the topic should send messages to.
		Conn Conn
	}
	// The Disconnect type is a Conn that when sent to a Topic it tells that Topic to stop sending the given Conn
	// messages.
	Disconnect Conn
	// The RemoveTopic type is a Topic that when sent to a Hub it tells that Hub to close the given Topic. Broadcasting
	// new messages won't work after this. See the package examples for usage.
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
	getDone := func() chan struct{} {
		if len(conns) > 0 {
			return nil
		}
		return done
	}

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
		case <-getDone():
			return
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
	// TODO: Data race between topics senders and hub when hub is closing the topics.
	// Find a way to close the topics automatically when the hub closes without
	// the hub's goroutine closing them.
	for t := range topics {
		close(t)
	}

	close(done)
}

func (t Topic) start(rc refCounter) {
	conns := map[Conn]int{}
	connect := func(c Connect) {
		if _, ok := conns[c.Conn]; ok {
			return
		}
		conns[c.Conn] = c.MessageCount
		rc <- c.Conn
	}

	for cmd := range t {
		switch v := cmd.(type) {
		case Conn:
			connect(Connect{Conn: v})
		case Connect:
			connect(v)
		case Disconnect:
			c := Conn(v)
			if _, ok := conns[c]; !ok {
				break
			}
			delete(conns, c)
			rc <- v
		default:
			for c, msgCount := range conns {
				c <- v
				if msgCount == 0 {
					// this conn receives an indefinite number of messages
					continue
				}
				msgCount--
				if msgCount == 0 {
					// this conn received all the messages requested
					delete(conns, c)
					rc <- Disconnect(c)
				} else {
					conns[c] = msgCount
				}
			}
		}
	}
	for c := range conns {
		rc <- Disconnect(c)
	}
}
