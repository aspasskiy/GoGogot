package openrouter

import (
	_ "embed"
	"encoding/json"
	"strconv"
	"strings"
)

//go:embed openrouter_models.json
var catalogJSON []byte

type ModelInfo struct {
	Name            string
	ContextLength   int
	Vision          bool
	InputPricePerM  float64
	OutputPricePerM float64
}

type rawModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Architecture struct {
		InputModalities []string `json:"input_modalities"`
	} `json:"architecture"`
}

func LoadCatalog() map[string]ModelInfo {
	var raw struct {
		Data []rawModel `json:"data"`
	}
	if err := json.Unmarshal(catalogJSON, &raw); err != nil {
		return nil
	}
	m := make(map[string]ModelInfo, len(raw.Data))
	for _, r := range raw.Data {
		inputPrice, _ := strconv.ParseFloat(r.Pricing.Prompt, 64)
		outputPrice, _ := strconv.ParseFloat(r.Pricing.Completion, 64)
		m[r.ID] = ModelInfo{
			Name:            r.Name,
			ContextLength:   r.ContextLength,
			Vision:          hasVision(r.Architecture.InputModalities),
			InputPricePerM:  inputPrice * 1_000_000,
			OutputPricePerM: outputPrice * 1_000_000,
		}
	}
	return m
}

func IsOpenRouter(baseURL string) bool {
	return strings.Contains(baseURL, "openrouter.ai")
}

func hasVision(modalities []string) bool {
	for _, m := range modalities {
		if m == "image" {
			return true
		}
	}
	return false
}
