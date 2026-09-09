package postgres

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxRows     = 500
	MaxCellRune = 200
)

type Client struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

type Session struct {
	PID      int64
	User     string
	Database string
	State    string
	Wait     string
	Duration time.Duration
	Query    string
}

type Block struct {
	BlockedPID    int64
	BlockingPID   int64
	BlockedQuery  string
	BlockingQuery string
	LockType      string
	Mode          string
}

type TableStat struct {
	Schema         string
	Name           string
	SizeBytes      int64
	LiveTuples     int64
	DeadTuples     int64
	LastVacuum     *time.Time
	LastAutoVacuum *time.Time
}

type Statement struct {
	Query   string
	Calls   int64
	MeanMs  float64
	TotalMs float64
	Rows    int64
}

func ConnectLazy(dsn string, timeout time.Duration) (*Client, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres dsn: %w", err)
	}
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}
	return &Client{pool: pool, timeout: timeout}, nil
}

func (c *Client) Close() {
	c.pool.Close()
}

func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, c.timeout)
}

func strptr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (c *Client) Activity(ctx context.Context) ([]Session, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	rows, err := c.pool.Query(ctx, `
SELECT pid, usename, datname, state, wait_event, query_start, query
FROM pg_stat_activity
WHERE datname IS NOT NULL AND pid <> pg_backend_pid()
ORDER BY query_start NULLS LAST
LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("pg_stat_activity: %w", err)
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		var user, db, state, wait, query *string
		var started *time.Time
		if err := rows.Scan(&s.PID, &user, &db, &state, &wait, &started, &query); err != nil {
			return nil, fmt.Errorf("scan session: %w", err)
		}
		s.User = strptr(user)
		s.Database = strptr(db)
		s.State = strptr(state)
		s.Wait = strptr(wait)
		s.Query = strptr(query)
		if started != nil {
			s.Duration = time.Since(*started).Round(time.Second)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (c *Client) Blocking(ctx context.Context) ([]Block, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	rows, err := c.pool.Query(ctx, `
SELECT blocked_act.pid, blocking_act.pid,
  blocked_act.query, blocking_act.query,
  blocked_locks.locktype, blocked_locks.mode
FROM pg_locks blocked_locks
JOIN pg_stat_activity blocked_act ON blocked_act.pid = blocked_locks.pid
JOIN pg_locks blocking_locks
  ON blocking_locks.locktype = blocked_locks.locktype
  AND blocking_locks.database IS NOT DISTINCT FROM blocked_locks.database
  AND blocking_locks.relation IS NOT DISTINCT FROM blocked_locks.relation
  AND blocking_locks.page IS NOT DISTINCT FROM blocked_locks.page
  AND blocking_locks.tuple IS NOT DISTINCT FROM blocked_locks.tuple
  AND blocking_locks.virtualxid IS NOT DISTINCT FROM blocked_locks.virtualxid
  AND blocking_locks.transactionid IS NOT DISTINCT FROM blocked_locks.transactionid
  AND blocking_locks.classid IS NOT DISTINCT FROM blocked_locks.classid
  AND blocking_locks.objid IS NOT DISTINCT FROM blocked_locks.objid
  AND blocking_locks.objsubid IS NOT DISTINCT FROM blocked_locks.objsubid
  AND blocking_locks.pid != blocked_locks.pid
JOIN pg_stat_activity blocking_act ON blocking_act.pid = blocking_locks.pid
WHERE NOT blocked_locks.granted`)
	if err != nil {
		return nil, fmt.Errorf("blocking locks: %w", err)
	}
	defer rows.Close()
	var out []Block
	for rows.Next() {
		var b Block
		var blockedQ, blockingQ *string
		if err := rows.Scan(&b.BlockedPID, &b.BlockingPID, &blockedQ, &blockingQ, &b.LockType, &b.Mode); err != nil {
			return nil, fmt.Errorf("scan block: %w", err)
		}
		b.BlockedQuery = strptr(blockedQ)
		b.BlockingQuery = strptr(blockingQ)
		out = append(out, b)
	}
	return out, rows.Err()
}

func (c *Client) Tables(ctx context.Context) ([]TableStat, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	rows, err := c.pool.Query(ctx, `
SELECT schemaname, relname, pg_total_relation_size(relid),
  n_live_tup, n_dead_tup, last_vacuum, last_autovacuum
FROM pg_stat_user_tables
ORDER BY pg_total_relation_size(relid) DESC
LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("table stats: %w", err)
	}
	defer rows.Close()
	var out []TableStat
	for rows.Next() {
		var t TableStat
		if err := rows.Scan(&t.Schema, &t.Name, &t.SizeBytes, &t.LiveTuples, &t.DeadTuples, &t.LastVacuum, &t.LastAutoVacuum); err != nil {
			return nil, fmt.Errorf("scan table stat: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (c *Client) Statements(ctx context.Context) ([]Statement, bool, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	rows, err := c.pool.Query(ctx, `
SELECT query, calls, mean_exec_time, total_exec_time, rows
FROM pg_stat_statements
ORDER BY total_exec_time DESC
LIMIT 100`)
	if err != nil {
		return nil, false, nil
	}
	defer rows.Close()
	var out []Statement
	for rows.Next() {
		var s Statement
		if err := rows.Scan(&s.Query, &s.Calls, &s.MeanMs, &s.TotalMs, &s.Rows); err != nil {
			return nil, true, fmt.Errorf("scan statement: %w", err)
		}
		out = append(out, s)
	}
	return out, true, rows.Err()
}

func (c *Client) Terminate(ctx context.Context, pid int64) (bool, error) {
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	var terminated bool
	if err := c.pool.QueryRow(ctx, `SELECT pg_terminate_backend($1)`, pid).Scan(&terminated); err != nil {
		return false, fmt.Errorf("terminate backend %d: %w", pid, err)
	}
	return terminated, nil
}

var readVerbs = map[string]bool{
	"SELECT": true, "WITH": true, "EXPLAIN": true,
	"SHOW": true, "VALUES": true, "TABLE": true,
}

var writeVerbs = map[string]bool{
	"INSERT": true, "UPDATE": true, "DELETE": true,
}

func AllowedStatement(query string, allowWrite bool) error {
	verb := firstVerb(query)
	if verb == "" {
		return fmt.Errorf("empty query")
	}
	if readVerbs[verb] {
		return nil
	}
	if allowWrite && writeVerbs[verb] {
		return nil
	}
	return fmt.Errorf("statement %q is not allowed here", verb)
}

func firstVerb(query string) string {
	rest := strings.TrimSpace(query)
	for {
		if strings.HasPrefix(rest, "--") {
			if i := strings.IndexByte(rest, '\n'); i >= 0 {
				rest = strings.TrimSpace(rest[i+1:])
				continue
			}
			return ""
		}
		if strings.HasPrefix(rest, "/*") {
			end := strings.Index(rest, "*/")
			if end < 0 {
				return ""
			}
			rest = strings.TrimSpace(rest[end+2:])
			continue
		}
		break
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	verb := strings.ToUpper(fields[0])
	return strings.Trim(verb, "();")
}

func (c *Client) RunQuery(ctx context.Context, query string, allowWrite bool) ([]string, [][]string, error) {
	if err := AllowedStatement(query, allowWrite); err != nil {
		return nil, nil, err
	}
	ctx, cancel := c.withTimeout(ctx)
	defer cancel()
	rows, err := c.pool.Query(ctx, query)
	if err != nil {
		return nil, nil, fmt.Errorf("run query: %w", err)
	}
	defer rows.Close()
	fields := rows.FieldDescriptions()
	cols := make([]string, len(fields))
	for i, f := range fields {
		cols[i] = f.Name
	}
	var out [][]string
	for rows.Next() && len(out) < MaxRows {
		vals, err := rows.Values()
		if err != nil {
			return nil, nil, fmt.Errorf("read row: %w", err)
		}
		rec := make([]string, len(vals))
		for i, v := range vals {
			rec[i] = cellText(v)
		}
		out = append(out, rec)
	}
	return cols, out, rows.Err()
}

func cellText(v any) string {
	var s string
	switch t := v.(type) {
	case nil:
		return "NULL"
	case string:
		s = t
	case []byte:
		s = string(t)
	case time.Time:
		s = t.Format(time.RFC3339)
	default:
		s = fmt.Sprintf("%v", t)
	}
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > MaxCellRune {
		return string(runes[:MaxCellRune-1]) + "…"
	}
	return s
}
