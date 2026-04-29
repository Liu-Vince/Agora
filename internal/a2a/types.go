// Package a2a implements the Agent-to-Agent protocol types and communication.
package a2a

import "time"

// AgentCard describes an agent's identity and capabilities.
type AgentCard struct {
	AgentID      string            `json:"agent_id"`
	Name         string            `json:"name"`
	HumanUser    string            `json:"human_user"`
	Host         string            `json:"host"`
	Port         int               `json:"port"`
	Capabilities []string          `json:"capabilities"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// EventType is the type of a room event.
type EventType string

const (
	EventBroadcast EventType = "broadcast"
	EventDM        EventType = "dm"
	EventSystem    EventType = "system"
	EventJoin      EventType = "join"
	EventLeave     EventType = "leave"
)

// CodeSnippet is a code snippet attached to a message.
type CodeSnippet struct {
	Language string `json:"language"`
	Code     string `json:"code"`
}

// MessageContext is optional context attached to a message.
type MessageContext struct {
	FilesReferenced []string      `json:"files_referenced,omitempty"`
	CodeSnippets    []CodeSnippet `json:"code_snippets,omitempty"`
}

// Event is a room message event.
type Event struct {
	EventID   string          `json:"event_id"`
	Room      string          `json:"room"`
	Type      EventType       `json:"type"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	Content   string          `json:"content"`
	Timestamp time.Time       `json:"timestamp"`
	Context   *MessageContext `json:"context,omitempty"`
}

// RegisterRequest is the request body for registering an agent.
type RegisterRequest struct {
	AgentCard AgentCard `json:"agent_card"`
}

// JoinRoomRequest is the request body for joining a room.
type JoinRoomRequest struct {
	AgentID string `json:"agent_id"`
}

// LeaveRoomRequest is the request body for leaving a room.
type LeaveRoomRequest struct {
	AgentID string `json:"agent_id"`
}

// SendMessageRequest is the request body for sending a message.
type SendMessageRequest struct {
	AgentID string          `json:"agent_id"`
	Content string          `json:"content"`
	Context *MessageContext `json:"context,omitempty"`
}
