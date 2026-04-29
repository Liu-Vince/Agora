// Command claude-room is the Claude Room client CLI.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/liuwenchang/claude-room/internal/a2a"
	"github.com/liuwenchang/claude-room/internal/client"
)

var rootCmd = &cobra.Command{
	Use:   "claude-room",
	Short: "Collaborative AI meeting room for Claude Code",
}

func main() {
	rootCmd.AddCommand(
		initCmd(),
		configCmd(),
		roomsCmd(),
		joinCmd(),
		leaveCmd(),
		watchCmd(),
		statusCmd(),
		mcpServerCmd(),
		installMCPCmd(),
	)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// --- init ---

func initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Generate default config at ~/.claude-room/config.yaml",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := client.WriteDefaultConfig(); err != nil {
				return err
			}
			path, _ := client.ConfigPath()
			fmt.Println("Config written to", path)
			return nil
		},
	}
}

// --- config ---

func configCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage configuration"}
	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("To set %q = %q, edit ~/.claude-room/config.yaml directly.\n", args[0], args[1])
			return nil
		},
	})
	return cmd
}

// --- rooms ---

func roomsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rooms",
		Short: "List all rooms on the Hub",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := client.LoadConfig()
			if err != nil {
				return err
			}
			cli := a2a.NewClient(cfg.Hub, cfg.AuthToken)
			rooms, err := cli.ListRooms(context.Background())
			if err != nil {
				return fmt.Errorf("list rooms: %w", err)
			}
			if len(rooms) == 0 {
				fmt.Println("No rooms.")
				return nil
			}
			for _, r := range rooms {
				fmt.Println(" •", r)
			}
			return nil
		},
	}
}

// --- join ---

func joinCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "join <room>",
		Short: "Join a room and open the TUI",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomName := args[0]
			cfg, err := client.LoadConfig()
			if err != nil {
				return err
			}

			// Build agent card.
			agentID := uuid.NewString()
			hostname, _ := os.Hostname()
			host := localIP()
			if host == "" {
				host = hostname
			}

			agentCard := a2a.AgentCard{
				AgentID:      agentID,
				Name:         cfg.Identity.Name,
				HumanUser:    cfg.Identity.HumanUser,
				Host:         host,
				Capabilities: cfg.Agent.Capabilities,
				Metadata: map[string]string{
					"cwd": cwd(),
				},
			}

			// Start local A2A agent server.
			agentSrv, err := a2a.NewAgentServer(agentCard)
			if err != nil {
				return fmt.Errorf("start agent server: %w", err)
			}
			agentCard.Port = agentSrv.Port()
			agentSrv.Start()
			defer agentSrv.Close()

			// Register + join the room.
			hubCli := a2a.NewClient(cfg.Hub, cfg.AuthToken)
			ctx := context.Background()

			if err := hubCli.RegisterAgent(ctx, agentCard); err != nil {
				return fmt.Errorf("register agent: %w", err)
			}
			if err := hubCli.JoinRoom(ctx, roomName, agentID); err != nil {
				return fmt.Errorf("join room: %w", err)
			}

			// Persist session.
			sess := client.Session{
				HubURL:    cfg.Hub,
				AuthToken: cfg.AuthToken,
				AgentID:   agentID,
				AgentName: cfg.Identity.Name,
				Room:      roomName,
				AgentPort: agentSrv.Port(),
			}
			if err := client.SaveSession(sess); err != nil {
				slog.Warn("save session", "err", err)
			}

			// Graceful cleanup on exit.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			go func() {
				<-sigCh
				leaveAndCleanup(cfg, agentID, roomName)
				os.Exit(0)
			}()

			// Start TUI.
			model := client.NewTUIModel(sess, hubCli)
			p := tea.NewProgram(model, tea.WithAltScreen())
			if _, err := p.Run(); err != nil {
				return err
			}

			leaveAndCleanup(cfg, agentID, roomName)
			return nil
		},
	}
}

