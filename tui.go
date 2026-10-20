package main

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	blueColor   = lipgloss.Color("4")
	cyanColor   = lipgloss.Color("6")
	yellowColor = lipgloss.Color("3")

	faintStyle     = lipgloss.NewStyle().Faint(true)
	boldStyle      = lipgloss.NewStyle().Bold(true)
	sectionStyle   = lipgloss.NewStyle().Foreground(cyanColor).Bold(true)
	selectedStyle  = lipgloss.NewStyle().Foreground(blueColor).Reverse(true).Bold(true)
	highlightStyle = lipgloss.NewStyle().Foreground(yellowColor).Reverse(true).Bold(true)
	borderStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(blueColor)
)

type matchedSession struct {
	session    Session
	matchedMsg *Message
}

type model struct {
	allSessions  []Session
	filtered     []matchedSession
	input        textinput.Model
	cursor       int
	selectedID   string
	agentVersion string
	width        int
	height       int
}

// newModel initializes the Bubble Tea model with loaded sessions.
func newModel(sessions []Session, agentVersion string) model {
	ti := textinput.New()
	ti.Placeholder = "Search conversations..."
	ti.Focus()
	ti.Prompt = "> "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(blueColor).Bold(true)

	m := model{
		allSessions:  sessions,
		input:        ti,
		agentVersion: agentVersion,
	}
	m.applyFilter("")
	return m
}

// applyFilter filters sessions by case-insensitive substring match in messages or title.
func (m *model) applyFilter(query string) {
	q := strings.TrimSpace(strings.ToLower(query))
	m.filtered = nil

	for _, s := range m.allSessions {
		if q == "" {
			m.filtered = append(m.filtered, matchedSession{session: s})
			continue
		}
		var firstMatch *Message
		for i := range s.Messages {
			if strings.Contains(s.Messages[i].LowerContent, q) {
				firstMatch = &s.Messages[i]
				break
			}
		}
		if firstMatch != nil || strings.Contains(s.LowerTitle, q) {
			m.filtered = append(m.filtered, matchedSession{
				session:    s,
				matchedMsg: firstMatch,
			})
		}
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

// Init starts cursor blinking for the text input.
func (m model) Init() tea.Cmd {
	return textinput.Blink
}

// Update handles keyboard navigation, window resize, and search input changes.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
			return m, nil
		case tea.KeyDown:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
			return m, nil
		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.selectedID = m.filtered[m.cursor].session.ID
				return m, tea.Quit
			}
			return m, nil
		}
	}

	prevVal := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != prevVal {
		m.cursor = 0
		m.applyFilter(m.input.Value())
	}
	return m, cmd
}

// View renders the search input, session list, and preview pane.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	searchBox := borderStyle.Width(max(10, m.width-2)).Render(m.input.View())
	footer := faintStyle.Render(fmt.Sprintf(" ↑/↓: navigate • enter: resume session • esc: quit • %s", m.agentVersion))

	bodyHeight := max(5, m.height-lipgloss.Height(searchBox)-lipgloss.Height(footer)-2)
	leftWidth := max(24, m.width*2/5-2)
	rightWidth := max(24, m.width-leftWidth-4)

	leftPane := borderStyle.Width(leftWidth).Height(bodyHeight).Render(m.renderList(leftWidth-2, bodyHeight))
	rightPane := borderStyle.Width(rightWidth).Height(bodyHeight).Render(m.renderPreview(rightWidth-2, bodyHeight))

	body := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, rightPane)
	return lipgloss.JoinVertical(lipgloss.Left, searchBox, body, footer)
}

// renderList renders the scrollable left column of sessions with dates and titles.
func (m model) renderList(width, height int) string {
	if len(m.filtered) == 0 {
		return faintStyle.Render("No matching sessions")
	}

	start := 0
	if m.cursor >= height {
		start = m.cursor - height + 1
	}
	end := min(len(m.filtered), start+height)

	var lines []string
	for i := start; i < end; i++ {
		s := m.filtered[i].session
		dateStr := s.UpdatedAt.Local().Format("2006-01-02")
		titleWidth := max(5, width-len(dateStr)-3)
		title := truncateLine(s.Title, titleWidth)
		row := fmt.Sprintf(" %s  %s", dateStr, title)
		row = padRight(row, width)

		if i == m.cursor {
			lines = append(lines, selectedStyle.Render(row))
		} else {
			lines = append(lines, faintStyle.Render(" "+dateStr)+"  "+title)
		}
	}
	return strings.Join(lines, "\n")
}

