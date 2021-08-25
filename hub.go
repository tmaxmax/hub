/*
Package hub provides primitives for working with the publish-subscribe messaging model.
All the types are Go channels, so the usage is identical to theirs.
*/
package hub

type (
	Hub   chan interface{}
	Conn  chan interface{}
	Topic interface{}

	TopicConn struct {
		Topic        Topic
		MessageCount int
	}

	Connect struct {
		Conn         Conn
		Topics       []Topic
		MessageCount int
		KeepAlive    bool
	}
	ConnectEach struct {
		Conn         Conn
		Topics       []TopicConn
		MessageCount int
		KeepAlive    bool
	}
	Disconnect struct {
		Conn   Conn
		Topics []Topic
	}

	Message struct {
		Message interface{}
		Topics  []Topic
	}

	Close []Topic
)

func (c *Connect) toConnectEach() *ConnectEach {
	topics := make([]TopicConn, 0, len(c.Topics))
	for _, t := range c.Topics {
		topics = append(topics, TopicConn{Topic: t})
	}

	return &ConnectEach{
		Conn:         c.Conn,
		Topics:       topics,
		MessageCount: c.MessageCount,
		KeepAlive:    c.KeepAlive,
	}
}

func New() Hub {
	h := make(Hub)

	go h.Start()

	return h
}

// Start starts the hub. Run this in a new goroutine. Don't call Start if you have created the
// Hub using New!
func (h Hub) Start() {
	m := newManager()
	defer m.close()

	for cmd := range h {
		switch v := cmd.(type) {
		case Message:
			m.message(&v)
		case Connect:
			m.connect(&v)
		case ConnectEach:
			m.connectEach(&v)
		case Disconnect:
			m.disconnect(&v)
		case Close:
			m.closeTopics(v)
		case Conn:
			m.connectEach(&ConnectEach{Conn: v})
		default:
			m.message(&Message{Message: v})
		}
	}
}

func (h Hub) Connect(topics ...Topic) Conn {
	conn := make(Conn)

	h <- Connect{
		Conn:   conn,
		Topics: topics,
	}

	return conn
}

func (h Hub) Disconnect(c Conn, topics ...Topic) {
	h <- Disconnect{
		Conn:   c,
		Topics: topics,
	}
}

func (h Hub) Send(message interface{}, topics ...Topic) {
	h <- Message{
		Message: message,
		Topics:  topics,
	}
}

func (h Hub) Close(topics ...Topic) {
	h <- Close(topics)
}

func (h Hub) Stop() {
	close(h)
}
