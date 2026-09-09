package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/prodtop/prodtop/internal/adapters/postgres"
	"github.com/prodtop/prodtop/internal/adapters/redis"
	"github.com/prodtop/prodtop/internal/config"
	"github.com/prodtop/prodtop/internal/export"
	"github.com/prodtop/prodtop/internal/favorites"
)

var tabNames = []string{"Activity", "Locks", "Tables", "SQL", "Redis"}

const (
	focusServices = 0
	focusMain     = 1
)

type Model struct {
	cfg          config.Config
	write        bool
	version      string
	active       int
	focus        int
	cursor       int
	row          int
	services     []string
	width        int
	height       int
	help         bool
	notice       string
	loading      bool
	sessions     []postgres.Session
	blocks       []postgres.Block
	tables       []postgres.TableStat
	statements   []postgres.Statement
	statementsOK bool
	clients      map[string]*postgres.Client
	sqlInput     textinput.Model
	editing      bool
	sqlCols      []string
	sqlRows      [][]string
	sqlDur       time.Duration
	lastQuery    string
	favs         []favorites.Entry
	favPos       int
	rinfo        *redis.Info
	slow         []redis.SlowEntry
	rkeys        []redis.KeyEntry
	rclients     map[string]*redis.Client
}

type tickMsg time.Time

type sessionsMsg struct {
	target   string
	sessions []postgres.Session
	err      error
}

type blocksMsg struct {
	target string
	blocks []postgres.Block
	err    error
}

type tablesMsg struct {
	target string
	tables []postgres.TableStat
	err    error
}

type statementsMsg struct {
	target     string
	statements []postgres.Statement
	available  bool
	err        error
}

type queryMsg struct {
	target string
	query  string
	cols   []string
	rows   [][]string
	took   time.Duration
	err    error
}

type redisMsg struct {
	target string
	info   *redis.Info
	slow   []redis.SlowEntry
	keys   []redis.KeyEntry
	err    error
}

func New(cfg config.Config, write bool, version string) Model {
	services := make([]string, 0, len(cfg.Postgres)+len(cfg.Redis))
	for _, pg := range cfg.Postgres {
		services = append(services, "pg "+pg.Name)
	}
	for _, r := range cfg.Redis {
		services = append(services, "redis "+r.Name)
	}
	ti := textinput.New()
	ti.Placeholder = "SELECT * FROM users LIMIT 20"
	ti.Prompt = "sql> "
	ti.CharLimit = 2000
	favs, _ := favorites.Load()
	return Model{
		cfg:      cfg,
		write:    write,
		version:  version,
		services: services,
		clients:  map[string]*postgres.Client{},
		sqlInput: ti,
		favs:     favs,
		favPos:   -1,
		rclients: map[string]*redis.Client{},
		width:    120,
		height:   30,
	}
}

func (m Model) WithNotice(notice string) Model {
	m.notice = notice
	return m
}

func (m Model) queryTimeout() time.Duration {
	return time.Duration(m.cfg.QueryTimeoutSeconds) * time.Second
}

func (m Model) refreshEvery() time.Duration {
	return time.Duration(m.cfg.RefreshSeconds) * time.Second
}

func (m Model) Init() tea.Cmd {
	if len(m.services) == 0 {
		return nil
	}
	m.loading = true
	return tea.Batch(m.fetchCmd(), m.tickCmd())
}

