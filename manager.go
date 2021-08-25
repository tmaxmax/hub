package hub

type (
	counter      Number
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

func newCounter(init Number) *counter {
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
func (c *counter) reset(init Number) {
	if init < 0 {
		return
	}
	*c = counter(init)
}

func getTopics(initial []Topic, defaultIfNone bool) []Topic {
	if defaultIfNone && len(initial) == 0 {
		return []Topic{nil}
	}
	return initial
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

func (m *manager) removeConnFromTopicNoRefCounter(t Topic, c Conn) bool {
	prevLen := len(m.topics[t])
	delete(m.topics[t], c)
	currLen := len(m.topics[t])

	if prevLen == currLen {
		return false
	} else if currLen == 0 {
		delete(m.topics, t)
	}

	return true
}

func (m *manager) removeConnFromTopicRefCountOnly(c Conn) bool {
	ref := m.conns[c]
	if ref.topics.dec() {
		delete(m.conns, c)
		if !ref.keep {
			close(c)
		}

		return true
	}

	return false
}

// removeConnFromTopic deletes the connection from the given topic and deletes the topic if it has no connections.
// Then it decrements the connection's topic counter and deletes the connection if it isn't connected to any topics.
// It also closes the connection channel if at connection KeepAlive wasn't specified. It returns true if the
// connection was removed.
func (m *manager) removeConnFromTopic(t Topic, c Conn) bool {
	if !m.removeConnFromTopicNoRefCounter(t, c) {
		return false
	}
	return m.removeConnFromTopicRefCountOnly(c)
}

func (m *manager) disconnectAll(d DisconnectAll) {
	c := Conn(d)
	ref, ok := m.conns[c]
	if !ok {
		return
	}

	if !ref.keep {
		close(c)
	}
	delete(m.conns, c)

	for t := range m.topics {
		m.removeConnFromTopicNoRefCounter(t, c)
	}
}

func (m *manager) disconnect(d *Disconnect) {
	if _, ok := m.conns[d.Conn]; !ok {
		return
	}

	for _, t := range getTopics(d.Topics, true) {
		if _, ok := m.topics[t]; !ok {
			continue
		}

		if m.removeConnFromTopic(t, d.Conn) {
			break
		}
	}
}

func (m *manager) closeTopics(c Close) {
	for _, t := range getTopics(c, true) {
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

func (m *manager) closeAllTopics() {
	for t, conns := range m.topics {
		delete(m.topics, t)
		for c := range conns {
			m.removeConnFromTopicRefCountOnly(c)
		}
	}
}

func (m *manager) message(msg *Message) {
	for _, t := range getTopics(msg.Topics, true) {
		for c, cnt := range m.topics[t] {
			c <- msg.Message

			if m.conns[c].messages.dec() {
				for t := range m.topics {
					m.removeConnFromTopic(t, c)
				}
			} else if cnt.dec() {
				m.removeConnFromTopic(t, c)
			}
		}
	}
}
