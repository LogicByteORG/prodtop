package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Guard struct {
	Write     bool
	AuditPath string
}

func DefaultAuditPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "prodtop", "audit.jsonl")
}

func (g Guard) RequireWrite(action string) error {
	if g.Write {
		return nil
	}
	return fmt.Errorf("refusing %s: prodtop runs read-only without --write", action)
}

type auditEntry struct {
	At     string `json:"at"`
	Action string `json:"action"`
	Detail string `json:"detail"`
}

func (g Guard) Record(action, detail string) error {
	if g.AuditPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(g.AuditPath), 0o755); err != nil {
		return fmt.Errorf("create audit dir: %w", err)
	}
	entry := auditEntry{
		At:     time.Now().UTC().Format(time.RFC3339),
		Action: action,
		Detail: detail,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encode audit entry: %w", err)
	}
	f, err := os.OpenFile(g.AuditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit log: %w", err)
	}
	return nil
}
