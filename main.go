package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time
type exitMsg int
type item struct {
	title       string
	description string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.description }
func (i item) FilterValue() string { return i.title }

type model struct {
	list           list.Model
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
	tag            int
	keys           *delegateKeyMap
	quitTimer      time.Time
	windowSize     tea.WindowSizeMsg
	selectedFile   string // New fie
	msg            tea.Msg
}

const (
	homeView = iota
	fileSelectionView
	subjectInputView
	confirmationView
	progressView
	quitView
	padding          = 2
	maxWidth         = 100
	debounceDuration = time.Second * 5
)

var (
	titleStyle         = lipgloss.NewStyle().MarginLeft(2).Foreground(lipgloss.Color("205"))
	itemStyle          = lipgloss.NewStyle().PaddingLeft(4).Align(lipgloss.Center, lipgloss.Center).Foreground(lipgloss.Color("200"))
	helpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render
	selectedItemStyle  = lipgloss.NewStyle().PaddingLeft(2).Foreground(lipgloss.Color("170"))
	statusMessageStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#04B575", Dark: "#04B575"}).
				Render
	asciiStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	quitTextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render
	style         = lipgloss.NewStyle().
			Width(300).
			PaddingLeft(10).
			PaddingRight(10).
			PaddingTop(20).
			MarginRight(10).
			MarginTop(10).
			Align(lipgloss.Left).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("63")).
			BorderTop(true).
			BorderLeft(true).
			BorderRight(true).
			BorderBottom(true)
)

func initialModel() model {
	currentDir, _ := os.UserHomeDir()

	csvFp := filepicker.New()
	csvFp.AllowedTypes = []string{".csv"}
	csvFp.CurrentDirectory = currentDir

	htmlFp := filepicker.New()
	htmlFp.AllowedTypes = []string{".html"}
	htmlFp.CurrentDirectory = currentDir

	ti := textinput.New()
	ti.Placeholder = "Enter email subject"
	ti.Focus()

	delegateKeys := newDelegateKeyMap()

	items := []list.Item{
		item{title: "CSV and HTML Upload", description: "Pick CSV and HTML files for email"},
		item{title: "Quit", description: "Exit the application"},
	}

	delegate := newItemDelegate(delegateKeys)
	homeList := list.New(items, delegate, 0, 0)
	homeList.Title = "Main Menu"
	homeList.SetShowTitle(false)
	homeList.SetFilteringEnabled(false)
	homeList.Styles.Title = titleStyle

	return model{
		list:           homeList,
		csvFilepicker:  csvFp,
		htmlFilepicker: htmlFp,
		textInput:      ti,
		progress:       progress.New(progress.WithDefaultGradient()),
		currentView:    homeView,
		keys:           delegateKeys,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.csvFilepicker.Init(), m.htmlFilepicker.Init(), textinput.Blink)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowSize = msg
		h, v := style.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)

	case tea.KeyMsg:
		switch m.currentView {
		case homeView:
			switch msg.String() {
			case "up", "k":
				m.list.CursorUp()
			case "down", "j":
				m.list.CursorDown()
			case "enter":
				selectedItem, ok := m.list.SelectedItem().(item)
				if ok {
					if selectedItem.title == "CSV and HTML Upload" {
						m.currentView = fileSelectionView
						return m, m.csvFilepicker.Init()
					} else if selectedItem.title == "Quit" {
						m.quitting = true
						return m, tea.Quit
					}
				}
			}
		case quitView:
			if time.Since(m.quitTimer) >= 5*time.Second {
				return m, tea.Quit
			}
		case progressView:
			m.tag++
			return m, tea.Tick(debounceDuration, func(_ time.Time) tea.Msg {
				return exitMsg(m.tag)
			})
		}

		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		}

	case tea.MouseMsg:
		return m, nil

	case exitMsg:
		if m.currentView == progressView && int(msg) == m.tag {
			m.quitting = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

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

	return m, cmd
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
		progressNum := float64(elapsed) / (5 * float64(time.Second))
		cmd := m.progress.SetPercent(progressNum)
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
				m.csvFilepicker, cmd = m.csvFilepicker.Update(msg)
				if selected, path := m.csvFilepicker.DidSelectFile(msg); selected {
					m.csvFilePath = path
					m.htmlFilepicker.CurrentDirectory = m.csvFilepicker.CurrentDirectory
					return m, nil
				}
			} else {
				m.htmlFilepicker, cmd = m.htmlFilepicker.Update(msg)
				if selected, path := m.htmlFilepicker.DidSelectFile(msg); selected {
					m.htmlFilePath = path
					m.currentView = subjectInputView
					return m, textinput.Blink
				}
			}
		default:
			if m.csvFilePath == "" {
				m.csvFilepicker, cmd = m.csvFilepicker.Update(msg)
			} else {
				m.htmlFilepicker, cmd = m.htmlFilepicker.Update(msg)
			}
		}
	default:
		if m.csvFilePath == "" {
			m.csvFilepicker, cmd = m.csvFilepicker.Update(msg)
		} else {
			m.htmlFilepicker, cmd = m.htmlFilepicker.Update(msg)
		}
	}

	return m, cmd
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

