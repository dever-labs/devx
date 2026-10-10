package ready

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWaitHTTPAndTCP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	err = Wait(context.Background(), []Probe{
		{Name: "web", HTTP: srv.URL, TCP: ln.Addr().String(), Interval: 10 * time.Millisecond},
		{Name: "noop"},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWaitBecomesReady(t *testing.T) {
	start := time.Now().Add(150 * time.Millisecond)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if time.Now().Before(start) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer srv.Close()
	if err := Wait(context.Background(), []Probe{{Name: "slow", HTTP: srv.URL, Interval: 20 * time.Millisecond, Timeout: 5 * time.Second}}); err != nil {
		t.Fatal(err)
	}
}

func TestWaitTimeout(t *testing.T) {
	err := Wait(context.Background(), []Probe{{Name: "dead", TCP: "127.0.0.1:1", Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond}})
	if err == nil || !strings.Contains(err.Error(), "dead") {
		t.Fatalf("expected failure naming probe, got %v", err)
	}
}

func TestWaitContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Wait(ctx, []Probe{{Name: "x", TCP: "127.0.0.1:1", Timeout: time.Minute}}); err == nil {
		t.Fatal("expected context error")
	}
}

func TestWaitFailsFastWhenDead(t *testing.T) {
	start := time.Now()
	err := Wait(context.Background(), []Probe{{Name: "gone", TCP: "127.0.0.1:1", Timeout: time.Minute, Alive: func() bool { return false }}})
	if err == nil || !strings.Contains(err.Error(), "exited") || time.Since(start) > 5*time.Second {
		t.Fatalf("expected fast failure, got %v after %v", err, time.Since(start))
	}
}
