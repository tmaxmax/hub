package hub_test

import (
	"reflect"
	"testing"

	"github.com/tmaxmax/hub"
)

func checkContents(tb testing.TB, c hub.Conn, expected ...interface{}) {
	tb.Helper()

	var got []interface{}
	for v := range c {
		got = append(got, v)
	}

	if !reflect.DeepEqual(got, expected) {
		tb.Fatalf("Invalid channel contents.\nExpected %#v\nGot %#v", expected, got)
	}
}

func assertPanic(tb testing.TB, message string, f func()) {
	tb.Helper()

	defer func() {
		_ = recover()
	}()

	f()
	tb.Fatal(message)
}

func TestMessageInexistentTopic(t *testing.T) {
	h, done := hub.New()
	h <- hub.Message{Topics: []hub.Topic{"A"}}
	close(h)
	<-done
}

func TestConnDefault(t *testing.T) {
	h, done := hub.New()
	conn := make(hub.Conn, 1)

	h <- hub.Connect{Conn: conn}
	h.Send("Hello world!")

	close(h)
	<-done

	checkContents(t, conn, "Hello world!")
}

func TestConnTopics(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B"}
	conn := make(hub.Conn, len(topics))

	h <- hub.Connect{Conn: conn, Topics: topics}
	h <- hub.Message{Message: "Hello world!", Topics: topics}
	close(h)
	<-done

	checkContents(t, conn, "Hello world!", "Hello world!")
}

func TestDisconnect(t *testing.T) {
	h, done := hub.New()
	conn := make(hub.Conn, 1)

	h <- conn
	h <- "Hello world!"
	h <- hub.Disconnect{Conn: conn}
	h <- "This should not be received"
	close(h)
	<-done

	checkContents(t, conn, "Hello world!")
}

func TestDisconnectTopics(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B", "C"}
	conn := make(hub.Conn, len(topics)+1)

	h <- hub.Connect{Conn: conn, Topics: topics}
	h <- hub.Message{Message: "Hello world!", Topics: topics}
	h.Disconnect(conn, topics[:2]...)
	h <- hub.Message{Message: "Hello again!", Topics: topics}
	close(h)
	<-done

	checkContents(t, conn, "Hello world!", "Hello world!", "Hello world!", "Hello again!")
}

func TestDisconnectInvalidConn(t *testing.T) {
	h, done := hub.New()
	h <- hub.Disconnect{}
	close(h)
	<-done
}

func TestDisconnectInvalidOrNotConnectedTopics(t *testing.T) {
	h, done := hub.New()
	a, b := make(hub.Conn, 3), make(hub.Conn, 4)

	h <- hub.Connect{Conn: a, Topics: []hub.Topic{"A", "B"}}
	h <- hub.Connect{Conn: b, Topics: []hub.Topic{"A", "C"}}
	h <- hub.Message{Message: "First", Topics: []hub.Topic{"A", "B", "C"}}
	h <- hub.Disconnect{Conn: a, Topics: []hub.Topic{"A", "C", "D"}}
	h <- hub.Message{Message: "Second", Topics: []hub.Topic{"A", "B", "C"}}

	close(h)
	<-done

	checkContents(t, a, "First", "First", "Second")
	checkContents(t, b, "First", "First", "Second", "Second")
}

func TestConnKeepAlive(t *testing.T) {
	h, done := hub.New()
	conn := make(hub.Conn, 2)

	h <- hub.Connect{Conn: conn, KeepAlive: true}
	h <- "Hello world!"
	h <- hub.Disconnect{Conn: conn}
	h <- "Hello again!"
	conn <- "Hello there!"
	close(h)
	<-done
	close(conn)

	checkContents(t, conn, "Hello world!", "Hello there!")
}

func TestDisconnectAll(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B"}
	a, b := make(hub.Conn, 1), make(hub.Conn, 3)

	h <- hub.Connect{Conn: a, Topics: topics[:1]}
	h <- hub.Connect{Conn: b, Topics: topics, KeepAlive: true}
	h <- hub.Message{Message: "First", Topics: topics}
	h <- hub.DisconnectAll(a)
	h <- hub.DisconnectAll(b)
	h.DisconnectAll(nil)
	h <- hub.Message{Message: "Second", Topics: topics}
	b <- "This is for you only!"
	close(h)
	<-done
	close(b)

	checkContents(t, a, "First")
	checkContents(t, b, "First", "First", "This is for you only!")
}

func TestCappedMessages(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B", "C"}
	limit := 5
	conn := make(hub.Conn, limit)

	h <- hub.Connect{
		Conn:         conn,
		Topics:       topics,
		MessageCount: limit,
	}
	h <- hub.Message{Message: "First", Topics: topics}
	h <- hub.Message{Message: "Second", Topics: topics}
	close(h)
	<-done

	checkContents(t, conn, "First", "First", "First", "Second", "Second")
}

