package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type palItem struct {
	id    string
	title string
	hint  string
}

func (m Model) paletteItems() []palItem {
	items := []palItem{
		{id: "go-activity", title: "Go to Activity", hint: "1"},
		{id: "go-locks", title: "Go to Locks", hint: "2"},
		{id: "go-tables", title: "Go to Tables", hint: "3"},
		{id: "go-sql", title: "Go to SQL", hint: "4"},
		{id: "go-redis", title: "Go to Redis", hint: "5"},
		{id: "refresh", title: "Refresh current view", hint: "r"},
		{id: "export", title: "Export current view to CSV", hint: "e"},
	}
	if m.active == 3 && m.lastQuery != "" {
		items = append(items, palItem{id: "clear-sql", title: "Clear SQL results", hint: "x"})
	}
	items = append(items,
		palItem{id: "help", title: "Show help", hint: "?"},
		palItem{id: "quit", title: "Quit", hint: "q"},
	)
	return items
}

func (m Model) filteredPalette() []palItem {
	q := strings.ToLower(strings.TrimSpace(m.palQuery))
	all := m.paletteItems()
	if q == "" {
		return all
	}
	var out []palItem
	for _, item := range all {
		if strings.Contains(strings.ToLower(item.title), q) {
			out = append(out, item)
		}
	}
	return out
}

func (m Model) clampPalCursor(items []palItem) {
	if len(items) == 0 {
		m.palCursor = 0
		return
	}
	if m.palCursor >= len(items) {
		m.palCursor = len(items) - 1
	}
	if m.palCursor < 0 {
		m.palCursor = 0
	}
}

func (m Model) closePalette() Model {
	m.palette = false
	m.palQuery = ""
	m.palCursor = 0
	return m
}

func (m Model) updatePalette(key string, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "ctrl+k":
		return m.closePalette(), nil
	case "enter":
		items := m.filteredPalette()
		if len(items) == 0 {
			return m.closePalette(), nil
		}
		m.clampPalCursor(items)
		id := items[m.palCursor].id
		m = m.closePalette()
		return m.runPalette(id)
	case "up", "k", "ctrl+p":
		items := m.filteredPalette()
		if m.palCursor > 0 {
			m.palCursor--
		} else if len(items) > 0 {
			m.palCursor = len(items) - 1
		}
		return m, nil
	case "down", "j", "ctrl+n":
		items := m.filteredPalette()
		if m.palCursor < len(items)-1 {
			m.palCursor++
		} else {
			m.palCursor = 0
		}
		return m, nil
	case "backspace":
		runes := []rune(m.palQuery)
		if len(runes) > 0 {
			m.palQuery = string(runes[:len(runes)-1])
		}
		m.palCursor = 0
		return m, nil
	}
	if len([]rune(key)) == 1 {
		m.palQuery += key
		m.palCursor = 0
	}
	return m, nil
}

func (m Model) runPalette(id string) (tea.Model, tea.Cmd) {
	switch id {
	case "go-activity", "go-locks", "go-tables", "go-sql", "go-redis":
		tabs := map[string]int{
			"go-activity": 0, "go-locks": 1, "go-tables": 2, "go-sql": 3, "go-redis": 4,
		}
		m.active = tabs[id]
		m.row = 0
		return m.refresh()
	case "refresh":
		return m.refresh()
	case "export":
		return m.exportCurrent()
	case "clear-sql":
		m.lastQuery = ""
		m.sqlCols = nil
		m.sqlRows = nil
		m.row = 0
		return m.refresh()
	case "help":
		m.help = true
		return m, nil
	case "quit":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) renderPalette() string {
	items := m.filteredPalette()
	m.clampPalCursor(items)
	lines := []string{
		titleStyle.Render("palette"),
		"> " + m.palQuery + "█",
		"",
	}
	shown := items
	if len(shown) > 10 {
		shown = shown[:10]
	}
	for i, item := range shown {
		line := "  " + item.title + mutedStyle.Render("   "+item.hint)
		if i == m.palCursor {
			line = selectedStyle.Render("> " + item.title + "   " + item.hint)
		}
		lines = append(lines, line)
	}
	if len(items) == 0 {
		lines = append(lines, mutedStyle.Render("no match"))
	}
	box := panelStyle.Width(56).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
