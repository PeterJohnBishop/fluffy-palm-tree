package main

import (
	"fluffy-palm-tree/tui"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

func main() {
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	c := NewSocketClient(conn)

	go c.WritePump()

	p := tea.NewProgram(tui.InitialChatModel(c))

	go tui.ListenForMessages(c, p)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

}

func NewSocketClient(conn *websocket.Conn) *tui.SocketClient {
	return &tui.SocketClient{
		Conn:     conn,
		SendChan: make(chan []byte),
	}

}
