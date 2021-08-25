package hub_test

import (
	"context"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/tmaxmax/hub"
)

func Example() {
	h := hub.New()
	// Create two topics. You can pass a buffer size to make if you desire.
	topicA, topicB := make(hub.Topic), make(hub.Topic)
	// Register the two topics. You must send them to Hub before broadcasting messages on the topics!
	h <- topicA
	h <- topicB

	// Create a Connect.
	conn := make(hub.Conn)
	// Subscribe to both topics with the same Connect
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

func Example_oneFromEachTopic() {
	h := hub.New()
	topics := map[string]hub.Topic{
		"positiveNumbers": make(hub.Topic),
		"negativeNumbers": make(hub.Topic),
		"zeroes":          make(hub.Topic),
	}
	conn := make(hub.Conn)
	wg := sync.WaitGroup{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	worker := func(t hub.Topic, gen func() int) {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(rand.Intn(200)) * time.Millisecond):
				t <- gen()
			}
		}
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		var messages []int

		for msg := range conn {
			messages = append(messages, msg.(int))
		}

		sort.Ints(messages)

		fmt.Printf("Received %d messages\n", len(messages))
		if len(messages) == 3 {
			if messages[0] < 0 {
				fmt.Println("One smaller than 0")
			}
			if messages[1] == 0 {
				fmt.Println("One equal to 0")
			}
			if messages[2] > 0 {
				fmt.Println("One greater than 0")
			}
		}
	}()

	for _, t := range topics {
		h <- t
		t <- hub.Connect{Conn: conn, MessageCount: 1}
	}

	wg.Add(len(topics))
	go worker(topics["positiveNumbers"], func() int {
		return rand.Int() + 1
	})
	go worker(topics["negativeNumbers"], func() int {
		return -rand.Int() - 1
	})
	go worker(topics["zeroes"], func() int {
		return 0
	})

	<-ctx.Done()
	close(h)

	wg.Wait()

	// Output:
	// Received 3 messages
	// One smaller than 0
	// One equal to 0
	// One greater than 0
}

func ExampleReceiveNFromTopics() {
	h := hub.New()
	topicA, topicB := make(hub.Topic), make(hub.Topic)
	// Registering topics
	h <- topicA
	h <- topicB
	one := hub.ReceiveNFromTopics(1, topicA, topicB)
	connAll := make(hub.Conn)
	// Subscribing connection
	topicA <- connAll
	topicB <- connAll

	wg := sync.WaitGroup{}
	connOneMsg := make(chan string, 1)
	connAllMsgs := make(chan []string, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		connOneMsg <- (<-one).(string)

		fmt.Println("connOne done! message received.")
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		var received []string
		for msg := range connAll {
			received = append(received, msg.(string))
		}

		connAllMsgs <- received
		// no need to disconnect, as the hub will close the conn automatically
		fmt.Println("connAll done! messages received.")
	}()

	topicA <- "Hello from A"
	topicB <- "Hello from B"
	topicA <- "Another message from A"
	topicA <- "Still from A"
	topicB <- "This message is your warning, hub will be closed"

	close(h)
	wg.Wait()

	oneMsg := <-connOneMsg
	allMsgs := <-connAllMsgs

	for _, msg := range allMsgs {
		if oneMsg == msg {
			fmt.Println("connOne received a message connAll also received!")
			break
		}
	}

	fmt.Printf("connAll received %d/5 messages.", len(allMsgs))

	// Output:
	// connOne done! message received.
	// connAll done! messages received.
	// connOne received a message connAll also received!
	// connAll received 5/5 messages.
}
