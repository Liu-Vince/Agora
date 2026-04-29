package hub

import (
	"sync"

	"github.com/liuwenchang/claude-room/internal/a2a"
)

// Registry manages all registered agents.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]a2a.AgentCard
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{agents: make(map[string]a2a.AgentCard)}
}

// Register stores or updates an agent card.
func (r *Registry) Register(card a2a.AgentCard) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[card.AgentID] = card
}

// Unregister removes an agent.
func (r *Registry) Unregister(agentID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.agents, agentID)
}

// Get returns the agent card for the given ID.
func (r *Registry) Get(agentID string) (a2a.AgentCard, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	card, ok := r.agents[agentID]
	return card, ok
}

// List returns all registered agent cards.
func (r *Registry) List() []a2a.AgentCard {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cards := make([]a2a.AgentCard, 0, len(r.agents))
	for _, c := range r.agents {
		cards = append(cards, c)
	}
	return cards
}