func (m Model) tickCmd() tea.Cmd {
	every := m.refreshEvery()
	return tea.Tick(every, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		return m, nil
	case tickMsg:
		cmds := []tea.Cmd{m.tickCmd()}
		if len(m.services) > 0 {
			m.loading = true
			if cmd := m.fetchCmd(); cmd != nil {
				cmds = append(cmds, cmd)
			} else {
				m.loading = false
			}
		}
		return m, tea.Batch(cmds...)
	case sessionsMsg:
		m.loading = false
		if msg.target != m.selectedTarget() || m.active != 0 {
			return m, nil
		}
		if msg.err != nil {
			m.dropClient(msg.target)
			m.notice = "postgres: " + msg.err.Error()
			return m, nil
		}
		m.sessions = msg.sessions
		m.notice = ""
		return m, nil
	case blocksMsg:
		m.loading = false
		if msg.target != m.selectedTarget() || m.active != 1 {
			return m, nil
		}
		if msg.err != nil {
			m.dropClient(msg.target)
			m.notice = "postgres: " + msg.err.Error()
			return m, nil
		}
		m.blocks = msg.blocks
		m.notice = ""
		return m, nil
	case tablesMsg:
		m.loading = false
		if msg.target != m.selectedTarget() || m.active != 2 {
			return m, nil
		}
		if msg.err != nil {
			m.dropClient(msg.target)
			m.notice = "postgres: " + msg.err.Error()
			return m, nil
		}
		m.tables = msg.tables
		m.notice = ""
		return m, nil
	case statementsMsg:
		m.loading = false
		if msg.target != m.selectedTarget() || m.active != 3 {
			return m, nil
		}
		if msg.err != nil {
			m.dropClient(msg.target)
			m.notice = "postgres: " + msg.err.Error()
			return m, nil
		}
		m.statements = msg.statements
		m.statementsOK = msg.available
		m.notice = ""
		return m, nil
	case queryMsg:
		m.loading = false
		if msg.target != m.selectedTarget() || m.active != 3 {
			return m, nil
		}
		if msg.err != nil {
			m.notice = msg.err.Error()
			return m, nil
		}
		m.sqlCols = msg.cols
		m.sqlRows = msg.rows
		m.sqlDur = msg.took
		m.lastQuery = msg.query
		m.row = 0
		m.notice = fmt.Sprintf("%d rows in %s", len(msg.rows), msg.took.Round(time.Millisecond))
		return m, nil
	case redisMsg:
		m.loading = false
		if msg.target != m.selectedTarget() || m.active != 4 {
			return m, nil
		}
		if msg.err != nil {
			m.dropRedisClient(msg.target)
			m.notice = "redis: " + msg.err.Error()
			return m, nil
		}
		m.rinfo = msg.info
		m.slow = msg.slow
		m.rkeys = msg.keys
		m.notice = ""
		return m, nil
	case tea.KeyPressMsg:
		if m.editing && m.active == 3 {
			return m.updateEditor(msg.String(), msg)
		}
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m Model) selectedTarget() string {
	if len(m.services) == 0 || m.cursor >= len(m.services) {
		return ""
	}
	return m.services[m.cursor]
}

func (m Model) selectedPostgres() (string, string, bool) {
	svc := m.selectedTarget()
	if !strings.HasPrefix(svc, "pg ") {
		return "", "", false
	}
	name := strings.TrimPrefix(svc, "pg ")
	for _, pg := range m.cfg.Postgres {
		if pg.Name == name {
			return name, pg.DSN, true
		}
	}
	return "", "", false
}

func (m Model) selectedRedis() (string, config.RedisTarget, bool) {
	svc := m.selectedTarget()
	if !strings.HasPrefix(svc, "redis ") {
		return "", config.RedisTarget{}, false
	}
	name := strings.TrimPrefix(svc, "redis ")
	for _, r := range m.cfg.Redis {
		if r.Name == name {
			return name, r, true
		}
	}
	return "", config.RedisTarget{}, false
}

func (m *Model) clientFor(target, dsn string) (*postgres.Client, error) {
	if c, ok := m.clients[target]; ok {
		return c, nil
	}
	c, err := postgres.ConnectLazy(dsn, m.queryTimeout())
	if err != nil {
		return nil, err
	}
	m.clients[target] = c
	return c, nil
}

func (m *Model) dropClient(target string) {
	if c, ok := m.clients[target]; ok {
		c.Close()
		delete(m.clients, target)
	}
}

func (m *Model) redisClientFor(target string, cfg config.RedisTarget) (*redis.Client, error) {
	if c, ok := m.rclients[target]; ok {
		return c, nil
	}
	c, err := redis.ConnectLazy(cfg.Addr, cfg.Password, cfg.DB, m.queryTimeout())
	if err != nil {
		return nil, err
	}
	m.rclients[target] = c
	return c, nil
}

func (m *Model) dropRedisClient(target string) {
	if c, ok := m.rclients[target]; ok {
		c.Close()
		delete(m.rclients, target)
	}
}

func (m Model) fetchCmd() tea.Cmd {
	if m.active == 4 {
		target, cfg, ok := m.selectedRedis()
		if !ok {
			return nil
		}
		client, err := m.redisClientFor(target, cfg)
		if err != nil {
			e := err
			return func() tea.Msg {
				return redisMsg{target: target, err: e}
			}
		}
		return func() tea.Msg {
			info, err := client.Info(context.Background())
			if err != nil {
				return redisMsg{target: target, err: err}
			}
			slow, err := client.Slowlog(context.Background())
			if err != nil {
				return redisMsg{target: target, err: err}
			}
			keys, err := client.Keys(context.Background())
			if err != nil {
				return redisMsg{target: target, err: err}
			}
			return redisMsg{target: target, info: info, slow: slow, keys: keys}
		}
	}
	target, dsn, ok := m.selectedPostgres()
	if !ok {
		return nil
	}
	client, err := m.clientFor(target, dsn)
	if err != nil {
		e := err
		return func() tea.Msg {
			return sessionsMsg{target: target, err: e}
		}
	}
	switch m.active {
	case 0:
		return func() tea.Msg {
			s, err := client.Activity(context.Background())
			return sessionsMsg{target: target, sessions: s, err: err}
		}
	case 1:
		return func() tea.Msg {
			b, err := client.Blocking(context.Background())
			return blocksMsg{target: target, blocks: b, err: err}
		}
	case 2:
		return func() tea.Msg {
			t, err := client.Tables(context.Background())
			return tablesMsg{target: target, tables: t, err: err}
		}
	case 3:
		return func() tea.Msg {
			s, available, err := client.Statements(context.Background())
			return statementsMsg{target: target, statements: s, available: available, err: err}
		}
	}
	return nil
}

func (m Model) updateEditor(key string, msg tea.Msg) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		return m.runSQL()
	case "esc":
		m.editing = false
		m.sqlInput.Blur()
		return m, nil
	}
	var cmd tea.Cmd
	m.sqlInput, cmd = m.sqlInput.Update(msg)
	return m, cmd
}

