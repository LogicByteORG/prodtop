package favorites

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const MaxFavorites = 50

type Entry struct {
	Name  string `yaml:"name"`
	Query string `yaml:"query"`
}

func Path() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "favorites.yaml"
	}
	return filepath.Join(home, ".config", "prodtop", "favorites.yaml")
}

func Load() ([]Entry, error) {
	return LoadPath(Path())
}

func LoadPath(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read favorites: %w", err)
	}
	var entries []Entry
	if err := yaml.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse favorites: %w", err)
	}
	return entries, nil
}

func Add(query string) (Entry, error) {
	return AddTo(Path(), query)
}

func AddTo(path, query string) (Entry, error) {
	entries, err := LoadPath(path)
	if err != nil {
		return Entry{}, err
	}
	if query == "" {
		return Entry{}, fmt.Errorf("cannot save empty query")
	}
	entry := Entry{Name: fmt.Sprintf("q%d", len(entries)+1), Query: query}
	entries = append(entries, entry)
	if len(entries) > MaxFavorites {
		entries = entries[len(entries)-MaxFavorites:]
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Entry{}, fmt.Errorf("create favorites dir: %w", err)
		}
	}
	data, err := yaml.Marshal(entries)
	if err != nil {
		return Entry{}, fmt.Errorf("encode favorites: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Entry{}, fmt.Errorf("write favorites: %w", err)
	}
	return entry, nil
}
