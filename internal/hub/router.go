package hub

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/liuwenchang/claude-room/internal/a2a"
)

// Subscriber is a single SSE connection subscribed to a room.
type Subscriber struct {
	agentID string
	ch      chan a2a.Event
}

// Router manages SSE subscriptions and broadcasts events.
type Router struct {
	mu          sync.RWMutex
	subscribers map[string][]*Subscriber // room -> subscribers
}

// NewRouter creates an empty Router.
func NewRouter() *Router {
	return &Router{subscribers: make(map[string][]*Subscriber)}
}

// Subscribe registers a subscriber for room and returns it.
func (r *Router) Subscribe(room, agentID string) *Subscriber {
	sub := &Subscriber{
		agentID: agentID,
		ch:      make(chan a2a.Event, 64),
	}
	r.mu.Lock()
	r.subscribers[room] = append(r.subscribers[room], sub)
	r.mu.Unlock()
	return sub
}

// Unsubscribe removes sub from the room's subscriber list.
func (r *Router) Unsubscribe(room string, sub *Subscriber) {
	r.mu.Lock()
	defer r.mu.Unlock()
	subs := r.subscribers[room]
	for i, s := range subs {
		if s == sub {
			r.subscribers[room] = append(subs[:i], subs[i+1:]...)
			return
		}
	}
}

// Broadcast sends evt to every subscriber in room.
func (r *Router) Broadcast(room string, evt a2a.Event) {
	r.mu.RLock()
	subs := make([]*Subscriber, len(r.subscribers[room]))
	copy(subs, r.subscribers[room])
	r.mu.RUnlock()

	for _, sub := range subs {
		select {
		case sub.ch <- evt:
		default:
			slog.Warn("subscriber channel full, dropping event", "agent_id", sub.agentID)
		}
	}
}

// FormatSSE serialises evt as a Server-Sent Events data frame.
func FormatSSE(evt a2a.Event) ([]byte, error) {
	data, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("data: %s\n\n", data)), nil
}

// KeepAlive returns an SSE comment used to keep the connection alive.
func KeepAlive() []byte {
	return []byte(": ping " + time.Now().Format(time.RFC3339) + "\n\n")
}
