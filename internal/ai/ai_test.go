package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_OpenAIRequiresAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := New("openai", "gpt-4o", ""); err == nil {
		t.Fatal("expected an error when OPENAI_API_KEY is unset")
	}

	t.Setenv("OPENAI_API_KEY", "sk-test")
	c, err := New("openai", "gpt-4o", "")
	if err != nil {
		t.Fatalf("New(openai): %v", err)
	}
	if c.baseURL != "https://api.openai.com/v1" {
		t.Errorf("baseURL = %q, want default OpenAI URL", c.baseURL)
	}
}

func TestNew_AnthropicRequiresAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	if _, err := New("anthropic", "claude-sonnet-4-5", ""); err == nil {
		t.Fatal("expected an error when ANTHROPIC_API_KEY is unset")
	}

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	c, err := New("anthropic", "claude-sonnet-4-5", "")
	if err != nil {
		t.Fatalf("New(anthropic): %v", err)
	}
	if c.baseURL != "https://api.anthropic.com" {
		t.Errorf("baseURL = %q, want default Anthropic URL", c.baseURL)
	}
}

func TestNew_AzureOpenAIRequiresKeyAndBaseURL(t *testing.T) {
	t.Setenv("AZURE_OPENAI_KEY", "")
	if _, err := New("azure-openai", "gpt-4o", "https://my-resource.openai.azure.com"); err == nil {
		t.Fatal("expected an error when AZURE_OPENAI_KEY is unset")
	}

	t.Setenv("AZURE_OPENAI_KEY", "azure-test")
	if _, err := New("azure-openai", "gpt-4o", ""); err == nil {
		t.Fatal("expected an error when baseURL is empty for azure-openai")
	}

	c, err := New("azure-openai", "gpt-4o", "https://my-resource.openai.azure.com")
	if err != nil {
		t.Fatalf("New(azure-openai): %v", err)
	}
	if c.baseURL != "https://my-resource.openai.azure.com" {
		t.Errorf("baseURL = %q, want the provided URL", c.baseURL)
	}
}

func TestNew_OllamaNeedsNoAPIKey(t *testing.T) {
	c, err := New("ollama", "llama3", "")
	if err != nil {
		t.Fatalf("New(ollama): %v", err)
	}
	if c.baseURL != "http://localhost:11434" {
		t.Errorf("baseURL = %q, want default Ollama URL", c.baseURL)
	}
	if c.apiKey != "" {
		t.Errorf("apiKey = %q, want empty for ollama", c.apiKey)
	}
}

func TestNew_EmptyProviderOrModelErrors(t *testing.T) {
	if _, err := New("", "gpt-4o", ""); err == nil {
		t.Fatal("expected an error for empty provider")
	}
	if _, err := New("openai", "", ""); err == nil {
		t.Fatal("expected an error for empty model")
	}
}

func TestNew_UnknownProviderErrors(t *testing.T) {
	if _, err := New("bogus-provider", "model", ""); err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
}

func TestClient_OpenAIComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer sk-test" {
			t.Errorf("Authorization header = %q, want Bearer sk-test", auth)
		}
		resp := openAIResponse{Choices: []struct {
			Message openAIMessage `json:"message"`
		}{{Message: openAIMessage{Role: "assistant", Content: "hello from openai"}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-test")
	c, err := New("openai", "gpt-4o", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got != "hello from openai" {
		t.Errorf("complete() = %q, want %q", got, "hello from openai")
	}
}

func TestClient_OpenAICompleteAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "rate limited"},
		})
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-test")
	c, _ := New("openai", "gpt-4o", srv.URL)
	_, err := c.complete(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("complete() error = %v, want it to mention 'rate limited'", err)
	}
}

func TestClient_OpenAICompleteNoChoicesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(openAIResponse{})
	}))
	defer srv.Close()

	t.Setenv("OPENAI_API_KEY", "sk-test")
	c, _ := New("openai", "gpt-4o", srv.URL)
	if _, err := c.complete(context.Background(), "sys", "user"); err == nil {
		t.Fatal("expected an error when no choices are returned")
	}
}

func TestClient_AnthropicComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if key := r.Header.Get("x-api-key"); key != "sk-ant-test" {
			t.Errorf("x-api-key header = %q, want sk-ant-test", key)
		}
		resp := anthropicResponse{Content: []struct {
			Text string `json:"text"`
		}{{Text: "hello from claude"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	c, err := New("anthropic", "claude-sonnet-4-5", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got != "hello from claude" {
		t.Errorf("complete() = %q, want %q", got, "hello from claude")
	}
}

func TestClient_AnthropicCompleteAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]string{"message": "invalid request"},
		})
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	c, _ := New("anthropic", "claude-sonnet-4-5", srv.URL)
	_, err := c.complete(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "invalid request") {
		t.Errorf("complete() error = %v, want it to mention 'invalid request'", err)
	}
}

func TestClient_AnthropicCompleteEmptyContentErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(anthropicResponse{})
	}))
	defer srv.Close()

	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	c, _ := New("anthropic", "claude-sonnet-4-5", srv.URL)
	if _, err := c.complete(context.Background(), "sys", "user"); err == nil {
		t.Fatal("expected an error for empty content")
	}
}

func TestClient_OllamaComplete(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ollamaResponse{Message: openAIMessage{Role: "assistant", Content: "hello from ollama"}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := New("ollama", "llama3", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got != "hello from ollama" {
		t.Errorf("complete() = %q, want %q", got, "hello from ollama")
	}
}

func TestClient_OllamaCompleteErrorField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(ollamaResponse{Error: "model not found"})
	}))
	defer srv.Close()

	c, _ := New("ollama", "llama3", srv.URL)
	_, err := c.complete(context.Background(), "sys", "user")
	if err == nil || !strings.Contains(err.Error(), "model not found") {
		t.Errorf("complete() error = %v, want it to mention 'model not found'", err)
	}
}

func TestClient_AzureOpenAIUsesOpenAICompletePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openAIResponse{Choices: []struct {
			Message openAIMessage `json:"message"`
		}{{Message: openAIMessage{Content: "azure reply"}}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("AZURE_OPENAI_KEY", "azure-test")
	c, err := New("azure-openai", "gpt-4o", srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := c.complete(context.Background(), "sys", "user")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if got != "azure reply" {
		t.Errorf("complete() = %q, want %q", got, "azure reply")
	}
}

func TestPostJSON_HTTPErrorStatusReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request body"))
	}))
	defer srv.Close()

	c, _ := New("ollama", "llama3", srv.URL)
	_, err := c.postJSON(context.Background(), srv.URL, map[string]string{"a": "b"}, nil)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("postJSON() error = %v, want it to mention HTTP 400", err)
	}
}

func TestPostJSON_UnreachableHostReturnsError(t *testing.T) {
	c, _ := New("ollama", "llama3", "http://127.0.0.1:1")
	if _, err := c.postJSON(context.Background(), "http://127.0.0.1:1/x", nil, nil); err == nil {
		t.Fatal("expected an error for an unreachable host")
	}
}