func (m model) renderHomeView() string {
	asciiArt := `
 /$$$$$$$$                                  /$$                                      /$$$$$$  /$$           /$$      
| $$_____/                                 | $$                                     /$$__  $$| $$          | $$      
| $$     /$$$$$$  /$$   /$$ /$$$$$$$   /$$$$$$$  /$$$$$$   /$$$$$$   /$$$$$$$      | $$  \__/| $$ /$$   /$$| $$$$$$$ 
| $$$$$ /$$__  $$| $$  | $$| $$__  $$ /$$__  $$ /$$__  $$ /$$__  $$ /$$_____/      | $$      | $$| $$  | $$| $$__  $$
| $$__/| $$  \ $$| $$  | $$| $$  \ $$| $$  | $$| $$$$$$$$| $$  \__/|  $$$$$$       | $$      | $$| $$  | $$| $$  \ $$
| $$   | $$  | $$| $$  | $$| $$  | $$| $$  | $$| $$_____/| $$       \____  $$      | $$    $$| $$| $$  | $$| $$  | $$
| $$   |  $$$$$$/|  $$$$$$/| $$  | $$|  $$$$$$$|  $$$$$$$| $$       /$$$$$$$/      |  $$$$$$/| $$|  $$$$$$/| $$$$$$$/
|__/    \______/  \______/ |__/  |__/ \_______/ \_______/|__/      |_______/        \______/ |__/ \______/ |_______/ 
                                                                                                                     
`
	centeredAscii := lipgloss.Place(m.windowSize.Width, 10,
		lipgloss.Center, lipgloss.Center,
		asciiStyle.Render(asciiArt))

	items := []string{}
	for i, listItem := range m.list.Items() {
		item, ok := listItem.(item)
		if !ok {
			continue
		}
		if i == m.list.Index() {
			items = append(items, selectedItemStyle.Render(fmt.Sprintf("> %s", item.title)))
		} else {
			items = append(items, itemStyle.Render(fmt.Sprintf("  %s", item.title)))
		}
	}

	menu := lipgloss.JoinVertical(lipgloss.Center, items...)
	centeredMenu := lipgloss.Place(m.windowSize.Width, m.windowSize.Height-20,
		lipgloss.Center, lipgloss.Center,
		menu)

	return lipgloss.JoinVertical(lipgloss.Center, centeredAscii, centeredMenu)
}

