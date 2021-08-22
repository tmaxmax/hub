package hub_test

import (
	"fmt"
	"sync"

	"github.com/tmaxmax/hub"
)

func ExampleNew() {
	h := hub.New()
	// Create two topics. You can pass a buffer size to make if you desire.
	topicA, topicB := make(hub.Topic), make(hub.Topic)
	// Register the two topics. You must send them to Hub before broadcasting messages on the topics!
	h <- topicA
	h <- topicB

	// Create a connection.
	conn := make(hub.Conn)
	// Subscribe to both topics with the same connection
	topicA <- conn
	topicB <- conn

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		// Receive the broadcasted messages. The conn channel is automatically closed
		// if it is disconnected from all topics.
		for msg := range conn {
			fmt.Println(msg)
		}
		fmt.Println("Done!")
	}()

	// Broadcast messages to the two topics
	topicA <- "Hello from A"
	topicA <- "It's nice to see you"
	topicB <- "Hello from B"

	// Close a single topic. Do not close the topic channel yourself!
	h <- hub.RemoveTopic(topicA)
	// Close the hub channel. This will close all topics and connections
	close(h)

	wg.Wait()

	// Output:
	// Hello from A
	// It's nice to see you
	// Hello from B
	// Done!
}
