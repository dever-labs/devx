package task

import (
	"context"
	"net"
	"testing"

	"github.com/dever-labs/devx/internal/config"
)

// TestSwapHostWithIP covers the host/port rewriting swapHostWithIP does
// for both a bare TCP address and an HTTP URL, with and without a known
// container-internal port override.
func TestSwapHostWithIP(t *testing.T) {
	cases := []struct {
		name string
		addr string
		ip   string
		port int
		want string
	}{
		{"tcp localhost, no port override", "localhost:4223", "172.17.0.5", 0, "172.17.0.5:4223"},
		{"tcp localhost, port override", "localhost:4223", "172.17.0.5", 4222, "172.17.0.5:4222"},
		{"tcp 127.0.0.1, port override", "127.0.0.1:4223", "172.17.0.5", 4222, "172.17.0.5:4222"},
		{"http url, port override", "http://localhost:4223/healthz", "172.17.0.5", 4222, "http://172.17.0.5:4222/healthz"},
		{"no host match, untouched", "example.com:4223", "172.17.0.5", 4222, "example.com:4223"},
		{"empty address, untouched", "", "172.17.0.5", 4222, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := swapHostWithIP(tc.addr, tc.ip, tc.port)
			if got != tc.want {
				t.Errorf("swapHostWithIP(%q, %q, %d) = %q, want %q", tc.addr, tc.ip, tc.port, got, tc.want)
			}
		})
	}
}

// TestContainerIP exercises containerIP against a fake `docker inspect`
// that prints an IP address, and a failure case.
func TestContainerIP(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := fakeRuntimeBin(t, "docker", withStdout("172.17.0.5"))
		withFakePath(t, dir)

		ip, err := containerIP(context.Background(), "docker", "api")
		if err != nil {
			t.Fatalf("containerIP: %v", err)
		}
		if ip != "172.17.0.5" {
			t.Errorf("containerIP = %q, want %q", ip, "172.17.0.5")
		}
	})

	t.Run("no network IP", func(t *testing.T) {
		dir := fakeRuntimeBin(t, "docker", withStdout(""))
		withFakePath(t, dir)

		if _, err := containerIP(context.Background(), "docker", "api"); err == nil {
			t.Fatal("expected an error when inspect reports no IP")
		}
	})

	t.Run("inspect fails", func(t *testing.T) {
		dir := fakeRuntimeBin(t, "docker", withExitCode(1))
		withFakePath(t, dir)

		if _, err := containerIP(context.Background(), "docker", "api"); err == nil {
			t.Fatal("expected an error when inspect exits non-zero")
		}
	})
}

// TestCheckReadyContainerAware_FallsBackToContainerIP is a regression
// test for issue #56 (DooD readiness): a TCP readiness check addressed
// at "localhost:<host-published-port>" fails (simulating devx itself
// running in a sibling container that can't reach the host's published
// port), but succeeds once retried against the container's own IP and
// internal port — which is what CheckReadyContainerAware should do
// automatically as a fallback.
func TestCheckReadyContainerAware_FallsBackToContainerIP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	containerPort := ln.Addr().(*net.TCPAddr).Port

	// A guaranteed-closed port on localhost, standing in for a
	// host-published port devx's own (sibling) container can't reach.
	ready := &config.ReadyCheck{TCP: "127.0.0.1:1"}

	dir := fakeRuntimeBin(t, "docker", withStdout("127.0.0.1"))
	withFakePath(t, dir)

	err = CheckReadyContainerAware(context.Background(), ready, ".", "docker", "api", containerPort)
	if err != nil {
		t.Fatalf("CheckReadyContainerAware should have fallen back to the container-IP:port and succeeded, got: %v", err)
	}
}

// TestCheckReadyContainerAware_CmdCheckSkipsFallback confirms a ready.cmd:
// check never triggers the container-IP fallback — the user already
// fully controls that command (matches the design note in driver.go).
func TestCheckReadyContainerAware_CmdCheckSkipsFallback(t *testing.T) {
	ready := &config.ReadyCheck{Cmd: "exit 1"}
	// No fake docker binary on PATH at all: if the fallback were
	// (incorrectly) attempted, containerIP's exec.CommandContext call
	// would fail with "executable file not found", a different error
	// than the expected ready.cmd failure.
	err := CheckReadyContainerAware(context.Background(), ready, ".", "docker", "api", 0)
	if err == nil {
		t.Fatal("expected the failing ready.cmd: to propagate")
	}
}