// renderPreview renders the right column with session details, first prompt, match, and last response.
func (m model) renderPreview(width, height int) string {
	if len(m.filtered) == 0 {
		return ""
	}
	item := m.filtered[m.cursor]
	s := item.session
	query := strings.TrimSpace(m.input.Value())

	header := []string{
		boldStyle.Render(truncateLine(s.Title, width)),
		faintStyle.Render(fmt.Sprintf("ID: %s • Date: %s • Steps: %d",
			s.ID, s.UpdatedAt.Local().Format("2006-01-02 15:04"), s.StepCount)),
		"",
	}

	remaining := max(3, height-len(header))
	sections := 2
	if item.matchedMsg != nil && query != "" {
		sections = 3
	}
	secHeight := max(2, (remaining-sections*2)/sections)

	var out []string
	out = append(out, header...)

	out = append(out, sectionStyle.Render("── First User Prompt ──"))
	out = append(out, formatBlock(s.FirstUserMsg, width, secHeight, ""))

	if item.matchedMsg != nil && query != "" {
		out = append(out, "")
		out = append(out, sectionStyle.Render(fmt.Sprintf("── First Match (%s) ──", item.matchedMsg.Role)))
		out = append(out, formatBlock(item.matchedMsg.Content, width, secHeight, query))
	}

	out = append(out, "")
	out = append(out, sectionStyle.Render("── Last AGY Response ──"))
	out = append(out, formatBlock(s.LastAgentMsg, width, secHeight, ""))

	return strings.Join(out, "\n")
}

// formatBlock extracts a window of text around the match (or start), wraps it fast, and limits to maxLines.
func formatBlock(text string, width, maxLines int, query string) string {
	if text == "" || width <= 0 || maxLines <= 0 {
		return faintStyle.Render("(empty)")
	}

	windowRunes := width * (maxLines + 4)
	runes := []rune(text)

	if query != "" {
		if runeIdx := indexRunesFold(runes, []rune(query)); runeIdx != -1 {
			startRune := max(0, runeIdx-windowRunes/3)
			endRune := min(len(runes), startRune+windowRunes)
			runes = runes[startRune:endRune]
		} else if len(runes) > windowRunes {
			runes = runes[:windowRunes]
		}
	} else if len(runes) > windowRunes {
		runes = runes[:windowRunes]
	}

	lines := wrapLines(string(runes), width)

	start := 0
	if query != "" {
		qRunes := []rune(query)
		for i, line := range lines {
			if indexRunesFold([]rune(line), qRunes) != -1 {
				start = max(0, i-maxLines/2)
				break
			}
		}
	}

	if start+maxLines < len(lines) {
		lines = append(lines[start:start+maxLines-1], faintStyle.Render("..."))
	} else {
		lines = lines[start:]
	}

	result := strings.Join(lines, "\n")
	if query != "" {
		result = highlightText(result, query)
	}
	return result
}

// wrapLines quickly splits text into lines fitting within width runes without full ANSI/grapheme overhead.
func wrapLines(text string, width int) []string {
	var out []string
	for _, rawLine := range strings.Split(text, "\n") {
		runes := []rune(strings.TrimRight(rawLine, "\r"))
		if len(runes) == 0 {
			out = append(out, "")
			continue
		}
		for len(runes) > width {
			cut := width
			for i := width; i > width/2; i-- {
				if runes[i-1] == ' ' {
					cut = i
					break
				}
			}
			out = append(out, strings.TrimRight(string(runes[:cut]), " "))
			runes = runes[cut:]
		}
		out = append(out, string(runes))
	}
	return out
}

// highlightText highlights all case-insensitive occurrences of query using terminal reverse style.
func highlightText(text, query string) string {
	textRunes := []rune(text)
	queryRunes := []rune(query)
	if len(queryRunes) == 0 {
		return text
	}

	var b strings.Builder
	i := 0
	for {
		idx := indexRunesFold(textRunes[i:], queryRunes)
		if idx == -1 {
			b.WriteString(string(textRunes[i:]))
			break
		}
		b.WriteString(string(textRunes[i : i+idx]))
		match := string(textRunes[i+idx : i+idx+len(queryRunes)])
		b.WriteString(highlightStyle.Render(match))
		i += idx + len(queryRunes)
	}
	return b.String()
}

// indexRunesFold returns the rune index of the first case-insensitive match of needle in haystack, or -1.
func indexRunesFold(haystack, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(haystack) {
		return -1
	}
	for i := 0; i <= len(haystack)-len(needle); i++ {
		match := true
		for j, nr := range needle {
			if unicode.ToLower(haystack[i+j]) != unicode.ToLower(nr) {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// truncateLine replaces newlines with spaces and truncates to maxRunes with an ellipsis.
func truncateLine(s string, maxRunes int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return string(runes[:maxRunes])
	}
	return string(runes[:maxRunes-1]) + "…"
}

// padRight pads a string with spaces up to width runes.
func padRight(s string, width int) string {
	r := len([]rune(s))
	if r >= width {
		return s
	}
	return s + strings.Repeat(" ", width-r)
}
