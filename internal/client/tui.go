package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/liuwenchang/claude-room/internal/a2a"
)

// --- styles ---

var (
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	memberStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8A8A8")).
			Padding(0, 1)

	inputBorderStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#7D56F4")).
				Padding(0, 1)

	timeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555"))
	nameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4"))
	dmStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#F4A256"))
	sysStyle  = lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#888888"))
	selfStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#56F4B0"))
)

// --- tea messages ---

type (
	eventMsg       a2a.Event
	memberMsg      []a2a.AgentCard
	errMsg         error
	sseConnected   struct{}
)

// eventCh carries SSE events from the background reader into the tea loop.
var eventCh = make(chan a2a.Event, 128)

// TUIModel is the bubbletea model for the chat room.
type TUIModel struct {
	room      string
	agentID   string
	agentName string
	client    *a2a.Client

	viewport viewport.Model
	input    textinput.Model
	members  []a2a.AgentCard
	lines    []string

	width  int
	height int
	ready  bool
	errStr string
}

// NewTUIModel creates the TUI model.
func NewTUIModel(sess Session, cli *a2a.Client) TUIModel {
	ti := textinput.New()
	ti.Placeholder = "Type a message… (@name msg for DM)"
	ti.Focus()
	ti.CharLimit = 2000

	return TUIModel{
		room:      sess.Room,
		agentID:   sess.AgentID,
		agentName: sess.AgentName,
		client:    cli,
		input:     ti,
	}
}

// Init implements tea.Model.
func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		m.fetchMembers(),
		m.connectSSE(),
	)
}

// Update implements tea.Model.
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerH := 2
		inputH := 3
		vpH := m.height - headerH - inputH - 1
		if vpH < 1 {
			vpH = 1
		}
		if !m.ready {
			m.viewport = viewport.New(m.width, vpH)
			m.ready = true
		} else {
			m.viewport.Width = m.width
			m.viewport.Height = vpH
		}
		m.input.Width = m.width - 6

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if cmd := m.sendMessage(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			cmds = append(cmds, cmd)
		}

	case sseConnected:
		cmds = append(cmds, m.waitEvent())

	case eventMsg:
		m.appendEvent(a2a.Event(msg))
		cmds = append(cmds, m.waitEvent())

	case memberMsg:
		m.members = []a2a.AgentCard(msg)
		cmds = append(cmds, m.scheduleMemberPoll())

	case errMsg:
		m.errStr = msg.Error()
	}

	var vpCmd tea.Cmd
	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)
	return m, tea.Batch(cmds...)
}

func (m *TUIModel) appendEvent(evt a2a.Event) {
	ts := timeStyle.Render(evt.Timestamp.Format("15:04"))
	var line string
	switch evt.Type {
	case a2a.EventBroadcast:
		ns := nameStyle
		if evt.From == m.agentID {
			ns = selfStyle
		}
		line = fmt.Sprintf("%s %s: %s", ts, ns.Render(m.nameFor(evt.From)), evt.Content)
	case a2a.EventDM:
		from := dmStyle.Render(m.nameFor(evt.From))
		to := dmStyle.Render(m.nameFor(evt.To))
		line = fmt.Sprintf("%s %s → %s: %s", ts, from, to, evt.Content)
	default:
		line = fmt.Sprintf("%s %s", ts, sysStyle.Render("✦ "+evt.Content))
	}
	m.lines = append(m.lines, line)
	m.viewport.SetContent(strings.Join(m.lines, "\n"))
	m.viewport.GotoBottom()
}

func (m *TUIModel) nameFor(agentID string) string {
	for _, mem := range m.members {
		if mem.AgentID == agentID {
			return mem.Name
		}
	}
	if agentID == m.agentID {
		return m.agentName
	}
	return agentID
}

// --- commands ---

func (m TUIModel) sendMessage() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	m.input.SetValue("")

	room := m.room
	agentID := m.agentID
	cli := m.client
	members := m.members

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		req := a2a.SendMessageRequest{AgentID: agentID, Content: text}

		if strings.HasPrefix(text, "@") {
			parts := strings.SplitN(text, " ", 2)
			if len(parts) == 2 {
				target := strings.TrimPrefix(parts[0], "@")
				req.Content = parts[1]
				toID := resolveAgentID(target, members)
				if toID != "" {
					if _, err := cli.SendDM(ctx, room, toID, req); err != nil {
						return errMsg(err)
					}
					return nil
				}
			}
		}

		if _, err := cli.SendBroadcast(ctx, room, req); err != nil {
			return errMsg(err)
		}
		return nil
	}
}

func resolveAgentID(nameOrID string, members []a2a.AgentCard) string {
	for _, m := range members {
		if m.AgentID == nameOrID ||
			strings.EqualFold(m.Name, nameOrID) ||
			strings.EqualFold(m.HumanUser, nameOrID) {
			return m.AgentID
		}
	}
	return ""
}

func (m TUIModel) connectSSE() tea.Cmd {
	room := m.room
	agentID := m.agentID
	cli := m.client
	return func() tea.Msg {
		resp, err := cli.StreamEvents(context.Background(), room, agentID)
		if err != nil {
			return errMsg(err)
		}
		go func() {
			defer resp.Body.Close()
			sc := bufio.NewScanner(resp.Body)
			for sc.Scan() {
				line := sc.Text()
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				var evt a2a.Event
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &evt); err == nil {
					eventCh <- evt
				}
			}
			if err := sc.Err(); err != nil && err != io.EOF {
				slog.Warn("SSE read error", "err", err)
			}
		}()
		return sseConnected{}
	}
}

func (m TUIModel) waitEvent() tea.Cmd {
	return func() tea.Msg {
		return eventMsg(<-eventCh)
	}
}

func (m TUIModel) fetchMembers() tea.Cmd {
	room := m.room
	cli := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		members, err := cli.ListMembers(ctx, room)
		if err != nil {
			return errMsg(err)
		}
		return memberMsg(members)
	}
}

func (m TUIModel) scheduleMemberPoll() tea.Cmd {
	return tea.Tick(10*time.Second, func(_ time.Time) tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		members, err := m.client.ListMembers(ctx, m.room)
		if err != nil {
			return errMsg(err)
		}
		return memberMsg(members)
	})
}

// View implements tea.Model.
func (m TUIModel) View() string {
	if !m.ready {
		return "Connecting to " + m.room + "…\n"
	}

	memberNames := make([]string, 0, len(m.members))
	for _, mem := range m.members {
		if mem.AgentID == m.agentID {
			memberNames = append(memberNames, selfStyle.Render(mem.Name+" (you)"))
		} else {
			memberNames = append(memberNames, nameStyle.Render(mem.Name))
		}
	}

	header := lipgloss.JoinHorizontal(lipgloss.Left,
		headerStyle.Render("  # "+m.room+"  "),
		memberStyle.Render(" "+strings.Join(memberNames, "  ")+" "),
	)

	var bottom string
	if m.errStr != "" {
		errView := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("⚠ " + m.errStr)
		bottom = lipgloss.JoinVertical(lipgloss.Left, errView, inputBorderStyle.Render(m.input.View()))
	} else {
		bottom = inputBorderStyle.Render(m.input.View())
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		header,
		m.viewport.View(),
		bottom,
	)
}
