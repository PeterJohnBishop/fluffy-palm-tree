package main

import (
	"encoding/json"
	"fluffy-palm-tree/tui"
	"fmt"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/gorilla/websocket"
)

func main() {
	fmt.Println("Spilling bubbletea!")
	conn, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	c := NewSocketClient(conn)

	go c.WritePump()

	p := tea.NewProgram(tui.InitialChatModel())

	go listenForMessages(c, p)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

}

type SocketClient struct {
	Conn     *websocket.Conn
	SendChan chan []byte
}

func NewSocketClient(conn *websocket.Conn) *SocketClient {
	return &SocketClient{
		Conn:     conn,
		SendChan: make(chan []byte),
	}

}

func (c *SocketClient) WritePump() {
	defer c.Conn.Close()
	for {
		message, ok := <-c.SendChan
		if !ok {
			return // Channel closed
		}
		err := c.Conn.WriteMessage(websocket.TextMessage, message)
		if err != nil {
			return
		}
	}
}

func listenForMessages(c *SocketClient, p *tea.Program) {
	for {
		_, message, err := c.Conn.ReadMessage()
		if err != nil {
			p.Send(tui.ErrMsg{Type: "error", Err: err})
			return
		}
		err = handleIncomingMessage(p, message)
		if err != nil {
			p.Send(tui.ErrMsg{Type: "error", Err: err})
			continue
		}
	}
}

func handleIncomingMessage(p *tea.Program, data []byte) error {
	// 1. Unmarshal into the "Envelope" to get the Type
	var envelope tui.WebSocketEvent
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}

	// 2. Switch based on the Type and unmarshal into the final struct
	switch envelope.Type {
	case "MSG":
		var event tui.MessageEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	case "USER": // or "STATUS" depending on your server logic
		var event tui.UserEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	case "ACK":
		var event tui.AckEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		p.Send(event)
		return nil

	default:
		return fmt.Errorf("unknown event type: %s", envelope.Type)
	}
}
