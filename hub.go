/*
Package hub provides primitives for working with the publish-subscribe messaging model.
All the types are Go channels, so the usage is identical to theirs.

The pattern is simple: publish messages to given topics. To do this a series of commands
are sent to the Hub, which is a plain channel, commands that are executed afterwards. Most
commands take topics as parameters, but if the topics are not specified a default topic
(a topic identified by the nil interface) is used.
*/
package hub

type (
	// Hub is the coordinator channel on which commands are sent. Even though in general
	// channels don't have to be closed, the Hub must be, or resources will be leaked otherwise.
	Hub chan interface{}
	// Conn is a connection. It is a channel on which the Hub sends messages
	// from each Topic the Conn is connected to.
	//
	// Message a Conn as a command and the Hub will connect it to the default topic.
	Conn chan interface{}
	// Topic is an identifier for a topic.
	Topic interface{}

	// Number is an alias for int. If a struct field of a command that manipulates a connections has this type,
	// it means that the first time the command is sent negative values are ignored,
	// and on subsequent times negative values do not reset specific properties of
	// the connections that the respective values manipulate.
	Number = int

	// TopicConn represents a connection to a topic. Use it with ConnectEach
	// to describe how the connection connects to topics.
	TopicConn struct {
		Topic Topic
		// The number of messages the connection should receive from the given topic.
		// If the ConnectEach command is sent multiple times for the same connection,
		// the number of messages is reset to the new value.
		MessageCount Number
	}

	// Connect is a command that tells the Hub to connect a Conn to the given topics.
	// After connecting, the Conn will receive messages from all the topics until it
	// disconnects, the topics are closed or the hub is closed.
	//
	// If no topics are specified, the Conn will receive all messages from the default topic.
	Connect struct {
		Conn   Conn
		Topics []Topic
		// The total number of messages the connection should receive.
		// Reset this value for the connection by resending this command with
		// the same Conn.
		MessageCount Number
		// Set this to true if you want the hub to not close the Conn channel automatically
		// when the Conn isn't connected to any topics, or it has received the specified
		// number of messages.
		KeepAlive bool
	}
	// ConnectEach is similar to Connect, but you can also specify how many messages
	// the Conn should receive from each Topic individually. In other words, Connect
	// would basically be ConnectEach with unset TopicConn.MessageCount.
	//
	// If no topics are specified, the Conn will receive all messages from the default topic.
	ConnectEach struct {
		Conn         Conn
		Topics       []TopicConn
		MessageCount Number
		KeepAlive    bool
	}
	// Disconnect is a command that tells the Hub to stop sending messages from the
	// given topics to the Conn. If no topics are given, the Conn is disconnected
	// from the default topic. Also, if KeepAlive wasn't set on connection its channel is also
	// closed.
	Disconnect struct {
		Conn   Conn
		Topics []Topic
	}

	// DisconnectAll is the same as Disconnect, but it disconnects the Conn from all the
	// topics it is connected to.
	DisconnectAll Conn

	// Message is a command that tells the Hub to publish the given Message to each
	// given Topic. If no topic is provided, the Hub publishes is to the default topic.
	Message struct {
		Message interface{}
		Topics  []Topic
	}

	// Close is a command that tells the hub to disconnect all connections that are
	// connected to the given topics. If no topics are given the default topic is
	// closed.
	Close []Topic
	// CloseAll is similar to Close, but it disconnects the connections from all topics.
	CloseAll struct{}
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

// New creates a Hub channel and starts the command execution loop.
// It also returns a channel that blocks until the hub is closed.
func New() (Hub, <-chan struct{}) {
	h := make(Hub)
	done := make(chan struct{})

	go func() {
		h.Start()
		close(done)
	}()

	return h, done
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
		case DisconnectAll:
			m.disconnectAll(v)
		case Close:
			m.closeTopics(v)
		case CloseAll:
			m.closeAllTopics()
		case Conn:
			m.connectEach(&ConnectEach{Conn: v})
		default:
			m.message(&Message{Message: v})
		}
	}
}

// Connect is a shortcut for the creating a Conn and sending a Connect command to the Hub.
func (h Hub) Connect(topics ...Topic) Conn {
	conn := make(Conn)

	h <- Connect{
		Conn:   conn,
		Topics: topics,
	}

	return conn
}

// Disconnect is a shortcut for sending a Disconnect command to the Hub.
func (h Hub) Disconnect(c Conn, topics ...Topic) {
	h <- Disconnect{
		Conn:   c,
		Topics: topics,
	}
}

// DisconnectAll is a shortcut for sending a DisconnectAll command to the Hub.
func (h Hub) DisconnectAll(c Conn) {
	h <- DisconnectAll(c)
}

// Send is a shortcut for sending a Message command to the Hub.
func (h Hub) Send(message interface{}, topics ...Topic) {
	h <- Message{
		Message: message,
		Topics:  topics,
	}
}

// Close is a shortcut for sending a Close command to the Hub.
func (h Hub) Close(topics ...Topic) {
	h <- Close(topics)
}
