// Command filezui is the interactive console UI for Filez. It walks through the
// same upload flow as the filez CLI (host, file, mode, options) with a live
// progress bar and animations, built on Bubble Tea.
package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/DasDarki/filez/internal/client/api"
	"github.com/DasDarki/filez/internal/client/config"
	"github.com/DasDarki/filez/internal/client/ui"
	"github.com/DasDarki/filez/internal/timefmt"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func main() {
	if _, err := tea.NewProgram(initialModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

type step int

const (
	stepHost step = iota
	stepFile
	stepMode
	stepTTL
	stepDownloads
	stepPassword
	stepUpload
	stepDone
	stepError
)

// Messages from the upload goroutine.
type progressMsg float64
type doneMsg struct{ res *api.UploadResult }
type errMsg struct{ err error }

type modeItem struct {
	title, desc, mode string
}

var modeItems = []modeItem{
	{"Permanent", "Keep the file forever", "permanent"},
	{"Temporary", "Auto-delete after a duration", "temp"},
	{"Limited downloads", "Delete after N downloads", "limited"},
	{"Password protected", "Require a password to open", "password"},
}

type model struct {
	cfg   *config.Config
	step  step
	width int

	hosts      []config.Host
	hostCursor int
	host       config.Host

	fileInput textinput.Model
	fileErr   string
	filePath  string

	modeCursor int
	mode       string
	ttl        string
	downloads  int
	password   string

	optInput textinput.Model
	optErr   string

	spinner  spinner.Model
	progress progress.Model
	frac     float64
	ch       chan tea.Msg

	result *api.UploadResult
	err    error
}

func initialModel() model {
	cfg, _ := config.Load()

	fi := textinput.New()
	fi.Placeholder = "path/to/file"
	fi.CharLimit = 4096
	fi.Width = 44

	oi := textinput.New()
	oi.Width = 44

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(ui.Accent)

	m := model{
		cfg:       cfg,
		hosts:     cfg.Hosts,
		fileInput: fi,
		optInput:  oi,
		spinner:   sp,
		progress:  progress.New(progress.WithDefaultGradient(), progress.WithWidth(44)),
	}

	switch {
	case len(m.hosts) == 0:
		m.step = stepError
		m.err = fmt.Errorf("no host configured — run: filez config hosts add")
	case len(m.hosts) == 1:
		m.host = m.hosts[0]
		m.step = stepFile
		m.fileInput.Focus()
	default:
		m.step = stepHost
		for i := range m.hosts {
			if m.hosts[i].Primary {
				m.hostCursor = i
			}
		}
	}
	return m
}

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		w := msg.Width - 8
		if w > 60 {
			w = 60
		}
		if w > 10 {
			m.progress.Width = w
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.step == stepUpload {
				return m, nil // don't abort mid-upload
			}
			return m, tea.Quit
		}
		return m.handleKey(msg)

	case spinner.TickMsg:
		if m.step == stepUpload {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case progressMsg:
		m.frac = float64(msg)
		if m.step == stepUpload {
			return m, listen(m.ch)
		}
		return m, nil

	case doneMsg:
		m.result = msg.res
		m.step = stepDone
		return m, nil

	case errMsg:
		m.err = msg.err
		m.step = stepError
		return m, nil
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.step {
	case stepHost:
		switch key {
		case "up", "k":
			if m.hostCursor > 0 {
				m.hostCursor--
			}
		case "down", "j":
			if m.hostCursor < len(m.hosts)-1 {
				m.hostCursor++
			}
		case "enter":
			m.host = m.hosts[m.hostCursor]
			m.step = stepFile
			m.fileInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case stepFile:
		if key == "enter" {
			path := m.fileInput.Value()
			if fi, err := os.Stat(path); err != nil {
				m.fileErr = "file not found"
				return m, nil
			} else if fi.IsDir() {
				m.fileErr = "that is a directory"
				return m, nil
			}
			m.fileErr = ""
			m.filePath = path
			m.step = stepMode
			return m, nil
		}
		var cmd tea.Cmd
		m.fileInput, cmd = m.fileInput.Update(msg)
		return m, cmd

	case stepMode:
		switch key {
		case "up", "k":
			if m.modeCursor > 0 {
				m.modeCursor--
			}
		case "down", "j":
			if m.modeCursor < len(modeItems)-1 {
				m.modeCursor++
			}
		case "enter":
			return m.chooseMode()
		}
		return m, nil

	case stepTTL:
		if key == "enter" {
			v := m.optInput.Value()
			if _, err := timefmt.Parse(v); err != nil {
				m.optErr = "invalid duration (try 20m, 2d, 2d20m, 1M)"
				return m, nil
			}
			m.ttl = v
			return m, m.beginUpload()
		}
		return m.updateOpt(msg)

	case stepDownloads:
		if key == "enter" {
			n, err := strconv.Atoi(m.optInput.Value())
			if err != nil || n < 1 {
				m.optErr = "enter a number ≥ 1"
				return m, nil
			}
			m.downloads = n
			return m, m.beginUpload()
		}
		return m.updateOpt(msg)

	case stepPassword:
		if key == "enter" {
			if m.optInput.Value() == "" {
				m.optErr = "password cannot be empty"
				return m, nil
			}
			m.password = m.optInput.Value()
			return m, m.beginUpload()
		}
		return m.updateOpt(msg)

	case stepDone, stepError:
		if key == "enter" || key == "q" {
			return m, tea.Quit
		}
		return m, nil
	}

	return m, nil
}

func (m model) updateOpt(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.optErr = ""
	var cmd tea.Cmd
	m.optInput, cmd = m.optInput.Update(msg)
	return m, cmd
}

func (m model) chooseMode() (tea.Model, tea.Cmd) {
	item := modeItems[m.modeCursor]
	m.mode = item.mode

	// Reset the option input for the next step.
	m.optInput.Reset()
	m.optInput.EchoMode = textinput.EchoNormal
	m.optErr = ""

	switch item.mode {
	case "permanent":
		return m, m.beginUpload()
	case "temp":
		m.optInput.Placeholder = "20m, 2d, 2d20m, 1M"
		m.optInput.Focus()
		m.step = stepTTL
		return m, textinput.Blink
	case "limited":
		m.optInput.Placeholder = "number of downloads"
		m.optInput.SetValue("1")
		m.optInput.Focus()
		m.step = stepDownloads
		return m, textinput.Blink
	case "password":
		m.mode = "permanent" // password is a permanent file with a password
		m.optInput.Placeholder = "password"
		m.optInput.EchoMode = textinput.EchoPassword
		m.optInput.Focus()
		m.step = stepPassword
		return m, textinput.Blink
	}
	return m, nil
}

func (m *model) beginUpload() tea.Cmd {
	m.step = stepUpload
	m.frac = 0
	m.ch = make(chan tea.Msg, 64)

	host := m.host
	client := api.FromHost(&host)
	opts := api.UploadOptions{Mode: m.mode, TTL: m.ttl, Downloads: m.downloads, Password: m.password}
	path := m.filePath
	ch := m.ch

	go func() {
		res, err := client.Upload(path, opts, func(sent, total int64) {
			if total > 0 {
				select {
				case ch <- progressMsg(float64(sent) / float64(total)):
				default:
				}
			}
		})
		if err != nil {
			ch <- errMsg{err}
		} else {
			ch <- doneMsg{res}
		}
	}()

	return tea.Batch(m.spinner.Tick, listen(ch))
}

func listen(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}
