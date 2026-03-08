package llm

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"gogogot/llm/openrouter"
)

type Provider struct {
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	Model           string  `json:"model"`
	BaseURL         string  `json:"base_url,omitempty"`
	APIKey          string  `json:"-"`
	Format          string  `json:"format,omitempty"`
	ContextWindow   int     `json:"context_window"`
	SupportsVision  bool    `json:"supports_vision,omitempty"`
	InputPricePerM  float64 `json:"-"`
	OutputPricePerM float64 `json:"-"`
}

var aliases = map[string]string{
	"claude":   "claude-sonnet-4-6",
	"deepseek": "deepseek/deepseek-v3.2",
	"gemini":   "google/gemini-3-pro-preview",
	"minimax":  "minimax/minimax-m2.5",
	"qwen":     "qwen/qwen3.5-397b-a17b",
	"llama":    "meta-llama/llama-4-maverick",
	"kimi":     "moonshotai/kimi-k2.5",
}

var defaultOrder = []string{"claude", "deepseek", "gemini", "qwen", "llama", "kimi", "minimax"}

type anthropicDef struct {
	Label         string
	ContextWindow int
	Vision        bool
	InputPerM     float64
	OutputPerM    float64
}

var anthropicModels = map[string]anthropicDef{
	"claude-sonnet-4-6": {
		Label: "Claude Sonnet 4.6", ContextWindow: 1_000_000,
		Vision: true, InputPerM: 3.0, OutputPerM: 15.0,
	},
}

var (
	catalogOnce sync.Once
	catalogData map[string]openrouter.ModelInfo
)

func getCatalog() map[string]openrouter.ModelInfo {
	catalogOnce.Do(func() {
		catalogData = openrouter.LoadCatalog()
	})
	return catalogData
}

// ResolveProvider builds a Provider from a model ID.
// Accepts a short alias ("deepseek") or a full OpenRouter slug ("deepseek/deepseek-v3.2").
func ResolveProvider(modelID string) (*Provider, error) {
	slug := modelID
	if resolved, ok := aliases[modelID]; ok {
		slug = resolved
	}

	if strings.Contains(slug, "/") {
		return resolveOpenRouter(modelID, slug)
	}

	if _, ok := anthropicModels[slug]; ok {
		return resolveAnthropic(modelID, slug)
	}

	return nil, fmt.Errorf("unknown model %q — use an alias (%s) or a full OpenRouter slug (vendor/model)",
		modelID, strings.Join(aliasKeys(), ", "))
}

// DefaultProvider returns the first available provider based on set API keys.
func DefaultProvider() (*Provider, error) {
	for _, id := range defaultOrder {
		p, err := ResolveProvider(id)
		if err == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("no LLM providers available — set ANTHROPIC_API_KEY or OPENROUTER_API_KEY")
}

func resolveAnthropic(id, slug string) (*Provider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set for model %q", id)
	}

	def := anthropicModels[slug]
	p := &Provider{
		ID: id, Label: def.Label, Model: slug,
		APIKey: apiKey, Format: "anthropic",
		ContextWindow: def.ContextWindow, SupportsVision: def.Vision,
		InputPricePerM: def.InputPerM, OutputPricePerM: def.OutputPerM,
	}
	return p, nil
}

func resolveOpenRouter(id, slug string) (*Provider, error) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY not set for model %q", id)
	}

	p := &Provider{
		ID: id, Model: slug,
		BaseURL: "https://openrouter.ai/api/v1",
		APIKey:  apiKey, Format: "openai",
	}

	if entry, ok := getCatalog()[slug]; ok {
		p.Label = entry.Name
		p.ContextWindow = entry.ContextLength
		p.SupportsVision = entry.Vision
		p.InputPricePerM = entry.InputPricePerM
		p.OutputPricePerM = entry.OutputPricePerM
	}

	return p, nil
}

func aliasKeys() []string {
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	return keys
}