func (m Model) runSQL() (tea.Model, tea.Cmd) {
	query := strings.TrimSpace(m.sqlInput.Value())
	if i := strings.IndexAny(query, "\r\n"); i >= 0 {
		query = strings.TrimSpace(query[:i])
	}
	if query == "" {
		m.notice = "type a query first"
		return m, nil
	}
	target, dsn, ok := m.selectedPostgres()
	if !ok {
		m.notice = "select a postgres service first"
		return m, nil
	}
	client, err := m.clientFor(target, dsn)
	if err != nil {
		m.notice = "postgres: " + err.Error()
		return m, nil
	}
	write := m.write
	m.loading = true
	return m, func() tea.Msg {
		start := time.Now()
		cols, rows, err := client.RunQuery(context.Background(), query, write)
		return queryMsg{target: target, query: query, cols: cols, rows: rows, took: time.Since(start), err: err}
	}
}

func (m Model) exportCurrent() (tea.Model, tea.Cmd) {
	name, header, rows := m.exportData()
	if name == "" {
		m.notice = "nothing to export on this tab"
		return m, nil
	}
	path := fmt.Sprintf("prodtop-%s-%s.csv", name, time.Now().Format("20060102-150405"))
	if err := export.WriteCSV(path, header, rows); err != nil {
		m.notice = "export: " + err.Error()
		return m, nil
	}
	m.notice = fmt.Sprintf("exported %d rows to %s", len(rows), path)
	return m, nil
}

