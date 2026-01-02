package tui

import (
	"encoding/json"
	"fluffy-palm-tree/encryption"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
	"github.com/schollz/pake/v3"
	"golang.org/x/crypto/argon2"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
const gap = "\n\n"

var (
	systemStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Italic(true)
	secureStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	lockedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
)

func generateUserID() string {
	b := make([]byte, 8)
	for i := range b {
		b[i] = charset[rand.IntN(len(charset))]
	}
	return string(b)
}

func (m *ChatModel) SetProgram(p *tea.Program) {
	m.program = p
}

func (m *ChatModel) runHandshake(password string, roomID string) tea.Cmd {
	return func() tea.Msg {
		headers := http.Header{}
		headers.Add("X-Room-Password", password)

		u := url.URL{Scheme: "ws", Host: "localhost:8080", Path: "/ws/" + roomID}
		conn, _, err := websocket.DefaultDialer.Dial(u.String(), headers)
		if err != nil {
			return ErrMsg{Err: err}
		}
		m.client.Conn = conn

		go m.client.WritePump()

		p, err := pake.InitCurve([]byte(password), 1, "p256")
		if err != nil {
			return ErrMsg{Err: err}
		}

		msgType, serverPoint, err := m.client.Conn.ReadMessage()
		if err != nil {
			return ErrMsg{Err: err}
		}

		if msgType != websocket.BinaryMessage {
			return ErrMsg{Err: fmt.Errorf("expected binary handshake, got %d", msgType)}
		}

		if err := p.Update(serverPoint); err != nil {
			return ErrMsg{Err: fmt.Errorf("pake update failed: %w", err)}
		}

		m.client.Conn.WriteMessage(websocket.BinaryMessage, p.Bytes())

		_, err = p.SessionKey()
		if err != nil {
			return ErrMsg{Err: err}
		}

		fixedSalt := []byte(encryption.SaltMaster)
		roomKey := argon2.IDKey([]byte(password), fixedSalt, 1, 64*1024, 4, 32)

		return HandshakeCompleteMsg{Key: roomKey}
	}
}

func (m *ChatModel) startRefreshing() tea.Cmd {
	return func() tea.Msg {
		go ListenForMessages(m.client, m.program, m)
		return nil
	}
}

func ListenForMessages(c *SocketClient, p *tea.Program, m *ChatModel) {
	if p == nil {
		return
	}
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

func handleIncomingMessage(p *tea.Program, data []byte) error {
	var envelope WebSocketEvent
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}

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
	default:
		return fmt.Errorf("unknown event type: %s", envelope.Type)
	}
}

func InitialChatModel(c *SocketClient, p *tea.Program) ChatModel {
	u := User{
		UID:       generateUserID(),
		Status:    "connected",
		Timestamp: time.Now().Unix(),
	}

	ri := textinput.New()
	ri.Placeholder = "Room Name (e.g. dev-chat)"
	ri.Focus()
	ri.CharLimit = 20

	ti := textinput.New()
	ti.Placeholder = "Shared Password"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'

	ta := textarea.New()
	ta.Placeholder = "Send a message..."
	ta.Prompt = "┃ "
	ta.CharLimit = 280
	ta.SetWidth(30)
	ta.SetHeight(3)
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.ShowLineNumbers = false
	ta.KeyMap.InsertNewline.SetEnabled(false)

	vp := viewport.New(30, 5)

	return ChatModel{
		state:         stateLogin,
		roomInput:     ri,
		passwordInput: ti,
		client:        c,
		program:       p,
		user:          u,
		textarea:      ta,
		messages:      []ChatMsg{},
		viewport:      vp,
		senderStyle:   lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:           nil,
	}
}

func (m *ChatModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m *ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(ErrMsg); ok {
		m.err = msg.Err
		return m, nil
	}

	switch m.state {
	case stateLogin:
		return m.updateLogin(msg)
	case stateChat:
		return m.updateChat(msg)
	}
	return m, nil
}

