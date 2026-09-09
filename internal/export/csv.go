package export

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
)

func WriteCSV(path string, header []string, rows [][]string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create export dir: %w", err)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create export file: %w", err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return fmt.Errorf("write csv header: %w", err)
	}
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("write csv rows: %w", err)
	}
	w.Flush()
	return w.Error()
}
