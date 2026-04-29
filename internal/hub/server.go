package hub

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/liuwenchang/claude-room/internal/a2a"
)

// Config holds Hub server configuration.
type Config struct {
	ListenAddr  string
	AuthToken   string
	HistorySize int
}

// Server is the Hub HTTP server.
type Server struct {
	cfg       Config
	mux       *chi.Mux
	registry  *Registry
	msgRouter *Router

	roomsMu sync.RWMutex
	rooms   map[string]*Room
}

// NewServer creates a Hub server with the given config.
func NewServer(cfg Config) *Server {
	if cfg.HistorySize <= 0 {
		cfg.HistorySize = defaultHistorySize
	}
	s := &Server{
		cfg:       cfg,
		registry:  NewRegistry(),
		msgRouter: NewRouter(),
		rooms:     make(map[string]*Room),
	}
	s.buildRoutes()
	return s
}

func (s *Server) buildRoutes() {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	if s.cfg.AuthToken != "" {
		r.Use(s.authMiddleware)
	}

	r.Get("/.well-known/agent.json", s.handleHubCard)
	r.Post("/agents/register", s.handleRegister)
	r.Post("/agents/unregister", s.handleUnregister)

	r.Get("/rooms", s.handleListRooms)
	r.Post("/rooms/{room}/join", s.handleJoinRoom)
	r.Post("/rooms/{room}/leave", s.handleLeaveRoom)
	r.Get("/rooms/{room}/members", s.handleListMembers)
	r.Post("/rooms/{room}/messages", s.handleBroadcast)
	r.Get("/rooms/{room}/messages", s.handleGetMessages)
	r.Post("/rooms/{room}/dm/{agentID}", s.handleDM)
	r.Get("/rooms/{room}/stream", s.handleStream)

	s.mux = r
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+s.cfg.AuthToken {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) getOrCreateRoom(name string) *Room {
	s.roomsMu.Lock()
	defer s.roomsMu.Unlock()
	if room, ok := s.rooms[name]; ok {
		return room
	}
	room := newRoom(name, s.cfg.HistorySize)
	s.rooms[name] = room
	return room
}

// --- handlers ---

func (s *Server) handleHubCard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a2a.AgentCard{
		AgentID:      "hub",
		Name:         "Claude Room Hub",
		Capabilities: []string{"room_management", "message_routing"},
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req a2a.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AgentCard.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id required")
		return
	}
	s.registry.Register(req.AgentCard)
	slog.Info("agent registered", "agent_id", req.AgentCard.AgentID, "name", req.AgentCard.Name)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleUnregister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	s.registry.Unregister(req.AgentID)
	slog.Info("agent unregistered", "agent_id", req.AgentID)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleListRooms(w http.ResponseWriter, r *http.Request) {
	s.roomsMu.RLock()
	names := make([]string, 0, len(s.rooms))
	for name := range s.rooms {
		names = append(names, name)
	}
	s.roomsMu.RUnlock()
	writeJSON(w, map[string][]string{"rooms": names})
}

func (s *Server) handleJoinRoom(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	var req a2a.JoinRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id required")
		return
	}

	room := s.getOrCreateRoom(roomName)
	room.AddMember(req.AgentID)

	card, _ := s.registry.Get(req.AgentID)
	name := card.Name
	if name == "" {
		name = req.AgentID
	}
	evt := a2a.Event{
		EventID:   uuid.NewString(),
		Room:      roomName,
		Type:      a2a.EventJoin,
		From:      req.AgentID,
		To:        "*",
		Content:   name + " joined the room",
		Timestamp: time.Now(),
	}
	room.AppendEvent(evt)
	s.msgRouter.Broadcast(roomName, evt)

	slog.Info("agent joined room", "agent_id", req.AgentID, "room", roomName)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLeaveRoom(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	var req a2a.LeaveRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	s.roomsMu.RLock()
	room, ok := s.rooms[roomName]
	s.roomsMu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	room.RemoveMember(req.AgentID)
	card, _ := s.registry.Get(req.AgentID)
	name := card.Name
	if name == "" {
		name = req.AgentID
	}
	evt := a2a.Event{
		EventID:   uuid.NewString(),
		Room:      roomName,
		Type:      a2a.EventLeave,
		From:      req.AgentID,
		To:        "*",
		Content:   name + " left the room",
		Timestamp: time.Now(),
	}
	room.AppendEvent(evt)
	s.msgRouter.Broadcast(roomName, evt)

	slog.Info("agent left room", "agent_id", req.AgentID, "room", roomName)
	writeJSON(w, map[string]string{"status": "ok"})
}

func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	s.roomsMu.RLock()
	room, ok := s.rooms[roomName]
	s.roomsMu.RUnlock()

	if !ok {
		writeJSON(w, map[string][]a2a.AgentCard{"members": {}})
		return
	}

	memberIDs := room.Members()
	members := make([]a2a.AgentCard, 0, len(memberIDs))
	for _, id := range memberIDs {
		if card, ok := s.registry.Get(id); ok {
			members = append(members, card)
		}
	}
	writeJSON(w, map[string][]a2a.AgentCard{"members": members})
}

func (s *Server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	var req a2a.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	s.roomsMu.RLock()
	room, ok := s.rooms[roomName]
	s.roomsMu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	evt := a2a.Event{
		EventID:   uuid.NewString(),
		Room:      roomName,
		Type:      a2a.EventBroadcast,
		From:      req.AgentID,
		To:        "*",
		Content:   req.Content,
		Timestamp: time.Now(),
		Context:   req.Context,
	}
	room.AppendEvent(evt)
	s.msgRouter.Broadcast(roomName, evt)
	writeJSON(w, map[string]string{"event_id": evt.EventID})
}

func (s *Server) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	s.roomsMu.RLock()
	room, ok := s.rooms[roomName]
	s.roomsMu.RUnlock()
	if !ok {
		writeJSON(w, map[string][]a2a.Event{"events": {}})
		return
	}
	writeJSON(w, map[string][]a2a.Event{"events": room.RecentEvents(limit)})
}

func (s *Server) handleDM(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	toAgentID := chi.URLParam(r, "agentID")
	var req a2a.SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}

	s.roomsMu.RLock()
	room, ok := s.rooms[roomName]
	s.roomsMu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	evt := a2a.Event{
		EventID:   uuid.NewString(),
		Room:      roomName,
		Type:      a2a.EventDM,
		From:      req.AgentID,
		To:        toAgentID,
		Content:   req.Content,
		Timestamp: time.Now(),
		Context:   req.Context,
	}
	room.AppendEvent(evt)
	s.msgRouter.Broadcast(roomName, evt)
	writeJSON(w, map[string]string{"event_id": evt.EventID})
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	roomName := chi.URLParam(r, "room")
	agentID := r.URL.Query().Get("agent_id")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Push recent history immediately on connect.
	s.roomsMu.RLock()
	room, exists := s.rooms[roomName]
	s.roomsMu.RUnlock()
	if exists {
		for _, evt := range room.RecentEvents(s.cfg.HistorySize) {
			if data, err := FormatSSE(evt); err == nil {
				w.Write(data)
			}
		}
		flusher.Flush()
	}

	sub := s.msgRouter.Subscribe(roomName, agentID)
	defer s.msgRouter.Unsubscribe(roomName, sub)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case evt := <-sub.ch:
			data, err := FormatSSE(evt)
			if err != nil {
				continue
			}
			if _, err := w.Write(data); err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			if _, err := w.Write(KeepAlive()); err != nil {
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
