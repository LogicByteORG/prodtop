package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireWrite(t *testing.T) {
	ro := Guard{}
	if err := ro.RequireWrite("terminate backend 1"); err == nil {
		t.Error("RequireWrite(read-only) = nil, want error")
	} else if !strings.Contains(err.Error(), "--write") {
		t.Errorf("RequireWrite error = %q, want --write hint", err.Error())
	}
	rw := Guard{Write: true}
	if err := rw.RequireWrite("terminate backend 1"); err != nil {
		t.Errorf("RequireWrite(write) = %v, want nil", err)
	}
}

func TestRecordAppendsJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.jsonl")
	g := Guard{Write: true, AuditPath: path}
	if err := g.Record("terminate-backend", "pid 42 on demo"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	line := string(data)
	if !strings.Contains(line, "terminate-backend") || !strings.Contains(line, "pid 42 on demo") {
		t.Errorf("audit line = %q, want action and detail", line)
	}
}

func TestRecordWithoutPathIsNoop(t *testing.T) {
	g := Guard{}
	if err := g.Record("anything", "detail"); err != nil {
		t.Errorf("Record(no path) = %v, want nil", err)
	}
}
