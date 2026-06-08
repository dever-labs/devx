package localai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"
)

const (
	MLXPort    = 8080
	OllamaPort = 11434

	BackendMLX    = "mlx"
	BackendOllama = "ollama"
	BackendAuto   = "auto"
	BackendRemote = "remote"
)

// Status describes a running local AI backend.
type Status struct {
	Backend  string // mlx | ollama
	URL      string // base URL (no trailing slash)
	APIURL   string // OpenAI-compatible /v1 URL
	Model    string // currently loaded model (best effort)
	Provider string // "openai" (mlx) or "ollama"
}

// Detect probes for a running local AI backend.
// Order: MLX (port 8080) → Ollama (port 11434) → Remote endpoint (if provided).
// If preferredBackend is "mlx" or "ollama", only that backend is tried first.
func Detect(preferredBackend string) (*Status, error) {
	return DetectWithEndpoint(preferredBackend, "")
}

// DetectWithEndpoint is like Detect but also probes a remote endpoint when
// preferredBackend is "remote".
func DetectWithEndpoint(preferredBackend, endpoint string) (*Status, error) {
	client := &http.Client{Timeout: 2 * time.Second}

	if preferredBackend == BackendRemote {
		if s := probeRemote(client, endpoint); s != nil {
			return s, nil
		}
		return nil, nil
	}
	if preferredBackend != BackendOllama {
		if s := probeMLX(client); s != nil {
			return s, nil
		}
	}
	if preferredBackend != BackendMLX {
		if s := probeOllama(client); s != nil {
			return s, nil
		}
	}
	return nil, nil
}

func probeMLX(client *http.Client) *Status {
	url := fmt.Sprintf("http://host.docker.internal:%d", MLXPort)
	if !checkURL(client, url+"/v1/models") {
		url = fmt.Sprintf("http://localhost:%d", MLXPort)
		if !checkURL(client, url+"/v1/models") {
			return nil
		}
	}
	model := fetchMLXModel(client, url)
	return &Status{
		Backend:  BackendMLX,
		URL:      url,
		APIURL:   url + "/v1",
		Model:    model,
		Provider: "openai",
	}
}

func probeOllama(client *http.Client) *Status {
	url := fmt.Sprintf("http://host.docker.internal:%d", OllamaPort)
	if !checkURL(client, url+"/api/tags") {
		url = fmt.Sprintf("http://localhost:%d", OllamaPort)
		if !checkURL(client, url+"/api/tags") {
			return nil
		}
	}
	return &Status{
		Backend:  BackendOllama,
		URL:      url,
		APIURL:   url + "/v1",
		Model:    "",
		Provider: "ollama",
	}
}

func probeRemote(client *http.Client, endpoint string) *Status {
	if endpoint == "" {
		return nil
	}
	base := strings.TrimRight(endpoint, "/")
	// Try OpenAI-compatible /v1/models first, then Ollama /api/tags.
	if checkURL(client, base+"/v1/models") {
		return &Status{
			Backend:  BackendRemote,
			URL:      base,
			APIURL:   base + "/v1",
			Provider: "openai",
		}
	}
	if checkURL(client, base+"/api/tags") {
		return &Status{
			Backend:  BackendRemote,
			URL:      base,
			APIURL:   base + "/v1",
			Provider: "ollama",
		}
	}
	return nil
}

func checkURL(client *http.Client, url string) bool {
	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func fetchMLXModel(client *http.Client, baseURL string) string {
	resp, err := client.Get(baseURL + "/v1/models")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Data) == 0 {
		return ""
	}
	return result.Data[0].ID
}

// IsAppleSilicon reports whether the current host is Apple Silicon (arm64 macOS).
func IsAppleSilicon() bool {
	return runtime.GOOS == "darwin" && runtime.GOARCH == "arm64"
}

// ResolveBackend returns the effective backend for a given preference string.
// "auto" resolves to "mlx" on Apple Silicon, "ollama" elsewhere.
func ResolveBackend(preference string) string {
	if preference == BackendAuto || preference == "" {
		if IsAppleSilicon() {
			return BackendMLX
		}
		return BackendOllama
	}
	return preference
}
