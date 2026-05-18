package events

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// Broadcaster manages SSE clients and broadcasts platform events.
type Broadcaster struct {
	mu      sync.RWMutex
	clients map[string]chan string // sessionID -> event channel
}

func NewBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients: make(map[string]chan string),
	}
}

// Subscribe registers a new SSE client and returns its event channel.
func (b *Broadcaster) Subscribe(sessionID string) chan string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan string, 64)
	b.clients[sessionID] = ch
	slog.Info("SSE client subscribed", "session", sessionID, "clients", len(b.clients))
	return ch
}

// Unsubscribe removes an SSE client.
func (b *Broadcaster) Unsubscribe(sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch, ok := b.clients[sessionID]; ok {
		close(ch)
		delete(b.clients, sessionID)
	}
	slog.Info("SSE client unsubscribed", "session", sessionID, "clients", len(b.clients))
}

// Broadcast sends an event to all connected clients.
func (b *Broadcaster) Broadcast(eventType string, data interface{}) {
	payload, err := json.Marshal(map[string]interface{}{
		"type":      eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      data,
	})
	if err != nil {
		slog.Error("Failed to marshal event", "error", err)
		return
	}

	b.mu.RLock()
	clients := make(map[string]chan string, len(b.clients))
	for id, ch := range b.clients {
		clients[id] = ch
	}
	b.mu.RUnlock()

	msg := fmt.Sprintf("data: %s\n\n", string(payload))
	for id, ch := range clients {
		select {
		case ch <- msg:
		default:
			slog.Warn("SSE client channel full, dropping event", "session", id)
		}
	}
}

func (b *Broadcaster) AgentStatus(name, status, agentType string) {
	b.Broadcast("agent.status_change", map[string]string{
		"name":   name,
		"status": status,
		"type":   agentType,
	})
}

func (b *Broadcaster) AgentRegistered(name, status, agentType string) {
	b.Broadcast("agent.registered", map[string]string{
		"name":   name,
		"status": status,
		"type":   agentType,
	})
}

func (b *Broadcaster) Task(eventType, taskId, agentName, state string) {
	b.Broadcast("task."+eventType, map[string]string{
		"task_id":    taskId,
		"agent_name": agentName,
		"state":      state,
	})
}

func (b *Broadcaster) Trace(taskId, eventType, agentName string) {
	b.Broadcast("trace.append", map[string]string{
		"task_id":    taskId,
		"event_type": eventType,
		"agent_name": agentName,
	})
}
