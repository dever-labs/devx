package task

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadState_MissingFile(t *testing.T) {
	s, err := LoadState(filepath.Join(t.TempDir(), "tasks.json"))
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Entries == nil || len(s.Entries) != 0 {
		t.Errorf("expected an empty non-nil Entries map, got %v", s.Entries)
	}
}

func TestLoadState_EmptyPath(t *testing.T) {
	s, err := LoadState("")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if s.Entries == nil {
		t.Error("expected a non-nil Entries map")
	}
}

func TestLoadState_MalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Fatal("expected an error for malformed tasks.json")
	}
}

func TestStateStore_SaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "tasks.json")

	s, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	want := StateEntry{
		ContainerName: "my-app-db",
		IsContainer:   true,
		LogPath:       "/tmp/db.log",
		StartedAt:     time.Now().Truncate(time.Second),
		PID:           1234,
	}
	s.Record("db", want)

	if err := s.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected Save to create parent dirs and the file: %v", err)
	}

	reloaded, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState after save: %v", err)
	}
	got, ok := reloaded.Lookup("db")
	if !ok {
		t.Fatal("expected a 'db' entry after reload")
	}
	if got.ContainerName != want.ContainerName || got.IsContainer != want.IsContainer ||
		got.LogPath != want.LogPath || got.PID != want.PID || !got.StartedAt.Equal(want.StartedAt) {
		t.Errorf("reloaded entry = %+v, want %+v", got, want)
	}
}

func TestStateStore_SaveEmptyPathIsNoop(t *testing.T) {
	s, _ := LoadState("")
	s.Record("x", StateEntry{PID: 1})
	if err := s.Save(""); err != nil {
		t.Fatalf("Save(\"\") should be a no-op, got %v", err)
	}
}

func TestStateStore_RemoveAndLookup(t *testing.T) {
	s, _ := LoadState("")
	s.Record("api", StateEntry{PID: 42})

	if _, ok := s.Lookup("api"); !ok {
		t.Fatal("expected to find 'api' after Record")
	}
	s.Remove("api")
	if _, ok := s.Lookup("api"); ok {
		t.Fatal("expected 'api' to be gone after Remove")
	}
	// Removing an already-absent entry must not panic or error.
	s.Remove("api")
}

func TestStateStore_All_ReturnsSnapshotCopy(t *testing.T) {
	s, _ := LoadState("")
	s.Record("a", StateEntry{PID: 1})
	s.Record("b", StateEntry{PID: 2})

	all := s.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	// Mutating the returned map must not affect the store.
	delete(all, "a")
	if _, ok := s.Lookup("a"); !ok {
		t.Error("All() should return a copy — mutating it affected the store")
	}
}
