package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/liuwenchang/claude-room/internal/a2a"
	"github.com/liuwenchang/claude-room/internal/mcp"
)

// MCPServer serves MCP over stdio, exposing room tools to Claude Code.
type MCPServer struct {
	transport *mcp.Transport
	session   Session
	a2aClient *a2a.Client
}

// NewMCPServer creates an MCPServer reading from r and writing to w.
func NewMCPServer(r io.Reader, w io.Writer) (*MCPServer, error) {
	sess, err := LoadSession()
	if err != nil {
		// No active session; tools will return errors.
		slog.Warn("no active session, tools requiring a room will fail")
	}
	var cli *a2a.Client
	if sess.HubURL != "" {
		cli = a2a.NewClient(sess.HubURL, sess.AuthToken)
	}
	return &MCPServer{
		transport: mcp.NewTransport(r, w),
		session:   sess,
		a2aClient: cli,
	}, nil
}

// Run serves requests until EOF.
func (s *MCPServer) Run(ctx context.Context) error {
	for {
		req, err := s.transport.ReadRequest()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.handle(ctx, req)
	}
}

func (s *MCPServer) handle(ctx context.Context, req *mcp.Request) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "notifications/initialized":
		// No response needed.
	case "notifications/cancelled":
		// No response needed.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(ctx, req)
	default:
		s.transport.ReplyError(req.ID, mcp.MethodNotFound, "method not found: "+req.Method)
	}
}

func (s *MCPServer) handleInitialize(req *mcp.Request) {
	result := mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		ServerInfo: mcp.ServerInfo{
			Name:    "claude-room",
			Version: "0.1.0",
		},
		Capabilities: mcp.Capabilities{
			Tools: &struct{}{},
		},
	}
	s.transport.Reply(req.ID, result)
}

func (s *MCPServer) handleToolsList(req *mcp.Request) {
	tools := []mcp.Tool{
		{
			Name:        "room_list",
			Description: "List all rooms available on the Hub.",
			InputSchema: mcp.InputSchema{Type: "object"},
		},
		{
			Name:        "room_join",
			Description: "Join a room by name.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]mcp.Property{
					"name": {Type: "string", Description: "Room name to join"},
				},
				Required: []string{"name"},
			},
		},
		{
			Name:        "room_leave",
			Description: "Leave the current room.",
			InputSchema: mcp.InputSchema{Type: "object"},
		},
		{
			Name:        "room_members",
			Description: "List all members in the current room.",
			InputSchema: mcp.InputSchema{Type: "object"},
		},
		{
			Name:        "room_send",
			Description: "Send a message to the current room. Pass 'to' (agent_id) for a DM.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]mcp.Property{
					"content": {Type: "string", Description: "Message content (Markdown)"},
					"to":      {Type: "string", Description: "Optional agent_id for DM; omit for broadcast"},
				},
				Required: []string{"content"},
			},
		},
		{
			Name:        "room_recent",
			Description: "Get recent messages from the current room.",
			InputSchema: mcp.InputSchema{
				Type: "object",
				Properties: map[string]mcp.Property{
					"limit": {Type: "number", Description: "Number of messages to return (default 20)"},
				},
			},
		},
		{
			Name:        "room_status",
			Description: "Show current room name, agent ID, and connection status.",
			InputSchema: mcp.InputSchema{Type: "object"},
		},
	}
	s.transport.Reply(req.ID, mcp.ToolsListResult{Tools: tools})
}

func (s *MCPServer) handleToolsCall(ctx context.Context, req *mcp.Request) {
	var params mcp.ToolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.transport.ReplyError(req.ID, mcp.InvalidParams, "invalid params")
		return
	}

	var (
		text string
		isErr bool
	)

	switch params.Name {
	case "room_list":
		text, isErr = s.toolRoomList(ctx)
	case "room_join":
		text, isErr = s.toolRoomJoin(ctx, params.Arguments)
	case "room_leave":
		text, isErr = s.toolRoomLeave(ctx)
	case "room_members":
		text, isErr = s.toolRoomMembers(ctx)
	case "room_send":
		text, isErr = s.toolRoomSend(ctx, params.Arguments)
	case "room_recent":
		text, isErr = s.toolRoomRecent(ctx, params.Arguments)
	case "room_status":
		text, isErr = s.toolRoomStatus()
	default:
		text = "unknown tool: " + params.Name
		isErr = true
	}

	s.transport.Reply(req.ID, mcp.ToolCallResult{
		Content: []mcp.ContentItem{{Type: "text", Text: text}},
		IsError: isErr,
	})
}

func (s *MCPServer) requireSession() error {
	if s.a2aClient == nil || s.session.HubURL == "" {
		return fmt.Errorf("no active session – run `claude-room join <room>` first")
	}
	return nil
}

func (s *MCPServer) toolRoomList(ctx context.Context) (string, bool) {
	if err := s.requireSession(); err != nil {
		return err.Error(), true
	}
	rooms, err := s.a2aClient.ListRooms(ctx)
	if err != nil {
		return fmt.Sprintf("error listing rooms: %v", err), true
	}
	if len(rooms) == 0 {
		return "No rooms available.", false
	}
	return "Rooms:\n- " + strings.Join(rooms, "\n- "), false
}

