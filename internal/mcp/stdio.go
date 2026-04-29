package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
)

// Transport handles JSON-RPC 2.0 framing over stdin/stdout.
// Each message is a single line of JSON terminated with \n.
type Transport struct {
	scanner *bufio.Scanner
	out     io.Writer
}

// NewTransport wraps r (stdin) and w (stdout).
func NewTransport(r io.Reader, w io.Writer) *Transport {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MiB max message
	return &Transport{scanner: sc, out: w}
}

// ReadRequest blocks until a request line is received.
func (t *Transport) ReadRequest() (*Request, error) {
	for t.scanner.Scan() {
		line := t.scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			slog.Warn("mcp: failed to unmarshal request", "err", err)
			continue
		}
		return &req, nil
	}
	if err := t.scanner.Err(); err != nil {
		return nil, fmt.Errorf("mcp transport read: %w", err)
	}
	return nil, io.EOF
}

// Send serialises v as a single JSON line to stdout.
func (t *Transport) Send(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("mcp transport marshal: %w", err)
	}
	data = append(data, '\n')
	_, err = t.out.Write(data)
	return err
}

// Reply sends a successful JSON-RPC response.
func (t *Transport) Reply(id json.RawMessage, result any) error {
	return t.Send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

// ReplyError sends an error JSON-RPC response.
func (t *Transport) ReplyError(id json.RawMessage, code int, msg string) error {
	return t.Send(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: msg},
	})
}

// Notify sends a JSON-RPC notification (no id).
func (t *Transport) Notify(method string, params any) error {
	return t.Send(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}
