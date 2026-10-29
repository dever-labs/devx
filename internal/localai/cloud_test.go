package localai

import "testing"

func TestDetectCloudProviders_ReturnsKnownProviders(t *testing.T) {
	providers := DetectCloudProviders()
	names := map[string]bool{}
	for _, p := range providers {
		names[p.Name] = true
	}
	for _, want := range []string{"anthropic", "openai", "copilot"} {
		if !names[want] {
			t.Errorf("expected DetectCloudProviders() to include %q, got %+v", want, providers)
		}
	}
}

func TestDetectCloudProviders_ConfiguredReflectsEnvVar(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "sk-test-key")

	providers := DetectCloudProviders()
	var anthropic, openai *CloudProvider
	for i := range providers {
		switch providers[i].Name {
		case "anthropic":
			anthropic = &providers[i]
		case "openai":
			openai = &providers[i]
		}
	}
	if anthropic == nil || openai == nil {
		t.Fatal("expected both anthropic and openai providers to be present")
	}
	if anthropic.Configured {
		t.Error("expected anthropic.Configured = false when ANTHROPIC_API_KEY is empty")
	}
	if !openai.Configured {
		t.Error("expected openai.Configured = true when OPENAI_API_KEY is set")
	}
}

func TestDetectCloudProviders_ProviderShape(t *testing.T) {
	providers := DetectCloudProviders()
	for _, p := range providers {
		if p.Title == "" {
			t.Errorf("provider %q has empty Title", p.Name)
		}
		if len(p.Tools) == 0 {
			t.Errorf("provider %q has no recommended tools", p.Name)
		}
	}
}
