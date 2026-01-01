package tui

import (
	"encoding/json"
	"fluffy-palm-tree/encryption"
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
	Content   []byte `json:"content"`
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
	client        *SocketClient
	user          User
	viewport      viewport.Model
	messages      []ChatMsg
	showDecrypted bool
	textarea      textarea.Model
	senderStyle   lipgloss.Style
	err           error
}

// custom text style
var (
	systemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true) // user connected/disconnected
	secureStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)     // encrypted
	lockedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)   // decrypted
)

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

	switch envelope.Type {
	case "MSG":
		var event ChatMsg // send message to tui
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	case "USER": // just for connect and disconnect for now
		var event UserMsg
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

	// create a user
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
		messages:    []ChatMsg{},
		viewport:    vp,
		senderStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:         nil,
	}
}

func (m ChatModel) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, func() tea.Msg {
		userEvent := UserMsg{
			Type:      "USER",
			Status:    "connected",
			UserID:    m.user.UID,
			Timestamp: time.Now().Unix(),
		}
		payload, _ := json.Marshal(userEvent)
		m.client.SendChan <- payload
		return nil
	})
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
	case tea.WindowSizeMsg:
		headerHeight := 1
		footerHeight := m.textarea.Height() + lipgloss.Height(gap) + 1
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight
		m.refreshViewport()
		m.viewport.GotoBottom()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlX:
			m.showDecrypted = !m.showDecrypted
			m.refreshViewport()
			return m, nil
		case tea.KeyCtrlC, tea.KeyEsc:
			// event for this is sent from the server side since write pump shuts down on tea.quit
			return m, tea.Quit
		case tea.KeyEnter:
			input := m.textarea.Value()
			if strings.TrimSpace(input) == "" {
				return m, nil
			}
			// encrypt message before sending
			inputBytes, _ := json.Marshal(input)
			encryptedInput, err := encryption.EncryptData(inputBytes, encryption.MasterKey)
			if err != nil {
				m.err = err
				return m, nil
			}

			event := ChatMsg{
				Type:      "MSG",
				Content:   encryptedInput,
				UserID:    m.user.UID,
				Timestamp: time.Now().Unix(),
			}

			payload, _ := json.Marshal(event)
			// select statement here prevents send from blocking. if the channel is full the default case exits the select, effectively killing this process silently
			select {
			case m.client.SendChan <- payload:
			default:
			}

			m.textarea.Reset()
		}

	case ChatMsg:
		m.messages = append(m.messages, msg)
		m.refreshViewport()
		m.viewport.GotoBottom()

	case UserMsg:
		sysMsg := ChatMsg{
			Type:    "USER",
			UserID:  msg.UserID,
			Content: []byte(fmt.Sprintf("User %s has %s", msg.UserID, msg.Status)),
		}

		m.messages = append(m.messages, sysMsg)
		m.refreshViewport()
		m.viewport.GotoBottom()
	case ErrMsg:
		m.err = msg.Err
		return m, nil
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m *ChatModel) renderMessage(msg ChatMsg, width int) string {
	var label string
	if msg.UserID == m.user.UID {
		label = m.senderStyle.Render("You: ")
	} else {
		label = fmt.Sprintf("%s: ", msg.UserID)
	}

	contentWidth := width - lipgloss.Width(label) - 1

	var text string
	if !m.showDecrypted {
		//text = fmt.Sprintf("[%x...]", msg.Content[:min(10, len(msg.Content))])
		text = fmt.Sprintf("%x", msg.Content)
	} else {
		decryptedBytes, err := encryption.DecryptData(msg.Content, encryption.MasterKey)
		if err != nil {
			text = "[Decryption Error]"
		} else {
			var originalText string
			json.Unmarshal(decryptedBytes, &originalText)
			text = originalText
		}
	}

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("242")).
		Width(contentWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, label, contentStyle.Render(text))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *ChatModel) refreshViewport() {
	var rendered []string

	wrapWidth := m.viewport.Width - 2
	if wrapWidth < 1 {
		wrapWidth = 1
	}

	for _, msg := range m.messages {
		line := m.renderMessage(msg, wrapWidth)
		rendered = append(rendered, line)
	}

	m.viewport.SetContent(strings.Join(rendered, "\n"))
}

// display the result
func (m ChatModel) View() string {
	status := secureStyle.Render("● ENCRYPTED")
	if m.showDecrypted {
		status = lockedStyle.Render("○ UNLOCKED (Ctrl+X)")
	}

	return fmt.Sprintf(
		"%s\n%s\n%s%s%s",
		m.viewport.View(),
		status,
		gap,
		m.textarea.View(),
		"\n (Ctrl+C to quit | Ctrl+X to toggle)",
	)
}
