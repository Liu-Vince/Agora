# Claude Room — 局域网 AI 会议室

> 让局域网内的同事各自跑本地 Claude Code，一键加入同一个"会议室"，多个 Claude Code 实例（背后是不同的人 + 各自的代码上下文）可以互相对话、协同讨论、共享发现。

## 项目目标

构建一个基于 A2A（Agent2Agent）协议的轻量协作系统，实现：

1. **Hub 服务**：内网部署的中心节点，管理房间、Agent 发现、消息路由
2. **Client CLI**：同事本地装的 Go 命令行工具，启动本地 A2A Agent + 暴露 MCP 工具给 Claude Code
3. **一键体验**：`claude-room join <房间名>` 即可让本机 Claude Code 进入会议室

## 核心使用场景

```
小张电脑：claude-room join arch-review        # 加入"架构评审"房间
小李电脑：claude-room join arch-review        # 加入同一房间
小王电脑：claude-room join arch-review

# 三人各自的 Claude Code 现在都能：
#   - 看到房间里有哪些其他 agent（小张的 / 小李的 / 小王的 Claude Code）
#   - 向房间发消息（广播）或私聊某个 agent
#   - 接收其他 agent 发来的消息
#   - 让自己机器上的 Claude Code 看到讨论上下文，给出基于本地代码的回应
```

例如，小张让自己的 Claude Code 说："把我们订单服务的当前架构总结一下，发到房间里"，
小李的 Claude Code 收到后，可以基于自己本地的支付服务代码补充："我这边的支付服务对接是这样的……"。

## 技术架构

```
┌─────────────────────────────────────────────────────────────┐
│                  Claude Room Hub (Go)                       │
│              内网部署，单实例即可，无状态可选                 │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │ Room Manager │  │ Agent        │  │ Message Router   │  │
│  │              │  │ Registry     │  │ (broadcast/DM)   │  │
│  └──────────────┘  └──────────────┘  └──────────────────┘  │
│                                                             │
│  HTTP/JSON-RPC (A2A 协议)  +  WebSocket (实时推送)          │
└────────────────┬────────────────────────────────────────────┘
                 │ 局域网 (默认端口 7777)
       ┌─────────┼─────────┬─────────────┐
       │         │         │             │
   ┌───▼───┐ ┌───▼───┐ ┌───▼───┐     ┌───▼───┐
   │小张PC │ │小李PC │ │小王PC │ ... │ 其他人 │
   │       │ │       │ │       │     │       │
   │┌─────┐│ │┌─────┐│ │┌─────┐│     │       │
   ││ CLI ││ ││ CLI ││ ││ CLI ││     │       │
   │└──┬──┘│ │└──┬──┘│ │└──┬──┘│     │       │
   │   │   │ │   │   │ │   │   │     │       │
   │┌──▼──┐│ │┌──▼──┐│ │┌──▼──┐│     │       │
   ││ A2A ││ ││ A2A ││ ││ A2A ││     │       │
   ││Agent││ ││Agent││ ││Agent││     │       │
   │└──┬──┘│ │└──┬──┘│ │└──┬──┘│     │       │
   │   │   │ │   │   │ │   │   │     │       │
   │┌──▼──┐│ │┌──▼──┐│ │┌──▼──┐│     │       │
   ││ MCP ││ ││ MCP ││ ││ MCP ││     │       │
   ││Srv  ││ ││Srv  ││ ││Srv  ││     │       │
   │└──┬──┘│ │└──┬──┘│ │└──┬──┘│     │       │
   │   │   │ │   │   │ │   │   │     │       │
   │┌──▼──┐│ │┌──▼──┐│ │┌──▼──┐│     │       │
   ││Cld  ││ ││Cld  ││ ││Cld  ││     │       │
   ││Code ││ ││Code ││ ││Code ││     │       │
   │└─────┘│ │└─────┘│ │└─────┘│     │       │
   └───────┘ └───────┘ └───────┘     └───────┘
```

**关键设计**：每个同事的本地 CLI 同时扮演两个角色：
- **A2A Agent**：通过 A2A 协议跟 Hub 和其他 Agent 通讯
- **MCP Server**：通过 MCP 协议把"发消息/读消息/列出成员"等能力暴露给本机的 Claude Code

这样 Claude Code 就把会议室体验当作一组"工具"来使用。

## 技术选型