func (m *ChatModel) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	var (
		cmd1 tea.Cmd
		cmd2 tea.Cmd
	)

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		headerHeight := 1
		footerHeight := m.textarea.Height() + lipgloss.Height(gap) + 1

		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - headerHeight - footerHeight

		m.textarea.SetWidth(msg.Width)

		m.refreshViewport()
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyTab, tea.KeyUp, tea.KeyDown:
			if m.roomInput.Focused() {
				m.roomInput.Blur()
				m.passwordInput.Focus()
			} else {
				m.passwordInput.Blur()
				m.roomInput.Focus()
			}
		case tea.KeyEnter:
			if m.passwordInput.Value() != "" && m.roomInput.Value() != "" {
				return m, m.runHandshake(m.passwordInput.Value(), m.roomInput.Value())
			}
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		}

	case HandshakeCompleteMsg:
		m.sessionKey = msg.Key
		m.state = stateChat
		m.textarea.Focus()
		return m, tea.Batch(
			m.sendPresence(),
			m.startRefreshing(),
		)
	}

	m.roomInput, cmd1 = m.roomInput.Update(msg)
	m.passwordInput, cmd2 = m.passwordInput.Update(msg)
	return m, tea.Batch(cmd1, cmd2)
}

func (m *ChatModel) sendPresence() tea.Cmd {
	return func() tea.Msg {
		userEvent := UserMsg{
			Type: "USER", Status: "connected",
			UserID: m.user.UID, Timestamp: time.Now().Unix(),
		}
		payload, _ := json.Marshal(userEvent)
		m.client.SendChan <- payload
		return nil
	}
}

func (m *ChatModel) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
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

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlX:
			m.showDecrypted = !m.showDecrypted
			m.refreshViewport()
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			input := m.textarea.Value()
			if strings.TrimSpace(input) == "" {
				return m, nil
			}

			inputBytes, _ := json.Marshal(input)
			encryptedInput, err := encryption.EncryptData(inputBytes, m.sessionKey)
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
			m.client.SendChan <- payload

			m.messages = append(m.messages, event)
			m.refreshViewport()
			m.viewport.GotoBottom()

			m.textarea.Reset()
			return m, nil
		}

	case ChatMsg:
		if msg.UserID != m.user.UID {
			m.messages = append(m.messages, msg)
			m.refreshViewport()
			m.viewport.GotoBottom()
		}

	case UserMsg:
		content, _ := json.Marshal(fmt.Sprintf("User %s is %s", msg.UserID, msg.Status))
		encrypted, _ := encryption.EncryptData(content, m.sessionKey)
		m.messages = append(m.messages, ChatMsg{
			Type: "USER", UserID: "SYSTEM", Content: encrypted, Timestamp: time.Now().Unix(),
		})
		m.refreshViewport()
		m.viewport.GotoBottom()
	}

	return m, tea.Batch(tiCmd, vpCmd)
}

func (m *ChatModel) renderMessage(msg ChatMsg, width int) string {
	var label string
	if msg.UserID == m.user.UID {
		label = m.senderStyle.Render("You: ")
	} else if msg.UserID == "SYSTEM" {
		label = systemStyle.Render("SYSTEM: ")
	} else {
		label = fmt.Sprintf("%s: ", msg.UserID)
	}

	contentWidth := width - lipgloss.Width(label) - 1

	var text string
	if !m.showDecrypted {
		text = fmt.Sprintf("%x", msg.Content)
	} else {
		decryptedBytes, err := encryption.DecryptData(msg.Content, m.sessionKey)
		if err != nil {
			text = "[Decryption Error]"
		} else {
			var originalText string
			if err := json.Unmarshal(decryptedBytes, &originalText); err != nil {
				text = string(decryptedBytes)
			} else {
				text = originalText
			}
		}
	}

	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("242")).
		Width(contentWidth)

	return lipgloss.JoinHorizontal(lipgloss.Top, label, contentStyle.Render(text))
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

func (m *ChatModel) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress Ctrl+C to quit", m.err)
	}

	if m.state == stateLogin {
		return lipgloss.JoinVertical(
			lipgloss.Left,
			"\n "+secureStyle.Render("SECURE PAKE LOGIN"),
			"\n Room Name:",
			" "+m.roomInput.View(),
			"\n Password:",
			" "+m.passwordInput.View(),
			"\n (Tab to switch | Enter to connect)",
		)
	}

	status := secureStyle.Render("● SESSION ENCRYPTED")
	if m.showDecrypted {
		status = lockedStyle.Render("○ SESSION DECRYPTED")
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