func (m Model) exportData() (string, []string, [][]string) {
	switch m.active {
	case 0:
		rows := make([][]string, len(m.sessions))
		for i, s := range m.sessions {
			rows[i] = []string{fmt.Sprintf("%d", s.PID), s.User, s.Database, s.State, s.Wait, formatAgo(s.Duration), s.Query}
		}
		return "activity", []string{"pid", "user", "database", "state", "wait", "duration", "query"}, rows
	case 1:
		rows := make([][]string, len(m.blocks))
		for i, b := range m.blocks {
			rows[i] = []string{fmt.Sprintf("%d", b.BlockedPID), fmt.Sprintf("%d", b.BlockingPID), b.LockType, b.Mode, b.BlockedQuery, b.BlockingQuery}
		}
		return "locks", []string{"blocked_pid", "blocking_pid", "locktype", "mode", "blocked_query", "blocking_query"}, rows
	case 2:
		rows := make([][]string, len(m.tables))
		for i, t := range m.tables {
			rows[i] = []string{t.Schema, t.Name, fmt.Sprintf("%d", t.SizeBytes), fmt.Sprintf("%d", t.LiveTuples), fmt.Sprintf("%d", t.DeadTuples)}
		}
		return "tables", []string{"schema", "table", "size_bytes", "live_tuples", "dead_tuples"}, rows
	case 3:
		if m.lastQuery != "" {
			return "sql", m.sqlCols, m.sqlRows
		}
		rows := make([][]string, len(m.statements))
		for i, s := range m.statements {
			rows[i] = []string{fmt.Sprintf("%d", s.Calls), fmt.Sprintf("%.3f", s.MeanMs), fmt.Sprintf("%.0f", s.TotalMs), fmt.Sprintf("%d", s.Rows), s.Query}
		}
		return "statements", []string{"calls", "mean_ms", "total_ms", "rows", "query"}, rows
	case 4:
		rows := make([][]string, len(m.slow))
		for i, s := range m.slow {
			rows[i] = []string{fmt.Sprintf("%d", s.ID), s.At.Format(time.RFC3339), s.Duration.String(), strings.Join(s.Args, " ")}
		}
		return "redis-slowlog", []string{"id", "at", "duration", "command"}, rows
	}
	return "", nil, nil
}

