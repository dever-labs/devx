//go:build !windows

package process

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStartLogsStop(t *testing.T) {
	s := New(t.TempDir())
	rec, started, err := s.Start(Spec{Name: "web", Run: "echo hello-$FOO; sleep 30", Env: map[string]string{"FOO": "bar"}})
	if err != nil || !started {
		t.Fatalf("start: %v started=%v", err, started)
	}
	if !s.Alive("web") {
		t.Fatal("expected alive")
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(strings.Join(s.Tail("web", 10), "\n"), "hello-bar") {
		time.Sleep(50 * time.Millisecond)
	}
	if got := s.Tail("web", 10); len(got) == 0 || got[0] != "hello-bar" {
		t.Fatalf("unexpected log: %v", got)
	}

	// Same command: idempotent. Different command: refused.
	if r2, started, err := s.Start(Spec{Name: "web", Run: "echo hello-$FOO; sleep 30"}); err != nil || started || r2.PID != rec.PID {
		t.Fatalf("expected no-op restart, got started=%v err=%v", started, err)
	}
	if _, _, err := s.Start(Spec{Name: "web", Run: "sleep 1"}); err == nil {
		t.Fatal("expected error for different command")
	}

	if err := s.Stop("web", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if alive(rec.PID) {
		t.Fatal("process should be gone")
	}
	if len(s.List()) != 0 {
		t.Fatal("record should be removed")
	}
}

func TestStopKillsChildren(t *testing.T) {
	s := New(t.TempDir())
	rec, _, err := s.Start(Spec{Name: "tree", Run: "sleep 60 & sleep 60"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := s.Stop("tree", time.Second); err != nil {
		t.Fatal(err)
	}
	if alive(rec.PID) {
		t.Fatal("leader should be dead")
	}
}

func TestExitedProcessNotAlive(t *testing.T) {
	s := New(t.TempDir())
	if _, _, err := s.Start(Spec{Name: "quick", Command: []string{"true"}}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && s.Alive("quick") {
		time.Sleep(50 * time.Millisecond)
	}
	if s.Alive("quick") {
		t.Fatal("expected exited process to be reported dead")
	}
}

func TestFollowNoFollow(t *testing.T) {
	s := New(t.TempDir())
	if _, _, err := s.Start(Spec{Name: "l", Run: "printf 'a\\nb\\n'"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	var lines []string
	if err := s.Follow(context.Background(), "l", false, func(l string) { lines = append(lines, l) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(lines, ",") != "a,b" {
		t.Fatalf("got %v", lines)
	}
}

func TestExecExitCode(t *testing.T) {
	code, err := Exec(context.Background(), Spec{Name: "x"}, []string{"sh", "-c", "exit 3"})
	if err != nil || code != 3 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}
