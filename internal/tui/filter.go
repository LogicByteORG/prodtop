package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m Model) updateFilter(key string, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		m.filter = strings.TrimSpace(m.filterInput.Value())
		m.filtering = false
		m.filterInput.Blur()
		m.row = 0
		return m, nil
	case "esc":
		m.filter = ""
		m.filtering = false
		m.filterInput.Blur()
		m.filterInput.SetValue("")
		m.row = 0
		return m, nil
	}
	var cmd tea.Cmd
	m.filterInput, cmd = m.filterInput.Update(msg)
	return m, cmd
}

func (m Model) unfilteredCount() int {
	switch m.active {
	case 0:
		return len(m.sessions)
	case 1:
		return len(m.blocks)
	case 2:
		return len(m.tables)
	case 3:
		if m.lastQuery != "" {
			return len(m.sqlRows)
		}
		return len(m.statements)
	case 4:
		return len(m.slow)
	}
	return 0
}

func (m Model) searchText(i int) string {
	switch m.active {
	case 0:
		s := m.sessions[i]
		return s.User + " " + s.Database + " " + s.State + " " + s.Wait + " " + s.Query
	case 1:
		b := m.blocks[i]
		return fmt.Sprintf("%d %d %s %s %s %s", b.BlockedPID, b.BlockingPID, b.LockType, b.Mode, b.BlockedQuery, b.BlockingQuery)
	case 2:
		t := m.tables[i]
		return t.Schema + " " + t.Name
	case 3:
		if m.lastQuery != "" {
			return strings.Join(m.sqlRows[i], " ")
		}
		return m.statements[i].Query
	case 4:
		return strings.Join(m.slow[i].Args, " ")
	}
	return ""
}

func (m Model) visible() []int {
	n := m.unfilteredCount()
	all := make([]int, n)
	for i := range all {
		all[i] = i
	}
	if m.filter == "" {
		return all
	}
	q := strings.ToLower(m.filter)
	var out []int
	for i := 0; i < n; i++ {
		if strings.Contains(strings.ToLower(m.searchText(i)), q) {
			out = append(out, i)
		}
	}
	return out
}

func (m Model) visibleRows() [][]string {
	vis := m.visible()
	out := make([][]string, len(vis))
	for pos, i := range vis {
		out[pos] = m.sqlRows[i]
	}
	return out
}

func (m Model) emptyMessage(fallback string) string {
	if m.filter != "" {
		return "no match for \"" + m.filter + "\" (esc clears)"
	}
	return fallback
}
