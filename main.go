package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	fileSelectionView = iota
	subjectInputView
	confirmationView
	progressView
	padding  = 2
	maxWidth = 80
)

var (
	titleStyle = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("205"))
	itemStyle  = lipgloss.NewStyle().PaddingLeft(4)
	helpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render
)

type tickMsg time.Time

type model struct {
	csvFilepicker  filepicker.Model
	htmlFilepicker filepicker.Model
	textInput      textinput.Model
	progress       progress.Model
	currentView    int
	csvFilePath    string
	htmlFilePath   string
	subject        string
	confirmed      bool
	quitting       bool
	err            error
	cursor         int
	startTime      time.Time
}

func initialModel() model {
	csvFp := filepicker.New()
	csvFp.AllowedTypes = []string{".csv"}
	csvFp.CurrentDirectory, _ = os.UserHomeDir()

	htmlFp := filepicker.New()
	htmlFp.AllowedTypes = []string{".html"}
	htmlFp.CurrentDirectory, _ = os.UserHomeDir()

	ti := textinput.New()
	ti.Placeholder = "Enter email subject"
	ti.Focus()

	return model{
		csvFilepicker:  csvFp,
		htmlFilepicker: htmlFp,
		textInput:      ti,
		progress:       progress.New(progress.WithDefaultGradient()),
		currentView:    fileSelectionView,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.csvFilepicker.Init(), m.htmlFilepicker.Init(), textinput.Blink)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}
	}

	switch m.currentView {
	case fileSelectionView:
		return updateFileSelection(msg, m)
	case subjectInputView:
		return updateSubjectInput(msg, m)
	case confirmationView:
		return updateConfirmation(msg, m)
	case progressView:
		return updateProgress(msg, m)
	}

	return m, nil
}

func updateProgress(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.progress.Width = msg.Width - padding*2 - 4
		if m.progress.Width > maxWidth {
			m.progress.Width = maxWidth
		}
		return m, nil

	case tickMsg:
		if m.startTime.IsZero() {
			m.startTime = time.Now()
		}
		elapsed := time.Since(m.startTime)
		if elapsed >= 5*time.Second {
			return m, tea.Quit
		}
		progress := float64(elapsed) / (5 * float64(time.Second))
		cmd := m.progress.SetPercent(progress)
		return m, tea.Batch(tickCmd(), cmd)

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd
	}

	return m, nil
}

func updateFileSelection(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			if m.csvFilePath == "" {
				m.csvFilepicker, cmd = m.csvFilepicker.Update(msg)
			} else {
				m.htmlFilepicker, cmd = m.htmlFilepicker.Update(msg)
			}
		case "enter":
			if m.csvFilePath == "" {
				if selected, path := m.csvFilepicker.DidSelectFile(msg); selected {
					m.csvFilePath = path
					m.htmlFilepicker.CurrentDirectory = m.csvFilepicker.CurrentDirectory
				}
			} else {
				if selected, path := m.htmlFilepicker.DidSelectFile(msg); selected {
					m.htmlFilePath = path
					m.currentView = subjectInputView
					return m, textinput.Blink
				}
			}
		}
	}

	var csvCmd, htmlCmd tea.Cmd
	m.csvFilepicker, csvCmd = m.csvFilepicker.Update(msg)
	m.htmlFilepicker, htmlCmd = m.htmlFilepicker.Update(msg)

	return m, tea.Batch(cmd, csvCmd, htmlCmd)
}

func updateSubjectInput(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.textInput, cmd = m.textInput.Update(msg)

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.subject = m.textInput.Value()
			m.currentView = confirmationView
		}
	}

	return m, cmd
}

func updateConfirmation(msg tea.Msg, m model) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "down", "j":
			m.cursor++
			if m.cursor > 1 {
				m.cursor = 0
			}
		case "up", "k":
			m.cursor--
			if m.cursor < 0 {
				m.cursor = 1
			}
		case "enter":
			m.confirmed = m.cursor == 0
			if m.confirmed {
				m.currentView = progressView
				return m, tickCmd()
			}
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var s strings.Builder

	switch m.currentView {
	case fileSelectionView:
		s.WriteString(titleStyle.Render("File Selection"))
		s.WriteString("\n\n")
		if m.csvFilePath == "" {
			s.WriteString(itemStyle.Render("Select CSV file:"))
			s.WriteString("\n")
			s.WriteString(m.csvFilepicker.View())
		} else {
			s.WriteString(itemStyle.Render(fmt.Sprintf("CSV file: %s", m.csvFilePath)))
			s.WriteString("\n\n")
			s.WriteString(itemStyle.Render("Select HTML file:"))
			s.WriteString("\n")
			s.WriteString(m.htmlFilepicker.View())
		}
	case subjectInputView:
		s.WriteString(titleStyle.Render("Email Subject"))
		s.WriteString("\n\n")
		s.WriteString(itemStyle.Render("Enter the email subject:"))
		s.WriteString("\n")
		s.WriteString(m.textInput.View())
	case confirmationView:
		s.WriteString(titleStyle.Render("Confirmation"))
		s.WriteString("\n\n")
		s.WriteString(itemStyle.Render(fmt.Sprintf("CSV file: %s", m.csvFilePath)))
		s.WriteString("\n")
		s.WriteString(itemStyle.Render(fmt.Sprintf("HTML file: %s", m.htmlFilePath)))
		s.WriteString("\n")
		s.WriteString(itemStyle.Render(fmt.Sprintf("Subject: %s", m.subject)))
		s.WriteString("\n\n")
		s.WriteString(itemStyle.Render("Send emails?"))
		s.WriteString("\n")
		for i, choice := range []string{"Yes", "No"} {
			if m.cursor == i {
				s.WriteString(itemStyle.Render(fmt.Sprintf("(•) %s", choice)))
			} else {
				s.WriteString(itemStyle.Render(fmt.Sprintf("( ) %s", choice)))
			}
			s.WriteString("\n")
		}
	case progressView:
		pad := strings.Repeat(" ", padding)
		s.WriteString("\n" + pad + m.progress.View() + "\n\n" + pad + helpStyle("Sending emails..."))
	}

	if m.currentView != progressView {
		s.WriteString("\n")
		s.WriteString(itemStyle.Render("(q to quit)"))
	}
	s.WriteString("\n")

	return s.String()
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func main() {
	p := tea.NewProgram(initialModel())
	m, err := p.Run()
	if err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}

	finalModel := m.(model)
	if finalModel.confirmed {
		fmt.Printf("Emails sent using CSV file: %s, HTML template: %s, and subject: %s\n",
			finalModel.csvFilePath, finalModel.htmlFilePath, finalModel.subject)
	} else {
		fmt.Println("Operation cancelled.")
	}
}