func (m model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	var s strings.Builder

	switch m.currentView {
	case homeView:
		return m.renderHomeView()
	case fileSelectionView:
		s.WriteString(titleStyle.Render("File Selection"))
		s.WriteString("\n\n")

		if m.csvFilePath == "" {
			s.WriteString(itemStyle.Render(fmt.Sprintf("Current Directory: %s", m.csvFilepicker.CurrentDirectory)))
			s.WriteString("\n")
			s.WriteString(itemStyle.Render("Select CSV file:"))
			s.WriteString("\n")
			fpView := m.csvFilepicker.View()
			s.WriteString(fpView)
			if len(fpView) == 0 {
				s.WriteString("No files found in this directory.\n")
			} else {
				// Display additional context for selected CSV file
				if selected, path := m.csvFilepicker.DidSelectFile(m.msg); selected {
					s.WriteString(itemStyle.Render(fmt.Sprintf("Selected CSV File: %s", path)))
				}
			}
		} else {
			// Similar logic for HTML file selection
			s.WriteString(itemStyle.Render(fmt.Sprintf("CSV file: %s", m.csvFilePath)))
			s.WriteString("\n")
			s.WriteString(itemStyle.Render(fmt.Sprintf("Current Directory: %s", m.htmlFilepicker.CurrentDirectory)))
			s.WriteString("\n")
			s.WriteString(itemStyle.Render("Select HTML file:"))
			s.WriteString("\n")
			fpView := m.htmlFilepicker.View()
			s.WriteString(fpView)
			if len(fpView) == 0 {
				s.WriteString("No files found in this directory.\n")
			} else {
				if selected, path := m.htmlFilepicker.DidSelectFile(m.msg); selected {
					s.WriteString(itemStyle.Render(fmt.Sprintf("Selected HTML File: %s", path)))
				}
			}
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
		s.WriteString("\n\n" + pad + helpStyle("Press q and wait for 5 second to quit"))
	case quitView:
		remaining := 5 - int(time.Since(m.quitTimer).Seconds())
		s.WriteString(quitTextStyle(fmt.Sprintf("Quitting in %d seconds...", remaining)))
	}

	if m.currentView != homeView && m.currentView != progressView && m.currentView != quitView {
		s.WriteString("\n")
		s.WriteString(itemStyle.Render("(q to quit)"))
	}

	return lipgloss.Place(m.windowSize.Width, m.windowSize.Height,
		lipgloss.Center, lipgloss.Center,
		style.Render(s.String()),
	)
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Millisecond*100, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen(), tea.WithMouseCellMotion())
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

func newItemDelegate(keys *delegateKeyMap) list.DefaultDelegate {
	d := list.NewDefaultDelegate()
	d.UpdateFunc = func(msg tea.Msg, m *list.Model) tea.Cmd {
		var title string
		if i, ok := m.SelectedItem().(item); ok {
			title = i.Title()
		} else {
			return nil
		}
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch {
			case key.Matches(msg, keys.choose):
				return m.NewStatusMessage(statusMessageStyle("You chose " + title))
			case key.Matches(msg, keys.remove):
				index := m.Index()
				m.RemoveItem(index)
				if len(m.Items()) == 0 {
					keys.remove.SetEnabled(false)
				}
				return m.NewStatusMessage(statusMessageStyle("Deleted " + title))
			}
		}
		return nil
	}
	help := []key.Binding{keys.choose, keys.remove}
	d.ShortHelpFunc = func() []key.Binding {
		return help
	}
	d.FullHelpFunc = func() [][]key.Binding {
		return [][]key.Binding{help}
	}
	return d
}

type delegateKeyMap struct {
	choose key.Binding
	remove key.Binding
}

func (d delegateKeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		d.choose,
		d.remove,
	}
}

func (d delegateKeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			d.choose,
			d.remove,
		},
	}
}

func newDelegateKeyMap() *delegateKeyMap {
	return &delegateKeyMap{
		choose: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "choose"),
		),
		remove: key.NewBinding(
			key.WithKeys("x", "backspace"),
			key.WithHelp("x", "delete"),
		),
	}
}

func listDirectoryContents(dir string) []string {
	files, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("Error reading directory: %v", err)}
	}

	var contents []string
	for _, file := range files {
		contents = append(contents, file.Name())
	}
	return contents
}
