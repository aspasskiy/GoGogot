package llm

import (
	"fmt"
	"github.com/aspasskiy/gogogot/internal/llm/catalog"
	"os"
	"strings"
	"sync"
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
	"gemini":   "google/gemini-3-flash-preview",
	"minimax":  "minimax/minimax-m2.5",
	"qwen":     "qwen/qwen3.5-397b-a17b",
	"llama":    "meta-llama/llama-4-maverick",
	"kimi":     "moonshotai/kimi-k2.5",
	"openai":   "openai/gpt-5-nano",
}

var anthropicToOpenRouter = map[string]string{
	"claude-opus-4-6":   "anthropic/claude-opus-4.6",
	"claude-sonnet-4-6": "anthropic/claude-sonnet-4.6",
	"claude-opus-4-5":   "anthropic/claude-opus-4.5",
	"claude-sonnet-4-5": "anthropic/claude-sonnet-4.5",
	"claude-haiku-4-5":  "anthropic/claude-haiku-4.5",
	"claude-sonnet-4":   "anthropic/claude-sonnet-4",
	"claude-haiku-3-5":  "anthropic/claude-haiku-3.5",
}

var (
	anthropicOnce    sync.Once
	anthropicCatalog map[string]catalog.ModelDef

	openaiOnce    sync.Once
	openaiCatalog map[string]catalog.ModelDef

	openrouterOnce    sync.Once
	openrouterCatalog map[string]catalog.ModelDef
)

func getAnthropicCatalog() map[string]catalog.ModelDef {
	anthropicOnce.Do(func() { anthropicCatalog = catalog.Anthropic() })
	return anthropicCatalog
}

func getOpenAICatalog() map[string]catalog.ModelDef {
	openaiOnce.Do(func() { openaiCatalog = catalog.OpenAI() })
	return openaiCatalog
}

func getOpenRouterCatalog() map[string]catalog.ModelDef {
	openrouterOnce.Do(func() { openrouterCatalog = catalog.OpenRouter() })
	return openrouterCatalog
}

// ResolveProvider builds a Provider from an exact model ID and provider name.
func ResolveProvider(modelID, provider string) (*Provider, error) {
	if resolved, ok := aliases[modelID]; ok {
		modelID = resolved
	}

	switch provider {
	case "anthropic":
		if strings.Contains(modelID, "/") {
			return nil, fmt.Errorf("model %q is an OpenRouter slug — use GOGOGOT_PROVIDER=openrouter", modelID)
		}
		if _, ok := getAnthropicCatalog()[modelID]; !ok {
			return nil, fmt.Errorf("unknown Anthropic model %q — available: %s", modelID, catalogKeys(getAnthropicCatalog()))
		}
		return resolveAnthropic(modelID)

	case "openai":
		if strings.Contains(modelID, "/") {
			return nil, fmt.Errorf("model %q is an OpenRouter slug — use GOGOGOT_PROVIDER=openrouter", modelID)
		}
		if _, ok := getOpenAICatalog()[modelID]; !ok {
			return nil, fmt.Errorf("unknown OpenAI model %q — available: %s", modelID, catalogKeys(getOpenAICatalog()))
		}
		return resolveOpenAI(modelID)

	case "openrouter":
		if _, ok := getAnthropicCatalog()[modelID]; ok {
			if orSlug, ok := anthropicToOpenRouter[modelID]; ok {
				return resolveOpenRouter(modelID, orSlug)
			}
			return nil, fmt.Errorf("model %q has no OpenRouter equivalent", modelID)
		}
		if _, ok := getOpenAICatalog()[modelID]; ok {
			return resolveOpenRouter(modelID, "openai/"+modelID)
		}
		if !strings.Contains(modelID, "/") {
			return nil, fmt.Errorf("unknown model %q — use a full OpenRouter slug (vendor/model)", modelID)
		}
		return resolveOpenRouter(modelID, modelID)

	default:
		return nil, fmt.Errorf("unknown provider %q — use 'anthropic', 'openai', or 'openrouter'", provider)
	}
}

func resolveAnthropic(model string) (*Provider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set for model %q", model)
	}
	return providerFromDef(getAnthropicCatalog()[model], model, apiKey, "", "anthropic"), nil
}

func resolveOpenAI(model string) (*Provider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set for model %q", model)
	}
	return providerFromDef(getOpenAICatalog()[model], model, apiKey, "https://api.openai.com/v1", "openai"), nil
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

	if def, ok := getOpenRouterCatalog()[slug]; ok {
		p.Label = def.Label
		p.ContextWindow = def.ContextWindow
		p.SupportsVision = def.Vision
		p.InputPricePerM = def.InputPricePerM
		p.OutputPricePerM = def.OutputPricePerM
	}

	return p, nil
}

func providerFromDef(def catalog.ModelDef, model, apiKey, baseURL, format string) *Provider {
	return &Provider{
		ID: model, Label: def.Label, Model: model,
		BaseURL: baseURL, APIKey: apiKey, Format: format,
		ContextWindow: def.ContextWindow, SupportsVision: def.Vision,
		InputPricePerM: def.InputPricePerM, OutputPricePerM: def.OutputPricePerM,
	}
}

func catalogKeys(m map[string]catalog.ModelDef) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return strings.Join(keys, ", ")
}
