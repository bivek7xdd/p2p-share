package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/progress" // ✅ Added progress bar
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
)

const (
	StateMenu = iota
	StateSend
	StateReceive
	StateConnecting
	StateWaitingForReceiver
	StateReceiverConnecting
	StateTransferring // ✅ New state for when the file is actively downloading/uploading!
	StateError
)

type pinMsg string
type errMsg error

// ✅ This is the message WebRTC will send to update the bar (from 0.0 to 1.0)
type progressMsg float64

type model struct {
	state   int
	cursor  int
	choices []string

	// UI Components
	fp       filepicker.Model
	ti       textinput.Model
	progress progress.Model // ✅ Our new progress bar!

	// App Data
	selectedFile string
	pinCode      string
	err          error
}

func initialModel() model {
	fp := filepicker.New()
	fp.CurrentDirectory, _ = os.Getwd()
	fp.AllowedTypes = []string{""}
	fp.Height = 10

	ti := textinput.New()
	ti.Placeholder = "1234"
	ti.CharLimit = 4
	ti.Width = 20

	// ✅ Initialize the progress bar with a beautiful color gradient
	prog := progress.New(progress.WithDefaultGradient())

	return model{
		state:    StateMenu,
		cursor:   0,
		choices:  []string{"📤 Send a File", "📥 Receive a File", "❌ Exit"},
		fp:       fp,
		ti:       ti,
		progress: prog,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fp.Init(), textinput.Blink)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ✅ Listen for progress updates from our future WebRTC logic!
	case progressMsg:
		cmd = m.progress.SetPercent(float64(msg))
		return m, cmd

	// ✅ The progress bar needs frame messages to animate smoothly
	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case pinMsg:
		m.pinCode = string(msg)
		m.state = StateWaitingForReceiver
		return m, nil

	case errMsg:
		m.err = msg
		m.state = StateError
		return m, nil

	case tea.WindowSizeMsg:
		m.fp.Height = msg.Height - 10
		if m.fp.Height < 5 {
			m.fp.Height = 5
		}
		// Adjust progress bar width dynamically
		m.progress.Width = msg.Width - 10
		if m.progress.Width > 60 {
			m.progress.Width = 60
		}

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

		if m.state == StateMenu {
			switch msg.String() {
			case "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.choices)-1 {
					m.cursor++
				}
			case "enter", " ":
				if m.cursor == 0 {
					m.state = StateSend
				} else if m.cursor == 1 {
					m.state = StateReceive
					m.ti.Focus()
					return m, textinput.Blink
				} else {
					return m, tea.Quit
				}
			}
			return m, nil
		}

		if m.state == StateReceive {
			if msg.String() == "esc" {
				m.state = StateMenu
				m.ti.Blur()
				return m, nil
			}
			if msg.String() == "enter" {
				m.pinCode = m.ti.Value()
				m.state = StateReceiverConnecting
				// (WebRTC connection will be triggered here)
				return m, nil
			}
		}
	}

	if m.state == StateSend {
		m.fp, cmd = m.fp.Update(msg)
		cmds = append(cmds, cmd)

		if didSelect, path := m.fp.DidSelectFile(msg); didSelect {
			m.selectedFile = path
			m.state = StateConnecting
			cmds = append(cmds, connectAndGetPIN())
		}
	} else if m.state == StateReceive {
		m.ti, cmd = m.ti.Update(msg)
		cmds = append(cmds, cmd)
	} else {
		if _, isKey := msg.(tea.KeyMsg); !isKey {
			m.fp, cmd = m.fp.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#7D56F4")).Padding(0, 1)
	s := titleStyle.Render(" P2P File Share ") + "\n\n"

	if m.state == StateMenu {
		s += "What would you like to do?\n\n"
		for i, choice := range m.choices {
			cursor := "  "
			if m.cursor == i {
				cursor = "> "
				s += lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render(cursor+choice) + "\n"
			} else {
				s += cursor + choice + "\n"
			}
		}
	} else if m.state == StateSend {
		s += "Select a file to send (Esc to go back):\n\n"
		s += m.fp.View()

	} else if m.state == StateConnecting {
		s += fmt.Sprintf("File Selected: %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render(m.selectedFile))
		s += "⏳ Generating PIN code..."

	} else if m.state == StateWaitingForReceiver {
		pinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Bold(true).Padding(1, 4).BorderStyle(lipgloss.RoundedBorder())
		s += "Tell the receiver to use this PIN:\n\n"
		s += pinStyle.Render(m.pinCode) + "\n\n"
		s += "Waiting for receiver to connect..."

	} else if m.state == StateReceive {
		s += "Enter the 4-digit PIN from the sender:\n\n"
		s += m.ti.View() + "\n\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("(Press Enter to confirm, Esc to go back)")

	} else if m.state == StateReceiverConnecting {
		s += fmt.Sprintf("Connecting to room %s...\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render(m.pinCode))
		s += "⏳ Negotiating WebRTC connection..."

	} else if m.state == StateTransferring {
		// ✅ Render the progress bar!
		s += fmt.Sprintf("🚀 Transferring File...\n\n")
		s += m.progress.View() + "\n\n"

		// We will update this text based on whether it is the sender or receiver later
		s += "Please keep this window open."

	} else if m.state == StateError {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Error:\n")
		s += fmt.Sprintf("%v", m.err)
	}

	if m.state != StateMenu && m.state != StateReceive {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("\n\nPress 'ctrl+c' to quit.")
	}

	return s
}

func connectAndGetPIN() tea.Cmd {
	return func() tea.Msg {
		ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8080/ws", nil)
		if err != nil {
			return errMsg(err)
		}

		ws.WriteMessage(websocket.TextMessage, []byte("SEND"))

		_, msg, err := ws.ReadMessage()
		if err != nil {
			ws.Close()
			return errMsg(err)
		}

		response := string(msg)
		if strings.HasPrefix(response, "PIN:") {
			pin := strings.Split(response, ":")[1]
			return pinMsg(pin)
		}

		ws.Close()
		return errMsg(fmt.Errorf("unexpected server response: %s", response))
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
