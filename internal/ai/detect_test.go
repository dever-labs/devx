package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetect_ParsesJSONResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Message: openAIMessage{Content: `{"DATABASE_URL":"postgres://db:5432/mydb"}`}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := New("ollama", "llama3", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	dir := t.TempDir()
	got, err := Detect(context.Background(), client, "postgres", map[string]string{"host": "db", "port": "5432"}, dir)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got["DATABASE_URL"] != "postgres://db:5432/mydb" {
		t.Errorf("Detect() = %+v, want DATABASE_URL set", got)
	}
}

func TestDetect_StripsMarkdownCodeFences(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Message: openAIMessage{Content: "```json\n{\"FOO\":\"bar\"}\n```"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := New("ollama", "llama3", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := Detect(context.Background(), client, "redis", map[string]string{}, t.TempDir())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if got["FOO"] != "bar" {
		t.Errorf("Detect() = %+v, want FOO=bar after fence stripping", got)
	}
}

func TestDetect_NonJSONResponseErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Message: openAIMessage{Content: "I'm not JSON, sorry!"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := New("ollama", "llama3", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = Detect(context.Background(), client, "postgres", map[string]string{}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "non-JSON") {
		t.Errorf("Detect() error = %v, want it to mention non-JSON response", err)
	}
}

func TestDetect_LLMCallFailureErrors(t *testing.T) {
	client, err := New("ollama", "llama3", "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = Detect(context.Background(), client, "postgres", map[string]string{}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "AI detect call failed") {
		t.Errorf("Detect() error = %v, want it to mention the AI call failure", err)
	}
}
