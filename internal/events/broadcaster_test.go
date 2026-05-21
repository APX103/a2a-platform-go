package events

import (
	"sync"
	"testing"
	"time"
)

// TestBroadcast_RaceCondition verifies that Broadcast does not panic
// when Unsubscribe is called concurrently.
func TestBroadcast_RaceCondition(t *testing.T) {
	b := NewBroadcaster()

	// Run many iterations to try to trigger the race
	for i := 0; i < 100; i++ {
		sessionID := "session-" + string(rune('a'+i%26))
		ch := b.Subscribe(sessionID)

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: broadcast events
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				b.Broadcast("test.event", map[string]string{"data": "value"})
			}
		}()

		// Goroutine 2: unsubscribe (closes channel)
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)
			b.Unsubscribe(sessionID)
		}()

		// Drain the channel to avoid blocking
		go func(c <-chan string) {
			for range c {
			}
		}(ch)

		wg.Wait()
	}
}

// TestBroadcast_MultipleClients verifies broadcasting to multiple clients.
func TestBroadcast_MultipleClients(t *testing.T) {
	b := NewBroadcaster()

	ch1 := b.Subscribe("client-1")
	ch2 := b.Subscribe("client-2")

	b.Broadcast("test", map[string]string{"msg": "hello"})

	select {
	case msg := <-ch1:
		if msg == "" {
			t.Error("client-1 received empty message")
		}
	case <-time.After(time.Second):
		t.Error("client-1 did not receive message")
	}

	select {
	case msg := <-ch2:
		if msg == "" {
			t.Error("client-2 received empty message")
		}
	case <-time.After(time.Second):
		t.Error("client-2 did not receive message")
	}

	b.Unsubscribe("client-1")
	b.Unsubscribe("client-2")
}

// TestUnsubscribe_Idempotent verifies that unsubscribing the same session twice does not panic.
func TestUnsubscribe_Idempotent(t *testing.T) {
	b := NewBroadcaster()

	sessionID := "session-x"
	b.Subscribe(sessionID)
	b.Unsubscribe(sessionID)
	b.Unsubscribe(sessionID) // should not panic
}
