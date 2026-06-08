package localai

// ModelSuggestion describes a model available for use.
type ModelSuggestion struct {
	Name        string // model tag (Ollama/MLX style)
	Description string
	VRAM        string // approximate VRAM / disk needed
}

// MainModels returns suggested main chat/reasoning models for local inference.
// These names work for both MLX and Ollama — mlx-setup maps them to HuggingFace repos.
func MainModels() []ModelSuggestion {
	return []ModelSuggestion{
		{"deepseek-r1:70b", "best reasoning, autonomous coding tasks", "~40 GB"},
		{"qwen2.5-coder:32b", "strong code generation, fast", "~18 GB"},
		{"qwen3:30b-a3b", "efficient MoE model, quick responses", "~18 GB"},
		{"qwen2.5-coder:14b", "great on 16 GB RAM machines", "~9 GB"},
		{"qwen2.5-coder:7b", "lightweight, fast iteration", "~5 GB"},
	}
}

// AutocompleteModels returns fast models suited for inline tab completion.
func AutocompleteModels() []ModelSuggestion {
	return []ModelSuggestion{
		{"qwen2.5-coder:1.5b", "very fast, minimal RAM usage", "~1 GB"},
		{"qwen2.5-coder:7b", "balanced speed and quality", "~5 GB"},
	}
}

// CloudModels returns suggested models for a given cloud provider.
func CloudModels(provider string) []ModelSuggestion {
	switch provider {
	case "anthropic":
		return []ModelSuggestion{
			{"claude-sonnet-4-5", "fast, capable — best for daily coding", ""},
			{"claude-opus-4", "most capable — complex reasoning tasks", ""},
			{"claude-haiku-4-5", "fastest and cheapest", ""},
		}
	case "openai":
		return []ModelSuggestion{
			{"gpt-4o", "fast, capable, good all-rounder", ""},
			{"o3", "best reasoning, slower", ""},
			{"gpt-4o-mini", "fast and cheap", ""},
		}
	default:
		return nil
	}
}
