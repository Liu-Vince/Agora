// Package client implements the local A2A agent, MCP server, and CLI session logic.
package client

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

// Config holds client-side configuration.
type Config struct {
	Hub       string   `mapstructure:"hub"`
	AuthToken string   `mapstructure:"auth_token"`
	Identity  Identity `mapstructure:"identity"`
	Agent     Agent    `mapstructure:"agent"`
	UI        UI       `mapstructure:"ui"`
}

// Identity is the user's identity configuration.
type Identity struct {
	Name      string `mapstructure:"name"`
	HumanUser string `mapstructure:"human_user"`
}

// Agent is the local agent configuration.
type Agent struct {
	ListenPort   int      `mapstructure:"listen_port"`
	Capabilities []string `mapstructure:"capabilities"`
}

// UI is the TUI configuration.
type UI struct {
	Theme string `mapstructure:"theme"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Hub:       "http://localhost:7777",
		AuthToken: "",
		Identity: Identity{
			Name:      "My Claude Code",
			HumanUser: os.Getenv("USER"),
		},
		Agent: Agent{
			ListenPort:   0,
			Capabilities: []string{"chat", "code_review"},
		},
		UI: UI{Theme: "dark"},
	}
}

// ConfigDir returns ~/.claude-room.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".claude-room"), nil
}

// ConfigPath returns the path to the config file.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// LoadConfig loads config from the default location.
func LoadConfig() (Config, error) {
	dir, err := ConfigDir()
	if err != nil {
		return Config{}, err
	}

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(dir)
	v.AutomaticEnv()

	// Defaults
	def := DefaultConfig()
	v.SetDefault("hub", def.Hub)
	v.SetDefault("auth_token", def.AuthToken)
	v.SetDefault("identity.name", def.Identity.Name)
	v.SetDefault("identity.human_user", def.Identity.HumanUser)
	v.SetDefault("agent.listen_port", def.Agent.ListenPort)
	v.SetDefault("agent.capabilities", def.Agent.Capabilities)
	v.SetDefault("ui.theme", def.UI.Theme)

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return Config{}, fmt.Errorf("read config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}

// WriteDefaultConfig writes a default config YAML to ~/.claude-room/config.yaml.
func WriteDefaultConfig() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("config already exists at %s", path)
	}

	content := `hub: "http://localhost:7777"
auth_token: ""
identity:
  name: "My Claude Code"
  human_user: ""
agent:
  listen_port: 0
  capabilities:
    - chat
    - code_review
ui:
  theme: "dark"
`
	return os.WriteFile(path, []byte(content), 0o644)
}