func (m Model) rowCount() int {
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

func (m Model) clampRow() {
	if n := m.rowCount(); n == 0 {
		m.row = 0
	} else if m.row >= n {
		m.row = n - 1
	}
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	if len(m.services) == 0 {
		m.notice = "no targets configured"
		return m, nil
	}
	m.loading = true
	if cmd := m.fetchCmd(); cmd != nil {
		return m, cmd
	}
	m.loading = false
	return m, nil
}

func (m Model) handleKey(key string) (tea.Model, tea.Cmd) {
	if m.help {
		m.help = false
		return m, nil
	}
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?", "H":
		m.help = true
		return m, nil
	case "tab":
		if m.focus == focusServices {
			m.focus = focusMain
		} else {
			m.focus = focusServices
		}
		return m, nil
	case "1", "2", "3", "4", "5":
		m.active = int(key[0] - '1')
		m.row = 0
		m.clampRow()
		return m.refresh()
	case "j", "down":
		if m.focus == focusServices {
			if m.cursor < len(m.services)-1 {
				m.cursor++
				m.row = 0
			}
			return m.refresh()
		}
		if m.row < m.rowCount()-1 {
			m.row++
		}
		return m, nil
	case "k", "up":
		if m.focus == focusServices {
			if m.cursor > 0 {
				m.cursor--
				m.row = 0
			}
			return m.refresh()
		}
		if m.row > 0 {
			m.row--
		}
		return m, nil
	case "g":
		if m.focus == focusServices {
			m.cursor = 0
		} else {
			m.row = 0
		}
		return m, nil
	case "G":
		if m.focus == focusServices {
			if len(m.services) > 0 {
				m.cursor = len(m.services) - 1
			}
		} else if m.rowCount() > 0 {
			m.row = m.rowCount() - 1
		}
		return m, nil
	case "enter":
		m.focus = focusMain
		return m.refresh()
	case "r":
		return m.refresh()
	case "i":
		if m.active == 3 {
			m.editing = true
			return m, m.sqlInput.Focus()
		}
		return m, nil
	case "f":
		if m.active == 3 {
			query := strings.TrimSpace(m.sqlInput.Value())
			if query == "" {
				query = m.lastQuery
			}
			entry, err := favorites.Add(query)
			if err != nil {
				m.notice = "favorites: " + err.Error()
				return m, nil
			}
			m.favs, _ = favorites.Load()
			m.notice = fmt.Sprintf("saved as %s (%d favorites)", entry.Name, len(m.favs))
			return m, nil
		}
		return m, nil
	case "o":
		if m.active == 3 && len(m.favs) > 0 {
			m.favPos = (m.favPos + 1) % len(m.favs)
			m.sqlInput.SetValue(m.favs[m.favPos].Query)
			m.editing = true
			return m, m.sqlInput.Focus()
		}
		return m, nil
	case "e":
		return m.exportCurrent()
	case "x":
		if m.active == 3 && m.lastQuery != "" {
			m.lastQuery = ""
			m.sqlCols = nil
			m.sqlRows = nil
			m.row = 0
			return m.refresh()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.help {
		return m.renderHelp()
	}
	mode := "READ-ONLY"
	if m.write {
		mode = "WRITE"
	}
	header := titleStyle.Render("prodtop") + " " +
		mutedStyle.Render(m.version) + "  " +
		statusStyle.Render(mode)
	if m.loading {
		header += "  " + mutedStyle.Render("loading…")
	}

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderServices(),
		m.renderMain(),
		m.renderDetail(),
	)
	parts := []string{header, m.renderTabs(), body, m.renderFooter()}
	if m.notice != "" {
		parts = append(parts, noticeStyle.Render(truncate(m.notice, m.width)))
	}
	return strings.Join(parts, "\n")
}

func (m Model) renderTabs() string {
	rendered := make([]string, len(tabNames))
	for i, name := range tabNames {
		label := " " + string(rune('1'+i)) + " " + name + " "
		if i == m.active {
			rendered[i] = activeTabStyle.Render("[" + label + "]")
		} else {
			rendered[i] = idleTabStyle.Render(" " + label + " ")
		}
	}
	return strings.Join(rendered, "")
}

func (m Model) renderServices() string {
	lines := []string{titleStyle.Render("services")}
	if len(m.services) == 0 {
		lines = append(lines, mutedStyle.Render("no targets configured"))
		lines = append(lines, mutedStyle.Render("see config.example.yaml"))
	} else {
		for i, svc := range m.services {
			if i == m.cursor && m.focus == focusServices {
				lines = append(lines, selectedStyle.Render("> "+svc))
			} else if i == m.cursor {
				lines = append(lines, "• "+svc)
			} else {
				lines = append(lines, "  "+svc)
			}
		}
	}
	return panelStyle.Width(26).Render(strings.Join(lines, "\n"))
}

func (m Model) renderMain() string {
	width := m.width - 62
	if width < 40 {
		width = 40
	}
	var lines []string
	switch m.active {
	case 0:
		lines = m.activityLines(width)
	case 1:
		lines = m.blocksLines(width)
	case 2:
		lines = m.tablesLines(width)
	case 3:
		lines = m.sqlLines(width)
	case 4:
		lines = m.redisLines(width)
	}
	return panelStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) activityLines(width int) []string {
	lines := []string{
		titleStyle.Render(fmt.Sprintf("activity  (%d sessions)", len(m.sessions))),
		mutedStyle.Render(
			cell("PID", 8) + cell("USER", 12) + cell("DB", 12) +
				cell("STATE", 12) + cell("WAIT", 16) + cell("DUR", 7) + "QUERY",
		),
	}
	qw := width - 8 - 12 - 12 - 12 - 16 - 7 - 8
	if qw < 20 {
		qw = 20
	}
	for i, s := range m.sessions {
		line := cell(fmt.Sprintf("%d", s.PID), 8) +
			cell(s.User, 12) + cell(s.Database, 12) +
			cell(s.State, 12) + cell(s.Wait, 16) +
			cell(formatAgo(s.Duration), 7) + truncate(s.Query, qw)
		if i == m.row && m.focus == focusMain {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.sessions) == 0 {
		lines = append(lines, mutedStyle.Render("no sessions, press r to refresh"))
	}
	return lines
}

func (m Model) blocksLines(width int) []string {
	lines := []string{
		titleStyle.Render(fmt.Sprintf("locks  (%d blocked)", len(m.blocks))),
		mutedStyle.Render(
			cell("BLOCKED", 9) + cell("BY", 9) + cell("LOCK", 18) + "BLOCKED QUERY",
		),
	}
	qw := width - 9 - 9 - 18 - 8
	if qw < 20 {
		qw = 20
	}
	for i, b := range m.blocks {
		line := cell(fmt.Sprintf("%d", b.BlockedPID), 9) +
			cell(fmt.Sprintf("%d", b.BlockingPID), 9) +
			cell(b.LockType+" "+b.Mode, 18) + truncate(b.BlockedQuery, qw)
		if i == m.row && m.focus == focusMain {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.blocks) == 0 {
		lines = append(lines, mutedStyle.Render("nothing blocked"))
	}
	return lines
}

func (m Model) tablesLines(width int) []string {
	lines := []string{
		titleStyle.Render(fmt.Sprintf("tables  (%d)", len(m.tables))),
		mutedStyle.Render(
			cell("SCHEMA", 10) + cell("TABLE", 28) + cell("SIZE", 10) +
				cell("LIVE", 10) + cell("DEAD", 10) + "LAST VACUUM",
		),
	}
	_ = width
	for i, t := range m.tables {
		line := cell(t.Schema, 10) + cell(t.Name, 28) +
			cell(formatBytes(t.SizeBytes), 10) +
			cell(fmt.Sprintf("%d", t.LiveTuples), 10) +
			cell(fmt.Sprintf("%d", t.DeadTuples), 10) +
			formatStamp(t.LastVacuum)
		if i == m.row && m.focus == focusMain {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if len(m.tables) == 0 {
		lines = append(lines, mutedStyle.Render("no tables, press r to refresh"))
	}
	return lines
}

func (m Model) sqlLines(width int) []string {
	lines := []string{titleStyle.Render("sql")}
	if m.editing {
		lines = append(lines, selectedStyle.Render("> ")+m.sqlInput.View())
	} else {
		lines = append(lines, mutedStyle.Render("  "+m.sqlInput.View()+"  ·  i to edit"))
	}
	if len(m.favs) > 0 {
		shown := m.favs
		if len(shown) > 3 {
			shown = shown[len(shown)-3:]
		}
		for _, f := range shown {
			lines = append(lines, mutedStyle.Render("  "+f.Name+": "+truncate(f.Query, width-10)+"  ·  o to load"))
		}
	}
	lines = append(lines, "")
	if m.lastQuery == "" {
		lines = append(lines, mutedStyle.Render("top queries by total time, run one to replace this view"))
		lines = append(lines, "")
		lines = append(lines, m.statementsLines(width)...)
		return lines
	}
	header := make([]string, len(m.sqlCols))
	for i, c := range m.sqlCols {
		header[i] = cell(c, 18)
	}
	lines = append(lines, mutedStyle.Render(strings.Join(header, "")))
	shown := m.sqlRows
	extra := 0
	if len(shown) > 30 {
		extra = len(shown) - 30
		shown = shown[:30]
	}
	for i, row := range shown {
		cells := make([]string, len(row))
		for j, v := range row {
			cells[j] = cell(v, 18)
		}
		line := strings.Join(cells, "")
		if i == m.row && m.focus == focusMain {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	if extra > 0 {
		lines = append(lines, mutedStyle.Render(fmt.Sprintf("+%d more rows, e to export all", extra)))
	}
	if len(m.sqlRows) == 0 {
		lines = append(lines, mutedStyle.Render("query returned no rows"))
	}
	return lines
}

func (m Model) redisLines(width int) []string {
	lines := []string{titleStyle.Render("redis")}
	if m.rinfo == nil {
		lines = append(lines, "", mutedStyle.Render("no data, press r to refresh"))
		return lines
	}
	info := m.rinfo
	hit := "-"
	if info.HitRate >= 0 {
		hit = fmt.Sprintf("%.1f%%", info.HitRate)
	}
	lines = append(lines,
		fmt.Sprintf("version %-10s role %-8s mode %s", info.Version, info.Role, info.Mode),
		fmt.Sprintf("uptime  %-10s clients %-4d dbsize %d", formatAgo(info.Uptime), info.Clients, info.DBSize),
		fmt.Sprintf("memory  %-10s ops/s %-6d hitrate %s", info.UsedHuman, info.OpsPerSec, hit),
		"",
		mutedStyle.Render(fmt.Sprintf("slowlog (%d)", len(m.slow))),
		mutedStyle.Render(cell("ID", 8)+cell("AT", 10)+cell("TOOK", 10)+"COMMAND"),
	)
	for i, s := range m.slow {
		line := cell(fmt.Sprintf("%d", s.ID), 8) +
			cell(s.At.Format("15:04:05"), 10) +
			cell(s.Duration.Round(time.Microsecond).String(), 10) +
			truncate(strings.Join(s.Args, " "), width-36)
		if i == m.row && m.focus == focusMain {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
		if i >= 19 {
			lines = append(lines, mutedStyle.Render(fmt.Sprintf("+%d more, e to export", len(m.slow)-20)))
			break
		}
	}
	if len(m.slow) == 0 {
		lines = append(lines, mutedStyle.Render("slowlog is empty"))
	}
	lines = append(lines, "", mutedStyle.Render(fmt.Sprintf("keys (%d sampled)", len(m.rkeys))))
	shown := m.rkeys
	if len(shown) > 8 {
		shown = shown[:8]
	}
	for _, k := range shown {
		ttl := "persist"
		if k.TTLSecs == -2 {
			ttl = "gone"
		} else if k.TTLSecs >= 0 {
			ttl = formatAgo(time.Duration(k.TTLSecs) * time.Second)
		}
		lines = append(lines, "  "+cell(k.Type, 10)+cell(ttl, 10)+truncate(k.Name, width-28))
	}
	return lines
}

func (m Model) statementsLines(width int) []string {
	lines := []string{titleStyle.Render(fmt.Sprintf("statements  (%d)", len(m.statements)))}
	if !m.statementsOK && len(m.statements) == 0 {
		lines = append(lines, "")
		lines = append(lines, mutedStyle.Render("pg_stat_statements is not available"))
		lines = append(lines, mutedStyle.Render("CREATE EXTENSION pg_stat_statements to enable this view"))
		return lines
	}
	lines = append(lines, mutedStyle.Render(
		cell("CALLS", 10)+cell("MEAN MS", 10)+cell("TOTAL MS", 11)+cell("ROWS", 10)+"QUERY",
	))
	qw := width - 10 - 10 - 11 - 10 - 8
	if qw < 20 {
		qw = 20
	}
	for i, s := range m.statements {
		line := cell(fmt.Sprintf("%d", s.Calls), 10) +
			cell(fmt.Sprintf("%.1f", s.MeanMs), 10) +
			cell(fmt.Sprintf("%.0f", s.TotalMs), 11) +
			cell(fmt.Sprintf("%d", s.Rows), 10) + truncate(s.Query, qw)
		if i == m.row && m.focus == focusMain {
			line = selectedStyle.Render(line)
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) renderDetail() string {
	lines := []string{titleStyle.Render("detail"), ""}
	switch m.active {
	case 0:
		if m.row < len(m.sessions) {
			s := m.sessions[m.row]
			lines = append(lines,
				selectedStyle.Render(fmt.Sprintf("pid %d", s.PID)),
				"user  "+s.User,
				"db    "+s.Database,
				"state "+s.State,
				"wait  "+s.Wait,
				"dur   "+formatAgo(s.Duration),
				"",
				truncate(s.Query, 200),
			)
		} else {
			lines = append(lines, mutedStyle.Render("nothing selected"))
		}
	case 1:
		if m.row < len(m.blocks) {
			b := m.blocks[m.row]
			lines = append(lines,
				selectedStyle.Render(fmt.Sprintf("%d blocked by %d", b.BlockedPID, b.BlockingPID)),
				"lock  "+b.LockType+" "+b.Mode,
				"",
				truncate(b.BlockedQuery, 200),
				"",
				mutedStyle.Render("blocking:"),
				truncate(b.BlockingQuery, 200),
			)
		} else {
			lines = append(lines, mutedStyle.Render("nothing blocked"))
		}
	case 2:
		if m.row < len(m.tables) {
			t := m.tables[m.row]
			lines = append(lines,
				selectedStyle.Render(t.Schema+"."+t.Name),
				"size  "+formatBytes(t.SizeBytes),
				fmt.Sprintf("live  %d", t.LiveTuples),
				fmt.Sprintf("dead  %d", t.DeadTuples),
				"vac   "+formatStamp(t.LastVacuum),
				"auto  "+formatStamp(t.LastAutoVacuum),
			)
		} else {
			lines = append(lines, mutedStyle.Render("nothing selected"))
		}
	case 3:
		if m.lastQuery != "" {
			if m.row < len(m.sqlRows) {
				row := m.sqlRows[m.row]
				lines = append(lines, selectedStyle.Render(fmt.Sprintf("row %d", m.row+1)))
				for i, v := range row {
					name := fmt.Sprintf("col%d", i+1)
					if i < len(m.sqlCols) {
						name = m.sqlCols[i]
					}
					lines = append(lines, name+"  "+truncate(v, 60))
				}
			} else {
				lines = append(lines, mutedStyle.Render("nothing selected"))
			}
			break
		}
		if m.row < len(m.statements) {
			s := m.statements[m.row]
			lines = append(lines,
				selectedStyle.Render(fmt.Sprintf("%d calls", s.Calls)),
				fmt.Sprintf("mean  %.1f ms", s.MeanMs),
				fmt.Sprintf("total %.0f ms", s.TotalMs),
				"",
				truncate(s.Query, 200),
			)
		} else {
			lines = append(lines, mutedStyle.Render("nothing selected"))
		}
	default:
		if m.active == 4 {
			if m.row < len(m.slow) {
				s := m.slow[m.row]
				lines = append(lines,
					selectedStyle.Render(fmt.Sprintf("slowlog %d", s.ID)),
					"at    "+s.At.Format(time.RFC3339),
					"took  "+s.Duration.String(),
					"",
				)
				for _, a := range s.Args {
					lines = append(lines, truncate(a, 60))
				}
			} else {
				lines = append(lines, mutedStyle.Render("nothing selected"))
			}
		} else {
			lines = append(lines, mutedStyle.Render("nothing selected"))
		}
	}
	return panelStyle.Width(28).Render(strings.Join(lines, "\n"))
}

func (m Model) renderFooter() string {
	if m.active == 3 {
		if m.editing {
			return mutedStyle.Render("enter run · esc stop editing")
		}
		return mutedStyle.Render("i edit · o favorite · f save · x clear · e export · r refresh · ? help · q quit")
	}
	return mutedStyle.Render("j/k move · tab focus · 1-5 tabs · enter open · e export · r refresh · ? help · q quit")
}

func (m Model) renderHelp() string {
	lines := []string{
		titleStyle.Render("prodtop keys"),
		"",
		"  j / down      move down",
		"  k / up        move up",
		"  tab           switch between services and main",
		"  g / G         first / last row",
		"  1-5           switch tab",
		"  enter         open service and refresh",
		"  r             refresh current view",
		"  i             edit sql (sql tab)",
		"  o             load next favorite (sql tab)",
		"  f             save query as favorite (sql tab)",
		"  x             clear results (sql tab)",
		"  e             export current view to csv",
		"  /             filter (next milestone)",
		"  ?             this screen",
		"  q             quit",
		"",
		mutedStyle.Render("press any key to close"),
	}
	return strings.Join(lines, "\n")
}
