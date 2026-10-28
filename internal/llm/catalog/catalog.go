package catalog

import (
	_ "embed"
	"encoding/json"
	"strconv"
)

type ModelDef struct {
	ID              string  `json:"id"`
	Label           string  `json:"label"`
	ContextWindow   int     `json:"context_window"`
	Vision          bool    `json:"vision"`
	InputPricePerM  float64 `json:"input_price_per_m"`
	OutputPricePerM float64 `json:"output_price_per_m"`

	// Prompt-cache prices; zero when the model has no cache rate or it is
	// unknown (cached tokens are then priced as plain input).
	CacheReadPricePerM  float64 `json:"cache_read_price_per_m,omitempty"`
	CacheWritePricePerM float64 `json:"cache_write_price_per_m,omitempty"`
}

//go:embed openai.json
var openaiJSON []byte

//go:embed anthropic.json
var anthropicJSON []byte

//go:embed openrouter_models.json
var openrouterJSON []byte

func OpenAI() map[string]ModelDef     { return loadClean(openaiJSON) }
func Anthropic() map[string]ModelDef  { return loadClean(anthropicJSON) }
func OpenRouter() map[string]ModelDef { return loadOpenRouter(openrouterJSON) }

func loadClean(data []byte) map[string]ModelDef {
	var models []ModelDef
	if err := json.Unmarshal(data, &models); err != nil {
		return nil
	}
	m := make(map[string]ModelDef, len(models))
	for _, def := range models {
		m[def.ID] = def
	}
	return m
}

// OpenRouter API returns a different schema; normalize into ModelDef.
func loadOpenRouter(data []byte) map[string]ModelDef {
	var raw struct {
		Data []struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
			Pricing       struct {
				Prompt          string `json:"prompt"`
				Completion      string `json:"completion"`
				InputCacheRead  string `json:"input_cache_read"`
				InputCacheWrite string `json:"input_cache_write"`
			} `json:"pricing"`
			Architecture struct {
				InputModalities []string `json:"input_modalities"`
			} `json:"architecture"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	m := make(map[string]ModelDef, len(raw.Data))
	for _, r := range raw.Data {
		inputPrice, _ := strconv.ParseFloat(r.Pricing.Prompt, 64)
		outputPrice, _ := strconv.ParseFloat(r.Pricing.Completion, 64)
		cacheReadPrice, _ := strconv.ParseFloat(r.Pricing.InputCacheRead, 64)
		cacheWritePrice, _ := strconv.ParseFloat(r.Pricing.InputCacheWrite, 64)
		vision := false
		for _, mod := range r.Architecture.InputModalities {
			if mod == "image" {
				vision = true
				break
			}
		}
		m[r.ID] = ModelDef{
			ID:              r.ID,
			Label:           r.Name,
			ContextWindow:   r.ContextLength,
			Vision:          vision,
			InputPricePerM:  inputPrice * 1_000_000,
			OutputPricePerM: outputPrice * 1_000_000,

			CacheReadPricePerM:  cacheReadPrice * 1_000_000,
			CacheWritePricePerM: cacheWritePrice * 1_000_000,
		}
	}
	return m
}
