package hub

import (
	"sync"

	"github.com/liuwenchang/claude-room/internal/a2a"
)

const defaultHistorySize = 200

// Room represents a chat room with members and message history.
type Room struct {
	Name       string
	mu         sync.RWMutex
	members    map[string]struct{}
	history    []a2a.Event
	maxHistory int
}

func newRoom(name string, maxHistory int) *Room {
	if maxHistory <= 0 {
		maxHistory = defaultHistorySize
	}
	return &Room{
		Name:       name,
		members:    make(map[string]struct{}),
		maxHistory: maxHistory,
	}
}

// AddMember adds an agent to the room.
func (r *Room) AddMember(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.members[agentID] = struct{}{}
}

// RemoveMember removes an agent from the room.
func (r *Room) RemoveMember(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.members, agentID)
}

// HasMember reports whether agentID is in the room.
func (r *Room) HasMember(agentID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.members[agentID]
	return ok
}

// Members returns all member agent IDs.
func (r *Room) Members() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.members))
	for id := range r.members {
		ids = append(ids, id)
	}
	return ids
}

// AppendEvent adds an event to history, dropping the oldest if over capacity.
func (r *Room) AppendEvent(evt a2a.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history, evt)
	if len(r.history) > r.maxHistory {
		r.history = r.history[len(r.history)-r.maxHistory:]
	}
}

// RecentEvents returns up to limit events from the end of history.
func (r *Room) RecentEvents(limit int) []a2a.Event {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := len(r.history)
	if limit <= 0 || limit > n {
		limit = n
	}
	out := make([]a2a.Event, limit)
	copy(out, r.history[n-limit:])
	return out
}

// MemberCount returns the number of current members.
func (r *Room) MemberCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.members)
}
