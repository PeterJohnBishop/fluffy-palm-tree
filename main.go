package main

import (
	"fluffy-palm-tree/encryption"
	"fluffy-palm-tree/tui"
	"log"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
)

func main() {
	// 1. Load environment variables for local development
	if err := godotenv.Load(); err != nil {
		log.Print("No .env file found")
	}

	// Initialize encryption (SaltMaster, etc.)
	encryption.InitEnv()

	// 2. Initialize an empty SocketClient.
	// We don't Dial here anymore. The Dial happens inside the TUI's runHandshake
	// once the user enters the Room ID and Password.
	c := &tui.SocketClient{
		SendChan: make(chan []byte, 256),
	}

	// 3. Initialize the TUI Model.
	// We pass nil for the program initially because 'p' doesn't exist yet.
	m := tui.InitialChatModel(c, nil)

	// 4. Create the Bubble Tea program.
	// We use the AltScreen (fullscreen) for a better chat experience.
	p := tea.NewProgram(&m, tea.WithAltScreen())

	// 5. Inject the program pointer back into the model.
	// This allows the model to send messages back to itself from goroutines.
	m.SetProgram(p)

	// 6. Start the TUI.
	if _, err := p.Run(); err != nil {
		log.Fatalf("TUI Error: %v", err)
	}
}