- **语言**：Go 1.22+（Hub 和 Client 都用 Go，单一二进制好分发）
- **协议**：
  - Agent ↔ Hub：HTTP + Server-Sent Events（A2A 标准）
  - Hub ↔ Hub 内部消息推送：channel
  - Client ↔ Claude Code：MCP over stdio（标准）
- **数据存储**：Hub 内存即可（房间和成员），重启后大家重新加入；如需持久化用 SQLite
- **配置**：YAML 配置文件 + 环境变量
- **A2A 协议参考**：https://a2a-protocol.org/latest/

## 项目结构

```
claude-room/
├── README.md
├── go.mod
├── go.sum
├── Makefile
├── cmd/
│   ├── hub/
│   │   └── main.go              # Hub 服务入口
│   └── claude-room/
│       └── main.go              # Client CLI 入口
├── internal/
│   ├── a2a/                     # A2A 协议实现（client + server 共用）
│   │   ├── types.go             # AgentCard, Message, Task 等
│   │   ├── client.go            # A2A 客户端
│   │   └── server.go            # A2A 服务端
│   ├── hub/
│   │   ├── server.go            # HTTP 服务器
│   │   ├── room.go              # 房间管理
│   │   ├── registry.go          # Agent 注册表
│   │   └── router.go            # 消息路由（广播 + 私聊）
│   ├── client/
│   │   ├── agent.go             # 本地 A2A Agent
│   │   ├── mcp_server.go        # MCP Server（暴露给 Claude Code）
│   │   ├── room_session.go      # 房间会话状态
│   │   └── config.go            # 客户端配置
│   └── mcp/
│       ├── protocol.go          # MCP 协议类型
│       └── stdio.go             # stdio transport
├── examples/
│   ├── hub-config.yaml
│   └── client-config.yaml
└── docs/
    ├── ARCHITECTURE.md
    └── PROTOCOL.md              # 自定义消息协议补充说明
```

## 详细需求

### 1. Hub 服务（cmd/hub）

**启动方式**：
```bash
claude-room-hub --config hub-config.yaml
# 或
claude-room-hub --port 7777
```

**对外提供的 HTTP API**（A2A 协议风格 + 房间扩展）：

| Method | Path | 说明 |
|--------|------|------|
| GET    | `/.well-known/agent.json`     | Hub 自身的 Agent Card |
| POST   | `/agents/register`            | Agent 注册（提交自己的 Agent Card + 监听地址） |
| POST   | `/agents/unregister`          | Agent 离开 |
| GET    | `/rooms`                      | 列出所有房间 |
| POST   | `/rooms/:name/join`           | 加入房间 |
| POST   | `/rooms/:name/leave`          | 离开房间 |
| GET    | `/rooms/:name/members`        | 列出房间成员（Agent Cards） |
| POST   | `/rooms/:name/messages`       | 向房间发广播消息 |
| POST   | `/rooms/:name/dm/:agent_id`   | 在房间内向某个 Agent 发私信 |
| GET    | `/rooms/:name/stream`         | SSE，实时推送房间内的消息事件 |

**Agent Card 结构**（参考 A2A 标准）：
```json
{
  "agent_id": "uuid",
  "name": "Frank's Claude Code",
  "human_user": "frank",
  "host": "192.168.1.42",
  "port": 18080,
  "capabilities": ["chat", "code_review", "file_read"],
  "metadata": {
    "project": "banking-miniprogram",
    "git_branch": "feature/live-stream",
    "cwd": "/Users/frank/code/banking"
  }
}
```

**消息事件结构**：
```json
{
  "event_id": "uuid",
  "room": "arch-review",
  "type": "broadcast | dm | system | join | leave",
  "from": "agent_id",
  "to": "agent_id 或 *",
  "content": "消息文本（Markdown）",
  "timestamp": "2026-04-29T10:00:00Z",
  "context": {                    // 可选，发送者附带的上下文
    "files_referenced": [...],
    "code_snippets": [...]
  }
}
```

**关键行为**：
- Agent 通过 SSE 长连接订阅房间消息，断线自动从房间剔除（带 30 秒宽限期，便于网络抖动恢复）
- 房间内消息默认保留最近 200 条在内存里，新加入者能拉到最近上下文
- 不需要鉴权（局域网信任环境），但要支持配置一个简单 token（防止有人误连）
- 全部日志结构化输出（zap 或 slog）

### 2. Client CLI（cmd/claude-room）

