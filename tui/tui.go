package tui

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

type SocketClient struct {
	Conn     *websocket.Conn
	SendChan chan []byte
}
type WebSocketEvent struct {
	Type string `json:"type"`
}

type ErrMsg struct {
	Type string `json:"type"`
	Err  error  `json:"err"`
}

type ChatMsg struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
}

type User struct {
	UID       string
	Status    string
	Timestamp int64
}

type UserMsg struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
}

type AckEvent struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

const gap = "\n\n"

type ChatModel struct {
	client      *SocketClient
	user        User
	viewport    viewport.Model
	messages    []string
	textarea    textarea.Model
	senderStyle lipgloss.Style
	err         error
}

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func generateUserID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

func (c *SocketClient) WritePump() {
	defer c.Conn.Close()
	for {
		message, ok := <-c.SendChan
		if !ok {
			return
		}
		err := c.Conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			return
		}
	}
}

func ListenForMessages(c *SocketClient, p *tea.Program) {
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			p.Send(ErrMsg{Type: "error", Err: err})
			return
		}
		err = handleIncomingMessage(p, message)
		if err != nil {
			p.Send(ErrMsg{Type: "error", Err: err})
			continue
		}
	}
}

func handleIncomingMessage(p *tea.Program, data []byte) error {
	var envelope WebSocketEvent
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}

	// Switch based on the Type and unmarshal into the final struct
	switch envelope.Type {
	case "MSG":
		var event ChatMsg
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	case "USER":
		var event UserMsg
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	case "CHAT":
		var event ChatMsg
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	case "ACK":
		var event AckEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	default:
		return fmt.Errorf("unknown event type: %s", envelope.Type)
	}
}

// the state
func InitialChatModel(c *SocketClient) ChatModel {

	u := User{
		UID:       generateUserID(),
		Status:    "connected",
		Timestamp: time.Now().Unix(),
	}

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Focus()

	ta.Prompt = "┃ "
	ta.CharLimit = 280

	ta.SetWidth(30)
	ta.SetHeight(3)

	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	ta.ShowLineNumbers = false

	vp := viewport.New(30, 5)
	vp.SetContent(fmt.Sprintf("Welcome! Your ID is: %s\nType a message and press Enter.", u.UID))

	ta.KeyMap.InsertNewline.SetEnabled(false)

	return ChatModel{
		client:      c,
		user:        u,
		textarea:    ta,
		messages:    []string{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
	}
}

func (m ChatModel) Init() tea.Cmd {
	return textarea.Blink
}

// change the state
func (m ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		tiCmd tea.Cmd
		vpCmd tea.Cmd
	)

	m.textarea, tiCmd = m.textarea.Update(msg)
	m.viewport, vpCmd = m.viewport.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			input := m.textarea.Value()
			if strings.TrimSpace(input) == "" {
				return m, nil
			}

			event := ChatMsg{
				Type:      "MSG",
				Content:   input,
				UserID:    m.user.UID,
				Timestamp: time.Now().Unix(),
			}

			payload, _ := json.Marshal(event)
			select {
			case m.client.SendChan <- payload:
			default:
			}

			m.textarea.Reset()
		}

	case ChatMsg:
		var label string
		if msg.UserID == m.user.UID {
			label = m.senderStyle.Render("You: ")
		} else {
			label = fmt.Sprintf("%s: ", msg.UserID)
		}

		m.messages = append(m.messages, label+msg.Content)
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()

	case UserMsg:
		systemStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
		m.messages = append(m.messages, systemStyle.Render(fmt.Sprintf("** User %s %s **", msg.UserID, msg.Status)))
		m.viewport.SetContent(lipgloss.NewStyle().Width(m.viewport.Width).Render(strings.Join(m.messages, "\n")))
		m.viewport.GotoBottom()

	case ErrMsg:
		m.err = msg.Err
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

// display the result
func (m ChatModel) View() string {
	return fmt.Sprintf(
		"%s%s%s",
		m.viewport.View(),
		gap,
		m.textarea.View(),
	)
}
