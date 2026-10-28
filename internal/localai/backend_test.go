package localai

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestCheckURL_SuccessAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	if !checkURL(client, srv.URL) {
		t.Error("expected checkURL to succeed against a running test server")
	}
	if checkURL(client, "http://127.0.0.1:1") {
		t.Error("expected checkURL to fail against an unreachable address")
	}
}

func TestCheckURL_ServerErrorStillCountsAsReachable(t *testing.T) {
	// Status < 500 counts as "reachable" per checkURL's contract — even a
	// 404 means something is listening and responding.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	if !checkURL(client, srv.URL) {
		t.Error("expected checkURL to return true for HTTP 404 (reachable, client error)")
	}
}

func TestCheckURL_5xxCountsAsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	if checkURL(client, srv.URL) {
		t.Error("expected checkURL to return false for HTTP 500")
	}
}

func TestFetchMLXModel_ParsesModelID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"mistral-7b"},{"id":"other"}]}`))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	got := fetchMLXModel(client, srv.URL)
	if got != "mistral-7b" {
		t.Errorf("fetchMLXModel() = %q, want %q", got, "mistral-7b")
	}
}

func TestFetchMLXModel_EmptyDataReturnsEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	if got := fetchMLXModel(client, srv.URL); got != "" {
		t.Errorf("fetchMLXModel() = %q, want empty string", got)
	}
}

func TestFetchMLXModel_UnreachableReturnsEmptyString(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	if got := fetchMLXModel(client, "http://127.0.0.1:1"); got != "" {
		t.Errorf("fetchMLXModel() = %q, want empty string on unreachable host", got)
	}
}

func TestFetchMLXModel_InvalidJSONReturnsEmptyString(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	if got := fetchMLXModel(client, srv.URL); got != "" {
		t.Errorf("fetchMLXModel() = %q, want empty string on invalid JSON", got)
	}
}

func TestProbeRemote_EmptyEndpointReturnsNil(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	if got := probeRemote(client, ""); got != nil {
		t.Errorf("probeRemote(\"\") = %+v, want nil", got)
	}
}

func TestProbeRemote_OpenAICompatibleEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	got := probeRemote(client, srv.URL+"/")
	if got == nil {
		t.Fatal("expected a Status for an OpenAI-compatible remote endpoint")
	}
	if got.Backend != BackendRemote || got.Provider != "openai" {
		t.Errorf("probeRemote() = %+v, want Backend=remote Provider=openai", got)
	}
	if got.URL != srv.URL {
		t.Errorf("probeRemote().URL = %q, want %q (trailing slash trimmed)", got.URL, srv.URL)
	}
}

func TestProbeRemote_OllamaCompatibleEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// checkURL treats any status < 500 as "reachable", so /v1/models
		// must 5xx here to force the fallback to /api/tags.
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	got := probeRemote(client, srv.URL)
	if got == nil {
		t.Fatal("expected a Status for an Ollama-compatible remote endpoint")
	}
	if got.Provider != "ollama" {
		t.Errorf("probeRemote().Provider = %q, want ollama", got.Provider)
	}
}

func TestProbeRemote_UnreachableReturnsNil(t *testing.T) {
	client := &http.Client{Timeout: 2 * time.Second}
	if got := probeRemote(client, "http://127.0.0.1:1"); got != nil {
		t.Errorf("probeRemote() = %+v, want nil for unreachable endpoint", got)
	}
}

func TestIsAppleSilicon_MatchesRuntime(t *testing.T) {
	want := runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
	if got := IsAppleSilicon(); got != want {
		t.Errorf("IsAppleSilicon() = %v, want %v", got, want)
	}
}

func TestResolveBackend_AutoResolvesByPlatform(t *testing.T) {
	want := BackendOllama
	if IsAppleSilicon() {
		want = BackendMLX
	}
	if got := ResolveBackend(BackendAuto); got != want {
		t.Errorf("ResolveBackend(auto) = %q, want %q", got, want)
	}
	if got := ResolveBackend(""); got != want {
		t.Errorf("ResolveBackend(\"\") = %q, want %q", got, want)
	}
}

func TestResolveBackend_ExplicitPassesThrough(t *testing.T) {
	for _, pref := range []string{BackendMLX, BackendOllama, BackendRemote, "custom"} {
		if got := ResolveBackend(pref); got != pref {
			t.Errorf("ResolveBackend(%q) = %q, want unchanged", pref, got)
		}
	}
}

func TestDetectWithEndpoint_RemotePreferenceUsesEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	status, err := DetectWithEndpoint(BackendRemote, srv.URL)
	if err != nil {
		t.Fatalf("DetectWithEndpoint: %v", err)
	}
	if status == nil || status.Backend != BackendRemote {
		t.Errorf("DetectWithEndpoint(remote) = %+v, want a remote Status", status)
	}
}

func TestDetectWithEndpoint_RemotePreferenceNoEndpointReturnsNil(t *testing.T) {
	status, err := DetectWithEndpoint(BackendRemote, "")
	if err != nil {
		t.Fatalf("DetectWithEndpoint: %v", err)
	}
	if status != nil {
		t.Errorf("DetectWithEndpoint(remote, \"\") = %+v, want nil", status)
	}
}
