// Package ready provides HTTP and TCP readiness probes shared by every
// service kind (container or host process).
package ready

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"
)

const (
	DefaultInterval = 2 * time.Second
	DefaultTimeout  = 2 * time.Minute
	probeTimeout    = 2 * time.Second
)

// Probe describes how to decide a service is ready. HTTP and TCP may both be
// set; every configured check must pass.
type Probe struct {
	Name     string
	HTTP     string
	TCP      string
	Interval time.Duration
	Timeout  time.Duration
	// Alive, when set, lets Wait fail fast if the service died instead of
	// waiting out the full timeout.
	Alive func() bool
}

// Empty reports whether the probe has nothing to check.
func (p Probe) Empty() bool { return p.HTTP == "" && p.TCP == "" }

// Check runs each configured check once.
func (p Probe) Check(ctx context.Context) bool {
	if p.HTTP != "" && !CheckHTTP(ctx, p.HTTP) {
		return false
	}
	if p.TCP != "" && !CheckTCP(ctx, p.TCP) {
		return false
	}
	return true
}

func (p Probe) describe() string {
	var parts []string
	if p.HTTP != "" {
		parts = append(parts, p.HTTP)
	}
	if p.TCP != "" {
		parts = append(parts, "tcp://"+p.TCP)
	}
	return strings.Join(parts, " + ")
}

// CheckHTTP returns true when url answers with a 2xx status.
func CheckHTTP(ctx context.Context, url string) bool {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

// CheckTCP returns true when addr (host:port) accepts a connection.
func CheckTCP(ctx context.Context, addr string) bool {
	d := net.Dialer{Timeout: probeTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// Wait polls all probes until they pass or their timeouts elapse. The error
// lists every probe that never became ready.
func Wait(ctx context.Context, probes []Probe) error {
	type pending struct {
		probe    Probe
		deadline time.Time
	}
	now := time.Now()
	var waiting []pending
	for _, p := range probes {
		if p.Empty() {
			continue
		}
		timeout := p.Timeout
		if timeout <= 0 {
			timeout = DefaultTimeout
		}
		waiting = append(waiting, pending{probe: p, deadline: now.Add(timeout)})
	}

	var failed []string
	for len(waiting) > 0 {
		var next []pending
		for _, w := range waiting {
			switch {
			case w.probe.Check(ctx):
			case w.probe.Alive != nil && !w.probe.Alive():
				failed = append(failed, fmt.Sprintf("%s (process exited before becoming ready)", w.probe.Name))
			case time.Now().After(w.deadline):
				failed = append(failed, fmt.Sprintf("%s (%s)", w.probe.Name, w.probe.describe()))
			default:
				next = append(next, w)
			}
		}
		waiting = next
		if len(waiting) == 0 {
			break
		}
		interval := DefaultInterval
		for _, w := range waiting {
			if w.probe.Interval > 0 && w.probe.Interval < interval {
				interval = w.probe.Interval
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}

	if len(failed) > 0 {
		sort.Strings(failed)
		return fmt.Errorf("readiness checks failed: %s", strings.Join(failed, ", "))
	}
	return nil
}