func TestCappedMessagesPerTopic(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B", "C"}
	topicConns := []hub.TopicConn{
		{Topic: "A", MessageCount: 1},
		{Topic: "B", MessageCount: 2},
		{Topic: "C", MessageCount: 3},
	}
	conn := make(hub.Conn, 6)

	h <- hub.ConnectEach{
		Conn:   conn,
		Topics: topicConns,
	}
	h <- hub.Message{Message: "First", Topics: topics}
	h <- hub.Message{Message: "Second", Topics: topics}
	h <- hub.Message{Message: "Third", Topics: topics}
	close(h)
	<-done

	checkContents(t, conn, "First", "First", "First", "Second", "Second", "Third")
}

func TestCappedMessagesCombined(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B"}
	topicConns := []hub.TopicConn{
		{Topic: "A", MessageCount: 1},
		{Topic: "B", MessageCount: 2},
	}
	a, b := make(hub.Conn, 2), make(hub.Conn, 3)

	h <- hub.ConnectEach{
		Conn:         a,
		Topics:       topicConns,
		MessageCount: 2,
	}
	h <- hub.ConnectEach{
		Conn:         b,
		Topics:       topicConns,
		MessageCount: 4,
	}
	h <- hub.Message{Message: "First", Topics: topics}
	h <- hub.Message{Message: "Second", Topics: topics}
	close(h)
	<-done

	checkContents(t, a, "First", "First")
	checkContents(t, b, "First", "First", "Second")
}

func TestCloseTopics(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B"}
	a, b, c := make(hub.Conn), make(hub.Conn, 1), make(hub.Conn, 1)

	h <- hub.Connect{
		Conn:   a,
		Topics: topics[:1],
	}
	h <- hub.Connect{
		Conn:   b,
		Topics: topics[1:],
	}
	h <- hub.Connect{
		Conn:      c,
		Topics:    topics[:1],
		KeepAlive: true,
	}
	h.Close(topics[:1]...)
	h <- hub.Message{Message: "From hub", Topics: topics}
	c <- "For C only"
	close(h)
	<-done
	close(c)

	assertPanic(t, "Channel a should have been closed by Hub", func() {
		close(a)
	})
	checkContents(t, b, "From hub")
	checkContents(t, c, "For C only")
}

func TestCloseDefaultTopic(t *testing.T) {
	h, done := hub.New()
	conn := h.Connect()
	h.Close()
	close(h)
	<-done

	assertPanic(t, "Channel should have been closed by Hub", func() {
		close(conn)
	})
}

func TestCloseInexistentTopic(t *testing.T) {
	h, done := hub.New()
	h.Close("lol")
	close(h)
	<-done
}

func TestCloseAllTopics(t *testing.T) {
	h, done := hub.New()
	topics := []hub.Topic{"A", "B"}
	a := h.Connect(topics...)
	b := make(hub.Conn, 1)

	h <- hub.Connect{
		Conn:      b,
		Topics:    topics,
		KeepAlive: true,
	}
	h <- hub.CloseAll{}
	h <- hub.Message{Message: "From hub", Topics: topics}
	close(h)
	<-done

	b <- "For B only"
	close(b)

	assertPanic(t, "Channel A should have been closed by hub", func() {
		close(a)
	})
	checkContents(t, b, "For B only")
}

func TestOverwriteConnectionProperties(t *testing.T) {
	h, done := hub.New()
	conn := make(hub.Conn, 13)

	h <- hub.Connect{
		Conn:         conn,
		MessageCount: 2,
	}
	h <- "First"
	h <- hub.ConnectEach{
		Conn: conn,
		Topics: []hub.TopicConn{
			{Topic: "A", MessageCount: 3},
			{Topic: "B", MessageCount: 2},
		},
		MessageCount: 7,
	}
	h <- hub.Message{Message: "Second", Topics: []hub.Topic{"A", "B"}}
	h <- hub.ConnectEach{
		Conn:         conn,
		Topics:       []hub.TopicConn{{Topic: "A", MessageCount: 2}},
		MessageCount: -1,
	}
	h <- hub.Message{Message: "Third", Topics: []hub.Topic{"A", "B"}}
	h <- hub.Message{Message: "Fourth", Topics: []hub.Topic{"A", "B"}}
	h <- "Fifth"
	h <- hub.Connect{
		Conn:      conn,
		Topics:    []hub.Topic{"C"},
		KeepAlive: true,
	}
	h <- hub.Message{Message: "Sixth", Topics: []hub.Topic{"C"}}
	h <- "Seventh"
	h <- "Eighth"
	h <- "Ninth"
	h <- "Tenth"
	close(h)
	<-done

	conn <- "For conn only"
	close(conn)

	checkContents(
		t,
		conn,
		"First",
		"Second", "Second",
		"Third", "Third",
		"Fourth",
		"Fifth",
		"Sixth",
		"Seventh",
		"Eighth",
		"Ninth",
		"Tenth",
		"For conn only")
}
