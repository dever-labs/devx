package localai

import "testing"

func TestMainModels_ReturnsNonEmptySuggestions(t *testing.T) {
	models := MainModels()
	if len(models) == 0 {
		t.Fatal("expected at least one main model suggestion")
	}
	for _, m := range models {
		if m.Name == "" {
			t.Errorf("model suggestion has empty Name: %+v", m)
		}
	}
}

func TestAutocompleteModels_ReturnsNonEmptySuggestions(t *testing.T) {
	models := AutocompleteModels()
	if len(models) == 0 {
		t.Fatal("expected at least one autocomplete model suggestion")
	}
}

func TestCloudModels_KnownProviders(t *testing.T) {
	cases := []struct {
		provider string
		wantMin  int
	}{
		{"anthropic", 1},
		{"openai", 1},
	}
	for _, tc := range cases {
		models := CloudModels(tc.provider)
		if len(models) < tc.wantMin {
			t.Errorf("CloudModels(%q) returned %d models, want at least %d", tc.provider, len(models), tc.wantMin)
		}
	}
}

func TestCloudModels_UnknownProviderReturnsNil(t *testing.T) {
	if got := CloudModels("bogus-provider"); got != nil {
		t.Errorf("CloudModels(bogus) = %+v, want nil", got)
	}
}
