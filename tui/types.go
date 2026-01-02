package tui

import (
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

type User struct {
	UID       string
	Status    string
	Timestamp int64
}

type SocketClient struct {
	Conn     *websocket.Conn
	SendChan chan []byte
}

type sessionState int

const (
	stateLogin sessionState = iota
	stateChat
)

type ChatModel struct {
	state         sessionState
	roomInput     textinput.Model
	passwordInput textinput.Model
	textarea      textarea.Model
	viewport      viewport.Model

	client        *SocketClient
	program       *tea.Program
	user          User
	messages      []ChatMsg
	sessionKey    []byte
	showDecrypted bool
	err           error
	senderStyle   lipgloss.Style
}

type WebSocketEvent struct {
	Type string `json:"type"`
}

type ChatMsg struct {
	Type      string `json:"type"`
	Content   []byte `json:"content"`
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
}

type UserMsg struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
}

type HandshakeCompleteMsg struct {
	Key []byte
}

type ErrMsg struct {
	Type string
	Err  error
}
