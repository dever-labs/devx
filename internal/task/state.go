package task

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StateEntry is one task's persisted runtime record, letting a later
// `devx status`/`logs`/`exec`/`down` invocation reconstruct what's
// running without holding any live process handle across separate CLI
// invocations (a plain process can't be re-attached to after `up -d`
// exits — only its log file and, for container-shaped tasks, its name
// survive the process boundary).
type StateEntry struct {
	ContainerName string    `json:"containerName,omitempty"`
	IsContainer   bool      `json:"isContainer"`
	LogPath       string    `json:"logPath"`
	StartedAt     time.Time `json:"startedAt"`
	PID           int       `json:"pid,omitempty"`
}

// StateStore is the in-memory + on-disk (.devx/tasks.json) record of
// every detached task started by this project.
type StateStore struct {
	mu      sync.Mutex
	Entries map[string]StateEntry `json:"entries"`
}

// LoadState reads path's existing state, or returns an empty store if it
// doesn't exist yet.
func LoadState(path string) (*StateStore, error) {
	s := &StateStore{Entries: map[string]StateEntry{}}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, err
	}
	if s.Entries == nil {
		s.Entries = map[string]StateEntry{}
	}
	return s, nil
}

// Record stores (or replaces) a task's entry.
func (s *StateStore) Record(name string, e StateEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Entries[name] = e
}

// Remove deletes a task's entry (after devx down/workspace rm).
func (s *StateStore) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Entries, name)
}

// Lookup returns a task's stored entry, if any.
func (s *StateStore) Lookup(name string) (StateEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.Entries[name]
	return e, ok
}

// All returns a snapshot copy of every entry.
func (s *StateStore) All() map[string]StateEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]StateEntry, len(s.Entries))
	for k, v := range s.Entries {
		out[k] = v
	}
	return out
}

// Save persists the store to path (creating parent directories as
// needed). A no-op when path is empty.
func (s *StateStore) Save(path string) error {
	if path == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
