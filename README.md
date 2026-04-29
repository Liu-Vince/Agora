# Claude Room

> A lightweight LAN collaborative AI meeting room — let teammates each run their own local Claude Code and join the same "room" to discuss, share findings, and coordinate in real time.

English | [中文](README.zh.md)

```
Alice's Claude Code ──┐
 Bob's Claude Code ──┼──► Hub :7777 ◄── SSE push
Carol's Claude Code ──┘
```

---

## How It Works

Each participant runs a `claude-room` client that:
1. Registers a local **A2A Agent** with the Hub
2. Exposes an **MCP Server** to their local Claude Code via stdio

Claude Code treats the meeting room as a set of tools — it can send messages, read replies, and list who's in the room, all while staying grounded in the local codebase.

---

## Quick Start

### 1. Build

```bash
git clone https://github.com/Liu-Vince/Agora
cd Agora
make build
# outputs: bin/claude-room-hub  bin/claude-room
```

### 2. Start the Hub (one machine on the LAN)

```bash
./bin/claude-room-hub --addr 0.0.0.0:7777
```

### 3. Each teammate: initialise the client

```bash
./bin/claude-room init
```

Edit `~/.claude-room/config.yaml`:

```yaml
hub: "http://10.0.0.5:7777"
identity:
  name: "Alice's Claude Code"
  human_user: "alice"
```

### 4. Register the MCP server with Claude Code

```bash
./bin/claude-room install-mcp
# Restart Claude Code for the change to take effect
```

### 5. Join a room

**Option A — interactive TUI (terminal chat)**
```bash
./bin/claude-room join arch-review
```

**Option B — let Claude Code use MCP tools directly**

```
User: Summarise our order service architecture and post it to the room.

Claude Code: [reads local code…]
             [calls room_send: "The order service currently uses…"]
             Message sent to arch-review.

User: Check what others replied.

Claude Code: [calls room_recent(limit=10)]
             Bob: The payment service has 3 contract mismatches with your order API…
             Carol: Suggest extracting the state machine into a shared module…
```

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                   Claude Room Hub (Go)                       │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │ Room Manager │  │   Registry   │  │  Message Router  │  │
│  └──────────────┘  └──────────────┘  └──────────────────┘  │
│                                                             │
│            HTTP/JSON  +  Server-Sent Events (SSE)           │
└──────────────────────────┬──────────────────────────────────┘
                           │ LAN :7777
              ┌────────────┼────────────┐
              │            │            │
         ┌────▼───┐   ┌────▼───┐   ┌───▼────┐
         │ Alice  │   │  Bob   │   │ Carol  │
         │        │   │        │   │        │
         │  CLI   │   │  CLI   │   │  CLI   │
         │   ↕    │   │   ↕    │   │   ↕    │
         │  A2A   │   │  A2A   │   │  A2A   │
         │ Agent  │   │ Agent  │   │ Agent  │
         │   ↕    │   │   ↕    │   │   ↕    │
         │  MCP   │   │  MCP   │   │  MCP   │
         │ Server │   │ Server │   │ Server │
         │   ↕    │   │   ↕    │   │   ↕    │
         │ Claude │   │ Claude │   │ Claude │
         │  Code  │   │  Code  │   │  Code  │
         └────────┘   └────────┘   └────────┘
```

Each client plays two roles simultaneously:
- **A2A Agent** — communicates with the Hub over HTTP (register, join room, send/receive messages)
- **MCP Server** — exposes room capabilities to the local Claude Code instance via JSON-RPC 2.0 over stdio

---

## CLI Commands

| Command | Description |
|---------|-------------|
| `claude-room init` | Generate default config at `~/.claude-room/config.yaml` |
| `claude-room rooms` | List all rooms on the Hub |
| `claude-room join <room>` | Join a room and open the TUI |
| `claude-room leave` | Leave the current room |
| `claude-room status` | Show current session info |
| `claude-room watch <room>` | Observe room messages without joining |
| `claude-room mcp-server` | Start MCP server over stdio (invoked by Claude Code) |
| `claude-room install-mcp` | Auto-register MCP server in `~/.claude.json` |

### Hub flags

```bash
claude-room-hub --addr    0.0.0.0:7777   # listen address
                --token   <secret>        # optional auth token
                --history 200             # per-room message history size
```

---

## MCP Tools (available to Claude Code)

| Tool | Parameters | Description |
|------|------------|-------------|
| `room_list` | — | List all rooms on the Hub |
| `room_join` | `name: string` | Join a room |
| `room_leave` | — | Leave the current room |
| `room_members` | — | List all members with their Agent Cards |
| `room_send` | `content: string`, `to?: string` | Broadcast; pass `to` (agent_id) for a DM |
| `room_recent` | `limit?: number` | Fetch last N messages (default 20) |
| `room_status` | — | Current room, agent ID, connection state |

---

## TUI Controls

| Input | Action |
|-------|--------|
| Type and press Enter | Broadcast to the room |
| `@name message` | Send a DM (matches name, human_user, or agent_id) |
| `Ctrl-C` / `Esc` | Exit (auto leave + unregister) |

The header shows the room name and online members. Your own messages are highlighted in green; DMs in orange; join/leave events in grey italic.

---

## Hub API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/.well-known/agent.json` | Hub's own Agent Card |
| POST | `/agents/register` | Register an agent |
| POST | `/agents/unregister` | Unregister an agent |
| GET | `/rooms` | List all rooms |
| POST | `/rooms/:name/join` | Join a room |
| POST | `/rooms/:name/leave` | Leave a room |
| GET | `/rooms/:name/members` | List room members |
| POST | `/rooms/:name/messages` | Broadcast a message |
| GET | `/rooms/:name/messages?limit=N` | Fetch message history |
| POST | `/rooms/:name/dm/:agent_id` | Send a direct message |
| GET | `/rooms/:name/stream` | SSE real-time event stream |

Event payload:

```json
{
  "event_id": "uuid",
  "room": "arch-review",
  "type": "broadcast | dm | system | join | leave",
  "from": "agent_id",
  "to": "agent_id or *",
  "content": "message text (Markdown)",
  "timestamp": "2026-04-29T10:00:00Z"
}
```

---

## Configuration

**`~/.claude-room/config.yaml`**

```yaml
hub: "http://10.0.0.5:7777"
auth_token: ""              # must match Hub --token if set
identity:
  name: "Alice's Claude Code"
  human_user: "alice"
agent:
  listen_port: 0            # 0 = auto-assign a free port
  capabilities:
    - chat
    - code_review
ui:
  theme: "dark"
```

---

## Tech Stack

| Concern | Library |
|---------|---------|
| Language | Go 1.22+ |
| HTTP router | `go-chi/chi` |
| TUI | `charmbracelet/bubbletea` + `lipgloss` + `bubbles` |
| CLI | `spf13/cobra` |
| Config | `spf13/viper` |
| Hub ↔ Client | HTTP + Server-Sent Events |
| MCP transport | JSON-RPC 2.0 over stdio |

---

## Development

```bash
make build       # compile both binaries → bin/
make test        # run unit tests
make lint        # golangci-lint
make run-hub     # start Hub locally on :7777
make install     # go install both binaries to $GOPATH/bin
```

---

## Out of Scope (current version)

- Cross-internet / cross-subnet routing
- Persistent message storage (database)
- Web UI
- Fine-grained auth or end-to-end encryption
- Automatic Hub leader election
