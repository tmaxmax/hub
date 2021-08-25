package hub

// ReceiveNFromTopics creates a channel that receives a total of n messages from the given topics.
// This channel is not a Conn and it is not directly connected to any topic. Prefer using the
// Connect struct for receiving a limited number of messages from a single topic
func ReceiveNFromTopics(n int, topics ...Topic) chan interface{} {
	if len(topics) == 0 {
		return nil
	}

	conn := make(Conn)
	out := make(chan interface{})

	for _, t := range topics {
		t <- conn
	}

	go func() {
		disconnected := false
		for msg := range conn {
			if n == 0 {
				if disconnected {
					continue
				}

				disconnected = true
				close(out)

				go func() {
					for _, t := range topics {
						t <- Disconnect(conn)
					}
				}()

				continue
			}
			out <- msg
			n--
		}
	}()

	return out
}
