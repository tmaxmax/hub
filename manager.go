package hub

type (
	counter      int
	connRefCount struct {
		topics   counter
		messages counter
		keep     bool
	}
	manager struct {
		topics map[Topic]map[Conn]*counter
		conns  map[Conn]*connRefCount
	}
)

func newCounter(init int) *counter {
	v := counter(init)
	return &v
}

func (c *counter) inc() {
	*c++
}

// dec returns false if the counter is already 0. it otherwise decrements the counter
// and returns true if the counter is 0 after decrementing.
func (c *counter) dec() bool {
	if *c == 0 {
		return false
	}

	*c--
	return *c == 0
}

// reset sets the counter's value if the given value is positive or 0.
func (c *counter) reset(init int) {
	if init < 0 {
		return
	}
	*c = counter(init)
}

func newManager() *manager {
	return &manager{
		topics: map[Topic]map[Conn]*counter{},
		conns:  map[Conn]*connRefCount{},
	}
}

func (m *manager) close() {
	for c, ref := range m.conns {
		if !ref.keep {
			close(c)
		}
	}
}

func (m *manager) getTopicConns(t Topic) map[Conn]*counter {
	if _, ok := m.topics[t]; !ok {
		m.topics[t] = map[Conn]*counter{}
	}
	return m.topics[t]
}

func (m *manager) connect(c *Connect) {
	m.connectEach(c.toConnectEach())
}

func (m *manager) connectEach(c *ConnectEach) {
	topics := c.Topics
	ref, ok := m.conns[c.Conn]
	if len(topics) == 0 && !ok {
		topics = []TopicConn{{}}
	}

	if ok {
		for _, t := range topics {
			topic := m.getTopicConns(t.Topic)
			cnt, ok := topic[c.Conn]
			if ok {
				cnt.reset(t.MessageCount)
			} else {
				topic[c.Conn] = newCounter(t.MessageCount)
				ref.topics.inc()
			}
		}
		ref.keep = c.KeepAlive
		ref.messages.reset(c.MessageCount)

		return
	}

	m.conns[c.Conn] = &connRefCount{
		topics:   counter(len(topics)),
		messages: counter(c.MessageCount),
		keep:     c.KeepAlive,
	}
	for _, t := range topics {
		m.getTopicConns(t.Topic)[c.Conn] = newCounter(t.MessageCount)
	}
}

func (m *manager) removeConnFromTopicNoRefCounter(t Topic, c Conn) {
	delete(m.topics[t], c)
	if len(m.topics[t]) == 0 {
		delete(m.topics, t)
	}
}

func (m *manager) removeConnFromTopicRefCountOnly(c Conn) bool {
	ref := m.conns[c]
	if ref.topics.dec() {
		delete(m.conns, c)
		if !ref.keep {
			close(c)
		}

		return false
	}

	return true
}

// removeConnFromTopic deletes the connection from the given topic and deletes the topic if it has no connections.
// Then it decrements the connection's topic counter and deletes the connection if it isn't connected to any topics.
// It also closes the connection channel if at connection KeepAlive wasn't specified. It returns true if the
// connection still exists after removal.
func (m *manager) removeConnFromTopic(t Topic, c Conn) bool {
	m.removeConnFromTopicNoRefCounter(t, c)
	return m.removeConnFromTopicRefCountOnly(c)
}

func (m *manager) disconnect(d *Disconnect) {
	c := d.Conn
	_, ok := m.conns[c]
	if !ok {
		return
	}

	close(c)
	delete(m.conns, c)

	if len(d.Topics) == 0 {
		for t := range m.topics {
			m.removeConnFromTopicNoRefCounter(t, c)
		}
	} else {
		for _, t := range d.Topics {
			m.removeConnFromTopicNoRefCounter(t, c)
		}
	}
}

func (m *manager) closeTopics(c Close) {
	topics := []Topic(c)

	for _, t := range topics {
		conns, ok := m.topics[t]
		if !ok {
			continue
		}

		delete(m.topics, t)
		for c := range conns {
			m.removeConnFromTopicRefCountOnly(c)
		}
	}
}

func (m *manager) message(msg *Message) {
	topics := msg.Topics
	if len(topics) == 0 {
		topics = []Topic{nil}
	}

	for _, t := range topics {
		for c := range m.topics[t] {
			c <- msg.Message

			exists := true
			if m.topics[t][c].dec() {
				exists = m.removeConnFromTopic(t, c)
			}

			if exists && m.conns[c].messages.dec() {
				for t := range m.topics {
					m.removeConnFromTopic(t, c)
				}
			}
		}
	}
}
