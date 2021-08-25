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
	h, done := hub.New()
	conn := h.Connect()

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		for msg := range conn {
			fmt.Println(msg)
		}
		fmt.Println("Done!")
	}()

	h.Send("Hello")
	h.Send("It's nice to see you")
	h.Send("I'll leave now")

	// Close the hub and wait for resources to be freed
	close(h)
	<-done

	// Wait for client
	wg.Wait()

	// Output:
	// Hello
	// It's nice to see you
	// I'll leave now
	// Done!
}

func Example_raw() {
	h, conn, done := make(hub.Hub), make(hub.Conn), make(chan struct{})

	go func() {
		h.Start()
		close(done)
	}()

	h <- hub.Connect{Conn: conn}

	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()

		for msg := range conn {
			fmt.Println(msg)
		}
		fmt.Println("Done!")
	}()

	h <- "Hello"
	h <- "It's nice to see you"
	h <- "I'll leave now"

	close(h)
	<-done

	wg.Wait()

	// Output:
	// Hello
	// It's nice to see you
	// I'll leave now
	// Done!
}

func Example_oneFromEachTopic() {
	h, done := hub.New()
	conn := make(hub.Conn)

	h <- hub.ConnectEach{
		Conn: conn,
		Topics: []hub.TopicConn{
			{Topic: "positiveNumbers", MessageCount: 1},
			{Topic: "negativeNumbers", MessageCount: 1},
			{Topic: "zeroes", MessageCount: 1},
		},
	}

	wg := sync.WaitGroup{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	worker := func(topic string, gen func() int) {
		defer wg.Done()

		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(rand.Intn(200)) * time.Millisecond):
				h <- hub.Message{
					Message: gen(),
					Topics:  []hub.Topic{topic},
				}
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

	wg.Add(3)
	go worker("positiveNumbers", func() int {
		return rand.Int() + 1
	})
	go worker("negativeNumbers", func() int {
		return -rand.Int() - 1
	})
	go worker("zeroes", func() int {
		return 0
	})

	<-ctx.Done()
	wg.Wait()

	close(h)
	<-done

	// Output:
	// Received 3 messages
	// One smaller than 0
	// One equal to 0
	// One greater than 0
}

func Example_receiveNFromTopics() {
	h, done := hub.New()
	connAll, connOne := make(hub.Conn), make(hub.Conn)

	h <- hub.Connect{Conn: connAll, Topics: []hub.Topic{"A", "B"}}
	h <- hub.Connect{Conn: connOne, Topics: []hub.Topic{"A", "B"}, MessageCount: 1}

	wg := sync.WaitGroup{}
	connOneMsg := make(chan string, 1)
	connAllMsgs := make(chan []string, 1)

	wg.Add(1)
	go func() {
		defer wg.Done()

		connOneMsg <- (<-connOne).(string)

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

	h <- hub.Message{Message: "Hello from A", Topics: []hub.Topic{"A"}}
	h <- hub.Message{Message: "Hello from B", Topics: []hub.Topic{"B"}}
	h <- hub.Message{Message: "Another message from A", Topics: []hub.Topic{"A"}}
	h <- hub.Message{Message: "Still from A", Topics: []hub.Topic{"A"}}
	h <- hub.Message{Message: "This message is your warning, hub will be closed", Topics: []hub.Topic{"B"}}

	close(h)
	<-done
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
