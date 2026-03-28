package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"panelofexperts/internal/model"
)

type Store struct {
	Root string
	mu   sync.Mutex
}

func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Store{Root: root}, nil
}

func (s *Store) SaveJSON(rel string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (s *Store) SaveText(rel, content string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.Root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (s *Store) AppendEvent(event model.ProgressEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.Root, "events.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

func (s *Store) SaveState(run model.RunState) error {
	return s.SaveJSON("state.json", run)
}
