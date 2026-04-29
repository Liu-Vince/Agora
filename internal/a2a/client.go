package a2a

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client is an A2A HTTP client for communicating with the Hub.
type Client struct {
	hubURL     string
	authToken  string
	httpClient *http.Client
}

// NewClient creates a new A2A client pointed at hubURL.
func NewClient(hubURL, authToken string) *Client {
	return &Client{
		hubURL:    hubURL,
		authToken: authToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *Client) doJSON(ctx context.Context, method, path string, body, result any) error {
	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, c.hubURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		return fmt.Errorf("server error %d: %s", resp.StatusCode, errResp.Error)
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// RegisterAgent registers an agent card with the Hub.
func (c *Client) RegisterAgent(ctx context.Context, card AgentCard) error {
	return c.doJSON(ctx, http.MethodPost, "/agents/register", RegisterRequest{AgentCard: card}, nil)
}

// UnregisterAgent removes an agent from the Hub.
func (c *Client) UnregisterAgent(ctx context.Context, agentID string) error {
	return c.doJSON(ctx, http.MethodPost, "/agents/unregister", map[string]string{"agent_id": agentID}, nil)
}

// ListRooms returns all room names on the Hub.
func (c *Client) ListRooms(ctx context.Context) ([]string, error) {
	var result struct {
		Rooms []string `json:"rooms"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/rooms", nil, &result); err != nil {
		return nil, err
	}
	return result.Rooms, nil
}

// JoinRoom joins a named room.
func (c *Client) JoinRoom(ctx context.Context, room, agentID string) error {
	return c.doJSON(ctx, http.MethodPost, "/rooms/"+room+"/join", JoinRoomRequest{AgentID: agentID}, nil)
}

// LeaveRoom leaves a named room.
func (c *Client) LeaveRoom(ctx context.Context, room, agentID string) error {
	return c.doJSON(ctx, http.MethodPost, "/rooms/"+room+"/leave", LeaveRoomRequest{AgentID: agentID}, nil)
}

// ListMembers returns all member AgentCards in a room.
func (c *Client) ListMembers(ctx context.Context, room string) ([]AgentCard, error) {
	var result struct {
		Members []AgentCard `json:"members"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/rooms/"+room+"/members", nil, &result); err != nil {
		return nil, err
	}
	return result.Members, nil
}

// SendBroadcast sends a broadcast message to a room.
func (c *Client) SendBroadcast(ctx context.Context, room string, req SendMessageRequest) (string, error) {
	var result struct {
		EventID string `json:"event_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/rooms/"+room+"/messages", req, &result); err != nil {
		return "", err
	}
	return result.EventID, nil
}

// SendDM sends a direct message to an agent inside a room.
func (c *Client) SendDM(ctx context.Context, room, toAgentID string, req SendMessageRequest) (string, error) {
	var result struct {
		EventID string `json:"event_id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/rooms/"+room+"/dm/"+toAgentID, req, &result); err != nil {
		return "", err
	}
	return result.EventID, nil
}

// GetRecentMessages returns the last limit messages from a room.
func (c *Client) GetRecentMessages(ctx context.Context, room string, limit int) ([]Event, error) {
	path := fmt.Sprintf("/rooms/%s/messages?limit=%d", room, limit)
	var result struct {
		Events []Event `json:"events"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &result); err != nil {
		return nil, err
	}
	return result.Events, nil
}

// StreamEvents opens an SSE connection and returns the raw HTTP response.
// The caller is responsible for closing the response body.
func (c *Client) StreamEvents(ctx context.Context, room, agentID string) (*http.Response, error) {
	url := fmt.Sprintf("%s/rooms/%s/stream?agent_id=%s", c.hubURL, room, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create stream request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := (&http.Client{}).Do(req) // No timeout for SSE
	if err != nil {
		return nil, fmt.Errorf("connect to stream: %w", err)
	}
	return resp, nil
}
