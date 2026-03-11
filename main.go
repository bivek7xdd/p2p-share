package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bivek7xdd/p2p-share/internal/transfer"
	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
)

var signalingServerURL string // injected at build time via -ldflags

const (
	StateMenu = iota
	StateSend
	StateSendManual
	StateReceive
	StateConnecting
	StateWaitingForReceiver
	StateReceiverConnecting
	StateTransferring
	StateSuccess
	StateError
)

type pinMsg struct {
	pin string
	ws  *websocket.Conn
}
type errMsg error
type progressMsg float64
type connectedMsg struct{}
type transferSuccessMsg struct{}

type model struct {
	state   int
	cursor  int
	choices []string

	// UI Components
	fp        filepicker.Model
	ti        textinput.Model
	pathInput textinput.Model
	progress  progress.Model

	// App Data
	selectedFile string
	pinCode      string
	err          error
	isSender     bool
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

	pathTi := textinput.New()
	pathTi.Placeholder = "/path/to/your/file.txt"
	pathTi.CharLimit = 0
	pathTi.Width = 50

	prog := progress.New(progress.WithDefaultGradient())

	return model{
		state:     StateMenu,
		cursor:    0,
		choices:   []string{"📤 Send a File (File Picker)", "📂 Send a File (Manual Path)", "📥 Receive a File", "❌ Exit"},
		fp:        fp,
		ti:        ti,
		pathInput: pathTi,
		progress:  prog,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fp.Init(), textinput.Blink)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case progressMsg:
		cmd = m.progress.SetPercent(float64(msg))
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case connectedMsg:
		m.state = StateTransferring
		return m, nil

	case transferSuccessMsg:
		m.state = StateSuccess
		return m, nil

	case pinMsg:
		m.isSender = true
		m.pinCode = msg.pin
		m.state = StateWaitingForReceiver

		cb := transfer.Callbacks{
			OnConnected: func() { globalProgram.Send(connectedMsg{}) },
			OnProgress:  func(p float64) { globalProgram.Send(progressMsg(p)) },
			OnSuccess:   func() { globalProgram.Send(transferSuccessMsg{}) },
			OnError:     func(e error) { globalProgram.Send(errMsg(e)) },
		}

		go transfer.StartSender(msg.ws, m.selectedFile, cb)
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
					m.state = StateSendManual
					m.pathInput.Focus()
					return m, textinput.Blink
				} else if m.cursor == 2 {
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
				m.isSender = false
				m.pinCode = strings.TrimSpace(m.ti.Value())
				if m.pinCode == "" {
					return m, nil
				}
				m.state = StateReceiverConnecting

				cb := transfer.Callbacks{
					OnConnected: func() { globalProgram.Send(connectedMsg{}) },
					OnProgress:  func(p float64) { globalProgram.Send(progressMsg(p)) },
					OnSuccess:   func() { globalProgram.Send(transferSuccessMsg{}) },
					OnError:     func(e error) { globalProgram.Send(errMsg(e)) },
				}

				go transfer.StartReceiver(m.pinCode, signalingServerURL, cb)
				return m, nil
			}
		}

		if m.state == StateSendManual {
			if msg.String() == "esc" {
				m.state = StateMenu
				m.pathInput.Blur()
				return m, nil
			}
			if msg.String() == "enter" {
				path := strings.TrimSpace(m.pathInput.Value())
				if path != "" {
					fInfo, err := os.Stat(path)
					if err == nil && !fInfo.IsDir() {
						m.selectedFile = path
						m.state = StateConnecting
						return m, connectAndGetPIN()
					} else {
						m.pathInput.SetValue("")
						m.pathInput.Placeholder = "Invalid path or directory! Try again."
					}
				}
				return m, nil
			}
		}

		if m.state == StateSuccess || m.state == StateError {
			if msg.String() == "enter" || msg.String() == "esc" || msg.String() == "q" {
				return m, tea.Quit
			}
		}
	}

	if m.state == StateSend {
		m.fp, cmd = m.fp.Update(msg)
		cmds = append(cmds, cmd)

		if didSelect, path := m.fp.DidSelectFile(msg); didSelect {
			fInfo, err := os.Stat(path)
			if err != nil || fInfo.IsDir() {
				// Don't send directories!
				return m, nil
			}
			m.selectedFile = path
			m.state = StateConnecting
			cmds = append(cmds, connectAndGetPIN())
		}
	} else if m.state == StateSendManual {
		m.pathInput, cmd = m.pathInput.Update(msg)
		cmds = append(cmds, cmd)
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
	logo := `
 ___  __  ___    ___ _
| _ \_  )| _ \  / __| |_  __ _ _ _ ___
|  _// / |  _/  \__ \ ' \/ _` + "`" + ` | '_/ -_)
|_| /___||_|    |___/_||_\__,_|_| \___|
`
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#04B575")).
		Bold(true).
		MarginBottom(1)

	s := titleStyle.Render(strings.TrimPrefix(logo, "\n"))

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

	} else if m.state == StateSendManual {
		s += "Enter the full path to the file you want to send:\n\n"
		s += m.pathInput.View() + "\n\n"
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("(Press Enter to confirm, Esc to go back)")

	} else if m.state == StateConnecting {
		s += fmt.Sprintf("File Selected: %s\n\n", lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render(filepath.Base(m.selectedFile)))
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
		if m.isSender {
			s += "🚀 Uploading File...\n\n"
		} else {
			s += "🚀 Downloading File...\n\n"
		}
		s += m.progress.View() + "\n\n"

		s += "Please keep this window open."

	} else if m.state == StateSuccess {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#04B575")).Render("✅ Transfer Complete!\n\n")
		if m.isSender {
			s += "The file was successfully sent."
		} else {
			s += "The file was safely downloaded to this folder."
		}
		s += "\n\nPress 'Enter' or 'q' to quit."

	} else if m.state == StateError {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Render("❌ Error:\n")
		s += fmt.Sprintf("%v", m.err)
		s += "\n\nPress 'Enter' or 'q' to quit."
	}

	if m.state != StateMenu && m.state != StateReceive && m.state != StateSuccess && m.state != StateError && m.state != StateSendManual {
		s += lipgloss.NewStyle().Foreground(lipgloss.Color("#666666")).Render("\n\nPress 'ctrl+c' to quit.")
	}

	return s
}

func connectAndGetPIN() tea.Cmd {
	return func() tea.Msg {
		ws, _, err := websocket.DefaultDialer.Dial(signalingServerURL, nil)
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
			return pinMsg{pin: pin, ws: ws}
		}

		ws.Close()
		return errMsg(fmt.Errorf("unexpected server response: %s", response))
	}
}

var globalProgram *tea.Program

func main() {
	godotenv.Load()
	if signalingServerURL == "" {
		signalingServerURL = os.Getenv("SIGNALING_SERVER_URL")
	}
	globalProgram = tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := globalProgram.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v\n", err)
		os.Exit(1)
	}
}