func leaveAndCleanup(cfg client.Config, agentID, room string) {
	cli := a2a.NewClient(cfg.Hub, cfg.AuthToken)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli.LeaveRoom(ctx, room, agentID)
	cli.UnregisterAgent(ctx, agentID)
	client.ClearSession()
}

// --- leave ---

func leaveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "leave",
		Short: "Leave the current room",
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := client.LoadSession()
			if err != nil {
				return fmt.Errorf("no active session: %w", err)
			}
			cli := a2a.NewClient(sess.HubURL, sess.AuthToken)
			ctx := context.Background()
			if err := cli.LeaveRoom(ctx, sess.Room, sess.AgentID); err != nil {
				return fmt.Errorf("leave room: %w", err)
			}
			cli.UnregisterAgent(ctx, sess.AgentID)
			client.ClearSession()
			fmt.Println("Left room", sess.Room)
			return nil
		},
	}
}

// --- watch ---

func watchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch <room>",
		Short: "Observe room messages without joining",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roomName := args[0]
			cfg, err := client.LoadConfig()
			if err != nil {
				return err
			}
			cli := a2a.NewClient(cfg.Hub, cfg.AuthToken)
			resp, err := cli.StreamEvents(context.Background(), roomName, "watch-"+uuid.NewString()[:8])
			if err != nil {
				return fmt.Errorf("stream: %w", err)
			}
			defer resp.Body.Close()

			fmt.Printf("Watching room %q (Ctrl-C to stop)…\n", roomName)
			dec := json.NewDecoder(resp.Body)
			// Read SSE line by line manually.
			buf := make([]byte, 4096)
			for {
				n, err := resp.Body.Read(buf)
				if n > 0 {
					fmt.Print(string(buf[:n]))
				}
				if err != nil {
					break
				}
			}
			_ = dec
			return nil
		},
	}
}

// --- status ---

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current session status",
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := client.LoadSession()
			if err != nil {
				fmt.Println("No active session.")
				return nil
			}
			fmt.Printf("Hub:       %s\nAgent ID:  %s\nName:      %s\nRoom:      %s\n",
				sess.HubURL, sess.AgentID, sess.AgentName, sess.Room)
			return nil
		},
	}
}

// --- mcp-server ---

func mcpServerCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "mcp-server",
		Short:  "Start MCP server (stdio) for Claude Code",
		Hidden: false,
		RunE: func(cmd *cobra.Command, args []string) error {
			srv, err := client.NewMCPServer(os.Stdin, os.Stdout)
			if err != nil {
				return err
			}
			return srv.Run(context.Background())
		},
	}
}

// --- install-mcp ---

func installMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install-mcp",
		Short: "Register claude-room as an MCP server in Claude Code",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installMCP()
		},
	}
}

func installMCP() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	// Try ~/.claude.json first, fall back to ~/.claude/config.json.
	paths := []string{
		home + "/.claude.json",
		home + "/.claude/claude.json",
	}

	var target string
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			target = p
			break
		}
	}
	if target == "" {
		target = paths[0]
	}

	// Backup.
	if _, err := os.Stat(target); err == nil {
		backup := target + ".bak." + time.Now().Format("20060102150405")
		data, _ := os.ReadFile(target)
		if err := os.WriteFile(backup, data, 0o644); err != nil {
			return fmt.Errorf("backup: %w", err)
		}
		fmt.Println("Backed up", target, "to", backup)
	}

	// Read existing JSON or start fresh.
	var cfg map[string]interface{}
	if data, err := os.ReadFile(target); err == nil {
		json.Unmarshal(data, &cfg)
	}
	if cfg == nil {
		cfg = make(map[string]interface{})
	}

	servers, _ := cfg["mcpServers"].(map[string]interface{})
	if servers == nil {
		servers = make(map[string]interface{})
	}
	servers["claude-room"] = map[string]interface{}{
		"command": "claude-room",
		"args":    []string{"mcp-server"},
	}
	cfg["mcpServers"] = servers

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	fmt.Println("MCP server registered in", target)
	fmt.Println("Restart Claude Code for the change to take effect.")
	return nil
}

// --- helpers ---

func cwd() string {
	d, _ := os.Getwd()
	return d
}

func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}
