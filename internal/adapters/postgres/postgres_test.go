package postgres

import (
	"testing"
	"time"
)

func TestConnectLazyRejectsBadDSN(t *testing.T) {
	if _, err := ConnectLazy("://missing-scheme", time.Second); err == nil {
		t.Error("ConnectLazy(bad dsn) = nil, want error")
	}
}

func TestAllowedStatement(t *testing.T) {
	cases := []struct {
		query      string
		allowWrite bool
		allowed    bool
	}{
		{"SELECT 1", false, true},
		{"  select * from users limit 5", false, true},
		{"WITH x AS (SELECT 1) SELECT * FROM x", false, true},
		{"EXPLAIN SELECT 1", false, true},
		{"SHOW server_version", false, true},
		{"-- count active\nSELECT count(*) FROM pg_stat_activity", false, true},
		{"/* nightly */ SELECT 1", false, true},
		{"SeLeCt 1", false, true},
		{"", false, false},
		{"   ", false, false},
		{"DROP TABLE users", false, false},
		{"DROP TABLE users", true, false},
		{"INSERT INTO t VALUES (1)", false, false},
		{"INSERT INTO t VALUES (1)", true, true},
		{"UPDATE t SET a = 1", false, false},
		{"UPDATE t SET a = 1", true, true},
		{"DELETE FROM t", false, false},
		{"DELETE FROM t", true, true},
		{"VACUUM users", true, false},
	}
	for _, tc := range cases {
		err := AllowedStatement(tc.query, tc.allowWrite)
		if tc.allowed && err != nil {
			t.Errorf("AllowedStatement(%q, %v) = %v, want nil", tc.query, tc.allowWrite, err)
		}
		if !tc.allowed && err == nil {
			t.Errorf("AllowedStatement(%q, %v) = nil, want error", tc.query, tc.allowWrite)
		}
	}
}
