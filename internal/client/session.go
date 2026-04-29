package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Session holds the runtime state for the current room session.
// It is persisted to disk so the MCP server can read it.
type Session struct {
	HubURL    string `json:"hub_url"`
	AuthToken string `json:"auth_token"`
	AgentID   string `json:"agent_id"`
	AgentName string `json:"agent_name"`
	Room      string `json:"room"`
	AgentPort int    `json:"agent_port"`
}

// SessionPath returns the path to the session state file.
func SessionPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

// SaveSession writes the session to disk.
func SaveSession(s Session) error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	path := filepath.Join(dir, "session.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

// LoadSession reads the session from disk.
func LoadSession() (Session, error) {
	path, err := SessionPath()
	if err != nil {
		return Session{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Session{}, fmt.Errorf("read session: %w", err)
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return Session{}, fmt.Errorf("unmarshal session: %w", err)
	}
	return s, nil
}

// ClearSession removes the session file.
func ClearSession() error {
	path, err := SessionPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
