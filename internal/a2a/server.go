package a2a

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
)

// AgentServer is a minimal A2A HTTP server for the client-side agent.
// It exposes the agent card and can receive incoming tasks.
type AgentServer struct {
	card     AgentCard
	tasksCh  chan Event
	listener net.Listener
	server   *http.Server
}

// NewAgentServer creates an AgentServer on a free port (port 0).
func NewAgentServer(card AgentCard) (*AgentServer, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	card.Port = port

	as := &AgentServer{
		card:    card,
		tasksCh: make(chan Event, 64),
		listener: ln,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/agent.json", as.handleCard)
	mux.HandleFunc("POST /tasks/send", as.handleTask)

	as.server = &http.Server{Handler: mux}
	return as, nil
}

// Card returns the agent card with the assigned port.
func (s *AgentServer) Card() AgentCard {
	return s.card
}

// Port returns the port the server is listening on.
func (s *AgentServer) Port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// Events returns the channel of incoming events (from tasks/send).
func (s *AgentServer) Events() <-chan Event {
	return s.tasksCh
}

// Start begins serving in a goroutine. Returns immediately.
func (s *AgentServer) Start() {
	go func() {
		if err := s.server.Serve(s.listener); err != nil && err != http.ErrServerClosed {
			slog.Error("agent server error", "err", err)
		}
	}()
}

// Close shuts down the agent server.
func (s *AgentServer) Close() error {
	return s.server.Close()
}

func (s *AgentServer) handleCard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.card)
}

func (s *AgentServer) handleTask(w http.ResponseWriter, r *http.Request) {
	var evt Event
	if err := json.NewDecoder(r.Body).Decode(&evt); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	select {
	case s.tasksCh <- evt:
	default:
		slog.Warn("agent task channel full, dropping event")
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintln(w, `{"status":"ok"}`)
}