**核心命令**：
```bash
# 首次配置
claude-room init                               # 生成默认配置 ~/.claude-room/config.yaml
claude-room config set hub http://10.0.0.5:7777
claude-room config set name "Frank"

# 房间操作
claude-room rooms                              # 列出 Hub 上的所有房间
claude-room create <房间名>                     # 创建房间（其实加入即创建）
claude-room join <房间名>                       # 加入房间，进入交互式 TUI
claude-room leave                              # 离开当前房间

# Claude Code 集成
claude-room mcp-server                         # 启动 MCP Server（stdio），由 Claude Code 调用
claude-room install-mcp                        # 自动把自己注册到 ~/.claude.json 里

# 调试
claude-room status                             # 查看本机 agent 状态、当前房间、连接状况
claude-room watch <房间名>                      # 不参与，只观察房间消息（debug 用）
```

**`claude-room join` 应该做的事**：
1. 读配置，找到 Hub 地址
2. 启动本地 A2A Agent（监听本机一个空闲端口）
3. 向 Hub 注册自己的 Agent Card
4. 加入指定房间
5. 进入交互式 TUI，显示：
   - 顶部：房间名 + 在线成员列表
   - 中部：消息流（区分广播/私信/系统消息）
   - 底部：输入框（直接打字就是广播；`@frank xxx` 就是私聊）
6. 同时确保本机的 MCP Server 在跑，让 Claude Code 也能"看见"这个房间

**`claude-room install-mcp` 应该做的事**：
检查并修改 `~/.claude.json`（或 `~/.claude/config.json`，按当前 Claude Code 版本约定），追加：
```json
{
  "mcpServers": {
    "claude-room": {
      "command": "claude-room",
      "args": ["mcp-server"]
    }
  }
}
```
执行前先备份原文件，提示用户确认。

### 3. MCP Server（暴露给本机 Claude Code 的工具）

启动方式：`claude-room mcp-server`（由 Claude Code 自动拉起，stdio 通讯）

**暴露的 MCP Tools**：

| Tool 名称 | 入参 | 说明 |
|-----------|------|------|
| `room_list` | 无 | 列出 Hub 上所有房间 |
| `room_join` | `name: string` | 加入房间 |
| `room_leave` | 无 | 离开当前房间 |
| `room_members` | 无 | 当前房间的所有成员（含每个人的 Agent Card） |
| `room_send` | `content: string`, `to?: string` | 发消息（不传 to 就是广播） |
| `room_recent` | `limit?: number` | 拉取当前房间最近 N 条消息 |
| `room_subscribe` | 无 | 订阅当前房间消息流（返回 stream） |
| `room_status` | 无 | 当前房间名 + 我的 agent_id + 在线状态 |

**这意味着**：Claude Code 可以这样使用——

```
用户：把我们当前订单服务的设计总结一下，发到房间里让大家看看

Claude Code（在本地）：[读取本地 order-service 代码并总结] 
                      [调用 mcp tool: room_send(content="...", to=null)]
                      已发送到房间 arch-review。

用户：看看大家的回复

Claude Code：[调用 mcp tool: room_recent(limit=10)]
            小李说：支付服务这边的接口和你的订单服务有3处契约不一致……
            小王说：建议把状态机抽出来作为公共模块……
```

### 4. A2A 协议实现要点

只需实现 A2A 协议中**任务消息传递**这一最小子集：

- Agent Card 发布（`/.well-known/agent.json`）
- `tasks/send`：向另一个 Agent 发送 task（用于 Agent 之间私聊）
- `tasks/sendSubscribe`：流式版本（SSE）
- `tasks/get`：查询 task 状态

不需要实现完整 A2A 状态机（不需要 `submitted/working/completed` 那一整套），消息发送即送达即可。

参考：
- https://a2a-protocol.org/latest/
- https://github.com/google/A2A（官方 spec 仓库）

### 5. MCP 协议实现要点

实现 MCP over stdio 的最小子集：
- `initialize` 握手
- `tools/list`
- `tools/call`
- `notifications/cancelled`

stdio 协议：JSON-RPC 2.0，每条消息一行，标准输入读、标准输出写。

参考：https://modelcontextprotocol.io/specification

### 6. 配置文件示例

**hub-config.yaml**：
```yaml
listen: "0.0.0.0:7777"
auth_token: ""              # 留空 = 无鉴权
log_level: "info"
room:
  max_members: 50
  history_size: 200
  idle_timeout_seconds: 1800
```

**~/.claude-room/config.yaml**：
```yaml
hub: "http://10.0.0.5:7777"
auth_token: ""
identity:
  name: "Frank"
  human_user: "frank"
agent:
  listen_port: 0            # 0 = 自动找空闲端口
  capabilities:
    - chat
    - code_review
ui:
  theme: "dark"
```

