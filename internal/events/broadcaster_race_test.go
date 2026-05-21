package events

import (
	"sync"
	"testing"
)

// TestBroadcast_SendToClosedChannel directly tests the race between Broadcast
// and Unsubscribe that can cause a panic when sending to a closed channel.
func TestBroadcast_SendToClosedChannel(t *testing.T) {
	b := NewBroadcaster()

	// This test runs many iterations to increase the chance of hitting the race.
	for i := 0; i < 500; i++ {
		sessionID := "session-race"
		b.Subscribe(sessionID)

		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: continuously broadcast
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				b.Broadcast("test", map[string]string{"i": "v"})
			}
		}()

		// Goroutine 2: unsubscribe (closes channel)
		go func() {
			defer wg.Done()
			b.Unsubscribe(sessionID)
		}()

		wg.Wait()
	}
}
