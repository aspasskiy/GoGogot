package llm

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed models.json
var defaultModelsJSON []byte

type Provider struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Model          string `json:"model"`
	BaseURL        string `json:"base_url,omitempty"`
	APIKey         string `json:"-"`
	Format         string `json:"format,omitempty"`
	ContextWindow  int    `json:"context_window"`
	SupportsVision bool   `json:"supports_vision,omitempty"`
}

type providerDef struct {
	ID             string `json:"id"`
	Label          string `json:"label"`
	Model          string `json:"model"`
	BaseURL        string `json:"base_url,omitempty"`
	APIKeyEnv      string `json:"api_key_env"`
	Format         string `json:"format,omitempty"`
	ContextWindow  int    `json:"context_window"`
	SupportsVision bool   `json:"supports_vision,omitempty"`
}

func LoadProviders(dataDir string) ([]Provider, error) {
	data := defaultModelsJSON

	if userFile := filepath.Join(dataDir, "models.json"); fileExists(userFile) {
		b, err := os.ReadFile(userFile)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", userFile, err)
		}
		data = b
	}

	var defs []providerDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("parse models.json: %w", err)
	}

	var providers []Provider
	for _, d := range defs {
		apiKey := os.Getenv(d.APIKeyEnv)
		if apiKey == "" {
			continue
		}
		providers = append(providers, Provider{
			ID:             d.ID,
			Label:          d.Label,
			Model:          d.Model,
			BaseURL:        d.BaseURL,
			APIKey:         apiKey,
			Format:         d.Format,
			ContextWindow:  d.ContextWindow,
			SupportsVision: d.SupportsVision,
		})
	}

	return providers, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