func (s *MCPServer) toolRoomJoin(ctx context.Context, args map[string]interface{}) (string, bool) {
	name, _ := args["name"].(string)
	if name == "" {
		return "name is required", true
	}
	// Reload config to get hub details.
	cfg, err := LoadConfig()
	if err != nil {
		return fmt.Sprintf("load config: %v", err), true
	}
	cli := a2a.NewClient(cfg.Hub, cfg.AuthToken)

	// Reload or create session with a persistent agent ID.
	sess, _ := LoadSession()
	if sess.AgentID == "" || sess.HubURL != cfg.Hub {
		return "run `claude-room join " + name + "` from the terminal to start a session", true
	}

	if err := cli.JoinRoom(ctx, name, sess.AgentID); err != nil {
		return fmt.Sprintf("join room: %v", err), true
	}
	sess.Room = name
	if err := SaveSession(sess); err != nil {
		slog.Warn("save session", "err", err)
	}
	s.session = sess
	s.a2aClient = cli
	return fmt.Sprintf("Joined room %q as %s.", name, sess.AgentName), false
}

func (s *MCPServer) toolRoomLeave(ctx context.Context) (string, bool) {
	if err := s.requireSession(); err != nil {
		return err.Error(), true
	}
	if s.session.Room == "" {
		return "Not in any room.", false
	}
	if err := s.a2aClient.LeaveRoom(ctx, s.session.Room, s.session.AgentID); err != nil {
		return fmt.Sprintf("leave room: %v", err), true
	}
	room := s.session.Room
	s.session.Room = ""
	SaveSession(s.session)
	return fmt.Sprintf("Left room %q.", room), false
}

func (s *MCPServer) toolRoomMembers(ctx context.Context) (string, bool) {
	if err := s.requireSession(); err != nil {
		return err.Error(), true
	}
	if s.session.Room == "" {
		return "Not in any room.", true
	}
	members, err := s.a2aClient.ListMembers(ctx, s.session.Room)
	if err != nil {
		return fmt.Sprintf("list members: %v", err), true
	}
	if len(members) == 0 {
		return "No members.", false
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Members in %q:\n", s.session.Room))
	for _, m := range members {
		sb.WriteString(fmt.Sprintf("- %s (%s) — %s\n", m.Name, m.HumanUser, m.AgentID))
	}
	return sb.String(), false
}

func (s *MCPServer) toolRoomSend(ctx context.Context, args map[string]interface{}) (string, bool) {
	if err := s.requireSession(); err != nil {
		return err.Error(), true
	}
	if s.session.Room == "" {
		return "Not in any room.", true
	}
	content, _ := args["content"].(string)
	if content == "" {
		return "content is required", true
	}
	toAgent, _ := args["to"].(string)

	req := a2a.SendMessageRequest{
		AgentID: s.session.AgentID,
		Content: content,
	}

	if toAgent != "" {
		id, err := s.a2aClient.SendDM(ctx, s.session.Room, toAgent, req)
		if err != nil {
			return fmt.Sprintf("send DM: %v", err), true
		}
		return fmt.Sprintf("DM sent (event_id: %s).", id), false
	}

	id, err := s.a2aClient.SendBroadcast(ctx, s.session.Room, req)
	if err != nil {
		return fmt.Sprintf("send broadcast: %v", err), true
	}
	return fmt.Sprintf("Message sent to room %q (event_id: %s).", s.session.Room, id), false
}

func (s *MCPServer) toolRoomRecent(ctx context.Context, args map[string]interface{}) (string, bool) {
	if err := s.requireSession(); err != nil {
		return err.Error(), true
	}
	if s.session.Room == "" {
		return "Not in any room.", true
	}
	limit := 20
	if v, ok := args["limit"]; ok {
		switch n := v.(type) {
		case float64:
			limit = int(n)
		case int:
			limit = n
		}
	}
	events, err := s.a2aClient.GetRecentMessages(ctx, s.session.Room, limit)
	if err != nil {
		return fmt.Sprintf("get messages: %v", err), true
	}
	if len(events) == 0 {
		return "No messages yet.", false
	}
	var sb strings.Builder
	for _, evt := range events {
		ts := evt.Timestamp.Format("15:04:05")
		switch evt.Type {
		case a2a.EventBroadcast:
			sb.WriteString(fmt.Sprintf("[%s] %s: %s\n", ts, evt.From, evt.Content))
		case a2a.EventDM:
			sb.WriteString(fmt.Sprintf("[%s] %s → %s (DM): %s\n", ts, evt.From, evt.To, evt.Content))
		case a2a.EventJoin:
			sb.WriteString(fmt.Sprintf("[%s] *** %s\n", ts, evt.Content))
		case a2a.EventLeave:
			sb.WriteString(fmt.Sprintf("[%s] *** %s\n", ts, evt.Content))
		default:
			sb.WriteString(fmt.Sprintf("[%s] %s\n", ts, evt.Content))
		}
	}
	return sb.String(), false
}

func (s *MCPServer) toolRoomStatus() (string, bool) {
	if s.session.HubURL == "" {
		return "No active session. Run `claude-room join <room>` first.", false
	}
	room := s.session.Room
	if room == "" {
		room = "(none)"
	}
	return fmt.Sprintf("Hub:      %s\nAgent ID: %s\nName:     %s\nRoom:     %s",
		s.session.HubURL, s.session.AgentID, s.session.AgentName, room), false
}
