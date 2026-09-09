package redis

import (
	"testing"
	"time"
)

const sampleINFO = "# Server\r\n" +
	"redis_version:7.2.4\r\n" +
	"redis_mode:standalone\r\n" +
	"uptime_in_seconds:90061\r\n" +
	"# Clients\r\n" +
	"connected_clients:12\r\n" +
	"# Memory\r\n" +
	"used_memory:1048576\r\n" +
	"used_memory_human:1.00M\r\n" +
	"# Stats\r\n" +
	"keyspace_hits:80\r\n" +
	"keyspace_misses:20\r\n" +
	"instantaneous_ops_per_sec:42\r\n" +
	"# Replication\r\n" +
	"role:master\r\n"

func TestParseSection(t *testing.T) {
	fields := parseSection(sampleINFO)
	if fields["redis_version"] != "7.2.4" {
		t.Errorf("version = %q, want 7.2.4", fields["redis_version"])
	}
	if fields["role"] != "master" {
		t.Errorf("role = %q, want master", fields["role"])
	}
	if _, ok := fields["# Server"]; ok {
		t.Error("comment line leaked into fields")
	}
	if number(fields, "connected_clients") != 12 {
		t.Errorf("clients = %d, want 12", number(fields, "connected_clients"))
	}
}

func TestHitRate(t *testing.T) {
	if got := hitRate(80, 20); got != 80 {
		t.Errorf("hitRate(80,20) = %v, want 80", got)
	}
	if got := hitRate(0, 0); got != -1 {
		t.Errorf("hitRate(0,0) = %v, want -1", got)
	}
}

func TestConnectLazyRejectsEmptyAddr(t *testing.T) {
	if _, err := ConnectLazy("  ", "", 0, time.Second); err == nil {
		t.Error("ConnectLazy(empty) = nil, want error")
	}
}