## 实现路线（按顺序，每一步都可独立验证）

### Phase 1：A2A 协议基础（先打通最小通讯）
- [ ] 定义 Agent Card 数据结构
- [ ] 实现 A2A HTTP server（监听 + 路由）
- [ ] 实现 A2A HTTP client（注册 + 发消息）
- [ ] 写单测：两个 Agent 直接发消息

### Phase 2：Hub 服务
- [ ] Hub 启动 + Agent 注册/注销
- [ ] 房间增删改查
- [ ] 消息广播（房间内）
- [ ] SSE 实时推送
- [ ] 集成测试：3 个 Agent 加入同一房间，互相收发广播

### Phase 3：Client CLI 基础
- [ ] `init` / `config` 命令
- [ ] `rooms` / `join` / `leave` 命令
- [ ] 简易 TUI（用 bubbletea，必须好看）
- [ ] 一个端到端 demo：3 个终端窗口模拟 3 人开会

### Phase 4：MCP Server 集成
- [ ] MCP stdio 协议实现
- [ ] 8 个 tools 实现
- [ ] `install-mcp` 命令（自动写 Claude Code 配置）
- [ ] 端到端 demo：用真实 Claude Code 接入会议室，让它发消息

### Phase 5：抛光
- [ ] 错误处理（断线重连、Hub 重启、并发）
- [ ] 日志和可观测性
- [ ] README + ARCHITECTURE 文档
- [ ] 一键安装脚本（curl | bash）
- [ ] 演示视频/GIF

## 验收标准

完整 Demo 场景：

1. 在一台机器（A）上启动 Hub：`claude-room-hub --port 7777`
2. 在三台机器（B/C/D）上各装一个 Client，配置好 Hub 地址
3. 每台机器上启动 Claude Code，它已经通过 MCP 接入了 claude-room
4. B 机器：`claude-room join demo`（开 TUI）
5. 在 B 的 Claude Code 中说："加入 demo 房间，并发一条消息说我开始评审订单服务"
   → Claude Code 调用 MCP tool 完成；B 的 TUI 同步显示该消息
6. C 和 D 同样在自己的 Claude Code 里加入 demo 房间
7. C 的 Claude Code："看一下房间里都有谁，给小张回个话说我也来评审支付服务"
   → C 收到房间历史，发回复；B 和 D 都实时看到
8. D 的 Claude Code："总结一下当前讨论"
   → D 拉取房间最近消息，给出总结

如果上述 8 步无缝完成，项目就成了。

## 技术约束

- Go 代码遵守 `gofmt`、`go vet`、`golangci-lint` 默认规则
- 所有公开 API 加 godoc 注释
- 关键路径有单测，集成测试用 testcontainers 或本地多进程
- 错误处理用 wrapped errors（`fmt.Errorf("...: %w", err)`）
- 日志用 `log/slog`
- HTTP 用标准库 `net/http` + `chi` 路由器（可选）
- TUI 用 `github.com/charmbracelet/bubbletea` + `lipgloss`
- CLI 用 `github.com/spf13/cobra`
- 配置用 `github.com/spf13/viper`
- 不要引入重型依赖（不要 gRPC，不要 protobuf，不要 etcd）

## 非目标（这一版不做）

- 跨网段、跨公网（只考虑局域网）
- 持久化历史消息到数据库
- Web UI（一切走 TUI + Claude Code）
- 鉴权和细粒度权限（局域网信任）
- 端到端加密
- Agent 能力的语义化匹配（先简单列表）
- 自动选举 Hub leader（单 Hub 即可）
- 跨房间消息

## 给实现者的提示

1. **从 A2A 类型定义开始**：先把数据结构确定下来，后面所有事情都顺
2. **Hub 先用纯内存实现**：等功能跑通再考虑持久化
3. **MCP Server 是关键卖点**：必须让 Claude Code 用起来"无感"，调用一个 tool 就完成事情
4. **TUI 要好看**：这是 demo 的脸面，用 bubbletea 做出聊天室的感觉
5. **多写集成测试**：3 个 agent 同时跑的并发场景容易出 bug
6. **每个 Phase 结束都要能 demo**：方便迭代和给同事看进度

---

请按照上述规格实现这个项目。先列出你的实现计划和疑问，然后从 Phase 1 开始，每完成一个 Phase 就跟我确认一次再继续下一个。
