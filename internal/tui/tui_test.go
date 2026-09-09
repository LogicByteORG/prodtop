package tui

import (
	"testing"

	"github.com/prodtop/prodtop/internal/adapters/postgres"
)

func activityModel() Model {
	m := Model{active: 0}
	m.sessions = []postgres.Session{
		{User: "app", Database: "shop", State: "active", Query: "SELECT * FROM orders"},
		{User: "reporter", Database: "shop", State: "idle", Query: "SELECT count(*) FROM users"},
	}
	return m
}

func TestVisibleWithoutFilter(t *testing.T) {
	m := activityModel()
	if got := m.visible(); len(got) != 2 {
		t.Errorf("visible() = %v, want both rows", got)
	}
}

func TestVisibleWithFilter(t *testing.T) {
	m := activityModel()
	m.filter = "reporter"
	got := m.visible()
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("visible() = %v, want [1]", got)
	}
}

func TestVisibleCaseInsensitive(t *testing.T) {
	m := activityModel()
	m.filter = "ORDERS"
	if got := m.visible(); len(got) != 1 || got[0] != 0 {
		t.Errorf("visible() = %v, want [0]", got)
	}
}

func TestRowCountFollowsFilter(t *testing.T) {
	m := activityModel()
	m.filter = "nothing matches this"
	if m.rowCount() != 0 {
		t.Errorf("rowCount() = %d, want 0", m.rowCount())
	}
}

func TestPaletteFilter(t *testing.T) {
	m := Model{}
	m.palQuery = "export"
	got := m.filteredPalette()
	if len(got) != 1 || got[0].id != "export" {
		t.Errorf("filteredPalette() = %+v, want export only", got)
	}
}
